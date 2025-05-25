package service

import (
	"context"
	"database/sql"
	"log"
	"tle_zone_v2/internal/common"
	"tle_zone_v2/internal/domain/model"
	"tle_zone_v2/internal/domain/repository"
	"tle_zone_v2/internal/platform/config"

	"github.com/google/uuid"
)

type SubmissionService struct {
	submissionRepo repository.SubmissionRepository
	problemRepo    repository.ProblemRepository
	execJobService *ExecutionJobService
	db             *sql.DB // For transactions
}

func NewSubmissionService(
	subRepo repository.SubmissionRepository,
	probRepo repository.ProblemRepository,
	execJobService *ExecutionJobService,
	db *sql.DB,
) *SubmissionService {
	return &SubmissionService{
		submissionRepo: subRepo,
		problemRepo:    probRepo,
		execJobService: execJobService,
		db:             db,
	}
}

type CreateSubmissionRequest struct {
	ProblemID  string `json:"problem_id"`
	LanguageID string `json:"language_id"` // Or slug
	Code       string `json:"code"`
}

func (s *SubmissionService) CreateSubmission(ctx context.Context, userID string, req CreateSubmissionRequest) (*model.Submission, error) {
	// Validate problem exists and is published
	problem, err := s.problemRepo.FindProblemByID(ctx, req.ProblemID) // Use slug if URLs are slug-based
	if err != nil {
		return nil, common.Errorf("problem not found: %w", err)
	}
	if problem.Status != model.StatusPublished {
		return nil, common.Errorf("problem is not published: %w", common.ErrForbidden)
	}

	// Validate language exists and is active
	language, err := s.problemRepo.GetLanguageByID(ctx, req.LanguageID) // Or by slug
	if err != nil || !language.IsActive {
		return nil, common.Errorf("language not found or inactive: %w", common.ErrBadRequest)
	}

	submission := &model.Submission{
		ID:               uuid.NewString(),
		UserID:           userID,
		ProblemID:        problem.ID,
		LanguageID:       language.ID,
		Code:             req.Code,
		Status:           model.StatusPending, // Will be updated to InQueue by job service
		IsRunCodeAttempt: false,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, common.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := s.submissionRepo.CreateSubmission(ctx, tx, submission); err != nil {
		return nil, common.Errorf("failed to create submission: %w", err)
	}

	// Enqueue job for execution
	job, err := s.execJobService.EnqueueSubmissionEvaluationJob(ctx, tx, submission.ID)
	if err != nil {
		return nil, common.Errorf("failed to enqueue submission job: %w", err)
	}

	// Update submission status to InQueue after job is created
	submission.Status = model.StatusInQueue // Reflecting job status
	// This status update might be better done by the job service after successful Redis push.
	// For now, assume job creation means it's effectively in queue.
	// Alternatively, don't update status here, let the worker update it to Processing.

	if err := tx.Commit(); err != nil {
		return nil, common.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Submission %s created and job %s enqueued.", submission.ID, job.ID)
	return submission, nil
}

type RunCodeRequest struct {
	ProblemID      *string  `json:"problem_id,omitempty"` // Optional: if running against problem examples
	LanguageID     string   `json:"language_id"`          // Or slug
	Code           string   `json:"code"`
	CustomInputs   []string `json:"custom_inputs"` // User provided inputs
	RuntimeLimitMs *int     `json:"runtime_limit_ms,omitempty"`
	MemoryLimitKb  *int     `json:"memory_limit_kb,omitempty"`
}

// RunCode is different: it doesn't save a persistent submission.
// It creates a temporary execution job. The result is returned via webhook but not stored long-term against the user's submission history.
// The worker needs to know this is a 'run_code' job type.
func (s *SubmissionService) RunCode(ctx context.Context, userID string, req RunCodeRequest) (string, error) { // Returns jobID
	// Validate language
	language, err := s.problemRepo.GetLanguageByID(ctx, req.LanguageID) // Or by slug
	if err != nil || !language.IsActive {
		return "", common.Errorf("language not found or inactive: %w", common.ErrBadRequest)
	}

	var problem *model.Problem
	var inputsToRun []string
	runtimeLimit := config.AppConfig.ProblemValidationDefaultRuntimeLimitMs // Default
	memoryLimit := config.AppConfig.ProblemValidationDefaultMemoryLimitKb   // Default

	if req.ProblemID != nil && *req.ProblemID != "" {
		p, err := s.problemRepo.FindProblemByID(ctx, *req.ProblemID)
		if err != nil {
			return "", common.Errorf("problem not found for run code: %w", err)
		}
		problem = p
		runtimeLimit = problem.RuntimeLimitMs
		memoryLimit = problem.MemoryLimitKb

		// If problemID is given, use its runnable examples as inputs if custom inputs are empty
		if len(req.CustomInputs) == 0 {
			examples, err := s.problemRepo.GetExamplesByProblemID(ctx, problem.ID)
			if err != nil {
				return "", common.Errorf("failed to get examples for problem: %w", err)
			}
			for _, ex := range examples {
				if ex.IsRunnable {
					inputsToRun = append(inputsToRun, ex.Input)
				}
			}
			if len(inputsToRun) == 0 {
				return "", common.Errorf("no runnable examples or custom inputs provided: %w", common.ErrBadRequest)
			}
		} else {
			inputsToRun = req.CustomInputs
		}
	} else {
		if len(req.CustomInputs) == 0 {
			return "", common.Errorf("no custom inputs provided for run code: %w", common.ErrBadRequest)
		}
		inputsToRun = req.CustomInputs
	}
	if req.RuntimeLimitMs != nil {
		runtimeLimit = *req.RuntimeLimitMs
	}
	if req.MemoryLimitKb != nil {
		memoryLimit = *req.MemoryLimitKb
	}

	// Enqueue a 'run_code' type job
	jobID, err := s.execJobService.EnqueueRunCodeJob(ctx, userID, language.ID, req.Code, inputsToRun, req.ProblemID, runtimeLimit, memoryLimit)
	if err != nil {
		return "", common.Errorf("failed to enqueue run code job: %w", err)
	}

	// Client will have to poll or use websockets for the result of this jobID if not using webhook for direct response.
	// For simplicity, we assume webhook will be used even for run_code.
	return jobID, nil
}

// GetSubmissionDetails, GetMySubmissions, GetSubmissionHistoryForProblem, DiffSubmissions...
