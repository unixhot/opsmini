// Package repository encapsulates the data access layer.
package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// SettingRepo provides data access for panel settings.
type SettingRepo struct {
	db *gorm.DB
}

// NewSettingRepo returns a new SettingRepo.
func NewSettingRepo(db *gorm.DB) *SettingRepo {
	return &SettingRepo{db: db}
}

// Get returns the setting value by key; returns an empty string if not found.
func (r *SettingRepo) Get(key string) (string, error) {
	var s model.Setting
	if err := r.db.First(&s, "key = ?", key).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return s.Value, nil
}

// List returns all settings as a map.
func (r *SettingRepo) List() (map[string]string, error) {
	var rows []model.Setting
	if err := r.db.Find(&rows).Error; err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, s := range rows {
		m[s.Key] = s.Value
	}
	return m, nil
}

// Set writes (upserts) a single setting.
func (r *SettingRepo) Set(key, value string) error {
	return r.db.Save(&model.Setting{Key: key, Value: value}).Error
}
