package mcpserver

import (
	"fmt"
	"strings"

	"samebits.com/evidra/internal/canon"
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

func toExecContractCanonicalAction(action *canon.CanonicalAction) *execcontract.CanonicalAction {
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

func buildSmartCanonicalAction(input PrescribeInput) (canon.CanonicalAction, error) {
	resource, err := parseSmartResource(input.Resource, input.Namespace)
	if err != nil {
		return canon.CanonicalAction{}, err
	}

	scopeClass := canon.ResolveScopeClass(input.Environment, []canon.ResourceID{resource})
	return canon.CanonicalAction{
		Tool:             strings.ToLower(strings.TrimSpace(input.Tool)),
		Operation:        strings.ToLower(strings.TrimSpace(input.Operation)),
		OperationClass:   smartOperationClass(input.Tool, input.Operation),
		ResourceIdentity: []canon.ResourceID{resource},
		ScopeClass:       scopeClass,
		ResourceCount:    1,
	}, nil
}

func parseSmartResource(raw, namespace string) (canon.ResourceID, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return canon.ResourceID{}, fmt.Errorf("resource is required when raw_artifact is omitted")
	}

	id := canon.ResourceID{Namespace: strings.TrimSpace(namespace)}
	if strings.Contains(value, "/") {
		parts := strings.SplitN(value, "/", 2)
		id.Kind = strings.TrimSpace(parts[0])
		id.Name = strings.TrimSpace(parts[1])
	} else {
		id.Name = value
	}
	if id.Name == "" {
		return canon.ResourceID{}, fmt.Errorf("resource is required when raw_artifact is omitted")
	}
	return id, nil
}

func smartOperationClass(tool, operation string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "kubectl", "oc", "helm", "kustomize":
		return smartK8sOperationClass(operation)
	case "terraform":
		switch strings.ToLower(strings.TrimSpace(operation)) {
		case "apply", "import":
			return "mutate"
		case "destroy":
			return "destroy"
		case "plan", "validate", "refresh", "show", "state":
			return "plan"
		default:
			return "unknown"
		}
	case "docker", "podman", "nerdctl", "docker-compose", "compose":
		switch strings.ToLower(strings.TrimSpace(operation)) {
		case "run", "create", "build", "push", "start", "up":
			return "mutate"
		case "rm", "stop", "kill", "down":
			return "destroy"
		case "inspect", "ps", "logs", "images":
			return "read"
		default:
			return "unknown"
		}
	default:
		return smartK8sOperationClass(operation)
	}
}

func smartK8sOperationClass(operation string) string {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "apply", "create", "patch", "upgrade", "install", "replace", "set", "annotate", "label",
		"rollout", "scale", "autoscale", "taint", "cordon", "uncordon", "rollback":
		return "mutate"
	case "delete", "uninstall", "drain", "destroy":
		return "destroy"
	case "get", "describe", "logs", "top", "diff", "list", "status", "show", "template", "plan":
		return "read"
	default:
		return "unknown"
	}
}
