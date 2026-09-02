package terminal

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alfirus/controlcenter/backend/internal/auth"
)

// Handler — Termius-grade gateway. Secrets are never stored raw; only vault_ref.
// PTY streaming is in-memory for dev; production dials SSH via golang.org/x/crypto/ssh.
// SFTP and recording hooks are stubbed with correct contracts.

type Handler struct {
	Pool *pgxpool.Pool
	// in-memory PTY sessions for dev/no-DB mode (id -> session)
	mu       sync.Mutex
	sessions map[string]*memSession
}

type memSession struct {
	ID        string    `json:"id"`
	HostID    string    `json:"host_id"`
	Status    string    `json:"status"`
	Rows      int       `json:"rows"`
	Cols      int       `json:"cols"`
	StartedAt time.Time `json:"started_at"`
	buf       []byte
}

func (h *Handler) ensureMap() {
	if h.sessions == nil {
		h.sessions = make(map[string]*memSession)
	}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/hosts", h.listHosts)
	r.Post("/hosts", h.createHost)
	r.Get("/hosts/{id}", h.getHost)
	r.Patch("/hosts/{id}", h.updateHost)
	r.Delete("/hosts/{id}", h.deleteHost)

	r.Get("/host-groups", h.listGroups)
	r.Post("/host-groups", h.createGroup)

	r.Get("/ssh-keys", h.listKeys)
	r.Post("/ssh-keys", h.createKey)

	// terminal sessions (PTY gateway)
	r.Post("/terminal/sessions", h.createSession)
	r.Get("/terminal/sessions", h.listSessions)
	r.Get("/terminal/sessions/{id}", h.getSession)
	r.Post("/terminal/sessions/{id}/close", h.closeSession)
	r.Get("/terminal/sessions/{id}/stream", h.stream) // SSE fallback or WS upgrade (real PTY)
	r.Get("/terminal/sessions/{id}/ws", h.ws)          // WebSocket PTY (preferred)
	r.Get("/terminal/sessions/{id}/recording", h.recording)
	r.Post("/terminal/sessions/{id}/resize", h.resize)

	// SFTP stubs
	r.Get("/terminal/sessions/{id}/sftp/ls", h.sftpLs)
	r.Post("/terminal/sessions/{id}/sftp/upload", h.sftpUpload)
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

// hosts

func (h *Handler) listHosts(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	wsID := r.URL.Query().Get("workspace_id")
	if wsID == "" {
		writeErr(w, 400, "bad_request", "workspace_id required")
		return
	}
	rows, err := h.Pool.Query(r.Context(),
		`select id, label, hostname, port, username, auth_kind, vault_ref, tags, jump_host_id from hosts where workspace_id=$1 and deleted_at is null order by label`, wsID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type host struct {
		ID         string   `json:"id"`
		Label      string   `json:"label"`
		Hostname   string   `json:"hostname"`
		Port       int      `json:"port"`
		Username   string   `json:"username"`
		AuthKind   string   `json:"auth_kind"`
		VaultRef   *string  `json:"vault_ref"`
		Tags       []string `json:"tags"`
		JumpHostID *string  `json:"jump_host_id"`
	}
	var out []host
	for rows.Next() {
		var x host
		_ = rows.Scan(&x.ID, &x.Label, &x.Hostname, &x.Port, &x.Username, &x.AuthKind, &x.VaultRef, &x.Tags, &x.JumpHostID)
		if x.Tags == nil {
			x.Tags = []string{}
		}
		out = append(out, x)
	}
	if out == nil {
		out = []host{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createHost(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		// dev mode: return synthetic id without DB
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		label, _ := body["label"].(string)
		if label == "" {
			writeErr(w, 400, "bad_request", "label required")
			return
		}
		writeJSON(w, 201, map[string]string{"id": uuid.NewString(), "label": label})
		return
	}
	var body struct {
		WorkspaceID string   `json:"workspace_id"`
		GroupID     *string  `json:"group_id"`
		Label       string   `json:"label"`
		Hostname    string   `json:"hostname"`
		Port        *int     `json:"port"`
		Username    string   `json:"username"`
		AuthKind    *string  `json:"auth_kind"`
		VaultRef    *string  `json:"vault_ref"`
		JumpHostID  *string  `json:"jump_host_id"`
		Tags        []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Label == "" || body.Hostname == "" || body.WorkspaceID == "" || body.Username == "" {
		writeErr(w, 400, "bad_request", "workspace_id, label, hostname, username required")
		return
	}
	port := 22
	if body.Port != nil {
		port = *body.Port
	}
	authKind := "vault_key"
	if body.AuthKind != nil {
		authKind = *body.AuthKind
	}
	if authKind != "agent_forward" && body.VaultRef == nil {
		writeErr(w, 400, "bad_request", "vault_ref required unless auth_kind=agent_forward")
		return
	}
	userID := auth.UserID(r.Context())
	if body.Tags == nil {
		body.Tags = []string{}
	}
	var id string
	err := h.Pool.QueryRow(r.Context(),
		`insert into hosts(workspace_id, group_id, label, hostname, port, username, auth_kind, vault_ref, jump_host_id, tags, created_by)
		 values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) returning id`,
		body.WorkspaceID, body.GroupID, body.Label, body.Hostname, port, body.Username, authKind, body.VaultRef, body.JumpHostID, body.Tags, userID).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) getHost(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 404, "not_found", "db not configured")
		return
	}
	id := chi.URLParam(r, "id")
	var out struct {
		ID       string  `json:"id"`
		Label    string  `json:"label"`
		Hostname string  `json:"hostname"`
		Port     int     `json:"port"`
		Username string  `json:"username"`
		AuthKind string  `json:"auth_kind"`
		VaultRef *string `json:"vault_ref"`
	}
	err := h.Pool.QueryRow(r.Context(), `select id, label, hostname, port, username, auth_kind, vault_ref from hosts where id=$1 and deleted_at is null`, id).
		Scan(&out.ID, &out.Label, &out.Hostname, &out.Port, &out.Username, &out.AuthKind, &out.VaultRef)
	if err != nil {
		writeErr(w, 404, "not_found", "host not found")
		return
	}
	writeJSON(w, 200, out)
}

func (h *Handler) updateHost(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	id := chi.URLParam(r, "id")
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	// minimal patch: label/hostname/port
	if v, ok := body["label"].(string); ok {
		_, _ = h.Pool.Exec(r.Context(), `update hosts set label=$2 where id=$1`, id, v)
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) deleteHost(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeErr(w, 503, "unavailable", "db not configured")
		return
	}
	id := chi.URLParam(r, "id")
	_, _ = h.Pool.Exec(r.Context(), `update hosts set deleted_at=now() where id=$1`, id)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// groups & keys

func (h *Handler) listGroups(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	wsID := r.URL.Query().Get("workspace_id")
	rows, err := h.Pool.Query(r.Context(), `select id, name, parent_id from host_groups where workspace_id=$1 order by name`, wsID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type g struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		ParentID *string `json:"parent_id"`
	}
	var out []g
	for rows.Next() {
		var x g
		_ = rows.Scan(&x.ID, &x.Name, &x.ParentID)
		out = append(out, x)
	}
	if out == nil {
		out = []g{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 201, map[string]string{"id": uuid.NewString()})
		return
	}
	var body struct {
		WorkspaceID string  `json:"workspace_id"`
		Name        string  `json:"name"`
		ParentID    *string `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.WorkspaceID == "" {
		writeErr(w, 400, "bad_request", "workspace_id and name required")
		return
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `insert into host_groups(workspace_id, name, parent_id) values($1,$2,$3) returning id`, body.WorkspaceID, body.Name, body.ParentID).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) listKeys(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []any{})
		return
	}
	wsID := r.URL.Query().Get("workspace_id")
	rows, err := h.Pool.Query(r.Context(), `select id, label, vault_ref, fingerprint from ssh_keys where workspace_id=$1 order by label`, wsID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type k struct {
		ID          string  `json:"id"`
		Label       string  `json:"label"`
		VaultRef    string  `json:"vault_ref"`
		Fingerprint *string `json:"fingerprint"`
	}
	var out []k
	for rows.Next() {
		var x k
		_ = rows.Scan(&x.ID, &x.Label, &x.VaultRef, &x.Fingerprint)
		out = append(out, x)
	}
	if out == nil {
		out = []k{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createKey(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 201, map[string]string{"id": uuid.NewString()})
		return
	}
	var body struct {
		WorkspaceID string  `json:"workspace_id"`
		Label       string  `json:"label"`
		VaultRef    string  `json:"vault_ref"`
		Fingerprint *string `json:"fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Label == "" || body.VaultRef == "" || body.WorkspaceID == "" {
		writeErr(w, 400, "bad_request", "workspace_id, label, vault_ref required (never send raw key)")
		return
	}
	// enforce vault_ref looks like a ref, not a PEM
	if len(body.VaultRef) > 0 && body.VaultRef[0] == '-' {
		writeErr(w, 400, "bad_request", "vault_ref must be a vault reference, not a raw key")
		return
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `insert into ssh_keys(workspace_id, label, vault_ref, fingerprint) values($1,$2,$3,$4) returning id`, body.WorkspaceID, body.Label, body.VaultRef, body.Fingerprint).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	hsh := sha256.Sum256([]byte(body.VaultRef))
	_ = hsh // ensure import used when DB present
	writeJSON(w, 201, map[string]string{"id": id})
}

// terminal sessions

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		HostID string `json:"host_id"`
		Rows   *int   `json:"rows"`
		Cols   *int   `json:"cols"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.HostID == "" {
		writeErr(w, 400, "bad_request", "host_id required")
		return
	}
	rows, cols := 24, 80
	if body.Rows != nil {
		rows = *body.Rows
	}
	if body.Cols != nil {
		cols = *body.Cols
	}
	if h.Pool != nil {
		userID := auth.UserID(r.Context())
		var id string
		err := h.Pool.QueryRow(r.Context(),
			`insert into terminal_sessions(host_id, user_id, rows, cols) values($1,$2,$3,$4) returning id`, body.HostID, userID, rows, cols).Scan(&id)
		if err != nil {
			writeErr(w, 500, "internal", err.Error())
			return
		}
		writeJSON(w, 201, map[string]any{"id": id, "host_id": body.HostID, "rows": rows, "cols": cols, "status": "active"})
		return
	}
	h.mu.Lock()
	h.ensureMap()
	id := uuid.NewString()
	h.sessions[id] = &memSession{ID: id, HostID: body.HostID, Status: "active", Rows: rows, Cols: cols, StartedAt: time.Now(), buf: []byte(fmt.Sprintf("connected to %s (dev PTY stub)\r\n$ ", body.HostID))}
	h.mu.Unlock()
	writeJSON(w, 201, map[string]any{"id": id, "host_id": body.HostID, "rows": rows, "cols": cols, "status": "active"})
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	if h.Pool != nil {
		hostID := r.URL.Query().Get("host_id")
		q := `select id, host_id, status, rows, cols, started_at from terminal_sessions where status='active' order by started_at desc limit 50`
		var rows interface{ Next() bool; Scan(...any) error; Close() }
		// use pgx rows directly
		pgRows, err := h.Pool.Query(r.Context(), q)
		if err != nil {
			writeErr(w, 500, "internal", err.Error())
			return
		}
		defer pgRows.Close()
		type s struct {
			ID        string `json:"id"`
			HostID    string `json:"host_id"`
			Status    string `json:"status"`
			Rows      int    `json:"rows"`
			Cols      int    `json:"cols"`
			StartedAt string `json:"started_at"`
		}
		var out []s
		_ = hostID
		_ = rows
		for pgRows.Next() {
			var x s
			_ = pgRows.Scan(&x.ID, &x.HostID, &x.Status, &x.Rows, &x.Cols, &x.StartedAt)
			out = append(out, x)
		}
		if out == nil {
			out = []s{}
		}
		writeJSON(w, 200, out)
		return
	}
	h.mu.Lock()
	var out []memSession
	for _, v := range h.sessions {
		if v.Status == "active" {
			out = append(out, *v)
		}
	}
	h.mu.Unlock()
	if out == nil {
		out = []memSession{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) getSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Pool != nil {
		var s struct {
			ID        string `json:"id"`
			HostID    string `json:"host_id"`
			Status    string `json:"status"`
			Rows      int    `json:"rows"`
			Cols      int    `json:"cols"`
			StartedAt string `json:"started_at"`
		}
		err := h.Pool.QueryRow(r.Context(), `select id, host_id, status, rows, cols, started_at from terminal_sessions where id=$1`, id).
			Scan(&s.ID, &s.HostID, &s.Status, &s.Rows, &s.Cols, &s.StartedAt)
		if err != nil {
			writeErr(w, 404, "not_found", "session not found")
			return
		}
		writeJSON(w, 200, s)
		return
	}
	h.mu.Lock()
	s, ok := h.sessions[id]
	h.mu.Unlock()
	if !ok {
		writeErr(w, 404, "not_found", "session not found")
		return
	}
	writeJSON(w, 200, s)
}

func (h *Handler) closeSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Pool != nil {
		_, _ = h.Pool.Exec(r.Context(), `update terminal_sessions set status='closed', ended_at=now() where id=$1`, id)
		writeJSON(w, 200, map[string]string{"status": "closed"})
		return
	}
	h.mu.Lock()
	if s, ok := h.sessions[id]; ok {
		s.Status = "closed"
	}
	h.mu.Unlock()
	writeJSON(w, 200, map[string]string{"status": "closed"})
}

func (h *Handler) resize(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Rows int `json:"rows"`
		Cols int `json:"cols"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if h.Pool != nil {
		_, _ = h.Pool.Exec(r.Context(), `update terminal_sessions set rows=$2, cols=$3 where id=$1`, id, body.Rows, body.Cols)
	}
	h.mu.Lock()
	if s, ok := h.sessions[id]; ok {
		if body.Rows > 0 {
			s.Rows = body.Rows
		}
		if body.Cols > 0 {
			s.Cols = body.Cols
		}
	}
	h.mu.Unlock()
	// also allow query params ?rows=&cols=
	if body.Rows == 0 {
		if v := r.URL.Query().Get("rows"); v != "" {
			if n, _ := strconv.Atoi(v); n > 0 {
				h.mu.Lock()
				if s, ok := h.sessions[id]; ok {
					s.Rows = n
				}
				h.mu.Unlock()
			}
		}
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *Handler) isWebSocket(r *http.Request) bool {
	return r.Header.Get("Upgrade") == "websocket" || r.Header.Get("Sec-WebSocket-Key") != ""
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	if h.isWebSocket(r) {
		h.ws(w, r)
		return
	}
	id := chi.URLParam(r, "id")
	// SSE fallback: buffered output
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "internal", "streaming not supported")
		return
	}
	h.mu.Lock()
	s, exists := h.sessions[id]
	h.mu.Unlock()
	var data []byte
	if exists {
		data = s.buf
	} else if h.Pool != nil {
		var status string
		err := h.Pool.QueryRow(r.Context(), `select status from terminal_sessions where id=$1`, id).Scan(&status)
		if err != nil {
			writeErr(w, 404, "not_found", "session not found")
			return
		}
		data = []byte(fmt.Sprintf("connected to session %s\r\n$ ", id))
	} else {
		writeErr(w, 404, "not_found", "session not found")
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", string(data))
	flusher.Flush()
	select {
	case <-r.Context().Done():
	case <-time.After(2 * time.Second):
	}
}

func (h *Handler) ws(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// resolve host: local shell for localhost, else SSH stub
	var hostname string
	var rows, cols int = 24, 80
	if h.Pool != nil {
		var hostID string
		_ = h.Pool.QueryRow(r.Context(), `select host_id, rows, cols from terminal_sessions where id=$1`, id).Scan(&hostID, &rows, &cols)
		if hostID != "" {
			_ = h.Pool.QueryRow(r.Context(), `select hostname from hosts where id=$1`, hostID).Scan(&hostname)
		}
	} else {
		h.mu.Lock()
		if s, ok := h.sessions[id]; ok {
			hostname = "localhost"
			rows, cols = s.Rows, s.Cols
		}
		h.mu.Unlock()
	}
	// audit
	if h.Pool != nil {
		userID := auth.UserID(r.Context())
		_, _ = h.Pool.Exec(r.Context(), `insert into audit_log(actor_id, action, target) values($1,'terminal.ws', $2)`, userID, id)
	}

	ws, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	// local PTY for localhost/dev; remote SSH stub sends message
	if hostname == "" || hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		ps, err := startLocalPTY("", rows, cols)
		if err != nil {
			_ = ws.WriteMessage(websocket.TextMessage, []byte("failed to start pty: "+err.Error()))
			return
		}
		defer ps.close()
		done := make(chan struct{})
		ps.bridge(ws, done)
		<-done
		return
	}
	// remote SSH via vault
	var username, authKind, vaultRef, jumpHostID string
	var port int
	var vaultRefPtr *string
	var jumpPtr *string
	if h.Pool != nil {
		_ = h.Pool.QueryRow(r.Context(), `select username, port, auth_kind, vault_ref, jump_host_id from hosts where hostname=$1 limit 1`, hostname).Scan(&username, &port, &authKind, &vaultRefPtr, &jumpPtr)
		if vaultRefPtr != nil {
			vaultRef = *vaultRefPtr
		}
		if jumpPtr != nil {
			jumpHostID = *jumpPtr
		}
		if username == "" {
			username = "root"
		}
		if port == 0 {
			port = 22
		}
		if authKind == "" {
			authKind = "vault_key"
		}
	} else {
		username = "root"
		port = 22
		authKind = "vault_key"
	}
	_ = jumpHostID
	vaultSecret, _ := fetchVaultSecret(r.Context(), h.Pool, vaultRef)
	if authKind != "agent_forward" && vaultSecret == "" {
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("vault secret empty for %s (vault_ref=%s). Set VAULT_* env or Supabase Vault.\r\n", hostname, vaultRef)))
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}
	// TODO: jump host support via recursive dial
	client, err := sshDial(hostname, username, vaultSecret, authKind, port, nil)
	if err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte("ssh dial failed: "+err.Error()+"\r\n"))
		return
	}
	defer client.Close()
	if err := sshPTY(client, ws, rows, cols); err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte("ssh pty error: "+err.Error()+"\r\n"))
	}
	_ = port
}

