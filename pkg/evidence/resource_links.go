package evidence

import (
	"fmt"
	"path/filepath"
)

// ResourceLink describes a reference to an evidence resource.
type ResourceLink struct {
	URI      string
	Name     string
	MIMEType string
}

// ResourceLinks builds the default link set for an evidence event.
func ResourceLinks(eventID, evidencePath string, includeFileLinks bool) []ResourceLink {
	if eventID == "" {
		return nil
	}
	links := []ResourceLink{{
		URI:      fmt.Sprintf("evidra://event/%s", eventID),
		Name:     "Evidence record",
		MIMEType: "application/json",
	}}
	mode, resolved, err := StorePathInfo(evidencePath)
	if err == nil && mode == "segmented" {
		links = append(links, ResourceLink{URI: "evidra://evidence/manifest", Name: "Evidence manifest", MIMEType: "application/json"})
		links = append(links, ResourceLink{URI: "evidra://evidence/segments", Name: "Evidence segments", MIMEType: "application/json"})
	}
	if includeFileLinks && err == nil {
		if abs, absErr := filepath.Abs(resolved); absErr == nil {
			links = append(links, ResourceLink{URI: "file://" + filepath.ToSlash(abs), Name: "Local evidence path", MIMEType: "text/plain"})
		}
	}
	return links
}

// StorePathInfo returns the store format and resolved path.
func StorePathInfo(path string) (mode string, resolved string, err error) {
	mode, err = StoreFormatAtPath(path)
	if err != nil {
		return "", "", err
	}
	abs, absErr := filepath.Abs(path)
	if absErr != nil {
		resolved = path
	} else {
		resolved = abs
	}
	return mode, resolved, nil
}
