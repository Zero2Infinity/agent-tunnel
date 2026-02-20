# WireGuard Integration Notes

## Overview

AgentTunnel supports WireGuard VPN for secure remote terminal access. This document covers the implementation, usage, and troubleshooting.

## Current Status

| Feature | Status |
|---------|--------|
| Local network access | ✅ Working |
| Key generation | ✅ Working |
| Client config generation | ✅ Working |
| QR code generation | ✅ Working |
| Peer management | ✅ Working |
| Internet access | ❌ Blocked by ISP |

---

## Quick Start

### Start WireGuard Server

```bash
# Default port (51820)
sudo ./agent-tunnel -wg

# Custom port
sudo ./agent-tunnel -wg -wg-port 443

# Custom HTTP port
sudo ./agent-tunnel -wg -port 8080
```

### Generate Client Config

```bash
# With DDNS endpoint
./agent-tunnel -gen-client my-phone -endpoint tunnel16420.tplinkdns.com:51820

# With local IP (for testing)
./agent-tunnel -gen-client local-test -endpoint 192.168.68.62:51820

# Without QR code
./agent-tunnel -gen-client my-phone -endpoint <endpoint> -show-qr=false
```

### Test Connection

```bash
# Check WireGuard status
sudo wg show

# Check listening ports
sudo lsof -i :51820 -P -n

# Check routes
netstat -rn | grep 10.8

# Test local access
curl http://10.8.0.1:4020
```

---

## Architecture

### Network Topology

```
┌─────────────────┐                    ┌─────────────────┐
│   Phone/Client  │                    │    Mac/Server   │
│                 │    WireGuard       │                 │
│  10.8.0.x ──────┼────────────────────┼───── 10.8.0.1   │
│                 │      Tunnel        │       :4020     │
│  Browser ───────┼───► HTTP ─────────►│   Terminal UI   │
└─────────────────┘                    └─────────────────┘
```

### Access URLs

| Network | URL |
|---------|-----|
| Local machine | `http://localhost:4020` |
| Local network | `http://192.168.68.62:4020` |
| WireGuard VPN | `http://10.8.0.1:4020` |

---

## File Locations

| File | Path |
|------|------|
| Server keys | `~/.agent-tunnel/server_keys.json` |
| Peer storage | `~/.agent-tunnel/peers.json` |
| WireGuard config | `/etc/wireguard/wg0.conf` |
| Client configs | `<name>.conf` (current directory) |

---

## Implementation Details

### How It Works

1. **Server Startup**
   - Creates WireGuard config at `/etc/wireguard/wg0.conf`
   - Runs `wg-quick up wg0`
   - Detects actual interface name (e.g., `utun4`)
   - Adds route for VPN subnet (`10.8.0.0/24`)
   - Loads peers from `~/.agent-tunnel/peers.json`
   - Binds HTTP server to both `0.0.0.0` and `10.8.0.1`

2. **Peer Management**
   - Peers stored in JSON file
   - Loaded on server startup
   - Added to WireGuard with `wg set` command

3. **Client Config Generation**
   - Generates Curve25519 key pair
   - Assigns IP from `10.8.0.0/24` subnet
   - Creates WireGuard `.conf` file
   - Displays QR code for easy import

### Key Components

| File | Purpose |
|------|---------|
| `internal/tunnel/manager.go` | WireGuard manager using wg-quick |
| `internal/tunnel/keys.go` | Curve25519 key generation |
| `internal/tunnel/peers.go` | Peer storage & config generation |
| `internal/tunnel/qrcode.go` | QR code terminal output |

---

## Troubleshooting

### Local Network Works, Internet Doesn't

**Likely cause:** ISP blocking incoming UDP ports

**Diagnosis:**
1. Check if WireGuard is listening:
   ```bash
   sudo lsof -i :<port> -P -n
   ```

2. Check port forwarding on router (must be UDP, not TCP)

3. Test with different ports (443, 80, 500)

**Solutions:**
- Use Tailscale (handles NAT traversal)
- Use VPS with open ports
- Use Cloudflare Tunnel

### No Handshake / rx/tx = 0

**Possible causes:**
1. Peer not added to server - restart server
2. Keys don't match - regenerate client config
3. Port not forwarded - check router settings
4. Firewall blocking - check macOS firewall

### Can't Access 10.8.0.1:4020 Locally

**Diagnosis:**
```bash
# Check route exists
netstat -rn | grep 10.8

# Should show:
# 10.8.0.0/24        utun4
# 10.8.0.1           10.8.0.1    UH    utun4
```

**Fix:**
```bash
# Restart server to re-add route
sudo pkill -f agent-tunnel
sudo ./agent-tunnel -wg
```

### WireGuard Shows No Peers

**Fix:**
```bash
# Restart server to reload peers
sudo pkill -f agent-tunnel
sudo ./agent-tunnel -wg

# Verify peers loaded
sudo wg show
```

---

## Known Issues

### ISP Port Blocking

Many residential ISPs block incoming UDP ports:
- AT&T Fiber: Blocks most/all incoming ports
- Solution: Use Tailscale, VPS, or Cloudflare Tunnel

### TCP vs UDP Port Checkers

Most online port checkers (canyouseeme.org) only test TCP ports.
WireGuard uses UDP, so these tests may show "closed" even when open.

**Better test:** Try actual WireGuard connection from external network.

---

## Future Improvements

1. **Tailscale Integration** - Add as alternative to WireGuard
2. **STUN/TURN Support** - Better NAT traversal
3. **IPv6 Support** - Better connectivity options
4. **Web UI** - Manage peers from browser
5. **Auto Port Detection** - Detect blocked ports automatically

---

## Related Files

- [AGENTS.md](AGENTS.md) - Development guidelines
- [README.md](README.md) - Project overview
