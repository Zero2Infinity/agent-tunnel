package pty

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

// Session represents a PTY session
type Session struct {
	ID      string
	Pty     *os.File
	Cmd     *exec.Cmd
	Mu      sync.Mutex
	Running bool
}

// Manager manages PTY sessions
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	shell    string
}

// NewManager creates a new PTY manager
func NewManager(shell string) *Manager {
	if shell == "" {
		shell = os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/bash"
		}
	}

	return &Manager{
		sessions: make(map[string]*Session),
		shell:    shell,
	}
}

// Create creates a new PTY session
func (m *Manager) Create(sessionID string) (*Session, error) {
	cmd := exec.Command(m.shell)
	cmd.Env = os.Environ()

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	session := &Session{
		ID:      sessionID,
		Pty:     ptmx,
		Cmd:     cmd,
		Running: true,
	}

	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	// Monitor process exit
	go func() {
		cmd.Wait()
		session.Mu.Lock()
		session.Running = false
		session.Mu.Unlock()
		m.Remove(sessionID)
	}()

	return session, nil
}

// Get retrieves a session by ID
func (m *Manager) Get(sessionID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, exists := m.sessions[sessionID]
	return session, exists
}

// Remove removes a session
func (m *Manager) Remove(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if session, exists := m.sessions[sessionID]; exists {
		session.Mu.Lock()
		if session.Running {
			session.Pty.Close()
			if session.Cmd.Process != nil {
				session.Cmd.Process.Kill()
			}
		}
		session.Mu.Unlock()
		delete(m.sessions, sessionID)
	}
}

// Resize resizes a PTY session
func (m *Manager) Resize(sessionID string, rows, cols uint16) error {
	m.mu.RLock()
	session, exists := m.sessions[sessionID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.Mu.Lock()
	defer session.Mu.Unlock()

	if !session.Running {
		return fmt.Errorf("session is not running")
	}

	return pty.Setsize(session.Pty, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

// List returns all active session IDs
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Cleanup closes all sessions
func (m *Manager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, session := range m.sessions {
		session.Mu.Lock()
		if session.Running {
			session.Pty.Close()
			if session.Cmd.Process != nil {
				session.Cmd.Process.Kill()
			}
		}
		session.Mu.Unlock()
		delete(m.sessions, id)
	}
}
