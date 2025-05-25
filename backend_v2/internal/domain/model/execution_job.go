package model

import (
	"encoding/json"
	"time"
)

const (
	JobTypeSubmissionEvaluation = "submission_evaluation"
	JobTypeProblemValidation    = "problem_validation"
	JobTypeRunCode              = "run_code"

	JobStatusQueued          = "Queued"
	JobStatusProcessing      = "Processing" // Worker picked it up, trying to get lock
	JobStatusLocked          = "Locked"     // Worker acquired lock, preparing to send
	JobStatusSentToExecutor  = "SentToExecutor"
	JobStatusCompleted       = "Completed"    // Webhook received
	JobStatusFailed          = "Failed"       // Worker failed before sending or unrecoverable error
	JobStatusWebhookTimedOut = "WebhookTimedOut" // If webhook doesn't arrive
)

type ExecutionJob struct {
	ID            string          `json:"id"`
	SubmissionID  *string         `json:"submission_id,omitempty"` // If evaluating a full submission
	ProblemID     *string         `json:"problem_id,omitempty"`    // If validating a problem or run code
	JobType       string          `json:"job_type"`
	Payload       json.RawMessage `json:"-"` // Not directly exposed; internal use
	Status        string          `json:"status"`
	Attempts      int             `json:"attempts"`
	LastError     *string         `json:"last_error,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// Payloads for different job types (stored in ExecutionJob.Payload)
type SubmissionEvaluationPayload struct {
	SubmissionID string `json:"submission_id"`
}
type ProblemValidationPayload struct {
	ProblemID string `json:"problem_id"`
}
type RunCodePayload struct { // Payload for a 'run code' job
	UserID         string   `json:"user_id"` // To associate temporary results if needed, or for logging
	LanguageID     string   `json:"language_id"`
	Code           string   `json:"code"`
	Inputs         []string `json:"inputs"` // User provided inputs for "run code"
	ProblemID      *string  `json:"problem_id,omitempty"` // Optional, if running against problem examples
	RuntimeLimitMs int      `json:"runtime_limit_ms"`
	MemoryLimitKb  int      `json:"memory_limit_kb"`
}