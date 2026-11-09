package service

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// ReportInput is everything the compliance report needs. Passing it in (rather
// than having the report reach into repositories) keeps the report a pure
// function of the current state: same inputs, same document.
type ReportInput struct {
	Hostname    string
	Version     string
	GeneratedAt time.Time
	Score       int
	Grade       string
	Items       []model.BaselineResult
	// FimChanges and ThreatCount add context: a 92-point score next to two active
	// threats is a different story than 92 points alone.
	FimChanges  int
	ThreatCount int
}

// categoryLabels turns the internal category keys into something a compliance
// reviewer can read. Unknown keys fall through unchanged so a new category is
// visible rather than blank.
var categoryLabels = map[string]string{
	"ssh": "SSH 配置", "password": "密码策略", "account": "账号安全",
	"file_perm": "文件权限", "perm": "文件权限", "firewall": "防火墙",
	"cron": "计划任务", "service": "服务配置", "systemd": "systemd 服务",
	"kernel": "内核加固", "audit": "审计日志", "exposure": "暴露面",
	"container": "容器", "web": "Web 服务",
}

// BuildComplianceReport renders the baseline result as one self-contained HTML
// file.
//
// HTML rather than PDF on purpose: a pure-Go PDF path needs CJK font embedding
// (several MB, re-added to every build) and produces a document that cannot be
// adjusted afterwards. Browsers print HTML to PDF with Ctrl+P, which covers the
// submission requirement without either cost.
func BuildComplianceReport(in ReportInput) string {
	var b strings.Builder
	pass, warn, fail := 0, 0, 0
	weightTotal, weightGot := 0, 0
	byCat := map[string][]model.BaselineResult{}
	for _, it := range in.Items {
		switch it.Status {
		case "pass":
			pass++
			weightGot += it.Weight
		case "warn":
			warn++
		default:
			fail++
		}
		weightTotal += it.Weight
		byCat[it.Category] = append(byCat[it.Category], it)
	}
	total := len(in.Items)
	passRate := 0.0
	if total > 0 {
		passRate = float64(pass) / float64(total) * 100
	}

	catNames := make([]string, 0, len(byCat))
	for k := range byCat {
		catNames = append(catNames, k)
	}
	sort.Strings(catNames)

	b.WriteString(`<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8">
<title>OpsMini 主机合规报告</title>
<style>
:root{--t1:#1a2233;--t2:#59657a;--t3:#8b97ab;--bd:#e5e9f2;--bg:#f7f9fc;
--ok:#16a34a;--warn:#d97706;--err:#dc2626;--pri:#2563eb}
*{box-sizing:border-box}
body{margin:0;padding:32px;background:#fff;color:var(--t1);
font:14px/1.65 -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif}
.wrap{max-width:960px;margin:0 auto}
h1{font-size:22px;margin:0 0 4px}
.sub{color:var(--t3);font-size:12.5px;margin-bottom:22px}
.cards{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin-bottom:24px}
.card{border:1px solid var(--bd);border-radius:10px;padding:14px 16px;background:var(--bg)}
.card .k{font-size:11.5px;color:var(--t3);letter-spacing:.4px}
.card .v{font-size:24px;font-weight:600;margin-top:4px}
.v.ok{color:var(--ok)} .v.warn{color:var(--warn)} .v.err{color:var(--err)}
h2{font-size:15px;margin:26px 0 10px;padding-bottom:6px;border-bottom:1px solid var(--bd)}
table{width:100%;border-collapse:collapse;font-size:13px}
th,td{text-align:left;padding:8px 10px;border-bottom:1px solid var(--bd);vertical-align:top}
th{background:var(--bg);font-weight:600;font-size:12px;color:var(--t2)}
td.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px}
.tag{display:inline-block;padding:1px 8px;border-radius:10px;font-size:11.5px;font-weight:600}
.tag.pass{background:#dcfce7;color:var(--ok)}
.tag.warn{background:#fef3c7;color:var(--warn)}
.tag.fail{background:#fee2e2;color:var(--err)}
.sug{color:var(--t2);font-size:12px}
.foot{margin-top:28px;padding-top:12px;border-top:1px solid var(--bd);color:var(--t3);font-size:11.5px}
@media print{body{padding:0}.wrap{max-width:none}}
</style></head><body><div class="wrap">`)

	// Header
	b.WriteString(fmt.Sprintf(`<h1>主机合规报告</h1>
<div class="sub">主机 %s · OpsMini %s · 生成时间 %s</div>`,
		esc(in.Hostname), esc(in.Version), in.GeneratedAt.Format("2006-01-02 15:04:05")))

	// Summary cards
	scoreClass := "ok"
	if in.Score < 60 {
		scoreClass = "err"
	} else if in.Score < 80 {
		scoreClass = "warn"
	}
	b.WriteString(fmt.Sprintf(`<div class="cards">
<div class="card"><div class="k">安全评分</div><div class="v %s">%d</div></div>
<div class="card"><div class="k">基线通过率</div><div class="v">%.1f%%</div></div>
<div class="card"><div class="k">检查项</div><div class="v">%d</div></div>
<div class="card"><div class="k">未通过 / 警告</div><div class="v">%d / %d</div></div>
</div>`, scoreClass, in.Score, passRate, total, fail, warn))

	if weightTotal > 0 {
		b.WriteString(fmt.Sprintf(`<div class="sub">加权得分 %d / %d（通过项权重合计，按项权重计算而非简单计数）</div>`,
			weightGot, weightTotal))
	}

	// Category summary
	if len(catNames) > 0 {
		b.WriteString(`<h2>分类汇总</h2><table><thead><tr><th>类别</th><th>项数</th><th>通过</th><th>警告</th><th>未通过</th><th>通过率</th></tr></thead><tbody>`)
		for _, cat := range catNames {
			items := byCat[cat]
			p, w, f := 0, 0, 0
			for _, it := range items {
				switch it.Status {
				case "pass":
					p++
				case "warn":
					w++
				default:
					f++
				}
			}
			rate := 0.0
			if len(items) > 0 {
				rate = float64(p) / float64(len(items)) * 100
			}
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%d</td><td>%d</td><td>%d</td><td>%d</td><td>%.0f%%</td></tr>`,
				esc(catLabel(cat)), len(items), p, w, f, rate))
		}
		b.WriteString(`</tbody></table>`)
	}

	// Unpassed items come first: a compliance reviewer reads what is wrong, not
	// the 40 checks that passed.
	var bad, good []model.BaselineResult
	for _, it := range in.Items {
		if it.Status == "pass" {
			good = append(good, it)
		} else {
			bad = append(bad, it)
		}
	}
	if len(bad) > 0 {
		b.WriteString(`<h2>需要处理的项</h2><table><thead><tr><th>类别</th><th>检查项</th><th>状态</th><th>详情</th><th>建议</th></tr></thead><tbody>`)
		for _, it := range bad {
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td><span class="tag %s">%s</span></td><td>%s</td><td class="sug">%s</td></tr>`,
				esc(catLabel(it.Category)), esc(it.Title), statusClass(it.Status), statusText(it.Status),
				esc(it.Detail), esc(it.Suggestion)))
		}
		b.WriteString(`</tbody></table>`)
	}

	if len(good) > 0 {
		b.WriteString(`<h2>已通过的项</h2><table><thead><tr><th>类别</th><th>检查项</th><th>详情</th></tr></thead><tbody>`)
		for _, it := range good {
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td>%s</td><td>%s</td></tr>`,
				esc(catLabel(it.Category)), esc(it.Title), esc(it.Detail)))
		}
		b.WriteString(`</tbody></table>`)
	}

	// Context section
	b.WriteString(fmt.Sprintf(`<h2>其它安全信号</h2><table><thead><tr><th>项目</th><th>数量</th></tr></thead><tbody>
<tr><td>文件完整性变更（累计）</td><td>%d</td></tr>
<tr><td>未处理威胁</td><td>%d</td></tr>
</tbody></table>`, in.FimChanges, in.ThreatCount))

	b.WriteString(fmt.Sprintf(`<div class="foot">本报告由 OpsMini 自动生成，检查项依据内置基线规则（%d 项，权重合计 %d）。
基线规则随版本更新，跨版本比较请以同一版本的报告为准。生成于 %s。</div>`,
		total, weightTotal, in.GeneratedAt.Format("2006-01-02 15:04:05")))
	b.WriteString(`</div></body></html>`)
	return b.String()
}

func catLabel(k string) string {
	if v, ok := categoryLabels[k]; ok {
		return v
	}
	return k
}

func statusText(s string) string {
	switch s {
	case "pass":
		return "通过"
	case "warn":
		return "警告"
	default:
		return "未通过"
	}
}

func statusClass(s string) string {
	switch s {
	case "pass":
		return "pass"
	case "warn":
		return "warn"
	default:
		return "fail"
	}
}

// esc escapes user-controlled text (file paths, command output) before it lands
// in the report. A path containing <script> must not become markup.
func esc(s string) string { return html.EscapeString(s) }
