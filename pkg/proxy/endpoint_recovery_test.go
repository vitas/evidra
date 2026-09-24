package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type failingReplacementEvidence struct {
	epNopEvidence
	failAt string
}

func (f failingReplacementEvidence) replaced(string, string, string, epOperation) error {
	if f.failAt == "replace" {
		return errors.New("replacement append failed")
	}
	return nil
}

func (f failingReplacementEvidence) prescribed(string, epOperation, []string) error {
	if f.failAt == "prescribe" {
		return errors.New("prescription append failed")
	}
	return nil
}

func TestFailedReplacementNeverLeavesUnacknowledgedOperationOpen(t *testing.T) {
	for _, failAt := range []string{"replace", "prescribe"} {
		t.Run(failAt, func(t *testing.T) {
			e := &epEndpoint{
				evidence: failingReplacementEvidence{failAt: failAt},
				state: &epSession{
					open:       &epOperation{ID: "EV-old", Objective: "Original work"},
					prescribes: 1,
				},
			}
			_, protocolErr := e.handlePrescribe(json.RawMessage(`{"objective":"New work","abandon_and_replace":true}`))
			if protocolErr == nil {
				t.Fatal("replacement succeeded despite evidence append failure")
			}
			data, ok := protocolErr.Data.(map[string]any)
			if !ok || data["error"] != "recorder_unhealthy" {
				t.Fatalf("replacement error = %v, want recorder_unhealthy", protocolErr)
			}
			if got := e.openOperationID(); got != "" {
				t.Fatalf("unacknowledged operation %q remained open after %s append failed", got, failAt)
			}
			if e.state.prescribes != 1 || e.state.replacements != 0 {
				t.Fatalf("failed replacement counted as successful: %+v", e.state)
			}
			instruction, _ := data["instruction"].(string)
			if !strings.Contains(instruction, "new endpoint process") {
				t.Fatalf("recovery instruction assumes this process can resume: %q", instruction)
			}
		})
	}
}

func TestRecorderUnavailableExplainsProcessRestart(t *testing.T) {
	var out bytes.Buffer
	e := &epEndpoint{client: bufio.NewWriter(&out)}
	if err := e.recorderUnavailable(&epMsg{ID: json.RawMessage("1")}, "get_status", "store unhealthy"); err != nil {
		t.Fatal(err)
	}
	var frame struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &frame); err != nil {
		t.Fatal(err)
	}
	if !frame.Result.IsError || len(frame.Result.Content) != 1 {
		t.Fatalf("unexpected refusal: %+v", frame.Result)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(frame.Result.Content[0].Text), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "recorder_unhealthy" {
		t.Fatalf("error = %v", body["error"])
	}
	instruction, _ := body["instruction"].(string)
	if !strings.Contains(instruction, "new endpoint process") {
		t.Fatalf("refusal instructs an impossible same-process retry: %q", instruction)
	}
}
