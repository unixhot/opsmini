// Package logpipe is a slimmed-down, filebeat-compatible log collector.
//
// It is not a wrapper: it parses filebeat's own YAML, reads files the same way
// (inode + offset registry, multiline), runs a subset of filebeat processors and
// publishes to Elasticsearch / Kafka / Logstash. The point is that a task written
// for filebeat runs unchanged on OpsMini, so the log platform does not need a
// second configuration format.
//
// Scope decisions live in docs/v1.4-log-pipeline.md: no K8s autodiscover, no
// Windows, three outputs only.
package logpipe

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Multiline merges continuation lines into one event, exactly like filebeat.
type Multiline struct {
	Pattern  string
	Negate   bool
	Match    string // after | before
	MaxLines int
	Timeout  time.Duration
}

// ProcSpec is one entry of the processors list, kept as raw parameters so the
// registry can construct it lazily and report unknown names instead of failing
// the whole task.
type ProcSpec struct {
	Type   string
	Params map[string]any
}

// Input is one filebeat input.
type Input struct {
	Type          string
	ID            string
	Paths         []string
	ExcludeFiles  []string
	Encoding      string
	Fields        map[string]any
	Processors    []ProcSpec
	Multiline     *Multiline
	IgnoreOlder   time.Duration
	CloseInactive time.Duration
	ScanFrequency time.Duration
}

// Task is one采集任务: one file under tasks.d, one input.
type Task struct {
	ID         string
	File       string
	Input      Input
	Output     *OutputSpec // nil = inherit the main config
	Warnings   []string
	LoadedAt   time.Time
	SourceHash string
}

// OutputSpec is the parsed output block.
type OutputSpec struct {
	Type          string // elasticsearch | kafka | logstash
	Hosts         []string
	Index         string
	Username      string
	Password      string
	APIKey        string
	Pipeline      string
	BulkMaxSize   int
	FlushInterval time.Duration
	// kafka
	Topic       string
	Partition   string
	Compression string
	RequiredAck int
	// logstash (lumberjack v2)
	LoadBalance      bool
	CompressionLevel int
	TTL              time.Duration
	// TLS / SASL shared
	CAFile   string
	CertFile string
	KeyFile  string
	Insecure bool
	SASLUser string
	SASLPass string
}

// MainConfig is the pipeline-level configuration (output + tasks.d location).
type MainConfig struct {
	Enabled      bool
	TasksDir     string
	RegistryPath string
	SpoolDir     string
	SpoolMaxMB   int
	ReloadPeriod time.Duration
	Output       OutputSpec
	// AgentType is what lands in agent.type. Defaults to "opsmini"; set it to
	// "filebeat" when an existing ES index template insists on that value.
	AgentType string
	Strict    bool
}

// DefaultMainConfig returns the built-in configuration: pipeline disabled until
// a task exists, everything under the install directory.
func DefaultMainConfig(dataDir string) MainConfig {
	return MainConfig{
		// Off unless something asks for it: a task file in tasks.d, an explicit
		// `logpipeline.enabled: true`, or a LogTask over gRPC. See Manager.Start.
		Enabled:      false,
		TasksDir:     filepath.Join(dataDir, "logs", "tasks.d"),
		RegistryPath: filepath.Join(dataDir, "logs", "registry.db"),
		SpoolDir:     filepath.Join(dataDir, "logs", "spool"),
		SpoolMaxMB:   512,
		ReloadPeriod: 10 * time.Second,
		AgentType:    "opsmini",
	}
}

