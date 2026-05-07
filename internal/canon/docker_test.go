package canon_test

import (
	"testing"

	"samebits.com/evidra/internal/canon"
)

func TestDockerAdapterCanHandle(t *testing.T) {
	a := &canon.DockerAdapter{}
	cases := []struct {
		tool string
		want bool
	}{
		// Docker-compatible CLIs — command string syntax.
		{"docker", true},
		{"nerdctl", true},
		{"podman", true},
		{"lima", true},
		// Compose tools — YAML artifact syntax.
		{"docker-compose", true},
		{"compose", true},
		// Different CLI syntax — must fall through to GenericAdapter.
		{"ctr", false},
		{"crictl", false},
		// Unrelated tools.
		{"kubectl", false},
		{"terraform", false},
		{"helm", false},
		{"", false},
	}
	for _, c := range cases {
		got := a.CanHandle(c.tool)
		if got != c.want {
			t.Errorf("CanHandle(%q) = %v, want %v", c.tool, got, c.want)
		}
	}
}

func TestDockerAdapterMassRemove(t *testing.T) {
	a := &canon.DockerAdapter{}
	artifact := "nerdctl rm -f demo-worker-1 demo-worker-2 demo-worker-3 demo-worker-4 demo-worker-5 demo-worker-6"
	result, err := a.Canonicalize("nerdctl", "rm", "development", []byte(artifact))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CanonicalAction.OperationClass != "destroy" {
		t.Errorf("operation_class = %q, want destroy", result.CanonicalAction.OperationClass)
	}
	if result.CanonicalAction.ResourceCount != 6 {
		t.Errorf("resource_count = %d, want 6", result.CanonicalAction.ResourceCount)
	}
	if result.CanonVersion != "docker/v1" {
		t.Errorf("canon_version = %q, want docker/v1", result.CanonVersion)
	}
}

func TestDockerAdapterMassRemove_BlastRadiusTrigger(t *testing.T) {
	// 8 containers: resource_count > BlastRadiusThreshold (5) so blast_radius can fire.
	a := &canon.DockerAdapter{}
	artifact := "nerdctl rm demo-1 demo-2 demo-3 demo-4 demo-5 demo-6 demo-7 demo-8"
	result, err := a.Canonicalize("nerdctl", "rm", "", []byte(artifact))
	if err != nil {
		t.Fatal(err)
	}
	if result.CanonicalAction.ResourceCount <= 5 {
		t.Errorf("resource_count = %d; blast_radius threshold is 5, need > 5", result.CanonicalAction.ResourceCount)
	}
}

func TestDockerAdapterRun_ExtractsName(t *testing.T) {
	a := &canon.DockerAdapter{}
	artifact := "nerdctl run -d --name web-01 nginx:alpine"
	result, err := a.Canonicalize("nerdctl", "run", "staging", []byte(artifact))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CanonicalAction.OperationClass != "mutate" {
		t.Errorf("operation_class = %q, want mutate", result.CanonicalAction.OperationClass)
	}
	if result.CanonicalAction.ResourceCount != 1 {
		t.Errorf("resource_count = %d, want 1", result.CanonicalAction.ResourceCount)
	}
	if len(result.CanonicalAction.ResourceIdentity) == 0 || result.CanonicalAction.ResourceIdentity[0].Name != "web-01" {
		t.Errorf("resource name not extracted: got %v", result.CanonicalAction.ResourceIdentity)
	}
}

func TestDockerAdapterRun_NameEquals(t *testing.T) {
	a := &canon.DockerAdapter{}
	artifact := "docker run -d --name=cache-01 redis:7-alpine"
	result, err := a.Canonicalize("docker", "run", "", []byte(artifact))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.CanonicalAction.ResourceIdentity) == 0 || result.CanonicalAction.ResourceIdentity[0].Name != "cache-01" {
		t.Errorf("resource name not extracted: got %v", result.CanonicalAction.ResourceIdentity)
	}
}

func TestDockerAdapterStop(t *testing.T) {
	a := &canon.DockerAdapter{}
	artifact := "nerdctl stop web-01 cache-01 api-01"
	result, err := a.Canonicalize("nerdctl", "stop", "", []byte(artifact))
	if err != nil {
		t.Fatal(err)
	}
	if result.CanonicalAction.OperationClass != "destroy" {
		t.Errorf("operation_class = %q, want destroy", result.CanonicalAction.OperationClass)
	}
	if result.CanonicalAction.ResourceCount != 3 {
		t.Errorf("resource_count = %d, want 3", result.CanonicalAction.ResourceCount)
	}
}

