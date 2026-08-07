package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/erikhoward/souschef/internal/config"
	"github.com/erikhoward/souschef/internal/enrich"
	"github.com/erikhoward/souschef/internal/httpapi"
	"github.com/erikhoward/souschef/internal/ideas"
	"github.com/erikhoward/souschef/internal/store"
	"github.com/erikhoward/souschef/internal/telegram"
	"github.com/erikhoward/souschef/internal/transcribe"
)

// version is overridden at build time via -ldflags.
var version = "0.1.0-dev"

// enrichDrainGrace bounds how long shutdown waits for in-flight background
// enrichment (internal/httpapi's EnrichInBackground) to finish before the
// store is closed underneath it. A hung upstream call must not block exit
// forever, so this is generous but not unlimited.
const enrichDrainGrace = 30 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "souschef %s failed: %v\n", version, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}

	hub := httpapi.NewHub()
	defer hub.Close()

	ideaService := ideas.NewService(st)
	enricher := enrich.New(cfg.Model, cfg.Effort)

	api := httpapi.New(httpapi.Deps{
		Ideas:    ideaService,
		Enricher: enricher,
		Hub:      hub,
	})

	// st.Close() is deferred, so it fires last, after run() returns. That is
	// still too early on its own: EnrichInBackground detaches goroutines
	// with their own 5-minute timeout, and if one is still mid-write when
	// the store closes, the write fails with "sql: database is closed" and
	// the idea is stuck at enrichment_status = 'pending' with its result
	// silently lost. The shutdown path below calls api.Drain(...) inline —
	// not via another defer — precisely so it blocks before this function
	// returns and st.Close() runs, rather than racing it.
	defer st.Close()

	srv := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", cfg.Port),
		Handler:           httpapi.WithStatic(api),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: it would sever the SSE stream on a fixed interval.
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("souschef %s listening on http://127.0.0.1:%d (db=%s, model=%s)",
			version, cfg.Port, cfg.DBPath, cfg.Model)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server: %v", err)
			stop()
		}
	}()

	telegramClient := telegram.NewClient(cfg.TelegramToken)
	bot, err := telegram.New(telegram.Deps{
		Client:      telegramClient,
		Ideas:       ideaService,
		Enricher:    enricher,
		Transcriber: transcribe.New(cfg.WhisperBin, cfg.WhisperModel),
		ChatID:      cfg.TelegramChatID,
		AudioDir:    cfg.AudioDir,
		WebBaseURL:  fmt.Sprintf("http://127.0.0.1:%d", cfg.Port),
	})
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}

	go func() {
		if err := bot.Run(ctx); err != nil {
			log.Printf("telegram: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down…")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)

	// The HTTP server no longer accepts requests once Shutdown returns, so
	// no new enrichment goroutines can start. Drain the ones already
	// running before the deferred st.Close() above executes.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), enrichDrainGrace)
	defer drainCancel()
	api.Drain(drainCtx)

	return shutdownErr
}
