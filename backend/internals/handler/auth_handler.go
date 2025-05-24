package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"tle-zone/internals/domain"
	"tle-zone/internals/model"
)

type authHandler struct {
	authService domain.AuthService
	userService domain.UserService
}

func NewAuthHandler(as domain.AuthService, us domain.UserService) domain.AuthHandler {
	return &authHandler{authService: as, userService: us}
}

func (ah *authHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.UserLogin

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user, err := ah.authService.Authenticate(req.Username, req.Password)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusBadRequest)
		return
	}

	// TODO: Create Token and set it to http cookies
	// token, err :=

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(user)
}

func (ah *authHandler) Signup(w http.ResponseWriter, r *http.Request) {
	var req model.UserRegistration

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	req.Password = strings.TrimSpace(req.Password)
	req.ConfirmPassword = strings.TrimSpace(req.ConfirmPassword)
	req.Username = strings.TrimSpace(req.Username)

	if len(req.Username) < 8 || len(req.Email) < 6 || len(req.Password) < 8 || req.ConfirmPassword != req.Password {
		http.Error(w, "password and confirm password does not match", http.StatusBadRequest)
		return
	}

	err := ah.userService.Register(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (ah *authHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
