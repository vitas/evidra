package evidence

import (
	"encoding/json"
	"testing"
)

func TestPrescriptionPayload_Marshal(t *testing.T) {
	t.Parallel()

	original := PrescriptionPayload{
		PrescriptionID:  "rx_01ABC",
		CanonicalAction: json.RawMessage(`{"op":"apply","resource":"deployment/nginx"}`),
		RiskInputs: []RiskInput{
			{
				Source:    "evidra/native",
				RiskLevel: "high",
				RiskTags:  []string{"k8s.privileged_container"},
				Detail:    "native detector output",
			},
			{
				Source:    "trivy/0.58.0",
				RiskLevel: "critical",
				RiskTags:  []string{"trivy.DS002"},
				Detail:    "7 findings (3 critical, 2 high, 2 medium)",
			},
		},
		EffectiveRisk: "critical",
		TTLMs:         30000,
		CanonSource:   "k8s",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("PrescriptionPayload marshal: %v", err)
	}

	var decoded PrescriptionPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("PrescriptionPayload unmarshal: %v", err)
	}

	if decoded.PrescriptionID != "rx_01ABC" {
		t.Errorf("prescription_id = %q, want %q", decoded.PrescriptionID, "rx_01ABC")
	}
	if decoded.TTLMs != 30000 {
		t.Errorf("ttl_ms = %d, want %d", decoded.TTLMs, 30000)
	}
	if decoded.CanonSource != "k8s" {
		t.Errorf("canon_source = %q, want %q", decoded.CanonSource, "k8s")
	}
	if decoded.EffectiveRisk != "critical" {
		t.Errorf("effective_risk = %q, want %q", decoded.EffectiveRisk, "critical")
	}
	if len(decoded.RiskInputs) != 2 {
		t.Fatalf("risk_inputs len = %d, want 2", len(decoded.RiskInputs))
	}
	if decoded.RiskInputs[0].Source != "evidra/native" {
		t.Fatalf("risk_inputs[0].source = %q, want evidra/native", decoded.RiskInputs[0].Source)
	}
	if got := decoded.NativeRiskTags(); len(got) != 1 || got[0] != "k8s.privileged_container" {
		t.Fatalf("NativeRiskTags() = %v, want native-only tag", got)
	}

	// Verify JSON field names.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("raw unmarshal: %v", err)
	}
	for _, key := range []string{"prescription_id", "canonical_action", "risk_inputs", "effective_risk", "ttl_ms", "canon_source"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}
}

func TestPrescriptionPayload_OmitEmpty(t *testing.T) {
	t.Parallel()

	p := PrescriptionPayload{
		PrescriptionID:  "rx_02",
		CanonicalAction: json.RawMessage(`{}`),
		EffectiveRisk:   "low",
		TTLMs:           5000,
		CanonSource:     "generic",
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("raw unmarshal: %v", err)
	}
	if _, ok := raw["risk_inputs"]; ok {
		t.Error("risk_inputs should be omitted when empty")
	}
	if _, ok := raw["risk_level"]; ok {
		t.Error("legacy risk_level should be omitted when unset")
	}
}

func TestPrescriptionPayload_NativeRiskTags_ReturnsNativeOnly(t *testing.T) {
	t.Parallel()

	p := PrescriptionPayload{
		RiskInputs: []RiskInput{
			{Source: "evidra/native", RiskTags: []string{"k8s.privileged_container"}},
			{Source: "trivy/0.58.0", RiskTags: []string{"trivy.DS002"}},
		},
	}
	got := p.NativeRiskTags()
	if len(got) != 1 || got[0] != "k8s.privileged_container" {
		t.Fatalf("NativeRiskTags() = %v, want native-only tags", got)
	}
}

func TestPrescriptionPayload_NativeRiskTags_NoNativeInput(t *testing.T) {
	t.Parallel()

	p := PrescriptionPayload{
		RiskInputs: []RiskInput{
			{Source: "trivy/0.58.0", RiskTags: []string{"trivy.DS002"}},
		},
	}
	if got := p.NativeRiskTags(); got != nil {
		t.Fatalf("NativeRiskTags() = %v, want nil", got)
	}
}

func TestPrescriptionPayload_NativeRiskTags_EmptyInputs(t *testing.T) {
	t.Parallel()

	p := PrescriptionPayload{}
	if got := p.NativeRiskTags(); got != nil {
		t.Fatalf("NativeRiskTags() = %v, want nil", got)
	}
}

func TestReportPayload_Marshal(t *testing.T) {
	t.Parallel()

	exitCode := 0
	original := ReportPayload{
		ReportID:       "rpt_01XYZ",
		PrescriptionID: "rx_01ABC",
		ExitCode:       &exitCode,
		Verdict:        VerdictSuccess,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("ReportPayload marshal: %v", err)
	}

	var decoded ReportPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("ReportPayload unmarshal: %v", err)
	}

	if decoded.ReportID != "rpt_01XYZ" {
		t.Errorf("report_id = %q, want %q", decoded.ReportID, "rpt_01XYZ")
	}
	if decoded.PrescriptionID != "rx_01ABC" {
		t.Errorf("prescription_id = %q, want %q", decoded.PrescriptionID, "rx_01ABC")
	}
	if decoded.ExitCode == nil || *decoded.ExitCode != 0 {
		t.Errorf("exit_code = %v, want 0", decoded.ExitCode)
	}
	if decoded.Verdict != VerdictSuccess {
		t.Errorf("verdict = %q, want %q", decoded.Verdict, VerdictSuccess)
	}

	// Verify JSON has "verdict":"success".
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("raw unmarshal: %v", err)
	}
	var v string
	if err := json.Unmarshal(raw["verdict"], &v); err != nil {
		t.Fatalf("unmarshal verdict: %v", err)
	}
	if v != "success" {
		t.Errorf("JSON verdict = %q, want %q", v, "success")
	}
}

