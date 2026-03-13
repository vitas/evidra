package promptfactory

import (
	"reflect"
	"strings"
	"testing"
)

func TestRenderDeterministic(t *testing.T) {
	t.Parallel()

	bundle, err := LoadBundle(repoRoot(t), "v1.0.1")
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}

	a, err := RenderFiles(repoRoot(t), bundle)
	if err != nil {
		t.Fatalf("RenderFiles a: %v", err)
	}
	b, err := RenderFiles(repoRoot(t), bundle)
	if err != nil {
		t.Fatalf("RenderFiles b: %v", err)
	}

	if !reflect.DeepEqual(a, b) {
		t.Fatal("non-deterministic render")
	}
}

func TestRenderFiles_ExpectedTargets(t *testing.T) {
	t.Parallel()

	bundle, err := LoadBundle(repoRoot(t), "v1.0.1")
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	files, err := RenderFiles(repoRoot(t), bundle)
	if err != nil {
		t.Fatalf("RenderFiles: %v", err)
	}

	wantIDs := map[string]bool{
		"mcp.initialize":         true,
		"mcp.prescribe":          true,
		"mcp.report":             true,
		"mcp.get_event":          true,
		"mcp.agent_contract":     true,
		"runtime.system":         true,
		"runtime.agent_contract": true,
		"skill.skill":            true,
	}
	if len(files) != len(wantIDs) {
		t.Fatalf("rendered files = %d, want %d", len(files), len(wantIDs))
	}
	for _, f := range files {
		if !wantIDs[f.ID] {
			t.Fatalf("unexpected file id: %s", f.ID)
		}
		if !strings.Contains(f.Content, "contract: v1.0.1") {
			t.Fatalf("%s missing contract header", f.ID)
		}
	}
}

func TestRenderFiles_SkillContainsFrontmatter(t *testing.T) {
	t.Parallel()

	bundle, err := LoadBundle(repoRoot(t), "v1.0.1")
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	files, err := RenderFiles(repoRoot(t), bundle)
	if err != nil {
		t.Fatalf("RenderFiles: %v", err)
	}

	var skill *RenderedFile
	for i := range files {
		if files[i].ID == "skill.skill" {
			skill = &files[i]
			break
		}
	}
	if skill == nil {
		t.Fatal("skill.skill target not found")
	}

	if !strings.HasPrefix(skill.Content, "---\nname: evidra\n") {
		t.Fatal("skill missing YAML frontmatter")
	}
	if !strings.Contains(skill.Content, "prescribe") {
		t.Fatal("skill missing prescribe protocol content")
	}
	if !strings.Contains(skill.Content, "report") {
		t.Fatal("skill missing report protocol content")
	}
	if len(bundle.Contract.Invariants) == 0 {
		t.Fatal("bundle has no invariants to check")
	}
	for _, inv := range bundle.Contract.Invariants {
		if !strings.Contains(skill.Content, inv) {
			t.Fatalf("skill missing invariant: %s", inv)
		}
	}
}
