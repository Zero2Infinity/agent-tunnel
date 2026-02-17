# AgentTunnel

AgentTunnel is an open-source tool that bridges your local terminal to a web browser, allowing you to access your CLI environment and AI agents from any device (iPad, mobile phone, or another computer).

## Features

- **Browser-Based Terminal**: Full-featured terminal using xterm.js with real-time output
- **Secure Authentication**: macOS PAM authentication with rate limiting (5 attempts per minute)
- **Session-Based Access**: Cookie-based sessions that expire when the browser closes
- **Mobile Optimized**: Responsive design optimized for tablets and mobile devices
- **WireGuard VPN**: Built-in WireGuard support for secure remote access from anywhere
- **JSON Logging**: Structured logging to stdout for monitoring and debugging
- **No Port Forwarding**: Just open one UDP port for WireGuard, access server via VPN IP

## Architecture

AgentTunnel consists of three main components:

1. **Frontend**: Single-page application with xterm.js terminal emulator
2. **PTY Server**: Go backend that manages shell processes and WebSocket connections
3. **Authentication**: PAM-based authentication with rate limiting

## Requirements

- macOS (for PAM authentication)
- Go 1.21 or later
- Modern web browser (Chrome, Safari, Firefox)

## Installation

### 1. Clone the repository

```bash
git clone https://github.com/yourusername/agent-tunnel.git
cd agent-tunnel
```

### 2. Install dependencies

```bash
go mod download
```

### 3. Build the server

```bash
go build -o agent-tunnel cmd/server/main.go
```

## Usage

### Standard Mode (Local Network)

```bash
# Run with default settings (port 4020, default shell)
./agent-tunnel

# Or with custom settings
PORT=8080 SHELL=/bin/zsh ./agent-tunnel
```

### WireGuard Mode (Secure Remote Access)

Access your terminal securely from anywhere using WireGuard VPN.

**1. Start the server with WireGuard enabled:**

```bash
# Enable WireGuard VPN mode (default port 51820)
./agent-tunnel -wg

# With custom WireGuard port
./agent-tunnel -wg -wg-port 51820

# Set your public endpoint for client configs
export AGENT_TUNNEL_ENDPOINT="your-public-ip:51820"
./agent-tunnel -wg
```

**2. Generate client configuration:**

```bash
# Generate a client config file
./agent-tunnel -gen-client my-iphone

# Creates my-iphone.conf with client configuration
```

**3. Setup client device:**

- **iOS/Android**: Install WireGuard app → Import from file → Select my-iphone.conf
- **macOS/Windows**: WireGuard app → Import tunnel from file
- **Connect to VPN**, then open browser to `http://10.8.0.1:4020`

**4. Firewall Setup:**

```bash
# Open WireGuard UDP port on your router/firewall
sudo ufw allow 51820/udp
# Server is now accessible remotely via VPN only
```

### Command-Line Flags

- `-port`: HTTP server port when not using WireGuard (default: 4020)
- `-shell`: Shell to use for PTY sessions (default: $SHELL or /bin/bash)
- `-static`: Path to static files (default: ./static)
- `-wg`: Enable WireGuard VPN mode
- `-wg-port`: WireGuard listen port (default: 51820)
- `-gen-client <name>`: Generate client configuration file

### Environment Variables

- `PORT`: HTTP server port (default: 4020)
- `SHELL`: Shell to use for PTY sessions (default: $SHELL or /bin/bash)
- `STATIC_PATH`: Path to static files (default: ./static)
- `AGENT_TUNNEL_ENDPOINT`: Public endpoint for client config generation (e.g., "1.2.3.4:51820")

### Access the terminal

1. Open your browser and navigate to `http://localhost:4020`
2. Enter your macOS username and password
3. Start using your terminal!

## Security

### Authentication
- Uses macOS PAM for authentication
- Rate limiting: 5 failed attempts per IP per minute
- Session cookies are HTTP-only and expire when browser closes
- No persistent sessions stored on server

### Network Security

**Standard Mode:**
- Designed for local network access
- Bind to localhost or specific interface

**WireGuard Mode:**
- Built-in WireGuard VPN using userspace implementation
- Server only accessible through VPN tunnel (10.8.0.1:4020)
- No TLS/HTTPS required (WireGuard provides transport security)
- Cryptokey routing ensures only authorized clients can connect
- Automatic key generation and management
- No port forwarding needed except single UDP port for WireGuard

### Rate Limiting
After 5 failed login attempts from the same IP address:
- Further attempts are blocked for 60 seconds
- Remaining attempts are displayed in the UI
- Rate limit resets after successful login

## Logging

AgentTunnel outputs structured JSON logs to stdout:

```json
{"level":"info","timestamp":"2026-02-16T10:30:00Z","service":"agent-tunnel","event":"server_start","port":"4020"}
{"level":"info","timestamp":"2026-02-16T10:30:05Z","service":"agent-tunnel","event":"login_attempt","ip":"100.64.0.1","username":"user","success":true}
{"level":"info","timestamp":"2026-02-16T10:32:00Z","service":"agent-tunnel","event":"pty_spawn","shell":"/bin/zsh","pid":1234}
```

### Log Events

