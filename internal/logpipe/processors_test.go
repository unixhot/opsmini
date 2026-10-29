package logpipe

import (
	"bufio"
	"compress/zlib"
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func mustProc(t *testing.T, typ string, params map[string]any) Processor {
	t.Helper()
	p, err := newProcessor(ProcSpec{Type: typ, Params: params})
	if err != nil {
		t.Fatalf("%s: %v", typ, err)
	}
	return p
}

func TestDissectNginx(t *testing.T) {
	p := mustProc(t, "dissect", map[string]any{
		"tokenizer": `%{clientip} - - [%{ts}] "%{method} %{path} HTTP/%{version}" %{status}`,
		"field":     "message",
	})
	ev := Event{"message": `10.1.2.3 - - [29/Oct/2026:09:10:01 +0800] "GET /api/a HTTP/1.1" 200`}
	out := p.Apply(ev)[0]
	if out["clientip"] != "10.1.2.3" {
		t.Fatalf("clientip = %v", out["clientip"])
	}
	if out["method"] != "GET" || out["path"] != "/api/a" {
		t.Fatalf("method/path = %v %v", out["method"], out["path"])
	}
	if out["status"] != "200" {
		t.Fatalf("status = %v", out["status"])
	}
}

func TestDissectSkipAndKey(t *testing.T) {
	// %{?skip} discards, %{&key} turns the value into a field name.
	p := mustProc(t, "dissect", map[string]any{
		"tokenizer": `%{?ignore} %{&name} %{value}`,
		"field":     "message",
	})
	ev := Event{"message": "skip status 200"}
	out := p.Apply(ev)[0]
	if out["status"] != "200" {
		t.Fatalf("key modifier not applied: %v", out)
	}
	if _, ok := out["ignore"]; ok {
		t.Fatalf("skip field leaked")
	}
}

func TestScriptProcessor(t *testing.T) {
	p := mustProc(t, "script", map[string]any{
		"lang": "javascript",
		"source": `
			var st = event.Get("status");
			if (st != null && st >= 500) { event.Put("level", "error"); } else { event.Put("level", "info"); }
			event.Append("tags", "scripted");
		`,
	})
	ev := Event{"status": int64(503)}
	out := p.Apply(ev)[0]
	if out["level"] != "error" {
		t.Fatalf("level = %v", out["level"])
	}
	tags, _ := out["tags"].([]any)
	if len(tags) != 1 || tags[0] != "scripted" {
		t.Fatalf("tags = %v", out["tags"])
	}
}

func TestScriptCancel(t *testing.T) {
	p := mustProc(t, "script", map[string]any{
		"lang":   "javascript",
		"source": `event.Cancel();`,
	})
	if got := p.Apply(Event{"message": "x"}); len(got) != 0 {
		t.Fatalf("cancel should drop the event")
	}
}

func TestRegisteredDomainAndSyslog(t *testing.T) {
	rd := mustProc(t, "registered_domain", map[string]any{"field": "url.full"})
	ev := rd.Apply(Event{"url": Event{"full": "https://a.b.example.co.uk/x"}})[0]
	if v, _ := getPath(ev, "url.domain"); v != "example.co.uk" {
		t.Fatalf("registered domain = %v", v)
	}

	sl := mustProc(t, "syslog", map[string]any{"field": "message", "format": "rfc3164"})
	ev = sl.Apply(Event{"message": "<34>Oct 29 09:10:01 web1 sshd[123]: Accepted publickey"})[0]
	if v, _ := getPath(ev, "syslog.program"); v != "sshd" {
		t.Fatalf("syslog program = %v", v)
	}
	if v, _ := getPath(ev, "syslog.severity"); v != 2 {
		t.Fatalf("syslog severity = %v", v)
	}
}

func TestCommunityIDIsDirectionIndependent(t *testing.T) {
	p := mustProc(t, "community_id", map[string]any{})
	a := p.Apply(Event{
		"source": Event{"ip": "10.0.0.1", "port": int64(5000)},
		"destination": Event{"ip": "10.0.0.2", "port": int64(80)},
		"network": Event{"iana_number": int64(6)},
	})[0]
	b := p.Apply(Event{
		"source": Event{"ip": "10.0.0.2", "port": int64(80)},
		"destination": Event{"ip": "10.0.0.1", "port": int64(5000)},
		"network": Event{"iana_number": int64(6)},
	})[0]
	ida, _ := getPath(a, "network.community_id")
	idb, _ := getPath(b, "network.community_id")
	if ida == "" || ida != idb {
		t.Fatalf("community id must be symmetric: %v vs %v", ida, idb)
	}
}

// A fake Logstash: performs the handshake, decompresses the block, ACKs it.
func TestLogstashLumberjackRoundTrip(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("listen: %v", err)
	}
	defer ln.Close()

	got := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		br := bufio.NewReader(conn)
		// handshake: client "2W"+win, we answer the same
		hdr := make([]byte, 4)
		if _, err := io.ReadFull(br, hdr); err != nil {
			return
		}
		_, _ = conn.Write(hdr)
		// one frame
		fh := make([]byte, 6)
		if _, err := io.ReadFull(br, fh); err != nil {
			return
		}
		body := make([]byte, binary.BigEndian.Uint32(fh[2:]))
		if _, err := io.ReadFull(br, body); err != nil {
			return
		}
		zr, err := zlib.NewReader(newReader(body))
		if err != nil {
			return
		}
		out, _ := io.ReadAll(zr)
		_ = zr.Close()
		got <- string(out)
		// ACK the highest seq we saw
		seq := uint32(0)
		for i := 0; i+10 <= len(out); i++ {
			if out[i] == ljVersion && out[i+1] == ljJSONFrame {
				l := binary.BigEndian.Uint32(out[i+2:])
				seq = binary.BigEndian.Uint32(out[i+6+int(l):])
			}
		}
		ack := make([]byte, 6)
		ack[0], ack[1] = ljVersion, ljAckFrame
		binary.BigEndian.PutUint32(ack[2:], seq)
		_, _ = conn.Write(ack)
	}()

	out, err := newLogstashOutput(OutputSpec{Type: "logstash", Hosts: []string{ln.Addr().String()}, CompressionLevel: 3})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := out.Publish(ctx, []Event{{"message": "hello-logstash"}}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case body := <-got:
		if !contains(body, "hello-logstash") {
			t.Fatalf("server did not receive the event: %q", body)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("server received nothing")
	}
	if out.Stats().Sent != 1 {
		t.Fatalf("sent counter = %d", out.Stats().Sent)
	}
}

