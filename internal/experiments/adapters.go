package experiments

import (
	"context"
	"fmt"
	"strings"
)

type ArtifactAgent interface {
	Name() string
	RunArtifact(context.Context, ArtifactAgentRequest) (ArtifactAgentResult, error)
}

type ExecutionAgent interface {
	Name() string
	RunExecution(context.Context, ExecutionAgentRequest) (ExecutionAgentResult, error)
}

func newArtifactAgent(name string) (ArtifactAgent, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dry-run", "dry_run", "dryrun":
		return &dryRunAgent{}, nil
	case "claude":
		return &claudeAgent{}, nil
	case "bifrost":
		return &bifrostAgent{}, nil
	default:
		return nil, fmt.Errorf("%w: artifact agent %q", ErrUnsupportedAgent, name)
	}
}

func newExecutionAgent(name string) (ExecutionAgent, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dry-run", "dry_run", "dryrun":
		return &dryRunAgent{}, nil
	case "mcp-kubectl", "mcp_kubectl", "mcpkubectl":
		return &mcpKubectlAgent{}, nil
	default:
		return nil, fmt.Errorf("%w: execution agent %q", ErrUnsupportedAgent, name)
	}
}
