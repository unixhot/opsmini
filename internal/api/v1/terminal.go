package v1

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/opsmini/opsmini/internal/pkg/jwt"
	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/platform/conpty"
)

// TerminalHandler web terminal handler (WebSocket + pty).
type TerminalHandler struct {
	jwtMgr   *jwt.Manager
	hasPerm  func(role, perm string) bool
	upgrader websocket.Upgrader
}

// NewTerminalHandler constructor. hasPerm is used to check terminal permission (terminal.use).
func NewTerminalHandler(jwtMgr *jwt.Manager, hasPerm func(role, perm string) bool) *TerminalHandler {
	return &TerminalHandler{
		jwtMgr:  jwtMgr,
		hasPerm: hasPerm,
		upgrader: websocket.Upgrader{
			// Only allow same-origin connections to prevent cross-site WebSocket hijacking (CSWSH).
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true // no Origin header (e.g. curl/ws clients), allow
				}
				u, err := url.Parse(origin)
				if err != nil {
					return false
				}
				return strings.EqualFold(u.Host, r.Host)
			},
		},
	}
}

// Serve GET /terminal?token=xxx
func (h *TerminalHandler) Serve(c *gin.Context) {
	// WebSocket cannot carry the Authorization header, so authenticate via query token instead.
	claims, err := h.jwtMgr.Parse(c.Query("token"))
	if err != nil {
		response.Error(c, 401, response.CodeUnauthorized, "invalid token")
		return
	}
	// Check terminal permission
	if h.hasPerm != nil && !h.hasPerm(claims.Role, "terminal.use") {
		response.Error(c, 403, response.CodeForbidden, "forbidden")
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	shell := conpty.DefaultShell()
	// Use the platform console so a Windows host gets a ConPTY and a Unix host a
	// pty; an initial size is given because the frontend only syncs the real one
	// after it fits the container.
	console, err := conpty.Start(c.Request.Context(), shell, conpty.ShellEnv(shell), 120, 32)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("failed to start console: "+err.Error()))
		return
	}
	defer func() { _ = console.Close() }()

	// pty → ws
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := console.Read(buf)
			if n > 0 {
				if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// ws → pty
	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		switch mt {
		case websocket.TextMessage, websocket.BinaryMessage:
			if handleResize(console, msg) {
				continue
			}
			// Write in a loop to ensure the entire input is written to the pty.
			// Otherwise, for raw-mode programs like vim during fast input/redraw, the pty write buffer may fill up,
			// and after a single partial Write the remaining bytes are dropped, causing file content to mismatch input.
			for len(msg) > 0 {
				n, werr := console.Write(msg)
				if werr != nil {
					return
				}
				if n <= 0 {
					return
				}
				msg = msg[n:]
			}
		case websocket.CloseMessage:
			return
		}
	}
}

// handleResize parses the window resize message {"type":"resize","cols":N,"rows":N}.
func handleResize(con conpty.Console, msg []byte) bool {
	var m struct {
		Type string  `json:"type"`
		Cols float64 `json:"cols"`
		Rows float64 `json:"rows"`
	}
	if err := json.Unmarshal(msg, &m); err != nil || m.Type != "resize" {
		return false
	}
	_ = con.Resize(uint16(m.Cols), uint16(m.Rows))
	return true
}
