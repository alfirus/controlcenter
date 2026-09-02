package ide

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

var lspUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// LSPProxyWS upgrades to WS and proxies JSON-RPC to the language's lsp_servers.command.
// Protocol: client WS text frames are JSON-RPC messages; server spawns command and bridges stdio <-> WS.
// If command not found, sends error and closes.
// Query ?buffer_id= optional for logging.
func (h *Handler) LSPProxyWS(w http.ResponseWriter, r *http.Request) {
	lang := chi.URLParam(r, "language")
	var command string
	var config any
	if h.Pool != nil {
		err := h.Pool.QueryRow(r.Context(), `select command, config from lsp_servers where language=$1`, lang).Scan(&command, &config)
		if err != nil {
			http.Error(w, `{"error":{"code":"not_found","message":"lsp not configured for language"}}`, http.StatusNotFound)
			return
		}
	} else {
		// dev fallback
		switch lang {
		case "go":
			command = "gopls"
		case "rust":
			command = "rust-analyzer"
		case "typescript":
			command = "typescript-language-server --stdio"
		case "python":
			command = "pylsp"
		default:
			http.Error(w, `{"error":{"code":"not_found","message":"unknown language"}}`, http.StatusNotFound)
			return
		}
	}
	parts := strings.Fields(command)
	if len(parts) == 0 {
		http.Error(w, `{"error":{"code":"internal","message":"empty lsp command"}}`, http.StatusInternalServerError)
		return
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		http.Error(w, `{"error":{"code":"unavailable","message":"lsp binary not found: `+err.Error()+`"}}`, http.StatusServiceUnavailable)
		return
	}
	ws, err := lspUpgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = cmd.Process.Kill()
		return
	}
	defer ws.Close()
	defer func() { _ = cmd.Process.Kill() }()

	// LSP uses Content-Length header framing; but many clients (Zed-style) send raw JSON-RPC over WS text frames.
	// We bridge raw frames: WS text -> stdin (with header), stdout -> WS text.
	// For gopls etc., we add Content-Length framing on stdin, and parse framing on stdout.

	// stdout -> WS (framed)
	go func() {
		reader := bufio.NewReader(stdout)
		for {
			// parse Content-Length
			headers := ""
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				headers += line
				if line == "\r\n" {
					break
				}
			}
			var length int
			for _, l := range strings.Split(headers, "\r\n") {
				if strings.HasPrefix(strings.ToLower(l), "content-length:") {
					_, _ = json.Marshal(l) // keep import
					// parse int
					val := strings.TrimSpace(strings.SplitN(l, ":", 2)[1])
					// simple atoi
					for _, c := range val {
						if c < '0' || c > '9' {
							break
						}
						length = length*10 + int(c-'0')
					}
				}
			}
			if length == 0 {
				continue
			}
			body := make([]byte, length)
			_, _ = io.ReadFull(reader, body)
			_ = ws.WriteMessage(websocket.TextMessage, body)
		}
	}()

	// WS -> stdin (add framing)
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return
		}
		// if already has Content-Length, pass through; else wrap
		if strings.Contains(string(data), "Content-Length:") {
			_, _ = stdin.Write(data)
		} else {
			header := struct {
				ContentLength int `json:"-"`
			}{}
			_ = header
			frame := "Content-Length: " + itoa(len(data)) + "\r\n\r\n" + string(data)
			_, _ = io.WriteString(stdin, frame)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// ensure pgxpool import used when Pool is nil path
var _ = sync.Mutex{}
var _ pgxpool.Pool
