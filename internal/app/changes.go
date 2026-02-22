package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/store"
)

type ChangeQuery struct {
	Subject               ParsedSubject
	From                  time.Time
	To                    time.Time
	Q                     string
	ResultStatus          string
	HealthStatus          string
	ExternalChangeID      string
	ExternalChangeIDState string
	TicketID              string
	TicketIDState         string
	ApprovalReference     string
	HasApprovals          string
	Limit                 int
	Cursor                string
}

type Change struct {
	ID                      string                `json:"id"`
	ChangeID                string                `json:"change_id"`
	Permalink               string                `json:"permalink,omitempty"`
	Subject                 string                `json:"subject"`
	Application             string                `json:"application"`
	Project                 string                `json:"project,omitempty"`
	TargetCluster           string                `json:"target_cluster"`
	Namespace               string                `json:"namespace"`
	PrimaryProvider         string                `json:"primary_provider"`
	PrimaryReference        string                `json:"primary_reference,omitempty"`
	Revision                string                `json:"revision,omitempty"`
	Initiator               string                `json:"initiator,omitempty"`
	ExternalChangeID        string                `json:"external_change_id,omitempty"`
	TicketID                string                `json:"ticket_id,omitempty"`
	ApprovalReference       string                `json:"approval_reference,omitempty"`
	ResultStatus            string                `json:"result_status"`
	HealthStatus            string                `json:"health_status"`
	HealthAtOperationStart  string                `json:"health_at_operation_start"`
	HealthAfterDeploy       string                `json:"health_after_deploy"`
	PostDeployDegradation   PostDeployDegradation `json:"post_deploy_degradation"`
	EvidenceLastUpdatedAt   time.Time             `json:"evidence_last_updated_at"`
	EvidenceWindowSeconds   int                   `json:"evidence_window_seconds,omitempty"`
	EvidenceMayBeIncomplete bool                  `json:"evidence_may_be_incomplete"`
	HasApprovals            bool                  `json:"has_approvals"`
	StartedAt               time.Time             `json:"started_at"`
	CompletedAt             time.Time             `json:"completed_at"`
	EventCount              int                   `json:"event_count"`
}

type ChangeDetail struct {
	Change
	Events []ce.StoredEvent `json:"events"`
}

type PostDeployDegradation struct {
	Observed       bool       `json:"observed"`
	FirstTimestamp *time.Time `json:"first_timestamp,omitempty"`
}

type ChangeListResult struct {
	Items      []Change `json:"items"`
	NextCursor string   `json:"next_cursor,omitempty"`
}

type ChangeEvidence struct {
	Change                 Change             `json:"change"`
	SupportingObservations []ce.StoredEvent   `json:"supporting_observations"`
	Approvals              []ApprovalEvidence `json:"approvals,omitempty"`
}

