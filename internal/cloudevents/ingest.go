package cloudevents

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// knownFields are top-level CloudEvents spec fields that are not extensions.
var knownFields = map[string]bool{
	"specversion":     true,
	"id":              true,
	"source":          true,
	"type":            true,
	"subject":         true,
	"time":            true,
	"datacontenttype": true,
	"dataschema":      true,
	"data":            true,
	"data_base64":     true,
}

// ParseRequest parses a CloudEvents HTTP request body into one or more StoredEvents.
// Supports application/cloudevents+json (single) and application/cloudevents-batch+json (batch).
func ParseRequest(r *http.Request, body []byte) ([]StoredEvent, error) {
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if idx := strings.Index(ct, ";"); idx >= 0 {
		ct = strings.TrimSpace(ct[:idx])
	}
	if ct == "application/cloudevents-batch+json" {
		return parseBatch(body)
	}
	e, err := parseSingle(body)
	if err != nil {
		return nil, err
	}
	return []StoredEvent{e}, nil
}

func parseSingle(body []byte) (StoredEvent, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return StoredEvent{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return rawToStoredEvent(raw)
}

func parseBatch(body []byte) ([]StoredEvent, error) {
	var raws []map[string]json.RawMessage
	if err := json.Unmarshal(body, &raws); err != nil {
		return nil, fmt.Errorf("invalid batch JSON: %w", err)
	}
	out := make([]StoredEvent, 0, len(raws))
	for _, raw := range raws {
		e, err := rawToStoredEvent(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func rawToStoredEvent(raw map[string]json.RawMessage) (StoredEvent, error) {
	e := StoredEvent{}
	if v, ok := raw["id"]; ok {
		_ = json.Unmarshal(v, &e.ID)
	}
	if v, ok := raw["source"]; ok {
		_ = json.Unmarshal(v, &e.Source)
	}
	if v, ok := raw["type"]; ok {
		_ = json.Unmarshal(v, &e.Type)
	}
	if v, ok := raw["subject"]; ok {
		_ = json.Unmarshal(v, &e.Subject)
	}
	if v, ok := raw["time"]; ok {
		var t time.Time
		if err := json.Unmarshal(v, &t); err == nil {
			e.Time = t.UTC()
		}
	}
	if v, ok := raw["data"]; ok {
		e.Data = json.RawMessage(v)
	}

	// Extensions: all non-standard top-level fields
	exts := make(map[string]interface{})
	for k, v := range raw {
		if knownFields[k] {
			continue
		}
		var val interface{}
		if err := json.Unmarshal(v, &val); err == nil {
			exts[k] = val
		}
	}
	if len(exts) > 0 {
		e.Extensions = exts
	}

	if err := Validate(&e); err != nil {
		return StoredEvent{}, err
	}

	h, err := ComputeIntegrityHash(e)
	if err != nil {
		return StoredEvent{}, err
	}
	e.IntegrityHash = h

	return e, nil
}

// ExtensionString returns the string value of an extension field, or "" if absent or not a string.
func ExtensionString(extensions map[string]interface{}, key string) string {
	if extensions == nil {
		return ""
	}
	raw, ok := extensions[key]
	if !ok {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

// ExtensionBool returns the boolean value of an extension field.
func ExtensionBool(extensions map[string]interface{}, key string) bool {
	if extensions == nil {
		return false
	}
	raw, ok := extensions[key]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		lv := strings.ToLower(strings.TrimSpace(v))
		return lv == "true" || lv == "1" || lv == "yes"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return false
	}
}
