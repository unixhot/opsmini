package logpipe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// Output is one destination. Publish must be safe to call from a single
// goroutine (the pipeline's sender) and must report an error whenever the batch
// was not accepted — the registry only advances on success.
type Output interface {
	Name() string
	Publish(ctx context.Context, events []Event) error
	Stats() OutputStats
	Close() error
}

// OutputStats is surfaced to the panel and to /metrics.
type OutputStats struct {
	Sent    int64     `json:"sent"`
	Failed  int64     `json:"failed"`
	LastErr string    `json:"last_error"`
	LastAt  time.Time `json:"last_at"`
}

// newOutput builds the configured destination.
func newOutput(spec OutputSpec) (Output, error) {
	switch spec.Type {
	case "elasticsearch":
		return newESOutput(spec)
	case "kafka":
		return newKafkaOutput(spec)
	case "logstash":
		return newLogstashOutput(spec)
	}
	return nil, fmt.Errorf("不支持的输出类型 %q", spec.Type)
}

// --- elasticsearch --------------------------------------------------------

type esOutput struct {
	spec    OutputSpec
	client  *http.Client
	next    uint64
	sent    int64
	failed  int64
	lastErr atomic.Value
	lastAt  atomic.Value
}

func newESOutput(spec OutputSpec) (*esOutput, error) {
	if len(spec.Hosts) == 0 {
		return nil, fmt.Errorf("elasticsearch 缺少 hosts")
	}
	return &esOutput{
		spec:   spec,
		client: &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (o *esOutput) Name() string { return "elasticsearch" }

// Publish writes one _bulk request. NDJSON has no way to partially succeed at
// the transport level, so HTTP success is required for the whole batch; the
// per-item errors are counted but do not block the registry (a malformed
// document must not wedge the pipeline forever).
func (o *esOutput) Publish(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, ev := range events {
		index := o.indexFor(ev)
		meta := map[string]any{"index": map[string]any{"_index": index}}
		if o.spec.Pipeline != "" {
			meta["index"].(map[string]any)["pipeline"] = o.spec.Pipeline
		}
		line, err := json.Marshal(meta)
		if err != nil {
			continue
		}
		buf.Write(line)
		buf.WriteByte('\n')
		doc, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		buf.Write(doc)
		buf.WriteByte('\n')
	}
	if buf.Len() == 0 {
		return nil
	}

	host := o.spec.Hosts[atomic.AddUint64(&o.next, 1)%uint64(len(o.spec.Hosts))]
	url := strings.TrimSuffix(host, "/") + "/_bulk"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		o.fail(err)
		return err
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	if o.spec.APIKey != "" {
		req.Header.Set("Authorization", "ApiKey "+o.spec.APIKey)
	} else if o.spec.Username != "" {
		req.SetBasicAuth(o.spec.Username, o.spec.Password)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		o.fail(err)
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		err := fmt.Errorf("bulk failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
		o.fail(err)
		return err
	}
	atomic.AddInt64(&o.sent, int64(len(events)))
	o.lastAt.Store(time.Now())
	// Item-level rejections are real but per-document; log the count so a
	// mapping conflict does not look like a healthy pipeline.
	var parsed struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Status int `json:"status"`
			Error  *struct {
				Type   string `json:"type"`
				Reason string `json:"reason"`
			} `json:"error"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Errors {
		bad := 0
		var reason string
		for _, it := range parsed.Items {
			for _, v := range it {
				if v.Status >= 300 {
					bad++
					if v.Error != nil && reason == "" {
						reason = v.Error.Type + ": " + v.Error.Reason
					}
				}
			}
		}
		if bad > 0 {
			atomic.AddInt64(&o.failed, int64(bad))
			o.lastErr.Store(fmt.Sprintf("%d 条被拒：%s", bad, reason))
			log.Printf("logpipe: elasticsearch 拒绝 %d 条（%s），首因：%s", bad, o.spec.Hosts[0], reason)
		}
	}
	return nil
}

// indexFor expands filebeat's date template (%{+yyyy.MM.dd}) using the event
// timestamp, not the wall clock, so replayed batches land in the right index.
func (o *esOutput) indexFor(ev Event) string {
	pattern := o.spec.Index
	if pattern == "" {
		pattern = "opsmini-%{+yyyy.MM.dd}"
	}
	if !strings.Contains(pattern, "%{") {
		return pattern
	}
	ts := time.Now().UTC()
	if v, ok := ev["@timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			ts = t
		}
	}
	// Supporting only the date format is deliberate: it is the only one real
	// templates use, and a Go-template engine would be a dependency for nothing.
	return strings.ReplaceAll(pattern, "%{+yyyy.MM.dd}", ts.Format("2006.01.02"))
}

func (o *esOutput) fail(err error) {
	atomic.AddInt64(&o.failed, 1)
	o.lastErr.Store(err.Error())
	o.lastAt.Store(time.Now())
}

func (o *esOutput) Stats() OutputStats {
	st := OutputStats{Sent: atomic.LoadInt64(&o.sent), Failed: atomic.LoadInt64(&o.failed)}
	if v, ok := o.lastErr.Load().(string); ok {
		st.LastErr = v
	}
	if v, ok := o.lastAt.Load().(time.Time); ok {
		st.LastAt = v
	}
	return st
}

func (o *esOutput) Close() error { return nil }
