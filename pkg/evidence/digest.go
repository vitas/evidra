package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// SHA256Hex returns the hex-encoded SHA256 digest of data.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ComputeDeclaredIntentDigest computes a stable digest for declared intent identity.
// It excludes ArtifactDigest so shape/content changes do not alter intent identity.
func ComputeDeclaredIntentDigest(intent DeclaredIntent) string {
	identity := struct {
		Tool      string `json:"tool,omitempty"`
		Operation string `json:"operation,omitempty"`
		Target    string `json:"target,omitempty"`
		Command   string `json:"command,omitempty"`
	}{
		Tool:      strings.TrimSpace(intent.Tool),
		Operation: strings.TrimSpace(intent.Operation),
		Target:    strings.TrimSpace(intent.Target),
		Command:   strings.TrimSpace(intent.Command),
	}
	raw, _ := json.Marshal(identity)
	return SHA256Hex(raw)
}