type ApprovalEvidence struct {
	Source    string `json:"source,omitempty"`
	Identity  string `json:"identity,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Reference string `json:"reference,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

var ErrInvalidChangeCursor = errors.New("invalid change cursor")

func (s *Service) ListChanges(ctx context.Context, q ChangeQuery) (ChangeListResult, error) {
	events, err := s.eventsForChangeQuery(ctx, q)
	if err != nil {
		return ChangeListResult{}, err
	}
	changes := buildChanges(events)
	changes = filterChanges(changes, q)
	cursorTS, cursorID, err := decodeChangeCursor(q.Cursor)
	if err != nil {
		return ChangeListResult{}, ErrInvalidChangeCursor
	}
	if !cursorTS.IsZero() {
		changes = applyChangeCursor(changes, cursorTS, cursorID)
	}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	if q.Limit > 500 {
		q.Limit = 500
	}
	result := ChangeListResult{Items: changes}
	if len(changes) > q.Limit {
		result.Items = changes[:q.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextCursor = encodeChangeCursor(last.CompletedAt, last.ID)
	}
	return result, nil
}

func (s *Service) GetChange(ctx context.Context, id string, q ChangeQuery) (ChangeDetail, error) {
	events, err := s.eventsForChangeQuery(ctx, q)
	if err != nil {
		return ChangeDetail{}, err
	}
	byID := buildChangesByID(events)
	c, ok := byID[id]
	if !ok {
		return ChangeDetail{}, store.ErrNotFound
	}
	sort.Slice(c.Events, func(i, j int) bool {
		if c.Events[i].Time.Equal(c.Events[j].Time) {
			return c.Events[i].ID < c.Events[j].ID
		}
		return c.Events[i].Time.Before(c.Events[j].Time)
	})
	return c, nil
}

func (s *Service) GetChangeTimeline(ctx context.Context, id string, q ChangeQuery) ([]ce.StoredEvent, error) {
	c, err := s.GetChange(ctx, id, q)
	if err != nil {
		return nil, err
	}
	return c.Events, nil
}

func (s *Service) GetChangeEvidence(ctx context.Context, id string, q ChangeQuery) (ChangeEvidence, error) {
	c, err := s.GetChange(ctx, id, q)
	if err != nil {
		return ChangeEvidence{}, err
	}
	supporting := make([]ce.StoredEvent, 0)
	approvals := make([]ApprovalEvidence, 0)
	seenApprovals := map[string]struct{}{}
	for _, e := range c.Events {
		if ce.ExtensionBool(e.Extensions, "supporting_observation") {
			supporting = append(supporting, e)
		}
		for _, a := range approvalsFromData(e.Data) {
			k := strings.ToLower(strings.TrimSpace(a.Source)) + "|" +
				strings.ToLower(strings.TrimSpace(a.Identity)) + "|" +
				strings.TrimSpace(a.Timestamp) + "|" +
				strings.TrimSpace(a.Reference) + "|" +
				strings.TrimSpace(a.Summary)
			if _, ok := seenApprovals[k]; ok {
				continue
			}
			seenApprovals[k] = struct{}{}
			approvals = append(approvals, a)
		}
	}
	return ChangeEvidence{
		Change:                 c.Change,
		SupportingObservations: supporting,
		Approvals:              approvals,
	}, nil
}

func (s *Service) eventsForChangeQuery(ctx context.Context, q ChangeQuery) ([]ce.StoredEvent, error) {
	res, err := s.repo.QueryTimeline(ctx, store.TimelineQuery{
		Subject:           q.Subject.App,
		Namespace:         q.Subject.Environment,
		Cluster:           q.Subject.Cluster,
		From:              q.From.UTC(),
		To:                q.To.UTC(),
		IncludeSupporting: true,
		Limit:             500,
	})
	if err != nil {
		return nil, err
	}
	events := res.Items
	if strings.TrimSpace(q.Q) != "" {
		events = filterEventsByQuery(events, q.Q)
	}
	return events, nil
}

func filterEventsByQuery(events []ce.StoredEvent, q string) []ce.StoredEvent {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return events
	}
	out := make([]ce.StoredEvent, 0, len(events))
	for _, e := range events {
		if eventMatchesQuery(e, q) {
			out = append(out, e)
		}
	}
	return out
}

func eventMatchesQuery(e ce.StoredEvent, q string) bool {
	if strings.Contains(strings.ToLower(e.ID), q) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Source), q) || strings.Contains(strings.ToLower(e.Type), q) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Subject), q) {
		return true
	}
	for _, v := range e.Extensions {
		if s, ok := v.(string); ok && strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(string(e.Data)), q)
}

func buildChanges(events []ce.StoredEvent) []Change {
	byID := buildChangesByID(events)
	out := make([]Change, 0, len(byID))
	for _, c := range byID {
		out = append(out, c.Change)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CompletedAt.Equal(out[j].CompletedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CompletedAt.After(out[j].CompletedAt)
	})
	return out
}

