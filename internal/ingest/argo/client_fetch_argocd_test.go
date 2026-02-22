package argo

import (
	"testing"
	"time"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestToSourceEventsFromApplication(t *testing.T) {
	startedAt := time.Date(2026, 2, 16, 11, 58, 0, 123456789, time.UTC)
	finishedAt := time.Date(2026, 2, 16, 11, 59, 0, 987654321, time.UTC)
	reconciledAt := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	app := argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			UID:  "uid-1",
			Name: "payments-api",
		},
		Spec: argocdv1alpha1.ApplicationSpec{
			Destination: argocdv1alpha1.ApplicationDestination{
				Server:    "https://kubernetes.default.svc",
				Namespace: "prod-eu",
			},
		},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Revision: "abc123",
				Status:   argocdv1alpha1.SyncStatusCodeSynced,
			},
			History: argocdv1alpha1.RevisionHistories{
				{
					ID:         11,
					Revision:   "abc123",
					DeployedAt: metav1.NewTime(finishedAt),
				},
			},
			OperationState: &argocdv1alpha1.OperationState{
				Phase:     "Succeeded",
				StartedAt: metav1.NewTime(startedAt),
				FinishedAt: &metav1.Time{
					Time: finishedAt,
				},
			},
			Health: argocdv1alpha1.HealthStatus{
				Status: "Healthy",
			},
			ReconciledAt: &metav1.Time{Time: reconciledAt},
		},
	}

	events := toSourceEvents(app)
	if len(events) == 0 {
		t.Fatalf("expected source events")
	}
	var finishedCount int
	var historyCount int
	var healthCount int
	for _, se := range events {
		switch se.EventType {
		case "argo.sync.finished":
			finishedCount++
			if se.HistoryID != 0 {
				t.Fatalf("terminal operation event must not carry history id")
			}
		case "argo.deployment.recorded":
			historyCount++
		case "argo.health.changed":
			healthCount++
			if !se.Occurred.Equal(reconciledAt) {
				t.Fatalf("health event should use reconciledAt, got %s", se.Occurred)
			}
		}
	}
	if finishedCount != 1 {
		t.Fatalf("expected single terminal sync.finished event, got %d", finishedCount)
	}
	if historyCount != 0 {
		t.Fatalf("expected overlapping history entry to be skipped, got %d", historyCount)
	}
	if healthCount != 1 {
		t.Fatalf("expected single health event, got %d", healthCount)
	}
}

func TestToSourceEventsHistoryBackfillWhenOperationStateMissing(t *testing.T) {
	app := argocdv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{
			UID:  "uid-2",
			Name: "orders-api",
		},
		Spec: argocdv1alpha1.ApplicationSpec{
			Destination: argocdv1alpha1.ApplicationDestination{
				Server:    "https://kubernetes.default.svc",
				Namespace: "prod-us",
			},
		},
		Status: argocdv1alpha1.ApplicationStatus{
			Sync: argocdv1alpha1.SyncStatus{
				Revision: "def456",
			},
			History: argocdv1alpha1.RevisionHistories{
				{
					ID:         42,
					Revision:   "def456",
					DeployedAt: metav1.NewTime(time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)),
				},
			},
		},
	}

	events := toSourceEvents(app)
	var recordedCount int
	for _, se := range events {
		if se.EventType == "argo.deployment.recorded" {
			recordedCount++
			if se.HistoryID != 42 {
				t.Fatalf("unexpected history id: %d", se.HistoryID)
			}
		}
	}
	if recordedCount != 1 {
		t.Fatalf("expected one history backfill event, got %d", recordedCount)
	}
}

func TestOperationKeyUsesNanosecondPrecision(t *testing.T) {
	revision := "abc123"
	started := time.Date(2026, 2, 16, 12, 0, 0, 123000000, time.UTC)
	finished := started.Add(200 * time.Millisecond)

	k1 := operationKey(revision, started, finished)
	k2 := operationKey(revision, started.Add(1*time.Nanosecond), finished)
	if k1 == k2 {
		t.Fatalf("expected distinct operation keys at nanosecond granularity")
	}
	if operationKey(revision, time.Time{}, time.Time{}) != "abc123:unknown" {
		t.Fatalf("expected deterministic unknown fallback key")
	}
}
