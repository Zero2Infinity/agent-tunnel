package tunnel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const peersFileName = "peers.json"

type Peer struct {
	Name       string    `json:"name"`
	PublicKey  string    `json:"publicKey"`
	PrivateKey string    `json:"privateKey"`
	IPAddress  string    `json:"ipAddress"`
	CreatedAt  time.Time `json:"createdAt"`
}

type PeerManager struct {
	peers     []Peer
	configDir string
	serverIP  string
	nextIP    int
}

func NewPeerManager(serverIP string) (*PeerManager, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return nil, err
	}

	pm := &PeerManager{
		peers:     make([]Peer, 0),
		configDir: configDir,
		serverIP:  serverIP,
		nextIP:    2,
	}

	if err := pm.loadPeers(); err != nil {
		return nil, err
	}

	return pm, nil
}

func (pm *PeerManager) getPeersPath() string {
	return filepath.Join(pm.configDir, peersFileName)
}

func (pm *PeerManager) loadPeers() error {
	peersPath := pm.getPeersPath()

	data, err := os.ReadFile(peersPath)
	if err != nil {
		if os.IsNotExist(err) {
			pm.peers = make([]Peer, 0)
			return nil
		}
		return fmt.Errorf("failed to read peers file: %w", err)
	}

	if err := json.Unmarshal(data, &pm.peers); err != nil {
		return fmt.Errorf("failed to parse peers file: %w", err)
	}

	for _, peer := range pm.peers {
		ipNum := parseIPNumber(peer.IPAddress)
		if ipNum >= pm.nextIP {
			pm.nextIP = ipNum + 1
		}
	}

	return nil
}

func (pm *PeerManager) SavePeers() error {
	data, err := json.MarshalIndent(pm.peers, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal peers: %w", err)
	}

	peersPath := pm.getPeersPath()
	if err := os.WriteFile(peersPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write peers file: %w", err)
	}

	return nil
}

func parseIPNumber(ip string) int {
	var n int
	fmt.Sscanf(ip, "10.8.0.%d", &n)
	return n
}

func (pm *PeerManager) AddPeer(name string) (*Peer, error) {
	privateKey, publicKey, err := GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate peer keys: %w", err)
	}

	ip := fmt.Sprintf("10.8.0.%d", pm.nextIP)
	pm.nextIP++

	peer := Peer{
		Name:       name,
		PublicKey:  EncodeBase64(publicKey),
		PrivateKey: EncodeBase64(privateKey),
		IPAddress:  ip,
		CreatedAt:  time.Now(),
	}

	pm.peers = append(pm.peers, peer)

	if err := pm.SavePeers(); err != nil {
		return nil, err
	}

	return &peer, nil
}

func (pm *PeerManager) RemovePeer(name string) error {
	index := -1
	for i, peer := range pm.peers {
		if peer.Name == name {
			index = i
			break
		}
	}

	if index == -1 {
		return fmt.Errorf("peer not found: %s", name)
	}

	pm.peers = append(pm.peers[:index], pm.peers[index+1:]...)

	return pm.SavePeers()
}

func (pm *PeerManager) GetPeer(name string) *Peer {
	for _, peer := range pm.peers {
		if peer.Name == name {
			return &peer
		}
	}
	return nil
}

func (pm *PeerManager) ListPeers() []Peer {
	return pm.peers
}

func (pm *PeerManager) GenerateClientConfig(peer *Peer, endpoint string) string {
	if endpoint == "" {
		endpoint = fmt.Sprintf("127.0.0.1:%d", DefaultListenPort)
	}

	config := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s/32
DNS = 1.1.1.1

[Peer]
PublicKey = %s
AllowedIPs = 0.0.0.0/0
Endpoint = %s
PersistentKeepalive = 25
`, peer.PrivateKey, peer.IPAddress, getServerPublicKey(), endpoint)

	return config
}

func getServerPublicKey() string {
	keys, err := LoadOrGenerateServerKeys()
	if err != nil {
		return ""
	}
	return keys.PublicKey
}
