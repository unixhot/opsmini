package service

import (
	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// NotificationService handles notifications for handlers and background detectors.
type NotificationService struct {
	repo *repository.NotificationRepo
}

// NewNotificationService creates a NotificationService.
func NewNotificationService(repo *repository.NotificationRepo) *NotificationService {
	return &NotificationService{repo: repo}
}

// Notify creates a notification (used by alert/security modules).
func (s *NotificationService) Notify(level, title, content string) {
	_ = s.repo.Create(&model.Notification{
		Level:   level,
		Title:   title,
		Content: content,
	})
}

// List returns the most recent limit notifications.
func (s *NotificationService) List(limit int) ([]model.Notification, error) {
	return s.repo.List(limit)
}

// UnreadCount returns the number of unread notifications.
func (s *NotificationService) UnreadCount() (int64, error) {
	return s.repo.UnreadCount()
}

// MarkRead marks a single notification as read.
func (s *NotificationService) MarkRead(id uint) error {
	return s.repo.MarkRead(id)
}

// MarkAllRead marks all notifications as read.
func (s *NotificationService) MarkAllRead() error {
	return s.repo.MarkAllRead()
}

// Delete deletes a single notification.
func (s *NotificationService) Delete(id uint) error {
	return s.repo.Delete(id)
}

// ClearAll clears all notifications.
func (s *NotificationService) ClearAll() error {
	return s.repo.ClearAll()
}
