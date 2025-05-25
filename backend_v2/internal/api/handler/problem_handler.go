package handler

// ... imports ...
import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"tle_zone_v2/internal/api/middleware"
	"tle_zone_v2/internal/app/service"
	"tle_zone_v2/internal/common"
	"tle_zone_v2/internal/domain/model"

	"github.com/go-chi/chi/v5"
)

type ProblemHandler struct {
	problemService *service.ProblemService
}

func NewProblemHandler(ps *service.ProblemService) *ProblemHandler {
	return &ProblemHandler{problemService: ps}
}

func (h *ProblemHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.listProblems)            // GET /api/v1/problems
	r.Get("/{problemSlug}", h.getProblem) // GET /api/v1/problems/two-sum

	r.Group(func(adminRouter chi.Router) {
		adminRouter.Use(middleware.Authenticator)
		adminRouter.Use(middleware.AdminOnly)
		adminRouter.Post("/", h.createProblem) // POST /api/v1/problems
		// adminRouter.Put("/{problemID}", h.updateProblem) // PUT /api/v1/problems/{id}
		// Add routes for managing examples, test cases, etc. for a problem by ID
	})
}

func (h *ProblemHandler) createProblem(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		common.RespondWithError(w, http.StatusUnauthorized, "Missing user context")
		return
	}

	var req service.CreateProblemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.RespondWithError(w, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	problem, err := h.problemService.CreateProblem(r.Context(), userID, req)
	if err != nil {
		common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
		return
	}
	common.RespondWithJSON(w, http.StatusCreated, problem)
}

func (h *ProblemHandler) listProblems(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("pageSize")
	difficultyStr := r.URL.Query().Get("difficulty")
	tagsStr := r.URL.Query().Get("tags") // comma-separated list of tag slugs/names

	page, _ := strconv.Atoi(pageStr)
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	var tagsFilter []string
	if tagsStr != "" {
		tagsFilter = strings.Split(tagsStr, ",")
	}

	userRole, _ := middleware.GetUserRoleFromContext(r.Context()) // Role might be empty if not authenticated for this public route

	problems, total, err := h.problemService.ListProblems(r.Context(), page, pageSize, model.ProblemDifficulty(difficultyStr), tagsFilter, userRole)
	if err != nil {
		common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
		return
	}

	// Paginated response structure
	type PaginatedProblemsResponse struct {
		Problems []model.Problem `json:"problems"`
		Total    int             `json:"total"`
		Page     int             `json:"page"`
		PageSize int             `json:"page_size"`
	}
	common.RespondWithJSON(w, http.StatusOK, PaginatedProblemsResponse{
		Problems: problems,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

func (h *ProblemHandler) getProblem(w http.ResponseWriter, r *http.Request) {
	problemSlug := chi.URLParam(r, "problemSlug")
	userRole, _ := middleware.GetUserRoleFromContext(r.Context())

	problem, err := h.problemService.GetProblemDetails(r.Context(), problemSlug, userRole)
	if err != nil {
		common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
		return
	}
	common.RespondWithJSON(w, http.StatusOK, problem)
}

// updateProblem handler
// ...
