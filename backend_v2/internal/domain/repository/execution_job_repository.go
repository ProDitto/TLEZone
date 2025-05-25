package repository

import (
	"context"
	"database/sql"
	"tle_zone_v2/internal/domain/model"
)

type ExecutionJobRepository interface {
	CreateJob(ctx context.Context, tx *sql.Tx, job *model.ExecutionJob) error
	GetJobByID(ctx context.Context, id string) (*model.ExecutionJob, error)
	GetJobBySubmissionID(ctx context.Context, submissionID string) (*model.ExecutionJob, error)
	UpdateJobStatus(ctx context.Context, tx *sql.Tx, jobID string, status string, lastError *string) error
	IncrementJobAttempts(ctx context.Context, tx *sql.Tx, jobID string) error
	// FindQueuedJob (maybe for recovery, though worker BRPOPs)
}

// Pg implementation would follow...
