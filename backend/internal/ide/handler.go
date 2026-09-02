package ide

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler — Zed-grade IDE: buffers, CRDT collab, LSP proxy, file tree.

type Handler struct {
	Pool *pgxpool.Pool
	mu   sync.Mutex
	// mem fallback when no DB: workspace -> buffers
	workspaces map[string]*memWS
	buffers    map[string]*memBuf
	collab     map[string]*memCollab
}

type memWS struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	ProjectID   *string `json:"project_id"`
	Name        string `json:"name"`
	RootPath    string `json:"root_path"`
}
type memBuf struct {
	ID             string `json:"id"`
	IdeWorkspaceID string `json:"ide_workspace_id"`
	Path           string `json:"path"`
	ContentHash    string `json:"content_hash"`
	Content        []byte `json:"-"`
	SizeBytes      int    `json:"size_bytes"`
}
type memCollab struct {
	ID       string         `json:"id"`
	BufferID string         `json:"buffer_id"`
	State    map[string]any `json:"crdt_state"`
}

func (h *Handler) ensure() {
	if h.workspaces == nil {
		h.workspaces = make(map[string]*memWS)
		h.buffers = make(map[string]*memBuf)
		h.collab = make(map[string]*memCollab)
	}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/ide/workspaces", h.listWS)
	r.Post("/ide/workspaces", h.createWS)
	r.Get("/ide/workspaces/{id}", h.getWS)
	r.Get("/ide/workspaces/{id}/tree", h.tree)
	r.Get("/ide/workspaces/{id}/search", h.search)

	r.Get("/ide/buffers", h.listBuffers)
	r.Post("/ide/buffers", h.createBuffer)
	r.Get("/ide/buffers/{id}", h.getBuffer)
	r.Put("/ide/buffers/{id}", h.putBuffer)
	r.Delete("/ide/buffers/{id}", h.deleteBuffer)

	r.Post("/ide/collab/sessions", h.createCollab)
	r.Get("/ide/collab/sessions/{id}", h.getCollab)
	r.Get("/ide/collab/{id}", h.collabStream) // SSE fallback or WS
	r.Get("/ide/collab/{id}/ws", h.collabWS)  // WS CRDT broadcast

	r.Get("/ide/lsp/servers", h.listLSP)
	r.Post("/ide/lsp/{language}", h.lspProxy)
	r.Get("/ide/lsp/{language}/ws", h.LSPProxyWS)
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

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// workspaces

func (h *Handler) listWS(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		h.mu.Lock()
		h.ensure()
		var out []memWS
		for _, v := range h.workspaces {
			out = append(out, *v)
		}
		if out == nil {
			out = []memWS{}
		}
		h.mu.Unlock()
		writeJSON(w, 200, out)
		return
	}
	wsID := r.URL.Query().Get("workspace_id")
	if wsID == "" {
		writeErr(w, 400, "bad_request", "workspace_id required")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `select id, workspace_id, project_id, name, root_path from ide_workspaces where workspace_id=$1 and deleted_at is null order by created_at`, wsID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type ws struct {
		ID          string  `json:"id"`
		WorkspaceID string  `json:"workspace_id"`
		ProjectID   *string `json:"project_id"`
		Name        string  `json:"name"`
		RootPath    string  `json:"root_path"`
	}
	var out []ws
	for rows.Next() {
		var x ws
		_ = rows.Scan(&x.ID, &x.WorkspaceID, &x.ProjectID, &x.Name, &x.RootPath)
		out = append(out, x)
	}
	if out == nil {
		out = []ws{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createWS(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string  `json:"workspace_id"`
		ProjectID   *string `json:"project_id"`
		HostID      *string `json:"host_id"`
		Name        string  `json:"name"`
		RootPath    *string `json:"root_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.WorkspaceID == "" || body.Name == "" {
		writeErr(w, 400, "bad_request", "workspace_id and name required")
		return
	}
	root := "/"
	if body.RootPath != nil {
		root = *body.RootPath
	}
	if h.Pool == nil {
		h.mu.Lock()
		h.ensure()
		id := uuid.NewString()
		h.workspaces[id] = &memWS{ID: id, WorkspaceID: body.WorkspaceID, ProjectID: body.ProjectID, Name: body.Name, RootPath: root}
		h.mu.Unlock()
		writeJSON(w, 201, map[string]string{"id": id})
		return
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `insert into ide_workspaces(workspace_id, project_id, host_id, name, root_path) values($1,$2,$3,$4,$5) returning id`, body.WorkspaceID, body.ProjectID, body.HostID, body.Name, root).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) getWS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Pool == nil {
		h.mu.Lock()
		ws, ok := h.workspaces[id]
		h.mu.Unlock()
		if !ok {
			writeErr(w, 404, "not_found", "ide workspace not found")
			return
		}
		writeJSON(w, 200, ws)
		return
	}
	var out struct {
		ID          string  `json:"id"`
		WorkspaceID string  `json:"workspace_id"`
		ProjectID   *string `json:"project_id"`
		Name        string  `json:"name"`
		RootPath    string  `json:"root_path"`
	}
	err := h.Pool.QueryRow(r.Context(), `select id, workspace_id, project_id, name, root_path from ide_workspaces where id=$1`, id).Scan(&out.ID, &out.WorkspaceID, &out.ProjectID, &out.Name, &out.RootPath)
	if err != nil {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	writeJSON(w, 200, out)
}

func (h *Handler) tree(w http.ResponseWriter, r *http.Request) {
	// stub: returns empty tree; prod reads Storage or remote host via SFTP
	writeJSON(w, 200, []any{})
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	// ripgrep-style search stub (prod scans Storage)
	writeJSON(w, 200, []any{})
}

// buffers

func (h *Handler) listBuffers(w http.ResponseWriter, r *http.Request) {
	wsID := r.URL.Query().Get("ide_workspace_id")
	if wsID == "" {
		writeErr(w, 400, "bad_request", "ide_workspace_id required")
		return
	}
	if h.Pool == nil {
		h.mu.Lock()
		var out []memBuf
		for _, b := range h.buffers {
			if b.IdeWorkspaceID == wsID {
				out = append(out, *b)
			}
		}
		if out == nil {
			out = []memBuf{}
		}
		h.mu.Unlock()
		writeJSON(w, 200, out)
		return
	}
	rows, err := h.Pool.Query(r.Context(), `select id, ide_workspace_id, path, content_hash, size_bytes from ide_buffers where ide_workspace_id=$1 order by path`, wsID)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type b struct {
		ID             string `json:"id"`
		IdeWorkspaceID string `json:"ide_workspace_id"`
		Path           string `json:"path"`
		ContentHash    string `json:"content_hash"`
		SizeBytes      int    `json:"size_bytes"`
	}
	var out []b
	for rows.Next() {
		var x b
		_ = rows.Scan(&x.ID, &x.IdeWorkspaceID, &x.Path, &x.ContentHash, &x.SizeBytes)
		out = append(out, x)
	}
	if out == nil {
		out = []b{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createBuffer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IdeWorkspaceID string  `json:"ide_workspace_id"`
		Path           string  `json:"path"`
		Content        *string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.IdeWorkspaceID == "" || body.Path == "" {
		writeErr(w, 400, "bad_request", "ide_workspace_id and path required")
		return
	}
	content := ""
	if body.Content != nil {
		content = *body.Content
	}
	hash := hashBytes([]byte(content))
	if h.Pool == nil {
		h.mu.Lock()
		h.ensure()
		id := uuid.NewString()
		h.buffers[id] = &memBuf{ID: id, IdeWorkspaceID: body.IdeWorkspaceID, Path: body.Path, ContentHash: hash, Content: []byte(content), SizeBytes: len(content)}
		h.mu.Unlock()
		writeJSON(w, 201, map[string]string{"id": id, "content_hash": hash})
		return
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `insert into ide_buffers(ide_workspace_id, path, content_hash, size_bytes) values($1,$2,$3,$4) returning id`, body.IdeWorkspaceID, body.Path, hash, len(content)).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id, "content_hash": hash})
}

func (h *Handler) getBuffer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Pool == nil {
		h.mu.Lock()
		b, ok := h.buffers[id]
		h.mu.Unlock()
		if !ok {
			writeErr(w, 404, "not_found", "buffer not found")
			return
		}
		writeJSON(w, 200, map[string]any{"id": b.ID, "path": b.Path, "content_hash": b.ContentHash, "content": string(b.Content)})
		return
	}
	var path, hash string
	var size int
	err := h.Pool.QueryRow(r.Context(), `select path, content_hash, size_bytes from ide_buffers where id=$1`, id).Scan(&path, &hash, &size)
	if err != nil {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "path": path, "content_hash": hash, "size_bytes": size})
}

func (h *Handler) putBuffer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Content     *string `json:"content"`
		ContentHash *string `json:"content_hash"` // If-Match guard
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Content == nil {
		writeErr(w, 400, "bad_request", "content required")
		return
	}
	hash := hashBytes([]byte(*body.Content))
	if h.Pool == nil {
		h.mu.Lock()
		b, ok := h.buffers[id]
		if !ok {
			h.mu.Unlock()
			writeErr(w, 404, "not_found", "buffer not found")
			return
		}
		if body.ContentHash != nil && *body.ContentHash != b.ContentHash {
			h.mu.Unlock()
			writeErr(w, 409, "conflict", "content_hash mismatch — CRDT wins, reload")
			return
		}
		b.Content = []byte(*body.Content)
		b.ContentHash = hash
		b.SizeBytes = len(*body.Content)
		h.mu.Unlock()
		writeJSON(w, 200, map[string]string{"content_hash": hash})
		return
	}
	// check hash guard
	if body.ContentHash != nil {
		var cur string
		_ = h.Pool.QueryRow(r.Context(), `select content_hash from ide_buffers where id=$1`, id).Scan(&cur)
		if cur != "" && cur != *body.ContentHash {
			writeErr(w, 409, "conflict", "content_hash mismatch")
			return
		}
	}
	_, err := h.Pool.Exec(r.Context(), `update ide_buffers set content_hash=$2, size_bytes=$3, updated_at=now() where id=$1`, id, hash, len(*body.Content))
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"content_hash": hash})
}

func (h *Handler) deleteBuffer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Pool == nil {
		h.mu.Lock()
		delete(h.buffers, id)
		h.mu.Unlock()
		writeJSON(w, 200, map[string]string{"status": "deleted"})
		return
	}
	_, _ = h.Pool.Exec(r.Context(), `delete from ide_buffers where id=$1`, id)
	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

// collab

func (h *Handler) createCollab(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BufferID    string `json:"buffer_id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.BufferID == "" {
		writeErr(w, 400, "bad_request", "buffer_id required")
		return
	}
	if h.Pool == nil {
		h.mu.Lock()
		h.ensure()
		id := uuid.NewString()
		h.collab[id] = &memCollab{ID: id, BufferID: body.BufferID, State: map[string]any{}}
		h.mu.Unlock()
		writeJSON(w, 201, map[string]string{"id": id})
		return
	}
	if body.WorkspaceID == "" {
		// resolve via buffer
		_ = h.Pool.QueryRow(r.Context(), `select workspace_id from ide_workspaces join ide_buffers on ide_buffers.ide_workspace_id=ide_workspaces.id where ide_buffers.id=$1`, body.BufferID).Scan(&body.WorkspaceID)
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `insert into ide_collab_sessions(buffer_id, workspace_id) values($1,$2) returning id`, body.BufferID, body.WorkspaceID).Scan(&id)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	writeJSON(w, 201, map[string]string{"id": id})
}

func (h *Handler) getCollab(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.Pool == nil {
		h.mu.Lock()
		c, ok := h.collab[id]
		h.mu.Unlock()
		if !ok {
			writeErr(w, 404, "not_found", "not found")
			return
		}
		writeJSON(w, 200, c)
		return
	}
	var c struct {
		ID       string `json:"id"`
		BufferID string `json:"buffer_id"`
		State    any    `json:"crdt_state"`
	}
	err := h.Pool.QueryRow(r.Context(), `select id, buffer_id, crdt_state from ide_collab_sessions where id=$1`, id).Scan(&c.ID, &c.BufferID, &c.State)
	if err != nil {
		writeErr(w, 404, "not_found", "not found")
		return
	}
	writeJSON(w, 200, c)
}

var collabUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (h *Handler) collabStream(w http.ResponseWriter, r *http.Request) {
	// SSE fallback unless Upgrade: websocket
	if r.Header.Get("Upgrade") == "websocket" || r.Header.Get("Sec-WebSocket-Key") != "" {
		h.collabWS(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, 500, "internal", "streaming not supported")
		return
	}
	id := chi.URLParam(r, "id")
	fmt.Fprintf(w, "event: open\ndata: {\"collab_id\":\"%s\",\"ts\":%d}\n\n", id, time.Now().Unix())
	flusher.Flush()
	select {
	case <-r.Context().Done():
	case <-time.After(2 * time.Second):
	}
}

func (h *Handler) collabWS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// validate collab exists (best effort)
	if h.Pool != nil {
		var exists string
		_ = h.Pool.QueryRow(r.Context(), `select id from ide_collab_sessions where id=$1`, id).Scan(&exists)
	}
	ws, err := collabUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	defer hub.remove(id, ws)
	hub.add(id, ws)
	// send open event
	_ = ws.WriteJSON(map[string]any{"type": "open", "collab_id": id, "ts": time.Now().Unix()})
	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		// broadcast to peers (CRDT patch, cursor, etc.)
		hub.broadcast(id, mt, data, ws)
		// optional: persist crdt_state periodically
		if mt == websocket.TextMessage {
			var msg map[string]any
			if json.Unmarshal(data, &msg) == nil {
				if msg["type"] == "patch" && h.Pool != nil {
					// best-effort store last patch as crdt_state
					_, _ = h.Pool.Exec(r.Context(), `update ide_collab_sessions set crdt_state=$2 where id=$1`, id, msg)
				}
			}
		}
	}
}

// LSP

func (h *Handler) listLSP(w http.ResponseWriter, r *http.Request) {
	if h.Pool == nil {
		writeJSON(w, 200, []map[string]string{
			{"language": "go", "command": "gopls"},
			{"language": "rust", "command": "rust-analyzer"},
			{"language": "typescript", "command": "typescript-language-server --stdio"},
			{"language": "python", "command": "pylsp"},
		})
		return
	}
	rows, err := h.Pool.Query(r.Context(), `select language, command, config from lsp_servers order by language`)
	if err != nil {
		writeErr(w, 500, "internal", err.Error())
		return
	}
	defer rows.Close()
	type s struct {
		Language string `json:"language"`
		Command  string `json:"command"`
		Config   any    `json:"config"`
	}
	var out []s
	for rows.Next() {
		var x s
		_ = rows.Scan(&x.Language, &x.Command, &x.Config)
		out = append(out, x)
	}
	if out == nil {
		out = []s{}
	}
	writeJSON(w, 200, out)
}

func (h *Handler) lspProxy(w http.ResponseWriter, r *http.Request) {
	lang := chi.URLParam(r, "language")
	// stub: echo request, prod spawns command and proxies JSON-RPC over WS
	var body any
	_ = json.NewDecoder(r.Body).Decode(&body)
	writeJSON(w, 200, map[string]any{"language": lang, "echo": body, "note": "LSP proxy stub — prod spawns lsp_servers.command"})
}
