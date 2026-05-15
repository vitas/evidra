package evidence

import (
	"encoding/json"
	"strings"
)

// CanonicalAction is an optional external normalization envelope. Evidra core
// stores it when supplied, but does not require or produce it.
type CanonicalAction struct {
	Tool              string       `json:"tool"`
	Operation         string       `json:"operation"`
	OperationClass    string       `json:"operation_class"`
	ResourceIdentity  []ResourceID `json:"resource_identity,omitempty"`
	ScopeClass        string       `json:"scope_class"`
	ResourceCount     int          `json:"resource_count"`
	ResourceShapeHash string       `json:"resource_shape_hash,omitempty"`
}

// ResourceID identifies a resource inside an optional canonical action.
type ResourceID struct {
	APIVersion string `json:"api_version,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
	Type       string `json:"type,omitempty"`
	Actions    string `json:"actions,omitempty"`
}

// ComputeCanonicalActionDigest computes a stable digest from canonical
// identity fields. Shape/content hash is deliberately excluded.
func ComputeCanonicalActionDigest(action CanonicalAction) string {
	identity := struct {
		Tool             string       `json:"tool"`
		Operation        string       `json:"operation"`
		OperationClass   string       `json:"operation_class"`
		ResourceIdentity []ResourceID `json:"resource_identity,omitempty"`
		ScopeClass       string       `json:"scope_class"`
		ResourceCount    int          `json:"resource_count"`
	}{
		Tool:             strings.TrimSpace(action.Tool),
		Operation:        strings.TrimSpace(action.Operation),
		OperationClass:   strings.TrimSpace(action.OperationClass),
		ResourceIdentity: action.ResourceIdentity,
		ScopeClass:       strings.TrimSpace(action.ScopeClass),
		ResourceCount:    action.ResourceCount,
	}
	raw, _ := json.Marshal(identity)
	return SHA256Hex(raw)
}

// NormalizeScopeClass maps common aliases to the optional canonical scope set.
func NormalizeScopeClass(scope string) string {
	v := strings.ToLower(strings.TrimSpace(scope))
	switch v {
	case "production", "prod":
		return "production"
	case "staging", "stage":
		return "staging"
	case "development", "dev", "test", "sandbox":
		return "development"
	default:
		return "unknown"
	}
}

// ResolveScopeClass derives scope from an explicit environment or resource namespace.
func ResolveScopeClass(env string, resources []ResourceID) string {
	if normalized := NormalizeScopeClass(env); normalized != "unknown" {
		return normalized
	}
	for _, r := range resources {
		ns := strings.ToLower(strings.TrimSpace(r.Namespace))
		switch {
		case strings.Contains(ns, "prod"):
			return "production"
		case strings.Contains(ns, "stag"):
			return "staging"
		case strings.Contains(ns, "dev"):
			return "development"
		}
	}
	return "unknown"
}
