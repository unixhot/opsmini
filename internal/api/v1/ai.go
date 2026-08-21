package v1

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// AIHandler AI assistant handler.
type AIHandler struct {
	svc *service.AIService
}

// NewAIHandler constructor.
func NewAIHandler(svc *service.AIService) *AIHandler {
	return &AIHandler{svc: svc}
}

// chatReq chat request.
type chatReq struct {
	Message   string `json:"message" binding:"required"`
	Context   string `json:"context"`    // current page context (optional)
	DeepThink bool   `json:"deep_think"` // whether deep thinking is enabled
}

// Chat POST /ai/chat
func (h *AIHandler) Chat(c *gin.Context) {
	var req chatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}

	messages := buildMessages(req)

	reply, err := h.svc.Chat(messages, req.DeepThink)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"reply": reply})
}

// ChatStream POST /ai/chat/stream - SSE streaming output.
func (h *AIHandler) ChatStream(c *gin.Context) {
	var req chatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}

	messages := buildMessages(req)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.Error(c, 500, response.CodeInternal, "streaming unsupported")
		return
	}

	writeEvent := func(payload any) {
		data, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", data)
		flusher.Flush()
	}

	done := false
	err := h.svc.ChatStream(c.Request.Context(), messages, req.DeepThink, func(delta string) {
		writeEvent(map[string]string{"delta": delta})
	})
	if err != nil {
		writeEvent(map[string]string{"error": err.Error()})
	} else {
		done = true
	}
	if done {
		_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		flusher.Flush()
	}
}

// buildMessages assembles chat messages (including page context).
func buildMessages(req chatReq) []service.ChatMessage {
	messages := []service.ChatMessage{{Role: "user", Content: req.Message}}
	if req.Context != "" {
		messages = append([]service.ChatMessage{
			{Role: "system", Content: "当前用户所在页面上下文：" + req.Context},
		}, messages...)
	}
	return messages
}