func TestDockerAdapterOperationFallback(t *testing.T) {
	a := &canon.DockerAdapter{}
	// Artifact that does not start with a known subcommand.
	artifact := "some-custom-script --args"
	result, err := a.Canonicalize("nerdctl", "rm", "", []byte(artifact))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls back to operation-based classification.
	if result.CanonicalAction.OperationClass != "destroy" {
		t.Errorf("operation_class = %q, want destroy (fallback from operation)", result.CanonicalAction.OperationClass)
	}
	// Falls back to artifact-digest-as-resource, count=1.
	if result.CanonicalAction.ResourceCount != 1 {
		t.Errorf("resource_count = %d, want 1 (fallback)", result.CanonicalAction.ResourceCount)
	}
}

func TestDockerAdapterEmptyArtifact(t *testing.T) {
	a := &canon.DockerAdapter{}
	result, err := a.Canonicalize("nerdctl", "rm", "", []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	// No panic, produces valid result.
	if result.CanonVersion != "docker/v1" {
		t.Errorf("canon_version = %q, want docker/v1", result.CanonVersion)
	}
}

func TestDockerAdapterShapeHashDeterministic(t *testing.T) {
	// Same containers in different order must produce same shape hash.
	a := &canon.DockerAdapter{}
	art1 := "nerdctl rm alpha beta gamma"
	art2 := "nerdctl rm gamma alpha beta"
	r1, _ := a.Canonicalize("nerdctl", "rm", "", []byte(art1))
	r2, _ := a.Canonicalize("nerdctl", "rm", "", []byte(art2))
	if r1.CanonicalAction.ResourceShapeHash != r2.CanonicalAction.ResourceShapeHash {
		t.Errorf("shape hash differs for same containers in different order: %q vs %q",
			r1.CanonicalAction.ResourceShapeHash, r2.CanonicalAction.ResourceShapeHash)
	}
}

func TestDockerAdapterInDefaultChain(t *testing.T) {
	adapters := canon.DefaultAdapters()

	for _, tool := range []string{"docker", "nerdctl", "podman", "lima"} {
		selected := canon.SelectAdapter(tool, adapters)
		if selected.Name() != "docker/v1" {
			t.Errorf("SelectAdapter(%q) = %q, want docker/v1", tool, selected.Name())
		}
	}

	// kubectl must still go to k8s/v1.
	selected := canon.SelectAdapter("kubectl", adapters)
	if selected.Name() != "k8s/v1" {
		t.Errorf("SelectAdapter(kubectl) = %q, want k8s/v1", selected.Name())
	}

	// ctr and crictl must fall through to generic/v1.
	for _, tool := range []string{"ctr", "crictl"} {
		selected = canon.SelectAdapter(tool, adapters)
		if selected.Name() != "generic/v1" {
			t.Errorf("SelectAdapter(%q) = %q, want generic/v1", tool, selected.Name())
		}
	}
}

func TestDockerAdapterComposeDown(t *testing.T) {
	a := &canon.DockerAdapter{}
	artifact := `
services:
  web:
    image: nginx:alpine
  api:
    image: python:3.12-alpine
  db:
    image: postgres:15-alpine
  cache:
    image: redis:7-alpine
  worker:
    image: python:3.12-alpine
  scheduler:
    image: python:3.12-alpine
`
	result, err := a.Canonicalize("docker-compose", "down", "staging", []byte(artifact))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CanonicalAction.OperationClass != "destroy" {
		t.Errorf("operation_class = %q, want destroy", result.CanonicalAction.OperationClass)
	}
	if result.CanonicalAction.ResourceCount != 6 {
		t.Errorf("resource_count = %d, want 6 (one per service)", result.CanonicalAction.ResourceCount)
	}
	if result.CanonVersion != "docker/v1" {
		t.Errorf("canon_version = %q, want docker/v1", result.CanonVersion)
	}
}

func TestDockerAdapterComposeDown_BlastRadiusTrigger(t *testing.T) {
	// 6 services torn down → resource_count > threshold 5 → blast_radius can fire.
	a := &canon.DockerAdapter{}
	artifact := `
services:
  svc1:
    image: nginx:alpine
  svc2:
    image: nginx:alpine
  svc3:
    image: nginx:alpine
  svc4:
    image: nginx:alpine
  svc5:
    image: nginx:alpine
  svc6:
    image: nginx:alpine
`
	result, err := a.Canonicalize("docker-compose", "down", "", []byte(artifact))
	if err != nil {
		t.Fatal(err)
	}
	if result.CanonicalAction.ResourceCount <= 5 {
		t.Errorf("resource_count = %d; blast_radius threshold is 5, need > 5", result.CanonicalAction.ResourceCount)
	}
}

func TestDockerAdapterComposeUp(t *testing.T) {
	a := &canon.DockerAdapter{}
	artifact := `
services:
  web:
    image: nginx:alpine
  db:
    image: postgres:15
`
	result, err := a.Canonicalize("docker-compose", "up", "", []byte(artifact))
	if err != nil {
		t.Fatal(err)
	}
	if result.CanonicalAction.OperationClass != "mutate" {
		t.Errorf("operation_class = %q, want mutate", result.CanonicalAction.OperationClass)
	}
	if result.CanonicalAction.ResourceCount != 2 {
		t.Errorf("resource_count = %d, want 2", result.CanonicalAction.ResourceCount)
	}
}

func TestDockerAdapterComposeServiceNames(t *testing.T) {
	a := &canon.DockerAdapter{}
	artifact := `
services:
  alpha:
    image: nginx:alpine
  beta:
    image: redis:7
`
	result, err := a.Canonicalize("compose", "down", "", []byte(artifact))
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	for _, r := range result.CanonicalAction.ResourceIdentity {
		names[r.Name] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("service names not extracted: got %v", result.CanonicalAction.ResourceIdentity)
	}
}

func TestDockerAdapterComposeShapeHashDeterministic(t *testing.T) {
	// Service order in YAML must not affect shape hash.
	a := &canon.DockerAdapter{}
	art1 := "services:\n  alpha:\n    image: nginx\n  beta:\n    image: redis\n"
	art2 := "services:\n  beta:\n    image: redis\n  alpha:\n    image: nginx\n"
	r1, _ := a.Canonicalize("docker-compose", "down", "", []byte(art1))
	r2, _ := a.Canonicalize("docker-compose", "down", "", []byte(art2))
	if r1.CanonicalAction.ResourceShapeHash != r2.CanonicalAction.ResourceShapeHash {
		t.Errorf("shape hash differs for same services in different YAML order: %q vs %q",
			r1.CanonicalAction.ResourceShapeHash, r2.CanonicalAction.ResourceShapeHash)
	}
}

func TestDockerAdapterComposeInvalidYAML(t *testing.T) {
	// Invalid YAML falls back gracefully — no panic, count=1.
	a := &canon.DockerAdapter{}
	result, err := a.Canonicalize("docker-compose", "down", "", []byte("not: valid: yaml: :::"))
	if err != nil {
		t.Fatal(err)
	}
	if result.CanonVersion != "docker/v1" {
		t.Errorf("canon_version = %q, want docker/v1", result.CanonVersion)
	}
}

func TestDockerAdapterCanHandle_ComposeTools(t *testing.T) {
	a := &canon.DockerAdapter{}
	for _, tool := range []string{"docker-compose", "compose"} {
		if !a.CanHandle(tool) {
			t.Errorf("CanHandle(%q) = false, want true", tool)
		}
	}
}

func TestDockerAdapterPrefixless(t *testing.T) {
	// Artifact without tool prefix: "rm foo bar baz".
	a := &canon.DockerAdapter{}
	artifact := "rm foo bar baz"
	result, err := a.Canonicalize("nerdctl", "rm", "", []byte(artifact))
	if err != nil {
		t.Fatal(err)
	}
	if result.CanonicalAction.OperationClass != "destroy" {
		t.Errorf("operation_class = %q, want destroy", result.CanonicalAction.OperationClass)
	}
	if result.CanonicalAction.ResourceCount != 3 {
		t.Errorf("resource_count = %d, want 3", result.CanonicalAction.ResourceCount)
	}
}

func TestDockerAdapterCompatibleToolPrefixes(t *testing.T) {
	t.Parallel()

	a := &canon.DockerAdapter{}
	cases := []struct {
		name  string
		tool  string
		cmd   string
		count int
	}{
		{
			name:  "podman",
			tool:  "podman",
			cmd:   "podman rm api cache worker",
			count: 3,
		},
		{
			name:  "lima",
			tool:  "lima",
			cmd:   "lima rm demo-1 demo-2 demo-3 demo-4 demo-5 demo-6",
			count: 6,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := a.Canonicalize(tc.tool, "rm", "", []byte(tc.cmd))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.CanonicalAction.OperationClass != "destroy" {
				t.Fatalf("operation_class = %q, want destroy", result.CanonicalAction.OperationClass)
			}
			if result.CanonicalAction.ResourceCount != tc.count {
				t.Fatalf("resource_count = %d, want %d", result.CanonicalAction.ResourceCount, tc.count)
			}
		})
	}
}
