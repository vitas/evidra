package analytics

import (
	"embed"
	"encoding/json"
	"fmt"

	"samebits.com/evidra/internal/score"
)

type publicSignalManifest struct {
	Signals []string `json:"signals"`
}

//go:embed public_signals.v1.1.0.json
var publicSignalFiles embed.FS

var stablePublicSignalNames = mustLoadPublicSignalManifest()

// PublicSignalNames returns the stable public signal order used by analytics views and API responses.
func PublicSignalNames(profile score.Profile) []string {
	_ = profile

	names := make([]string, len(stablePublicSignalNames))
	copy(names, stablePublicSignalNames)
	return names
}

func mustLoadPublicSignalManifest() []string {
	data, err := publicSignalFiles.ReadFile("public_signals.v1.1.0.json")
	if err != nil {
		panic(fmt.Sprintf("read public signal manifest: %v", err))
	}
	signals, err := decodePublicSignalManifest(data)
	if err != nil {
		panic(fmt.Sprintf("decode public signal manifest: %v", err))
	}
	return signals
}

func decodePublicSignalManifest(data []byte) ([]string, error) {
	var manifest publicSignalManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse public signal manifest: %w", err)
	}
	if len(manifest.Signals) == 0 {
		return nil, fmt.Errorf("parse public signal manifest: signals must not be empty")
	}

	signals := make([]string, 0, len(manifest.Signals))
	seen := make(map[string]struct{}, len(manifest.Signals))
	for _, name := range manifest.Signals {
		if name == "" {
			return nil, fmt.Errorf("parse public signal manifest: signal name must not be empty")
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("parse public signal manifest: duplicate signal %q", name)
		}
		seen[name] = struct{}{}
		signals = append(signals, name)
	}
	return signals, nil
}
