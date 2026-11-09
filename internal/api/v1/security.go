package v1

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// SecurityHandler security overview handler.
type SecurityHandler struct {
	svc *service.SecurityService
	// Baseline is used by the compliance report: it can run a scan on demand when
	// the host has never been checked. Optional.
	Baseline *service.BaselineService
	// Hostname / Version are stamped into the report; both fall back to sane
	// placeholders so a report is never generated with empty fields.
	Hostname func() string
	Version  string
	// FimChangeCount / ThreatCount add context to the report. Optional.
	FimChangeCount func() int
	ThreatCount    func() int
}

// NewSecurityHandler constructor.
func NewSecurityHandler(svc *service.SecurityService) *SecurityHandler {
	return &SecurityHandler{svc: svc}
}

// Overview GET /security/overview
func (h *SecurityHandler) Overview(c *gin.Context) {
	ov, err := h.svc.Overview()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, ov)
}

// Report GET /security/report - downloads the compliance report as one HTML file.
//
// It regenerates the number or not: an operator exporting a report expects "the
// current state", not "whatever was last scanned three weeks ago". The cost is
// forty-odd local file checks.
func (h *SecurityHandler) Report(c *gin.Context) {
	if h.Baseline == nil {
		response.Error(c, 404, response.CodeNotFound, "基线服务未启用")
		return
	}
	items, err := h.Baseline.Latest()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	if len(items) == 0 {
		// Generating a report from nothing would produce a document full of zeros
		// that looks like a failing host; scan first, then report.
		if items, err = h.Baseline.RunScan(); err != nil {
			response.Error(c, 500, response.CodeInternal, "尚未体检且自动体检失败："+err.Error())
			return
		}
	}
	ov, _ := h.svc.Overview()
	in := service.ReportInput{
		Hostname:    h.Hostname(),
		Version:     h.Version,
		GeneratedAt: time.Now(),
		Items:       items,
	}
	if ov != nil {
		in.Score = ov.Score
		in.Grade = ov.Grade
	}
	if h.FimChangeCount != nil {
		in.FimChanges = h.FimChangeCount()
	}
	if h.ThreatCount != nil {
		in.ThreatCount = h.ThreatCount()
	}
	html := service.BuildComplianceReport(in)
	name := fmt.Sprintf("opsmini-compliance-%s.html", time.Now().Format("20060102-1504"))
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Data(200, "text/html; charset=utf-8", []byte(html))
}
