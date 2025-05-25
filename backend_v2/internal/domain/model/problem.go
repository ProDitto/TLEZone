package model

import (
	"time"
)

type ProblemDifficulty string
type ProblemStatus string

const (
	DifficultyEasy   ProblemDifficulty = "Easy"
	DifficultyMedium ProblemDifficulty = "Medium"
	DifficultyHard   ProblemDifficulty = "Hard"

	StatusDraft             ProblemStatus = "Draft"
	StatusPendingValidation ProblemStatus = "PendingValidation"
	StatusPublished         ProblemStatus = "Published"
	StatusRejected          ProblemStatus = "Rejected"
)

type Problem struct {
	ID                   string            `json:"id"`
	Title                string            `json:"title"`
	Slug                 string            `json:"slug"`
	Description          string            `json:"description"`
	Difficulty           ProblemDifficulty `json:"difficulty"`
	Status               ProblemStatus     `json:"status"`
	SolutionCode         *string           `json:"solution_code,omitempty"` // Admin only view
	SolutionLanguageID   *string           `json:"solution_language_id,omitempty"`
	RuntimeLimitMs       int               `json:"runtime_limit_ms"`
	MemoryLimitKb        int               `json:"memory_limit_kb"`
	CreatedByID          *string           `json:"created_by_id,omitempty"`
	ValidatedByID        *string           `json:"validated_by_id,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
	Tags                 []Tag             `json:"tags,omitempty"`
	Examples             []Example         `json:"examples,omitempty"`               // Public test cases
	TestCases            []TestCase        `json:"test_cases,omitempty"`             // Hidden test cases (admin only view)
	SolutionLanguageSlug *string           `json:"solution_language_slug,omitempty"` // For convenience
	CreatedByUsername    *string           `json:"created_by_username,omitempty"`    // For display
	ValidatedByUsername  *string           `json:"validated_by_username,omitempty"`  // For display
}

type Example struct {
	ID             string    `json:"id"`
	ProblemID      string    `json:"problem_id"`
	Input          string    `json:"input"`
	ExpectedOutput *string   `json:"expected_output,omitempty"`
	Explanation    *string   `json:"explanation,omitempty"`
	IsRunnable     bool      `json:"is_runnable"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type TestCase struct { // Hidden test cases
	ID             string    `json:"id"`
	ProblemID      string    `json:"problem_id"`
	Input          string    `json:"input"`
	ExpectedOutput string    `json:"expected_output"`
	IsHidden       bool      `json:"is_hidden"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
