package automationevent

import (
	"encoding/json"
	"strings"
	"testing"

	"samebits.com/evidra-benchmark/pkg/evidence"
)

func TestRecordContractValidateRequiredFields(t *testing.T) {
	in := RecordInput{}
	err := ValidateRecordInput(in)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "contract_version") {
		t.Fatalf("error should mention contract_version, got: %v", err)
	}
}

func TestRecordContractValidateRequiresArtifactOrCanonicalAction(t *testing.T) {
	in := RecordInput{
		ContractVersion: RecordContractVersionV1,
		SessionID:       "sess_1",
		OperationID:     "op_1",
		Tool:            "terraform",
		Operation:       "apply",
		Environment:     "staging",
		Actor: evidence.Actor{
			Type: "ci",
			ID:   "github-actions",
		},
		ExitCode:   0,
		DurationMs: 4200,
	}
	err := ValidateRecordInput(in)
	if err == nil {
		t.Fatal("expected error when both raw_artifact and canonical_action are missing")
	}
	if !strings.Contains(err.Error(), "raw_artifact") {
		t.Fatalf("error should mention raw_artifact/canonical_action requirement, got: %v", err)
	}
}

func TestRecordContractValidateAllowsRawArtifactPath(t *testing.T) {
	in := RecordInput{
		ContractVersion: RecordContractVersionV1,
		SessionID:       "sess_1",
		OperationID:     "op_1",
		Tool:            "kubectl",
		Operation:       "apply",
		Environment:     "staging",
		Actor: evidence.Actor{
			Type: "human",
			ID:   "operator-1",
		},
		ExitCode:    0,
		DurationMs:  1300,
		RawArtifact: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: demo\n",
	}
	if err := ValidateRecordInput(in); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRecordContractValidateAllowsCanonicalActionPath(t *testing.T) {
	in := RecordInput{
		ContractVersion: RecordContractVersionV1,
		SessionID:       "sess_2",
		OperationID:     "op_2",
		Tool:            "terraform",
		Operation:       "apply",
		Environment:     "production",
		Actor: evidence.Actor{
			Type: "ci",
			ID:   "pipeline-1",
		},
		ExitCode:        1,
		DurationMs:      9300,
		CanonicalAction: json.RawMessage(`{"tool":"terraform","operation":"apply","operation_class":"mutate","scope_class":"production","resource_count":1,"resource_shape_hash":"sha256:test"}`),
	}
	if err := ValidateRecordInput(in); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestRecordContractValidateRejectsNegativeDuration(t *testing.T) {
	in := RecordInput{
		ContractVersion: RecordContractVersionV1,
		SessionID:       "sess_3",
		OperationID:     "op_3",
		Tool:            "helm",
		Operation:       "upgrade",
		Environment:     "development",
		Actor: evidence.Actor{
			Type: "ci",
			ID:   "pipeline-2",
		},
		ExitCode:        0,
		DurationMs:      -1,
		CanonicalAction: json.RawMessage(`{"tool":"helm","operation":"upgrade","operation_class":"mutate","scope_class":"development","resource_count":1,"resource_shape_hash":"sha256:test"}`),
	}
	err := ValidateRecordInput(in)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "duration_ms") {
		t.Fatalf("error should mention duration_ms, got: %v", err)
	}
}

