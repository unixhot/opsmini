package service

import (
	"errors"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// WebsiteService handles website management.
type WebsiteService struct {
	repo *repository.WebsiteRepo
}

// NewWebsiteService creates a WebsiteService.
func NewWebsiteService(repo *repository.WebsiteRepo) *WebsiteService {
	return &WebsiteService{repo: repo}
}

// WebsiteReq is the input for creating/updating a website.
type WebsiteReq struct {
	Domain     string `json:"domain" binding:"required"`
	Dir        string `json:"dir"`
	Runtime    string `json:"runtime"`
	PHPVersion string `json:"php_version"`
	SSL        bool   `json:"ssl"`
	Status     int    `json:"status"`
}

// Create creates a website.
func (s *WebsiteService) Create(req WebsiteReq) (*model.Website, error) {
	w := &model.Website{
		Domain:     req.Domain,
		Dir:        req.Dir,
		Runtime:    orDefault(req.Runtime, "php"),
		PHPVersion: req.PHPVersion,
		SSL:        req.SSL,
		Status:     orStatus(req.Status),
	}
	if err := s.repo.Create(w); err != nil {
		return nil, err
	}
	return w, nil
}

// Update updates a website.
func (s *WebsiteService) Update(id uint, req WebsiteReq) (*model.Website, error) {
	w, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	w.Domain = req.Domain
	w.Dir = req.Dir
	if req.Runtime != "" {
		w.Runtime = req.Runtime
	}
	w.PHPVersion = req.PHPVersion
	w.SSL = req.SSL
	w.Status = orStatus(req.Status)
	if err := s.repo.Update(w); err != nil {
		return nil, err
	}
	return w, nil
}

// Delete deletes a website.
func (s *WebsiteService) Delete(id uint) error { return s.repo.Delete(id) }

// List returns all websites.
func (s *WebsiteService) List() ([]model.Website, error) { return s.repo.List() }

// orDefault returns the non-empty value or the default value.
func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// orStatus returns a valid status (0 or 1).
func orStatus(v int) int {
	if v == 0 {
		return 0
	}
	return 1
}

// ErrNotFound is the generic "not found" error.
var ErrNotFound = errors.New("not found")
