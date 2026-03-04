package invocation

import (
	"errors"
	"fmt"
)

// Canonical keys for ToolInvocation.Params and ToolInvocation.Context.
const (
	KeyTarget     = "target"
	KeyPayload    = "payload"
	KeyRiskTags   = "risk_tags"
	KeyScenarioID = "scenario_id"
	KeyAction     = "action"
	KeySource     = "source"
	KeyIntent     = "intent"
)

type Actor struct {
	Type   string `json:"type"`
	ID     string `json:"id"`
	Origin string `json:"origin"`
}

type ToolInvocation struct {
	Actor       Actor                  `json:"actor"`
	Tool        string                 `json:"tool"`
	Operation   string                 `json:"operation"`
	Params      map[string]interface{} `json:"params"`
	Context     map[string]interface{} `json:"context"`
	Environment string                 `json:"environment,omitempty"`
}

func (ti *ToolInvocation) ValidateStructure() error {
	if ti.Actor.Type == "" {
		return errors.New("actor.type is required")
	}
	if ti.Actor.ID == "" {
		return errors.New("actor.id is required")
	}
	if ti.Actor.Origin == "" {
		return errors.New("actor.origin is required")
	}
	if ti.Tool == "" {
		return errors.New("tool is required")
	}
	if ti.Operation == "" {
		return errors.New("operation is required")
	}
	if ti.Params == nil {
		return errors.New("params is required")
	}

	if err := validateMap(ti.Params, KeyTarget); err != nil {
		return err
	}
	if err := validateMap(ti.Params, KeyPayload); err != nil {
		return err
	}
	if err := validateMap(ti.Params, KeyAction); err != nil {
		return err
	}
	if err := validateRiskTags(ti.Params[KeyRiskTags]); err != nil {
		return err
	}

	return nil
}

func validateMap(params map[string]interface{}, key string) error {
	val, ok := params[key]
	if !ok || val == nil {
		return nil
	}
	if _, isMap := val.(map[string]interface{}); !isMap {
		return fmt.Errorf("field '%s' must be a JSON object", key)
	}
	return nil
}

func validateRiskTags(val interface{}) error {
	if val == nil {
		return nil
	}

	switch v := val.(type) {
	case []string:
		return nil
	case []interface{}:
		for _, entry := range v {
			if _, ok := entry.(string); !ok {
				return errors.New("field 'risk_tags' must be a list of strings")
			}
		}
		return nil
	default:
		return errors.New("field 'risk_tags' must be a list of strings")
	}
}
