package v1

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/opsmini/opsmini/internal/pkg/jwt"
	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/platform/screen"
)

// GraphicHandler streams the interactive desktop to the browser as JPEG frames.
//
// The command-line terminal covers most administration; this exists for the
// things a shell cannot do — a GUI installer, a print dialog, a vendor tool that
// only ships an MFC window. First version is view-only: frames go out, input does
// not come in, so a mistake here cannot move the mouse of a logged-in user.
type GraphicHandler struct {
	jwtMgr   *jwt.Manager
	hasPerm  func(role, perm string) bool
	upgrader websocket.Upgrader
}

// NewGraphicHandler constructor. Permission is terminal.use, the same one that
// gates the shell: whoever may run commands on the host may also look at it.
func NewGraphicHandler(jwtMgr *jwt.Manager, hasPerm func(role, perm string) bool) *GraphicHandler {
	return &GraphicHandler{
		jwtMgr:  jwtMgr,
		hasPerm: hasPerm,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1 << 20, // one 1600px JPEG comfortably fits
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
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

// Serve GET /terminal/graphic?token=xxx&fps=2&quality=60&width=1600
func (h *GraphicHandler) Serve(c *gin.Context) {
	claims, err := h.jwtMgr.Parse(c.Query("token"))
	if err != nil {
		response.Error(c, 401, response.CodeUnauthorized, "invalid token")
		return
	}
	if h.hasPerm != nil && !h.hasPerm(claims.Role, "terminal.use") {
		response.Error(c, 403, response.CodeForbidden, "forbidden")
		return
	}

	fps := clampInt(c.Query("fps"), 1, 10, 2)
	quality := clampInt(c.Query("quality"), 20, 90, 60)
	width := clampInt(c.Query("width"), 320, 3840, 1600)
	// Input injection is opt-in per connection. Defaulting to read-only means a
	// stray connection (a cached tab, a monitoring probe) can never move the
	// mouse of whoever is sitting at the machine.
	control := c.Query("control") == "1"
	interval := time.Second / time.Duration(fps)

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// A read loop is required even in read-only mode: without it the control
	// frames (close, ping) are never processed and a closed tab leaves the
	// capture loop running until a write fails.
	done := make(chan struct{})
	injector := screen.NewInput()
	go func() {
		defer close(done)
		for {
			_, msg, rerr := conn.ReadMessage()
			if rerr != nil {
				return
			}
			if control {
				applyInput(injector, msg)
			}
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			frame, w, h, cerr := screen.Capture(c.Request.Context(), width, quality)
			if cerr != nil {
				_ = conn.WriteJSON(gin.H{"error": cerr.Error()})
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				return
			}
			// Dimensions travel as text so the viewer can size its canvas before
			// the first frame is decoded.
			if err := conn.WriteJSON(gin.H{"width": w, "height": h, "bytes": len(frame)}); err != nil {
				return
			}
		}
	}
}

// applyInput decodes one browser input event and hands it to the injector.
// Malformed messages are ignored: they can only come from our own page, and
// dropping one is better than tearing down the session mid-drag.
func applyInput(in screen.Input, msg []byte) {
	var ev struct {
		Type   string  `json:"type"`
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Button int     `json:"button"`
		Down   bool    `json:"down"`
		Delta  int     `json:"delta"`
		VK     int     `json:"vk"`
	}
	if err := json.Unmarshal(msg, &ev); err != nil {
		return
	}
	switch ev.Type {
	case "move":
		_ = in.MoveTo(ev.X, ev.Y)
	case "button":
		_ = in.Button(ev.Button, ev.Down)
	case "wheel":
		_ = in.Wheel(ev.Delta)
	case "key":
		if ev.VK > 0 && ev.VK < 256 {
			_ = in.Key(uint16(ev.VK), ev.Down)
		}
	}
}

func clampInt(raw string, min, max, def int) int {
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
