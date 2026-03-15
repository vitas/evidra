package export

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"samebits.com/evidra/pkg/evidence"
	"samebits.com/evidra/pkg/version"
)

// BundleManifest describes the exported bundle.
type BundleManifest struct {
	BundleVersion string `json:"bundle_version"`
	Anonymized    bool   `json:"anonymized"`
	SaltHint      string `json:"salt_hint,omitempty"`
	EntryCount    int    `json:"entry_count"`
	EvidraVersion string `json:"evidra_version"`
	SpecVersion   string `json:"spec_version"`
	ExportedAt    string `json:"exported_at"`
}

// BundleMetadata summarizes the exported evidence.
type BundleMetadata struct {
	TotalOperations int            `json:"total_operations"`
	SignalSummary   map[string]int `json:"signal_summary"`
	Actors          []string       `json:"actors"`
	Tools           []string       `json:"tools"`
	ScopeClasses    []string       `json:"scope_classes"`
	TimeRange       *TimeRange     `json:"time_range,omitempty"`
}

// TimeRange is the first and last timestamp in the evidence.
type TimeRange struct {
	First string `json:"first"`
	Last  string `json:"last"`
}

// Options configures the export.
type Options struct {
	EvidenceDir      string
	OutputDir        string
	Anonymize        bool
	IncludeScorecard bool
}

// Export reads evidence, anonymizes it, and writes a bundle directory.
func Export(opts Options) error {
	entries, err := loadAllEntries(opts.EvidenceDir)
	if err != nil {
		return fmt.Errorf("export: load evidence: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("export: no evidence entries found in %s", opts.EvidenceDir)
	}

	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return fmt.Errorf("export: mkdir: %w", err)
	}

	var anon *Anonymizer
	if opts.Anonymize {
		anon = NewAnonymizer()
	}

	// Process entries
	var processed []evidence.EvidenceEntry
	for _, e := range entries {
		if anon != nil {
			e = anon.AnonymizeEntry(e)
		}
		processed = append(processed, e)
	}

	// Write evidence.jsonl
	evidencePath := filepath.Join(opts.OutputDir, "evidence.jsonl")
	if err := writeEntriesJSONL(evidencePath, processed); err != nil {
		return err
	}

	// Write manifest.json
	manifest := BundleManifest{
		BundleVersion: "1.0",
		Anonymized:    opts.Anonymize,
		EntryCount:    len(processed),
		EvidraVersion: version.Version,
		SpecVersion:   version.SpecVersion,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	if anon != nil {
		manifest.SaltHint = anon.SaltHint()
	}
	if err := writeJSON(filepath.Join(opts.OutputDir, "manifest.json"), manifest); err != nil {
		return err
	}

	// Write metadata.json
	meta := buildMetadata(processed)
	if err := writeJSON(filepath.Join(opts.OutputDir, "metadata.json"), meta); err != nil {
		return err
	}

	// Copy scorecard if requested
	if opts.IncludeScorecard {
		copyScorecard(opts.EvidenceDir, opts.OutputDir)
	}

	return nil
}

func loadAllEntries(evidenceDir string) ([]evidence.EvidenceEntry, error) {
	var entries []evidence.EvidenceEntry

	err := filepath.WalkDir(evidenceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		fileEntries, err := parseJSONLFile(path)
		if err != nil {
			return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
		}
		entries = append(entries, fileEntries...)
		return nil
	})

	return entries, err
}

func parseJSONLFile(path string) ([]evidence.EvidenceEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []evidence.EvidenceEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry evidence.EvidenceEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // skip malformed entries
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

func buildMetadata(entries []evidence.EvidenceEntry) BundleMetadata {
	meta := BundleMetadata{
		SignalSummary: make(map[string]int),
	}
	actors := map[string]bool{}
	tools := map[string]bool{}
	scopes := map[string]bool{}
	var first, last time.Time

	for _, e := range entries {
		if first.IsZero() || e.Timestamp.Before(first) {
			first = e.Timestamp
		}
		if e.Timestamp.After(last) {
			last = e.Timestamp
		}

		if e.Actor.ID != "" {
			actors[e.Actor.ID] = true
		}

		switch e.Type {
		case evidence.EntryTypePrescribe:
			meta.TotalOperations++
			extractToolAndScope(e.Payload, tools, scopes)
		case evidence.EntryTypeSignal:
			var sig struct {
				SignalName string `json:"signal_name"`
			}
			if json.Unmarshal(e.Payload, &sig) == nil && sig.SignalName != "" {
				meta.SignalSummary[sig.SignalName]++
			}
		}
	}

	for a := range actors {
		meta.Actors = append(meta.Actors, a)
	}
	for t := range tools {
		meta.Tools = append(meta.Tools, t)
	}
	for s := range scopes {
		meta.ScopeClasses = append(meta.ScopeClasses, s)
	}

	if !first.IsZero() {
		meta.TimeRange = &TimeRange{
			First: first.Format(time.RFC3339),
			Last:  last.Format(time.RFC3339),
		}
	}

	return meta
}

func extractToolAndScope(payload json.RawMessage, tools, scopes map[string]bool) {
	var p struct {
		CanonicalAction struct {
			Tool       string `json:"tool"`
			ScopeClass string `json:"scope_class"`
		} `json:"canonical_action"`
	}
	if json.Unmarshal(payload, &p) == nil {
		if p.CanonicalAction.Tool != "" {
			tools[p.CanonicalAction.Tool] = true
		}
		if p.CanonicalAction.ScopeClass != "" {
			scopes[p.CanonicalAction.ScopeClass] = true
		}
	}
}

func copyScorecard(evidenceDir, outputDir string) {
	// Try to find scorecard.json in or near the evidence dir
	candidates := []string{
		filepath.Join(evidenceDir, "scorecard.json"),
		filepath.Join(filepath.Dir(evidenceDir), "scorecard.json"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		os.WriteFile(filepath.Join(outputDir, "scorecard.json"), data, 0644)
		return
	}
}

func writeEntriesJSONL(path string, entries []evidence.EvidenceEntry) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		f.Write(line)
		f.Write([]byte("\n"))
	}
	return nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
