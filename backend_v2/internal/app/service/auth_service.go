package service

import (
	"context"
	"errors"
	"fmt"
	"tle_zone_v2/internal/common"
	"tle_zone_v2/internal/common/security"
	"tle_zone_v2/internal/domain/model"
	"tle_zone_v2/internal/domain/repository"

	"github.com/google/uuid"
)

type AuthService struct {
	userRepo repository.UserRepository
}

func NewAuthService(userRepo repository.UserRepository) *AuthService {
	return &AuthService{userRepo: userRepo}
}

type SignupRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	LoginField string `json:"login_field"` // Can be username or email
	Password   string `json:"password"`
}

type AuthResponse struct {
	User  *model.User `json:"user"`
	Token string      `json:"token"`
}

func (s *AuthService) Signup(ctx context.Context, req SignupRequest) (*AuthResponse, error) {
	if req.Username == "" || req.Email == "" || req.Password == "" {
		return nil, common.ErrBadRequest
	}
	// Add more validation (email format, password strength etc.)

	hashedPassword, err := security.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &model.User{
		ID:             uuid.NewString(),
		Username:       req.Username,
		Email:          req.Email,
		HashedPassword: hashedPassword,
		Role:           model.RoleUser, // Default role
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		// Repo might return common.ErrConflict
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	token, err := security.GenerateToken(user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	user.HashedPassword = "" // Clear password before returning
	return &AuthResponse{User: user, Token: token}, nil
}

func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	if req.LoginField == "" || req.Password == "" {
		return nil, common.ErrBadRequest
	}

	var user *model.User
	var err error

	// Try finding by email first, then by username
	user, err = s.userRepo.FindByEmail(ctx, req.LoginField)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			user, err = s.userRepo.FindByUsername(ctx, req.LoginField)
		}
	}

	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil, common.ErrUnauthorized // Generic message for security
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	if !security.CheckPasswordHash(req.Password, user.HashedPassword) {
		return nil, common.ErrUnauthorized
	}

	token, err := security.GenerateToken(user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	user.HashedPassword = ""
	return &AuthResponse{User: user, Token: token}, nil
}
