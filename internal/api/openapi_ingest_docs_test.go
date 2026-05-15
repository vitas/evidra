package api

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestOpenAPIIngestRoutesDocumentContracts(t *testing.T) {
	t.Parallel()

	spec := loadOpenAPISpec(t)

	assertPathResponses(t, spec, "/v1/evidence/ingest/prescribe", []string{"200", "202", "400", "401", "500", "503"})
	assertPathResponses(t, spec, "/v1/evidence/ingest/report", []string{"200", "202", "400", "401", "404", "500", "503"})

	assertSchemaOneOfRefs(t, spec, "IngestPrescribeRequest", []string{"IngestPrescribeTypedRequest", "IngestPrescribeOverrideRequest"})
	assertSchemaOneOfRefs(t, spec, "IngestReportRequest", []string{"IngestReportNonDeclinedRequest", "IngestReportDeclinedRequest", "IngestReportOverrideRequest"})
	assertSchemaOneOfRefs(t, spec, "IngestReportOverrideRequest", []string{"IngestReportNonDeclinedOverrideRequest", "IngestReportDeclinedOverrideRequest"})
	assertSchemaExists(t, spec, "IngestPrescribeTypedRequest")
	assertSchemaExists(t, spec, "IngestPrescribeOverrideRequest")
	assertSchemaExists(t, spec, "IngestReportNonDeclinedRequest")
	assertSchemaExists(t, spec, "IngestReportDeclinedRequest")
	assertSchemaExists(t, spec, "IngestReportOverrideRequest")
	assertSchemaExists(t, spec, "IngestReportNonDeclinedOverrideRequest")
	assertSchemaExists(t, spec, "IngestReportDeclinedOverrideRequest")

	assertSchemaRequiredFields(t, schemaObjectNode(t, spec, "IngestReportNonDeclinedRequest"), []string{"contract_version", "actor", "session_id", "operation_id", "trace_id", "flavor", "evidence", "source", "prescription_id", "verdict", "exit_code"})
	assertSchemaRequiredFields(t, schemaObjectNode(t, spec, "IngestReportDeclinedRequest"), []string{"contract_version", "actor", "session_id", "operation_id", "trace_id", "flavor", "evidence", "source", "prescription_id", "verdict", "decision_context"})
	assertSchemaRequiredFields(t, schemaObjectNode(t, spec, "IngestReportNonDeclinedOverrideRequest"), []string{"payload_override"})
	assertSchemaRequiredFields(t, schemaObjectNode(t, spec, "IngestReportDeclinedOverrideRequest"), []string{"payload_override"})
	assertSchemaAnyOfRequiredFields(t, findMappingValue(t, findMappingValue(t, schemaObjectNode(t, spec, "IngestReportNonDeclinedRequest"), "not"), "anyOf"), []string{"payload_override", "decision_context"})
	assertSchemaAnyOfRequiredFields(t, findMappingValue(t, findMappingValue(t, schemaObjectNode(t, spec, "IngestReportDeclinedRequest"), "not"), "anyOf"), []string{"payload_override", "exit_code"})
	assertSchemaRequiredFields(t, findSchemaProperty(t, spec, "IngestReportNonDeclinedOverrideRequest", "payload_override"), []string{"prescription_id", "verdict", "exit_code"})
	assertSchemaRequiredFields(t, findSchemaProperty(t, spec, "IngestReportDeclinedOverrideRequest", "payload_override"), []string{"prescription_id", "verdict", "decision_context"})
	assertSchemaRequiredFields(t, schemaObjectNode(t, spec, "IngestPrescribeOverrideRequest"), []string{"payload_override"})
	assertSchemaRequiredFields(t, schemaObjectNode(t, spec, "IngestReportNonDeclinedOverrideRequest"), []string{"payload_override"})
	assertSchemaRequiredFields(t, schemaObjectNode(t, spec, "IngestReportDeclinedOverrideRequest"), []string{"payload_override"})
	_ = findSchemaProperty(t, spec, "IngestPrescribeOverrideRequest", "artifact_digest")
	_ = findSchemaProperty(t, spec, "IngestReportNonDeclinedOverrideRequest", "artifact_digest")
	_ = findSchemaProperty(t, spec, "IngestReportDeclinedOverrideRequest", "artifact_digest")
	assertSchemaRequiredFields(t, findMappingValue(t, findSchemaProperty(t, spec, "IngestReportNonDeclinedOverrideRequest", "payload_override"), "not"), []string{"decision_context"})
	assertSchemaRequiredFields(t, findMappingValue(t, findSchemaProperty(t, spec, "IngestReportDeclinedOverrideRequest", "payload_override"), "not"), []string{"exit_code"})
	assertSchemaNoProperty(t, findSchemaProperty(t, spec, "IngestReportNonDeclinedOverrideRequest", "payload_override"), "artifact_digest")
	assertSchemaNoProperty(t, findSchemaProperty(t, spec, "IngestReportDeclinedOverrideRequest", "payload_override"), "artifact_digest")
	assertSchemaAnyOfRequiredFields(t, findMappingValue(t, findMappingValue(t, schemaObjectNode(t, spec, "IngestReportNonDeclinedOverrideRequest"), "not"), "anyOf"), []string{"prescription_id", "verdict", "exit_code", "decision_context", "external_refs"})
	assertSchemaAnyOfRequiredFields(t, findMappingValue(t, findMappingValue(t, schemaObjectNode(t, spec, "IngestReportDeclinedOverrideRequest"), "not"), "anyOf"), []string{"prescription_id", "verdict", "exit_code", "decision_context", "external_refs"})
	assertSchemaEnumValues(t, findSchemaProperty(t, spec, "IngestReportNonDeclinedRequest", "verdict"), []string{"success", "failure", "error"})
	assertSchemaEnumValues(t, findSchemaProperty(t, spec, "IngestReportDeclinedRequest", "verdict"), []string{"declined"})
	assertSchemaEnumValues(t, findSchemaProperty(t, spec, "IngestReportNonDeclinedOverrideRequest", "payload_override", "verdict"), []string{"success", "failure", "error"})
	assertSchemaEnumValues(t, findSchemaProperty(t, spec, "IngestReportDeclinedOverrideRequest", "payload_override", "verdict"), []string{"declined"})

	assertPrescribeExamples(t, spec)
	assertReportExamples(t, spec)
}

