package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

// Models GET /ai/models
//
// Lists what the configured endpoint actually serves. Local deployments name their
// models freely (`qwen2.5:7b-instruct-q4_K_M`, or whatever --served-model-name was
// set), so a free-text field is a guessing game — the panel fills its dropdown from
// this instead.
// Models probes a provider and returns the models it serves.
//
// The body is optional: send {provider, base, key} to test the values currently in
// the form (they are not saved), or send nothing to use the saved configuration.
// POST rather than GET on purpose — the API key must not end up in an access log or
// a proxy URL.
func (h *AIHandler) Models(c *gin.Context) {
	var req struct {
		Provider string `json:"provider"`
		Base     string `json:"base"`
		Key      string `json:"key"`
	}
	_ = c.ShouldBindJSON(&req)

	var (
		models []string
		err    error
	)
	if strings.TrimSpace(req.Base) != "" {
		models, err = h.svc.ListModelsFor(c.Request.Context(), req.Provider, req.Base, req.Key)
	} else {
		models, err = h.svc.ListModels(c.Request.Context())
	}
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"models": models})
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
