package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agent-tunnel/internal/auth"
	"agent-tunnel/internal/pty"
	"agent-tunnel/internal/tunnel"
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
	tunnelMgr   *tunnel.Manager
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
	s.logger.LogEvent("server_start", map[string]interface{}{
		"port":       s.port,
		"staticPath": s.staticPath,
	})

	s.wsHandler = ws.NewHandler(s.ptyManager, s.logger)

	mux := http.NewServeMux()

	mux.HandleFunc("/api/login", s.handleLogin)
	mux.HandleFunc("/api/logout", s.handleLogout)

	mux.Handle("/ws", s.wsHandler)

	fs := http.FileServer(http.Dir(s.staticPath))
	mux.Handle("/", fs)

	var listener net.Listener
	var err error

	log.Printf("Attempting to bind to :%s", s.port)
	listener, err = net.Listen("tcp", ":"+s.port)
	if err != nil {
		return fmt.Errorf("failed to listen on port %s: %w", s.port, err)
	}
	log.Printf("Successfully bound to :%s", s.port)

	server := &http.Server{
		Handler: mux,
	}

	go s.handleShutdown(server, listener)

	if s.tunnelMgr != nil {
		wgAddr := s.tunnelMgr.GetServerIP() + ":" + s.port
		wgListener, wgErr := net.Listen("tcp", wgAddr)
		if wgErr != nil {
			log.Printf("Warning: Could not bind to WireGuard IP %s: %v", wgAddr, wgErr)
		} else {
			log.Printf("Successfully bound to WireGuard IP: %s", wgAddr)
			wgServer := &http.Server{Handler: mux}
			go func() {
				if err := wgServer.Serve(wgListener); err != nil && err != http.ErrServerClosed {
					log.Printf("WireGuard listener error: %v", err)
				}
			}()
			s.logger.LogEvent("wg_server_start", map[string]interface{}{
				"serverIP":   s.tunnelMgr.GetServerIP(),
				"port":       s.port,
				"listenPort": s.tunnelMgr.GetListenPort(),
				"wgListener": "active",
			})
		}
		log.Printf("Server starting on http://localhost:%s (WireGuard: http://%s:%s)",
			s.port, s.tunnelMgr.GetServerIP(), s.port)
	} else {
		log.Printf("Server starting on http://localhost:%s", s.port)
	}

	return server.Serve(listener)
}

// handleLogin handles login requests
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIP := auth.GetClientIP(r.RemoteAddr)

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
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
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
func (s *Server) handleShutdown(server *http.Server, listener net.Listener) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	s.logger.LogEvent("server_shutdown", map[string]interface{}{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		s.logger.LogError("server_shutdown_error", err, map[string]interface{}{})
	}

	listener.Close()

	s.ptyManager.Cleanup()

	if s.tunnelMgr != nil {
		s.tunnelMgr.Stop()
	}
}

func main() {
	port := flag.String("port", "4020", "HTTP server port")
	shell := flag.String("shell", "", "Shell to use (default: $SHELL)")
	staticPath := flag.String("static", "./static", "Path to static files")
	wgEnabled := flag.Bool("wg", false, "Enable WireGuard VPN mode")
	wgPort := flag.Int("wg-port", 51820, "WireGuard listen port")
	genClient := flag.String("gen-client", "", "Generate client configuration file")
	showQR := flag.Bool("show-qr", true, "Show QR code when generating client config")
	endpoint := flag.String("endpoint", "", "Public endpoint for client config (e.g., 1.2.3.4:51820)")
	serverIP := flag.String("server-ip", "10.8.0.1", "WireGuard server IP address")
	flag.Parse()

	logger := &Logger{service: "agent-tunnel"}

	if *genClient != "" {
		pm, err := tunnel.NewPeerManager(*serverIP)
		if err != nil {
			log.Fatalf("Failed to create peer manager: %v", err)
		}

		peer, err := pm.AddPeer(*genClient)
		if err != nil {
			log.Fatalf("Failed to add peer: %v", err)
		}

		endpointToUse := *endpoint
		if endpointToUse == "" {
			endpointToUse = fmt.Sprintf("127.0.0.1:%d", *wgPort)
		}

		config := pm.GenerateClientConfig(peer, endpointToUse)

		configFile := *genClient + ".conf"
		if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
			log.Fatalf("Failed to write config file: %v", err)
		}
		fmt.Printf("Client configuration saved to: %s\n", configFile)

		if *showQR {
			fmt.Println("\nScan this QR code with your WireGuard app:")
			if err := tunnel.PrintQRCodeHalfBlock(config); err != nil {
				log.Printf("Failed to generate QR code: %v", err)
			}
		}

		fmt.Printf("\nWireGuard endpoint: %s\n", endpointToUse)
		fmt.Printf("Client IP: %s\n", peer.IPAddress)

		return
	}

	var tunnelMgr *tunnel.Manager
	if *wgEnabled {
		mgr, err := tunnel.NewManager(*serverIP, *wgPort)
		if err != nil {
			log.Fatalf("Failed to create tunnel manager: %v", err)
		}

		if err := mgr.Start(); err != nil {
			log.Fatalf("Failed to start WireGuard: %v", err)
		}

		pm, err := tunnel.NewPeerManager(*serverIP)
		if err != nil {
			mgr.Stop()
			log.Fatalf("Failed to create peer manager: %v", err)
		}

		for _, peer := range pm.ListPeers() {
			if err := mgr.AddPeer(peer.PublicKey, peer.IPAddress+"/32"); err != nil {
				logger.LogError("wg_add_peer_error", err, map[string]interface{}{
					"peer": peer.Name,
				})
			} else {
				logger.LogEvent("wg_peer_added", map[string]interface{}{
					"peer":      peer.Name,
					"ip":        peer.IPAddress,
					"publicKey": peer.PublicKey,
				})
			}
		}

		tunnelMgr = mgr
	}

	server := &Server{
		logger:      logger,
		auth:        auth.NewAuthenticator(),
		rateLimiter: auth.NewRateLimiter(),
		ptyManager:  pty.NewManager(*shell),
		tunnelMgr:   tunnelMgr,
		port:        *port,
		staticPath:  *staticPath,
	}

	if err := server.Start(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
