package v1

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/creack/pty"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/opsmini/opsmini/internal/pkg/jwt"
	"github.com/opsmini/opsmini/internal/pkg/response"
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

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	cmd := exec.Command(shell)
	// Critical: set TERM and other terminal env vars, otherwise full-screen interactive programs like top/vim cannot render correctly.
	// Use C.UTF-8 for LANG (built into glibc, avoids setlocale warnings when en_US.UTF-8 is not installed).
	// VIMINIT fallback:
	//   - conceallevel=0 / concealcursor=nc: disable markdown syntax conceal so ## heading chars aren't hidden and misalign the cursor;
	//   - ttimeoutlen=500: override the ttimeoutlen=100 default in CentOS/RHEL /etc/vimrc (too short).
	//     If the delay between bytes of the arrow-key escape sequence (\x1bOC) exceeds 100ms (common over the public internet),
	//     vim misreads \x1b as ESC and exits insert mode, and treats OC as plain chars, breaking arrow keys in insert mode.
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"LANG=C.UTF-8",
		"VIMINIT=set conceallevel=0 concealcursor=nc ttimeoutlen=500",
	)
	// Use StartWithSize to give the pty an initial size (after the frontend fits, the real size is synced via resize messages).
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 120, Rows: 32})
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("failed to start pty: "+err.Error()))
		return
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
	}()

	// pty → ws
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
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
			if handleResize(ptmx, msg) {
				continue
			}
			// Write in a loop to ensure the entire input is written to the pty.
			// Otherwise, for raw-mode programs like vim during fast input/redraw, the pty write buffer may fill up,
			// and after a single partial Write the remaining bytes are dropped, causing file content to mismatch input.
			for len(msg) > 0 {
				n, werr := ptmx.Write(msg)
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
func handleResize(ptmx *os.File, msg []byte) bool {
	var m struct {
		Type string  `json:"type"`
		Cols float64 `json:"cols"`
		Rows float64 `json:"rows"`
	}
	if err := json.Unmarshal(msg, &m); err != nil || m.Type != "resize" {
		return false
	}
	_ = pty.Setsize(ptmx, &pty.Winsize{
		Cols: uint16(m.Cols),
		Rows: uint16(m.Rows),
	})
	return true
}
