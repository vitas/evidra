package argo

import (
	"encoding/json"
	"fmt"
	"strings"

	ce "evidra/internal/cloudevents"
)

func NormalizeSourceEvent(se SourceEvent, defaultEnv string) (ce.StoredEvent, error) {
	if strings.TrimSpace(se.ID) == "" || strings.TrimSpace(se.App) == "" || se.Occurred.IsZero() {
		return ce.StoredEvent{}, fmt.Errorf("invalid source event")
	}
	env := strings.TrimSpace(se.Namespace)
	if env == "" {
		env = strings.TrimSpace(defaultEnv)
	}
	if env == "" {
		env = "unknown"
	}
	cluster := strings.TrimSpace(se.Cluster)
	if cluster == "" {
		cluster = "unknown"
	}
	actor := strings.TrimSpace(se.Actor)
	if actor == "" {
		actor = "argocd"
	}
	eventType := strings.TrimSpace(se.EventType)
	if eventType == "" {
		eventType = "argo.sync.finished"
	}
	revision := strings.TrimSpace(se.Revision)

	extensions := map[string]interface{}{
		"cluster":         cluster,
		"namespace":       env,
		"initiator":       actor,
		"sync_revision":   revision,
		"argo_event_type": eventType,
	}
	if revision != "" {
		extensions["commit_sha"] = revision
	}
	if strings.TrimSpace(se.Result) != "" {
		extensions["argo_result"] = strings.TrimSpace(se.Result)
	}
	if se.HistoryID > 0 {
		extensions["history_id"] = se.HistoryID
	}
	if strings.TrimSpace(se.OperationKey) != "" {
		extensions["operation_id"] = strings.TrimSpace(se.OperationKey)
	}
	if strings.TrimSpace(se.HealthStatus) != "" {
		extensions["health_status"] = strings.TrimSpace(se.HealthStatus)
	}

	dataFields := map[string]interface{}{
		"argocd_app": strings.TrimSpace(se.App),
		"phase":      strings.TrimSpace(se.Result),
		"result":     strings.TrimSpace(se.Result),
	}
	if se.Payload != nil {
		dataFields["source_payload"] = se.Payload
		if annotations, ok := payloadAnnotations(se.Payload); ok {
			if v := strings.TrimSpace(annotations["evidra.io/change-id"]); v != "" {
				extensions["external_change_id"] = v
			}
			if v := strings.TrimSpace(annotations["evidra.io/ticket"]); v != "" {
				extensions["ticket_id"] = v
			}
			if v := strings.TrimSpace(annotations["evidra.io/approvals-ref"]); v != "" {
				extensions["approval_reference"] = v
			}
			if v := strings.TrimSpace(annotations["evidra.io/approvals-json"]); v != "" {
				extensions["approvals_json"] = v
				var parsed []map[string]interface{}
				if err := json.Unmarshal([]byte(v), &parsed); err == nil {
					dataFields["approvals"] = parsed
				}
			}
		}
	}

	dataJSON, err := json.Marshal(dataFields)
	if err != nil {
		return ce.StoredEvent{}, fmt.Errorf("marshal data: %w", err)
	}

	return ce.StoredEvent{
		ID:         "evt_argocd_" + se.ID,
		Source:     "argocd",
		Type:       eventType,
		Time:       se.Occurred.UTC(),
		Subject:    strings.TrimSpace(se.App),
		Extensions: extensions,
		Data:       json.RawMessage(dataJSON),
	}, nil
}

func payloadAnnotations(payload map[string]interface{}) (map[string]string, bool) {
	raw, ok := payload["annotations"]
	if !ok || raw == nil {
		return nil, false
	}
	out := map[string]string{}
	switch vv := raw.(type) {
	case map[string]string:
		for k, v := range vv {
			out[k] = strings.TrimSpace(v)
		}
	case map[string]interface{}:
		for k, v := range vv {
			if s, ok := v.(string); ok {
				out[k] = strings.TrimSpace(s)
			}
		}
	default:
		return nil, false
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}
