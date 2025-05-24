package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"tle-zone/internals/domain"
	"tle-zone/internals/model"

	"github.com/go-chi/chi/v5"
)

type submissionHandler struct {
	submissionService domain.SubmissionService
}

func NewSubmissionHandler(ss domain.SubmissionService) domain.SubmissionHandler {
	return &submissionHandler{submissionService: ss}
}

func (sh *submissionHandler) SubmitCode(w http.ResponseWriter, r *http.Request) {
	userId, ok := r.Context().Value("userID").(int)
	if !ok {
		http.Error(w, "error fetching userID from context", http.StatusInternalServerError)
		return
	}

	var req model.Submission
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req.UserID = userId

	submissionId, err := sh.submissionService.CreateSubmission(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(submissionId)
}

func (sh *submissionHandler) GetSubmissionResult(w http.ResponseWriter, r *http.Request) {
	userId, ok := r.Context().Value("userID").(int)
	if !ok {
		http.Error(w, "error fetching userID from context", http.StatusInternalServerError)
		return
	}

	submissionIDStr := chi.URLParam(r, "submissionID")
	submissionID, err := strconv.Atoi(submissionIDStr)
	if err != nil || submissionID <= 0 {
		http.Error(w, "invalid submission ID", http.StatusBadRequest)
		return
	}

	submission, err := sh.submissionService.GetSubmissionByID(submissionID, userId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(submission)
}
