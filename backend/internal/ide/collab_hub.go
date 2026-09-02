package ide

import (
	"sync"

	"github.com/gorilla/websocket"
)

type collabHub struct {
	mu      sync.Mutex
	clients map[string]map[*websocket.Conn]bool // collab_id -> set
}

var hub = &collabHub{clients: make(map[string]map[*websocket.Conn]bool)}

func (h *collabHub) add(id string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[id] == nil {
		h.clients[id] = make(map[*websocket.Conn]bool)
	}
	h.clients[id][conn] = true
}

func (h *collabHub) remove(id string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m, ok := h.clients[id]; ok {
		delete(m, conn)
		if len(m) == 0 {
			delete(h.clients, id)
		}
	}
}

func (h *collabHub) broadcast(id string, msgType int, data []byte, exclude *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients[id] {
		if c == exclude {
			continue
		}
		_ = c.WriteMessage(msgType, data)
	}
}