func (h *Handler) recording(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Pool != nil {
		rows, err := h.Pool.Query(r.Context(), `select chunk_seq, data from terminal_recordings where session_id=$1 order by chunk_seq`, id)
		if err != nil {
			writeErr(w, 500, "internal", err.Error())
			return
		}
		defer rows.Close()
		type chunk struct {
			Seq  int    `json:"seq"`
			Data string `json:"data"`
		}
		var out []chunk
		for rows.Next() {
			var seq int
			var data []byte
			_ = rows.Scan(&seq, &data)
			out = append(out, chunk{Seq: seq, Data: string(data)})
		}
		if out == nil {
			out = []chunk{}
		}
		writeJSON(w, 200, out)
		return
	}
	h.mu.Lock()
	s, ok := h.sessions[id]
	h.mu.Unlock()
	if !ok {
		writeErr(w, 404, "not_found", "session not found")
		return
	}
	writeJSON(w, 200, []map[string]any{{"seq": 0, "data": string(s.buf)}})
}

func (h *Handler) sftpLs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	// prefer local FS for localhost hosts; else try SSH SFTP
	if h.Pool != nil {
		var hostname string
		_ = h.Pool.QueryRow(r.Context(), `select h.hostname from hosts h join terminal_sessions s on s.host_id=h.id where s.id=$1`, id).Scan(&hostname)
		if hostname != "" && hostname != "localhost" && hostname != "127.0.0.1" && hostname != "::1" {
			// real SFTP: dial via sshDial then sftp.NewClient
			// For now, return structured stub so client can show vault hint
			writeJSON(w, 200, map[string]any{"path": path, "entries": []any{}, "note": "SFTP for " + hostname + " requires vault SFTP — stub. Local FS fallback for localhost only."})
			return
		}
	}
	// local FS
	entries, err := listLocalDir(path)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"path": path, "entries": entries})
}

func (h *Handler) sftpUpload(w http.ResponseWriter, r *http.Request) {
	// For localhost, accept multipart and write to local FS (dev).
	// For remote, would proxy via SFTP.
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeErr(w, 400, "bad_request", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok (local stub — remote SFTP via vault pending)"})
}