func buildChangesByID(events []ce.StoredEvent) map[string]ChangeDetail {
	out := make(map[string]ChangeDetail)
	for _, e := range events {
		id, provider, reference, revision := deriveChangeIdentity(e)
		cur, ok := out[id]
		if !ok {
			cluster := ce.ExtensionString(e.Extensions, "cluster")
			namespace := ce.ExtensionString(e.Extensions, "namespace")
			cur = ChangeDetail{
				Change: Change{
					ID:                     id,
					ChangeID:               id,
					Permalink:              "/ui/explorer/change/" + id,
					Subject:                e.Subject,
					Application:            e.Subject,
					TargetCluster:          cluster,
					Namespace:              namespace,
					PrimaryProvider:        provider,
					PrimaryReference:       reference,
					Revision:               revision,
					StartedAt:              e.Time.UTC(),
					CompletedAt:            e.Time.UTC(),
					ResultStatus:           "unknown",
					HealthStatus:           "unknown",
					HealthAtOperationStart: "unknown",
					HealthAfterDeploy:      "unknown",
				},
			}
		}
		externalChangeID, ticketID, approvalRef := externalCorrelationFromEvent(e)
		if cur.ExternalChangeID == "" {
			cur.ExternalChangeID = externalChangeID
		}
		if cur.TicketID == "" {
			cur.TicketID = ticketID
		}
		if cur.ApprovalReference == "" {
			cur.ApprovalReference = approvalRef
		}
		cur.Events = append(cur.Events, e)
		if len(approvalsFromData(e.Data)) > 0 {
			cur.HasApprovals = true
		}
		if cur.Project == "" {
			cur.Project = ce.ExtensionString(e.Extensions, "project")
		}
		if cur.Namespace == "" {
			cur.Namespace = ce.ExtensionString(e.Extensions, "namespace")
		}
		if cur.TargetCluster == "" {
			cur.TargetCluster = ce.ExtensionString(e.Extensions, "cluster")
		}
		if cur.Initiator == "" {
			initiator := ce.ExtensionString(e.Extensions, "initiator")
			if strings.TrimSpace(initiator) != "" {
				cur.Initiator = strings.TrimSpace(initiator)
			}
		}
		if e.Time.UTC().Before(cur.StartedAt) {
			cur.StartedAt = e.Time.UTC()
		}
		if e.Time.UTC().After(cur.CompletedAt) {
			cur.CompletedAt = e.Time.UTC()
		}
		cur.EventCount = len(cur.Events)
		cur.ResultStatus = mergeResultStatus(cur.ResultStatus, inferResultStatus(e))
		cur.HealthStatus = mergeHealthStatus(cur.HealthStatus, inferHealthStatus(e))
		out[id] = cur
	}
	for id, c := range out {
		out[id] = finalizeChange(c)
	}
	return out
}

const evidenceWindowSeconds = 300

func finalizeChange(c ChangeDetail) ChangeDetail {
	sort.Slice(c.Events, func(i, j int) bool {
		if c.Events[i].Time.Equal(c.Events[j].Time) {
			return c.Events[i].ID < c.Events[j].ID
		}
		return c.Events[i].Time.Before(c.Events[j].Time)
	})
	if len(c.Events) == 0 {
		return c
	}
	c.StartedAt = c.Events[0].Time.UTC()
	c.CompletedAt = c.Events[len(c.Events)-1].Time.UTC()
	c.EventCount = len(c.Events)
	c.EvidenceLastUpdatedAt = c.CompletedAt
	c.EvidenceWindowSeconds = evidenceWindowSeconds
	c.EvidenceMayBeIncomplete = time.Since(c.CompletedAt) < time.Duration(evidenceWindowSeconds)*time.Second

	startHealth := "unknown"
	endHealth := "unknown"
	lastHealth := "unknown"
	var firstDegradedAfterStart *time.Time
	for _, e := range c.Events {
		h := inferHealthStatus(e)
		if h == "unknown" {
			continue
		}
		if !e.Time.After(c.StartedAt) {
			startHealth = h
		}
		if !e.Time.Before(c.CompletedAt) {
			endHealth = h
		}
		if firstDegradedAfterStart == nil && e.Time.After(c.StartedAt) && h == "degraded" {
			ts := e.Time.UTC()
			firstDegradedAfterStart = &ts
		}
		lastHealth = h
	}
	if endHealth == "unknown" {
		endHealth = lastHealth
	}
	c.HealthAtOperationStart = startHealth
	c.HealthAfterDeploy = endHealth
	c.PostDeployDegradation = PostDeployDegradation{
		Observed:       strings.EqualFold(startHealth, "healthy") && firstDegradedAfterStart != nil,
		FirstTimestamp: firstDegradedAfterStart,
	}
	return c
}

