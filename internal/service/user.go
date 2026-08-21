package service

import (
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// UserService handles user management.
type UserService struct {
	repo *repository.UserRepo
}

// NewUserService creates a UserService.
func NewUserService(repo *repository.UserRepo) *UserService {
	return &UserService{repo: repo}
}

// CreateUserReq is the input for creating a user.
type CreateUserReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role"`
}

// Create creates a user.
func (s *UserService) Create(req CreateUserReq) (*model.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	role := req.Role
	if role == "" {
		role = model.RoleOperator
	}
	u := &model.User{
		Username:     req.Username,
		PasswordHash: string(hash),
		Role:         role,
		Status:       1,
	}
	if err := s.repo.Create(u); err != nil {
		return nil, err
	}
	return u, nil
}

// UpdateUserReq is the input for updating a user (fields are optional; only the provided fields are updated).
type UpdateUserReq struct {
	Role     string `json:"role"`
	Password string `json:"password"` // non-empty updates the password
	Status   *int   `json:"status"`   // 1 enabled, 0 disabled
}

// Update updates a user (role/status/password).
func (s *UserService) Update(id uint, req UpdateUserReq) (*model.User, error) {
	u, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if req.Role != "" {
		u.Role = req.Role
	}
	if req.Status != nil {
		u.Status = *req.Status
	}
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		u.PasswordHash = string(hash)
	}
	if err := s.repo.Update(u); err != nil {
		return nil, err
	}
	return u, nil
}

// List returns all users.
func (s *UserService) List() ([]model.User, error) {
	return s.repo.List()
}

// Get returns a single user by id.
func (s *UserService) Get(id uint) (*model.User, error) {
	return s.repo.FindByID(id)
}

// Delete deletes a user.
func (s *UserService) Delete(id uint) error {
	return s.repo.Delete(id)
}

// ProfileReq is the input for updating the current user's own profile.
type ProfileReq struct {
	Nickname string `json:"nickname"`
	Icon     string `json:"icon"`
	Email    string `json:"email"`
}

// UpdateProfile updates the current user's nickname/icon/email.
func (s *UserService) UpdateProfile(id uint, req ProfileReq) (*model.User, error) {
	u, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	u.Nickname = req.Nickname
	u.Icon = req.Icon
	u.Email = req.Email
	if err := s.repo.Update(u); err != nil {
		return nil, err
	}
	return u, nil
}

// ChangePasswordReq is the input for changing one's own password.
type ChangePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// ChangePassword verifies the old password and sets a new one.
func (s *UserService) ChangePassword(id uint, req ChangePasswordReq) error {
	u, err := s.repo.FindByID(id)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
		return errors.New("current password is incorrect")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.PasswordHash = string(hash)
	return s.repo.Update(u)
}
