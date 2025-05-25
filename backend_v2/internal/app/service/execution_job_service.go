package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"tle_zone_v2/internal/common"
	"tle_zone_v2/internal/domain/model"
	"tle_zone_v2/internal/domain/repository"
	"tle_zone_v2/internal/platform/config"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type ExecutionJobService struct {
	jobRepo        repository.ExecutionJobRepository
	submissionRepo repository.SubmissionRepository // To update submission status to InQueue
	rdb            *redis.Client
	db             *sql.DB
}

func NewExecutionJobService(jobRepo repository.ExecutionJobRepository, subRepo repository.SubmissionRepository, rdb *redis.Client, db *sql.DB) *ExecutionJobService {
	return &ExecutionJobService{jobRepo: jobRepo, submissionRepo: subRepo, rdb: rdb, db: db}
}

// EnqueueSubmissionEvaluationJob creates a job record and pushes its ID to Redis.
func (s *ExecutionJobService) EnqueueSubmissionEvaluationJob(ctx context.Context, tx *sql.Tx, submissionID string) (*model.ExecutionJob, error) {
	payloadData := model.SubmissionEvaluationPayload{SubmissionID: submissionID}
	payloadBytes, err := json.Marshal(payloadData)
	if err != nil {
		return nil, common.Errorf("failed to marshal submission eval payload: %w", err)
	}

	job := &model.ExecutionJob{
		ID:           uuid.NewString(),
		SubmissionID: &submissionID,
		JobType:      model.JobTypeSubmissionEvaluation,
		Payload:      payloadBytes,
		Status:       model.JobStatusQueued,
	}

	if err := s.jobRepo.CreateJob(ctx, tx, job); err != nil { // Use the passed transaction
		return nil, common.Errorf("failed to create execution job in DB: %w", err)
	}

	// Push job ID to Redis queue
	if err := s.rdb.LPush(ctx, config.AppConfig.ExecutionQueueName, job.ID).Err(); err != nil {
		// This is a critical failure point. If DB commit happens but Redis push fails, job is orphaned.
		// Consider distributed transaction patterns or retry mechanisms here for production.
		// For now, error out, leading to transaction rollback.
		return nil, common.Errorf("failed to push job ID to Redis queue: %w", err)
	}

	// Update submission status to InQueue
	// This should also use the transaction
	if err := s.submissionRepo.UpdateSubmissionStatus(ctx, tx, submissionID, model.StatusInQueue, nil, nil); err != nil {
		return nil, common.Errorf("failed to update submission status to InQueue: %w", err)
	}

	log.Printf("Execution job %s for submission %s enqueued successfully.", job.ID, submissionID)
	return job, nil
}

// EnqueueProblemValidationJob (similar, but JobType is ProblemValidation and payload is ProblemValidationPayload)
func (s *ExecutionJobService) EnqueueProblemValidationJob(ctx context.Context, problemID string) (*model.ExecutionJob, error) {
	payloadData := model.ProblemValidationPayload{ProblemID: problemID}
	payloadBytes, err := json.Marshal(payloadData)
	if err != nil {
		return nil, common.Errorf("failed to marshal problem validation payload: %w", err)
	}

	job := &model.ExecutionJob{
		ID:        uuid.NewString(),
		ProblemID: &problemID,
		JobType:   model.JobTypeProblemValidation,
		Payload:   payloadBytes,
		Status:    model.JobStatusQueued,
	}

	// Use a new transaction or no transaction if this is called outside one
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, common.Errorf("failed to begin transaction for problem validation job: %w", err)
	}
	defer tx.Rollback()

	if err := s.jobRepo.CreateJob(ctx, tx, job); err != nil {
		return nil, common.Errorf("failed to create problem validation job in DB: %w", err)
	}

	if err := s.rdb.LPush(ctx, config.AppConfig.ExecutionQueueName, job.ID).Err(); err != nil {
		return nil, common.Errorf("failed to push problem validation job ID to Redis queue: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, common.Errorf("failed to commit transaction for problem validation job: %w", err)
	}

	log.Printf("Problem validation job %s for problem %s enqueued successfully.", job.ID, problemID)
	return job, nil
}

// EnqueueRunCodeJob
func (s *ExecutionJobService) EnqueueRunCodeJob(ctx context.Context, userID, languageID, code string, inputs []string, problemID *string, runtimeLimitMs, memoryLimitKb int) (string, error) {
	payloadData := model.RunCodePayload{
		UserID:         userID,
		LanguageID:     languageID,
		Code:           code,
		Inputs:         inputs,
		ProblemID:      problemID,
		RuntimeLimitMs: runtimeLimitMs,
		MemoryLimitKb:  memoryLimitKb,
	}
	payloadBytes, err := json.Marshal(payloadData)
	if err != nil {
		return "", common.Errorf("failed to marshal run code payload: %w", err)
	}

	job := &model.ExecutionJob{
		ID:        uuid.NewString(),
		JobType:   model.JobTypeRunCode,
		Payload:   payloadBytes,
		Status:    model.JobStatusQueued,
		ProblemID: problemID, // Store problem ID for context if provided
	}

	// Run code jobs might not need a persistent DB record, or they could.
	// For simplicity, let's create one.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", common.Errorf("failed to begin transaction for run code job: %w", err)
	}
	defer tx.Rollback()

	if err := s.jobRepo.CreateJob(ctx, tx, job); err != nil {
		return "", common.Errorf("failed to create run code job in DB: %w", err)
	}

	if err := s.rdb.LPush(ctx, config.AppConfig.ExecutionQueueName, job.ID).Err(); err != nil {
		return "", common.Errorf("failed to push run code job ID to Redis queue: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", common.Errorf("failed to commit transaction for run code job: %w", err)
	}
	log.Printf("Run code job %s enqueued successfully.", job.ID)
	return job.ID, nil
}

// UpdateJobStatus (called by worker or webhook handler)
// ...
