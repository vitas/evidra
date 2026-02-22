package kubernetes

import (
	"testing"
	"time"

	ce "evidra/internal/cloudevents"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestNormalizeEventMapsFields(t *testing.T) {
	c := NewCollector(Config{
		Interval:    15 * time.Second,
		Namespace:   "prod",
		Cluster:     "eu-1",
		Environment: "prod-eu",
	}, nil, nil, logr.Discard())

	ts := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	e := corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "payments-api.1",
			Namespace:       "prod",
			UID:             types.UID("uid-1"),
			ResourceVersion: "42",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "payments-api-7f8d9",
			Namespace: "prod",
		},
		Reason:              "BackOff",
		Type:                "Warning",
		ReportingController: "kubelet",
		LastTimestamp:       metav1.NewTime(ts),
	}

	got := c.normalizeEvent(e)
	if got.ID != "evt_k8s_uid-1_42" {
		t.Fatalf("unexpected id: %s", got.ID)
	}
	if got.Source != "kubernetes" {
		t.Fatalf("unexpected source: %s", got.Source)
	}
	if got.Type != "k8s.incident.signal" {
		t.Fatalf("unexpected type: %s", got.Type)
	}
	if got.Subject != "payments-api-7f8d9" {
		t.Fatalf("unexpected subject: %s", got.Subject)
	}
	if ce.ExtensionString(got.Extensions, "namespace") != "prod-eu" {
		t.Fatalf("unexpected namespace: %s", ce.ExtensionString(got.Extensions, "namespace"))
	}
	if ce.ExtensionString(got.Extensions, "cluster") != "eu-1" {
		t.Fatalf("unexpected cluster: %s", ce.ExtensionString(got.Extensions, "cluster"))
	}
	if ce.ExtensionString(got.Extensions, "initiator") != "kubelet" {
		t.Fatalf("unexpected initiator: %s", ce.ExtensionString(got.Extensions, "initiator"))
	}
	if !got.Time.Equal(ts) {
		t.Fatalf("unexpected time: %s", got.Time)
	}
	if !ce.ExtensionBool(got.Extensions, "supporting_observation") {
		t.Fatalf("expected supporting_observation to be true")
	}
}

func TestEventTimestampPriority(t *testing.T) {
	now := time.Now().UTC()
	older := now.Add(-10 * time.Minute)
	newer := now.Add(-2 * time.Minute)

	e := corev1.Event{
		EventTime:     metav1.NewMicroTime(newer),
		LastTimestamp: metav1.NewTime(older),
		FirstTimestamp: metav1.NewTime(
			older.Add(-time.Minute),
		),
	}
	if got := eventTimestamp(e); !got.Equal(newer) {
		t.Fatalf("expected event time first, got %s", got)
	}

	e = corev1.Event{
		LastTimestamp: metav1.NewTime(newer),
		FirstTimestamp: metav1.NewTime(
			older,
		),
	}
	if got := eventTimestamp(e); !got.Equal(newer) {
		t.Fatalf("expected last timestamp, got %s", got)
	}

	e = corev1.Event{
		FirstTimestamp: metav1.NewTime(older),
	}
	if got := eventTimestamp(e); !got.Equal(older) {
		t.Fatalf("expected first timestamp, got %s", got)
	}
}
