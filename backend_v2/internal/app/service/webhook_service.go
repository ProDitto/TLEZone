package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
	"tle_zone_v2/internal/common"
	"tle_zone_v2/internal/domain/model"
	"tle_zone_v2/internal/domain/repository"

	"github.com/google/uuid"
)

type WebhookService struct {
	submissionRepo repository.SubmissionRepository
	problemRepo    repository.ProblemRepository
	jobRepo        repository.ExecutionJobRepository
	db             *sql.DB
}

func NewWebhookService(
	subRepo repository.SubmissionRepository,
	probRepo repository.ProblemRepository,
	jobRepo repository.ExecutionJobRepository,
	db *sql.DB,
) *WebhookService {
	return &WebhookService{
		submissionRepo: subRepo,
		problemRepo:    probRepo,
		jobRepo:        jobRepo,
		db:             db,
	}
}

// This is the payload expected from the external execution service
type ExecutionResultPayload struct {
	JobID             string                    `json:"job_id"` // The ID of our ExecutionJob
	OverallStatus     model.SubmissionStatus    `json:"overall_status"`
	ExecutionTimeMs   *int                      `json:"execution_time_ms,omitempty"` // Overall if applicable
	MemoryKb          *int                      `json:"memory_kb,omitempty"`         // Overall if applicable
	CompilationOutput *string                   `json:"compilation_output,omitempty"`
	ErrorOutput       *string                   `json:"error_output,omitempty"`      // For runtime errors
	TestCaseResults   []TestCaseExecutionResult `json:"test_case_results,omitempty"` // Results for each test case
}

type TestCaseExecutionResult struct {
	Input           string                 `json:"input"`                  // To match against our test cases if IDs aren't passed back
	TestCaseID      *string                `json:"test_case_id,omitempty"` // Ideal if executor can echo this back
	ActualOutput    *string                `json:"actual_output,omitempty"`
	Status          model.SubmissionStatus `json:"status"`
	ExecutionTimeMs *int                   `json:"execution_time_ms,omitempty"`
	MemoryKb        *int                   `json:"memory_kb,omitempty"`
}

func (s *WebhookService) HandleExecutionResult(ctx context.Context, payload ExecutionResultPayload) error {
	log.Printf("Webhook received for JobID: %s, OverallStatus: %s", payload.JobID, payload.OverallStatus)

	job, err := s.jobRepo.GetJobByID(ctx, payload.JobID)
	if err != nil {
		return common.Errorf("job %s not found: %w", payload.JobID, common.ErrNotFound)
	}
	if job.Status == model.JobStatusCompleted || job.Status == model.JobStatusFailed {
		log.Printf("WARN: Job %s already processed (status: %s). Ignoring webhook.", job.ID, job.Status)
		return nil // Idempotency
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return common.Errorf("failed to begin transaction for webhook: %w", err)
	}
	defer tx.Rollback()

	switch job.JobType {
	case model.JobTypeSubmissionEvaluation:
		if job.SubmissionID == nil {
			s.jobRepo.UpdateJobStatus(ctx, tx, job.ID, model.JobStatusFailed, func() *string { s := "missing submission ID for evaluation job"; return &s }())
			tx.Commit()
			return common.Errorf("submission ID missing for job type %s, job %s", job.JobType, job.ID)
		}
		err = s.processSubmissionEvaluationResult(ctx, tx, job, payload)

	case model.JobTypeProblemValidation:
		if job.ProblemID == nil {
			s.jobRepo.UpdateJobStatus(ctx, tx, job.ID, model.JobStatusFailed, func() *string { s := "missing problem ID for validation job"; return &s }())
			tx.Commit()
			return common.Errorf("problem ID missing for job type %s, job %s", job.JobType, job.ID)
		}
		err = s.processProblemValidationResult(ctx, tx, job, payload)

	case model.JobTypeRunCode:
		// For 'run_code', we might not update a DB submission record.
		// The result could be pushed to a temporary Redis key for the user to fetch,
		// or if using WebSockets, pushed directly to the client.
		// For now, just log it and mark job complete.
		log.Printf("RunCode Job %s result: Status %s. Details: %+v", job.ID, payload.OverallStatus, payload)
		// No DB updates other than the job itself
		err = nil // No specific processing error here

	default:
		s.jobRepo.UpdateJobStatus(ctx, tx, job.ID, model.JobStatusFailed, func() *string { s := "unknown job type " + job.JobType; return &s }())
		tx.Commit()
		return common.Errorf("unknown job type: %s for job %s", job.JobType, job.ID)
	}

	if err != nil {
		errMsg := err.Error()
		s.jobRepo.UpdateJobStatus(ctx, tx, job.ID, model.JobStatusFailed, &errMsg)
		tx.Commit() // Commit job status update even on processing error
		return common.Errorf("failed to process webhook for job %s: %w", job.ID, err)
	}

	// Mark job as completed
	if err := s.jobRepo.UpdateJobStatus(ctx, tx, job.ID, model.JobStatusCompleted, nil); err != nil {
		// Log this, but the main processing might have succeeded.
		log.Printf("ERROR: Failed to mark job %s as completed after processing: %v", job.ID, err)
	}

	return tx.Commit()
}

