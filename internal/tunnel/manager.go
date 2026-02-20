package tunnel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const (
	DefaultServerIP   = "10.8.0.1"
	DefaultListenPort = 51820
	DefaultMTU        = 1420
	DefaultInterface  = "wg0"
)

type Manager struct {
	mu         sync.RWMutex
	serverIP   string
	listenPort int
	iface      string
	configPath string
	running    bool
}

func NewManager(serverIP string, listenPort int) (*Manager, error) {
	if serverIP == "" {
		serverIP = DefaultServerIP
	}
	if listenPort == 0 {
		listenPort = DefaultListenPort
	}

	configPath := "/etc/wireguard"

	return &Manager{
		serverIP:   serverIP,
		listenPort: listenPort,
		iface:      DefaultInterface,
		configPath: configPath,
		running:    false,
	}, nil
}

func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("manager already running")
	}

	keys, err := LoadOrGenerateServerKeys()
	if err != nil {
		return fmt.Errorf("failed to load server keys: %w", err)
	}

	if err := os.MkdirAll(m.configPath, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configContent := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s/24
ListenPort = %d
MTU = %d
`, keys.PrivateKey, m.serverIP, m.listenPort, DefaultMTU)

	configFile := filepath.Join(m.configPath, m.iface+".conf")
	if err := os.WriteFile(configFile, []byte(configContent), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	cmd := exec.Command("wg-quick", "up", m.iface)
	cmd.Env = append(os.Environ(), "PATH=/opt/homebrew/bin:/usr/local/bin:"+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to start WireGuard: %w, output: %s", err, string(output))
	}

	actualIface, err := m.detectInterface()
	if err != nil {
		fmt.Printf("Warning: could not detect actual interface name: %v\n", err)
	} else {
		m.iface = actualIface
		fmt.Printf("Detected WireGuard interface: %s\n", actualIface)
	}

	if err := m.addVPNRoute(); err != nil {
		fmt.Printf("Warning: failed to add VPN route: %v\n", err)
	}

	m.running = true
	return nil
}

func (m *Manager) addVPNRoute() error {
	cmd := exec.Command("route", "-n", "add", "-inet", "10.8.0.0/24", "-interface", m.iface)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add route: %w, output: %s", err, string(output))
	}
	fmt.Printf("Added route: 10.8.0.0/24 -> %s\n", m.iface)
	return nil
}

func (m *Manager) detectInterface() (string, error) {
	cmd := exec.Command("wg", "show", "interfaces")
	cmd.Env = append(os.Environ(), "PATH=/opt/homebrew/bin:/usr/local/bin:"+os.Getenv("PATH"))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get interfaces: %w", err)
	}

	interfaces := strings.TrimSpace(string(output))
	if interfaces == "" {
		return "", fmt.Errorf("no WireGuard interfaces found")
	}

	ifaces := strings.Split(interfaces, "\n")
	if len(ifaces) > 0 {
		return strings.TrimSpace(ifaces[0]), nil
	}

	return "", fmt.Errorf("could not parse interface name")
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	cmd := exec.Command("wg-quick", "down", DefaultInterface)
	cmd.Env = append(os.Environ(), "PATH=/opt/homebrew/bin:/usr/local/bin:"+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stop WireGuard: %w, output: %s", err, string(output))
	}

	m.running = false
	return nil
}

func (m *Manager) AddPeer(publicKeyB64 string, allowedIPs string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return fmt.Errorf("manager not running")
	}

	if allowedIPs == "" {
		allowedIPs = m.serverIP + "/32"
	}

	cmd := exec.Command("wg", "set", m.iface, "peer", publicKeyB64, "allowed-ips", allowedIPs)
	cmd.Env = append(os.Environ(), "PATH=/opt/homebrew/bin:/usr/local/bin:"+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add peer to %s: %w, output: %s", m.iface, err, string(output))
	}

	return nil
}

func (m *Manager) AddPeerWithIP(publicKeyB64 string, peerIP string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return fmt.Errorf("manager not running")
	}

	cmd := exec.Command("wg", "set", m.iface, "peer", publicKeyB64, "allowed-ips", peerIP+"/32")
	cmd.Env = append(os.Environ(), "PATH=/opt/homebrew/bin:/usr/local/bin:"+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add peer to %s: %w, output: %s", m.iface, err, string(output))
	}

	return nil
}

func (m *Manager) RemovePeer(publicKeyB64 string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.running {
		return fmt.Errorf("manager not running")
	}

	cmd := exec.Command("wg", "set", m.iface, "peer", publicKeyB64, "remove")
	cmd.Env = append(os.Environ(), "PATH=/opt/homebrew/bin:/usr/local/bin:"+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove peer from %s: %w, output: %s", m.iface, err, string(output))
	}

	return nil
}

func (m *Manager) GetServerIP() string {
	return m.serverIP
}

func (m *Manager) GetListenPort() int {
	return m.listenPort
}

func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

func (m *Manager) GetInterface() string {
	return m.iface
}
