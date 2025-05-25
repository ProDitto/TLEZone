package model

import "time"

type SubmissionStatus string

const (
	StatusPending            SubmissionStatus = "Pending"
	StatusInQueue            SubmissionStatus = "InQueue"
	StatusProcessing         SubmissionStatus = "Processing"
	StatusAccepted           SubmissionStatus = "Accepted"
	StatusWrongAnswer        SubmissionStatus = "WrongAnswer"
	StatusTimeLimitExceeded  SubmissionStatus = "TimeLimitExceeded"
	StatusMemoryLimitExceeded SubmissionStatus = "MemoryLimitExceeded"
	StatusCompilationError   SubmissionStatus = "CompilationError"
	StatusRuntimeError       SubmissionStatus = "RuntimeError"
	StatusSystemError        SubmissionStatus = "SystemError"       // Error in our system/executor
	StatusValidationError    SubmissionStatus = "ValidationError" // For problem validation itself
)

type Submission struct {
	ID                  string             `json:"id"`
	UserID              string             `json:"user_id"`
	ProblemID           string             `json:"problem_id"`
	LanguageID          string             `json:"language_id"`
	Code                string             `json:"code"` // Might omit from general listings
	Status              SubmissionStatus   `json:"status"`
	ExecutionTimeMs     *int               `json:"execution_time_ms,omitempty"`
	MemoryKb            *int               `json:"memory_kb,omitempty"`
	IsRunCodeAttempt    bool               `json:"is_run_code_attempt"`
	SubmittedAt         time.Time          `json:"submitted_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
	TestCaseResults     []SubmissionTestCaseResult `json:"test_case_results,omitempty"` // Detailed results for each test case
	UserUsername        *string            `json:"user_username,omitempty"`             // For display
	ProblemTitle        *string            `json:"problem_title,omitempty"`             // For display
	LanguageSlug        *string            `json:"language_slug,omitempty"`             // For display
}

type SubmissionTestCaseResult struct {
	ID                string           `json:"id"`
	SubmissionID      string           `json:"submission_id"`
	TestCaseID        string           `json:"test_case_id"` // Links to the original TestCase
	ActualOutput      *string          `json:"actual_output,omitempty"`
	Status            SubmissionStatus `json:"status"` // Status for this specific test case
	ExecutionTimeMs   *int             `json:"execution_time_ms,omitempty"`
	MemoryKb          *int             `json:"memory_kb,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
}

// For "Run Code" API which doesn't create full submission record, but result is similar
type RunCodeResult struct {
	Input             string           `json:"input"` // The input it ran against
	ExpectedOutput    *string          `json:"expected_output,omitempty"`
	ActualOutput      *string          `json:"actual_output,omitempty"`
	Status            SubmissionStatus `json:"status"`
	ExecutionTimeMs   *int             `json:"execution_time_ms,omitempty"`
	MemoryKb          *int             `json:"memory_kb,omitempty"`
	CompilationOutput *string          `json:"compilation_output,omitempty"`
	ErrorOutput       *string          `json:"error_output,omitempty"`
}