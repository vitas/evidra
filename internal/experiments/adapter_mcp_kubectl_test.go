package experiments

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPKubectlAgentRunExecutionOK(t *testing.T) {
	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact.yaml")
	if err := os.WriteFile(artifact, []byte("apiVersion: v1\nkind: Pod\n"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	config := filepath.Join(tmp, "mcp-config.json")
	if err := os.WriteFile(config, []byte(`{"mcpServers":{"evidra":{"command":"evidra-mcp","args":[]}}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	writeExecutable(t, filepath.Join(binDir, "npx"), `#!/usr/bin/env bash
set -euo pipefail
tool=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --tool-name)
      tool="$2"; shift 2 ;;
    *) shift ;;
  esac
done
if [[ "$tool" == "prescribe" ]]; then
  cat <<'JSON'
{"structuredContent":{"ok":true,"prescription_id":"rx-123","effective_risk":"critical","risk_inputs":[{"source":"evidra/native","risk_level":"critical","risk_tags":["k8s.privileged_container"]}]}}
JSON
  exit 0
fi
if [[ "$tool" == "report" ]]; then
  cat <<'JSON'
{"structuredContent":{"ok":true,"report_id":"rp-123"}}
JSON
  exit 0
fi
echo "unexpected tool" >&2
exit 2
`)
	writeExecutable(t, filepath.Join(binDir, "kubectl"), "#!/usr/bin/env bash\nset -euo pipefail\nexit 0\n")
	writeExecutable(t, filepath.Join(binDir, "evidra-mcp"), "#!/usr/bin/env bash\nset -euo pipefail\nexit 0\n")

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("EVIDRA_MCP_CONFIG", config)
	t.Setenv("EVIDRA_MCP_INSPECTOR_BIN", filepath.Join(binDir, "npx"))

	agent := &mcpKubectlAgent{}
	result, err := agent.RunExecution(context.Background(), ExecutionAgentRequest{
		Scenario: ExecutionScenario{
			ScenarioID:     "s-1",
			Tool:           "kubectl",
			Operation:      "apply",
			ArtifactPath:   artifact,
			ExecuteCommand: "true",
		},
		ModelID: "claude/sonnet",
		Prompt: PromptInfo{
			ContractVersion: "v1.0.1",
		},
	})
	if err != nil {
		t.Fatalf("RunExecution error: %v", err)
	}
	if readBool(result.Output, "prescribe_ok") != true {
		t.Fatalf("prescribe_ok=false: %v", result.Output)
	}
	if readBool(result.Output, "report_ok") != true {
		t.Fatalf("report_ok=false: %v", result.Output)
	}
	if readString(result.Output, "risk_level") != "critical" {
		t.Fatalf("risk_level: %v", result.Output)
	}
	if len(readStringSlice(result.Output, "risk_tags")) != 1 {
		t.Fatalf("risk_tags: %v", result.Output)
	}
	if !strings.Contains(result.RawStream, `"phase":"prescribe"`) || !strings.Contains(result.RawStream, `"phase":"report"`) {
		t.Fatalf("raw stream missing phases: %s", result.RawStream)
	}
}
