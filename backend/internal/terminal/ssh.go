package terminal

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func sshDial(host, user, vaultSecret, authKind string, port int, jumpConn net.Conn) (*ssh.Client, error) {
	var auths []ssh.AuthMethod
	switch authKind {
	case "vault_password":
		auths = append(auths, ssh.Password(vaultSecret))
	case "agent_forward":
		if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
			if c, err := net.Dial("unix", sock); err == nil {
				ag := agent.NewClient(c)
				auths = append(auths, ssh.PublicKeysCallback(ag.Signers))
			}
		}
		fallthrough
	case "vault_key":
		if vaultSecret != "" {
			if signer, err := ssh.ParsePrivateKey([]byte(vaultSecret)); err == nil {
				auths = append(auths, ssh.PublicKeys(signer))
			} else {
				auths = append(auths, ssh.Password(vaultSecret))
			}
		}
		if len(auths) == 0 {
			if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
				if c, err := net.Dial("unix", sock); err == nil {
					ag := agent.NewClient(c)
					auths = append(auths, ssh.PublicKeysCallback(ag.Signers))
				}
			}
		}
	}
	if len(auths) == 0 {
		auths = append(auths, ssh.Password(""))
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         8 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	if jumpConn != nil {
		c, chans, reqs, err := ssh.NewClientConn(jumpConn, addr, cfg)
		if err != nil {
			return nil, err
		}
		return ssh.NewClient(c, chans, reqs), nil
	}
	return ssh.Dial("tcp", addr, cfg)
}

func sshPTY(client *ssh.Client, ws *websocket.Conn, rows, cols int) error {
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		return err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return err
	}
	if err := sess.Shell(); err != nil {
		return err
	}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if err != nil {
				_ = ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "ssh closed"))
				return
			}
			_ = ws.WriteMessage(websocket.BinaryMessage, buf[:n])
		}
	}()
	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		if mt == websocket.TextMessage {
			var msg struct {
				Type string `json:"type"`
				Data string `json:"data"`
				Rows int    `json:"rows"`
				Cols int    `json:"cols"`
			}
			if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" {
				_ = sess.WindowChange(msg.Rows, msg.Cols)
				continue
			}
			if msg.Type == "input" {
				_, _ = stdin.Write([]byte(msg.Data))
				continue
			}
		}
		_, _ = stdin.Write(data)
	}
}
