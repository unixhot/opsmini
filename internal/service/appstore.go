// Package service provides the business logic layer.
package service

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// AppInfo is a software store app (with install status, without scripts, for list display).
type AppInfo struct {
	ID            uint   `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Category      string `json:"category"`
	Icon          string `json:"icon"`
	Color         string `json:"color"`
	Version       string `json:"version"`
	Ports         string `json:"ports"`
	ContainerName string `json:"container_name"`
	Installed     bool   `json:"installed"` // whether the container exists
	Running       bool   `json:"running"`   // whether the container is running
}

// AppStoreService handles the software store: app list + one-click install (Docker Shell script) + uninstall.
type AppStoreService struct {
	repo    *repository.AppRepo
	docker  *DockerService // may be nil (no Docker environment)
	dataDir func() string  // data directory provider (read from settings, configurable)
}

// NewAppStoreService creates an AppStoreService.
func NewAppStoreService(repo *repository.AppRepo, docker *DockerService, dataDir func() string) *AppStoreService {
	return &AppStoreService{repo: repo, docker: docker, dataDir: dataDir}
}

// List returns the app list (with install/running status).
func (s *AppStoreService) List() ([]AppInfo, error) {
	apps, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	// container status: name -> whether running
	state := map[string]string{}
	if s.docker != nil {
		if cs, err := s.docker.Containers(); err == nil {
			for _, c := range cs {
				state[c.Name] = c.State
			}
		}
	}
	out := make([]AppInfo, 0, len(apps))
	for _, a := range apps {
		st, exists := state[a.ContainerName]
		out = append(out, AppInfo{
			ID:            a.ID,
			Slug:          a.Slug,
			Name:          a.Name,
			Description:   a.Description,
			Category:      a.Category,
			Icon:          a.Icon,
			Color:         a.Color,
			Version:       a.Version,
			Ports:         a.Ports,
			ContainerName: a.ContainerName,
			Installed:     exists,
			Running:       exists && st == "running",
		})
	}
	return out, nil
}

// Get returns a single app's full info (including install script, for the config page editor).
func (s *AppStoreService) Get(slug string) (*model.App, error) {
	return s.repo.FindBySlug(slug)
}

// Update updates an app (config page script editing, etc.).
func (s *AppStoreService) Update(app *model.App) error {
	return s.repo.Upsert(app)
}

// Install executes the app's install Shell script (docker run one-click startup).
// It returns the script output; returns an error when Docker is unavailable or the script fails.
func (s *AppStoreService) Install(slug string) (string, error) {
	if s.docker == nil {
		return "", errors.New("docker not available")
	}
	app, err := s.repo.FindBySlug(slug)
	if err != nil {
		return "", err
	}
	// confirm the daemon is available first, to avoid discovering a connection failure only after running the script.
	if _, err := s.docker.Containers(); err != nil {
		return "", fmt.Errorf("docker daemon unavailable: %w", err)
	}
	cmd := exec.Command("bash", "-c", app.InstallScript)
	dir := "/var/lib/opsmini/data"
	if s.dataDir != nil {
		if d := s.dataDir(); d != "" {
			dir = d
		}
	}
	cmd.Env = append(os.Environ(), "DATA_DIR="+dir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("install failed: %w", err)
	}
	return out.String(), nil
}

// Uninstall uninstalls an app: removes the corresponding container (does not delete data volumes, preserving user data).
func (s *AppStoreService) Uninstall(slug string) error {
	if s.docker == nil {
		return errors.New("docker not available")
	}
	app, err := s.repo.FindBySlug(slug)
	if err != nil {
		return err
	}
	return s.docker.RemoveContainer(app.ContainerName)
}

// Start starts an app container.
func (s *AppStoreService) Start(slug string) error {
	app, err := s.dockerName(slug)
	if err != nil {
		return err
	}
	return s.docker.StartContainer(app)
}

// Stop stops an app container.
func (s *AppStoreService) Stop(slug string) error {
	app, err := s.dockerName(slug)
	if err != nil {
		return err
	}
	return s.docker.StopContainer(app)
}

// Restart restarts an app container.
func (s *AppStoreService) Restart(slug string) error {
	app, err := s.dockerName(slug)
	if err != nil {
		return err
	}
	return s.docker.RestartContainer(app)
}

// dockerName verifies Docker availability and returns the app's container name.
func (s *AppStoreService) dockerName(slug string) (string, error) {
	if s.docker == nil {
		return "", errors.New("docker not available")
	}
	app, err := s.repo.FindBySlug(slug)
	if err != nil {
		return "", err
	}
	return app.ContainerName, nil
}

// Create creates an app template.
func (s *AppStoreService) Create(app *model.App) error {
	return s.repo.Upsert(app)
}

// Delete deletes an app template (only removes the template definition, not the installed container).
func (s *AppStoreService) Delete(slug string) error {
	return s.repo.DeleteBySlug(slug)
}
