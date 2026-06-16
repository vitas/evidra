package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestUIHandler_FallsBackToIndexForClientRoutes(t *testing.T) {
	t.Parallel()

	handler := uiHandler(testUIFS())
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Evidra app shell") {
		t.Fatalf("fallback body = %q, want index shell", rec.Body.String())
	}
}

func TestUIHandler_DoesNotServeIndexForMissingAssets(t *testing.T) {
	t.Parallel()

	handler := uiHandler(testUIFS())
	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if strings.Contains(rec.Body.String(), "Evidra app shell") {
		t.Fatal("missing asset returned the SPA index")
	}
}

func TestUIHandler_ServesExistingAssets(t *testing.T) {
	t.Parallel()

	handler := uiHandler(testUIFS())
	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<svg") {
		t.Fatalf("asset body = %q, want SVG", rec.Body.String())
	}
}

func testUIFS() fs.FS {
	return fstest.MapFS{
		"index.html": {
			Data: []byte("<!doctype html><title>Evidra app shell</title>"),
		},
		"favicon.svg": {
			Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		},
	}
}
