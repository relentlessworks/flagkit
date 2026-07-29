package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/relentlessworks/flagkit/internal/api"
	"github.com/relentlessworks/flagkit/internal/auth"
	"github.com/relentlessworks/flagkit/internal/config"
	"github.com/relentlessworks/flagkit/internal/store"
)

func main() {
	cfg := config.Load()

	// Initialize store
	st, err := store.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}

	// Initialize auth
	authSvc := auth.New(st)

	// Initialize API
	handler := api.New(st, authSvc)
	mux := handler.Routes()

	// Start server
	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		srv.Close()
	}()

	fmt.Fprintf(os.Stderr, "flagkit starting on %s (db: %s)\n", cfg.Addr, cfg.DBPath)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}