func loadOpenAPISpec(t *testing.T) *yaml.Node {
	t.Helper()
	return loadOpenAPISpecFromPath(t, filepath.Join("..", "..", "cmd", "evidra-api", "static", "openapi.yaml"))
}

func loadOpenAPISpecFromPath(t *testing.T, path string) *yaml.Node {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read openapi spec: %v", err)
	}

	var spec yaml.Node
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse openapi spec: %v", err)
	}
	if len(spec.Content) == 0 {
		t.Fatal("openapi spec is empty")
	}
	return &spec
}

func assertPathResponses(t *testing.T, spec *yaml.Node, path string, want []string) {
	t.Helper()

	pathNode := findMappingValue(t, findMappingValue(t, spec.Content[0], "paths"), path)
	postNode := findMappingValue(t, pathNode, "post")
	responses := findMappingValue(t, postNode, "responses")

	for _, code := range want {
		if findMappingValueOptional(responses, code) == nil {
			t.Fatalf("path %s missing response code %s", path, code)
		}
	}
}

func assertSchemaExists(t *testing.T, spec *yaml.Node, name string) {
	t.Helper()

	schemas := findMappingValue(t, findMappingValue(t, spec.Content[0], "components"), "schemas")
	if findMappingValueOptional(schemas, name) == nil {
		t.Fatalf("missing schema %s", name)
	}
}

func assertSchemaOneOfRefs(t *testing.T, spec *yaml.Node, name string, want []string) {
	t.Helper()

	schemas := findMappingValue(t, findMappingValue(t, spec.Content[0], "components"), "schemas")
	schema := findMappingValue(t, schemas, name)
	oneOf := findMappingValue(t, schema, "oneOf")
	if len(oneOf.Content) != len(want) {
		t.Fatalf("schema %s oneOf len = %d, want %d", name, len(oneOf.Content), len(want))
	}
	for i, ref := range want {
		item := oneOf.Content[i]
		got := findMappingValue(t, item, "$ref")
		if got.Value != "#/components/schemas/"+ref {
			t.Fatalf("schema %s oneOf[%d] ref = %q, want %q", name, i, got.Value, "#/components/schemas/"+ref)
		}
	}
}

