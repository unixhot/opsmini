// Package service provides the business logic layer.
package service

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/opsmini/opsmini/internal/repository"
)

// SettingService handles panel settings business logic.
type SettingService struct {
	repo *repository.SettingRepo
}

// NewSettingService creates a SettingService.
func NewSettingService(repo *repository.SettingRepo) *SettingService {
	return &SettingService{repo: repo}
}

// Get reads a setting by key.
func (s *SettingService) Get(key string) (string, error) {
	return s.repo.Get(key)
}

// List returns all settings.
func (s *SettingService) List() (map[string]string, error) {
	return s.repo.List()
}

// Set writes settings in batch.
func (s *SettingService) Set(kv map[string]string) error {
	for k, v := range kv {
		if err := s.repo.Set(k, v); err != nil {
			return err
		}
	}
	return nil
}

// EnsureSecret ensures the setting key holds a random secret value, generating
// and persisting a 32-byte random hex string when it is missing. It returns the
// effective secret. Used for server-side secrets (JWT signing key) that must not
// live in the config file or be exposed through the UI.
func (s *SettingService) EnsureSecret(key string) (string, error) {
	if v, err := s.Get(key); err == nil && v != "" {
		return v, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	secret := hex.EncodeToString(b)
	if err := s.Set(map[string]string{key: secret}); err != nil {
		return "", err
	}
	return secret, nil
}
