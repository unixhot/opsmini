package logpipe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
)

// kafkaOutput publishes each event as one JSON message.
//
// ProduceSync is used on purpose: the pipeline advances the file offset only
// after the broker acknowledged the batch, and a synchronous produce makes that
// contract obvious. Batching is still handled inside franz-go.
type kafkaOutput struct {
	client *kgo.Client
	topic  string
	sent   int64
	failed int64
	lastErr atomic.Value
	lastAt  atomic.Value
}

func newKafkaOutput(spec OutputSpec) (*kafkaOutput, error) {
	if len(spec.Hosts) == 0 {
		return nil, fmt.Errorf("kafka 缺少 hosts")
	}
	opts := []kgo.Opt{
		kgo.SeedBrokers(spec.Hosts...),
		kgo.DefaultProduceTopic(orDefault(spec.Topic, "opsmini-logs")),
		kgo.ProduceRequestTimeout(30 * time.Second),
		kgo.RecordRetries(3),
	}
	switch spec.Partition {
	case "round_robin":
		opts = append(opts, kgo.RecordPartitioner(kgo.RoundRobinPartitioner()))
	case "random":
		// franz-go has no explicit "random" partitioner; round-robin without
		// keys gives the same user-visible behaviour.
		opts = append(opts, kgo.RecordPartitioner(kgo.RoundRobinPartitioner()))
	default: // hash (filebeat default is hash on the message key)
		opts = append(opts, kgo.RecordPartitioner(kgo.StickyKeyPartitioner(nil)))
	}
	switch spec.Compression {
	case "gzip":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.GzipCompression()))
	case "snappy":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.SnappyCompression()))
	case "lz4":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.Lz4Compression()))
	case "zstd":
		opts = append(opts, kgo.ProducerBatchCompression(kgo.ZstdCompression()))
	}
	if spec.RequiredAck == 0 {
		// filebeat's default is 1 (leader only); -1 means all in-sync replicas.
		opts = append(opts, kgo.RequiredAcks(kgo.LeaderAck()))
	}
	if spec.SASLUser != "" {
		opts = append(opts, kgo.SASL(plain.Auth{User: spec.SASLUser, Pass: spec.SASLPass}.AsMechanism()))
	}
	if tlsCfg, err := tlsConfig(spec); err == nil && tlsCfg != nil {
		opts = append(opts, kgo.DialTLSConfig(tlsCfg))
	}
	cl, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &kafkaOutput{client: cl, topic: orDefault(spec.Topic, "opsmini-logs")}, nil
}

func (o *kafkaOutput) Name() string { return "kafka" }

func (o *kafkaOutput) Publish(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	recs := make([]*kgo.Record, 0, len(events))
	for _, ev := range events {
		body, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		recs = append(recs, &kgo.Record{Topic: o.topic, Value: body})
	}
	if len(recs) == 0 {
		return nil
	}
	results := o.client.ProduceSync(ctx, recs...)
	if err := results.FirstErr(); err != nil {
		atomic.AddInt64(&o.failed, int64(len(recs)))
		o.lastErr.Store(err.Error())
		o.lastAt.Store(time.Now())
		return err
	}
	atomic.AddInt64(&o.sent, int64(len(recs)))
	o.lastAt.Store(time.Now())
	return nil
}

func (o *kafkaOutput) Stats() OutputStats {
	st := OutputStats{Sent: atomic.LoadInt64(&o.sent), Failed: atomic.LoadInt64(&o.failed)}
	if v, ok := o.lastErr.Load().(string); ok {
		st.LastErr = v
	}
	if v, ok := o.lastAt.Load().(time.Time); ok {
		st.LastAt = v
	}
	return st
}

func (o *kafkaOutput) Close() error {
	if o.client != nil {
		o.client.Close()
	}
	return nil
}

// tlsConfig builds a client TLS config from the filebeat ssl.* fields.
func tlsConfig(spec OutputSpec) (*tls.Config, error) {
	if spec.CAFile == "" && spec.CertFile == "" && !spec.Insecure {
		return nil, nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: spec.Insecure}
	if spec.CAFile != "" {
		pem, err := os.ReadFile(spec.CAFile)
		if err != nil {
			return nil, err
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("无法解析 CA 文件 %s", spec.CAFile)
		}
		cfg.RootCAs = pool
	}
	if spec.CertFile != "" && spec.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(spec.CertFile, spec.KeyFile)
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}
