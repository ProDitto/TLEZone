package handler

import (
	"net/http"
	"tle_zone_v2/internal/api/middleware"
	"tle_zone_v2/internal/app/service"
	"tle_zone_v2/internal/common"

	"github.com/go-chi/chi/v5"
)

// ... imports ...
type SubmissionHandler struct {
	submissionService *service.SubmissionService
}

func NewSubmissionHandler(ss *service.SubmissionService) *SubmissionHandler {
	return &SubmissionHandler{submissionService: ss}
}
func (h *SubmissionHandler) RegisterRoutes(r chi.Router) {
	r.Use(middleware.Authenticator) // All submission routes require auth
	r.Post("/", h.createSubmission)
	r.Post("/run", h.runCode)
	// GET /me (my submissions)
	// GET /{submissionID}
	// GET /problem/{problemID}/history (user's history for a problem)
	// GET /{submissionID1}/diff/{submissionID2}
}

func (h *SubmissionHandler) createSubmission(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok { /* ... error ... */
		return
	}

	var req service.CreateSubmissionRequest
	// ... decode ... error ...

	submission, _ := h.submissionService.CreateSubmission(r.Context(), userID, req)
	// ... error handling ...
	common.RespondWithJSON(w, http.StatusAccepted, submission) // Accepted (202) as it's async
}

func (h *SubmissionHandler) runCode(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok { /* ... error ... */
		return
	}

	var req service.RunCodeRequest
	// ... decode ... error ...

	jobID, _ := h.submissionService.RunCode(r.Context(), userID, req)
	// ... error handling ...
	common.RespondWithJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}

// ... other submission handlers ...
