// Package service provides the business logic layer.
package service

import (
	"errors"
	"strings"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// AppCategoryService handles app category operations.
type AppCategoryService struct {
	repo *repository.AppCategoryRepo
}

// NewAppCategoryService creates an AppCategoryService.
func NewAppCategoryService(repo *repository.AppCategoryRepo) *AppCategoryService {
	return &AppCategoryService{repo: repo}
}

// List returns all categories.
func (s *AppCategoryService) List() ([]model.AppCategory, error) {
	return s.repo.List()
}

// Create creates a custom category.
func (s *AppCategoryService) Create(key, name string) (*model.AppCategory, error) {
	key = strings.TrimSpace(key)
	if key == "" || name == "" {
		return nil, errors.New("分类标识与名称不能为空")
	}
	if _, err := s.repo.FindByKey(key); err == nil {
		return nil, errors.New("分类标识已存在")
	}
	c := &model.AppCategory{Key: key, Name: name, Builtin: false}
	if err := s.repo.Create(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Update updates a category (built-in categories can only be renamed).
func (s *AppCategoryService) Update(id uint, key, name string) error {
	list, err := s.repo.List()
	if err != nil {
		return err
	}
	for i := range list {
		if list[i].ID == id {
			if !list[i].Builtin {
				list[i].Key = key
			}
			list[i].Name = name
			return s.repo.Update(&list[i])
		}
	}
	return errors.New("分类不存在")
}

// Delete deletes a custom category (built-in categories cannot be deleted).
func (s *AppCategoryService) Delete(id uint) error {
	list, err := s.repo.List()
	if err != nil {
		return err
	}
	for _, c := range list {
		if c.ID == id {
			if c.Builtin {
				return errors.New("内置分类不可删除")
			}
			return s.repo.Delete(id)
		}
	}
	return errors.New("分类不存在")
}
