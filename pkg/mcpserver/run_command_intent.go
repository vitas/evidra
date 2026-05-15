package mcpserver

import (
	"strings"

	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/proxy"
)

func deriveAutoPrescribeInput(command string, actorID string) (PrescribeInput, bool, error) {
	if actorID == "" {
		actorID = "infrastructure-agent"
	}
	tool, operation, class := proxy.ClassifyCommand(command)
	if class != proxy.OpMutate && class != proxy.OpDestroy {
		return PrescribeInput{}, false, nil
	}

	action := evidence.CanonicalAction{
		Tool:           strings.ToLower(strings.TrimSpace(tool)),
		Operation:      strings.ToLower(strings.TrimSpace(operation)),
		OperationClass: string(class),
		ScopeClass:     "unknown",
	}

	if resource, namespace, ok := deriveCommandTarget(command); ok {
		resourceID, err := parseSmartResource(resource, namespace)
		if err != nil {
			return PrescribeInput{}, false, err
		}
		action.ResourceIdentity = []evidence.ResourceID{resourceID}
		action.ResourceCount = 1
		action.ScopeClass = evidence.ResolveScopeClass("", []evidence.ResourceID{resourceID})
	}

	return PrescribeInput{
		Actor:           InputActor{Type: "agent", ID: actorID, Origin: "mcp"},
		Tool:            tool,
		Operation:       operation,
		CanonicalAction: &action,
	}, true, nil
}

func deriveCommandTarget(command string) (resource string, namespace string, ok bool) {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) < 3 {
		return "", "", false
	}

	namespace = commandNamespace(fields)

	switch fields[1] {
	case "patch", "delete", "describe", "get", "logs":
		if len(fields) >= 4 && !strings.HasPrefix(fields[3], "-") {
			return normalizeCommandResource(fields[2], fields[3]), namespace, true
		}
	case "scale", "annotate", "label":
		if len(fields) >= 4 && !strings.HasPrefix(fields[3], "-") {
			return normalizeCommandResource(fields[2], fields[3]), namespace, true
		}
	case "rollout":
		if len(fields) >= 4 && !strings.HasPrefix(fields[3], "-") {
			return normalizeCommandResource(fields[2], fields[3]), namespace, true
		}
	}

	return "", namespace, false
}

func commandNamespace(fields []string) string {
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case "-n", "--namespace":
			if i+1 < len(fields) {
				return strings.TrimSpace(fields[i+1])
			}
		default:
			if strings.HasPrefix(fields[i], "--namespace=") {
				return strings.TrimSpace(strings.TrimPrefix(fields[i], "--namespace="))
			}
		}
	}
	return ""
}

func normalizeCommandResource(kind, name string) string {
	kind = strings.TrimSpace(kind)
	name = strings.TrimSpace(name)
	if kind == "" || name == "" {
		return ""
	}
	if strings.Contains(kind, "/") {
		return kind
	}
	if strings.Contains(name, "/") {
		return name
	}
	return kind + "/" + name
}
