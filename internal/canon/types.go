package canon

import (
	"encoding/json"
	"strings"
	"time"
)

// CanonicalAction is the normalized representation of an infrastructure operation.
type CanonicalAction struct {
	Tool              string       `json:"tool"`
	Operation         string       `json:"operation"`
	OperationClass    string       `json:"operation_class"`
	ResourceIdentity  []ResourceID `json:"resource_identity"`
	ScopeClass        string       `json:"scope_class"`
	ResourceCount     int          `json:"resource_count"`
	ResourceShapeHash string       `json:"resource_shape_hash"`
}

// intentFields contains only the identity fields used to compute intent_digest.
// Deliberately excludes resource_shape_hash so that shape changes do not
// alter the intent digest.
type intentFields struct {
	Tool             string       `json:"tool"`
	Operation        string       `json:"operation"`
	OperationClass   string       `json:"operation_class"`
	ResourceIdentity []ResourceID `json:"resource_identity"`
	ScopeClass       string       `json:"scope_class"`
	ResourceCount    int          `json:"resource_count"`
}

// ComputeIntentDigest returns the SHA256 digest of the identity-only fields
// of a CanonicalAction, excluding resource_shape_hash.
func ComputeIntentDigest(action CanonicalAction) string {
	fields := intentFields{
		Tool:             action.Tool,
		Operation:        action.Operation,
		OperationClass:   action.OperationClass,
		ResourceIdentity: action.ResourceIdentity,
		ScopeClass:       action.ScopeClass,
		ResourceCount:    action.ResourceCount,
	}
	data, _ := json.Marshal(fields)
	return SHA256Hex(data)
}

// Prescription records intent before an infrastructure operation.
type Prescription struct {
	ID              string          `json:"prescription_id"`
	CanonicalAction CanonicalAction `json:"canonical_action"`
	ArtifactDigest  string          `json:"artifact_digest"`
	IntentDigest    string          `json:"intent_digest"`
	RiskLevel       string          `json:"risk_level"`
	RiskTags        []string        `json:"risk_tags"`
	RiskDetails     []string        `json:"risk_details"`
	Timestamp       time.Time       `json:"ts"`
	Signature       string          `json:"signature"`
}

// Report records the outcome of an infrastructure operation.
type Report struct {
	ID             string    `json:"report_id"`
	PrescriptionID string    `json:"prescription_id"`
	ActorID        string    `json:"actor_id"`
	Tool           string    `json:"tool"`
	Operation      string    `json:"operation"`
	ExitCode       int       `json:"exit_code"`
	ArtifactDigest string    `json:"artifact_digest"`
	Timestamp      time.Time `json:"ts"`
	Verdict        string    `json:"verdict"`
	Signals        []string  `json:"signals"`
	Signature      string    `json:"signature"`
}

// ResourceID identifies a single resource within a canonical action.
type ResourceID struct {
	APIVersion string `json:"api_version,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
	Type       string `json:"type,omitempty"`
	Actions    string `json:"actions,omitempty"`
}

// CanonResult holds the output of canonicalization.
type CanonResult struct {
	ArtifactDigest  string // SHA256 of raw bytes
	IntentDigest    string // SHA256 of canonical JSON
	CanonicalAction CanonicalAction
	CanonVersion    string          // e.g. "k8s/v1", "terraform/v1", "generic/v1"
	RawAction       json.RawMessage // JSON-serialized CanonicalAction
	ParseError      error           // non-nil if adapter couldn't parse
}

// Adapter normalizes raw tool artifacts into CanonResult.
type Adapter interface {
	Name() string
	CanHandle(tool string) bool
	Canonicalize(tool, operation, environment string, rawArtifact []byte) (CanonResult, error)
}

// DefaultAdapters returns the built-in adapter chain in selection order:
// k8s → terraform → docker → generic fallback.
func DefaultAdapters() []Adapter {
	return []Adapter{
		&K8sAdapter{},
		&TerraformAdapter{},
		&DockerAdapter{},
		&GenericAdapter{},
	}
}

// SelectAdapter returns the first adapter that can handle the given tool.
// Falls back to GenericAdapter if none match.
func SelectAdapter(tool string, adapters []Adapter) Adapter {
	for _, a := range adapters {
		if a.CanHandle(tool) {
			return a
		}
	}
	return &GenericAdapter{}
}

// Canonicalize dispatches to the appropriate adapter based on tool name.
func Canonicalize(tool, operation, environment string, rawArtifact []byte) CanonResult {
	adapter := SelectAdapter(tool, DefaultAdapters())
	result, err := adapter.Canonicalize(tool, operation, environment, rawArtifact)
	if err != nil {
		result.ParseError = err
	}
	return result
}

func k8sOperationClass(op string) string {
	switch op {
	case "apply", "create", "patch", "upgrade", "install":
		return "mutate"
	case "delete", "uninstall":
		return "destroy"
	case "get", "describe", "logs":
		return "read"
	default:
		return "unknown"
	}
}

func terraformOperationClass(op string) string {
	switch op {
	case "apply":
		return "mutate"
	case "destroy":
		return "destroy"
	case "plan":
		return "plan"
	default:
		return "unknown"
	}
}

// ResolveScopeClass determines the environment-based scope class.
// If env is explicitly "production", "staging", or "development", it is returned directly.
// Otherwise, resource namespaces are scanned for environment hints.
// Falls back to "unknown" if no match is found.
func ResolveScopeClass(env string, resources []ResourceID) string {
	envRaw := strings.TrimSpace(env)
	if envRaw != "" {
		normalized := NormalizeScopeClass(envRaw)
		if normalized != "unknown" {
			return normalized
		}
	}

	// Scan resource namespaces for hints.
	for _, r := range resources {
		ns := strings.ToLower(r.Namespace)
		if ns == "" {
			continue
		}
		if strings.Contains(ns, "prod") {
			return "production"
		}
		if strings.Contains(ns, "stag") {
			return "staging"
		}
		if strings.Contains(ns, "dev") {
			return "development"
		}
	}

	return "unknown"
}

// NormalizeScopeClass maps protocol aliases to runtime canonical scope values.
// Runtime canonical set: production | staging | development | unknown.
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
