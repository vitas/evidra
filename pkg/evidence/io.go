package evidence

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func streamFileRecords(path string, fn func(Record, int) error) error {
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
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return fmt.Errorf("parse JSONL line %d: %w", lineNo, err)
		}
		if err := fn(rec, lineNo); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read JSONL: %w", err)
	}
	return nil
}

func appendRecordLine(path string, record Record) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open evidence log: %w", err)
	}
	defer f.Close()

	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append record: %w", err)
	}
	return nil
}
