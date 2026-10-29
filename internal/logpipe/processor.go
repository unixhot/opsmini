package logpipe

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Processor transforms one event into zero or more events (drop = zero,
// extract_array = many). Keeping the signature this narrow is what lets the
// pipeline stay a straight chain of function calls.
type Processor interface {
	Name() string
	Apply(ev Event) []Event
}

// buildProcessors instantiates the chain. Unknown names are reported instead of
// failing the task: a task that uses one unsupported processor should still
// collect logs (unless strict mode is on, which the caller decides).
func buildProcessors(specs []ProcSpec) ([]Processor, []string) {
	var out []Processor
	var warnings []string
	for _, s := range specs {
		p, err := newProcessor(s)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("processor %s: %v", s.Type, err))
			continue
		}
		out = append(out, p)
	}
	return out, warnings
}

func newProcessor(s ProcSpec) (Processor, error) {
	switch s.Type {
	case "add_fields":
		return &addFieldsProc{
			fields: asMap(s.Params["fields"]),
			target: asString(s.Params["target"]),
		}, nil
	case "add_tags":
		return &addTagsProc{tags: asStringSlice(s.Params["tags"])}, nil
	case "drop_fields":
		return &dropFieldsProc{fields: asStringSlice(s.Params["fields"])}, nil
	case "include_fields":
		return &includeFieldsProc{fields: asStringSlice(s.Params["fields"])}, nil
	case "rename":
		return &renameProc{fields: parseFromTo(s.Params["fields"]), failMissing: asBool(s.Params["fail_on_error"])}, nil
	case "copy_fields":
		return &copyProc{fields: parseFromTo(s.Params["fields"])}, nil
	case "convert":
		return &convertProc{fields: parseConvert(s.Params["fields"]), missing: asBool(s.Params["ignore_missing"])}, nil
	case "decode_json_fields":
		return &decodeJSONProc{
			fields:     asStringSlice(s.Params["fields"]),
			target:     asString(s.Params["target"]),
			overwrite:  asBool(s.Params["overwrite_keys"]),
			addErrKey:  asBool(s.Params["add_error_key"]),
			expandDots: asBool(s.Params["expand_dot"]),
		}, nil
	case "drop_event":
		cond, err := parseWhen(s.Params["when"])
		if err != nil {
			return nil, err
		}
		return &dropEventProc{when: cond}, nil
	case "replace":
		return newReplaceProc(s.Params)
	case "add_id":
		return &addIDProc{target: orDefault(asString(s.Params["target"]), "@metadata._id")}, nil
	case "timestamp":
		return newTimestampProc(s.Params)
	// The rest of filebeat's catalogue lives in processors_extra.go.
	case "dissect":
		return newDissectProc(s.Params)
	case "script":
		return newScriptProc(s.Params)
	case "decode_csv_fields":
		return newDecodeCSVProc(s.Params)
	case "decode_duration":
		return &decodeDurationProc{fields: asStringSlice(s.Params["fields"]), target: asString(s.Params["target_field"])}, nil
	case "extract_array":
		return newExtractArrayProc(s.Params)
	case "decode_base64_field":
		return newDecodeB64Proc(s.Params)
	case "decompress_gzip_field":
		return newGunzipProc(s.Params)
	case "urldecode_fields":
		return newURLDecodeProc(s.Params)
	case "truncate_fields":
		return newTruncateProc(s.Params)
	case "move_fields":
		return newMoveProc(s.Params)
	case "add_locale":
		return newAddLocaleProc(s.Params)
	case "add_observer_metadata":
		return newAddObserverMetaProc(s.Params)
	case "add_host_metadata":
		return newAddHostMetaProc(s.Params)
	case "add_network_direction":
		return newAddNetDirectionProc(s.Params)
	case "fingerprint":
		return newFingerprintProc(s.Params)
	case "community_id":
		return newCommunityIDProc(s.Params)
	case "registered_domain":
		return newRegDomainProc(s.Params)
	case "syslog":
		return newSyslogProc(s.Params)
	case "decode_xml":
		return newDecodeXMLProc(s.Params)
	}
	return nil, fmt.Errorf("未实现（已跳过）")
}

// --- add_fields / add_tags ------------------------------------------------

type addFieldsProc struct {
	fields map[string]any
	target string
}

func (p *addFieldsProc) Name() string { return "add_fields" }
func (p *addFieldsProc) Apply(ev Event) []Event {
	for k, v := range p.fields {
		if p.target == "" {
			setPath(ev, k, v)
		} else {
			setPath(ev, p.target+"."+k, v)
		}
	}
	return []Event{ev}
}

type addTagsProc struct{ tags []string }

func (p *addTagsProc) Name() string { return "add_tags" }
func (p *addTagsProc) Apply(ev Event) []Event {
	if len(p.tags) == 0 {
		return []Event{ev}
	}
	cur, _ := getPath(ev, "tags")
	var list []any
	if a, ok := cur.([]any); ok {
		list = a
	}
	for _, t := range p.tags {
		list = append(list, t)
	}
	setPath(ev, "tags", list)
	return []Event{ev}
}

