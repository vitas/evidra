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

func (a *TerraformAdapter) Name() string               { return "terraform/v1" }
func (a *TerraformAdapter) CanHandle(tool string) bool { return tool == "terraform" }
func (a *TerraformAdapter) Canonicalize(tool, operation, environment string, rawArtifact []byte) (CanonResult, error) {
	r, err := canonicalizeTerraform(tool, operation, environment, rawArtifact)
	if err != nil {
		return r, err
	}
	return r, r.ParseError
}

func canonicalizeTerraform(tool, operation, environment string, rawArtifact []byte) (CanonResult, error) {
	artifactDigest := SHA256Hex(rawArtifact)

	var plan tfjson.Plan
	if err := json.Unmarshal(rawArtifact, &plan); err != nil {
		return CanonResult{
			ArtifactDigest: artifactDigest,
			CanonVersion:   "terraform/v1",
			ParseError:     fmt.Errorf("canon.terraform: parse plan JSON: %w", err),
		}, nil
	}

	if len(plan.ResourceChanges) == 0 {
		// Treat as a no-op plan
		action := CanonicalAction{
			Tool:              tool,
			Operation:         operation,
			OperationClass:    terraformOperationClass(operation),
			ResourceIdentity:  nil,
			ScopeClass:        ResolveScopeClass(environment, nil),
			ResourceCount:     0,
			ResourceShapeHash: SHA256Hex([]byte("empty")),
		}
		actionJSON, err := json.Marshal(action)
		if err != nil {
			return CanonResult{}, fmt.Errorf("marshal canonical action: %w", err)
		}
		return CanonResult{
			ArtifactDigest:  artifactDigest,
			IntentDigest:    ComputeIntentDigest(action),
			CanonicalAction: action,
			CanonVersion:    "terraform/v1",
			RawAction:       actionJSON,
		}, nil
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
	scopeClass := ResolveScopeClass(environment, identities)

	action := CanonicalAction{
		Tool:              tool,
		Operation:         operation,
		OperationClass:    opClass,
		ResourceIdentity:  identities,
		ScopeClass:        scopeClass,
		ResourceCount:     len(changes),
		ResourceShapeHash: shapeHash,
	}

	actionJSON, err := json.Marshal(action)
	if err != nil {
		return CanonResult{}, fmt.Errorf("marshal canonical action: %w", err)
	}
	intentDigest := ComputeIntentDigest(action)

	return CanonResult{
		ArtifactDigest:  artifactDigest,
		IntentDigest:    intentDigest,
		CanonicalAction: action,
		CanonVersion:    "terraform/v1",
		RawAction:       actionJSON,
	}, nil
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

	return SHA256Hex(data)
}
