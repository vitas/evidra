// gen_external_bundle builds the canonical conformance fixture at
// tests/external_bundle_v1 using only the public evidence package API.
//
// Run once per protocol change:
//
//	go run ./scripts/gen_external_bundle
//
// The committed fixture, not this generator, is the contract: signatures
// depend on wall-clock timestamps, so output is stable in structure but not
// byte-for-byte reproducible. Regenerating re-keys the bundle.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/version"
)

const outDir = "tests/external_bundle_v1"

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen_external_bundle:", err)
		os.Exit(1)
	}
}

func main() {
	if err := os.RemoveAll(outDir); err != nil {
		must(fmt.Errorf("clean %s: %w", outDir, err))
	}
	must(os.MkdirAll(outDir, 0o755))

	signer, err := evidence.NewEphemeralSigner()
	must(err)

	actor := evidence.Actor{
		Type:       "agent",
		ID:         "fixture-producer",
		Provenance: "external-bundle",
		Version:    "fixture",
	}
	meta := func(kind evidence.EvidenceKind) *evidence.EvidenceMetadata {
		return &evidence.EvidenceMetadata{Kind: kind}
	}
	src := &evidence.SourceMetadata{System: "gen_external_bundle"}

	prescriptionID := "01EXTERNALPRESCRIPTION00000001"
	exitCode := 0
	reportPayload := evidence.ReportPayload{
		ReportID:       "01EXTERNALREPORT000000000001",
		PrescriptionID: prescriptionID,
		ExitCode:       &exitCode,
		Verdict:        evidence.VerdictSuccess,
		Flavor:         evidence.FlavorImperative,
		Evidence:       meta(evidence.EvidenceKindObserved),
		Source:         src,
	}
	reportRaw, err := json.Marshal(reportPayload)
	must(err)

	prescribePayload := evidence.PrescriptionPayload{
		PrescriptionID: prescriptionID,
		Intent: &evidence.DeclaredIntent{
			Tool:      "kubectl",
			Operation: "rollout-restart",
			Target:    "deployment/checkouts",
			Command:   "kubectl rollout restart deployment/checkouts -n shop",
		},
		EffectiveRisk: "medium",
		TTLMs:         evidence.DefaultTTLMs,
		CanonSource:   "external",
		Flavor:        evidence.FlavorImperative,
		Evidence:      meta(evidence.EvidenceKindDeclared),
		Source:        src,
	}
	prescribeRaw, err := json.Marshal(prescribePayload)
	must(err)

	sessionStartRaw, err := json.Marshal(evidence.SessionStartPayload{
		Labels: map[string]string{"scenario": "external-bundle-conformance"},
	})
	must(err)
	sessionEndRaw, err := json.Marshal(evidence.SessionEndPayload{Status: "completed"})
	must(err)

	fixtureEntries := []struct {
		entryID string
		typ     evidence.EntryType
		payload json.RawMessage
	}{
		{"01EXTERNALBUNDLEFIXTURE000SST1", evidence.EntryTypeSessionStart, sessionStartRaw},
		{"01EXTERNALBUNDLEFIXTURE000PRS1", evidence.EntryTypePrescribe, prescribeRaw},
		{"01EXTERNALBUNDLEFIXTURE000RPT1", evidence.EntryTypeReport, reportRaw},
		{"01EXTERNALBUNDLEFIXTURE000SST2", evidence.EntryTypeSessionEnd, sessionEndRaw},
	}

	var prev string
	for _, fe := range fixtureEntries {
		entry, err := evidence.BuildEntry(evidence.EntryBuildParams{
			EntryID:      fe.entryID,
			Type:         fe.typ,
			SessionID:    "01EXTERNALBUNDLESESSION00001",
			OperationID:  "01EXTERNALBUNDLEOPERATI0001",
			TraceID:      "external-bundle-trace",
			Actor:        actor,
			Payload:      fe.payload,
			PreviousHash: prev,
			ScopeDimensions: map[string]string{
				"environment": "conformance",
				"workspace":   "fixture",
			},
			SpecVersion: version.SpecVersion,
			Signer:      signer,
		})
		must(err)
		must(evidence.AppendEntryAtPath(outDir, entry))
		prev = entry.Hash
	}

	must(evidence.SaveBundleManifest(outDir, evidence.BundleManifest{
		Spec: evidence.BundleSpecV1,
		Producer: evidence.BundleProducer{
			Name:    "evidra-gen_external_bundle",
			Version: version.Version,
		},
		TrustLevel: evidence.TrustEphemeral,
		PublicKey:  signer.PublicKeyBase64(),
		Notes:      "Conformance fixture for external evidence bundles. Validate with: bin/evidra validate --evidence-dir tests/external_bundle_v1",
	}))

	// The store lock is transient; a published bundle must not ship it.
	_ = os.Remove(filepath.Join(outDir, ".evidra.lock"))

	fmt.Println("generated", filepath.Clean(outDir))
}
