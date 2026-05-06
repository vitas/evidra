package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestMarkdownAPIReference_CoversLiveExternalIngestSurface(t *testing.T) {
	t.Parallel()

	doc := loadMarkdownAPIReference(t)

	requiredSnippets := []string{
		"### `POST /v1/evidence/ingest/prescribe`",
		"### `POST /v1/evidence/ingest/report`",
		"### `HEAD /auth/check`",
		"`{\"status\":\"ok\"}`",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(doc, snippet) {
			t.Fatalf("api reference missing %q", snippet)
		}
	}
	if strings.Contains(doc, "proxy|smart") {
		t.Fatal("api reference still uses proxy|smart wording for top-level bench filters")
	}
}

func TestOpenAPIReference_StaysAlignedWithLiveRoutes(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t)

	assertPathOperations(t, spec, "/auth/check", []string{"get", "head"})
	assertQueryParameterDefaults(t, spec, "/v1/evidence/entries", "limit", "100", "1000")
	assertOperationHasQueryParameter(t, spec, "/v1/evidence/entries", "get", "period")
	assertRequestBodyDoesNotRequireField(t, spec, "/v1/keys", "post", "label")
}

func loadMarkdownAPIReference(t *testing.T) string {
	t.Helper()

	path := filepath.Join("..", "..", "docs", "api-reference.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read markdown api reference: %v", err)
	}
	return string(raw)
}

func assertPathOperations(t *testing.T, spec *yaml.Node, path string, want []string) {
	t.Helper()

	pathNode := findMappingValue(t, findMappingValue(t, spec.Content[0], "paths"), path)
	for _, method := range want {
		if findMappingValueOptional(pathNode, method) == nil {
			t.Fatalf("path %s missing %s operation", path, method)
		}
	}
}

func assertOperationHasQueryParameter(t *testing.T, spec *yaml.Node, path, method, paramName string) {
	t.Helper()

	params := operationParameters(t, spec, path, method)
	for _, param := range params.Content {
		name := findMappingValueOptional(param, "name")
		in := findMappingValueOptional(param, "in")
		if name != nil && in != nil && name.Value == paramName && in.Value == "query" {
			return
		}
	}
	t.Fatalf("%s %s missing query parameter %s", strings.ToUpper(method), path, paramName)
}

func assertQueryParameterDefaults(t *testing.T, spec *yaml.Node, path, paramName, wantDefault, wantMaximum string) {
	t.Helper()

	params := operationParameters(t, spec, path, "get")
	for _, param := range params.Content {
		name := findMappingValueOptional(param, "name")
		in := findMappingValueOptional(param, "in")
		if name == nil || in == nil || name.Value != paramName || in.Value != "query" {
			continue
		}

		schema := findMappingValue(t, param, "schema")
		def := findMappingValueOptional(schema, "default")
		if def == nil || def.Value != wantDefault {
			t.Fatalf("%s query param %s default = %v, want %s", path, paramName, nodeValue(def), wantDefault)
		}
		max := findMappingValueOptional(schema, "maximum")
		if max == nil || max.Value != wantMaximum {
			t.Fatalf("%s query param %s maximum = %v, want %s", path, paramName, nodeValue(max), wantMaximum)
		}
		return
	}

	t.Fatalf("%s missing query param %s", path, paramName)
}

func assertRequestBodyDoesNotRequireField(t *testing.T, spec *yaml.Node, path, method, field string) {
	t.Helper()

	op := findMappingValue(t, findMappingValue(t, findMappingValue(t, spec.Content[0], "paths"), path), method)
	requestBody := findMappingValue(t, op, "requestBody")
	content := findMappingValue(t, requestBody, "content")
	jsonBody := findMappingValue(t, content, "application/json")
	schema := findMappingValue(t, jsonBody, "schema")
	required := findMappingValueOptional(schema, "required")
	if required == nil {
		return
	}
	for _, item := range required.Content {
		if item.Value == field {
			t.Fatalf("%s %s should not require %s", strings.ToUpper(method), path, field)
		}
	}
}

func operationParameters(t *testing.T, spec *yaml.Node, path, method string) *yaml.Node {
	t.Helper()

	op := findMappingValue(t, findMappingValue(t, findMappingValue(t, spec.Content[0], "paths"), path), method)
	return findMappingValue(t, op, "parameters")
}

func nodeValue(n *yaml.Node) string {
	if n == nil {
		return "<nil>"
	}
	return n.Value
}
