package mcpserver

import (
	"samebits.com/evidra/internal/lifecycle"
	"samebits.com/evidra/pkg/evidence"
)

func toRetryRecorder(rt *RetryTracker) lifecycle.RetryRecorder {
	var rec lifecycle.RetryRecorder
	if rt != nil {
		rec = rt
	}
	return rec
}

func toEvidenceActor(actor InputActor) evidence.Actor {
	skillVersion := actor.SkillVersion
	if skillVersion == "" {
		skillVersion = contractSkillVersion
	}
	return evidence.Actor{
		Type:         actor.Type,
		ID:           actor.ID,
		Provenance:   actor.Origin,
		InstanceID:   actor.InstanceID,
		Version:      actor.Version,
		SkillVersion: skillVersion,
	}
}

func toLifecyclePrescribeInput(input PrescribeInput) lifecycle.PrescribeInput {
	intent := evidence.DeclaredIntent{}
	if input.CanonicalAction == nil {
		intent = evidence.DeclaredIntent{
			Tool:      input.Tool,
			Operation: input.Operation,
			Target:    declaredIntentTarget(input),
		}
	}
	return lifecycle.PrescribeInput{
		Actor:           toEvidenceActor(input.Actor),
		Intent:          intent,
		Tool:            input.Tool,
		Operation:       input.Operation,
		RawArtifact:     []byte(input.RawArtifact),
		Environment:     input.Environment,
		CanonicalAction: input.CanonicalAction,
		SessionID:       input.SessionID,
		OperationID:     input.OperationID,
		Attempt:         input.Attempt,
		TraceID:         input.TraceID,
		SpanID:          input.SpanID,
		ParentSpanID:    input.ParentSpanID,
		ScopeDimensions: input.ScopeDimensions,
		Flavor:          evidence.FlavorImperative,
		EvidenceKind:    evidence.EvidenceKindDeclared,
		SourceSystem:    "mcp",
		AutoPrescribed:  input.AutoPrescribed,
	}
}

func declaredIntentTarget(input PrescribeInput) string {
	if input.Resource == "" {
		return ""
	}
	if input.Namespace == "" {
		return input.Resource
	}
	return input.Namespace + "/" + input.Resource
}

func toLifecycleReportInput(input ReportInput) lifecycle.ReportInput {
	return lifecycle.ReportInput{
		PrescriptionID:  input.PrescriptionID,
		Verdict:         input.Verdict,
		ExitCode:        input.ExitCode,
		DecisionContext: input.DecisionContext,
		ArtifactDigest:  input.ArtifactDigest,
		Actor:           toEvidenceActor(input.Actor),
		ExternalRefs:    input.ExternalRefs,
		SessionID:       input.SessionID,
		OperationID:     input.OperationID,
		SpanID:          input.SpanID,
		ParentSpanID:    input.ParentSpanID,
		Flavor:          evidence.FlavorImperative,
		EvidenceKind:    evidence.EvidenceKindDeclared,
		SourceSystem:    "mcp",
	}
}
