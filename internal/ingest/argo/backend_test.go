package argo

import "testing"

func TestNewFetchFuncInvalidBackend(t *testing.T) {
	_, err := NewFetchFunc(BackendOptions{Backend: "invalid", URL: "http://example"})
	if err == nil {
		t.Fatalf("expected backend error")
	}
}

func TestNewFetchFuncArgoClientStub(t *testing.T) {
	fetch, err := NewFetchFunc(BackendOptions{URL: "http://argocd.example"})
	if err != nil {
		t.Fatalf("expected argocd-client fetch func, got error: %v", err)
	}
	if fetch == nil {
		t.Fatalf("expected fetch func")
	}
}
