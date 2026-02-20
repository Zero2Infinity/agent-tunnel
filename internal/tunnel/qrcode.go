package tunnel

import (
	"fmt"
	"os"

	"github.com/mdp/qrterminal/v3"
)

func PrintQRCode(config string) error {
	if len(config) > 2953 {
		return fmt.Errorf("config too large for QR code (max 2953 bytes)")
	}

	qrterminal.Generate(config, qrterminal.M, os.Stdout)
	return nil
}

func PrintQRCodeHalfBlock(config string) error {
	if len(config) > 2953 {
		return fmt.Errorf("config too large for QR code (max 2953 bytes)")
	}

	qrterminal.GenerateHalfBlock(config, qrterminal.M, os.Stdout)
	return nil
}
