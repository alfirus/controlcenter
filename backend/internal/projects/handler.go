package projects

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct{ Pool *pgxpool.Pool }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects", h.list)
	r.Post("/projects", h.create)
	r.Get("/projects/{id}/tasks", h.listTasks)
	r.Post("/projects/{id}/tasks", h.createTask)
	r.Patch("/tasks/{id}", h.updateTask)
	r.Post("/projects/{id}/link-github", h.linkGithub)
}

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

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	wsID := r.URL.Query().Get("workspace_id")
	if wsID == "" {
		writeErr(w, 400, "bad_request", "workspace_id required")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `select id, name, description from projects where workspace_id=$1 and deleted_at is null order by created_at`, wsID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type proj struct {
		ID          string  `json:"id"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	var out []proj
	for rows.Next() {
		var x proj
		_ = rows.Scan(&x.ID, &x.Name, &x.Description)
		out = append(out, x)
	}
	if out == nil {
		out = []proj{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	var body struct {
		WorkspaceID string  `json:"workspace_id"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.WorkspaceID == "" {
		writeErr(w, 400, "bad_request", "workspace_id and name required")
		return
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `insert into projects(workspace_id, name, description) values($1,$2,$3) returning id`, body.WorkspaceID, body.Name, body.Description).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	pid := chi.URLParam(r, "id")
	rows, err := h.Pool.Query(r.Context(), `select id, title, status, github_repo, github_issue_id from tasks where project_id=$1 and deleted_at is null order by created_at`, pid)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type tsk struct {
		ID            string  `json:"id"`
		Title         string  `json:"title"`
		Status        string  `json:"status"`
		GithubRepo    *string `json:"github_repo"`
		GithubIssueID *int64  `json:"github_issue_id"`
	}
	var out []tsk
	for rows.Next() {
		var x tsk
		_ = rows.Scan(&x.ID, &x.Title, &x.Status, &x.GithubRepo, &x.GithubIssueID)
		out = append(out, x)
	}
	if out == nil {
		out = []tsk{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	pid := chi.URLParam(r, "id")
	var body struct {
		Title  string  `json:"title"`
		Body   *string `json:"body"`
		Status *string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" {
		writeErr(w, 400, "bad_request", "title required")
		return
	}
	status := "todo"
	if body.Status != nil {
		status = *body.Status
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `insert into tasks(project_id, title, body, status) values($1,$2,$3,$4) returning id`, pid, body.Title, body.Body, status).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	tid := chi.URLParam(r, "id")
	var body struct {
		Title  *string `json:"title"`
		Status *string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	if body.Status != nil && *body.Status != "todo" && *body.Status != "doing" && *body.Status != "done" {
		writeErr(w, 400, "bad_request", "invalid status")
		return
	}
	// minimal patch
	if body.Title != nil {
		_, _ = h.Pool.Exec(r.Context(), `update tasks set title=$2, updated_at=now() where id=$1`, tid, *body.Title)
	}
	if body.Status != nil {
		_, _ = h.Pool.Exec(r.Context(), `update tasks set status=$2, updated_at=now() where id=$1`, tid, *body.Status)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) linkGithub(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	pid := chi.URLParam(r, "id")
	var body struct {
		Repo           string `json:"repo"`
		InstallationID int64  `json:"installation_id"`
		WorkspaceID    string `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Repo == "" {
		writeErr(w, 400, "bad_request", "repo required")
		return
	}
	_, err := h.Pool.Exec(r.Context(), `insert into github_repo_links(workspace_id, installation_id, repo, project_id) values($1,$2,$3,$4) on conflict(workspace_id, repo) do update set project_id=$4`,
		body.WorkspaceID, body.InstallationID, body.Repo, pid)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "linked"})
}
