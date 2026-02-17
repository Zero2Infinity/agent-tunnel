package ws

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"agent-tunnel/internal/pty"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Handler manages WebSocket connections
type Handler struct {
	ptyManager *pty.Manager
	logger     Logger
}

// Logger interface for logging
type Logger interface {
	LogEvent(event string, data map[string]interface{})
}

// Message types for WebSocket communication
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type ResizeData struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

// NewHandler creates a new WebSocket handler
func NewHandler(ptyManager *pty.Manager, logger Logger) *Handler {
	return &Handler{
		ptyManager: ptyManager,
		logger:     logger,
	}
}

// Handle handles WebSocket connections
func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	// Check authentication via cookie
	cookie, err := r.Cookie("session")
	if err != nil || cookie.Value == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		h.logger.LogEvent("ws_auth_failed", map[string]interface{}{
			"error": "missing_session_cookie",
		})
		return
	}

	sessionID := cookie.Value

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.LogEvent("ws_upgrade_failed", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}
	defer conn.Close()

	h.logger.LogEvent("ws_connect", map[string]interface{}{
		"session_id": sessionID,
	})

	// Create PTY session
	ptySession, err := h.ptyManager.Create(sessionID)
	if err != nil {
		h.logger.LogEvent("pty_spawn_failed", map[string]interface{}{
			"session_id": sessionID,
			"error":      err.Error(),
		})
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","data":"Failed to create PTY session"}`))
		return
	}

	h.logger.LogEvent("pty_spawn", map[string]interface{}{
		"session_id": sessionID,
		"shell":      ptySession.Cmd.Path,
		"pid":        ptySession.Cmd.Process.Pid,
	})

	// Start time for duration tracking
	startTime := time.Now()

	// Use WaitGroup to coordinate goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine 1: Read from PTY and send to WebSocket
	go func() {
		defer wg.Done()
		reader := bufio.NewReader(ptySession.Pty)
		buf := make([]byte, 1024)

		for {
			n, err := reader.Read(buf)
			if err != nil {
				if err != io.EOF {
					h.logger.LogEvent("pty_read_error", map[string]interface{}{
						"session_id": sessionID,
						"error":      err.Error(),
					})
				}
				// Close WebSocket to signal the other goroutine to exit
				conn.Close()
				return
			}

			if n > 0 {
				// Send to browser
				if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					h.logger.LogEvent("ws_write_error", map[string]interface{}{
						"session_id": sessionID,
						"error":      err.Error(),
					})
					// Close WebSocket to signal the other goroutine to exit
					conn.Close()
					return
				}
			}
		}
	}()

	// Goroutine 2: Read from WebSocket and write to PTY
	go func() {
		defer wg.Done()

		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					h.logger.LogEvent("ws_read_error", map[string]interface{}{
						"session_id": sessionID,
						"error":      err.Error(),
					})
				}
				// Connection closed, exit goroutine
				return
			}

			if messageType == websocket.BinaryMessage || messageType == websocket.TextMessage {
				// Check if it's a resize message
				var msg Message
				if err := json.Unmarshal(data, &msg); err == nil {
					if msg.Type == "resize" {
						var resizeData ResizeData
						if err := json.Unmarshal(msg.Data, &resizeData); err == nil {
							h.ptyManager.Resize(sessionID, resizeData.Rows, resizeData.Cols)
							h.logger.LogEvent("pty_resize", map[string]interface{}{
								"session_id": sessionID,
								"rows":       resizeData.Rows,
								"cols":       resizeData.Cols,
							})
						}
						continue
					}
				}

				// Write data to PTY
				if _, err := ptySession.Pty.Write(data); err != nil {
					h.logger.LogEvent("pty_write_error", map[string]interface{}{
						"session_id": sessionID,
						"error":      err.Error(),
					})
					// PTY error, exit goroutine
					return
				}
			}
		}
	}()

	// Wait for both goroutines to finish
	wg.Wait()

	// Clean up
	h.ptyManager.Remove(sessionID)
	duration := time.Since(startTime).Seconds()
	h.logger.LogEvent("ws_disconnect", map[string]interface{}{
		"session_id":   sessionID,
		"duration_sec": duration,
	})
}

// ServeHTTP implements http.Handler interface
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.Handle(w, r)
}
