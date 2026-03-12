package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"odds-calculator/backend/internal/api"
	"odds-calculator/backend/internal/auth"
	"odds-calculator/backend/internal/storage"
)

func main() {
	port := envOrDefault("PORT", "8080")
	dbPath := envOrDefault("DB_PATH", "./odds.db")
	jwtSecret := envOrDefault("JWT_SECRET", "dev-secret-change-me")

	store, err := storage.New(dbPath)
	if err != nil {
		log.Fatalf("failed to init storage: %v", err)
	}
	defer store.Close()

	authService := auth.NewService(store, jwtSecret, 24*time.Hour)
	server := api.NewServer(authService, store)

	log.Printf("odds-calculator backend listening on :%s", port)
	if err := http.ListenAndServe(":"+port, server.Routes()); err != nil {
		log.Fatalf("server exited: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
