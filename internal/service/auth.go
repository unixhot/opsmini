// Package service provides the business logic layer.
package service

import (
	"errors"
	"os"
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
}

// NewAuthService creates an AuthService.
func NewAuthService(repo *repository.UserRepo, jwtMgr *jwt.Manager, mfaEnabled func() bool) *AuthService {
	return &AuthService{repo: repo, jwt: jwtMgr, mfaEnabled: mfaEnabled}
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