- `server_start`: Server initialized
- `login_attempt`: Authentication attempt (success/failure)
- `rate_limit_hit`: Too many failed attempts
- `pty_spawn`: New PTY session created
- `ws_connect`: WebSocket connection established
- `ws_disconnect`: Session ended with duration
- `pty_resize`: Terminal size changed
- `error`: Any errors

## Project Structure

```
agent-tunnel/
├── cmd/server/main.go           # Entry point, HTTP routes
├── internal/
│   ├── auth/
│   │   ├── pam.go              # PAM authentication
│   │   └── ratelimit.go        # Rate limiting
│   ├── pty/
│   │   └── manager.go          # PTY session management
│   ├── tunnel/
│   │   └── manager.go          # WireGuard tunnel management
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

## Development

### Run in development mode

```bash
go run cmd/server/main.go
```

### Build for production

```bash
go build -ldflags="-s -w" -o agent-tunnel cmd/server/main.go
```

### Testing

```bash
go test ./...
```

## Future Enhancements

### Multi-Machine Support (Phase 2)
- WireGuard mesh networking across multiple home machines
- Agent registry across machines
- Machine discovery and health monitoring
- Work distribution to available machines

### Agent Proxy (Phase 3)
- REST API for agent control
- Structured agent status monitoring
- Background agent support (non-terminal agents)
- Cross-machine agent communication

### Git Worktree Integration
- Auto-detect git worktrees
- Project context switching
- Per-project environment variables

## Troubleshooting

### WireGuard Connection Issues

#### "Unable to import tunnel" on mobile app

This usually means the configuration file has formatting issues or missing information.

**Solution:**
1. Check that the .conf file has proper format:
   ```
   [Interface]
   PrivateKey = <base64-key>
   Address = 10.8.0.2/32
   DNS = 1.1.1.1, 8.8.8.8

   [Peer]
   PublicKey = <server-public-key>
   AllowedIPs = 0.0.0.0/0
   Endpoint = <server-ip>:51820
   PersistentKeepalive = 25
   ```

2. Ensure there are no `<` or `>` characters in the file (placeholder text)

3. Try using the QR code instead:
   ```bash
   ./agent-tunnel -gen-client my-device -show-qr
   ```
   Then scan the QR code with WireGuard app.

#### "Handshake did not complete" or connection timeout

This means the client can't reach the server.

**For Local Network (Same WiFi):**
- The generated config uses your local IP (e.g., 192.168.x.x)
- Both devices must be on the same network
- Disable firewall temporarily to test: `sudo ufw disable`

**For Remote Access (Internet):**
1. Find your public IPv4 address (use `-4` flag to force IPv4):
   ```bash
   curl -4 ifconfig.me
   ```
   Note: Without the `-4` flag, you might get an IPv6 address which won't work with most WireGuard setups.

2. Set the endpoint when generating config:
   ```bash
   export AGENT_TUNNEL_ENDPOINT="1.2.3.4:51820"
   ./agent-tunnel -gen-client my-device
   ```

3. Open port 51820/UDP on your router (port forwarding)
   - Log into your router admin panel
   - Find "Port Forwarding" or "Virtual Servers"
   - Forward UDP port 51820 to your computer's local IP

#### Testing the connection

1. **Test locally first:**
   ```bash
   # On server
   ./agent-tunnel -wg
   
   # Check if port is listening
   sudo lsof -i :51820
   ```

2. **Verify config is valid:**
   ```bash
   # The validation runs automatically when generating
   ./agent-tunnel -gen-client test
   ```

3. **Check WireGuard status:**
   - In the mobile app, look for transfer statistics
   - If "Received" stays at 0 bytes, traffic isn't reaching the server

#### Common Issues

| Issue | Solution |
|-------|----------|
| Config won't import | Ensure file ends with `.conf`, not `.txt` |
| QR code won't scan | Increase terminal font size or use file import |
| Can connect but no internet | Change `AllowedIPs = 0.0.0.0/0` to `AllowedIPs = 10.8.0.0/24` |
| Works on WiFi but not cellular | Check router port forwarding; verify public IP |
| Connection drops after a while | This is normal - tap to reconnect in WireGuard app |

### PAM Authentication Issues
If you encounter PAM authentication errors:
1. Ensure your macOS username and password are correct
2. Check that your user account has shell access
3. Verify PAM configuration for the `agent-tunnel` service

### Port Already in Use
If port 4020 is already in use:
```bash
PORT=8080 ./agent-tunnel
```

### WebSocket Connection Failed
1. Check that the server is running
2. Verify you're using the correct URL
3. Check browser console for JavaScript errors
4. Ensure session cookie is set (try logging in again)

## Inspiration

AgentTunnel is inspired by [VibeTunnel](https://github.com/amantus-ai/vibetunnel) by Amantus Machina, which provides similar functionality with a focus on AI agent integration.

## License

MIT License - see LICENSE file for details

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## Acknowledgments

- [xterm.js](https://xtermjs.org/) - The terminal emulator used in the frontend
- [creack/pty](https://github.com/creack/pty) - PTY management for Go
- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket implementation
- [msteinert/pam](https://github.com/msteinert/pam) - PAM authentication for Go
