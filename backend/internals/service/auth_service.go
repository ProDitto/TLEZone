package service

import (
	"errors"
	"tle-zone/internals/domain"
	"tle-zone/internals/model"
	"tle-zone/internals/utils"
)

type authService struct {
	userRepo domain.UserRepository
	authRepo domain.AuthRepository
}

func NewAuthService(ar domain.AuthRepository) *authService {
	return &authService{authRepo: ar}
}

func (as authService) Authenticate(username, password string) (
	*model.User, error,
) {
	hashed := utils.HashPassword(password)

	user, err := as.userRepo.FindByCreds(username, hashed)
	if err != nil {
		return nil, errors.New("invalid credentials")
	}

	// token, err := as.authRepo.

	return user, nil
}