// LoadMain parses the main pipeline config (a filebeat-shaped YAML).
//
// Only the keys we act on are read; anything else is ignored on purpose, so a
// filebeat.yml copied from an existing host still loads.
func LoadMain(path string, def MainConfig) (MainConfig, error) {
	cfg := def
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}

	if v, ok := doc["filebeat.config.inputs"].(map[string]any); ok {
		if p, _ := v["path"].(string); p != "" {
			// filebeat uses a glob; we take the directory part.
			cfg.TasksDir = filepath.Dir(p)
		}
		if rp, ok := v["reload.period"].(string); ok {
			if d, err := time.ParseDuration(rp); err == nil && d > 0 {
				cfg.ReloadPeriod = d
			}
		}
	}
	if v, ok := doc["queue.disk"].(map[string]any); ok {
		if p, _ := v["path"].(string); p != "" {
			cfg.SpoolDir = p
		}
		if mb := asInt(v["max_size"]); mb > 0 {
			cfg.SpoolMaxMB = mb
		}
	}
	if out, ok := parseOutput(doc); ok {
		cfg.Output = out
	}
	// OpsMini-only section; ignored by filebeat, so the same file stays valid there.
	if v, ok := doc["logpipeline"].(map[string]any); ok {
		if c, ok := v["compatibility"].(map[string]any); ok {
			if t, _ := c["agent_type"].(string); strings.EqualFold(t, "filebeat") {
				cfg.AgentType = "filebeat"
			}
		}
		if s, ok := v["strict"].(bool); ok {
			cfg.Strict = s
		}
	}
	return cfg, nil
}

// LoadTasks reads every task file in dir.
func LoadTasks(dir string, strict bool) ([]Task, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	var out []Task
	var warnings []string
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") && !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		ts, ws, err := ParseTask(data, path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		for _, w := range ws {
			warnings = append(warnings, name+": "+w)
		}
		out = append(out, ts...)
	}
	if strict && len(warnings) > 0 {
		return nil, warnings
	}
	return out, warnings
}

// ParseTask parses one task file. Three shapes are accepted because log
// platforms emit all three: a bare list, `inputs:`, and `filebeat.inputs:`.
func ParseTask(data []byte, path string) ([]Task, []string, error) {
	var warnings []string

	var list []map[string]any
	if err := yaml.Unmarshal(data, &list); err != nil || len(list) == 0 {
		var doc map[string]any
		if err2 := yaml.Unmarshal(data, &doc); err2 != nil {
			return nil, nil, fmt.Errorf("neither a list nor a mapping")
		}
		if v, ok := doc["filebeat.inputs"].([]any); ok {
			list = toMapList(v)
		} else if v, ok := doc["inputs"].([]any); ok {
			list = toMapList(v)
		}
		// Per-task output is parsed but not honoured (D4/Q4): keeping output in
		// one place is what makes a task portable back to real filebeat.
		if _, ok := parseOutput(doc); ok {
			warnings = append(warnings, "任务文件里的 output 段被忽略（出口只在主配置定义）")
		}
	}
	if len(list) == 0 {
		return nil, warnings, errors.New("no inputs")
	}

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var out []Task
	for i, m := range list {
		in, ws := parseInput(m)
		warnings = append(warnings, ws...)
		if in.Type == "" || len(in.Paths) == 0 {
			continue
		}
		if in.ID == "" {
			// filebeat 8 requires an explicit id; derive a stable one so that a
			// config edit does not reset the read offsets.
			in.ID = fmt.Sprintf("%s-%d", base, i)
			warnings = append(warnings, fmt.Sprintf("输入缺少 id，已派生成 %s（位点按 id 保存，建议显式声明）", in.ID))
		}
		out = append(out, Task{ID: in.ID, File: path, Input: in, LoadedAt: time.Now()})
	}
	return out, warnings, nil
}

