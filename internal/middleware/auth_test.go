package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/jwt"
)

// newTestJWT returns a manager plus a freshly signed access token.
func newTestJWT(t *testing.T) (*jwt.Manager, string) {
	t.Helper()
	mgr := jwt.NewManager("test-secret", time.Hour, 24*time.Hour)
	access, _, err := mgr.Generate(7, "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return mgr, access
}

// runAuth registers a route guarded by mw and reports the status code, so the
// middleware is exercised the way Gin actually uses it.
func runAuth(mw gin.HandlerFunc, req *http.Request) int {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/probe", mw, func(c *gin.Context) {
		uid := c.GetUint("userID")
		c.JSON(http.StatusOK, gin.H{"uid": uid, "role": c.GetString("role")})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// TestAuthQueryAcceptsHeaderAndQuery documents why the report endpoints need this
// middleware: the browser opens them in a new tab, where no header can be set.
func TestAuthQueryAcceptsHeaderAndQuery(t *testing.T) {
	mgr, token := newTestJWT(t)

	for name, req := range map[string]*http.Request{
		"header": httptest.NewRequest(http.MethodGet, "/probe", nil),
		"query":  httptest.NewRequest(http.MethodGet, "/probe?token="+token, nil),
	} {
		if name == "header" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if code := runAuth(AuthQuery(mgr, "token"), req); code != http.StatusOK {
			t.Errorf("%s: want 200, got %d", name, code)
		}
	}
}

// TestAuthQueryRejects covers the ways a caller can fail to authenticate.
func TestAuthQueryRejects(t *testing.T) {
	mgr, token := newTestJWT(t)

	cases := map[string]struct {
		req  *http.Request
		code int
	}{
		"no credentials": {
			req:  httptest.NewRequest(http.MethodGet, "/probe", nil),
			code: http.StatusUnauthorized,
		},
		"empty query token": {
			req:  httptest.NewRequest(http.MethodGet, "/probe?token=", nil),
			code: http.StatusUnauthorized,
		},
		"garbage query token": {
			req:  httptest.NewRequest(http.MethodGet, "/probe?token=not-a-jwt", nil),
			code: http.StatusUnauthorized,
		},
		"garbage header": {
			req:  httptest.NewRequest(http.MethodGet, "/probe", nil),
			code: http.StatusUnauthorized,
		},
	}
	for name, tc := range cases {
		if name == "garbage header" {
			tc.req.Header.Set("Authorization", "Bearer not-a-jwt")
		}
		if code := runAuth(AuthQuery(mgr, "token"), tc.req); code != tc.code {
			t.Errorf("%s: want %d, got %d", name, tc.code, code)
		}
	}

	// A revoked token must be refused even when supplied in the query string.
	mgr.Revoke(token)
	if code := runAuth(AuthQuery(mgr, "token"), httptest.NewRequest(http.MethodGet, "/probe?token="+token, nil)); code != http.StatusUnauthorized {
		t.Errorf("revoked token: want 401, got %d", code)
	}
}

// TestAuthIgnoresQueryToken guards the boundary: the plain Auth middleware must
// keep rejecting query-string tokens, so widening report access cannot silently
// widen the rest of the API.
func TestAuthIgnoresQueryToken(t *testing.T) {
	mgr, token := newTestJWT(t)
	req := httptest.NewRequest(http.MethodGet, "/probe?token="+token, nil)
	if code := runAuth(Auth(mgr), req); code != http.StatusUnauthorized {
		t.Errorf("Auth must not accept a query token, got %d", code)
	}
}
