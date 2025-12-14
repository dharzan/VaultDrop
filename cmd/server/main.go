// Package main is the entry point for the VaultDrop binary. In Go every
// executable program must define package main and a main() function, while
// libraries use other package names.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dharsanguruparan/VaultDrop/internal/config"
	"github.com/dharsanguruparan/VaultDrop/internal/processing"
	"github.com/dharsanguruparan/VaultDrop/internal/server"
	"github.com/dharsanguruparan/VaultDrop/internal/signing"
	"github.com/dharsanguruparan/VaultDrop/internal/storage"
)

func main() {
	// Step 1: load configuration from environment variables (Go prefers
	// returning values + errors rather than throwing exceptions).
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	// Step 2: construct dependencies. In Go it's idiomatic to instantiate
	// structs via constructors that return pointers.
	store := storage.NewMemoryStore()
	processor := processing.New(store, cfg.ProcessingPool)
	signer := signing.NewSigner(cfg.SigningSecret)
	// server.New wires together config + dependencies and prepares HTTP routes.
	srv, err := server.New(cfg, store, processor, signer)
	if err != nil {
		log.Fatalf("init server: %v", err)
	}
	// Step 3: create a context that cancels when SIGINT/SIGTERM arrive. Context
	// is Go's mechanism for cancellation deadlines and propagation.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("VaultDrop listening on %s", cfg.Address)
	// Step 4: block until the HTTP server exits.
	if err := srv.Serve(ctx); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
