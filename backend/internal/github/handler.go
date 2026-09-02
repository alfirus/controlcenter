package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	Pool          *pgxpool.Pool
	WebhookSecret string
}

func (h *Handler) Routes(r chi.Router) {
	r.Post("/webhooks/github", h.webhook)
	r.Get("/github/install", h.install)
}

// PublicRoutes mounts only the public webhook (no auth).
func (h *Handler) PublicRoutes(r chi.Router) { r.Post("/github", h.webhook) }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}
func writeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func (h *Handler) Install(w http.ResponseWriter, r *http.Request) { h.install(w, r) }
func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) { h.webhook(w, r) }
func (h *Handler) install(w http.ResponseWriter, r *http.Request) {
	// Redirect to GitHub App install — app id configured via GITHUB_APP_ID
	writeJSON(w, 200, map[string]string{"status": "configure GITHUB_APP_ID and visit https://github.com/apps/<app>/installations/new"})
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	if h.WebhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		mac := hmac.New(sha256.New, []byte(h.WebhookSecret))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(sig), []byte(expected)) {
			writeErr(w, 401, "unauthorized", "invalid webhook signature")
			return
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeErr(w, 400, "bad_request", "invalid json")
		return
	}
	event := r.Header.Get("X-GitHub-Event")
	if h.Pool != nil {
		// dead-letter on parse failure, otherwise upsert task for issues
		if event == "issues" {
			_ = h.handleIssueEvent(r, payload)
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok", "event": event})
}

func (h *Handler) handleIssueEvent(r *http.Request, payload map[string]any) error {
	action, _ := payload["action"].(string)
	issue, _ := payload["issue"].(map[string]any)
	repo, _ := payload["repository"].(map[string]any)
	if issue == nil || repo == nil {
		return nil
	}
	repoName, _ := repo["full_name"].(string)
	issueNumF, _ := issue["number"].(float64)
	title, _ := issue["title"].(string)
	body, _ := issue["body"].(string)
	state, _ := issue["state"].(string)

	status := "todo"
	if state == "closed" {
		status = "done"
	} else if action == "opened" || action == "reopened" {
		status = "todo"
	}
	// find linked project for this repo
	var projectID string
	err := h.Pool.QueryRow(r.Context(), `select project_id from github_repo_links where repo=$1 limit 1`, repoName).Scan(&projectID)
	if err != nil {
		// no link — store dead letter for manual linking
		_, _ = h.Pool.Exec(r.Context(), `insert into webhook_dead_letters(source, payload) values('github',$1)`, payload)
		return nil
	}
	issueID := int64(issueNumF)
	_, err = h.Pool.Exec(r.Context(),
		`insert into tasks(project_id, title, body, status, github_repo, github_issue_id)
		 values($1,$2,$3,$4,$5,$6)
		 on conflict(github_repo, github_issue_id) where github_issue_id is not null
		 do update set title=$2, body=$3, status=$4, updated_at=now()`,
		projectID, title, body, status, repoName, issueID)
	return err
}