func (s *WebhookService) processSubmissionEvaluationResult(ctx context.Context, tx *sql.Tx, job *model.ExecutionJob, payload ExecutionResultPayload) error {
	submissionID := *job.SubmissionID
	submission, err := s.submissionRepo.GetSubmissionByID(ctx, submissionID) // Get full submission details
	if err != nil {
		return common.Errorf("submission %s not found for job %s: %w", submissionID, job.ID, err)
	}

	// Update main submission record
	err = s.submissionRepo.UpdateSubmissionStatus(ctx, tx, submissionID, payload.OverallStatus, payload.ExecutionTimeMs, payload.MemoryKb)
	if err != nil {
		return common.Errorf("failed to update submission %s status: %w", submissionID, err)
	}

	// Store individual test case results
	if len(payload.TestCaseResults) > 0 {
		// Fetch original test cases for the problem to link results
		problemTestCases, err := s.problemRepo.GetTestCasesByProblemID(ctx, submission.ProblemID)
		if err != nil {
			return common.Errorf("failed to fetch problem test cases for submission %s: %w", submissionID, err)
		}

		testCaseMap := make(map[string]model.TestCase)    // map by input for simple matching
		if payload.TestCaseResults[0].TestCaseID == nil { // If executor doesn't echo back IDs
			for _, ptc := range problemTestCases {
				testCaseMap[ptc.Input] = ptc
			}
		}

		var resultsToStore []model.SubmissionTestCaseResult
		for _, res := range payload.TestCaseResults {
			var originalTestCaseID string
			if res.TestCaseID != nil {
				originalTestCaseID = *res.TestCaseID
			} else {
				// Attempt to match by input (less robust)
				if matchedTC, ok := testCaseMap[res.Input]; ok {
					originalTestCaseID = matchedTC.ID
				} else {
					log.Printf("WARN: Could not match test case result input for submission %s. Input: %s", submissionID, res.Input)
					continue // Skip if cannot match
				}
			}

			resultsToStore = append(resultsToStore, model.SubmissionTestCaseResult{
				ID:              uuid.NewString(),
				SubmissionID:    submissionID,
				TestCaseID:      originalTestCaseID, // This is crucial
				ActualOutput:    res.ActualOutput,
				Status:          res.Status,
				ExecutionTimeMs: res.ExecutionTimeMs,
				MemoryKb:        res.MemoryKb,
			})
		}
		if err := s.submissionRepo.CreateSubmissionTestResults(ctx, tx, resultsToStore); err != nil {
			return common.Errorf("failed to store submission test case results for %s: %w", submissionID, err)
		}
	}

	// If accepted, update user_solved_problems
	if payload.OverallStatus == model.StatusAccepted && !submission.IsRunCodeAttempt {
		err = s.submissionRepo.MarkProblemSolved(ctx, tx, submission.UserID, submission.ProblemID, submission.ID)
		if err != nil {
			// Log this as a warning, main submission update is more critical
			log.Printf("WARN: Failed to mark problem %s solved for user %s (submission %s): %v", submission.ProblemID, submission.UserID, submission.ID, err)
		}
	}

	log.Printf("Submission %s processed with status %s.", submissionID, payload.OverallStatus)
	return nil
}

func (s *WebhookService) processProblemValidationResult(ctx context.Context, tx *sql.Tx, job *model.ExecutionJob, payload ExecutionResultPayload) error {
	problemID := *job.ProblemID
	problem, err := s.problemRepo.FindProblemByID(ctx, problemID)
	if err != nil {
		return common.Errorf("problem %s not found for validation job %s: %w", problemID, job.ID, err)
	}

	var newStatus model.ProblemStatus
	var validationErrorMsg *string

	if payload.OverallStatus == model.StatusAccepted { // Assuming problem's solution code passed all its test cases
		newStatus = model.StatusPublished
		log.Printf("Problem %s validation successful. Setting status to Published.", problemID)
	} else {
		newStatus = model.StatusRejected
		errMsg := fmt.Sprintf("Validation failed. Executor status: %s", payload.OverallStatus)
		if payload.CompilationOutput != nil && *payload.CompilationOutput != "" {
			errMsg += ". Compilation: " + *payload.CompilationOutput
		}
		if payload.ErrorOutput != nil && *payload.ErrorOutput != "" {
			errMsg += ". Runtime Error: " + *payload.ErrorOutput
		}
		validationErrorMsg = &errMsg // This could be stored on the problem or job
		log.Printf("Problem %s validation failed. Status: %s. Details: %s", problemID, payload.OverallStatus, errMsg)
	}

	problem.Status = newStatus
	// problem.ValidatedByID = &systemUserID // Or the admin who triggered validation if tracked
	problem.UpdatedAt = time.Now() // Manually update as UpdateProblem might not be called directly with all fields

	// Simplified update: just status. A full problem update might be needed.
	// For a proper implementation, you'd call problemRepo.UpdateProblem or a specific method.
	// Here, we directly update the status for brevity in this example.
	updateQuery := `UPDATE problems SET status = $1, validated_by = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3`
	// Who is validated_by? Could be the job system itself or an admin ID if available.
	// For now, let's assume job carries some admin context or use a system user ID.
	// If job context doesn't have user who initiated, might be null or a system user.
	var systemUserIDPlaceholder *string // = &config.SystemUserID if you have one
	_, err = tx.ExecContext(ctx, updateQuery, newStatus, systemUserIDPlaceholder, problemID)
	if err != nil {
		return common.Errorf("failed to update problem %s status after validation: %w", problemID, err)
	}

	// Log the detailed test case results for validation if needed.
	if validationErrorMsg != nil {
		log.Println("Validation error: ", validationErrorMsg)
	}
	// payload.TestCaseResults can be inspected.

	return nil
}
