package service

import (
	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// DatabaseService handles database management.
type DatabaseService struct {
	repo *repository.DatabaseRepo
}

// NewDatabaseService creates a DatabaseService.
func NewDatabaseService(repo *repository.DatabaseRepo) *DatabaseService {
	return &DatabaseService{repo: repo}
}

// DatabaseReq is the input for creating/updating a database.
type DatabaseReq struct {
	Name    string `json:"name" binding:"required"`
	Type    string `json:"type"`
	Charset string `json:"charset"`
	Status  int    `json:"status"`
}

// Create creates a database instance.
func (s *DatabaseService) Create(req DatabaseReq) (*model.Database, error) {
	d := &model.Database{
		Name:    req.Name,
		Type:    orDefault(req.Type, "mysql"),
		Charset: orDefault(req.Charset, "utf8mb4"),
		Status:  orStatus(req.Status),
	}
	if err := s.repo.Create(d); err != nil {
		return nil, err
	}
	return d, nil
}

// Update updates a database instance.
func (s *DatabaseService) Update(id uint, req DatabaseReq) (*model.Database, error) {
	d, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	d.Name = req.Name
	if req.Type != "" {
		d.Type = req.Type
	}
	if req.Charset != "" {
		d.Charset = req.Charset
	}
	d.Status = orStatus(req.Status)
	if err := s.repo.Update(d); err != nil {
		return nil, err
	}
	return d, nil
}

// Delete deletes a database instance.
func (s *DatabaseService) Delete(id uint) error { return s.repo.Delete(id) }

// List returns all database instances.
func (s *DatabaseService) List() ([]model.Database, error) { return s.repo.List() }
