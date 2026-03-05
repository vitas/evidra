package mcpserver

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed schemas/prescribe.schema.json
var prescribeSchemaBytes []byte

//go:embed schemas/report.schema.json
var reportSchemaBytes []byte

//go:embed schemas/get_event.schema.json
var getEventSchemaBytes []byte

func loadInputSchema(raw []byte, name string) (map[string]any, error) {
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, fmt.Errorf("failed to parse embedded MCP schema %s: %w", name, err)
	}
	return schema, nil
}
