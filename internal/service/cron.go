package service

import (
	"context"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// CronService handles scheduled task CRUD and scheduling.
// It only schedules "panel tasks" (kind=panel); system/user-sourced tasks are
// managed by the system cron and shown read-only by the panel.
type CronService struct {
	repo *repository.CronJobRepo
	cron *cron.Cron

	mu      sync.Mutex
	entries map[uint]cron.EntryID // jobID -> entryID
}

// NewCronService creates a CronService: loads enabled tasks and starts the scheduler.
func NewCronService(repo *repository.CronJobRepo) *CronService {
	s := &CronService{
		repo:    repo,
		cron:    cron.New(),
		entries: make(map[uint]cron.EntryID),
	}
	s.reload()
	s.cron.Start()
	return s
}

// Stop stops the scheduler.
func (s *CronService) Stop() { s.cron.Stop() }

// reload loads all enabled tasks.
func (s *CronService) reload() {
	jobs, err := s.repo.ListEnabled()
	if err != nil {
		log.Printf("cron reload: %v", err)
		return
	}
	for _, j := range jobs {
		s.register(j)
	}
}

// register adds a task to the scheduler (ignores system/user sources and invalid expressions).
func (s *CronService) register(j model.CronJob) {
	if j.Kind != model.CronKindPanel || !j.Enabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.entries[j.ID]; exists {
		return
	}
	id, err := s.cron.AddFunc(j.Schedule, func() { s.execute(j) })
	if err != nil {
		log.Printf("cron add %q: %v", j.Name, err)
		return
	}
	s.entries[j.ID] = id
}

// unregister removes a task from the scheduler.
func (s *CronService) unregister(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if eid, ok := s.entries[id]; ok {
		s.cron.Remove(eid)
		delete(s.entries, id)
	}
}

// execute runs a task and updates its last run time.
func (s *CronService) execute(j model.CronJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	start := time.Now()
	out, err := exec.CommandContext(ctx, "sh", "-c", j.Command).CombinedOutput()
	_ = out
	if err != nil {
		log.Printf("cron %q (id=%d) failed after %s: %v", j.Name, j.ID, time.Since(start), err)
	} else {
		log.Printf("cron %q (id=%d) done in %s", j.Name, j.ID, time.Since(start))
	}

	now := time.Now()
	if cur, e := s.repo.FindByID(j.ID); e == nil {
		cur.LastRun = &now
		_ = s.repo.Update(cur)
	}
}

// CronReq is the input for creating/updating a task.
type CronReq struct {
	Name     string `json:"name" binding:"required"`
	Kind     string `json:"kind"`
	User     string `json:"user"`
	Schedule string `json:"schedule" binding:"required"`
	Command  string `json:"command" binding:"required"`
	Enabled  *bool  `json:"enabled"`
}

// Create creates a task.
func (s *CronService) Create(req CronReq) (*model.CronJob, error) {
	j := &model.CronJob{
		Name:     req.Name,
		Kind:     orDefault(req.Kind, model.CronKindPanel),
		User:     req.User,
		Schedule: req.Schedule,
		Command:  req.Command,
		Enabled:  req.Enabled == nil || *req.Enabled,
	}
	if err := s.repo.Create(j); err != nil {
		return nil, err
	}
	if j.Enabled {
		s.register(*j)
	}
	return j, nil
}

// Update updates a task and syncs the scheduler.
func (s *CronService) Update(id uint, req CronReq) (*model.CronJob, error) {
	j, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	j.Name = req.Name
	if req.Kind != "" {
		j.Kind = req.Kind
	}
	j.User = req.User
	j.Schedule = req.Schedule
	j.Command = req.Command
	if req.Enabled != nil {
		j.Enabled = *req.Enabled
	}
	if err := s.repo.Update(j); err != nil {
		return nil, err
	}
	// sync scheduler: remove the old entry first, then re-register as needed
	s.unregister(id)
	if j.Enabled {
		s.register(*j)
	}
	return j, nil
}

// Delete deletes a task and removes it from the scheduler.
func (s *CronService) Delete(id uint) error {
	s.unregister(id)
	return s.repo.Delete(id)
}

// List returns all tasks (including system/user sources, merged by SystemCronJobs).
func (s *CronService) List() ([]model.CronJob, error) { return s.repo.List() }
