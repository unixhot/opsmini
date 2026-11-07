// Package service provides the business logic layer.
package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/pkg/jwt"
	"github.com/opsmini/opsmini/internal/repository"
)

var (
	// ErrInvalidCredential indicates an invalid username or password.
	ErrInvalidCredential = errors.New("invalid username or password")
	// ErrUserDisabled indicates the user is disabled.
	ErrUserDisabled = errors.New("user disabled")
	// ErrInvalidMFACode indicates an invalid MFA code.
	ErrInvalidMFACode = errors.New("invalid mfa code")
	// ErrInvalidMFAToken indicates an invalid MFA pre-auth token.
	ErrInvalidMFAToken = errors.New("invalid mfa token")
	// ErrSSODisabled indicates the OpsAnt SSO login is disabled by the panel switch.
	ErrSSODisabled = errors.New("sso login disabled: turn on \"Allow OpsAnt SSO login\" in panel settings")
	// ErrInvalidSSOToken indicates an invalid or expired SSO token.
	ErrInvalidSSOToken = errors.New("invalid sso token")
	// ErrSSONotConfigured indicates the panel accepts SSO but has no shared token to
	// verify signatures with. It is a configuration gap, not a policy decision, and
	// reporting it as "disabled" sent operators to the wrong switch.
	ErrSSONotConfigured = errors.New("sso not configured: OpsAnt token is empty, fill it in Settings > Connect to OpsAnt (or agent.server_token in config.yaml)")
)

