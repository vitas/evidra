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
func (a *GenericAdapter) Canonicalize(tool, operation, environment string, rawArtifact []byte) (CanonResult, error) {
	r := canonicalizeGeneric(tool, operation, environment, rawArtifact)
	return r, nil
}

func canonicalizeGeneric(tool, operation, environment string, rawArtifact []byte) CanonResult {
	artifactDigest := sha256Hex(rawArtifact)

	identity := []ResourceID{{
		Name: artifactDigest,
	}}

	action := CanonicalAction{
		Tool:              tool,
		Operation:         operation,
		OperationClass:    "unknown",
		ResourceIdentity:  identity,
		ScopeClass:        ResolveScopeClass(environment, identity),
		ResourceCount:     1,
		ResourceShapeHash: artifactDigest,
	}

	actionJSON, _ := json.Marshal(action)
	intentDigest := ComputeIntentDigest(action)

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

// ComputeArtifactDigest returns the sha256:<hex> digest of raw artifact bytes.
func ComputeArtifactDigest(data []byte) string {
	return SHA256Hex(data)
}

// keep unexported alias for internal use
func sha256Hex(data []byte) string {
	return SHA256Hex(data)
}