// --- drop / include -------------------------------------------------------

type dropFieldsProc struct{ fields []string }

func (p *dropFieldsProc) Name() string { return "drop_fields" }
func (p *dropFieldsProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		delPath(ev, f)
	}
	return []Event{ev}
}

type includeFieldsProc struct{ fields []string }

func (p *includeFieldsProc) Name() string { return "include_fields" }
func (p *includeFieldsProc) Apply(ev Event) []Event {
	keep := map[string]bool{}
	for _, f := range p.fields {
		keep[f] = true
	}
	for k := range ev {
		// Pipeline metadata is always kept: dropping it breaks indexing.
		if k == "@timestamp" || k == "@metadata" {
			continue
		}
		if !keep[k] {
			delete(ev, k)
		}
	}
	return []Event{ev}
}

// --- rename / copy --------------------------------------------------------

type fromTo struct {
	from string
	to   string
}

type renameProc struct {
	fields      []fromTo
	failMissing bool
}

func (p *renameProc) Name() string { return "rename" }
func (p *renameProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		v, ok := getPath(ev, f.from)
		if !ok {
			continue
		}
		delPath(ev, f.from)
		setPath(ev, f.to, v)
	}
	return []Event{ev}
}

type copyProc struct{ fields []fromTo }

func (p *copyProc) Name() string { return "copy_fields" }
func (p *copyProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		if v, ok := getPath(ev, f.from); ok {
			setPath(ev, f.to, v)
		}
	}
	return []Event{ev}
}

// --- convert --------------------------------------------------------------

type convertField struct {
	field string
	typ   string
}

type convertProc struct {
	fields  []convertField
	missing bool
}

func (p *convertProc) Name() string { return "convert" }
func (p *convertProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		v, ok := getPath(ev, f.field)
		if !ok {
			continue
		}
		conv, ok := convertValue(v, f.typ)
		if !ok {
			continue
		}
		setPath(ev, f.field, conv)
	}
	return []Event{ev}
}

func convertValue(v any, typ string) (any, bool) {
	s := asString(v)
	switch typ {
	case "integer", "long", "int":
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			if fv, ok := v.(float64); ok {
				return int64(fv), true
			}
			return nil, false
		}
		return n, true
	case "float", "double":
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return nil, false
		}
		return f, true
	case "string":
		return s, true
	case "boolean", "bool":
		b, err := strconv.ParseBool(strings.TrimSpace(s))
		if err != nil {
			switch strings.ToLower(s) {
			case "yes", "on":
				return true, true
			case "no", "off":
				return false, true
			}
			return nil, false
		}
		return b, true
	case "ip":
		return s, true
	}
	return nil, false
}

// --- decode_json_fields ---------------------------------------------------

type decodeJSONProc struct {
	fields     []string
	target     string
	overwrite  bool
	addErrKey  bool
	expandDots bool
}

func (p *decodeJSONProc) Name() string { return "decode_json_fields" }
func (p *decodeJSONProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		raw, ok := getPath(ev, f)
		if !ok {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(asString(raw)), &parsed); err != nil {
			if p.addErrKey {
				setPath(ev, "error.message", err.Error())
			}
			continue
		}
		for k, v := range parsed {
			if p.target == "" {
				if _, exists := ev[k]; exists && !p.overwrite {
					continue
				}
				setPath(ev, k, v)
				continue
			}
			setPath(ev, p.target+"."+k, v)
		}
	}
	return []Event{ev}
}

// --- drop_event -----------------------------------------------------------

type dropEventProc struct{ when *cond }

func (p *dropEventProc) Name() string { return "drop_event" }
func (p *dropEventProc) Apply(ev Event) []Event {
	if p.when == nil || p.when.match(ev) {
		return nil
	}
	return []Event{ev}
}

// --- replace --------------------------------------------------------------

type replaceField struct {
	field string
	re    *regexp.Regexp
	with  string
}

type replaceProc struct{ fields []replaceField }

func newReplaceProc(params map[string]any) (Processor, error) {
	items, _ := params["fields"].([]any)
	var out []replaceField
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		re, err := regexp.Compile(asString(m["pattern"]))
		if err != nil {
			return nil, err
		}
		out = append(out, replaceField{
			field: asString(m["field"]),
			re:    re,
			with:  asString(m["replacement"]),
		})
	}
	return &replaceProc{fields: out}, nil
}

func (p *replaceProc) Name() string { return "replace" }
func (p *replaceProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		v, ok := getPath(ev, f.field)
		if !ok {
			continue
		}
		setPath(ev, f.field, f.re.ReplaceAllString(asString(v), f.with))
	}
	return []Event{ev}
}

// --- add_id ---------------------------------------------------------------

type addIDProc struct{ target string }

