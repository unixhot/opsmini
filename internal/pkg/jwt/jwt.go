// Package jwt wraps JWT signing and verification.
package jwt

import (
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims holds the custom claims.
type Claims struct {
	UserID  uint   `json:"uid"`
	Role    string `json:"role"`
	Purpose string `json:"purpose,omitempty"` // empty = normal login; "mfa" = MFA pre-authentication
	jwt.RegisteredClaims
}

// Manager is the JWT manager.
type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration

	mu      sync.Mutex
	revoked map[string]time.Time // token -> expiration time (in-memory blacklist)
}

// NewManager creates a manager.
func NewManager(secret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		revoked:    make(map[string]time.Time),
	}
}

// Generate signs a pair of access/refresh tokens.
func (m *Manager) Generate(userID uint, role string) (access, refresh string, err error) {
	access, err = m.sign(userID, role, "", m.accessTTL)
	if err != nil {
		return "", "", err
	}
	refresh, err = m.sign(userID, role, "", m.refreshTTL)
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

// GenerateMFA signs a short-lived (5-minute) MFA pre-authentication token,
// used only during the stage after username/password verification has passed but MFA verification is still pending.
func (m *Manager) GenerateMFA(userID uint, role string) (string, error) {
	return m.sign(userID, role, "mfa", 5*time.Minute)
}

// IsMFA reports whether the token is an MFA pre-authentication token.
func (m *Manager) IsMFA(tokenStr string) bool {
	claims, err := m.Parse(tokenStr)
	return err == nil && claims.Purpose == "mfa"
}

// sign issues a signed JWT for the given user with the given purpose (access/refresh) and TTL.
func (m *Manager) sign(userID uint, role, purpose string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:  userID,
		Role:    role,
		Purpose: purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Parse parses and validates the token.
func (m *Manager) Parse(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid token")
}

// Revoke adds the token to the blacklist until it expires naturally.
func (m *Manager) Revoke(tokenStr string) {
	claims, err := m.Parse(tokenStr)
	if err != nil {
		return
	}
	exp := time.Now()
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Time
	}
	m.mu.Lock()
	m.revoked[tokenStr] = exp
	m.mu.Unlock()
}

// IsRevoked reports whether the token has been revoked (and cleans up expired entries along the way).
func (m *Manager) IsRevoked(tokenStr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for k, exp := range m.revoked {
		if now.After(exp) {
			delete(m.revoked, k)
		}
	}
	_, ok := m.revoked[tokenStr]
	return ok
}
