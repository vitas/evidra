package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ce "evidra/internal/cloudevents"
	"evidra/internal/store"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type Config struct {
	Interval    time.Duration
	Namespace   string
	Cluster     string
	Environment string
	Kubeconfig  string
	Context     string
}

type Collector struct {
	Interval    time.Duration
	Namespace   string
	Cluster     string
	Environment string
	Client      kubernetes.Interface
	Sink        store.Repository
	Logger      logr.Logger

	lastResourceVersion string
}

func NewCollector(cfg Config, client kubernetes.Interface, sink store.Repository, logger logr.Logger) *Collector {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	cluster := strings.TrimSpace(cfg.Cluster)
	if cluster == "" {
		cluster = "unknown"
	}
	env := strings.TrimSpace(cfg.Environment)
	if env == "" {
		env = "unknown"
	}
	return &Collector{
		Interval:    interval,
		Namespace:   strings.TrimSpace(cfg.Namespace),
		Cluster:     cluster,
		Environment: env,
		Client:      client,
		Sink:        sink,
		Logger:      logger,
	}
}

func NewClientFromKubeconfig(kubeconfigPath, kubeContext string) (kubernetes.Interface, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if strings.TrimSpace(kubeconfigPath) != "" {
		rules.ExplicitPath = kubeconfigPath
	}
	overrides := &clientcmd.ConfigOverrides{}
	if strings.TrimSpace(kubeContext) != "" {
		overrides.CurrentContext = strings.TrimSpace(kubeContext)
	}
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(restCfg)
}

func (c *Collector) Start(ctx context.Context) {
	if c.Interval <= 0 {
		c.Interval = 30 * time.Second
	}
	wait.UntilWithContext(ctx, func(ctx context.Context) {
		c.poll(ctx)
	}, c.Interval)
}

func (c *Collector) poll(ctx context.Context) {
	if c.Client == nil || c.Sink == nil {
		return
	}
	opts := metav1.ListOptions{
		FieldSelector: fields.Everything().String(),
	}
	if c.lastResourceVersion != "" {
		opts.ResourceVersion = c.lastResourceVersion
	}
	events, err := c.Client.CoreV1().Events(c.Namespace).List(ctx, opts)
	if err != nil {
		c.logError(err, "kubernetes collector list events failed")
		return
	}
	for _, e := range events.Items {
		normalized := c.normalizeEvent(e)
		if c.shouldCollapseLifecycle(ctx, normalized) {
			normalized = c.toCollapsedSupporting(normalized)
		}
		if _, _, err := c.Sink.IngestEvent(ctx, normalized); err != nil {
			c.logError(err, "kubernetes collector ingest failed")
		}
	}
	c.lastResourceVersion = events.ResourceVersion
}

func (c *Collector) normalizeEvent(e corev1.Event) ce.StoredEvent {
	app := inferApp(e)
	actor := strings.TrimSpace(e.ReportingController)
	if actor == "" {
		actor = strings.TrimSpace(e.Source.Component)
	}
	if actor == "" {
		actor = "kubernetes"
	}
	id := fmt.Sprintf("evt_k8s_%s_%s", string(e.UID), e.ResourceVersion)
	if strings.TrimSpace(e.ResourceVersion) == "" {
		id = fmt.Sprintf("evt_k8s_%s_%d", string(e.UID), time.Now().UTC().UnixNano())
	}
	reason := strings.ToLower(strings.TrimSpace(e.Reason))
	eventType := "k8s.supporting.observation"
	supportingClass := "supporting_evidence"
	if strings.EqualFold(strings.TrimSpace(e.Type), "warning") {
		eventType = "k8s.incident.signal"
		supportingClass = "incident_signal"
	}
	if isLifecycleNoise(reason) {
		eventType = "k8s.supporting.lifecycle"
		supportingClass = "lifecycle_noise"
	}
	if reason == "" {
		reason = "resource_event"
	}
	ts := eventTimestamp(e)
	extensions := map[string]interface{}{
		"cluster":                "eu-1",
		"namespace":              c.Environment,
		"initiator":              actor,
		"supporting_observation": true,
		"supporting_class":       supportingClass,
		"event_reason":           reason,
		"k8s_namespace":          e.Namespace,
		"involved_kind":          e.InvolvedObject.Kind,
		"involved_name":          e.InvolvedObject.Name,
		"involved_namespace":     e.InvolvedObject.Namespace,
		"reason":                 e.Reason,
		"type":                   e.Type,
		"reporting_controller":   e.ReportingController,
	}
	// Use the configured cluster value
	extensions["cluster"] = c.Cluster
	data, _ := json.Marshal(map[string]interface{}{
		"involved_kind": e.InvolvedObject.Kind,
		"involved_name": e.InvolvedObject.Name,
		"reason":        e.Reason,
		"message":       e.Message,
		"resource":      fmt.Sprintf("%s/%s", strings.ToLower(e.InvolvedObject.Kind), e.InvolvedObject.Name),
	})
	return ce.StoredEvent{
		ID:         id,
		Source:     "kubernetes",
		Type:       eventType,
		Time:       ts.UTC(),
		Subject:    app,
		Extensions: extensions,
		Data:       json.RawMessage(data),
	}
}

