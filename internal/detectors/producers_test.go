package detectors

import (
	"testing"

	"samebits.com/evidra-benchmark/internal/canon"
)

type staticProducer struct {
	name string
	tags []string
}

func (p *staticProducer) Name() string { return p.name }
func (p *staticProducer) ProduceTags(_ canon.CanonicalAction, _ []byte) []string {
	return append([]string(nil), p.tags...)
}

func TestProduceAll_DedupesAcrossProducers(t *testing.T) {
	t.Parallel()

	prodMu.Lock()
	orig := producers
	producers = []TagProducer{
		&staticProducer{name: "a", tags: []string{"k8s.privileged_container"}},
		&staticProducer{name: "b", tags: []string{"k8s.privileged_container", "custom.tag"}},
	}
	prodMu.Unlock()
	defer func() {
		prodMu.Lock()
		producers = orig
		prodMu.Unlock()
	}()

	tags := ProduceAll(canon.CanonicalAction{}, nil)
	if countTag(tags, "k8s.privileged_container") != 1 {
		t.Fatalf("expected deduped k8s.privileged_container, got %v", tags)
	}
	if countTag(tags, "custom.tag") != 1 {
		t.Fatalf("expected custom.tag, got %v", tags)
	}
}

func countTag(tags []string, want string) int {
	n := 0
	for _, t := range tags {
		if t == want {
			n++
		}
	}
	return n
}
