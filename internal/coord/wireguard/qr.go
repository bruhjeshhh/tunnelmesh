package wireguard

import (
	"encoding/base64"
	"errors"

	"github.com/skip2/go-qrcode"
)

// GenerateQRCode generates a QR code PNG image from the given config string.
// Returns the PNG image data.
func GenerateQRCode(config string, size int) ([]byte, error) {
	if config == "" {
		return nil, errors.New("config cannot be empty")
	}

	png, err := qrcode.Encode(config, qrcode.Medium, size)
	if err != nil {
		return nil, err
	}

	return png, nil
}

// GenerateQRCodeDataURL generates a QR code as a data URL.
// Returns a string like "data:image/png;base64,..."
func GenerateQRCodeDataURL(config string, size int) (string, error) {
	png, err := GenerateQRCode(config, size)
	if err != nil {
		return "", err
	}

	// Encode as base64 data URL
	b64 := base64.StdEncoding.EncodeToString(png)
	return "data:image/png;base64," + b64, nil
}
