package shared

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func ValidSHA256Signature(secret string, body []byte, header string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	parts := strings.SplitN(header, "=", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "sha256") {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimSpace(parts[1]))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(expected, provided)
}
