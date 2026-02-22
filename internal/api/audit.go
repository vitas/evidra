package api

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type auditEvent struct {
	Time          string   `json:"time"`
	Decision      string   `json:"decision"`
	Mechanism     string   `json:"mechanism"`
	Actor         string   `json:"actor,omitempty"`
	Roles         []string `json:"roles,omitempty"`
	Method        string   `json:"method"`
	Path          string   `json:"path"`
	RemoteIP      string   `json:"remote_ip,omitempty"`
	RequestID     string   `json:"request_id,omitempty"`
	CorrelationID string   `json:"correlation_id,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

func (s *Server) auditAuth(r *http.Request, decision, mechanism, actor string, roles []string, reason string) {
	ev := auditEvent{
		Time:          time.Now().UTC().Format(time.RFC3339),
		Decision:      strings.TrimSpace(decision),
		Mechanism:     strings.TrimSpace(mechanism),
		Actor:         strings.TrimSpace(actor),
		Roles:         roles,
		Method:        r.Method,
		Path:          r.URL.Path,
		RemoteIP:      requestRemoteIP(r),
		RequestID:     strings.TrimSpace(r.Header.Get("X-Request-Id")),
		CorrelationID: strings.TrimSpace(r.Header.Get("X-Correlation-Id")),
		Reason:        strings.TrimSpace(reason),
	}
	b, err := json.Marshal(ev)
	if err != nil {
		log.Printf("audit_auth decision=%s mechanism=%s path=%s reason=%s", ev.Decision, ev.Mechanism, ev.Path, ev.Reason)
		return
	}
	line := "audit_auth " + string(b)
	log.Print(line)
	s.writeAuditLine(line)
}

func requestRemoteIP(r *http.Request) string {
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func (s *Server) writeAuditLine(line string) {
	path := strings.TrimSpace(s.auth.Audit.LogFile)
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		log.Printf("audit_auth_file_error path=%s err=%v", path, err)
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line + "\n")
}
