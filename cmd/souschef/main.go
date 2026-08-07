package main

import (
	"fmt"
	"log"
	"os"

	"github.com/erikhoward/souschef/internal/config"
)

// version is overridden at build time via -ldflags.
var version = "0.1.0-dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "souschef %s failed to start.\n\n%v\n", version, err)
		os.Exit(1)
	}
	log.Printf("souschef %s — db=%s port=%d model=%s", version, cfg.DBPath, cfg.Port, cfg.Model)
}
