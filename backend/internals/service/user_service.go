package service

import (
	"tle-zone/internals/domain"
	"tle-zone/internals/model"
)

type userService struct {
	userRepo domain.UserRepository
}

func NewUserService(ur domain.UserRepository) domain.UserService {
	return &userService{userRepo: ur}
}

func (us *userService) Register(user *model.UserRegistration) error {
	return us.userRepo.Create(&model.UserDB{
		Username: user.Username,
		Email:    user.Email,
		Role:     "user",
		Password: user.Password,
	})
}

func (us *userService) GetUserByID(id int) (*model.User, error) {
	return us.userRepo.FindByID(id)
}

func (us *userService) UpdateProfile(userID int, update *model.UserUpdate) error {
	return us.userRepo.Update(&model.User{
		ID:       userID,
		Username: update.Username,
		Email:    update.Email,
	})
}
