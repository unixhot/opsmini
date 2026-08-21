package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// CrontabHandler read-only system/user crontab interface.
type CrontabHandler struct {
	svc *service.CrontabService
}

// NewCrontabHandler constructor.
func NewCrontabHandler(svc *service.CrontabService) *CrontabHandler {
	return &CrontabHandler{svc: svc}
}

// SystemJobs GET /cron-jobs/system - system-level crontab (/etc/crontab, /etc/cron.d/*).
func (h *CrontabHandler) SystemJobs(c *gin.Context) {
	response.OK(c, h.svc.SystemJobs())
}

// UserJobs GET /cron-jobs/user - user crontab (/var/spool/cron/*).
func (h *CrontabHandler) UserJobs(c *gin.Context) {
	response.OK(c, h.svc.UserJobs())
}
