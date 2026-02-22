package api

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v4"
)

func (s *Server) resolveRoles(r *http.Request) (source, subject string, roles []string, enforced bool, err error) {
	if s.auth.JWT.Enabled {
		subject, roles, err = s.rolesFromJWT(r)
		if err != nil {
			return "jwt", "", nil, true, err
		}
		return "jwt", subject, roles, true, nil
	}
	if s.auth.OIDC.Enabled {
		roles, err = s.rolesFromHeader(r)
		if err != nil {
			return "header", "", nil, true, err
		}
		return "header", "", roles, true, nil
	}
	return "", "", nil, false, nil
}

func (s *Server) rolesFromHeader(r *http.Request) ([]string, error) {
	header := strings.TrimSpace(r.Header.Get(s.auth.OIDC.RolesHeader))
	if header == "" {
		return nil, errors.New("missing oidc roles")
	}
	out := make([]string, 0, 4)
	for _, part := range strings.Split(header, ",") {
		role := strings.ToLower(strings.TrimSpace(part))
		if role != "" {
			out = append(out, role)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("missing oidc roles")
	}
	return out, nil
}

func (s *Server) rolesFromJWT(r *http.Request) (string, []string, error) {
	raw := strings.TrimSpace(bearerToken(r.Header.Get("Authorization")))
	if raw == "" {
		return "", nil, errors.New("missing bearer token")
	}
	token, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		switch token.Method.Alg() {
		case jwt.SigningMethodHS256.Alg():
			secret := strings.TrimSpace(s.auth.JWT.HS256Secret)
			if secret == "" {
				return nil, fmt.Errorf("hs256 secret not configured")
			}
			return []byte(secret), nil
		case jwt.SigningMethodRS256.Alg():
			if strings.TrimSpace(s.auth.JWT.JWKSURL) != "" {
				return s.resolveJWTKeyFromJWKS(token)
			}
			pemText := strings.TrimSpace(s.auth.JWT.RS256PublicKeyPEM)
			if pemText == "" {
				return nil, fmt.Errorf("rs256 public key not configured")
			}
			key, err := parseRSAPublicKeyPEM(pemText)
			if err != nil {
				return nil, err
			}
			return key, nil
		default:
			return nil, fmt.Errorf("unsupported jwt signing algorithm: %s", token.Method.Alg())
		}
	})
	if err != nil || token == nil || !token.Valid {
		return "", nil, errors.New("invalid jwt token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", nil, errors.New("invalid jwt claims")
	}
	if !claims.VerifyIssuer(s.auth.JWT.Issuer, true) {
		return "", nil, errors.New("invalid jwt issuer")
	}
	if !claims.VerifyAudience(s.auth.JWT.Audience, true) {
		return "", nil, errors.New("invalid jwt audience")
	}
	if !claims.VerifyExpiresAt(time.Now().Unix(), true) {
		return "", nil, errors.New("jwt token expired")
	}
	roles := extractClaimRoles(claims, s.auth.JWT.RolesClaim)
	if len(roles) == 0 {
		return "", nil, errors.New("missing jwt roles")
	}
	subject := ""
	if rawSub, ok := claims["sub"].(string); ok {
		subject = strings.TrimSpace(rawSub)
	}
	return subject, roles, nil
}

func (s *Server) resolveJWTKeyFromJWKS(token *jwt.Token) (interface{}, error) {
	if s.jwksCache == nil {
		return nil, errors.New("jwks cache not configured")
	}
	kid := strings.TrimSpace(fmt.Sprintf("%v", token.Header["kid"]))
	if kid == "" {
		return nil, errors.New("jwt token missing kid header")
	}
	s.jwksMu.Lock()
	defer s.jwksMu.Unlock()
	return s.jwksCache.resolveKey(kid)
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func parseRSAPublicKeyPEM(raw string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("invalid rs256 public key pem")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := key.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("rs256 public key is not rsa")
	}
	return pub, nil
}

func extractClaimRoles(claims jwt.MapClaims, claimName string) []string {
	claimName = strings.TrimSpace(claimName)
	if claimName == "" {
		claimName = "roles"
	}
	raw, ok := claims[claimName]
	if !ok {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 4)
	appendRole := func(v string) {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			return
		}
		if _, exists := seen[v]; exists {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	switch vv := raw.(type) {
	case string:
		for _, part := range strings.FieldsFunc(vv, func(r rune) bool { return r == ',' || r == ' ' }) {
			appendRole(part)
		}
	case []string:
		for _, item := range vv {
			appendRole(item)
		}
	case []interface{}:
		for _, item := range vv {
			if s, ok := item.(string); ok {
				appendRole(s)
			}
		}
	}
	return out
}
