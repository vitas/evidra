package proxy

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
)

type epFailingWriter struct{ err error }

func (w epFailingWriter) Write(_ []byte) (int, error) { return 0, w.err }

// The relay's writer tests went away with the relay, so the endpoint's own write
// paths are covered here rather than left asserted by deleted code: a failed write
// must surface as an error with the direction it happened in, because a wrapper that
// silently drops frames on a closed pipe would look to the agent like an upstream
// that stopped answering.
func TestWriteClientPropagatesWriterError(t *testing.T) {
	want := errors.New("client pipe closed")
	e := &epEndpoint{client: bufio.NewWriter(epFailingWriter{want})}
	err := e.writeClient([]byte(`{"jsonrpc":"2.0"}` + "\n"))
	if !errors.Is(err, want) {
		t.Fatalf("writeClient error = %v, want wrapped %v", err, want)
	}
	if !strings.Contains(err.Error(), "write client") {
		t.Errorf("error does not name the direction: %v", err)
	}
}

func TestWriteUpstreamPropagatesWriterError(t *testing.T) {
	want := errors.New("upstream exited")
	e := &epEndpoint{childW: bufio.NewWriter(epFailingWriter{want})}
	err := e.writeUpstream([]byte(`{"jsonrpc":"2.0"}` + "\n"))
	if !errors.Is(err, want) {
		t.Fatalf("writeUpstream error = %v, want wrapped %v", err, want)
	}
	if !strings.Contains(err.Error(), "write upstream") {
		t.Errorf("error does not name the direction: %v", err)
	}
}

// Both writers must flush, and only once: an endpoint that wrote responses into a
// buffer it never flushed would deliver nothing while reporting success.
func TestWritersFlushExactlyOnceOnSuccess(t *testing.T) {
	var clientBuf, upstreamBuf bytes.Buffer
	e := &epEndpoint{client: bufio.NewWriter(&clientBuf), childW: bufio.NewWriter(&upstreamBuf)}
	frame := []byte(`{"jsonrpc":"2.0","id":1}` + "\n")
	if err := e.writeClient(frame); err != nil {
		t.Fatal(err)
	}
	if err := e.writeUpstream(frame); err != nil {
		t.Fatal(err)
	}
	if clientBuf.String() != string(frame) || upstreamBuf.String() != string(frame) {
		t.Errorf("after write: client=%q upstream=%q, want the frame flushed to both",
			clientBuf.String(), upstreamBuf.String())
	}
	if err := e.client.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := clientBuf.String(); got != string(frame) {
		t.Errorf("second flush duplicated the frame: %q", got)
	}
}
