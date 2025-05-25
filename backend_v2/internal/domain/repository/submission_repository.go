package repository

import (
	"context"
	"database/sql"
	"tle_zone_v2/internal/domain/model"
)

type SubmissionRepository interface {
	CreateSubmission(ctx context.Context, tx *sql.Tx, sub *model.Submission) error
	GetSubmissionByID(ctx context.Context, id string) (*model.Submission, error)
	UpdateSubmissionStatus(ctx context.Context, tx *sql.Tx, submissionID string, status model.SubmissionStatus, execTimeMs *int, memoryKb *int) error
	// GetSubmissionsByUserIDAndProblemID, ListSubmissions, etc.
	CreateSubmissionTestResults(ctx context.Context, tx *sql.Tx, results []model.SubmissionTestCaseResult) error
	GetSubmissionTestResults(ctx context.Context, submissionID string) ([]model.SubmissionTestCaseResult, error)

	// For "Run Code" that doesn't persist full submission
	// StoreTemporaryRunResult (e.g. in Redis with TTL, or a short-lived table)
	// Or, if execution job ID is used for run_code, webhook updates that job.

	// For code history
	GetSubmissionsForUserProblem(ctx context.Context, userID, problemID string, limit, offset int) ([]model.Submission, int, error)

	// For leaderboards
	MarkProblemSolved(ctx context.Context, tx *sql.Tx, userID, problemID, submissionID string) error
	CountSolvedProblemsByUser(ctx context.Context, userID string) (int, error)
	GetLeaderboard(ctx context.Context, limit int) ([]model.LeaderboardEntry, error)
}

// Pg implementation would follow...
