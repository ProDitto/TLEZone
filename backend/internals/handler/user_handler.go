package handler

import (
	"encoding/json"
	"net/http"
	"tle-zone/internals/domain"
	"tle-zone/internals/model"
)

type userHandler struct {
	userService       domain.UserService
	submissionService domain.SubmissionService
}

func NewUserHandler(us domain.UserService) domain.UserHandler {
	return &userHandler{userService: us}
}

func (uh *userHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	userId, ok := r.Context().Value("userID").(int)
	if !ok {
		http.Error(w, "error fetching userID from context", http.StatusInternalServerError)
		return
	}

	user, err := uh.userService.GetUserByID(userId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(user)
}

func (uh *userHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userId, ok := r.Context().Value("userID").(int)
	if !ok {
		http.Error(w, "error fetching userID from context", http.StatusInternalServerError)
		return
	}

	var req model.UserUpdate

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := uh.userService.UpdateProfile(userId, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (uh *userHandler) ListUserSubmissions(w http.ResponseWriter, r *http.Request) {
	userId, ok := r.Context().Value("userID").(int)
	if !ok {
		http.Error(w, "error fetching userID from context", http.StatusInternalServerError)
		return
	}

	submissions, err := uh.submissionService.GetUserSubmissions(userId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(submissions)
}
