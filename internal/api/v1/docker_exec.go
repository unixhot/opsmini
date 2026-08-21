package v1

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/opsmini/opsmini/internal/pkg/response"
)

// ContainerExec GET /containers/:id/exec?token=xxx - interactively enter a container (WebSocket + Docker exec).
// Reuse the host terminal's WebSocket protocol: I/O is binary frames; resize message {"type":"resize","cols":N,"rows":N}.
func (h *DockerHandler) ContainerExec(c *gin.Context) {
	// WebSocket cannot carry the Authorization header, so authenticate via query token instead.
	claims, err := h.jwtMgr.Parse(c.Query("token"))
	if err != nil {
		response.Error(c, 401, response.CodeUnauthorized, "invalid token")
		return
	}
	if h.hasPerm != nil && !h.hasPerm(claims.Role, "container.edit") {
		response.Error(c, 403, response.CodeForbidden, "forbidden")
		return
	}

	containerID := c.Param("id")

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Initial size 120x32; after the frontend fits, the real size is synced via resize messages.
	execID, attach, err := h.svc.ExecShell(containerID, 120, 32)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("failed to exec into container: "+err.Error()))
		return
	}
	defer attach.Close()

	// attach(exec) → ws
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := attach.Reader.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if rerr != nil {
				return
			}
		}
	}()

	// ws → attach(exec)
	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.TextMessage, websocket.BinaryMessage:
			if h.handleExecResize(execID, msg) {
				continue
			}
			if _, werr := attach.Conn.Write(msg); werr != nil {
				return
			}
		case websocket.CloseMessage:
			return
		}
	}
}

// handleExecResize parses the resize message and adjusts the exec terminal size.
func (h *DockerHandler) handleExecResize(execID string, msg []byte) bool {
	var m struct {
		Type string  `json:"type"`
		Cols float64 `json:"cols"`
		Rows float64 `json:"rows"`
	}
	if err := json.Unmarshal(msg, &m); err != nil || m.Type != "resize" {
		return false
	}
	_ = h.svc.ExecResize(execID, uint(m.Cols), uint(m.Rows))
	return true
}
