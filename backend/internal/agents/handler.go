package agents

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct{ Pool *pgxpool.Pool }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/agents", h.list)
	r.Post("/agents", h.register)
	r.Post("/agents/{id}/invite", h.invite)
	r.Post("/agents/{id}/message", h.proxyMessage)
	r.Post("/agents/{id}/events", h.inbound)
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
	rows, err := h.Pool.Query(r.Context(), `select id, hermes_endpoint, persona, display_name from agents order by created_at`)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type a struct {
		ID             string `json:"id"`
		HermesEndpoint string `json:"hermes_endpoint"`
		Persona        string `json:"persona"`
		DisplayName    string `json:"display_name"`
	}
	var out []a
	for rows.Next() {
		var x a
		_ = rows.Scan(&x.ID, &x.HermesEndpoint, &x.Persona, &x.DisplayName)
		out = append(out, x)
	}
	if out == nil {
		out = []a{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	var body struct {
		HermesEndpoint string `json:"hermes_endpoint"`
		Persona        string `json:"persona"`
		DisplayName    string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.HermesEndpoint == "" || body.Persona == "" {
		writeErr(w, 400, "bad_request", "hermes_endpoint and persona required")
		return
	}
	if body.DisplayName == "" {
		body.DisplayName = body.Persona
	}
	var id string
	err := h.Pool.QueryRow(r.Context(),
		`insert into agents(hermes_endpoint, persona, display_name) values($1,$2,$3) on conflict(hermes_endpoint, persona) do update set display_name=$3 returning id`,
		body.HermesEndpoint, body.Persona, body.DisplayName).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	agentID := chi.URLParam(r, "id")
	var body struct {
		WorkspaceID string  `json:"workspace_id"`
		ChannelID   *string `json:"channel_id"`
		ProjectID   *string `json:"project_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.WorkspaceID == "" {
		writeErr(w, 400, "bad_request", "workspace_id required")
		return
	}
	_, err := h.Pool.Exec(r.Context(),
		`insert into agent_memberships(agent_id, workspace_id, channel_id, project_id) values($1,$2,$3,$4) on conflict do nothing`,
		agentID, body.WorkspaceID, body.ChannelID, body.ProjectID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "invited"})
}

func (h *Handler) proxyMessage(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	agentID := chi.URLParam(r, "id")
	var endpoint string
	err := h.Pool.QueryRow(r.Context(), `select hermes_endpoint from agents where id=$1`, agentID).Scan(&endpoint)
	if err != nil {
		writeErr(w, 404, "not_found", "agent not found")
		return
	}
	body, _ := io.ReadAll(r.Body)
	req, err := http.NewRequestWithContext(r.Context(), "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeErr(w, 502, "bad_gateway", err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *Handler) inbound(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	agentID := chi.URLParam(r, "id")
	var body struct {
		Type      string  `json:"type"`
		ChannelID *string `json:"channel_id"`
		Body      string  `json:"body"`
		RootID    *string `json:"root_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	switch body.Type {
	case "message", "":
		if body.ChannelID == nil || body.Body == "" {
			writeErr(w, 400, "bad_request", "channel_id and body required for message")
			return
		}
		var id string
		err := h.Pool.QueryRow(r.Context(),
			`insert into messages(channel_id, agent_id, body, root_id) values($1,$2,$3,$4) returning id`,
			*body.ChannelID, agentID, body.Body, body.RootID).Scan(&id)
		if err != nil {
			writeErr(w, 500, "internal", err.Error())
			return
		}
		writeJSON(w, 201, map[string]string{"id": id})
	default:
		writeErr(w, 400, "bad_request", "unknown type")
	}
}
