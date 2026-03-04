package canon

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

// TerraformAdapter handles terraform plan JSON artifacts.
type TerraformAdapter struct{}

func (a *TerraformAdapter) Name() string               { return "tf/v1" }
func (a *TerraformAdapter) CanHandle(tool string) bool { return tool == "terraform" }
func (a *TerraformAdapter) Canonicalize(tool, operation string, rawArtifact []byte) (CanonResult, error) {
	r := canonicalizeTerraform(tool, operation, rawArtifact)
	return r, r.ParseError
}

func canonicalizeTerraform(tool, operation string, rawArtifact []byte) CanonResult {
	artifactDigest := sha256Hex(rawArtifact)

	var plan tfjson.Plan
	if err := json.Unmarshal(rawArtifact, &plan); err != nil {
		return CanonResult{
			ArtifactDigest: artifactDigest,
			CanonVersion:   "terraform/v1",
			ParseError:     fmt.Errorf("canon.terraform: parse plan JSON: %w", err),
		}
	}

	if len(plan.ResourceChanges) == 0 {
		// Treat as a no-op plan
		action := CanonicalAction{
			Tool:              tool,
			Operation:         operation,
			OperationClass:    terraformOperationClass(operation),
			ResourceIdentity:  nil,
			ScopeClass:        "unknown",
			ResourceCount:     0,
			ResourceShapeHash: sha256Hex([]byte("empty")),
		}
		actionJSON, _ := json.Marshal(action)
		return CanonResult{
			ArtifactDigest:  artifactDigest,
			IntentDigest:    sha256Hex(actionJSON),
			CanonicalAction: action,
			CanonVersion:    "terraform/v1",
			RawAction:       actionJSON,
		}
	}

	// Sort resource changes by type + name
	changes := make([]*tfjson.ResourceChange, len(plan.ResourceChanges))
	copy(changes, plan.ResourceChanges)
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Type != changes[j].Type {
			return changes[i].Type < changes[j].Type
		}
		return changes[i].Name < changes[j].Name
	})

	// Extract identities (type + name + actions, NOT address)
	identities := make([]ResourceID, 0, len(changes))
	for _, rc := range changes {
		actions := formatTerraformActions(rc.Change)
		identities = append(identities, ResourceID{
			Type:    rc.Type,
			Name:    rc.Name,
			Actions: actions,
		})
	}

	// Compute shape hash from sorted type+name+actions
	shapeHash := computeTerraformShapeHash(changes)

	opClass := terraformOperationClass(operation)
	scopeClass := "namespace" // Terraform plans are inherently multi-resource
	if len(identities) == 1 {
		scopeClass = "single"
	}

	action := CanonicalAction{
		Tool:              tool,
		Operation:         operation,
		OperationClass:    opClass,
		ResourceIdentity:  identities,
		ScopeClass:        scopeClass,
		ResourceCount:     len(changes),
		ResourceShapeHash: shapeHash,
	}

	actionJSON, _ := json.Marshal(action)
	intentDigest := sha256Hex(actionJSON)

	return CanonResult{
		ArtifactDigest:  artifactDigest,
		IntentDigest:    intentDigest,
		CanonicalAction: action,
		CanonVersion:    "terraform/v1",
		RawAction:       actionJSON,
	}
}

func formatTerraformActions(change *tfjson.Change) string {
	if change == nil {
		return "unknown"
	}
	actions := make([]string, len(change.Actions))
	for i, a := range change.Actions {
		actions[i] = string(a)
	}
	sort.Strings(actions)
	return strings.Join(actions, ",")
}

func computeTerraformShapeHash(changes []*tfjson.ResourceChange) string {
	type shapeEntry struct {
		Type    string   `json:"type"`
		Name    string   `json:"name"`
		Actions []string `json:"actions"`
	}

	entries := make([]shapeEntry, 0, len(changes))
	for _, rc := range changes {
		var actions []string
		if rc.Change != nil {
			for _, a := range rc.Change.Actions {
				actions = append(actions, string(a))
			}
		}
		sort.Strings(actions)
		entries = append(entries, shapeEntry{
			Type:    rc.Type,
			Name:    rc.Name,
			Actions: actions,
		})
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return ""
	}

	return sha256Hex(data)
}
