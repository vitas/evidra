package api

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"evidra/internal/store"
)

const maxIngestBodyBytes int64 = 1 << 20 // 1 MiB

func validWebhookSignature(secret string, body []byte, header string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	parts := strings.SplitN(header, "=", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "sha256") {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimSpace(parts[1]))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(expected, provided)
}

func decodeJSON(body io.ReadCloser, dst interface{}) error {
	defer body.Close()
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func decodeJSONBytes(body []byte, dst interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, code int, errCode, message string, details interface{}, retryable bool) {
	writeJSON(w, code, map[string]interface{}{
		"error": map[string]interface{}{
			"code":      errCode,
			"message":   message,
			"details":   details,
			"retryable": retryable,
		},
	})
}

func handleStoreErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInvalidInput), errors.Is(err, store.ErrInvalidCursor):
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error(), nil, false)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "EVENT_ID_CONFLICT", err.Error(), nil, false)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error(), nil, false)
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error(), nil, true)
	}
}

func readBodyLimited(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	return io.ReadAll(r.Body)
}
