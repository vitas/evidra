package argo

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	argocdapiclient "github.com/argoproj/argo-cd/v2/pkg/apiclient"
	argocdapplication "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	argocdv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	argoio "github.com/argoproj/argo-cd/v2/util/io"
	synccommon "github.com/argoproj/gitops-engine/pkg/sync/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewArgoCDClientFetcher(opts BackendOptions) (FetchFunc, error) {
	serverAddr, err := normalizeArgoServerAddr(opts.URL)
	if err != nil {
		return nil, err
	}
	client, err := argocdapiclient.NewClient(&argocdapiclient.ClientOptions{
		ServerAddr: serverAddr,
		AuthToken:  strings.TrimSpace(opts.Token),
		Insecure:   true,
		GRPCWeb:    true,
	})
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context) ([]SourceEvent, error) {
		conn, appClient, err := client.NewApplicationClient()
		if err != nil {
			return nil, err
		}
		defer argoio.Close(conn)

		list, err := appClient.List(ctx, &argocdapplication.ApplicationQuery{})
		if err != nil {
			return nil, err
		}
		out := make([]SourceEvent, 0, len(list.Items))
		for _, app := range list.Items {
			out = append(out, toSourceEvents(app)...)
		}
		return out, nil
	}, nil
}

func normalizeArgoServerAddr(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("argo api url is empty")
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", err
		}
		host := strings.TrimSpace(u.Host)
		if host == "" {
			return "", fmt.Errorf("invalid argo api url")
		}
		return host, nil
	}
	return raw, nil
}

func toSourceEvents(app argocdv1alpha1.Application) []SourceEvent {
	out := make([]SourceEvent, 0, 8)
	appUID := strings.TrimSpace(string(app.UID))
	appName := strings.TrimSpace(app.Name)
	revision := app.Status.Sync.Revision
	if revision == "" && len(app.Status.Sync.Revisions) > 0 {
		revision = app.Status.Sync.Revisions[0]
	}
	cluster := strings.TrimSpace(app.Spec.Destination.Server)
	if cluster == "" {
		cluster = "unknown"
	}
	namespace := strings.TrimSpace(app.Spec.Destination.Namespace)
	actor := resolveActor(app)
	annotations := evidraAnnotations(app.Annotations)
	terminalRevision, terminalStartedAt, terminalFinishedAt, hasTerminal := terminalOperationWindow(app, revision)

	for _, h := range app.Status.History {
		occurred := historyOccurredAt(h, app)
		hRevision := strings.TrimSpace(h.Revision)
		if hRevision == "" {
			hRevision = strings.TrimSpace(revision)
		}
		if hasTerminal && sameRevisionWindow(hRevision, terminalRevision, occurred, terminalStartedAt, terminalFinishedAt) {
			continue
		}
		id := deterministicHistoryID(appUID, h.ID, hRevision, occurred)
		opKey := historyOperationKey(h.ID, hRevision, occurred)
		payload := map[string]interface{}{
			"history_id":    h.ID,
			"sync_revision": hRevision,
		}
		if len(annotations) > 0 {
			payload["annotations"] = annotations
		}
		out = append(out, SourceEvent{
			ID:           id,
			AppUID:       appUID,
			App:          appName,
			Cluster:      cluster,
			Namespace:    namespace,
			Revision:     hRevision,
			Occurred:     occurred,
			Actor:        actor,
			EventType:    "argo.deployment.recorded",
			Result:       "Recorded",
			HistoryID:    h.ID,
			OperationKey: opKey,
			Payload:      payload,
		})
	}

	if app.Status.OperationState != nil {
		phase := strings.TrimSpace(string(app.Status.OperationState.Phase))
		startedAt := app.Status.OperationState.StartedAt.Time.UTC()
		finishedAt := timeFromMeta(app.Status.OperationState.FinishedAt)
		if finishedAt.IsZero() {
			finishedAt = startedAt
		}
		if finishedAt.IsZero() {
			finishedAt = mostRecentTime(app)
		}
		opKey := operationKey(revision, startedAt, finishedAt)
		switch app.Status.OperationState.Phase {
		case synccommon.OperationRunning, synccommon.OperationTerminating:
			if !startedAt.IsZero() {
				payload := map[string]interface{}{
					"operation_phase": phase,
				}
				if len(annotations) > 0 {
					payload["annotations"] = annotations
				}
				out = append(out, SourceEvent{
					ID:           deterministicOperationID(appUID, opKey, "start"),
					AppUID:       appUID,
					App:          appName,
					Cluster:      cluster,
					Namespace:    namespace,
					Revision:     strings.TrimSpace(revision),
					Occurred:     startedAt,
					Actor:        actor,
					EventType:    "argo.sync.started",
					Result:       phase,
					OperationKey: opKey,
					Payload:      payload,
				})
			}
		case synccommon.OperationSucceeded, synccommon.OperationFailed, synccommon.OperationError:
			payload := map[string]interface{}{
				"operation_phase": phase,
			}
			if len(annotations) > 0 {
				payload["annotations"] = annotations
			}
			out = append(out, SourceEvent{
				ID:           deterministicOperationID(appUID, opKey, "finish"),
				AppUID:       appUID,
				App:          appName,
				Cluster:      cluster,
				Namespace:    namespace,
				Revision:     strings.TrimSpace(revision),
				Occurred:     finishedAt,
				Actor:        actor,
				EventType:    "argo.sync.finished",
				Result:       phase,
				OperationKey: opKey,
				Payload:      payload,
			})
		}
	}

	health := strings.TrimSpace(string(app.Status.Health.Status))
	if health != "" {
		occurred := timeFromMeta(app.Status.ReconciledAt)
		if occurred.IsZero() {
			occurred = time.Now().UTC()
		}
		payload := map[string]interface{}{
			"health_status": health,
			"observed_at":   occurred.UTC().Format(time.RFC3339Nano),
		}
		if len(annotations) > 0 {
			payload["annotations"] = annotations
		}
		out = append(out, SourceEvent{
			ID:           deterministicHealthID(appUID, health, occurred),
			AppUID:       appUID,
			App:          appName,
			Cluster:      cluster,
			Namespace:    namespace,
			Revision:     strings.TrimSpace(revision),
			Occurred:     occurred,
			Actor:        "argocd",
			EventType:    "argo.health.changed",
			HealthStatus: health,
			Payload:      payload,
		})
	}
	return out
}

