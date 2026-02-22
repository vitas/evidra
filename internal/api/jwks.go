package api

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"
)

type jwksKeyCache struct {
	url          string
	refreshEvery time.Duration
	lastRefresh  time.Time
	keys         map[string]*rsa.PublicKey
	client       *http.Client
}

type jwksDocument struct {
	Keys []jwkEntry `json:"keys"`
}

type jwkEntry struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func newJWKSKeyCache(cfg JWTPolicy) *jwksKeyCache {
	if strings.TrimSpace(cfg.JWKSURL) == "" {
		return nil
	}
	refresh := 5 * time.Minute
	if d, err := time.ParseDuration(strings.TrimSpace(cfg.JWKSRefresh)); err == nil && d > 0 {
		refresh = d
	}
	return &jwksKeyCache{
		url:          strings.TrimSpace(cfg.JWKSURL),
		refreshEvery: refresh,
		keys:         make(map[string]*rsa.PublicKey),
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *jwksKeyCache) resolveKey(kid string) (*rsa.PublicKey, error) {
	if c == nil {
		return nil, errors.New("jwks disabled")
	}
	now := time.Now().UTC()
	if now.Sub(c.lastRefresh) >= c.refreshEvery || len(c.keys) == 0 {
		if err := c.refresh(); err != nil && len(c.keys) == 0 {
			return nil, err
		}
	}
	key, ok := c.keys[kid]
	if ok {
		return key, nil
	}
	if err := c.refresh(); err != nil {
		return nil, err
	}
	key, ok = c.keys[kid]
	if !ok {
		return nil, errors.New("jwks key not found")
	}
	return key, nil
}

func (c *jwksKeyCache) refresh() error {
	req, err := http.NewRequest(http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("jwks fetch failed")
	}
	var doc jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return err
	}
	next := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, entry := range doc.Keys {
		if strings.ToUpper(strings.TrimSpace(entry.Kty)) != "RSA" {
			continue
		}
		if strings.TrimSpace(entry.Kid) == "" || strings.TrimSpace(entry.N) == "" || strings.TrimSpace(entry.E) == "" {
			continue
		}
		key, err := rsaKeyFromJWK(entry.N, entry.E)
		if err != nil {
			continue
		}
		next[entry.Kid] = key
	}
	if len(next) == 0 {
		return errors.New("jwks contains no rsa keys")
	}
	c.keys = next
	c.lastRefresh = time.Now().UTC()
	return nil
}

func rsaKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(nB64))
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(eB64))
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nb)
	e := new(big.Int).SetBytes(eb).Int64()
	if n.Sign() <= 0 || e <= 0 {
		return nil, errors.New("invalid rsa jwk parameters")
	}
	return &rsa.PublicKey{
		N: n,
		E: int(e),
	}, nil
}
