package handler

import (
	"net/http"
	"tle-zone/internals/domain"
)

type adminHandler struct {
	adminService domain.AdminService
}

func NewAdminHandler(as domain.AdminService) domain.AdminHandler {
	return &adminHandler{adminService: as}
}

func (ah *adminHandler) CreateProblem(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
func (ah *adminHandler) UpdateProblem(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
func (ah *adminHandler) InvalidateProblem(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
func (ah *adminHandler) BanUser(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
