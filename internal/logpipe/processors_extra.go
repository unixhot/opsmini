package logpipe

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// The processors in this file cover the rest of filebeat's catalogue (the ones
// that do not need an external service). Each one is a small pure function so
// that a misbehaving config can only lose fields, never panic the pipeline.

// --- dissect --------------------------------------------------------------

// dissect parses unstructured text with a tokenizer like
//   %{clientip} - - [%{ts}] "%{method} %{path}"
// Supported modifiers: %{field}, %{field->} (skip trailing), %{?skip},
// %{+append}, %{&key} (value becomes a key for the following field).
type dissectProc struct {
	tokenizer string
	field     string
	prefix    string
	segs      []dissectSeg
}

type dissectSeg struct {
	literal string
	field   string
	skip    bool
	append  bool
	keyOf   bool
	padRight bool
}

func newDissectProc(params map[string]any) (Processor, error) {
	tok := asString(params["tokenizer"])
	if tok == "" {
		return nil, fmt.Errorf("dissect 缺少 tokenizer")
	}
	segs, err := parseDissect(tok)
	if err != nil {
		return nil, err
	}
	return &dissectProc{
		tokenizer: tok,
		field:     orDefault(asString(params["field"]), "message"),
		prefix:    asString(params["target_prefix"]),
		segs:      segs,
	}, nil
}

func parseDissect(tok string) ([]dissectSeg, error) {
	var segs []dissectSeg
	i := 0
	for i < len(tok) {
		if strings.HasPrefix(tok[i:], "%{") {
			end := strings.Index(tok[i:], "}")
			if end < 0 {
				return nil, fmt.Errorf("dissect tokenizer 括号不匹配")
			}
			inner := tok[i+2 : i+end]
			seg := dissectSeg{}
			if strings.HasPrefix(inner, "+") {
				seg.append = true
				inner = inner[1:]
			}
			if strings.HasPrefix(inner, "&") {
				seg.keyOf = true
				inner = inner[1:]
			}
			if strings.HasPrefix(inner, "?") {
				seg.skip = true
				inner = inner[1:]
			}
			if strings.HasSuffix(inner, "->") {
				seg.padRight = true
				inner = strings.TrimSuffix(inner, "->")
			}
			seg.field = inner
			segs = append(segs, seg)
			i += end + 1
			continue
		}
		j := strings.Index(tok[i:], "%{")
		if j < 0 {
			segs = append(segs, dissectSeg{literal: tok[i:]})
			break
		}
		segs = append(segs, dissectSeg{literal: tok[i : i+j]})
		i += j
	}
	return segs, nil
}

func (p *dissectProc) Name() string { return "dissect" }
func (p *dissectProc) Apply(ev Event) []Event {
	raw, ok := getPath(ev, p.field)
	if !ok {
		return []Event{ev}
	}
	text := asString(raw)
	values := map[string]string{}
	pendingKey := ""
	pos := 0

	for i, seg := range p.segs {
		if seg.literal != "" {
			if !strings.HasPrefix(text[pos:], seg.literal) {
				return []Event{ev} // 不匹配就不改事件
			}
			pos += len(seg.literal)
			continue
		}
		// 字段值一直取到下一个字面量为止。
		var nextLit string
		if i+1 < len(p.segs) && p.segs[i+1].literal != "" {
			nextLit = p.segs[i+1].literal
		}
		var val string
		if nextLit == "" {
			val = text[pos:]
			pos = len(text)
		} else {
			idx := strings.Index(text[pos:], nextLit)
			if idx < 0 {
				return []Event{ev}
			}
			val = text[pos : pos+idx]
			pos += idx
		}
		val = strings.TrimSpace(val)
		switch {
		case seg.skip:
			// %{?name}: 匹配但不输出
		case seg.keyOf:
			// %{&key}: 这个值成为下一个字段的键名
			pendingKey = val
		case seg.append && pendingKey != "":
			values[pendingKey] += val
		case pendingKey != "":
			values[pendingKey] = val
			pendingKey = ""
		default:
			values[seg.field] = val
		}
	}
	for k, v := range values {
		target := k
		if p.prefix != "" {
			target = p.prefix + "." + k
		}
		setPath(ev, target, v)
	}
	return []Event{ev}
}