func TestReportPayload_DeclinedMarshalOmitsExitCode(t *testing.T) {
	t.Parallel()

	original := ReportPayload{
		ReportID:       "rpt_01XYZ",
		PrescriptionID: "rx_01ABC",
		Verdict:        VerdictDeclined,
		DecisionContext: &DecisionContext{
			Trigger: "risk_threshold_exceeded",
			Reason:  "risk_level=critical and blast_radius covers production namespace",
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("ReportPayload marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("raw unmarshal: %v", err)
	}
	if _, ok := raw["exit_code"]; ok {
		t.Fatal("declined payload should omit exit_code")
	}
	if _, ok := raw["decision_context"]; !ok {
		t.Fatal("declined payload must include decision_context")
	}
}

func TestFindingPayload_Marshal(t *testing.T) {
	t.Parallel()

	original := FindingPayload{
		Tool:        "trivy",
		ToolVersion: "0.50.1",
		RuleID:      "CVE-2024-1234",
		Severity:    "critical",
		Resource:    "docker.io/nginx:latest",
		Message:     "Known vulnerability in libssl",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("FindingPayload marshal: %v", err)
	}

	var decoded FindingPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("FindingPayload unmarshal: %v", err)
	}

	if decoded.Tool != "trivy" {
		t.Errorf("tool = %q, want %q", decoded.Tool, "trivy")
	}
	if decoded.ToolVersion != "0.50.1" {
		t.Errorf("tool_version = %q, want %q", decoded.ToolVersion, "0.50.1")
	}
	if decoded.RuleID != "CVE-2024-1234" {
		t.Errorf("rule_id = %q, want %q", decoded.RuleID, "CVE-2024-1234")
	}

	// Verify tool_version appears in JSON output.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("raw unmarshal: %v", err)
	}
	if _, ok := raw["tool_version"]; !ok {
		t.Error("missing JSON key tool_version")
	}
}

func TestFindingPayload_ToolVersionOmitEmpty(t *testing.T) {
	t.Parallel()

	p := FindingPayload{
		Tool:     "trivy",
		RuleID:   "CVE-2024-1234",
		Severity: "critical",
		Resource: "docker.io/nginx:latest",
		Message:  "Known vulnerability in libssl",
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("raw unmarshal: %v", err)
	}
	if _, ok := raw["tool_version"]; ok {
		t.Error("tool_version should be omitted when empty")
	}
}

func TestSignalPayload_Marshal(t *testing.T) {
	t.Parallel()

	original := SignalPayload{
		SignalName: "retry_loop",
		SubSignal:  "identical_retry",
		EntryRefs:  []string{"entry_01", "entry_02", "entry_03"},
		Details:    "3 identical retries within 60s",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("SignalPayload marshal: %v", err)
	}

	var decoded SignalPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("SignalPayload unmarshal: %v", err)
	}

	if decoded.SignalName != "retry_loop" {
		t.Errorf("signal_name = %q, want %q", decoded.SignalName, "retry_loop")
	}
	if len(decoded.EntryRefs) != 3 {
		t.Errorf("entry_refs len = %d, want 3", len(decoded.EntryRefs))
	}
}

func TestSignalPayload_OmitEmpty(t *testing.T) {
	t.Parallel()

	p := SignalPayload{
		SignalName: "blast_radius",
		EntryRefs:  []string{"entry_01"},
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("raw unmarshal: %v", err)
	}
	if _, ok := raw["sub_signal"]; ok {
		t.Error("sub_signal should be omitted when empty")
	}
	if _, ok := raw["details"]; ok {
		t.Error("details should be omitted when empty")
	}
}

func TestCanonFailurePayload_Marshal(t *testing.T) {
	t.Parallel()

	original := CanonFailurePayload{
		ErrorCode:    "PARSE_ERROR",
		ErrorMessage: "invalid YAML at line 42",
		Adapter:      "k8s",
		RawDigest:    "sha256:abcdef1234567890",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("CanonFailurePayload marshal: %v", err)
	}

	var decoded CanonFailurePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("CanonFailurePayload unmarshal: %v", err)
	}

	if decoded.ErrorCode != "PARSE_ERROR" {
		t.Errorf("error_code = %q, want %q", decoded.ErrorCode, "PARSE_ERROR")
	}
	if decoded.Adapter != "k8s" {
		t.Errorf("adapter = %q, want %q", decoded.Adapter, "k8s")
	}
}

func TestVerdictFromExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code int
		want Verdict
	}{
		{name: "zero_success", code: 0, want: VerdictSuccess},
		{name: "one_failure", code: 1, want: VerdictFailure},
		{name: "negative_error", code: -1, want: VerdictError},
		{name: "signal_kill_failure", code: 137, want: VerdictFailure},
		{name: "negative_two_error", code: -2, want: VerdictError},
		{name: "exit_two_failure", code: 2, want: VerdictFailure},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := VerdictFromExitCode(tt.code)
			if got != tt.want {
				t.Errorf("VerdictFromExitCode(%d) = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestVerdict_Valid(t *testing.T) {
	t.Parallel()

	cases := map[Verdict]bool{
		VerdictSuccess:  true,
		VerdictFailure:  true,
		VerdictError:    true,
		VerdictDeclined: true,
		Verdict("nope"): false,
	}

	for verdict, want := range cases {
		if got := verdict.Valid(); got != want {
			t.Fatalf("Valid(%q) = %t, want %t", verdict, got, want)
		}
	}
}
