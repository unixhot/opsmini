package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// signSSO builds a token exactly the way OpsAnt does (opsant/views/host_views.py):
// base64url(identity|expiry|hex(hmac_sha256(secret, identity|expiry))) without padding.
func signSSO(identity, secret string, expiry int64) string {
	payload := identity + "|" + strconv.FormatInt(expiry, 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	raw := payload + "|" + hex.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func TestVerifySSOTokenAcceptsInstanceID(t *testing.T) {
	const secret = "a3f1c9d2b7e84f60"
	const instanceID = "opsmini-7k3m9x2qv4ab"
	token := signSSO(instanceID, secret, time.Now().Add(5*time.Minute).Unix())

	// OpsAnt signs with the host identity it stores, which for OpsMini-managed hosts is
	// the panel instance ID. Verification used to compare only against os.Hostname(),
	// so a perfectly valid token was always rejected.
	if _, err := verifySSOToken(token, secret, []string{"web-01", instanceID}); err != nil {
		t.Fatalf("token signed with instance ID rejected: %v", err)
	}

	// A hostname-signed token must keep working: records created before the instance ID
	// existed still carry the OS hostname.
	legacy := signSSO("web-01", secret, time.Now().Add(5*time.Minute).Unix())
	if _, err := verifySSOToken(legacy, secret, []string{"web-01", instanceID}); err != nil {
		t.Fatalf("token signed with hostname rejected: %v", err)
	}

	// Case-insensitive, because OpsAnt may have recorded a differently cased hostname.
	upper := signSSO("WEB-01", secret, time.Now().Add(5*time.Minute).Unix())
	if _, err := verifySSOToken(upper, secret, []string{"web-01", instanceID}); err != nil {
		t.Fatalf("case-insensitive identity comparison failed: %v", err)
	}
}

func TestVerifySSOTokenRejectsForeignHost(t *testing.T) {
	const secret = "a3f1c9d2b7e84f60"
	token := signSSO("opsmini-otherhost01", secret, time.Now().Add(5*time.Minute).Unix())

	_, err := verifySSOToken(token, secret, []string{"web-01", "opsmini-7k3m9x2qv4ab"})
	if !errors.Is(err, ErrInvalidSSOToken) {
		t.Fatalf("expected ErrInvalidSSOToken, got %v", err)
	}
	if !strings.Contains(err.Error(), "issued for") {
		t.Fatalf("error should name the identity mismatch, got %v", err)
	}
}

func TestVerifySSOTokenRejectsWrongSecret(t *testing.T) {
	token := signSSO("opsmini-7k3m9x2qv4ab", "secret-one", time.Now().Add(5*time.Minute).Unix())

	_, err := verifySSOToken(token, "secret-two", []string{"opsmini-7k3m9x2qv4ab"})
	if !errors.Is(err, ErrInvalidSSOToken) {
		t.Fatalf("expected ErrInvalidSSOToken, got %v", err)
	}
	// The message has to point at the token mismatch: it is the failure an operator hits
	// after connecting the panel with a token that differs from OpsAnt's OPSMINI_TOKEN.
	if !strings.Contains(err.Error(), "signature mismatch") {
		t.Fatalf("error should mention the signature mismatch, got %v", err)
	}
}

func TestVerifySSOTokenRejectsExpired(t *testing.T) {
	const secret = "a3f1c9d2b7e84f60"
	token := signSSO("opsmini-7k3m9x2qv4ab", secret, time.Now().Add(-time.Minute).Unix())

	_, err := verifySSOToken(token, secret, []string{"opsmini-7k3m9x2qv4ab"})
	if !errors.Is(err, ErrInvalidSSOToken) || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected an expiry error, got %v", err)
	}
}

func TestVerifySSOTokenEmptySecretIsNotReportedAsDisabled(t *testing.T) {
	// An empty secret is a configuration gap. Reporting it as "sso login disabled" sent
	// operators to the enable switch while the real problem was the missing token.
	_, err := verifySSOToken("anything", "", []string{"web-01"})
	if !errors.Is(err, ErrSSONotConfigured) {
		t.Fatalf("expected ErrSSONotConfigured, got %v", err)
	}
	if errors.Is(err, ErrSSODisabled) {
		t.Fatalf("empty secret must not be reported as disabled")
	}
}

func TestSSOLoginReportsDisabledSwitchAndMissingTokenApart(t *testing.T) {
	// Only the pre-checks are exercised: reaching the repository needs a database, and the
	// two failure modes this test targets are decided before that.
	disabled := NewAuthService(nil, nil, nil, func() bool { return false }, nil, nil)
	if _, err := disabled.SSOLogin("t", "web-01"); !errors.Is(err, ErrSSODisabled) {
		t.Fatalf("expected ErrSSODisabled, got %v", err)
	}

	missingToken := NewAuthService(nil, nil, nil, func() bool { return true }, func() string { return "" }, nil)
	if _, err := missingToken.SSOLogin("t", "web-01"); !errors.Is(err, ErrSSONotConfigured) {
		t.Fatalf("expected ErrSSONotConfigured, got %v", err)
	}

	// A token saved in the panel must be picked up without a restart: the secret is
	// resolved per call, and "" then a value has to behave differently.
	var secret string
	resolved := NewAuthService(nil, nil, nil, func() bool { return true }, func() string { return secret }, nil)
	if _, err := resolved.SSOLogin("t", "web-01"); !errors.Is(err, ErrSSONotConfigured) {
		t.Fatalf("expected ErrSSONotConfigured while empty, got %v", err)
	}
	secret = "a3f1c9d2b7e84f60"
	if _, err := resolved.SSOLogin("t", "web-01"); errors.Is(err, ErrSSONotConfigured) {
		t.Fatalf("secret set later was not picked up: %v", err)
	}
}
