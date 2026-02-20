package tunnel

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/curve25519"
)

const (
	configDirName  = ".agent-tunnel"
	serverKeysFile = "server_keys.json"
)

type ServerKeys struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
}

func getConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	configDir := filepath.Join(homeDir, configDirName)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}
	return configDir, nil
}

func getServerKeysPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, serverKeysFile), nil
}

func GenerateKeyPair() ([]byte, []byte, error) {
	privateKey := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(privateKey); err != nil {
		return nil, nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to derive public key: %w", err)
	}

	return privateKey, publicKey, nil
}

func PublicKeyFromPrivate(privateKey []byte) ([]byte, error) {
	if len(privateKey) != curve25519.ScalarSize {
		return nil, fmt.Errorf("invalid private key size: got %d, want %d", len(privateKey), curve25519.ScalarSize)
	}
	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("failed to derive public key: %w", err)
	}
	return publicKey, nil
}

func EncodeBase64(key []byte) string {
	return base64.StdEncoding.EncodeToString(key)
}

func DecodeBase64(keyStr string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 key: %w", err)
	}
	return key, nil
}

func Base64ToHex(base64Key string) (string, error) {
	key, err := DecodeBase64(base64Key)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}

func LoadOrGenerateServerKeys() (*ServerKeys, error) {
	keysPath, err := getServerKeysPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(keysPath)
	if err == nil {
		var keys ServerKeys
		if err := json.Unmarshal(data, &keys); err != nil {
			return nil, fmt.Errorf("failed to parse server keys: %w", err)
		}
		return &keys, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read server keys: %w", err)
	}

	privateKey, publicKey, err := GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	keys := &ServerKeys{
		PrivateKey: EncodeBase64(privateKey),
		PublicKey:  EncodeBase64(publicKey),
	}

	data, err = json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal server keys: %w", err)
	}

	if err := os.WriteFile(keysPath, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to write server keys: %w", err)
	}

	return keys, nil
}