func deriveChangeIdentity(e ce.StoredEvent) (id string, provider string, reference string, revision string) {
	ext := e.Extensions
	cluster := ce.ExtensionString(ext, "cluster")
	namespace := ce.ExtensionString(ext, "namespace")
	subjectID := strings.ToLower(e.Subject + ":" + namespace + ":" + cluster)
	provider = normalizePrimaryProvider(e.Source, ext)
	reference = firstExtension(ext, "primary_reference", "operation_id", "history_id", "deploy_id", "pipeline_id", "run_id", "job_id")
	revision = firstExtension(ext, "revision", "sync_revision", "commit_sha")
	if reference != "" {
		return "chg_" + stableHash(subjectID+":"+provider+":"+reference), provider, reference, revision
	}
	finishedAt := e.Time.UTC().Format(time.RFC3339)
	return "chg_" + stableHash(subjectID+":"+provider+":"+revision+":"+finishedAt), provider, reference, revision
}

func normalizePrimaryProvider(source string, extensions map[string]interface{}) string {
	if ce.ExtensionBool(extensions, "supporting_observation") {
		if firstExtension(extensions, "operation_id", "history_id", "deploy_id", "sync_revision") != "" {
			return "argo"
		}
	}
	if v := ce.ExtensionString(extensions, "primary_provider"); v != "" {
		return strings.ToLower(v)
	}
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "argocd", "argo":
		return "argo"
	case "github_actions", "gha":
		return "gha"
	case "gitlab_ci", "gitlabci":
		return "gitlabci"
	case "jenkins":
		return "jenkins"
	default:
		s := strings.ToLower(strings.TrimSpace(source))
		if s == "" {
			return "generic"
		}
		return s
	}
}

