package auth

import (
	"fmt"

	"github.com/msteinert/pam/v2"
)

// Authenticator handles PAM authentication
type Authenticator struct{}

// NewAuthenticator creates a new PAM authenticator
func NewAuthenticator() *Authenticator {
	return &Authenticator{}
}

// Authenticate validates username/password using PAM
func (a *Authenticator) Authenticate(username, password string) error {
	// Use "login" service which exists on all macOS systems
	transaction, err := pam.StartFunc("login", username, func(style pam.Style, msg string) (string, error) {
		switch style {
		case pam.PromptEchoOff:
			return password, nil
		case pam.PromptEchoOn:
			return password, nil
		case pam.ErrorMsg:
			return "", fmt.Errorf("PAM error: %s", msg)
		case pam.TextInfo:
			return "", nil
		default:
			return "", fmt.Errorf("unrecognized PAM message style: %v", style)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to start PAM transaction: %w", err)
	}
	defer transaction.End()

	err = transaction.Authenticate(0)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	return nil
}
