package logpipe

import (
	"bufio"
	"compress/zlib"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Logstash does not speak HTTP: filebeat talks to it over Lumberjack v2, a
// binary framing protocol with a window and per-frame ACKs. Implementing it is
// the only way to be a drop-in replacement for a host that already ships logs to
// Logstash.
//
//	handshake : client "2W" + uint16(window)  →  server "2W" + uint16(window)
//	frame     : version(1)=2 | type(1) | len(uint32BE) | payload | seq(uint32BE)*
//	types     : 'W' window, 'C' compressed block, 'J' json, 'D' data
//	ack       : version(2) | type('2') | seq(uint32BE)
//
// The ACK is what lets the pipeline advance the file offset safely: an unacked
// batch is re-sent after a reconnect, which is at-least-once.
const (
	ljVersion   = 2
	ljWinFrame  = 'W'
	ljCompFrame = 'C'
	ljJSONFrame = 'J'
	ljDataFrame = 'D'
	ljAckFrame  = '2'
)

type logstashOutput struct {
	spec OutputSpec
	host string

	mu   sync.Mutex
	conn net.Conn
	br   *bufio.Reader
	seq  uint32

	sent    int64
	failed  int64
	lastErr atomic.Value
	lastAt  atomic.Value
}

func newLogstashOutput(spec OutputSpec) (*logstashOutput, error) {
	if len(spec.Hosts) == 0 {
		return nil, fmt.Errorf("logstash 缺少 hosts")
	}
	return &logstashOutput{spec: spec, host: spec.Hosts[0]}, nil
}

func (o *logstashOutput) Name() string { return "logstash" }

func (o *logstashOutput) Publish(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	if err := o.ensureConn(ctx); err != nil {
		return o.fail(err)
	}

	// Compress one block and send it as a single 'C' frame: fewer syscalls and
	// closer to what filebeat does with compression_level > 0.
	var payload []byte
	for _, ev := range events {
		body, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		o.seq++
		frame := make([]byte, 0, len(body)+16)
		frame = append(frame, ljVersion, ljJSONFrame)
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(body)))
		frame = append(frame, l[:]...)
		frame = append(frame, body...)
		binary.BigEndian.PutUint32(l[:], o.seq)
		frame = append(frame, l[:]...)
		payload = append(payload, frame...)
	}
	if len(payload) == 0 {
		return nil
	}

	if err := o.writeFrames(payload); err != nil {
		o.closeLocked()
		return o.fail(err)
	}
	if err := o.waitAck(ctx, o.seq); err != nil {
		o.closeLocked()
		return o.fail(err)
	}
	atomic.AddInt64(&o.sent, int64(len(events)))
	o.lastAt.Store(time.Now())
	return nil
}

