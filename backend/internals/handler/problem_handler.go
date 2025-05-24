package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"tle-zone/internals/domain"
	"tle-zone/internals/model"

	"github.com/go-chi/chi/v5"
)

type problemHandler struct {
	problemService domain.ProblemService
}

func NewProblemHandler(ps domain.ProblemService) domain.ProblemHandler {
	return &problemHandler{problemService: ps}
}

func (ph *problemHandler) ListProblems(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	filter := model.ProblemFilter{
		DifficultyIDs: parseCommaSeparatedInts(query.Get("difficulty")),
		TagIDs:        parseCommaSeparatedInts(query.Get("tags")),
		Page:          parsePositiveInt(query.Get("page"), 1),
		PageSize:      parsePositiveInt(query.Get("pageSize"), 20),
	}

	if filter.PageSize > 20 {
		filter.PageSize = 20
	}

	problems, err := ph.problemService.ListProblems(&filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(problems)
}

func (ph *problemHandler) GetProblemByID(w http.ResponseWriter, r *http.Request) {
	problemIDStr := chi.URLParam(r, "problemID")
	problemID, err := strconv.Atoi(problemIDStr)
	if err != nil || problemID <= 0 {
		http.Error(w, "invalid problem ID", http.StatusBadRequest)
		return
	}

	problem, err := ph.problemService.GetProblemByID(problemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(problem)
}