// --- script (goja) --------------------------------------------------------

// scriptProc runs a user JavaScript snippet against the event, matching
// filebeat's `script` processor contract closely enough for existing scripts:
// the event is exposed as `event` with Get/Put/Delete/Append/Tag/Cancel.
type scriptProc struct {
	source string
	lang   string
}

func newScriptProc(params map[string]any) (Processor, error) {
	lang := orDefault(strings.ToLower(asString(params["lang"])), "javascript")
	if lang != "javascript" && lang != "js" {
		return nil, fmt.Errorf("只支持 javascript 脚本（lang=%s）", lang)
	}
	src := asString(params["source"])
	if src == "" {
		if f := asString(params["file"]); f != "" {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			src = string(b)
		}
	}
	if src == "" {
		return nil, fmt.Errorf("script 缺少 source/file")
	}
	return &scriptProc{source: src, lang: lang}, nil
}

func (p *scriptProc) Name() string { return "script" }

func (p *scriptProc) Apply(ev Event) []Event {
	vm := goja.New()
	obj := vm.NewObject()
	for k, v := range ev {
		_ = obj.Set(k, v)
	}
	cancelled := false
	obj.Set("Get", func(call goja.FunctionCall) goja.Value {
		k := call.Argument(0).String()
		v, ok := getPath(ev, k)
		if !ok {
			return goja.Undefined()
		}
		return vm.ToValue(v)
	})
	obj.Set("Put", func(call goja.FunctionCall) goja.Value {
		setPath(ev, call.Argument(0).String(), call.Argument(1).Export())
		_ = obj.Set(call.Argument(0).String(), call.Argument(1).Export())
		return goja.Undefined()
	})
	obj.Set("Delete", func(call goja.FunctionCall) goja.Value {
		delPath(ev, call.Argument(0).String())
		return goja.Undefined()
	})
	obj.Set("Append", func(call goja.FunctionCall) goja.Value {
		k, v := call.Argument(0).String(), call.Argument(1).Export()
		cur, _ := getPath(ev, k)
		list, _ := cur.([]any)
		setPath(ev, k, append(list, v))
		return goja.Undefined()
	})
	obj.Set("Tag", func(call goja.FunctionCall) goja.Value {
		cur, _ := getPath(ev, "tags")
		list, _ := cur.([]any)
		setPath(ev, "tags", append(list, call.Argument(0).Export()))
		return goja.Undefined()
	})
	obj.Set("Cancel", func(call goja.FunctionCall) goja.Value {
		cancelled = true
		return goja.Undefined()
	})
	_ = vm.Set("event", obj)
	if _, err := vm.RunString(p.source); err != nil {
		setPath(ev, "error.message", "script: "+err.Error())
		return []Event{ev}
	}
	if cancelled {
		return nil
	}
	return []Event{ev}
}

// --- decode_csv_fields ----------------------------------------------------

type decodeCSVProc struct {
	fields    []string
	target    string
	separator string
	overwrite bool
}

func newDecodeCSVProc(params map[string]any) (Processor, error) {
	sep := asString(params["separator"])
	if sep == "" {
		sep = ","
	}
	return &decodeCSVProc{
		fields:    asStringSlice(params["fields"]),
		target:    asString(params["target"]),
		separator: sep,
		overwrite: asBool(params["overwrite_keys"]),
	}, nil
}