// ensureConn dials and performs the handshake when there is no live connection.
func (o *logstashOutput) ensureConn(ctx context.Context) error {
	if o.conn != nil {
		return nil
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", o.host)
	if err != nil {
		return err
	}
	if tlsCfg, terr := tlsConfig(o.spec); terr == nil && tlsCfg != nil {
		tc := tls.Client(conn, tlsCfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return err
		}
		conn = tc
	}
	o.conn = conn
	o.br = bufio.NewReaderSize(conn, 64<<10)

	// Handshake: both sides announce the same window size (we use 1 because we
	// wait for the ACK of every batch; a larger window only helps pipelining,
	// which this synchronous design does not do).
	if err := writeWindow(conn, 1); err != nil {
		o.closeLocked()
		return err
	}
	if err := o.readHandshake(); err != nil {
		o.closeLocked()
		return err
	}
	return nil
}

func (o *logstashOutput) readHandshake() error {
	hdr := make([]byte, 2)
	if _, err := io.ReadFull(o.br, hdr); err != nil {
		return err
	}
	if hdr[0] != '2' || hdr[1] != ljWinFrame {
		return fmt.Errorf("lumberjack 握手失败：%q", hdr)
	}
	var win [2]byte
	if _, err := io.ReadFull(o.br, win[:]); err != nil {
		return err
	}
	return nil
}

func writeWindow(w io.Writer, n uint16) error {
	buf := make([]byte, 4)
	buf[0], buf[1] = '2', ljWinFrame
	binary.BigEndian.PutUint16(buf[2:], n)
	_, err := w.Write(buf)
	return err
}

// writeFrames sends the block, compressing it when the payload is worth it.
func (o *logstashOutput) writeFrames(payload []byte) error {
	level := o.spec.CompressionLevel
	if level <= 0 {
		level = 3
	}
	var block []byte
	if level > 0 && len(payload) > 512 {
		var buf writerBuffer
		zw, _ := zlib.NewWriterLevel(&buf, level)
		if _, err := zw.Write(payload); err == nil && zw.Close() == nil {
			block = buf.b
		}
	}
	typ := byte(ljJSONFrame)
	if block == nil {
		// Uncompressed: wrap the already-serialized frames in a 'C'-less path by
		// sending the raw frames sliced back out is not worth it; compress with
		// level 0 (stored) to keep a single wire format.
		var buf writerBuffer
		zw, _ := zlib.NewWriterLevel(&buf, zlib.NoCompression)
		if _, err := zw.Write(payload); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		block = buf.b
	}
	typ = ljCompFrame

	hdr := make([]byte, 6)
	hdr[0], hdr[1] = ljVersion, typ
	binary.BigEndian.PutUint32(hdr[2:], uint32(len(block)))
	if _, err := o.conn.Write(hdr); err != nil {
		return err
	}
	_, err := o.conn.Write(block)
	return err
}

// waitAck reads ACK frames until the server confirms the last sequence number.
func (o *logstashOutput) waitAck(ctx context.Context, want uint32) error {
	deadline := time.Now().Add(30 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = o.conn.SetReadDeadline(deadline)
	for {
		hdr := make([]byte, 2)
		if _, err := io.ReadFull(o.br, hdr); err != nil {
			return fmt.Errorf("等待 ACK 失败：%w", err)
		}
		if hdr[0] != ljVersion {
			return fmt.Errorf("lumberjack 版本不符：%d", hdr[0])
		}
		switch hdr[1] {
		case ljAckFrame:
			var seq [4]byte
			if _, err := io.ReadFull(o.br, seq[:]); err != nil {
				return err
			}
			if binary.BigEndian.Uint32(seq[:]) >= want {
				return nil
			}
		case ljWinFrame:
			var win [2]byte
			if _, err := io.ReadFull(o.br, win[:]); err != nil {
				return err
			}
		default:
			// Unknown frame with a length prefix: skip it instead of desyncing.
			var l [4]byte
			if _, err := io.ReadFull(o.br, l[:]); err != nil {
				return err
			}
			if _, err := io.CopyN(io.Discard, o.br, int64(binary.BigEndian.Uint32(l[:]))); err != nil {
				return err
			}
		}
	}
}

func (o *logstashOutput) closeLocked() {
	if o.conn != nil {
		_ = o.conn.Close()
		o.conn = nil
		o.br = nil
	}
}

func (o *logstashOutput) fail(err error) error {
	atomic.AddInt64(&o.failed, 1)
	o.lastErr.Store(err.Error())
	o.lastAt.Store(time.Now())
	return err
}

func (o *logstashOutput) Stats() OutputStats {
	st := OutputStats{Sent: atomic.LoadInt64(&o.sent), Failed: atomic.LoadInt64(&o.failed)}
	if v, ok := o.lastErr.Load().(string); ok {
		st.LastErr = v
	}
	if v, ok := o.lastAt.Load().(time.Time); ok {
		st.LastAt = v
	}
	return st
}

func (o *logstashOutput) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closeLocked()
	return nil
}

// writerBuffer avoids pulling bytes.Buffer into the hot path twice.
type writerBuffer struct{ b []byte }

func (w *writerBuffer) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}
