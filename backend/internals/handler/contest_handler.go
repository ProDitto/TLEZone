package handler

import (
	"net/http"
	"tle-zone/internals/domain"
)

type contestHandler struct {
	contestService domain.ContestService
}

func NewContestHandler(cs domain.ContestService) domain.ContestHandler {
	return &contestHandler{contestService: cs}
}

func (ch *contestHandler) CreateContest(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (ch *contestHandler) JoinContest(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (ch *contestHandler) GetContestStatus(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (ch *contestHandler) ListContests(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
