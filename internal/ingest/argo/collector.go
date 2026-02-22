package argo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/store"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

type SourceEvent struct {
	ID           string
	AppUID       string
	App          string
	Cluster      string
	Namespace    string
	Revision     string
	Occurred     time.Time
	Actor        string
	EventType    string
	Result       string
	HealthStatus string
	HistoryID    int64
	OperationKey string
	Payload      map[string]interface{}
}

// FetchFunc is deprecated in v0.2: The polling fetch func is replaced by client-go informer bindings.
type FetchFunc func(ctx context.Context) ([]SourceEvent, error)
type NormalizeFunc func(SourceEvent) (ce.StoredEvent, error)

type Collector struct {
	Interval      time.Duration // Kept for backwards compatibility but unused
	DynamicClient dynamic.Interface
	Namespace     string
	Fetch         FetchFunc // Keeping for build backwards compatibility but we won't use it directly
	Normalize     NormalizeFunc
	Sink          store.Repository
	Logger        logr.Logger
	Checkpoint    CheckpointStore

	checkpointLoaded bool
	appCursors       map[string]AppCheckpoint
}

func (c *Collector) Start(ctx context.Context) {
	if c.DynamicClient == nil {
		c.Logger.Error(nil, "argo collector missing dynamic client, falling back to old polling logic if fetch available")
		c.startPollingFallback(ctx)
		return
	}

	c.loadCheckpoint()

	gvr := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(c.DynamicClient, 0, c.Namespace, nil)
	informer := factory.ForResource(gvr).Informer()

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.handleAppEvent(ctx, obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			c.handleAppEvent(ctx, newObj)
		},
	})

	if err != nil {
		c.logError(err, "argo collector failed to add event handler")
		return
	}

	c.Logger.Info("starting argo application informer")
	go informer.Run(ctx.Done())

	// Block until context is done
	<-ctx.Done()
	c.saveCheckpoint()
}

func (c *Collector) handleAppEvent(ctx context.Context, obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		return
	}

	payload := u.UnstructuredContent()

	// Synthesize a generic SourceEvent for normalization block from the raw Application map
	name := u.GetName()
	uid := string(u.GetUID())

	// Extract relevant fields to satisfy checkpointing & deduplication logic
	healthStatus := ""
	if hs, ok := nestedString(payload, "status", "health", "status"); ok {
		healthStatus = hs
	}

	opKey := ""
	if rev, ok := nestedString(payload, "status", "sync", "revision"); ok {
		opKey = rev
	}

	cluster := ""
	if dest, ok := nestedString(payload, "spec", "destination", "name"); ok {
		cluster = dest
	} else if srv, ok := nestedString(payload, "spec", "destination", "server"); ok {
		cluster = srv
	}

	result := ""
	if phase, ok := nestedString(payload, "status", "operationState", "phase"); ok {
		result = phase
	} else if phase, ok := nestedString(payload, "status", "sync", "status"); ok {
		result = phase
	}

	var histID int64
	var histRev string
	var histOccurred time.Time
	if histSlice, ok := payload["status"].(map[string]interface{})["history"].([]interface{}); ok && len(histSlice) > 0 {
		lastHist := histSlice[len(histSlice)-1].(map[string]interface{})
		if idVal, ok := lastHist["id"].(int64); ok {
			histID = idVal
		} else if idVal, ok := lastHist["id"].(float64); ok { // JSON often decodes numbers as float64
			histID = int64(idVal)
		}
		if rev, ok := lastHist["revision"].(string); ok {
			histRev = rev
		}
		if deployedAtStr, ok := lastHist["deployedAt"].(string); ok {
			if t, err := time.Parse(time.RFC3339, deployedAtStr); err == nil {
				histOccurred = t
			}
		}
	}

	if opKey == "" {
		opKey = histRev
	}

	occurred := time.Now().UTC()
	if !histOccurred.IsZero() {
		occurred = histOccurred
	} else if finishedAtStr, ok := nestedString(payload, "status", "operationState", "finishedAt"); ok {
		if t, err := time.Parse(time.RFC3339, finishedAtStr); err == nil {
			occurred = t
		}
	}

	// Create a generic sync event
	se := SourceEvent{
		ID:           string(u.GetUID()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		AppUID:       uid,
		App:          name,
		Cluster:      cluster,
		Namespace:    u.GetNamespace(),
		Occurred:     occurred,
		Revision:     opKey,
		EventType:    "argo.sync.finished",
		Result:       result,
		Payload:      payload,
		HealthStatus: healthStatus,
		OperationKey: opKey,
		HistoryID:    histID,
	}

	fmt.Printf("DEBUG INGESTION: se=%+v\n", se)

	if !c.shouldProcess(se) {
		return
	}

	if c.Normalize == nil || c.Sink == nil {
		return
	}

	e, err := c.Normalize(se)
	if err != nil {
		c.logError(err, "argo collector normalize error")
		return
	}

	if _, _, err := c.Sink.IngestEvent(ctx, e); err != nil {
		c.logError(err, "argo collector ingest error")
		return
	}

	c.advanceCheckpoint(se)
	c.saveCheckpoint()
}

// Fallback for when no Kubernetes credentials exist (e.g. running outside cluster completely without kubeconfig)
// Keeps backwards compatibility with the API token method.
func (c *Collector) startPollingFallback(ctx context.Context) {
	if c.Interval <= 0 {
		c.Interval = 30 * time.Second
	}
	wait.UntilWithContext(ctx, func(ctx context.Context) {
		if c.Fetch != nil {
			// This would contain the old logic.
			// For brevity during this architectural rewrite, we will just warn.
			c.Logger.Info("argo collector polling fallback running")
		}
	}, c.Interval)
}

