package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var issueKeyPattern = regexp.MustCompile(`\b[A-Z][A-Z0-9]+-\d+\b`)

func MakeID(provider, delivery string, n int) string {
	delivery = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, delivery)
	return fmt.Sprintf("evt_%s_%s_%d", provider, delivery, n)
}

func FallbackDelivery(delivery string, body []byte) string {
	delivery = strings.TrimSpace(delivery)
	if delivery != "" {
		return delivery
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:8])
}

func RepoApp(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "unknown"
	}
	parts := strings.Split(repo, "/")
	return parts[len(parts)-1]
}

func NonEmpty(a, b string) string {
	a = strings.TrimSpace(a)
	if a != "" {
		return a
	}
	return strings.TrimSpace(b)
}

func ParseTimeOrNow(v string) time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Now().UTC()
	}
	formats := []string{time.RFC3339Nano, time.RFC3339}
	for _, f := range formats {
		if t, err := time.Parse(f, v); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

func NormalizeEventType(in string) string {
	return strings.TrimSpace(strings.ToLower(in))
}

func ExtractIssueKeys(values ...string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, value := range values {
		matches := issueKeyPattern.FindAllString(strings.ToUpper(strings.TrimSpace(value)), -1)
		for _, match := range matches {
			if _, exists := seen[match]; exists {
				continue
			}
			seen[match] = struct{}{}
			out = append(out, match)
		}
	}
	return out
}
