package evidence

import (
	"github.com/oklog/ulid/v2"
)

// GenerateTraceID creates a new trace_id as a ULID.
// MCP server should call this once at startup; CLI should call once per invocation.
func GenerateTraceID() string {
	return ulid.Make().String()
}
