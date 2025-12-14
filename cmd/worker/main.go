package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"

	"github.com/dharsanguruparan/VaultDrop/internal/config"
	"github.com/dharsanguruparan/VaultDrop/internal/database"
	"github.com/dharsanguruparan/VaultDrop/internal/repository"
	"github.com/dharsanguruparan/VaultDrop/internal/s3storage"
	"github.com/dharsanguruparan/VaultDrop/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer pool.Close()
	if err := database.EnsureSchema(ctx, pool); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}
	repo := repository.NewDocumentRepository(pool)

	store, err := s3storage.New(cfg)
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	if err := store.EnsureBuckets(ctx); err != nil {
		log.Fatalf("ensure buckets: %v", err)
	}

	server := asynq.NewServer(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}, asynq.Config{
		Concurrency: cfg.ProcessingPool,
	})
	processor := worker.NewProcessor(repo, store)
	mux := processor.Handler()

	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()

	if err := server.Run(mux); err != nil {
		log.Printf("worker stopped: %v", err)
		os.Exit(1)
	}
}
