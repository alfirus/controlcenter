package comm

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alfirus/controlcenter/backend/internal/auth"
)

type Handler struct{ Pool *pgxpool.Pool }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/workspaces", h.listWorkspaces)
	r.Post("/workspaces", h.createWorkspace)
	r.Get("/workspaces/{id}/members", h.listMembers)
	r.Patch("/workspaces/{id}/members", h.updateMember)

	r.Get("/channels", h.listChannels)
	r.Post("/channels", h.createChannel)
	r.Get("/channels/{id}/messages", h.listMessages)
	r.Post("/channels/{id}/messages", h.postMessage)
	r.Get("/search", h.search)
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

func (h *Handler) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	userID := auth.UserID(r.Context())
	rows, err := h.Pool.Query(r.Context(),
		`select w.id, w.name, w.created_at from workspaces w join workspace_members m on m.workspace_id=w.id where m.user_id=$1 and w.deleted_at is null order by w.created_at`, userID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type ws struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}
	var out []ws
	for rows.Next() {
		var x ws
		_ = rows.Scan(&x.ID, &x.Name, &x.CreatedAt)
		out = append(out, x)
	}
	if out == nil {
		out = []ws{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createWorkspace(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeErr(w, 400, "bad_request", "name required")
		return
	}
	userID := auth.UserID(r.Context())
	var id string
	err := h.Pool.QueryRow(r.Context(),
		`insert into workspaces(name, created_by) values($1,$2) returning id`, body.Name, userID).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	_, _ = h.Pool.Exec(r.Context(), `insert into workspace_members(workspace_id, user_id, role) values($1,$2,'admin') on conflict do nothing`, id, userID)
	writeJSON(w, 201, map[string]string{"id": id, "name": body.Name})
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	id := chi.URLParam(r, "id")
	rows, err := h.Pool.Query(r.Context(), `select user_id, role from workspace_members where workspace_id=$1`, id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type m struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	var out []m
	for rows.Next() {
		var x m
		_ = rows.Scan(&x.UserID, &x.Role)
		out = append(out, x)
	}
	if out == nil {
		out = []m{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) updateMember(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	if body.Role != "admin" && body.Role != "member" && body.Role != "guest" {
		writeErr(w, 400, "bad_request", "invalid role")
		return
	}
	_, err := h.Pool.Exec(r.Context(), `update workspace_members set role=$3 where workspace_id=$1 and user_id=$2`, id, body.UserID, body.Role)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) listChannels(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	wsID := r.URL.Query().Get("workspace_id")
	if wsID == "" {
		writeErr(w, 400, "bad_request", "workspace_id required")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `select id, name, kind from channels where workspace_id=$1 and deleted_at is null order by name`, wsID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type ch struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Kind string `json:"kind"`
	}
	var out []ch
	for rows.Next() {
		var x ch
		_ = rows.Scan(&x.ID, &x.Name, &x.Kind)
		out = append(out, x)
	}
	if out == nil {
		out = []ch{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createChannel(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
		Kind        string `json:"kind"`
		Purpose     string `json:"purpose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.WorkspaceID == "" {
		writeErr(w, 400, "bad_request", "workspace_id and name required")
		return
	}
	if body.Kind == "" {
		body.Kind = "open"
	}
	userID := auth.UserID(r.Context())
	var id string
	err := h.Pool.QueryRow(r.Context(),
		`insert into channels(workspace_id, name, kind, purpose, created_by) values($1,$2,$3,$4,$5) returning id`,
		body.WorkspaceID, body.Name, body.Kind, body.Purpose, userID).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	_, _ = h.Pool.Exec(r.Context(), `insert into channel_members(channel_id, user_id) values($1,$2) on conflict do nothing`, id, userID)
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	chID := chi.URLParam(r, "id")
	limit := 50
	rows, err := h.Pool.Query(r.Context(),
		`select id, channel_id, coalesce(author_id::text,''), coalesce(agent_id::text,''), body, created_at from messages where channel_id=$1 and deleted_at is null order by created_at desc, id desc limit $2`, chID, limit)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type msg struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
		AuthorID  string `json:"author_id"`
		AgentID   string `json:"agent_id"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
	}
	var out []msg
	for rows.Next() {
		var x msg
		_ = rows.Scan(&x.ID, &x.ChannelID, &x.AuthorID, &x.AgentID, &x.Body, &x.CreatedAt)
		out = append(out, x)
	}
	if out == nil {
		out = []msg{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) postMessage(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	chID := chi.URLParam(r, "id")
	var body struct {
		Body   string `json:"body"`
		RootID *string `json:"root_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Body == "" {
		writeErr(w, 400, "bad_request", "body required")
		return
	}
	userID := auth.UserID(r.Context())
	var id string
	err := h.Pool.QueryRow(r.Context(),
		`insert into messages(channel_id, author_id, body, root_id) values($1,$2,$3,$4) returning id`,
		chID, userID, body.Body, body.RootID).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		writeErr(w, 400, "bad_request", "q required")
		return
	}
	rows, err := h.Pool.Query(r.Context(),
		`select id, channel_id, body, ts_rank(search, plainto_tsquery('english',$1)) as rank from messages where search @@ plainto_tsquery('english',$1) and deleted_at is null order by rank desc limit 20`, q)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type hit struct {
		ID        string  `json:"id"`
		ChannelID string  `json:"channel_id"`
		Body      string  `json:"body"`
		Rank      float32 `json:"rank"`
	}
	var out []hit
	for rows.Next() {
		var x hit
		_ = rows.Scan(&x.ID, &x.ChannelID, &x.Body, &x.Rank)
		out = append(out, x)
	}
	if out == nil {
		out = []hit{}
	}
	writeJSON(w, 200, out)
}
