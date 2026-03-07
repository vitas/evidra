package detectors

import (
	"sync"

	"samebits.com/evidra-benchmark/internal/canon"
)

// Stability indicates detector maturity.
type Stability string

const (
	Stable       Stability = "stable"
	Experimental Stability = "experimental"
	Deprecated   Stability = "deprecated"
)

// VocabularyLevel classifies detector outputs.
type VocabularyLevel string

const (
	ResourceRisk  VocabularyLevel = "resource"
	OperationRisk VocabularyLevel = "operation"
)

// Detector inspects an operation for a specific risk pattern.
type Detector interface {
	Tag() string
	BaseSeverity() string
	Detect(action canon.CanonicalAction, raw []byte) bool
	Metadata() TagMetadata
}

// TagMetadata describes detector metadata for export and prompts.
type TagMetadata struct {
	Tag          string          `json:"tag" yaml:"tag"`
	BaseSeverity string          `json:"base_severity" yaml:"base_severity"`
	Stability    Stability       `json:"stability" yaml:"stability"`
	Level        VocabularyLevel `json:"level" yaml:"level"`
	Domain       string          `json:"domain" yaml:"domain"`
	SourceKind   string          `json:"source_kind" yaml:"source_kind"`
	Summary      string          `json:"summary" yaml:"summary"`
}

var (
	regMu    sync.RWMutex
	registry []Detector
)

// Register adds a detector to the global registry.
func Register(d Detector) {
	if d == nil {
		return
	}
	regMu.Lock()
	defer regMu.Unlock()
	registry = append(registry, d)
}

// All returns all registered detectors.
func All() []Detector {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]Detector, len(registry))
	copy(out, registry)
	return out
}

// RunAll runs all registered detectors and returns fired tags.
func RunAll(action canon.CanonicalAction, raw []byte) []string {
	var tags []string
	seen := make(map[string]bool)
	for _, d := range All() {
		if !d.Detect(action, raw) {
			continue
		}
		tag := d.Tag()
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}

// AllMetadata returns metadata for all registered detectors.
func AllMetadata() []TagMetadata {
	ds := All()
	out := make([]TagMetadata, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Metadata())
	}
	return out
}

// StableOnly returns metadata for stable detectors only.
func StableOnly() []TagMetadata {
	ds := All()
	out := make([]TagMetadata, 0, len(ds))
	for _, d := range ds {
		m := d.Metadata()
		if m.Stability == Stable {
			out = append(out, m)
		}
	}
	return out
}
