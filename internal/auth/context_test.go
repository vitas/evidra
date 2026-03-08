package auth

import (
	"context"
	"testing"
)

func TestTenantID_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := WithTenantID(context.Background(), "tenant-123")
	got := TenantID(ctx)
	if got != "tenant-123" {
		t.Fatalf("expected tenant-123, got %s", got)
	}
}

func TestTenantID_Missing(t *testing.T) {
	t.Parallel()
	got := TenantID(context.Background())
	if got != "" {
		t.Fatalf("expected empty string for missing tenant, got %s", got)
	}
}
