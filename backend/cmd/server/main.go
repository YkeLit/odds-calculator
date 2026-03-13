package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"odds-calculator/backend/internal/api"
	"odds-calculator/backend/internal/auth"
	"odds-calculator/backend/internal/holdem"
	"odds-calculator/backend/internal/storage"
)

func main() {
	port := envOrDefault("PORT", "8080")
	dbPath := envOrDefault("DB_PATH", "./odds.db")
	jwtSecret := envOrDefault("JWT_SECRET", "dev-secret-change-me")
	cachePath := envOrDefault("MCCFR_CACHE_PATH", "./mccfr_cache.bin")

	store, err := storage.New(dbPath)
	if err != nil {
		log.Fatalf("failed to init storage: %v", err)
	}
	defer store.Close()

	// Load MCCFR InfoSet cache from disk (best-effort: missing file is not an error)
	if err := holdem.LoadCacheFromFile(cachePath); err != nil {
		log.Printf("warn: could not load MCCFR cache (%s): %v", cachePath, err)
	} else {
		log.Printf("MCCFR cache loaded from %s", cachePath)
	}

	authService := auth.NewService(store, jwtSecret, 24*time.Hour)
	server := api.NewServer(authService, store)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: server.Routes(),
	}

	// Periodic auto-save every 10 minutes to protect against ungraceful kills.
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := holdem.SaveCacheToFile(cachePath); err != nil {
					log.Printf("warn: MCCFR cache auto-save failed: %v", err)
				} else {
					log.Printf("MCCFR cache auto-saved to %s", cachePath)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Graceful shutdown on SIGINT / SIGTERM.
	go func() {
		<-ctx.Done()
		log.Println("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("odds-calculator backend listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server exited: %v", err)
	}

	// Save cache after the server has fully stopped.
	if err := holdem.SaveCacheToFile(cachePath); err != nil {
		log.Printf("warn: could not save MCCFR cache: %v", err)
	} else {
		log.Printf("MCCFR cache saved to %s", cachePath)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
