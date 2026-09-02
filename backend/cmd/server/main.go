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

	"github.com/alfirus/controlcenter/backend/internal/agents"
	"github.com/alfirus/controlcenter/backend/internal/auth"
	"github.com/alfirus/controlcenter/backend/internal/calendar"
	"github.com/alfirus/controlcenter/backend/internal/comm"
	"github.com/alfirus/controlcenter/backend/internal/config"
	gh "github.com/alfirus/controlcenter/backend/internal/github"
	"github.com/alfirus/controlcenter/backend/internal/ide"
	"github.com/alfirus/controlcenter/backend/internal/projects"
	"github.com/alfirus/controlcenter/backend/internal/terminal"
)

func main() {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	cfg := config.Load()

	var pool *pgxpool.Pool
	if cfg.DatabaseURL == "" {
		logger.Warn().Msg("DATABASE_URL empty — running without DB (dev/CI)")
	} else {
		var err error
		pool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			logger.Warn().Err(err).Msg("db pool init failed, continuing without DB")
			pool = nil
		} else {
			defer pool.Close()
		}
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

	commH := &comm.Handler{Pool: pool}
	projH := &projects.Handler{Pool: pool}
	calH := &calendar.Handler{Pool: pool}
	agentH := &agents.Handler{Pool: pool}
	ghH := &gh.Handler{Pool: pool, WebhookSecret: cfg.GithubWebhookSecret}
	termH := &terminal.Handler{Pool: pool}
	ideH := &ide.Handler{Pool: pool}

	// public webhook (HMAC verified, no JWT) — POST /v1/webhooks/github
	r.Route("/v1/webhooks", func(r chi.Router) { r.Post("/github", ghH.Webhook) })

	r.Route("/v1", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "v1", "docs": "/api/openapi.yaml"})
		})

		// authenticated routes — Middleware handles dev/CI when SUPABASE_JWT_SECRET empty (injects dev user)
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(cfg.SupabaseJWTSecret))
			commH.Routes(r)
			projH.Routes(r)
			calH.Routes(r)
			agentH.Routes(r)
			termH.Routes(r)
			ideH.Routes(r)

			r.Get("/github/install", ghH.Install)

			r.Get("/oauth/google/start", func(w http.ResponseWriter, req *http.Request) {
				if cfg.GoogleClientID == "" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(503)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "unavailable", "message": "google oauth not configured"}})
					return
				}
				url := "https://accounts.google.com/o/oauth2/auth?client_id=" + cfg.GoogleClientID + "&redirect_uri=" + cfg.GoogleRedirectURL + "&response_type=code&scope=https://www.googleapis.com/auth/calendar&access_type=offline&prompt=consent"
				http.Redirect(w, req, url, http.StatusFound)
			})
			r.Get("/oauth/google/callback", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "callback — token exchange TODO Phase 4"})
			})

			r.Post("/invites/{token}/accept", func(w http.ResponseWriter, req *http.Request) {
				token := chi.URLParam(req, "token")
				userID := auth.UserID(req.Context())
				if pool == nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(503)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "unavailable", "message": "db not configured"}})
					return
				}
				var wsID, role string
				err := pool.QueryRow(req.Context(), `select workspace_id, role from invites where token=$1 and expires_at > now()`, token).Scan(&wsID, &role)
				if err != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(404)
					_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "not_found", "message": "invalid or expired invite"}})
					return
				}
				_, _ = pool.Exec(req.Context(), `insert into workspace_members(workspace_id, user_id, role) values($1,$2,$3) on conflict do update set role=$3`, wsID, userID, role)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "workspace_id": wsID})
			})
		})
	})

	addr := ":" + cfg.Port
	logger.Info().Str("addr", addr).Msg("starting backend")
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
