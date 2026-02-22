package cloudevents

import (
	"fmt"
	"time"
)

// Validate checks required fields of a StoredEvent. It defaults Time to now() if missing.
func Validate(e *StoredEvent) error {
	if e.ID == "" {
		return fmt.Errorf("id is required")
	}
	if e.Source == "" {
		return fmt.Errorf("source is required")
	}
	if e.Type == "" {
		return fmt.Errorf("type is required")
	}
	if len(e.Data) == 0 || string(e.Data) == "null" {
		return fmt.Errorf("data is required")
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	return nil
}