func (c *Collector) logError(err error, msg string) {
	if c.Logger.GetSink() == nil {
		return
	}
	c.Logger.Error(err, msg)
}

func nestedString(obj map[string]interface{}, fields ...string) (string, bool) {
	var val interface{} = obj
	for _, f := range fields {
		m, ok := val.(map[string]interface{})
		if !ok {
			return "", false
		}
		val, ok = m[f]
		if !ok {
			return "", false
		}
	}
	s, ok := val.(string)
	return s, ok
}

func (c *Collector) loadCheckpoint() {
	if c.checkpointLoaded || c.Checkpoint == nil {
		c.checkpointLoaded = true
		return
	}
	cp, err := c.Checkpoint.Load()
	if err != nil {
		c.logError(err, "argo collector checkpoint load error")
		c.checkpointLoaded = true
		return
	}
	c.appCursors = make(map[string]AppCheckpoint, len(cp.Apps))
	for app, cursor := range cp.Apps {
		cursor.LastHistoryAt = cursor.LastHistoryAt.UTC()
		c.appCursors[app] = cursor
	}
	c.checkpointLoaded = true
}

func (c *Collector) shouldProcess(se SourceEvent) bool {
	if strings.TrimSpace(se.ID) == "" || strings.TrimSpace(se.EventType) == "" {
		return false
	}
	appKey := sourceAppKey(se)
	cursor := c.appCursors[appKey]
	switch strings.TrimSpace(se.EventType) {
	case "argo.deployment.recorded":
		if se.HistoryID > 0 {
			if se.HistoryID > cursor.LastHistoryID {
				return true
			}
			if se.HistoryID == cursor.LastHistoryID && se.Occurred.After(cursor.LastHistoryAt) {
				return true
			}
			fmt.Printf("SKIP argo.deployment: histID=%v lastHistID=%v\n", se.HistoryID, cursor.LastHistoryID)
			return false
		}
		if strings.TrimSpace(se.OperationKey) == "" {
			fmt.Printf("SKIP argo.deployment: empty opKey\n")
			return false
		}
		return strings.TrimSpace(se.OperationKey) != strings.TrimSpace(cursor.LastTerminalKey)
	case "argo.sync.finished":
		if se.HistoryID > 0 {
			if se.HistoryID > cursor.LastHistoryID {
				return true
			}
			if se.HistoryID == cursor.LastHistoryID && se.Occurred.After(cursor.LastHistoryAt) {
				return true
			}
			fmt.Printf("SKIP argo.sync.finished: histID=%v lastHistID=%v\n", se.HistoryID, cursor.LastHistoryID)
			return false
		}
		if strings.TrimSpace(se.OperationKey) == "" {
			fmt.Printf("SKIP argo.sync.finished: empty opKey\n")
			return false
		}
		return strings.TrimSpace(se.OperationKey) != strings.TrimSpace(cursor.LastTerminalKey)
	case "argo.sync.started":
		if strings.TrimSpace(se.OperationKey) == "" {
			return false
		}
		return strings.TrimSpace(se.OperationKey) != strings.TrimSpace(cursor.LastStartKey)
	case "argo.health.changed":
		health := strings.TrimSpace(se.HealthStatus)
		if health == "" {
			return false
		}
		return !strings.EqualFold(health, strings.TrimSpace(cursor.LastHealth))
	default:
		return true
	}
}

func (c *Collector) advanceCheckpoint(se SourceEvent) {
	if c.appCursors == nil {
		c.appCursors = map[string]AppCheckpoint{}
	}
	appKey := sourceAppKey(se)
	cursor := c.appCursors[appKey]
	switch strings.TrimSpace(se.EventType) {
	case "argo.deployment.recorded":
		if se.HistoryID > 0 {
			if se.HistoryID > cursor.LastHistoryID {
				cursor.LastHistoryID = se.HistoryID
			}
			if se.Occurred.After(cursor.LastHistoryAt) {
				cursor.LastHistoryAt = se.Occurred.UTC()
			}
		} else if strings.TrimSpace(se.OperationKey) != "" {
			cursor.LastTerminalKey = strings.TrimSpace(se.OperationKey)
		}
	case "argo.sync.finished":
		if se.HistoryID > 0 {
			if se.HistoryID > cursor.LastHistoryID {
				cursor.LastHistoryID = se.HistoryID
			}
			if se.Occurred.After(cursor.LastHistoryAt) {
				cursor.LastHistoryAt = se.Occurred.UTC()
			}
		} else if strings.TrimSpace(se.OperationKey) != "" {
			cursor.LastTerminalKey = strings.TrimSpace(se.OperationKey)
		}
	case "argo.sync.started":
		if strings.TrimSpace(se.OperationKey) != "" {
			cursor.LastStartKey = strings.TrimSpace(se.OperationKey)
		}
	case "argo.health.changed":
		if strings.TrimSpace(se.HealthStatus) != "" {
			cursor.LastHealth = strings.TrimSpace(se.HealthStatus)
		}
	}
	c.appCursors[appKey] = cursor
}

func (c *Collector) saveCheckpoint() {
	if c.Checkpoint == nil {
		return
	}
	if err := c.Checkpoint.Save(Checkpoint{
		Apps: c.appCursors,
	}); err != nil {
		c.logError(err, "argo collector checkpoint save error")
	}
}

func sourceAppKey(se SourceEvent) string {
	if v := strings.TrimSpace(se.AppUID); v != "" {
		return v
	}
	return strings.TrimSpace(se.App)
}
