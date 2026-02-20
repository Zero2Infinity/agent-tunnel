# AgentTunnel - Agent Guidelines

## Project Overview

AgentTunnel is a browser-based terminal access tool for macOS. It provides secure remote terminal access via WebSocket, with PAM authentication and optional WireGuard VPN support.

## Build Commands

```bash
# Build the server
go build -o agent-tunnel cmd/server/main.go

# Build for production (optimized)
go build -ldflags="-s -w" -o agent-tunnel cmd/server/main.go

# Run in development mode
go run cmd/server/main.go

# Run with flags
./agent-tunnel -port 4020 -shell /bin/zsh

# Run with WireGuard VPN mode
./agent-tunnel -wg
./agent-tunnel -wg -wg-port 51820

# Generate client configuration
./agent-tunnel -gen-client my-iphone
./agent-tunnel -gen-client my-iphone -show-qr=false
./agent-tunnel -gen-client my-iphone -endpoint 1.2.3.4:51820

# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/auth

# Run a single test
go test -run TestFunctionName ./internal/auth

# Run a single test file
go test -v ./internal/auth -run TestRateLimiter

# Run with verbose output
go test -v ./...

# Check for race conditions
go test -race ./...

# Format code
go fmt ./...

# Vet code
go vet ./...

# Download dependencies
go mod download

# Tidy dependencies
go mod tidy
```

## Project Structure

```
agent-tunnel/
├── cmd/server/main.go           # Entry point, HTTP routes, server setup
├── internal/
│   ├── auth/
│   │   ├── pam.go              # PAM authentication
│   │   └── ratelimit.go        # IP-based rate limiting
│   ├── pty/
│   │   └── manager.go          # PTY session management
│   ├── tunnel/
│   │   ├── keys.go            # WireGuard key generation
│   │   ├── manager.go         # WireGuard device management
│   │   ├── peers.go           # Peer management & config generation
│   │   └── qrcode.go          # QR code generation
│   └── ws/
│       └── handler.go          # WebSocket handling
├── static/
│   ├── index.html              # Single-page UI
│   ├── app.js                  # Terminal client
│   └── style.css               # Mobile-optimized styles
├── go.mod
├── go.sum
└── README.md
```

## Code Style Guidelines

### Imports

Group imports in three sections, separated by blank lines:
1. Standard library packages
2. Third-party packages
3. Local packages (agent-tunnel/internal/*)

```go
import (
    "context"
    "fmt"
    "sync"

    "github.com/gorilla/websocket"
    "github.com/creack/pty"

    "agent-tunnel/internal/auth"
    "agent-tunnel/internal/pty"
)
```

### Naming Conventions

- **Packages**: lowercase, single word when possible (e.g., `auth`, `pty`, `ws`)
- **Types**: PascalCase, exported types start with uppercase (e.g., `Session`, `Manager`)
- **Interfaces**: PascalCase, often end with "-er" for single-method interfaces (e.g., `Logger`)
- **Functions/Methods**: PascalCase for exported, camelCase for unexported
- **Constants**: PascalCase for exported, camelCase for unexported
- **Acronyms**: Keep consistent case (e.g., `PTY`, `WebSocket`, `IP`)

### Type Definitions

- Use struct types for data containers
- Define interfaces for abstractions
- Prefer composition over inheritance

```go
type Session struct {
    ID      string
    Pty     *os.File
    Cmd     *exec.Cmd
    Mu      sync.Mutex
    Running bool
}

type Logger interface {
    LogEvent(event string, data map[string]interface{})
}
```

### Error Handling

- Return errors as the last return value
- Wrap errors with context using `fmt.Errorf`
- Use error wrapping with `%w` for proper error chains

```go
if err != nil {
    return fmt.Errorf("failed to start PTY: %w", err)
}
```

- Log errors with context using structured logging
- Never ignore errors silently

### JSON Logging

Use structured JSON logging throughout. The Logger interface requires:

```go
type Logger interface {
    LogEvent(event string, data map[string]interface{})
    LogError(event string, err error, data map[string]interface{})
}
```

Standard log events:
- `server_start`, `server_shutdown`
- `login_attempt`, `rate_limit_hit`, `logout`
- `ws_connect`, `ws_disconnect`, `ws_auth_failed`
- `pty_spawn`, `pty_resize`, `pty_read_error`, `pty_write_error`
- `wg_server_start`, `wg_peer_added`, `wg_add_peer_error`

### Concurrency

- Use `sync.Mutex` or `sync.RWMutex` for shared state
- Prefer `sync.RWMutex` when reads are more frequent than writes
- Always lock/unlock in the same function scope using `defer`
- Use `sync.WaitGroup` for goroutine coordination

```go
m.mu.Lock()
defer m.mu.Unlock()
```

### HTTP Handlers

- Check HTTP method first for non-GET handlers
- Use appropriate HTTP status codes
- Set `Content-Type: application/json` for JSON responses
- Use `http.Error` for simple error responses

```go
if r.Method != http.MethodPost {
    http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    return
}
```

### Comments

- No doc comments required for unexported functions/types
- Exported types and functions should have brief doc comments
- No inline comments explaining obvious code

### Testing Conventions

- Place tests in the same package with `_test.go` suffix
- Use table-driven tests for multiple test cases
- Test file naming: `<name>_test.go`
- Use `t.Run()` for subtests

```go
func TestRateLimiter(t *testing.T) {
    tests := []struct {
        name     string
        attempts int
        want     bool
    }{
        {"first attempt", 1, true},
        {"at limit", 5, false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test code
        })
    }
}
```

## Architecture Notes

### Authentication Flow

1. Client POSTs to `/api/login` with username/password
2. Server validates via PAM (macOS `login` service)
3. On success, set HTTP-only session cookie
4. Rate limit: 5 attempts per IP per 60 seconds

### WebSocket Flow

1. Client connects to `/ws` with session cookie
2. Server validates cookie, upgrades to WebSocket
3. Server creates PTY session
4. Bidirectional data flow: PTY <-> WebSocket <-> Browser

### Package Dependencies

- `github.com/gorilla/websocket` - WebSocket implementation
- `github.com/creack/pty` - PTY management
- `github.com/msteinert/pam/v2` - PAM authentication

### Platform Requirements

- macOS only (PAM authentication)
- Go 1.21+
- Modern web browser with WebSocket support

## Environment Variables

- `PORT` - HTTP server port (default: 4020)
- `SHELL` - Default shell for PTY sessions
- `STATIC_PATH` - Path to static files (default: ./static)
- `AGENT_TUNNEL_ENDPOINT` - Public endpoint for WireGuard client configs

## Common Tasks

### Adding a new API endpoint

1. Add handler method to `Server` struct in `cmd/server/main.go`
2. Register route in `Start()` method
3. Add corresponding logging events

### Adding a new internal package

1. Create directory under `internal/`
2. Define types and interfaces in `<name>.go`
3. Export through package-level `New*()` constructor functions
4. Import as `agent-tunnel/internal/<name>`

### Adding WireGuard support

1. Create `internal/tunnel/manager.go`
2. Implement key generation, peer management
3. Integrate with server startup in `cmd/server/main.go`
4. Add command-line flags for WireGuard options
