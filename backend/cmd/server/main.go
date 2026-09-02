package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chiMW "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/alfirus/controlcenter/backend/internal/config"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	cfg := config.Load()

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		// allow boot without DB for health checks in CI
		logger.Warn().Err(err).Msg("db pool init failed, continuing without DB")
	} else {
		defer pool.Close()
	}

	r := chi.NewRouter()
	r.Use(chiMW.RealIP)
	r.Use(chiMW.Recoverer)
	r.Use(chiMW.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		status := map[string]string{"status": "ok"}
		if pool != nil {
			ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
			defer cancel()
			if err := pool.Ping(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "degraded", "db": err.Error()})
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})

	// v1 placeholder — real handlers wired in Phase 1+
	r.Route("/v1", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "v1", "docs": "/v1/openapi.yaml"})
		})
	})

	addr := ":" + cfg.Port
	logger.Info().Str("addr", addr).Msg("starting backend")
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
