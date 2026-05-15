package mcpserver

import (
	"fmt"
	"strings"

	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/execcontract"
)

func preparePrescribeInput(input PrescribeInput) (PrescribeInput, error) {
	if err := execcontract.ValidatePrescribeInput(toExecContractPrescribeInput(input)); err != nil {
		return PrescribeInput{}, err
	}
	return input, nil
}

func toExecContractPrescribeInput(input PrescribeInput) execcontract.PrescribeInput {
	return execcontract.PrescribeInput{
		Tool:            input.Tool,
		Operation:       input.Operation,
		RawArtifact:     input.RawArtifact,
		Resource:        input.Resource,
		Namespace:       input.Namespace,
		CanonicalAction: toExecContractCanonicalAction(input.CanonicalAction),
		Actor: execcontract.Actor{
			Type:         input.Actor.Type,
			ID:           input.Actor.ID,
			Origin:       input.Actor.Origin,
			InstanceID:   input.Actor.InstanceID,
			Version:      input.Actor.Version,
			SkillVersion: input.Actor.SkillVersion,
		},
		SessionID:       input.SessionID,
		OperationID:     input.OperationID,
		Attempt:         input.Attempt,
		TraceID:         input.TraceID,
		SpanID:          input.SpanID,
		ParentSpanID:    input.ParentSpanID,
		Environment:     input.Environment,
		ScopeDimensions: input.ScopeDimensions,
	}
}

func toExecContractCanonicalAction(action *evidence.CanonicalAction) *execcontract.CanonicalAction {
	if action == nil {
		return nil
	}

	resourceIdentity := make([]execcontract.ResourceID, 0, len(action.ResourceIdentity))
	for _, resource := range action.ResourceIdentity {
		resourceIdentity = append(resourceIdentity, execcontract.ResourceID{
			APIVersion: resource.APIVersion,
			Kind:       resource.Kind,
			Namespace:  resource.Namespace,
			Name:       resource.Name,
			Type:       resource.Type,
			Actions:    resource.Actions,
		})
	}

	return &execcontract.CanonicalAction{
		ResourceIdentity:  resourceIdentity,
		ResourceCount:     action.ResourceCount,
		OperationClass:    action.OperationClass,
		ScopeClass:        action.ScopeClass,
		ResourceShapeHash: action.ResourceShapeHash,
	}
}

func parseSmartResource(raw, namespace string) (evidence.ResourceID, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return evidence.ResourceID{}, fmt.Errorf("resource is required when raw_artifact is omitted")
	}

	id := evidence.ResourceID{Namespace: strings.TrimSpace(namespace)}
	if strings.Contains(value, "/") {
		parts := strings.SplitN(value, "/", 2)
		id.Kind = strings.TrimSpace(parts[0])
		id.Name = strings.TrimSpace(parts[1])
	} else {
		id.Name = value
	}
	if id.Name == "" {
		return evidence.ResourceID{}, fmt.Errorf("resource is required when raw_artifact is omitted")
	}
	return id, nil
}
