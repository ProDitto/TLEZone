package handler

import (
	"encoding/json"
	"net/http"
	"tle_zone_v2/internal/app/service"
	"tle_zone_v2/internal/common"

	"github.com/go-chi/chi/v5"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Post("/signup", h.signup)
	r.Post("/login", h.login)
}

func (h *AuthHandler) signup(w http.ResponseWriter, r *http.Request) {
	var req service.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.RespondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}

	resp, err := h.authService.Signup(r.Context(), req)
	if err != nil {
		common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
		return
	}
	common.RespondWithJSON(w, http.StatusCreated, resp)
}

func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.RespondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}
	resp, err := h.authService.Login(r.Context(), req)
	if err != nil {
		common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
		return
	}
	common.RespondWithJSON(w, http.StatusOK, resp)
}
