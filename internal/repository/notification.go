package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// NotificationRepo provides data access for notifications.
type NotificationRepo struct {
	db *gorm.DB
}

// NewNotificationRepo returns a new NotificationRepo.
func NewNotificationRepo(db *gorm.DB) *NotificationRepo {
	return &NotificationRepo{db: db}
}

// Create creates a notification.
func (r *NotificationRepo) Create(n *model.Notification) error {
	return r.db.Create(n).Error
}

// List returns the latest limit notifications (in reverse chronological order).
func (r *NotificationRepo) List(limit int) ([]model.Notification, error) {
	if limit <= 0 {
		limit = 200
	}
	out := make([]model.Notification, 0)
	err := r.db.Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// Count counts the total notifications.
func (r *NotificationRepo) Count() (int64, error) {
	var n int64
	err := r.db.Model(&model.Notification{}).Count(&n).Error
	return n, err
}

// UnreadCount counts unread notifications.
func (r *NotificationRepo) UnreadCount() (int64, error) {
	var n int64
	err := r.db.Model(&model.Notification{}).Where("read = ?", false).Count(&n).Error
	return n, err
}

// MarkRead marks a single notification as read.
func (r *NotificationRepo) MarkRead(id uint) error {
	return r.db.Model(&model.Notification{}).Where("id = ?", id).Update("read", true).Error
}

// MarkAllRead marks all notifications as read.
func (r *NotificationRepo) MarkAllRead() error {
	return r.db.Model(&model.Notification{}).Where("read = ?", false).Update("read", true).Error
}

// Delete deletes a single notification.
func (r *NotificationRepo) Delete(id uint) error {
	return r.db.Delete(&model.Notification{}, id).Error
}

// ClearAll clears all notifications.
func (r *NotificationRepo) ClearAll() error {
	return r.db.Where("1 = 1").Delete(&model.Notification{}).Error
}