func parseInput(m map[string]any) (Input, []string) {
	var warnings []string
	in := Input{
		Type:          asString(m["type"]),
		ID:            asString(m["id"]),
		Paths:         asStringSlice(m["paths"]),
		ExcludeFiles:  asStringSlice(m["exclude_files"]),
		Encoding:      asString(m["encoding"]),
		Fields:        asMap(m["fields"]),
		IgnoreOlder:   asDuration(m["ignore_older"]),
		CloseInactive: asDuration(m["close_inactive"]),
		ScanFrequency: asDuration(m["scan_frequency"]),
	}
	if in.Encoding == "" {
		in.Encoding = "utf-8"
	}
	if in.Type != "" && in.Type != "filestream" && in.Type != "log" {
		return in, []string{fmt.Sprintf("不支持的 input 类型 %q（仅 filestream/log）", in.Type)}
	}
	if ml, ok := m["multiline"].(map[string]any); ok {
		in.Multiline = &Multiline{
			Pattern:  asString(ml["pattern"]),
			Negate:   asBool(ml["negate"]),
			Match:    strings.ToLower(asString(ml["match"])),
			MaxLines: asInt(ml["max_lines"]),
			Timeout:  asDuration(ml["timeout"]),
		}
		if in.Multiline.Pattern == "" {
			in.Multiline = nil
		}
		if in.Multiline != nil && in.Multiline.Match == "" {
			in.Multiline.Match = "after"
		}
	}
	if raw, ok := m["processors"].([]any); ok {
		for _, item := range raw {
			pm, ok := item.(map[string]any)
			if !ok || len(pm) != 1 {
				warnings = append(warnings, "processors 条目格式不正确，已跳过")
				continue
			}
			for name, params := range pm {
				in.Processors = append(in.Processors, ProcSpec{Type: name, Params: asMap(params)})
			}
		}
	}
	// Fields declared by filebeat's own known keys that we ignore would silently
	// change behaviour, so name them instead of pretending support.
	for _, k := range []string{"harvester_buffer_size", "max_bytes", "json", "parsers", "paths_container"} {
		if _, ok := m[k]; ok {
			warnings = append(warnings, fmt.Sprintf("字段 %s 未实现（已忽略）", k))
		}
	}
	return in, warnings
}

func parseOutput(doc map[string]any) (OutputSpec, bool) {
	for _, t := range []string{"elasticsearch", "kafka", "logstash"} {
		raw, ok := doc["output."+t].(map[string]any)
		if !ok {
			continue
		}
		o := OutputSpec{
			Type:             t,
			Hosts:            asStringSlice(raw["hosts"]),
			Index:            asString(raw["index"]),
			Username:         asString(raw["username"]),
			Password:         asString(raw["password"]),
			APIKey:           asString(raw["api_key"]),
			Pipeline:         asString(raw["pipeline"]),
			BulkMaxSize:      asInt(raw["bulk_max_size"]),
			FlushInterval:    asDuration(raw["flush_interval"]),
			Topic:            asString(raw["topic"]),
			Partition:        asString(raw["partition"]),
			Compression:      asString(raw["compression"]),
			RequiredAck:      asInt(raw["required_acks"]),
			LoadBalance:      asBool(raw["loadbalance"]),
			CompressionLevel: asInt(raw["compression_level"]),
			TTL:              asDuration(raw["ttl"]),
			CAFile:           asString(raw["ssl.certificate_authorities"]),
			SASLUser:         asString(raw["sasl.username"]),
			SASLPass:         asString(raw["sasl.password"]),
		}
		if blob, ok := raw["partition.hash"].(map[string]any); ok {
			if len(asStringSlice(blob["hash"])) > 0 {
				o.Partition = "hash"
			}
		}
		if blob, ok := raw["sasl"].(map[string]any); ok {
			o.SASLUser = asString(blob["username"])
			o.SASLPass = asString(blob["password"])
		}
		if blob, ok := raw["ssl"].(map[string]any); ok {
			o.Insecure = asString(blob["verification_mode"]) == "none"
			o.CAFile = asString(blob["certificate_authorities"])
			o.CertFile = asString(blob["certificate"])
			o.KeyFile = asString(blob["key"])
		}
		if o.BulkMaxSize <= 0 {
			o.BulkMaxSize = 2000
		}
		if o.FlushInterval <= 0 {
			o.FlushInterval = 5 * time.Second
		}
		if o.Topic == "" {
			o.Topic = "opsmini-logs"
		}
		if o.CompressionLevel <= 0 {
			o.CompressionLevel = 3
		}
		return o, true
	}
	return OutputSpec{}, false
}

// --- small typed helpers ------------------------------------------------

func toMapList(in []any) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, v := range in {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case fmt.Stringer:
		return t.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	case int:
		return t != 0
	}
	return false
}

func asInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		var n int
		fmt.Sscanf(strings.TrimSuffix(t, "MB"), "%d", &n)
		return n
	}
	return 0
}

func asDuration(v any) time.Duration {
	switch t := v.(type) {
	case string:
		d, err := time.ParseDuration(t)
		if err != nil {
			return 0
		}
		return d
	case int:
		return time.Duration(t) * time.Second
	case float64:
		return time.Duration(t) * time.Second
	}
	return 0
}

func asStringSlice(v any) []string {
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s := asString(x); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	}
	return nil
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}
