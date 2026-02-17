package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agent-tunnel/internal/auth"
	"agent-tunnel/internal/pty"
	"agent-tunnel/internal/ws"
)

// Logger provides JSON logging
type Logger struct {
	service string
}

// LogEvent logs an event in JSON format
func (l *Logger) LogEvent(event string, data map[string]interface{}) {
	logEntry := map[string]interface{}{
		"level":     "info",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   l.service,
		"event":     event,
	}

	for k, v := range data {
		logEntry[k] = v
	}

	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("Failed to marshal log entry: %v", err)
		return
	}
	log.Println(string(jsonBytes))
}

// LogError logs an error event
func (l *Logger) LogError(event string, err error, data map[string]interface{}) {
	logEntry := map[string]interface{}{
		"level":     "error",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"service":   l.service,
		"event":     event,
		"error":     err.Error(),
	}

	for k, v := range data {
		logEntry[k] = v
	}

	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("Failed to marshal log entry: %v", err)
		return
	}
	log.Println(string(jsonBytes))
}

// Server represents the HTTP server
type Server struct {
	logger      *Logger
	auth        *auth.Authenticator
	rateLimiter *auth.RateLimiter
	ptyManager  *pty.Manager
	wsHandler   *ws.Handler
	port        string
	staticPath  string
}

// NewServer creates a new server instance
func NewServer(port, shell, staticPath string) *Server {
	logger := &Logger{service: "agent-tunnel"}

	return &Server{
		logger:      logger,
		auth:        auth.NewAuthenticator(),
		rateLimiter: auth.NewRateLimiter(),
		ptyManager:  pty.NewManager(shell),
		port:        port,
		staticPath:  staticPath,
	}
}

// Start initializes and starts the HTTP server
func (s *Server) Start() error {
	// Create WebSocket handler
	s.wsHandler = ws.NewHandler(s.ptyManager, s.logger)

	// Setup routes
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)

	// WebSocket endpoint
	mux.Handle("/ws", s.wsHandler)

	// Static files
	fs := http.FileServer(http.Dir(s.staticPath))
	mux.Handle("/", fs)

	// Create HTTP server
	server := &http.Server{
		Addr:    ":" + s.port,
		Handler: mux,
	}

	log.Printf("Server starting on http://localhost:%s", s.port)

	// Graceful shutdown
	go s.handleShutdown(server)

	return server.ListenAndServe()
}

// handleLogin handles login requests
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIP := auth.GetClientIP(r.RemoteAddr)

	// Check rate limit
	if !s.rateLimiter.Allow(clientIP) {
		s.logger.LogEvent("rate_limit_hit", map[string]interface{}{
			"ip": clientIP,
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Too many failed attempts. Please try again later.",
		})
		return
	}

	// Parse request
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.LogError("login_parse_error", err, map[string]interface{}{
			"ip": clientIP,
		})
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Authenticate
	err := s.auth.Authenticate(req.Username, req.Password)
	if err != nil {
		remaining := s.rateLimiter.GetRemainingAttempts(clientIP)
		s.logger.LogEvent("login_attempt", map[string]interface{}{
			"ip":        clientIP,
			"username":  req.Username,
			"success":   false,
			"remaining": remaining,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":     "Invalid username or password",
			"remaining": remaining,
		})
		return
	}

	// Success - clear rate limit and set session cookie
	s.rateLimiter.Reset(clientIP)

	sessionID := fmt.Sprintf("%s_%d", req.Username, time.Now().Unix())
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})

	s.logger.LogEvent("login_attempt", map[string]interface{}{
		"ip":         clientIP,
		"username":   req.Username,
		"success":    true,
		"session_id": sessionID,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
	})
}

// handleLogout handles logout requests
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1, // Delete cookie
	})

	s.logger.LogEvent("logout", map[string]interface{}{
		"ip": auth.GetClientIP(r.RemoteAddr),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
	})
}

// handleShutdown handles graceful shutdown
func (s *Server) handleShutdown(server *http.Server) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	s.logger.LogEvent("server_shutdown", map[string]interface{}{})

	// Create a context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shutdown HTTP server
	if err := server.Shutdown(ctx); err != nil {
		s.logger.LogError("server_shutdown_error", err, map[string]interface{}{})
	}

	// Cleanup PTY sessions
	s.ptyManager.Cleanup()
}

func main() {
	// Parse command-line flags
	port := flag.String("port", "4020", "HTTP server port")
	shell := flag.String("shell", "", "Shell to use (default: $SHELL)")
	staticPath := flag.String("static", "./static", "Path to static files")
	flag.Parse()

	server := NewServer(*port, *shell, *staticPath)
	if err := server.Start(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