// TokenPair is the token pair returned on login.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// LoginResult is the login result: when credentials pass but MFA is required,
// MFARequired and MFAToken are returned; otherwise the token pair is returned directly.
type LoginResult struct {
	MFARequired  bool   `json:"mfa_required"`
	MFAToken     string `json:"mfa_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// AuthService handles authentication.
type AuthService struct {
	repo       *repository.UserRepo
	jwt        *jwt.Manager
	mfaEnabled func() bool // global MFA toggle (read from settings)
	ssoEnabled func() bool // OpsAnt SSO login toggle (read from settings)
	// ssoSecret resolves the shared secret for verifying OpsAnt SSO tokens on every
	// request. It must be a function: the token is normally configured in the panel
	// (settings table) and can be changed without restarting the process, so a value
	// captured at startup would go stale or stay empty.
	ssoSecret func() string
	// ssoIdentity returns host identities other than os.Hostname() that may appear in
	// an OpsAnt token. OpsAnt signs with the host identity it stores for the host,
	// which for OpsMini-managed hosts is the panel instance ID.
	ssoIdentity func() []string
}

// NewAuthService creates an AuthService.
func NewAuthService(repo *repository.UserRepo, jwtMgr *jwt.Manager, mfaEnabled func() bool, ssoEnabled func() bool, ssoSecret func() string, ssoIdentity func() []string) *AuthService {
	return &AuthService{repo: repo, jwt: jwtMgr, mfaEnabled: mfaEnabled, ssoEnabled: ssoEnabled, ssoSecret: ssoSecret, ssoIdentity: ssoIdentity}
}

// Login verifies credentials; if MFA is globally enabled and the user has it bound,
// it returns an MFA pre-auth token.
func (s *AuthService) Login(username, password string) (*LoginResult, error) {
	u, err := s.repo.FindByUsername(username)
	if err != nil {
		return nil, ErrInvalidCredential
	}
	if u.Status != 1 {
		return nil, ErrUserDisabled
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredential
	}

	if s.mfaEnabled != nil && s.mfaEnabled() && u.MFAEnabled && u.MFASecret != "" {
		mfaToken, err := s.jwt.GenerateMFA(u.ID, u.Role)
		if err != nil {
			return nil, err
		}
		return &LoginResult{MFARequired: true, MFAToken: mfaToken}, nil
	}

	now := time.Now()
	u.LastLogin = &now
	_ = s.repo.Update(u)

	access, refresh, err := s.jwt.Generate(u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	return &LoginResult{AccessToken: access, RefreshToken: refresh}, nil
}

// SSOLogin verifies a one-time SSO token issued by OpsAnt and logs in as the admin user.
func (s *AuthService) SSOLogin(ssoToken, localHostname string) (*TokenPair, error) {
	if s.ssoEnabled == nil || !s.ssoEnabled() {
		return nil, ErrSSODisabled
	}
	secret := ""
	if s.ssoSecret != nil {
		secret = strings.TrimSpace(s.ssoSecret())
	}
	if secret == "" {
		return nil, ErrSSONotConfigured
	}
	// os.Hostname() is only one of the identities OpsAnt may have signed with; the
	// instance ID is the one it uses for hosts discovered through the OpsMini link.
	identities := []string{localHostname}
	if s.ssoIdentity != nil {
		identities = append(identities, s.ssoIdentity()...)
	}
	if _, err := verifySSOToken(ssoToken, secret, identities); err != nil {
		return nil, err
	}
	// 以管理员身份登录（OpsAnt 已完成认证，信任该来源）
	var admin *model.User
	users, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	for i := range users {
		if users[i].Role == "admin" && users[i].Status == 1 {
			admin = &users[i]
			break
		}
	}
	if admin == nil {
		return nil, ErrUserDisabled
	}
	access, refresh, err := s.jwt.Generate(admin.ID, admin.Role)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

// verifySSOToken verifies an OpsAnt-issued token (format: base64url(identity|expiry|signature)).
//
// identity is whatever OpsAnt stores as the host name for the host it is opening. For
// hosts discovered through the OpsMini link that value is the panel instance ID
// (opsmini-xxxxxxxxxxxx), not the machine's hostname, so the caller passes every
// identity this panel answers to. The signature is checked before identity and expiry
// are reported on, so an unauthenticated caller cannot probe for them.
func verifySSOToken(ssoToken, secret string, identities []string) (string, error) {
	if secret == "" {
		return "", ErrSSONotConfigured
	}
	raw, err := base64.RawURLEncoding.DecodeString(ssoToken)
	if err != nil {
		return "", fmt.Errorf("%w: malformed encoding", ErrInvalidSSOToken)
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 {
		return "", fmt.Errorf("%w: malformed payload", ErrInvalidSSOToken)
	}
	identity, expiryStr, sig := parts[0], parts[1], parts[2]
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return "", fmt.Errorf("%w: malformed expiry", ErrInvalidSSOToken)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(identity + "|" + expiryStr))
	if !hmac.Equal([]byte(sig), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		// The most common real-world cause is a token that differs between OpsAnt and
		// this panel, so say so instead of leaving "invalid" to be interpreted.
		return "", fmt.Errorf("%w: signature mismatch (OpsAnt token differs from the one configured here)", ErrInvalidSSOToken)
	}
	if expiry < time.Now().Unix() {
		return "", fmt.Errorf("%w: expired", ErrInvalidSSOToken)
	}
	if !matchIdentity(identity, identities) {
		return "", fmt.Errorf("%w: token was issued for %q, this panel answers to %v", ErrInvalidSSOToken, identity, identities)
	}
	return identity, nil
}

// matchIdentity reports whether id equals one of the accepted identities. Comparison is
// case-insensitive because hostnames are, and OpsAnt may record a different case than
// os.Hostname() returns locally.
func matchIdentity(id string, identities []string) bool {
	for _, want := range identities {
		if want = strings.TrimSpace(want); want != "" && strings.EqualFold(id, want) {
			return true
		}
	}
	return false
}

// VerifyMFALogin verifies the MFA code and issues the formal tokens on success.
func (s *AuthService) VerifyMFALogin(mfaToken, code string) (*TokenPair, error) {
	claims, err := s.jwt.Parse(mfaToken)
	if err != nil || claims.Purpose != "mfa" {
		return nil, ErrInvalidMFAToken
	}
	u, err := s.repo.FindByID(claims.UserID)
	if err != nil {
		return nil, ErrInvalidMFAToken
	}
	if !ValidateTOTP(u.MFASecret, code) {
		return nil, ErrInvalidMFACode
	}
	now := time.Now()
	u.LastLogin = &now
	_ = s.repo.Update(u)

	access, refresh, err := s.jwt.Generate(u.ID, u.Role)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

// SetupMFA generates a new TOTP secret and otpauth URI for the user (not yet enabled).
// The issuer includes the hostname to distinguish between multiple OpsMini instances.
func (s *AuthService) SetupMFA(uid uint) (secret, otpauthURL string, err error) {
	u, err := s.repo.FindByID(uid)
	if err != nil {
		return "", "", err
	}
	secret, err = GenerateTOTPSecret()
	if err != nil {
		return "", "", err
	}
	u.MFASecret = secret
	u.MFAEnabled = false // enabled only after verification passes
	if err := s.repo.Update(u); err != nil {
		return "", "", err
	}
	issuer := "OpsMini"
	if host, e := os.Hostname(); e == nil && host != "" {
		issuer = "OpsMini(" + host + ")"
	}
	otpauthURL = BuildOTPAuthURL(secret, u.Username, issuer)
	return secret, otpauthURL, nil
}

// EnableMFA enables MFA after the dynamic code passes verification.
func (s *AuthService) EnableMFA(uid uint, code string) error {
	u, err := s.repo.FindByID(uid)
	if err != nil {
		return err
	}
	if u.MFASecret == "" {
		return errors.New("mfa not set up")
	}
	if !ValidateTOTP(u.MFASecret, code) {
		return ErrInvalidMFACode
	}
	u.MFAEnabled = true
	return s.repo.Update(u)
}

// DisableMFA disables and clears the user's MFA binding.
func (s *AuthService) DisableMFA(uid uint) error {
	u, err := s.repo.FindByID(uid)
	if err != nil {
		return err
	}
	u.MFAEnabled = false
	u.MFASecret = ""
	return s.repo.Update(u)
}

// MFAStatus returns the user's MFA binding status and the global toggle state.
func (s *AuthService) MFAStatus(uid uint) (map[string]interface{}, error) {
	u, err := s.repo.FindByID(uid)
	if err != nil {
		return nil, err
	}
	global := false
	if s.mfaEnabled != nil {
		global = s.mfaEnabled()
	}
	return map[string]interface{}{
		"mfa_enabled":  u.MFAEnabled,
		"mfa_global":   global,
		"mfa_username": u.Username,
	}, nil
}

// Refresh exchanges a refresh token for a new token pair.
func (s *AuthService) Refresh(refresh string) (*TokenPair, error) {
	claims, err := s.jwt.Parse(refresh)
	if err != nil {
		return nil, errors.New("invalid refresh token")
	}
	access, newRefresh, err := s.jwt.Generate(claims.UserID, claims.Role)
	if err != nil {
		return nil, err
	}
	return &TokenPair{AccessToken: access, RefreshToken: newRefresh}, nil
}

// Logout revokes a token (added to the blacklist until it expires naturally).
func (s *AuthService) Logout(tokenStr string) {
	s.jwt.Revoke(tokenStr)
}

// GetUser looks up a user by username (used by Me and others).
func (s *AuthService) GetUser(username string) (*model.User, error) {
	return s.repo.FindByUsername(username)
}
