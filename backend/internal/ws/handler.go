package ws

import (
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"sniping_engine/internal/logbus"
)

type Handler struct {
	bus          *logbus.Bus
	allowOrigins []string
	upgrader     websocket.Upgrader
}

func NewHandler(bus *logbus.Bus, allowOrigins []string) *Handler {
	h := &Handler{
		bus:          bus,
		allowOrigins: allowOrigins,
	}
	h.upgrader = websocket.Upgrader{
		CheckOrigin: h.checkOrigin,
	}
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	for _, msg := range h.bus.Snapshot() {
		if err := conn.WriteJSON(msg); err != nil {
			return
		}
	}

	ch, cancel := h.bus.Subscribe(256)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		}
	}
}

func (h *Handler) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if len(h.allowOrigins) == 0 {
		return false
	}
	for _, o := range h.allowOrigins {
		if o == "*" {
			return true
		}
		if strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