func (p *decodeCSVProc) Name() string { return "decode_csv_fields" }
func (p *decodeCSVProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		raw, ok := getPath(ev, f)
		if !ok {
			continue
		}
		r := csv.NewReader(strings.NewReader(asString(raw)))
		r.Comma = []rune(p.separator)[0]
		rec, err := r.Read()
		if err != nil {
			continue
		}
		for i, v := range rec {
			key := strconv.Itoa(i)
			if p.target == "" {
				if _, exists := ev[key]; exists && !p.overwrite {
					continue
				}
				ev[key] = v
				continue
			}
			setPath(ev, p.target+"."+key, v)
		}
	}
	return []Event{ev}
}

// --- decode_duration ------------------------------------------------------

type decodeDurationProc struct {
	fields []string
	target string
}

func (p *decodeDurationProc) Name() string { return "decode_duration" }
func (p *decodeDurationProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		raw, ok := getPath(ev, f)
		if !ok {
			continue
		}
		d, err := time.ParseDuration(asString(raw))
		if err != nil {
			continue
		}
		key := p.target
		if key == "" {
			key = f + ".ns"
		}
		setPath(ev, key, d.Nanoseconds())
	}
	return []Event{ev}
}

// --- extract_array --------------------------------------------------------

type extractArrayProc struct {
	field   string
	mappings []arrayMap
}

type arrayMap struct{ from, to string }

func newExtractArrayProc(params map[string]any) (Processor, error) {
	list, _ := params["mappings"].([]any)
	var ms []arrayMap
	for _, it := range list {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ms = append(ms, arrayMap{from: asString(m["from"]), to: asString(m["to"])})
	}
	return &extractArrayProc{field: asString(params["field"]), mappings: ms}, nil
}

func (p *extractArrayProc) Name() string { return "extract_array" }
func (p *extractArrayProc) Apply(ev Event) []Event {
	raw, ok := getPath(ev, p.field)
	if !ok {
		return []Event{ev}
	}
	list, ok := raw.([]any)
	if !ok {
		return []Event{ev}
	}
	for _, m := range p.mappings {
		for _, item := range list {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if v, ok := obj[m.from]; ok && m.to != "" && !strings.Contains(m.to, "[") {
				setPath(ev, strings.ReplaceAll(m.to, "[]", ""), v)
			}
		}
	}
	return []Event{ev}
}

// --- decode_base64_field / decompress_gzip_field / urldecode_fields -------

type decodeB64Proc struct{ fields []decodeField }

type decodeField struct{ field, to string }

func newDecodeB64Proc(params map[string]any) (Processor, error) {
	return &decodeB64Proc{fields: parseDecodeFields(params["fields"])}, nil
}

func (p *decodeB64Proc) Name() string { return "decode_base64_field" }
func (p *decodeB64Proc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		raw, ok := getPath(ev, f.field)
		if !ok {
			continue
		}
		out, err := base64.StdEncoding.DecodeString(asString(raw))
		if err != nil {
			continue
		}
		setPath(ev, orDefault(f.to, f.field), string(out))
	}
	return []Event{ev}
}

type gunzipProc struct{ fields []decodeField }

func newGunzipProc(params map[string]any) (Processor, error) {
	return &gunzipProc{fields: parseDecodeFields(params["fields"])}, nil
}

func (p *gunzipProc) Name() string { return "decompress_gzip_field" }
func (p *gunzipProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		raw, ok := getPath(ev, f.field)
		if !ok {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(asString(raw))
		if err != nil {
			continue
		}
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			continue
		}
		out, err := io.ReadAll(io.LimitReader(zr, 4<<20))
		_ = zr.Close()
		if err != nil {
			continue
		}
		setPath(ev, orDefault(f.to, f.field), string(out))
	}
	return []Event{ev}
}

type urldecodeProc struct{ fields []decodeField }

func newURLDecodeProc(params map[string]any) (Processor, error) {
	return &urldecodeProc{fields: parseDecodeFields(params["fields"])}, nil
}

