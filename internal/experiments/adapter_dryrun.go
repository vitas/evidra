package experiments

import "context"

type dryRunAgent struct{}

func (a *dryRunAgent) Name() string { return "dry-run" }

func (a *dryRunAgent) RunArtifact(_ context.Context, _ ArtifactAgentRequest) (ArtifactAgentResult, error) {
	return ArtifactAgentResult{
		Output: map[string]any{
			"predicted_risk_level":   "",
			"predicted_risk_details": []string{},
		},
		StdoutLog: "dry-run\n",
	}, nil
}

func (a *dryRunAgent) RunExecution(_ context.Context, _ ExecutionAgentRequest) (ExecutionAgentResult, error) {
	return ExecutionAgentResult{
		Output: map[string]any{
			"prescribe_ok": true,
			"report_ok":    true,
			"exit_code":    0,
			"risk_level":   "",
			"risk_tags":    []string{},
		},
		StdoutLog: "dry-run\n",
	}, nil
}
