package calendar

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/go-chi/chi/v5"
)

type Handler struct{ Pool *pgxpool.Pool }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/calendars", h.listCalendars)
	r.Post("/calendars", h.createCalendar)
	r.Get("/events", h.listEvents)
	r.Post("/events", h.createEvent)
	r.Patch("/events/{id}", h.updateEvent)
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

func (h *Handler) listCalendars(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	wsID := r.URL.Query().Get("workspace_id")
	if wsID == "" {
		writeErr(w, 400, "bad_request", "workspace_id required")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `select id, name, kind, google_calendar_id from calendars where workspace_id=$1 order by created_at`, wsID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type cal struct {
		ID               string  `json:"id"`
		Name             string  `json:"name"`
		Kind             string  `json:"kind"`
		GoogleCalendarID *string `json:"google_calendar_id"`
	}
	var out []cal
	for rows.Next() {
		var x cal
		_ = rows.Scan(&x.ID, &x.Name, &x.Kind, &x.GoogleCalendarID)
		out = append(out, x)
	}
	if out == nil {
		out = []cal{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createCalendar(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		Name        string `json:"name"`
		Kind        string `json:"kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.WorkspaceID == "" {
		writeErr(w, 400, "bad_request", "workspace_id and name required")
		return
	}
	if body.Kind == "" {
		body.Kind = "team"
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `insert into calendars(workspace_id, name, kind) values($1,$2,$3) returning id`, body.WorkspaceID, body.Name, body.Kind).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	calID := r.URL.Query().Get("calendar_id")
	if calID == "" {
		writeErr(w, 400, "bad_request", "calendar_id required")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `select id, title, start_at, end_at from events where calendar_id=$1 and deleted_at is null order by start_at`, calID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type ev struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		StartAt string `json:"start_at"`
		EndAt   string `json:"end_at"`
	}
	var out []ev
	for rows.Next() {
		var x ev
		_ = rows.Scan(&x.ID, &x.Title, &x.StartAt, &x.EndAt)
		out = append(out, x)
	}
	if out == nil {
		out = []ev{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	var body struct {
		CalendarID string `json:"calendar_id"`
		Title      string `json:"title"`
		StartAt    string `json:"start_at"`
		EndAt      string `json:"end_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Title == "" || body.CalendarID == "" {
		writeErr(w, 400, "bad_request", "calendar_id, title, start_at, end_at required")
		return
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `insert into events(calendar_id, title, start_at, end_at) values($1,$2,$3,$4) returning id`, body.CalendarID, body.Title, body.StartAt, body.EndAt).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) updateEvent(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Title   *string `json:"title"`
		StartAt *string `json:"start_at"`
		EndAt   *string `json:"end_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	if body.Title != nil {
		_, _ = h.Pool.Exec(r.Context(), `update events set title=$2, updated_at=now() where id=$1`, id, *body.Title)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