func (p *urldecodeProc) Name() string { return "urldecode_fields" }
func (p *urldecodeProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		raw, ok := getPath(ev, f.field)
		if !ok {
			continue
		}
		if out, err := url.QueryUnescape(asString(raw)); err == nil {
			setPath(ev, orDefault(f.to, f.field), out)
		}
	}
	return []Event{ev}
}

func parseDecodeFields(v any) []decodeField {
	list, _ := v.([]any)
	out := make([]decodeField, 0, len(list))
	for _, it := range list {
		switch m := it.(type) {
		case map[string]any:
			out = append(out, decodeField{field: asString(m["field"]), to: asString(m["to"])})
		case string:
			out = append(out, decodeField{field: m})
		}
	}
	return out
}

// --- truncate_fields / move_fields ---------------------------------------

type truncateProc struct {
	fields  []string
	maxLen  int
	failMissing bool
}

func newTruncateProc(params map[string]any) (Processor, error) {
	n := asInt(params["max_characters"])
	if n <= 0 {
		return nil, fmt.Errorf("truncate_fields 需要 max_characters")
	}
	return &truncateProc{fields: asStringSlice(params["fields"]), maxLen: n}, nil
}

func (p *truncateProc) Name() string { return "truncate_fields" }
func (p *truncateProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		raw, ok := getPath(ev, f)
		if !ok {
			continue
		}
		s := asString(raw)
		r := []rune(s)
		if len(r) > p.maxLen {
			s = string(r[:p.maxLen])
		}
		setPath(ev, f, s)
	}
	return []Event{ev}
}

type moveProc struct{ fields []fromTo }

func newMoveProc(params map[string]any) (Processor, error) {
	return &moveProc{fields: parseFromTo(params["fields"])}, nil
}

func (p *moveProc) Name() string { return "move_fields" }
func (p *moveProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		if v, ok := getPath(ev, f.from); ok {
			delPath(ev, f.from)
			setPath(ev, f.to, v)
		}
	}
	return []Event{ev}
}

// --- add_locale / add_host_metadata / add_network_direction --------------

type addLocaleProc struct{ target string }

func newAddLocaleProc(params map[string]any) (Processor, error) {
	return &addLocaleProc{target: orDefault(asString(params["target"]), "event.timezone")}, nil
}

func (p *addLocaleProc) Name() string { return "add_locale" }
func (p *addLocaleProc) Apply(ev Event) []Event {
	tz := os.Getenv("TZ")
	if tz == "" {
		if b, err := os.ReadFile("/etc/timezone"); err == nil {
			tz = strings.TrimSpace(string(b))
		}
	}
	if tz == "" {
		tz = "UTC"
	}
	setPath(ev, p.target, tz)
	return []Event{ev}
}

type addHostMetaProc struct{ replace bool }

func newAddHostMetaProc(params map[string]any) (Processor, error) {
	return &addHostMetaProc{replace: asBool(params["replace"])}, nil
}

func (p *addHostMetaProc) Name() string { return "add_host_metadata" }
func (p *addHostMetaProc) Apply(ev Event) []Event {
	if _, ok := ev["host"]; ok && !p.replace {
		return []Event{ev}
	}
	ev["host"] = Event{
		"name": hostMeta.name,
		"os":   Event{"type": hostMeta.os},
		"arch": hostMeta.arch,
		"ip":   hostMeta.ip,
	}
	return []Event{ev}
}

type addNetDirectionProc struct{ internal []*net.IPNet }

func newAddNetDirectionProc(params map[string]any) (Processor, error) {
	var nets []*net.IPNet
	for _, cidr := range asStringSlice(params["internal_networks"]) {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil {
			nets = append(nets, n)
		}
	}
	if len(nets) == 0 {
		for _, cidr := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "fc00::/7", "::1/128"} {
			if _, n, err := net.ParseCIDR(cidr); err == nil {
				nets = append(nets, n)
			}
		}
	}
	return &addNetDirectionProc{internal: nets}, nil
}

