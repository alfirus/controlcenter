package terminal

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// ptySession wraps a local shell PTY for localhost hosts.
// For remote SSH hosts the gateway would dial x/crypto/ssh + requestPty; stubbed.
type ptySession struct {
	cmd  *exec.Cmd
	ptmx *os.File
	ws   *websocket.Conn
	mu   sync.Mutex
}

func startLocalPTY(shell string, rows, cols int) (*ptySession, error) {
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/zsh"
		}
	}
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	if rows > 0 && cols > 0 {
		_ = pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	}
	return &ptySession{cmd: cmd, ptmx: ptmx}, nil
}

func (p *ptySession) resize(rows, cols int) {
	_ = pty.Setsize(p.ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (p *ptySession) close() {
	if p.ptmx != nil {
		_ = p.ptmx.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
	}
}

// bridge copies PTY <-> WebSocket.
// WS text messages: JSON {type:"resize", rows, cols} or {type:"input", data:string}
// PTY output -> WS binary or JSON {type:"output", data: base64}
func (p *ptySession) bridge(ws *websocket.Conn, done chan struct{}) {
	p.ws = ws
	// PTY -> WS
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := p.ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
				}
				_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "pty closed"))
				close(done)
				return
			}
			p.mu.Lock()
			err = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
			p.mu.Unlock()
			if err != nil {
				close(done)
				return
			}
		}
	}()
	// WS -> PTY
	go func() {
		for {
			mt, data, err := ws.ReadMessage()
			if err != nil {
				close(done)
				return
			}
			if mt == websocket.TextMessage {
				var msg struct {
					Type string `json:"type"`
					Data string `json:"data"`
					Rows int    `json:"rows"`
					Cols int    `json:"cols"`
				}
				if err := json.Unmarshal(data, &msg); err == nil {
					switch msg.Type {
					case "resize":
						p.resize(msg.Rows, msg.Cols)
					case "input":
						_, _ = p.ptmx.Write([]byte(msg.Data))
					default:
						_, _ = p.ptmx.Write(data)
					}
					continue
				}
			}
			// binary or unknown text -> raw input
			_, _ = p.ptmx.Write(data)
		}
	}()
}