func (c *Collector) shouldCollapseLifecycle(ctx context.Context, event ce.StoredEvent) bool {
	if !strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", event.Extensions["supporting_class"])), "lifecycle_noise") {
		return false
	}
	from := event.Time.Add(-10 * time.Minute)
	to := event.Time.Add(10 * time.Minute)
	timeline, err := c.Sink.QueryTimeline(ctx, store.TimelineQuery{
		Subject:           event.Subject,
		Namespace:         ce.ExtensionString(event.Extensions, "namespace"),
		Cluster:           ce.ExtensionString(event.Extensions, "cluster"),
		From:              from.UTC(),
		To:                to.UTC(),
		Source:            "argocd",
		Type:              "argo.sync.finished",
		IncludeSupporting: true,
		Limit:             1,
	})
	if err != nil {
		c.logError(err, "kubernetes collector correlation lookup failed")
		return false
	}
	return len(timeline.Items) > 0
}

func (c *Collector) toCollapsedSupporting(event ce.StoredEvent) ce.StoredEvent {
	window := event.Time.UTC().Truncate(5 * time.Minute)
	reason := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", event.Extensions["event_reason"])))
	if reason == "" {
		reason = "resource_event"
	}
	event.ID = fmt.Sprintf(
		"evt_k8s_support_%s_%s_%d",
		slug(strings.ToLower(event.Subject)),
		slug(reason),
		window.Unix(),
	)
	newExt := make(map[string]interface{}, len(event.Extensions)+2)
	for k, v := range event.Extensions {
		newExt[k] = v
	}
	newExt["collapsed"] = true
	newExt["collapse_window_start"] = window.Format(time.RFC3339)
	event.Extensions = newExt
	return event
}

func inferApp(e corev1.Event) string {
	name := strings.TrimSpace(e.InvolvedObject.Name)
	if name == "" {
		return "unknown"
	}
	return name
}

func isLifecycleNoise(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "pulled", "pulling", "created", "started", "killing", "scheduled", "successfulcreate":
		return true
	default:
		return false
	}
}

func slug(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	if in == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range in {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		if r == '-' || r == '_' {
			b.WriteRune('-')
			continue
		}
		b.WriteRune('-')
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}

func eventTimestamp(e corev1.Event) time.Time {
	if !e.EventTime.IsZero() {
		return e.EventTime.Time
	}
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	if !e.FirstTimestamp.IsZero() {
		return e.FirstTimestamp.Time
	}
	if e.Series != nil && !e.Series.LastObservedTime.IsZero() {
		return e.Series.LastObservedTime.Time
	}
	return time.Now().UTC()
}

func (c *Collector) logError(err error, msg string) {
	if c.Logger.GetSink() == nil {
		return
	}
	c.Logger.Error(err, msg)
}
