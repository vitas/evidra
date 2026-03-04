package canon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// GenericAdapter is the fallback adapter for unknown tools.
type GenericAdapter struct{}

func (a *GenericAdapter) Name() string            { return "generic/v1" }
func (a *GenericAdapter) CanHandle(_ string) bool { return true }
func (a *GenericAdapter) Canonicalize(tool, operation string, rawArtifact []byte) (CanonResult, error) {
	r := canonicalizeGeneric(tool, operation, rawArtifact)
	return r, nil
}

func canonicalizeGeneric(tool, operation string, rawArtifact []byte) CanonResult {
	artifactDigest := sha256Hex(rawArtifact)

	identity := []ResourceID{{
		Name: artifactDigest,
	}}

	action := CanonicalAction{
		Tool:              tool,
		Operation:         operation,
		OperationClass:    "unknown",
		ResourceIdentity:  identity,
		ScopeClass:        "single",
		ResourceCount:     1,
		ResourceShapeHash: artifactDigest,
	}

	actionJSON, _ := json.Marshal(action)
	intentDigest := sha256Hex(actionJSON)

	return CanonResult{
		ArtifactDigest:  artifactDigest,
		IntentDigest:    intentDigest,
		CanonicalAction: action,
		CanonVersion:    "generic/v1",
		RawAction:       actionJSON,
	}
}

// SHA256Hex returns the hex-encoded SHA256 digest of data.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// keep unexported alias for internal use
func sha256Hex(data []byte) string {
	return SHA256Hex(data)
}
