// Package httpapi exposes the REST surface and the SSE stream. It owns no SQL
// and no domain rules — every decision is delegated to internal/ideas.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/erikhoward/souschef/internal/ideas"
	"github.com/erikhoward/souschef/internal/store"
)

// Enricher is the slice of internal/enrich this package needs. Declaring it
// here keeps HTTP tests runnable with a stub and no API key.
type Enricher interface {
	Enrich(ctx context.Context, rawText string) (ideas.Metadata, error)
	// Model names the model doing the classifying. It is recorded on every
	// enriched idea so a later model change can find and re-run exactly the
	// ideas the old one classified.
	Model() string
}

type Deps struct {
	Ideas    *ideas.Service
	Enricher Enricher
	Hub      *Hub
}

type Server struct {
	ideas    *ideas.Service
	enricher Enricher
	hub      *Hub
	mux      *http.ServeMux

	// wg and inFlight track background enrichment goroutines started by
	// EnrichInBackground, so Drain can wait for them before the caller
	// closes the store. See Drain for why this matters.
	wg       sync.WaitGroup
	inFlight atomic.Int64
}

func New(deps Deps) *Server {
	s := &Server{ideas: deps.Ideas, enricher: deps.Enricher, hub: deps.Hub}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/ideas", s.listIdeas)
	mux.HandleFunc("POST /api/ideas", s.createIdea)
	mux.HandleFunc("GET /api/ideas/{id}", s.getIdea)
	mux.HandleFunc("PATCH /api/ideas/{id}", s.patchIdea)
	mux.HandleFunc("DELETE /api/ideas/{id}", s.deleteIdea)
	mux.HandleFunc("POST /api/ideas/{id}/archive", s.archiveIdea)
	mux.HandleFunc("POST /api/ideas/{id}/restore", s.restoreIdea)
	mux.HandleFunc("POST /api/ideas/{id}/reenrich", s.reenrichIdea)
	mux.HandleFunc("POST /api/ideas/{id}/notes", s.addNote)
	mux.HandleFunc("POST /api/ideas/{id}/links", s.addLink)
	mux.HandleFunc("DELETE /api/ideas/{id}/links/{other}", s.removeLink)
	mux.HandleFunc("POST /api/ideas/{id}/merge", s.mergeIdea)
	mux.HandleFunc("GET /events", s.hub.ServeHTTP)
	s.mux = mux
	return s
}

// ServeHTTP makes *Server itself an http.Handler, so callers that don't need
// the drain machinery can keep treating it as one.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// Drain waits for in-flight background enrichment goroutines (started by
// EnrichInBackground) to finish, bounded by ctx.
//
// This must run after the HTTP server has stopped accepting new requests
// but before the caller closes the store: EnrichInBackground detaches a
// goroutine with its own 5-minute timeout, and if the store closes while
// one is still writing its result, the write fails with "sql: database is
// closed", the error is logged and swallowed, and the idea is left stuck
// at enrichment_status = 'pending' with its result silently lost.
//
// If ctx is done before every goroutine finishes (a hung upstream call
// must not block process exit forever), Drain logs how many were still
// running rather than exiting silently.
func (s *Server) Drain(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		if n := s.inFlight.Load(); n > 0 {
			log.Printf("httpapi: shutdown grace period expired with %d enrichment goroutine(s) still running", n)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("httpapi: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeDomainError maps domain errors onto status codes in one place so no
// handler has to remember the mapping.
func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ideas.ErrEmptyText),
		errors.Is(err, ideas.ErrTooLong),
		errors.Is(err, ideas.ErrSelfLink),
		errors.Is(err, ideas.ErrSelfMerge):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("httpapi: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON body: "+err.Error())
		return false
	}
	return true
}

var validSources = map[ideas.Source]bool{
	ideas.SourceWeb:           true,
	ideas.SourceTelegramText:  true,
	ideas.SourceTelegramVoice: true,
}

