package api

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"net/http"
)

func handlePubkey(pubkey ed25519.PublicKey) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if pubkey == nil {
			writeError(w, http.StatusNotImplemented, "signing not configured")
			return
		}
		der, err := x509.MarshalPKIXPublicKey(pubkey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "marshal public key")
			return
		}
		block := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write(block)
	}
}