func (p *addIDProc) Name() string { return "add_id" }
func (p *addIDProc) Apply(ev Event) []Event {
	// A stable id makes ES indexing idempotent: replaying a batch after a crash
	// overwrites instead of duplicating.
	h := sha256.Sum256([]byte(fmt.Sprintf("%v|%v|%v",
		ev["@timestamp"], hostMeta.name, ev["message"])))
	setPath(ev, p.target, hex.EncodeToString(h[:20]))
	return []Event{ev}
}

// --- timestamp ------------------------------------------------------------

type timestampProc struct {
	field   string
	layouts []string
	target  string
}

func newTimestampProc(params map[string]any) (Processor, error) {
	layouts := asStringSlice(params["layouts"])
	if len(layouts) == 0 {
		layouts = []string{
			time.RFC3339Nano, time.RFC3339,
			"2006-01-02 15:04:05", "2006-01-02T15:04:05",
			"02/Jan/2006:15:04:05 -0700", // nginx/apache common log
			time.RFC1123Z, time.RFC1123, time.ANSIC,
		}
	}
	return &timestampProc{
		field:   asString(params["field"]),
		layouts: layouts,
		target:  orDefault(asString(params["target_field"]), "@timestamp"),
	}, nil
}

func (p *timestampProc) Name() string { return "timestamp" }
func (p *timestampProc) Apply(ev Event) []Event {
	v, ok := getPath(ev, p.field)
	if !ok {
		return []Event{ev}
	}
	s := asString(v)
	for _, layout := range p.layouts {
		if t, err := time.Parse(layout, s); err == nil {
			setPath(ev, p.target, t.UTC().Format(time.RFC3339Nano))
			return []Event{ev}
		}
	}
	return []Event{ev}
}

// --- when 条件 -------------------------------------------------------------

// cond is a small subset of filebeat's condition language: enough for
// drop_event and practical filtering without a parser generator.
type cond struct {
	kind  string // equals | contains | regexp | range | and | or | not | exists
	field string
	value string
	re    *regexp.Regexp
	subs  []*cond
	gte   *float64
	lte   *float64
}

func parseWhen(v any) (*cond, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, nil
	}
	for k, raw := range m {
		switch k {
		case "equals", "contains":
			body, _ := raw.(map[string]any)
			for f, val := range body {
				return &cond{kind: k, field: f, value: asString(val)}, nil
			}
		case "regexp":
			body, _ := raw.(map[string]any)
			for f, val := range body {
				re, err := regexp.Compile(asString(val))
				if err != nil {
					return nil, err
				}
				return &cond{kind: k, field: f, re: re}, nil
			}
		case "range":
			body, _ := raw.(map[string]any)
			for f, val := range body {
				c := &cond{kind: "range", field: f}
				lim, _ := val.(map[string]any)
				if g, ok := toFloat(lim["gte"]); ok {
					c.gte = &g
				}
				if l, ok := toFloat(lim["lte"]); ok {
					c.lte = &l
				}
				return c, nil
			}
		case "has_fields":
			return &cond{kind: "exists", value: asString(raw)}, nil
		case "and", "or":
			list, _ := raw.([]any)
			c := &cond{kind: k}
			for _, item := range list {
				sub, err := parseWhen(item)
				if err != nil {
					return nil, err
				}
				if sub != nil {
					c.subs = append(c.subs, sub)
				}
			}
			return c, nil
		case "not":
			sub, err := parseWhen(raw)
			if err != nil {
				return nil, err
			}
			return &cond{kind: "not", subs: []*cond{sub}}, nil
		}
	}
	return nil, nil
}

func (c *cond) match(ev Event) bool {
	if c == nil {
		return true
	}
	switch c.kind {
	case "and":
		for _, s := range c.subs {
			if !s.match(ev) {
				return false
			}
		}
		return true
	case "or":
		for _, s := range c.subs {
			if s.match(ev) {
				return true
			}
		}
		return false
	case "not":
		return len(c.subs) == 1 && !c.subs[0].match(ev)
	case "exists":
		for _, f := range strings.Split(c.value, ",") {
			if _, ok := getPath(ev, strings.TrimSpace(f)); !ok {
				return false
			}
		}
		return true
	}
	v, ok := getPath(ev, c.field)
	if !ok {
		return false
	}
	s := asString(v)
	switch c.kind {
	case "equals":
		return s == c.value
	case "contains":
		return strings.Contains(s, c.value)
	case "regexp":
		return c.re != nil && c.re.MatchString(s)
	case "range":
		f, ok := toFloat(v)
		if !ok {
			return false
		}
		if c.gte != nil && f < *c.gte {
			return false
		}
		if c.lte != nil && f > *c.lte {
			return false
		}
		return true
	}
	return false
}

// --- helpers --------------------------------------------------------------

func parseFromTo(v any) []fromTo {
	list, _ := v.([]any)
	out := make([]fromTo, 0, len(list))
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, fromTo{from: asString(m["from"]), to: asString(m["to"])})
	}
	return out
}

func parseConvert(v any) []convertField {
	list, _ := v.([]any)
	out := make([]convertField, 0, len(list))
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, convertField{field: asString(m["field"]), typ: asString(m["type"])})
	}
	return out
}

func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
