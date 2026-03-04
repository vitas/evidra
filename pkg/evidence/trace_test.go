package evidence

import (
	"testing"
)

func TestGenerateTraceID_NonEmpty(t *testing.T) {
	t.Parallel()

	id := GenerateTraceID()
	if id == "" {
		t.Fatal("GenerateTraceID returned empty string")
	}
}

func TestGenerateTraceID_ULIDLength(t *testing.T) {
	t.Parallel()

	id := GenerateTraceID()
	if len(id) != 26 {
		t.Fatalf("expected ULID length 26, got %d (%q)", len(id), id)
	}
}

func TestGenerateTraceID_Unique(t *testing.T) {
	t.Parallel()

	id1 := GenerateTraceID()
	id2 := GenerateTraceID()
	if id1 == id2 {
		t.Fatalf("two calls returned the same value: %s", id1)
	}
}
