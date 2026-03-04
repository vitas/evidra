package evidence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// streamFileEntries reads a JSONL file line by line, unmarshalling each
// non-empty line as an EvidenceEntry and passing it to fn with a 1-based
// line number.
func streamFileEntries(path string, fn func(EvidenceEntry, int) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry EvidenceEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return fmt.Errorf("parse JSONL line %d: %w", lineNo, err)
		}
		if err := fn(entry, lineNo); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read JSONL: %w", err)
	}
	return nil
}

// appendEntryLine marshals an EvidenceEntry to JSON and appends it as a
// single line to the file at path, creating the file if it does not exist.
func appendEntryLine(path string, entry EvidenceEntry) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open evidence log: %w", err)
	}
	defer f.Close()

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append entry: %w", err)
	}
	return nil
}
