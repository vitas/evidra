package api

import (
	"errors"
	"net/http"
	"strings"
)

var errRateLimited = errors.New("rate limited")

func (s *Server) authorizeRead(r *http.Request) error {
	if s.rateLimiter != nil && !s.rateLimiter.Allow(r, "read") {
		s.auditAuth(r, "deny", "rate_limit", "", nil, "read rate limit exceeded")
		return errRateLimited
	}
	if err := s.authorizeRoles(r, "reader", "exporter", "admin"); err != nil {
		s.auditAuth(r, "deny", "roles", "", nil, err.Error())
		return err
	}
	return s.authorizeReadToken(r)
}

func (s *Server) authorizeReadToken(r *http.Request) error {
	token := strings.TrimSpace(s.auth.Read.Token)
	if token == "" {
		s.auditAuth(r, "allow", "none", "", nil, "")
		return nil
	}
	if !matchBearer(r.Header.Get("Authorization"), token) {
		s.auditAuth(r, "deny", "bearer", "", nil, "missing or invalid bearer token")
		return errors.New("missing or invalid bearer token")
	}
	s.auditAuth(r, "allow", "bearer", "static-token", nil, "")
	return nil
}

func (s *Server) authorizeIngest(r *http.Request, body []byte) error {
	if s.rateLimiter != nil && !s.rateLimiter.Allow(r, "ingest") {
		s.auditAuth(r, "deny", "rate_limit", "", nil, "ingest rate limit exceeded")
		return errRateLimited
	}
	if err := s.authorizeRoles(r, "admin"); err != nil {
		s.auditAuth(r, "deny", "roles", "", nil, err.Error())
		return err
	}
	ingestToken := strings.TrimSpace(s.auth.Ingest.Bearer.Token)
	if ingestToken == "" {
		ingestToken = strings.TrimSpace(s.auth.Read.Token)
	}
	if ingestToken != "" && !matchBearer(r.Header.Get("Authorization"), ingestToken) {
		s.auditAuth(r, "deny", "bearer", "", nil, "missing or invalid bearer token")
		return errors.New("missing or invalid bearer token")
	}
	if strings.TrimSpace(s.auth.Ingest.GenericWebhook.Secret) != "" {
		header := strings.TrimSpace(r.Header.Get(s.auth.Ingest.GenericWebhook.Header))
		if !validWebhookSignature(s.auth.Ingest.GenericWebhook.Secret, body, header) {
			s.auditAuth(r, "deny", "webhook-signature", "", nil, "invalid webhook signature")
			return errors.New("invalid webhook signature")
		}
	}
	s.auditAuth(r, "allow", "ingest", "", nil, "")
	return nil
}

func (s *Server) authorizeExport(r *http.Request) error {
	if s.rateLimiter != nil && !s.rateLimiter.Allow(r, "export") {
		s.auditAuth(r, "deny", "rate_limit", "", nil, "export rate limit exceeded")
		return errRateLimited
	}
	if err := s.authorizeRoles(r, "exporter", "admin"); err != nil {
		return err
	}
	return s.authorizeReadToken(r)
}

func withAuthDefaults(in AuthConfig) AuthConfig {
	if strings.TrimSpace(in.Ingest.GenericWebhook.Header) == "" {
		in.Ingest.GenericWebhook.Header = "X-Evidra-Signature"
	}
	if strings.TrimSpace(in.OIDC.RolesHeader) == "" {
		in.OIDC.RolesHeader = "X-Auth-Roles"
	}
	if strings.TrimSpace(in.JWT.RolesClaim) == "" {
		in.JWT.RolesClaim = "roles"
	}
	if strings.TrimSpace(in.JWT.JWKSRefresh) == "" {
		in.JWT.JWKSRefresh = "5m"
	}
	if in.Rate.ReadPerMinute <= 0 {
		in.Rate.ReadPerMinute = 600
	}
	if in.Rate.ExportPerMinute <= 0 {
		in.Rate.ExportPerMinute = 120
	}
	if in.Rate.IngestPerMinute <= 0 {
		in.Rate.IngestPerMinute = 240
	}
	return in
}

func matchBearer(header, expected string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	return strings.TrimSpace(parts[1]) == expected
}

func (s *Server) authorizeRoles(r *http.Request, allowed ...string) error {
	source, subject, roles, enforced, err := s.resolveRoles(r)
	if err != nil {
		return err
	}
	if !enforced {
		return nil
	}
	roleSet := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		roleSet[strings.ToLower(strings.TrimSpace(role))] = struct{}{}
	}
	for _, needed := range allowed {
		if _, ok := roleSet[strings.ToLower(strings.TrimSpace(needed))]; ok {
			s.auditAuth(r, "allow", source, subject, roles, "")
			return nil
		}
	}
	return errors.New("insufficient role")
}