func assertSchemaRequiredFields(t *testing.T, node *yaml.Node, want []string) {
	t.Helper()

	required := findMappingValue(t, node, "required")
	if len(required.Content) != len(want) {
		t.Fatalf("required len = %d, want %d", len(required.Content), len(want))
	}
	for i, field := range want {
		if required.Content[i].Value != field {
			t.Fatalf("required[%d] = %q, want %q", i, required.Content[i].Value, field)
		}
	}
}

func assertSchemaAnyOfRequiredFields(t *testing.T, node *yaml.Node, want []string) {
	t.Helper()

	if node == nil { //nolint:staticcheck // t.Fatal stops execution
		t.Fatal("anyOf node is nil")
		return
	}
	if len(node.Content) != len(want) {
		t.Fatalf("anyOf len = %d, want %d", len(node.Content), len(want))
	}
	got := map[string]struct{}{}
	for _, item := range node.Content {
		required := findMappingValue(t, item, "required")
		if len(required.Content) != 1 {
			t.Fatalf("anyOf item required len = %d, want 1", len(required.Content))
		}
		got[required.Content[0].Value] = struct{}{}
	}
	for _, field := range want {
		if _, ok := got[field]; !ok {
			t.Fatalf("anyOf missing required field %s", field)
		}
	}
}

func assertSchemaEnumValues(t *testing.T, node *yaml.Node, want []string) {
	t.Helper()

	enum := findMappingValue(t, node, "enum")
	if len(enum.Content) != len(want) {
		t.Fatalf("enum len = %d, want %d", len(enum.Content), len(want))
	}
	for i, value := range want {
		if enum.Content[i].Value != value {
			t.Fatalf("enum[%d] = %q, want %q", i, enum.Content[i].Value, value)
		}
	}
}

func assertSchemaNoProperty(t *testing.T, node *yaml.Node, property string) {
	t.Helper()

	properties := findMappingValueOptional(node, "properties")
	if properties == nil {
		t.Fatal("schema has no properties node")
	}
	if findMappingValueOptional(properties, property) != nil {
		t.Fatalf("schema unexpectedly contains property %s", property)
	}
}

func findSchemaProperty(t *testing.T, spec *yaml.Node, schemaName string, path ...string) *yaml.Node {
	t.Helper()

	node := schemaObjectNode(t, spec, schemaName)
	for _, key := range path {
		node = findMappingValue(t, node, "properties")
		node = findMappingValue(t, node, key)
	}
	return node
}

func schemaObjectNode(t *testing.T, spec *yaml.Node, schemaName string) *yaml.Node {
	t.Helper()

	schemas := findMappingValue(t, findMappingValue(t, spec.Content[0], "components"), "schemas")
	schema := findMappingValue(t, schemas, schemaName)
	if allOf := findMappingValueOptional(schema, "allOf"); allOf != nil {
		for _, item := range allOf.Content {
			if findMappingValueOptional(item, "properties") != nil {
				return item
			}
		}
		t.Fatalf("schema %s allOf has no object node", schemaName)
	}
	return schema
}

func assertPrescribeExamples(t *testing.T, spec *yaml.Node) {
	t.Helper()

	examples := findOperationExamples(t, spec, "/v1/evidence/ingest/prescribe")
	typed := decodeExampleMap(t, findMappingValue(t, examples, "typed"))
	override := decodeExampleMap(t, findMappingValue(t, examples, "override"))

	if typed["payload_override"] != nil {
		t.Fatal("prescribe typed example should not set payload_override")
	}
	if _, ok := typed["intent"]; !ok {
		t.Fatal("prescribe typed example missing intent")
	}
	if _, ok := typed["smart_target"]; ok {
		t.Fatal("prescribe typed example should not set smart_target when intent is present")
	}

	overrideBody := mustMap(t, override["payload_override"], "prescribe override payload_override")
	if _, ok := overrideBody["intent"]; !ok {
		t.Fatal("prescribe override payload_override missing intent")
	}
	if _, ok := override["canonical_action"]; ok {
		t.Fatal("prescribe override example should not set top-level canonical_action")
	}
	if _, ok := override["smart_target"]; ok {
		t.Fatal("prescribe override example should not set top-level smart_target")
	}
	if _, ok := override["artifact_digest"]; !ok {
		t.Fatal("prescribe override example missing top-level artifact_digest")
	}
}