func stableHash(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func firstExtension(extensions map[string]interface{}, keys ...string) string {
	if extensions == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := extensions[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func dataString(data json.RawMessage, key string) string {
	if len(data) == 0 {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func externalCorrelationFromEvent(e ce.StoredEvent) (externalChangeID, ticketID, approvalReference string) {
	ext := e.Extensions
	externalChangeID = ce.ExtensionString(ext, "external_change_id")
	if externalChangeID == "" {
		externalChangeID = ce.ExtensionString(ext, "change_id")
	}
	ticketID = ce.ExtensionString(ext, "ticket_id")
	approvalReference = ce.ExtensionString(ext, "approval_reference")

	// Also check data.source_payload.annotations for argo events
	if externalChangeID == "" || ticketID == "" || approvalReference == "" {
		var dataMap map[string]interface{}
		if len(e.Data) > 0 {
			if err := json.Unmarshal(e.Data, &dataMap); err == nil {
				sourcePayload := nestedMap(dataMap, "source_payload")
				annotations := nestedMap(sourcePayload, "annotations")
				if externalChangeID == "" {
					externalChangeID = stringFromAny(annotations["evidra.io/change-id"])
				}
				if ticketID == "" {
					ticketID = stringFromAny(annotations["evidra.io/ticket"])
				}
				if approvalReference == "" {
					approvalReference = stringFromAny(annotations["evidra.io/approvals-ref"])
				}
			}
		}
	}
	return
}

func approvalsFromData(data json.RawMessage) []ApprovalEvidence {
	if len(data) == 0 {
		return nil
	}
	var dataMap map[string]interface{}
	if err := json.Unmarshal(data, &dataMap); err != nil {
		return nil
	}
	return approvalsFromMap(dataMap)
}

func approvalsFromMap(dataMap map[string]interface{}) []ApprovalEvidence {
	out := make([]ApprovalEvidence, 0)
	appendIfSet := func(a ApprovalEvidence) {
		if strings.TrimSpace(a.Source) == "" &&
			strings.TrimSpace(a.Identity) == "" &&
			strings.TrimSpace(a.Timestamp) == "" &&
			strings.TrimSpace(a.Reference) == "" &&
			strings.TrimSpace(a.Summary) == "" {
			return
		}
		out = append(out, a)
	}
	parseEntry := func(raw interface{}) {
		m, ok := raw.(map[string]interface{})
		if !ok || m == nil {
			return
		}
		appendIfSet(ApprovalEvidence{
			Source:    firstMapString(m, "source", "approval.source"),
			Identity:  firstMapString(m, "identity", "approval.identity"),
			Timestamp: firstMapString(m, "timestamp", "approval.timestamp"),
			Reference: firstMapString(m, "reference", "approval.reference"),
			Summary:   firstMapString(m, "summary", "approval.summary"),
		})
	}

	if raw, ok := dataMap["approvals"]; ok {
		switch vv := raw.(type) {
		case []interface{}:
			for _, item := range vv {
				parseEntry(item)
			}
		case map[string]interface{}:
			parseEntry(vv)
		}
	}

	// Also check source_payload.annotations for evidra.io/approvals-json
	sourcePayload := nestedMap(dataMap, "source_payload")
	annotations := nestedMap(sourcePayload, "annotations")
	if raw := stringFromAny(annotations["evidra.io/approvals-json"]); raw != "" {
		var list []map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &list); err == nil {
			for _, item := range list {
				parseEntry(item)
			}
			return out
		}
		var single map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &single); err == nil {
			parseEntry(single)
		}
	}
	return out
}

func inferResultStatus(e ce.StoredEvent) string {
	status := dataString(e.Data, "status")
	if status == "" {
		status = dataString(e.Data, "result")
	}
	if status == "" {
		status = dataString(e.Data, "phase")
	}
	if status == "" {
		status = dataString(e.Data, "outcome")
	}
	bucket := strings.ToLower(e.Type + " " + status)
	if strings.Contains(bucket, "fail") || strings.Contains(bucket, "error") || strings.Contains(bucket, "degrad") || strings.Contains(bucket, "abort") {
		return "failed"
	}
	if strings.Contains(bucket, "success") || strings.Contains(bucket, "succeed") || strings.Contains(bucket, "healthy") || strings.Contains(bucket, "complete") {
		return "succeeded"
	}
	return "unknown"
}

func mergeResultStatus(current, next string) string {
	if current == "failed" || next == "failed" {
		return "failed"
	}
	if current == "succeeded" || next == "succeeded" {
		return "succeeded"
	}
	return "unknown"
}

func inferHealthStatus(e ce.StoredEvent) string {
	raw := strings.ToLower(ce.ExtensionString(e.Extensions, "health_status"))
	if raw == "" {
		raw = strings.ToLower(ce.ExtensionString(e.Extensions, "health"))
	}
	switch raw {
	case "healthy":
		return "healthy"
	case "degraded":
		return "degraded"
	case "progressing":
		return "progressing"
	case "missing":
		return "missing"
	default:
		return "unknown"
	}
}

func mergeHealthStatus(current, next string) string {
	if next == "degraded" {
		return "degraded"
	}
	if next == "progressing" && current != "degraded" {
		return "progressing"
	}
	if next == "healthy" && current == "unknown" {
		return "healthy"
	}
	if next == "missing" && current == "unknown" {
		return "missing"
	}
	return current
}

func filterChanges(changes []Change, q ChangeQuery) []Change {
	out := make([]Change, 0, len(changes))
	wantExternal := strings.TrimSpace(q.ExternalChangeID)
	wantTicket := strings.TrimSpace(q.TicketID)
	wantApprovalRef := strings.TrimSpace(q.ApprovalReference)
	externalState := strings.ToLower(strings.TrimSpace(q.ExternalChangeIDState))
	ticketState := strings.ToLower(strings.TrimSpace(q.TicketIDState))
	wantHasApprovals := strings.ToLower(strings.TrimSpace(q.HasApprovals))
	for _, c := range changes {
		if q.ResultStatus != "" && c.ResultStatus != q.ResultStatus {
			continue
		}
		if q.HealthStatus != "" && c.HealthStatus != q.HealthStatus {
			continue
		}
		if externalState == "set" && strings.TrimSpace(c.ExternalChangeID) == "" {
			continue
		}
		if externalState == "unset" && strings.TrimSpace(c.ExternalChangeID) != "" {
			continue
		}
		if wantExternal != "" && !strings.EqualFold(c.ExternalChangeID, wantExternal) {
			continue
		}
		if ticketState == "set" && strings.TrimSpace(c.TicketID) == "" {
			continue
		}
		if ticketState == "unset" && strings.TrimSpace(c.TicketID) != "" {
			continue
		}
		if wantTicket != "" && !strings.EqualFold(c.TicketID, wantTicket) {
			continue
		}
		if wantApprovalRef != "" && !strings.EqualFold(c.ApprovalReference, wantApprovalRef) {
			continue
		}
		if wantHasApprovals == "yes" && !c.HasApprovals {
			continue
		}
		if wantHasApprovals == "no" && c.HasApprovals {
			continue
		}
		out = append(out, c)
	}
	return out
}

func encodeChangeCursor(ts time.Time, id string) string {
	raw, _ := json.Marshal(map[string]string{
		"ts": ts.UTC().Format(time.RFC3339Nano),
		"id": id,
	})
	return base64.RawStdEncoding.EncodeToString(raw)
}

func decodeChangeCursor(cursor string) (time.Time, string, error) {
	if strings.TrimSpace(cursor) == "" {
		return time.Time{}, "", nil
	}
	b, err := base64.RawStdEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", ErrInvalidChangeCursor
	}
	var payload map[string]string
	if err := json.Unmarshal(b, &payload); err != nil {
		return time.Time{}, "", ErrInvalidChangeCursor
	}
	ts, err := time.Parse(time.RFC3339Nano, payload["ts"])
	if err != nil {
		return time.Time{}, "", ErrInvalidChangeCursor
	}
	id := payload["id"]
	if strings.TrimSpace(id) == "" {
		return time.Time{}, "", ErrInvalidChangeCursor
	}
	return ts.UTC(), id, nil
}

func applyChangeCursor(changes []Change, cursorTS time.Time, cursorID string) []Change {
	out := make([]Change, 0, len(changes))
	for _, c := range changes {
		if c.CompletedAt.Before(cursorTS) || (c.CompletedAt.Equal(cursorTS) && c.ID > cursorID) {
			out = append(out, c)
		}
	}
	return out
}

func nestedMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	switch vv := raw.(type) {
	case map[string]interface{}:
		return vv
	case map[string]string:
		out := make(map[string]interface{}, len(vv))
		for k, v := range vv {
			out[k] = v
		}
		return out
	default:
		return nil
	}
}

func stringFromAny(raw interface{}) string {
	switch vv := raw.(type) {
	case string:
		return strings.TrimSpace(vv)
	default:
		return ""
	}
}

func firstMapString(m map[string]interface{}, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