func (s *Server) createIdea(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RawText   string       `json:"raw_text"`
		Source    ideas.Source `json:"source"`
		SourceRef string       `json:"source_ref"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Source == "" {
		body.Source = ideas.SourceWeb
	}
	if !validSources[body.Source] {
		writeError(w, http.StatusBadRequest, "unknown source: "+string(body.Source))
		return
	}

	idea, err := s.ideas.Create(r.Context(), body.RawText, body.Source, body.SourceRef)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	s.hub.Broadcast(Event{Type: "idea.created", Idea: &idea})

	// Respond now; classify in the background. Capture must never block on
	// the network — that is the property the whole design is built around.
	s.EnrichInBackground(idea.ID, idea.RawText)

	writeJSON(w, http.StatusCreated, idea)
}

// EnrichInBackground classifies an idea and pushes the result over SSE. It
// runs detached from the request, so it uses its own context.
//
// The goroutine is tracked via wg/inFlight so Drain can wait for it to
// finish before the store is closed underneath it.
func (s *Server) EnrichInBackground(id, rawText string) {
	s.wg.Add(1)
	s.inFlight.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.inFlight.Add(-1)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		updated, err := s.enrichOnce(ctx, id, rawText)
		if err != nil {
			log.Printf("httpapi: enrich %s: %v", id, err)
			return
		}
		s.hub.Broadcast(Event{Type: "idea.updated", Idea: &updated})
	}()
}

// enrichOnce runs a single classification and records the outcome either way.
// A failure is a recorded state, not a dropped request.
func (s *Server) enrichOnce(ctx context.Context, id, rawText string) (ideas.Idea, error) {
	meta, err := s.enricher.Enrich(ctx, rawText)
	if err != nil {
		return s.ideas.RecordEnrichmentFailure(ctx, id, err.Error())
	}
	return s.ideas.ApplyEnrichment(ctx, id, meta, s.enricher.Model())
}

func (s *Server) reenrichIdea(w http.ResponseWriter, r *http.Request) {
	idea, err := s.ideas.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}

	// Synchronous, unlike creation: the caller pressed Retry and is waiting
	// for an answer, so the outcome belongs in this response.
	updated, err := s.enrichOnce(r.Context(), idea.ID, idea.RawText)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.hub.Broadcast(Event{Type: "idea.updated", Idea: &updated})
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) listIdeas(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit, _ := strconv.Atoi(q.Get("limit"))
	filter := ideas.ListFilter{
		Query:      q.Get("q"),
		Stage:      q.Get("stage"),
		Difficulty: q.Get("difficulty"),
		Duration:   q.Get("duration"),
		Treatment:  q.Get("treatment"),
		Archived:   ideas.ArchivedScope(q.Get("archived")),
		Sort:       q.Get("sort"),
		Order:      q.Get("order"),
		Limit:      limit,
	}

	out, err := s.ideas.List(r.Context(), filter)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if out == nil {
		out = []ideas.Idea{} // never serialise null — the UI calls .map on this
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getIdea(w http.ResponseWriter, r *http.Request) {
	idea, err := s.ideas.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, idea)
}

func (s *Server) patchIdea(w http.ResponseWriter, r *http.Request) {
	var patch map[string]any
	if !decodeBody(w, r, &patch) {
		return
	}

	updated, err := s.ideas.Correct(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		// Correct returns a plain error for an unknown or wrongly-typed field.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.hub.Broadcast(Event{Type: "idea.updated", Idea: &updated})
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteIdea(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.ideas.Delete(r.Context(), id); err != nil {
		writeDomainError(w, err)
		return
	}
	s.hub.Broadcast(Event{Type: "idea.deleted", ID: id})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) archiveIdea(w http.ResponseWriter, r *http.Request) { s.setArchived(w, r, true) }
func (s *Server) restoreIdea(w http.ResponseWriter, r *http.Request) { s.setArchived(w, r, false) }

func (s *Server) setArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	var (
		updated ideas.Idea
		err     error
	)
	if archived {
		updated, err = s.ideas.Archive(r.Context(), r.PathValue("id"))
	} else {
		updated, err = s.ideas.Restore(r.Context(), r.PathValue("id"))
	}
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.hub.Broadcast(Event{Type: "idea.updated", Idea: &updated})
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) addNote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Body string `json:"body"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if _, err := s.ideas.AddNote(r.Context(), r.PathValue("id"), body.Body); err != nil {
		writeDomainError(w, err)
		return
	}
	s.respondWithIdea(w, r, r.PathValue("id"))
}

func (s *Server) addLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OtherID string `json:"other_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.ideas.Link(r.Context(), r.PathValue("id"), body.OtherID); err != nil {
		writeDomainError(w, err)
		return
	}
	s.respondWithIdea(w, r, r.PathValue("id"))
}

func (s *Server) removeLink(w http.ResponseWriter, r *http.Request) {
	if err := s.ideas.Unlink(r.Context(), r.PathValue("id"), r.PathValue("other")); err != nil {
		writeDomainError(w, err)
		return
	}
	s.respondWithIdea(w, r, r.PathValue("id"))
}

func (s *Server) mergeIdea(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DuplicateID string `json:"duplicate_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	merged, err := s.ideas.Merge(r.Context(), r.PathValue("id"), body.DuplicateID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.hub.Broadcast(Event{Type: "idea.updated", Idea: &merged})
	s.hub.Broadcast(Event{Type: "idea.deleted", ID: body.DuplicateID})
	writeJSON(w, http.StatusOK, merged)
}

// respondWithIdea re-reads and returns the idea, so mutations that touch
// relations always answer with fully hydrated state.
func (s *Server) respondWithIdea(w http.ResponseWriter, r *http.Request, id string) {
	idea, err := s.ideas.Get(r.Context(), id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	s.hub.Broadcast(Event{Type: "idea.updated", Idea: &idea})
	writeJSON(w, http.StatusOK, idea)
}
