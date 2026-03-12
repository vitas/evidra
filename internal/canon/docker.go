package canon

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// DockerAdapter handles docker and nerdctl command artifacts.
// It parses command strings to extract operation class and resource count,
// enabling blast_radius detection for containerd-based operations without
// requiring the caller to supply canonical_action overrides.
type DockerAdapter struct{}

// dockerCompatibleTools lists CLIs handled by DockerAdapter, split into two groups:
//
// Command-string tools (docker, nerdctl, podman, lima) share Docker CLI syntax.
// The raw_artifact is expected to be a command string (e.g. "nerdctl rm foo bar").
//
// Compose tools (docker-compose, compose) use YAML artifacts (compose.yml content).
// The raw_artifact is parsed as YAML to extract service names and resource count.
//
// Tools with different CLI syntax (ctr, crictl) use GenericAdapter + canonical_action override.
var dockerCompatibleTools = map[string]bool{
	"docker":         true,
	"nerdctl":        true,
	"podman":         true,
	"lima":           true,
	"docker-compose": true,
	"compose":        true,
}

// composeTool returns true for tools whose artifact is a compose YAML file.
func composeTool(tool string) bool {
	return tool == "docker-compose" || tool == "compose"
}

func (a *DockerAdapter) Name() string { return "docker/v1" }
func (a *DockerAdapter) CanHandle(tool string) bool {
	return dockerCompatibleTools[tool]
}
func (a *DockerAdapter) Canonicalize(tool, operation, environment string, rawArtifact []byte) (CanonResult, error) {
	return canonicalizeDocker(tool, operation, environment, rawArtifact)
}

func canonicalizeDocker(tool, operation, environment string, rawArtifact []byte) (CanonResult, error) {
	artifactDigest := SHA256Hex(rawArtifact)

	var opClass string
	var resources []ResourceID

	if composeTool(tool) {
		opClass = composeOperationClass(operation)
		resources = parseComposeServices(rawArtifact, tool)
	} else {
		cmd := strings.TrimSpace(string(rawArtifact))
		opClass, resources = parseDockerCommand(tool, operation, cmd)
	}

	if len(resources) == 0 {
		// Fallback: use artifact digest as single anonymous resource.
		resources = []ResourceID{{Name: artifactDigest, Type: tool}}
	}

	// Sort for deterministic shape hash regardless of command argument order.
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Name < resources[j].Name
	})

	scopeClass := ResolveScopeClass(environment, resources)

	shapeData, err := json.Marshal(resources)
	if err != nil {
		return CanonResult{}, fmt.Errorf("marshal resources: %w", err)
	}
	shapeHash := SHA256Hex(shapeData)

	action := CanonicalAction{
		Tool:              tool,
		Operation:         operation,
		OperationClass:    opClass,
		ResourceIdentity:  resources,
		ScopeClass:        scopeClass,
		ResourceCount:     len(resources),
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
		CanonVersion:    "docker/v1",
		RawAction:       actionJSON,
	}, nil
}

// dockerDestroySubcmds are subcommands that remove or stop resources.
var dockerDestroySubcmds = map[string]bool{
	"rm": true, "remove": true,
	"stop": true, "kill": true,
	"rmi": true, "down": true,
}

// dockerMutateSubcmds are subcommands that create or modify resources.
var dockerMutateSubcmds = map[string]bool{
	"run": true, "create": true, "start": true,
	"restart": true, "update": true, "up": true,
	"pull": true, "build": true, "push": true,
}

// parseDockerCommand extracts operation class and resource identities from a
// docker/nerdctl command string. The command may include the tool name prefix
// (e.g., "nerdctl rm foo bar") or start directly with the subcommand ("rm foo bar").
//
// For destroy subcommands (rm, stop, kill, rmi, down), all non-flag tokens after
// the subcommand are treated as resource names, enabling resource_count > 1 and
// thus blast_radius detection.
//
// For run/create, the --name flag value is extracted as the resource identity.
func parseDockerCommand(tool, operation, cmd string) (opClass string, resources []ResourceID) {
	tokens := strings.Fields(cmd)
	if len(tokens) == 0 {
		return dockerOperationClass(operation), nil
	}

	idx := 0
	// Skip leading tool name if present.
	if tokens[idx] == tool || tokens[idx] == "docker" || tokens[idx] == "nerdctl" {
		idx++
	}
	if idx >= len(tokens) {
		return dockerOperationClass(operation), nil
	}

	subcmd := tokens[idx]
	idx++

	if dockerDestroySubcmds[subcmd] {
		// Collect all non-flag tokens as container/image names.
		for ; idx < len(tokens); idx++ {
			tok := tokens[idx]
			if strings.HasPrefix(tok, "-") {
				continue
			}
			resources = append(resources, ResourceID{Name: tok, Type: tool})
		}
		return "destroy", resources
	}

	if dockerMutateSubcmds[subcmd] {
		if subcmd == "run" || subcmd == "create" {
			// Extract --name value as the resource identity.
			for i := idx; i < len(tokens); i++ {
				if (tokens[i] == "--name" || tokens[i] == "-n") && i+1 < len(tokens) {
					resources = append(resources, ResourceID{Name: tokens[i+1], Type: tool})
					break
				}
				if strings.HasPrefix(tokens[i], "--name=") {
					name := strings.TrimPrefix(tokens[i], "--name=")
					resources = append(resources, ResourceID{Name: name, Type: tool})
					break
				}
			}
		}
		return "mutate", resources
	}

	return dockerOperationClass(operation), nil
}

// dockerOperationClass maps a docker/nerdctl operation string to an operation class.
// Used as fallback when subcommand parsing yields no match.
func dockerOperationClass(operation string) string {
	switch operation {
	case "rm", "remove", "stop", "kill", "rmi", "down", "delete":
		return "destroy"
	case "run", "create", "start", "restart", "update", "up", "pull", "build", "push":
		return "mutate"
	default:
		return "unknown"
	}
}

// composeOperationClass maps a docker-compose operation to an operation class.
// compose down / rm / stop → destroy (services are removed or halted).
// compose up / start / restart / pull / build → mutate (services are created or updated).
func composeOperationClass(operation string) string {
	switch operation {
	case "down", "rm", "remove", "stop", "kill":
		return "destroy"
	case "up", "start", "restart", "pull", "build", "push", "run", "create":
		return "mutate"
	default:
		return "unknown"
	}
}

// parseComposeServices parses a compose YAML artifact and returns one ResourceID
// per declared service. Service names become the resource identities, enabling
// accurate resource_count for blast_radius detection on compose down operations.
//
// Returns nil if the artifact is not valid compose YAML or has no services section.
func parseComposeServices(raw []byte, tool string) []ResourceID {
	var cf struct {
		Services map[string]interface{} `yaml:"services"`
	}
	if err := yaml.Unmarshal(raw, &cf); err != nil || len(cf.Services) == 0 {
		return nil
	}
	resources := make([]ResourceID, 0, len(cf.Services))
	for name := range cf.Services {
		resources = append(resources, ResourceID{Name: name, Type: "compose-service"})
	}
	return resources
}
