package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// CronJobRepo provides data access for cron jobs.
type CronJobRepo struct {
	db *gorm.DB
}

// NewCronJobRepo returns a new CronJobRepo.
func NewCronJobRepo(db *gorm.DB) *CronJobRepo {
	return &CronJobRepo{db: db}
}

// Create creates a job.
func (r *CronJobRepo) Create(j *model.CronJob) error { return r.db.Create(j).Error }

// Update saves a job.
func (r *CronJobRepo) Update(j *model.CronJob) error { return r.db.Save(j).Error }

// Delete deletes a job.
func (r *CronJobRepo) Delete(id uint) error { return r.db.Delete(&model.CronJob{}, id).Error }

// FindByID finds a job by ID.
func (r *CronJobRepo) FindByID(id uint) (*model.CronJob, error) {
	var j model.CronJob
	if err := r.db.First(&j, id).Error; err != nil {
		return nil, err
	}
	return &j, nil
}

// List returns all jobs.
func (r *CronJobRepo) List() ([]model.CronJob, error) {
	var list []model.CronJob
	err := r.db.Order("id").Find(&list).Error
	return list, err
}

// ListEnabled returns all enabled panel-managed jobs (for the scheduler to load).
func (r *CronJobRepo) ListEnabled() ([]model.CronJob, error) {
	var list []model.CronJob
	err := r.db.Where("enabled = ?", true).Order("id").Find(&list).Error
	return list, err
}
