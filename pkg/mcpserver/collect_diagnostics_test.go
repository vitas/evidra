package mcpserver

import (
	"context"
	"strings"
	"testing"
)

func TestCollectDiagnostics_KubernetesWorkload(t *testing.T) {
	t.Parallel()

	outputs := map[string]RunCommandOutput{
		"kubectl get pods -n demo": {
			OK: true,
			Output: `pods: 2 total, 1 running, 1 crashloopbackoff
  web-abc: CrashLoopBackOff (0/1 ready)
  web-def: Running (1/1 ready)`,
		},
		"kubectl describe deployment/web -n demo": {
			OK: true,
			Output: `Name: web
Namespace: demo
events (last 5):
  Warning FailedPull 2m kubelet Failed to pull image "nginx:99.99"`,
		},
		"kubectl get events -n demo --sort-by=.lastTimestamp": {
			OK: true,
			Output: `LAST SEEN   TYPE      REASON      OBJECT      MESSAGE
2m          Warning   FailedPull  pod/web-abc Failed to pull image "nginx:99.99"`,
		},
		"kubectl logs web-abc -n demo --tail=50": {
			OK: true,
			Output: `logs web-abc (last 3 lines, 3 total):
panic: image pull failed
back-off pulling image
check image tag`,
		},
	}

	handler := &collectDiagnosticsHandler{
		run: func(_ context.Context, command string) RunCommandOutput {
			out, ok := outputs[command]
			if !ok {
				return RunCommandOutput{OK: false, Error: "unexpected command: " + command}
			}
			return out
		},
	}

	_, out, err := handler.Handle(context.Background(), nil, CollectDiagnosticsInput{
		Namespace: "demo",
		Workload:  "deployment/web",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !out.OK {
		t.Fatalf("collect_diagnostics ok=false: %+v", out)
	}
	if len(out.Commands) != 4 {
		t.Fatalf("commands len = %d, want 4 (%v)", len(out.Commands), out.Commands)
	}
	if got := out.Commands[3]; got != "kubectl logs web-abc -n demo --tail=50" {
		t.Fatalf("logs command = %q, want kubectl logs web-abc -n demo --tail=50", got)
	}
	if len(out.Findings) < 3 {
		t.Fatalf("findings len = %d, want at least 3", len(out.Findings))
	}
	if !strings.Contains(out.Summary, "CrashLoopBackOff") {
		t.Fatalf("summary missing pod issue: %s", out.Summary)
	}
	if !strings.Contains(out.Summary, "FailedPull") {
		t.Fatalf("summary missing describe/event issue: %s", out.Summary)
	}
	if !strings.Contains(out.Summary, "next checks") {
		t.Fatalf("summary missing next checks: %s", out.Summary)
	}
}
