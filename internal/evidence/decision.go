package evidence

// Decision represents the outcome of a policy or risk evaluation.
// This type replaces the OPA-specific pkg/policy.Decision from the source project.
type Decision struct {
	Allow          bool     `json:"allow"`
	RiskLevel      string   `json:"risk_level"`
	Reason         string   `json:"reason"`
	PolicyRef      string   `json:"policy_ref,omitempty"`
	BundleRevision string   `json:"bundle_revision,omitempty"`
	ProfileName    string   `json:"profile_name,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
	Hints          []string `json:"hints,omitempty"`
	Hits           []string `json:"hits,omitempty"`
}
