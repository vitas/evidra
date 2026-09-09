package signal

import "testing"

func TestDetectUnprescribedMutations(t *testing.T) {
	entries := []Entry{
		{EventID: "p1", IsPrescription: true},                    // model claim
		{EventID: "r1", IsReport: true, PrescriptionID: "p1"},    // linked report
		{EventID: "p2", IsPrescription: true, AutoPrescribed: true},
		{EventID: "r2", IsReport: true, PrescriptionID: "p2"},
		{EventID: "p3", IsPrescription: true, AutoPrescribed: true},
	}
	got := DetectUnprescribedMutations(entries)
	if got.Name != "unprescribed_mutation" {
		t.Fatalf("signal name = %q", got.Name)
	}
	if got.Count != 2 {
		t.Fatalf("count = %d, want 2", got.Count)
	}
	if len(got.EventIDs) != 2 || got.EventIDs[0] != "p2" || got.EventIDs[1] != "p3" {
		t.Fatalf("event ids = %v, want [p2 p3]", got.EventIDs)
	}
}

func TestDetectUnprescribedMutationsEmpty(t *testing.T) {
	got := DetectUnprescribedMutations(nil)
	if got.Count != 0 || len(got.EventIDs) != 0 {
		t.Fatalf("empty input should yield zero signal, got %+v", got)
	}
}

func TestUnprescribedMutationIsRegistered(t *testing.T) {
	for _, name := range RegisteredSignalNames() {
		if name == "unprescribed_mutation" {
			return
		}
	}
	t.Fatalf("detector not registered: %v", RegisteredSignalNames())
}
