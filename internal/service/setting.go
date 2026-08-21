// Package service provides the business logic layer.
package service

import "github.com/opsmini/opsmini/internal/repository"

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
