package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/repository"
)

// AccessLogHandler panel access log handler.
type AccessLogHandler struct {
	repo *repository.AccessLogRepo
}

// NewAccessLogHandler constructor.
func NewAccessLogHandler(repo *repository.AccessLogRepo) *AccessLogHandler {
	return &AccessLogHandler{repo: repo}
}

// List GET /access-logs?limit=200
func (h *AccessLogHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	list, err := h.repo.List(limit)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}