func assertReportExamples(t *testing.T, spec *yaml.Node) {
	t.Helper()

	examples := findOperationExamples(t, spec, "/v1/evidence/ingest/report")
	typed := decodeExampleMap(t, findMappingValue(t, examples, "typed"))
	override := decodeExampleMap(t, findMappingValue(t, examples, "override"))

	if typed["payload_override"] != nil {
		t.Fatal("report typed example should not set payload_override")
	}
	if _, ok := typed["prescription_id"]; !ok {
		t.Fatal("report typed example missing prescription_id")
	}
	if _, ok := typed["verdict"]; !ok {
		t.Fatal("report typed example missing verdict")
	}
	if typedVerdict, _ := typed["verdict"].(string); typedVerdict == "declined" {
		if _, ok := typed["decision_context"]; !ok {
			t.Fatal("declined report typed example missing decision_context")
		}
	} else {
		if _, ok := typed["exit_code"]; !ok {
			t.Fatal("non-declined report typed example missing exit_code")
		}
	}

	overrideBody := mustMap(t, override["payload_override"], "report override payload_override")
	if _, ok := overrideBody["prescription_id"]; !ok {
		t.Fatal("report override payload_override missing prescription_id")
	}
	if _, ok := overrideBody["verdict"]; !ok {
		t.Fatal("report override payload_override missing verdict")
	}
	if verdict, _ := overrideBody["verdict"].(string); verdict == "declined" {
		if _, ok := overrideBody["decision_context"]; !ok {
			t.Fatal("declined report override payload_override missing decision_context")
		}
		if _, ok := overrideBody["exit_code"]; ok {
			t.Fatal("declined report override payload_override should not set exit_code")
		}
	} else {
		if _, ok := overrideBody["exit_code"]; !ok {
			t.Fatal("non-declined report override payload_override missing exit_code")
		}
	}
	if _, ok := override["prescription_id"]; ok {
		t.Fatal("report override example should not set top-level prescription_id")
	}
	if _, ok := override["verdict"]; ok {
		t.Fatal("report override example should not set top-level verdict")
	}
	if _, ok := override["exit_code"]; ok {
		t.Fatal("report override example should not set top-level exit_code")
	}
	if _, ok := override["decision_context"]; ok {
		t.Fatal("report override example should not set top-level decision_context")
	}
	if _, ok := override["artifact_digest"]; !ok {
		t.Fatal("report override example missing top-level artifact_digest")
	}
}

func findOperationExamples(t *testing.T, spec *yaml.Node, path string) *yaml.Node {
	t.Helper()

	pathNode := findMappingValue(t, findMappingValue(t, spec.Content[0], "paths"), path)
	postNode := findMappingValue(t, pathNode, "post")
	requestBody := findMappingValue(t, postNode, "requestBody")
	content := findMappingValue(t, requestBody, "content")
	applicationJSON := findMappingValue(t, content, "application/json")
	return findMappingValue(t, applicationJSON, "examples")
}

func findMappingValue(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()

	value := findMappingValueOptional(node, key)
	if value == nil {
		t.Fatalf("missing key %q", key)
	}
	return value
}

func findMappingValueOptional(node *yaml.Node, key string) *yaml.Node {
	if node == nil {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func decodeExampleMap(t *testing.T, node *yaml.Node) map[string]any {
	t.Helper()

	if node == nil {
		t.Fatal("example node is nil")
	}
	var out map[string]any
	if err := node.Decode(&out); err != nil {
		t.Fatalf("decode example: %v", err)
	}
	if value, ok := out["value"].(map[string]any); ok {
		return value
	}
	return out
}

func mustMap(t *testing.T, value any, context string) map[string]any {
	t.Helper()

	if value == nil {
		t.Fatalf("%s is nil", context)
	}
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s is %T, want map[string]any", context, value)
	}
	return m
}