func evidraAnnotations(annotations map[string]string) map[string]string {
	if len(annotations) == 0 {
		return nil
	}
	keys := []string{
		"evidra.io/change-id",
		"evidra.io/ticket",
		"evidra.io/approvals-ref",
		"evidra.io/approvals-json",
	}
	out := map[string]string{}
	for _, k := range keys {
		if v := strings.TrimSpace(annotations[k]); v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func resolveActor(app argocdv1alpha1.Application) string {
	if app.Status.OperationState != nil {
		if user := strings.TrimSpace(app.Status.OperationState.Operation.InitiatedBy.Username); user != "" {
			return user
		}
		if app.Status.OperationState.Operation.InitiatedBy.Automated {
			return "argocd-automated"
		}
	}
	return "argocd"
}

func deterministicHistoryID(appUID string, historyID int64, revision string, occurred time.Time) string {
	appUID = strings.TrimSpace(appUID)
	if historyID > 0 {
		return appUID + ":hist:" + strconv.FormatInt(historyID, 10)
	}
	return appUID + ":hist:" + strings.TrimSpace(revision) + ":" + trimTimeKey(occurred)
}

func deterministicOperationID(appUID, opKey, suffix string) string {
	return strings.TrimSpace(appUID) + ":op:" + strings.TrimSpace(opKey) + ":" + strings.TrimSpace(suffix)
}

func deterministicHealthID(appUID, health string, occurred time.Time) string {
	return strings.TrimSpace(appUID) + ":health:" + strings.ToLower(strings.TrimSpace(health)) + ":" + trimTimeKey(occurred)
}

func operationKey(revision string, startedAt, finishedAt time.Time) string {
	revision = strings.TrimSpace(revision)
	if revision == "" {
		revision = "unknown"
	}
	if !startedAt.IsZero() {
		return revision + ":" + strconv.FormatInt(startedAt.UTC().UnixNano(), 10)
	}
	if !finishedAt.IsZero() {
		return revision + ":" + strconv.FormatInt(finishedAt.UTC().UnixNano(), 10)
	}
	return revision + ":unknown"
}

func historyOperationKey(historyID int64, revision string, occurred time.Time) string {
	if historyID > 0 {
		return "hist:" + strconv.FormatInt(historyID, 10)
	}
	return operationKey(revision, time.Time{}, occurred)
}

func historyOccurredAt(history argocdv1alpha1.RevisionHistory, app argocdv1alpha1.Application) time.Time {
	if !history.DeployedAt.IsZero() {
		return history.DeployedAt.Time.UTC()
	}
	if history.DeployStartedAt != nil && !history.DeployStartedAt.IsZero() {
		return history.DeployStartedAt.Time.UTC()
	}
	return mostRecentTime(app)
}

func mostRecentTime(app argocdv1alpha1.Application) time.Time {
	choices := []time.Time{
		timeFromMeta(app.Status.ReconciledAt),
	}
	if app.Status.OperationState != nil {
		choices = append(choices, timeFromMeta(app.Status.OperationState.FinishedAt))
		choices = append(choices, app.Status.OperationState.StartedAt.Time.UTC())
	}
	if len(app.Status.History) > 0 {
		last := app.Status.History[len(app.Status.History)-1]
		choices = append(choices, last.DeployedAt.Time.UTC())
		if last.DeployStartedAt != nil {
			choices = append(choices, last.DeployStartedAt.Time.UTC())
		}
	}
	latest := time.Time{}
	for _, ts := range choices {
		if ts.IsZero() {
			continue
		}
		if ts.After(latest) {
			latest = ts
		}
	}
	if latest.IsZero() {
		return time.Now().UTC()
	}
	return latest.UTC()
}

func timeFromMeta(ts *metav1.Time) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.Time.UTC()
}

func terminalOperationWindow(app argocdv1alpha1.Application, revision string) (rev string, startedAt time.Time, finishedAt time.Time, ok bool) {
	if app.Status.OperationState == nil {
		return "", time.Time{}, time.Time{}, false
	}
	phase := app.Status.OperationState.Phase
	switch phase {
	case synccommon.OperationSucceeded, synccommon.OperationFailed, synccommon.OperationError:
	default:
		return "", time.Time{}, time.Time{}, false
	}
	rev = strings.TrimSpace(revision)
	if rev == "" {
		rev = strings.TrimSpace(app.Status.Sync.Revision)
	}
	startedAt = app.Status.OperationState.StartedAt.Time.UTC()
	finishedAt = timeFromMeta(app.Status.OperationState.FinishedAt)
	if finishedAt.IsZero() {
		finishedAt = startedAt
	}
	return rev, startedAt, finishedAt, true
}

func sameRevisionWindow(historyRevision, terminalRevision string, historyOccurred, terminalStartedAt, terminalFinishedAt time.Time) bool {
	if strings.TrimSpace(historyRevision) == "" || strings.TrimSpace(terminalRevision) == "" {
		return false
	}
	if strings.TrimSpace(historyRevision) != strings.TrimSpace(terminalRevision) {
		return false
	}
	if historyOccurred.IsZero() || terminalStartedAt.IsZero() || terminalFinishedAt.IsZero() {
		return true
	}
	const jitter = 2 * time.Minute
	start := terminalStartedAt.Add(-jitter)
	end := terminalFinishedAt.Add(jitter)
	return !historyOccurred.Before(start) && !historyOccurred.After(end)
}

func trimTimeKey(ts time.Time) string {
	if ts.IsZero() {
		return "0"
	}
	return strconv.FormatInt(ts.UTC().Unix(), 10)
}