func TestSpoolWriteAndReplay(t *testing.T) {
	dir := t.TempDir()
	sp := newSpool(dir, 1)
	batch := []Event{{"message": "a"}, {"message": "b"}}
	if err := sp.Write(batch); err != nil {
		t.Fatalf("write: %v", err)
	}
	if sp.Pending() != 1 {
		t.Fatalf("pending = %d", sp.Pending())
	}
	var replayed [][]Event
	n := sp.Replay(context.Background(), func(ev []Event) error {
		replayed = append(replayed, ev)
		return nil
	})
	if n != 1 || len(replayed) != 1 || len(replayed[0]) != 2 {
		t.Fatalf("replay = %d batches %d", n, len(replayed))
	}
	if sp.Pending() != 0 {
		t.Fatalf("spool not drained: %d", sp.Pending())
	}
}

// A failing publish must keep the file so it can be retried.
func TestSpoolKeepsBatchOnFailure(t *testing.T) {
	dir := t.TempDir()
	sp := newSpool(dir, 1)
	_ = sp.Write([]Event{{"message": "x"}})
	n := sp.Replay(context.Background(), func([]Event) error { return io.ErrUnexpectedEOF })
	if n != 0 || sp.Pending() != 1 {
		t.Fatalf("failed replay must keep the file: n=%d pending=%d", n, sp.Pending())
	}
}

type sliceReader struct {
	b []byte
	i int
}

func newReader(b []byte) *sliceReader { return &sliceReader{b: b} }
func (r *sliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