func (p *addNetDirectionProc) Name() string { return "add_network_direction" }
func (p *addNetDirectionProc) Apply(ev Event) []Event {
	src, _ := getPath(ev, "source.ip")
	dst, _ := getPath(ev, "destination.ip")
	s := net.ParseIP(asString(src))
	d := net.ParseIP(asString(dst))
	if s == nil || d == nil {
		return []Event{ev}
	}
	srcIn, dstIn := p.private(s), p.private(d)
	switch {
	case srcIn && !dstIn:
		ev["network"] = Event{"direction": "outbound"}
	case !srcIn && dstIn:
		ev["network"] = Event{"direction": "inbound"}
	case !srcIn && !dstIn:
		ev["network"] = Event{"direction": "external"}
	default:
		ev["network"] = Event{"direction": "internal"}
	}
	return []Event{ev}
}

func (p *addNetDirectionProc) private(ip net.IP) bool {
	for _, n := range p.internal {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// --- fingerprint ----------------------------------------------------------

type fingerprintProc struct {
	fields []string
	target string
	method string
}

func newFingerprintProc(params map[string]any) (Processor, error) {
	return &fingerprintProc{
		fields: asStringSlice(params["fields"]),
		target: orDefault(asString(params["target"]), "@metadata._id"),
		method: orDefault(strings.ToLower(asString(params["method"])), "sha256"),
	}, nil
}

func (p *fingerprintProc) Name() string { return "fingerprint" }
func (p *fingerprintProc) Apply(ev Event) []Event {
	var sb strings.Builder
	for _, f := range p.fields {
		if v, ok := getPath(ev, f); ok {
			sb.WriteString(asString(v))
			sb.WriteByte('|')
		}
	}
	if sb.Len() == 0 {
		return []Event{ev}
	}
	var sum string
	if p.method == "md5" {
		h := sha1.Sum([]byte(sb.String()))
		sum = hex.EncodeToString(h[:])
	} else if p.method == "sha1" {
		h := sha1.Sum([]byte(sb.String()))
		sum = hex.EncodeToString(h[:])
	} else {
		h := sha256.Sum256([]byte(sb.String()))
		sum = hex.EncodeToString(h[:])
	}
	setPath(ev, p.target, sum[:40])
	return []Event{ev}
}

// --- community_id ---------------------------------------------------------

// communityIdProc computes the Community ID v1 flow hash, which is what makes
// network events joinable with other tools that speak the same standard.
type communityIdProc struct {
	srcField, dstField, srcPortField, dstPortField, protoField, target string
}

func newCommunityIDProc(params map[string]any) (Processor, error) {
	return &communityIdProc{
		srcField:     orDefault(asString(params["source_ip"]), "source.ip"),
		dstField:     orDefault(asString(params["destination_ip"]), "destination.ip"),
		srcPortField: orDefault(asString(params["source_port"]), "source.port"),
		dstPortField: orDefault(asString(params["destination_port"]), "destination.port"),
		protoField:   orDefault(asString(params["iana_number"]), "network.iana_number"),
		target:       orDefault(asString(params["target"]), "network.community_id"),
	}, nil
}

func (p *communityIdProc) Name() string { return "community_id" }
func (p *communityIdProc) Apply(ev Event) []Event {
	ip1 := asString(mustGet(ev, p.srcField))
	ip2 := asString(mustGet(ev, p.dstField))
	if ip1 == "" || ip2 == "" {
		return []Event{ev}
	}
	port1 := uint64(asInt(mustGet(ev, p.srcPortField)))
	port2 := uint64(asInt(mustGet(ev, p.dstPortField)))
	proto := uint64(asInt(mustGet(ev, p.protoField)))
	var a, b net.IP
	if proto == 6 || proto == 17 {
		a, b = net.ParseIP(ip1), net.ParseIP(ip2)
		if a == nil || b == nil {
			return []Event{ev}
		}
		// Canonical order: lower address+port first, so both directions hash the
		// same way.
		if bytes.Compare(a, b) > 0 || (a.Equal(b) && port1 > port2) {
			a, b = b, a
			port1, port2 = port2, port1
		}
	} else {
		a, b = net.ParseIP(ip1), net.ParseIP(ip2)
		if a == nil || b == nil {
			return []Event{ev}
		}
		if bytes.Compare(a, b) > 0 {
			a, b = b, a
		}
		port1, port2 = 0, 0
	}
	buf := make([]byte, 0, 40)
	if a.To4() != nil {
		buf = append(buf, a.To4()...)
		buf = append(buf, b.To4()...)
	} else {
		buf = append(buf, a.To16()...)
		buf = append(buf, b.To16()...)
	}
	buf = append(buf, byte(proto))
	buf = append(buf, byte(port1>>8), byte(port1), byte(port2>>8), byte(port2))
	h := sha1.Sum(buf)
	setPath(ev, p.target, "1:"+hex.EncodeToString(h[:])[:40])
	return []Event{ev}
}

// --- registered_domain ----------------------------------------------------

// registeredDomainProc reduces a hostname to its registrable domain.
// A full public-suffix list is ~200KB; the common suffixes below cover the
// hostnames that appear in logs, and anything else falls back to "last two
// labels" which is right for the vast majority of cases.
var publicSuffix2 = map[string]bool{
	"co.uk": true, "org.uk": true, "ac.uk": true, "gov.uk": true, "com.cn": true,
	"net.cn": true, "org.cn": true, "gov.cn": true, "com.hk": true, "com.tw": true,
	"co.jp": true, "or.jp": true, "ne.jp": true, "co.kr": true, "or.kr": true,
	"com.au": true, "net.au": true, "org.au": true, "co.nz": true, "com.br": true,
	"com.sg": true, "com.my": true, "co.in": true, "co.za": true, "com.ru": true,
}

type regDomainProc struct {
	field, target string
}

func newRegDomainProc(params map[string]any) (Processor, error) {
	return &regDomainProc{
		field:  asString(params["field"]),
		target: orDefault(asString(params["target"]), "url.domain"),
	}, nil
}

func (p *regDomainProc) Name() string { return "registered_domain" }
func (p *regDomainProc) Apply(ev Event) []Event {
	raw, ok := getPath(ev, p.field)
	if !ok {
		return []Event{ev}
	}
	host := strings.TrimSuffix(strings.ToLower(asString(raw)), ".")
	// 字段里常常是整个 URL（带 scheme 和 path），先取出主机名。
	if strings.Contains(host, "://") {
		if u, err := url.Parse(host); err == nil && u.Hostname() != "" {
			host = u.Hostname()
		}
	} else if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return []Event{ev}
	}
	reg := host
	if len(labels) >= 3 && publicSuffix2[strings.Join(labels[len(labels)-2:], ".")] {
		reg = strings.Join(labels[len(labels)-3:], ".")
	} else {
		reg = strings.Join(labels[len(labels)-2:], ".")
	}
	setPath(ev, p.target, reg)
	setPath(ev, "url.subdomain", strings.TrimSuffix(strings.TrimSuffix(host, reg), "."))
	return []Event{ev}
}

// --- syslog ---------------------------------------------------------------

// syslogProc parses an RFC3164 or RFC5424 line into syslog.* fields.
type syslogProc struct {
	field, format, tz string
}

func newSyslogProc(params map[string]any) (Processor, error) {
	return &syslogProc{
		field:  orDefault(asString(params["field"]), "message"),
		format: orDefault(strings.ToLower(asString(params["format"])), "rfc3164"),
		tz:     asString(params["timezone"]),
	}, nil
}

func (p *syslogProc) Name() string { return "syslog" }

// rfc3164Head: "Oct 29 09:10:01 host program[pid]: message"
var rfc3164Head = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})\s+(\S+)\s+([^:\[]+?)(?:\[(\d+)\])?:\s?(.*)$`)

func (p *syslogProc) Apply(ev Event) []Event {
	raw, ok := getPath(ev, p.field)
	if !ok {
		return []Event{ev}
	}
	line := asString(raw)
	if strings.HasPrefix(line, "<") {
		if end := strings.Index(line, ">"); end > 0 {
			if pri, err := strconv.Atoi(line[1:end]); err == nil {
				setPath(ev, "syslog.priority", pri)
				setPath(ev, "syslog.facility", pri/8)
				setPath(ev, "syslog.severity", pri%8)
			}
			line = strings.TrimSpace(line[end+1:])
		}
	}
	if p.format == "rfc5424" {
		// <PRI>1 TIMESTAMP HOST APP PROCID MSGID STRUCTURED MSG
		f := strings.SplitN(line, " ", 7)
		if len(f) >= 5 {
			setPath(ev, "syslog.version", f[0])
			setPath(ev, "syslog.timestamp", f[1])
			setPath(ev, "syslog.hostname", f[2])
			setPath(ev, "syslog.appname", f[3])
			if len(f) == 7 {
				setPath(ev, "syslog.message", f[6])
			}
		}
		return []Event{ev}
	}
	if m := rfc3164Head.FindStringSubmatch(line); m != nil {
		setPath(ev, "syslog.timestamp", m[1])
		setPath(ev, "syslog.hostname", m[2])
		setPath(ev, "syslog.program", strings.TrimSpace(m[3]))
		if m[4] != "" {
			if pid, err := strconv.Atoi(m[4]); err == nil {
				setPath(ev, "syslog.procid", pid)
			}
		}
		setPath(ev, "syslog.message", m[5])
	}
	return []Event{ev}
}

// --- decode_xml -----------------------------------------------------------

type decodeXMLProc struct {
	fields []string
	target string
}

func newDecodeXMLProc(params map[string]any) (Processor, error) {
	return &decodeXMLProc{
		fields: asStringSlice(params["fields"]),
		target: asString(params["target"]),
	}, nil
}

func (p *decodeXMLProc) Name() string { return "decode_xml" }
func (p *decodeXMLProc) Apply(ev Event) []Event {
	for _, f := range p.fields {
		raw, ok := getPath(ev, f)
		if !ok {
			continue
		}
		var node xmlNode
		if err := xml.Unmarshal([]byte(asString(raw)), &node); err != nil {
			continue
		}
		for k, v := range node.toMap() {
			if p.target == "" {
				setPath(ev, k, v)
			} else {
				setPath(ev, p.target+"."+k, v)
			}
		}
	}
	return []Event{ev}
}

// xmlNode is a generic element tree: filebeat maps element names to fields and
// attributes to <attr>, which this mirrors.
type xmlNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Content  string     `xml:",chardata"`
	Children []xmlNode  `xml:",any"`
}

func (n *xmlNode) toMap() map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(n.Content) != "" {
		out[n.XMLName.Local] = strings.TrimSpace(n.Content)
	}
	for _, a := range n.Attrs {
		if a.Name.Local == "xmlns" || strings.HasPrefix(a.Name.Local, "xmlns") {
			continue
		}
		out["_attr:"+a.Name.Local] = a.Value
	}
	for i := range n.Children {
		child := &n.Children[i]
		name := child.XMLName.Local
		val := child.toMap()[name]
		if val == nil {
			continue
		}
		if existing, ok := out[name]; ok {
			switch t := existing.(type) {
			case []any:
				out[name] = append(t, val)
			default:
				out[name] = []any{existing, val}
			}
		} else {
			out[name] = val
		}
	}
	return out
}

func mustGet(ev Event, path string) any {
	v, _ := getPath(ev, path)
	return v
}
