package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// NotificationHandler notification handler.
type NotificationHandler struct {
	svc *service.NotificationService
}

// NewNotificationHandler constructor.
func NewNotificationHandler(svc *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// List GET /notifications?limit=200 - notification list + unread count (time descending).
func (h *NotificationHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	list, err := h.svc.List(limit)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	unread, _ := h.svc.UnreadCount()
	response.OK(c, gin.H{"unread": unread, "notifications": list})
}

// UnreadCount GET /notifications/unread-count - unread count (for the top-right badge).
func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	n, err := h.svc.UnreadCount()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"unread": n})
}

// MarkRead PUT /notifications/:id/read - mark a single one as read.
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	if err := h.svc.MarkRead(uint(id)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// MarkAllRead PUT /notifications/read-all - mark all as read.
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	if err := h.svc.MarkAllRead(); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Delete DELETE /notifications/:id - delete a single one.
func (h *NotificationHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// ClearAll DELETE /notifications - clear all.
func (h *NotificationHandler) ClearAll(c *gin.Context) {
	if err := h.svc.ClearAll(); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}
