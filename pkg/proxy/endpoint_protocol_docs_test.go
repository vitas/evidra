package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type publishedField struct {
	kind     string
	required bool
	values   []string
}

var inlineProtocolCode = regexp.MustCompile("`([^`]+)`")

func checkPublishedToolFields(document string, tools []epTool) error {
	for _, tool := range tools {
		fields, err := publishedFieldsFor(document, tool.Name)
		if err != nil {
			return err
		}
		var schema struct {
			Properties map[string]struct {
				Type string   `json:"type"`
				Enum []string `json:"enum"`
			} `json:"properties"`
			Required []string `json:"required"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			return fmt.Errorf("%s schema: %w", tool.Name, err)
		}
		if len(fields) != len(schema.Properties) {
			return fmt.Errorf("%s document lists %d fields, schema has %d", tool.Name, len(fields), len(schema.Properties))
		}
		for name, property := range schema.Properties {
			field, ok := fields[name]
			if !ok {
				return fmt.Errorf("%s document omits %s", tool.Name, name)
			}
			kind := property.Type
			if len(property.Enum) > 0 {
				kind = "enum"
			}
			if field.kind != kind || field.required != slices.Contains(schema.Required, name) {
				return fmt.Errorf("%s %s type/required mismatch: document=%s/%t schema=%s/%t",
					tool.Name, name, field.kind, field.required, kind, slices.Contains(schema.Required, name))
			}
			if !slices.Equal(field.values, property.Enum) {
				return fmt.Errorf("%s %s enum mismatch: document=%v schema=%v", tool.Name, name, field.values, property.Enum)
			}
		}
	}
	return nil
}

func publishedFieldsFor(document, toolName string) (map[string]publishedField, error) {
	heading := "## `" + toolName + "`"
	at := strings.Index(document, heading)
	if at < 0 {
		return nil, fmt.Errorf("protocol document has no %s heading", toolName)
	}
	section := document[at+len(heading):]
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	lines := strings.Split(section, "\n")
	table := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "| Field |") {
			table = i + 2
			break
		}
	}
	if table < 0 {
		return nil, fmt.Errorf("%s has no field table", toolName)
	}
	fields := make(map[string]publishedField)
	for _, line := range lines[table:] {
		if !strings.HasPrefix(line, "|") {
			break
		}
		columns := strings.Split(strings.Trim(line, "|"), "|")
		if len(columns) != 4 {
			return nil, fmt.Errorf("%s malformed field row: %s", toolName, line)
		}
		name := strings.Trim(strings.TrimSpace(columns[0]), "`")
		if _, exists := fields[name]; exists {
			return nil, fmt.Errorf("%s duplicate field %s", toolName, name)
		}
		required := strings.TrimSpace(columns[2])
		if required != "Yes" && required != "No" {
			return nil, fmt.Errorf("%s %s has invalid required value %s", toolName, name, required)
		}
		field := publishedField{kind: strings.TrimSpace(columns[1]), required: required == "Yes"}
		for _, match := range inlineProtocolCode.FindAllStringSubmatch(columns[3], -1) {
			field.values = append(field.values, match[1])
		}
		fields[name] = field
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("%s field table is empty", toolName)
	}
	return fields, nil
}

func TestPublishedProtocolFieldsMatchAdvertisedToolSchemas(t *testing.T) {
	raw, err := os.ReadFile("../../docs/protocol.md")
	if err != nil {
		t.Fatal(err)
	}
	document := string(raw)
	if err := checkPublishedToolFields(document, epLocalTools); err != nil {
		t.Fatal(err)
	}

	// Prove the check detects the drift it is meant to prevent.
	for _, mutation := range []struct {
		name, before, after string
	}{
		{"required field", "| `objective` | string | Yes |", "| `objective` | string | No |"},
		{"report enum", "`completed`, `failed`, `cancelled`, or `abandoned`", "`completed`, `failed`, or `abandoned`"},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			changed := strings.Replace(document, mutation.before, mutation.after, 1)
			if changed == document {
				t.Fatalf("test mutation did not match protocol document: %q", mutation.before)
			}
			if err := checkPublishedToolFields(changed, epLocalTools); err == nil {
				t.Fatal("schema drift was accepted")
			}
		})
	}
}
