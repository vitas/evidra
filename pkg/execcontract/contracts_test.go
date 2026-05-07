package execcontract

import "testing"

func TestPrescribeFullToolDefinition_UsesDedicatedSchema(t *testing.T) {
	t.Parallel()

	def, err := PrescribeFullToolDefinition()
	if err != nil {
		t.Fatalf("PrescribeFullToolDefinition: %v", err)
	}
	if def.Name != PrescribeFullToolName {
		t.Fatalf("name = %q, want %q", def.Name, PrescribeFullToolName)
	}

	required, ok := def.Parameters["required"].([]string)
	if !ok {
		t.Fatalf("required = %#v, want []string", def.Parameters["required"])
	}
	for _, field := range []string{"tool", "operation", "actor"} {
		if !contains(required, field) {
			t.Fatalf("required missing %q: %#v", field, required)
		}
	}
	if !contains(required, "raw_artifact") {
		t.Fatalf("required missing raw_artifact: %#v", required)
	}

	properties, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v, want map[string]any", def.Parameters["properties"])
	}
	if _, ok := properties["raw_artifact"]; !ok {
		t.Fatalf("properties missing raw_artifact: %#v", properties)
	}
}

func TestPrescribeSmartToolDefinition_UsesDedicatedSchema(t *testing.T) {
	t.Parallel()

	def, err := PrescribeSmartToolDefinition()
	if err != nil {
		t.Fatalf("PrescribeSmartToolDefinition: %v", err)
	}
	if def.Name != PrescribeSmartToolName {
		t.Fatalf("name = %q, want %q", def.Name, PrescribeSmartToolName)
	}

	required, ok := def.Parameters["required"].([]string)
	if !ok {
		t.Fatalf("required = %#v, want []string", def.Parameters["required"])
	}
	for _, field := range []string{"tool", "operation", "actor", "resource"} {
		if !contains(required, field) {
			t.Fatalf("required missing %q: %#v", field, required)
		}
	}

	properties, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v, want map[string]any", def.Parameters["properties"])
	}
	for _, field := range []string{"resource", "namespace"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("properties missing %q: %#v", field, properties)
		}
	}
}

func TestValidatePrescribeFullInput_RequiresRawArtifact(t *testing.T) {
	t.Parallel()

	err := ValidatePrescribeFullInput(PrescribeFullInput{
		Tool:      "kubectl",
		Operation: "apply",
		Actor: Actor{
			Type:   "agent",
			ID:     "demo",
			Origin: "mcp-stdio",
		},
	})
	if err == nil {
		t.Fatal("expected validation error when raw_artifact is missing")
	}
}

func TestValidatePrescribeSmartInput_RequiresResource(t *testing.T) {
	t.Parallel()

	err := ValidatePrescribeSmartInput(PrescribeSmartInput{
		Tool:      "kubectl",
		Operation: "apply",
		Actor: Actor{
			Type:   "agent",
			ID:     "demo",
			Origin: "mcp-stdio",
		},
	})
	if err == nil {
		t.Fatal("expected validation error when resource is missing")
	}
}

func TestValidatePrescribeSmartInput_AllowsNamespaceTarget(t *testing.T) {
	t.Parallel()

	err := ValidatePrescribeSmartInput(PrescribeSmartInput{
		Tool:      "kubectl",
		Operation: "apply",
		Resource:  "deployment/web",
		Namespace: "default",
		Actor: Actor{
			Type:   "agent",
			ID:     "demo",
			Origin: "mcp-stdio",
		},
	})
	if err != nil {
		t.Fatalf("ValidatePrescribeSmartInput: %v", err)
	}
}

func TestValidateReportInput_DeclinedRules(t *testing.T) {
	t.Parallel()

	exitCode := 0
	err := ValidateReportInput(ReportInput{
		PrescriptionID: "presc-1",
		Verdict:        VerdictDeclined,
		ExitCode:       &exitCode,
		DecisionContext: &DecisionContext{
			Trigger: "risk_threshold_exceeded",
			Reason:  "blast radius too large",
		},
	})
	if err == nil {
		t.Fatal("expected declined validation error when exit_code is present")
	}

	err = ValidateReportInput(ReportInput{
		PrescriptionID: "presc-1",
		Verdict:        VerdictDeclined,
		DecisionContext: &DecisionContext{
			Trigger: "risk_threshold_exceeded",
			Reason:  "blast radius too large",
		},
	})
	if err != nil {
		t.Fatalf("declined validation failed: %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
