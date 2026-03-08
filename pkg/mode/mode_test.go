package mode

import (
	"testing"
)

func TestResolve_NoURL(t *testing.T) {
	t.Parallel()
	r, err := Resolve(Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.IsOnline {
		t.Error("expected offline mode when URL is empty")
	}
	if r.Client != nil {
		t.Error("expected nil client in offline mode")
	}
	if r.FallbackPolicy != "closed" {
		t.Errorf("expected default fallback=closed, got %s", r.FallbackPolicy)
	}
}

func TestResolve_ForceOffline(t *testing.T) {
	t.Parallel()
	r, err := Resolve(Config{
		URL:          "https://api.example.com",
		APIKey:       "test-key",
		ForceOffline: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.IsOnline {
		t.Error("expected offline when --offline is set")
	}
}

func TestResolve_NoAPIKey(t *testing.T) {
	t.Parallel()
	_, err := Resolve(Config{URL: "https://api.example.com"})
	if err == nil {
		t.Fatal("expected error when URL set but no API key")
	}
}

func TestResolve_Online(t *testing.T) {
	t.Parallel()
	r, err := Resolve(Config{
		URL:    "https://api.example.com",
		APIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.IsOnline {
		t.Error("expected online mode")
	}
	if r.Client == nil {
		t.Error("expected non-nil client")
	}
}

func TestResolve_FallbackNormalization(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input, want string
	}{
		{"", "closed"},
		{"closed", "closed"},
		{"offline", "offline"},
		{"OFFLINE", "offline"},
		{"junk", "closed"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			r, _ := Resolve(Config{FallbackPolicy: tt.input})
			if r.FallbackPolicy != tt.want {
				t.Errorf("got %s, want %s", r.FallbackPolicy, tt.want)
			}
		})
	}
}

func TestResolve_WhitespaceURL(t *testing.T) {
	t.Parallel()
	r, err := Resolve(Config{URL: "  ", APIKey: "key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.IsOnline {
		t.Error("expected offline for whitespace URL")
	}
}
