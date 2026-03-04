package canon

import "encoding/json"

// CanonicalAction is the normalized representation of an infrastructure operation.
type CanonicalAction struct {
	Tool              string       `json:"tool"`
	Operation         string       `json:"operation"`
	OperationClass    string       `json:"operation_class"`
	ResourceIdentity  []ResourceID `json:"resource_identity"`
	ScopeClass        string       `json:"scope_class"`
	ResourceCount     int          `json:"resource_count"`
	ResourceShapeHash string       `json:"resource_shape_hash"`
	RiskTags          []string     `json:"risk_tags,omitempty"`
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
	Canonicalize(tool, operation string, rawArtifact []byte) (CanonResult, error)
}

// DefaultAdapters returns the built-in adapter chain (k8s, terraform, generic fallback).
func DefaultAdapters() []Adapter {
	return []Adapter{
		&K8sAdapter{},
		&TerraformAdapter{},
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
func Canonicalize(tool, operation string, rawArtifact []byte) CanonResult {
	adapter := SelectAdapter(tool, DefaultAdapters())
	result, err := adapter.Canonicalize(tool, operation, rawArtifact)
	if err != nil {
		result.ParseError = err
	}
	return result
}

// OperationClass maps an operation string to a class.
func OperationClass(tool, operation string) string {
	switch tool {
	case "kubectl", "oc":
		return k8sOperationClass(operation)
	case "terraform":
		return terraformOperationClass(operation)
	default:
		return "unknown"
	}
}

func k8sOperationClass(op string) string {
	switch op {
	case "apply", "create", "patch":
		return "mutate"
	case "delete":
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

// ScopeClass determines the scope class based on resource identity.
func ScopeClass(resources []ResourceID) string {
	if len(resources) == 0 {
		return "unknown"
	}

	namespaces := make(map[string]bool)
	for _, r := range resources {
		if r.Namespace != "" {
			namespaces[r.Namespace] = true
		}
	}

	if len(resources) == 1 {
		return "single"
	}
	if len(namespaces) <= 1 {
		return "namespace"
	}
	return "cluster"
}
