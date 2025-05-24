package model

import "time"

type User struct {
	ID       int
	Username string
	Email    string
	Role     string
}

type UserRegistration struct {
	Username        string
	Email           string
	Password        string
	ConfirmPassword string
}

type UserUpdate struct {
	Username string
	Email    string
}

type UserLogin struct {
	Username string
	Password string
}

type basic struct {
	ID   int
	Name string
}

type Tag basic
type Difficulty basic

type ProblemInfo struct {
	ID         int
	Title      string
	Tags       []Tag
	Difficulty Difficulty
}

type ExampleTestCase struct {
	ID          int
	Input       string
	Expected    string
	Explanation string
}

type ProblemDetail struct {
	ID               int
	Title            string
	Tags             []Tag
	Difficulty       Difficulty
	Description      string
	ExampleTestCases []ExampleTestCase
	Constraints      []string
}

type ProblemFilter struct {
	DifficultyIDs []int
	TagIDs        []int
	Page          int
	PageSize      int
}

type TestCase struct {
	ID             int
	Input          string
	ExpectedOutput string
}

type Submission struct {
	ID         int
	UserID     int
	ProblemID  int
	Code       string
	LanguageID int
	Status     string
	RuntimeMS  int
	MemoryKB   int
}

type ExecutionResult struct {
	SubmissionID int
	Status       string
	RuntimeMS    int
	MemoryKB     int
	Output       string
	Error        string
}

type Contest struct {
	ID         int
	Name       string
	StartTime  time.Time
	EndTime    time.Time
	ProblemIDs []int
	Status     string
}

type LeaderboardScope struct {
	ContestID *int // nil for global leaderboard
}

type LeaderboardEntry struct {
	Rank     int
	UserID   int
	Username string
	Score    int
}

type CodeExecutionRequest struct {
	SubmissionID  int    // Used to link execution result to a submission
	Code          string // User-submitted code
	LanguageID    int
	TestCases     []TestCase
	TimeLimitMS   int
	MemoryLimitKB int
}

type TestCaseExecutionResult struct {
	ID        int
	Status    string
	RuntimeMS int
	MemoryKB  int
	Message   string
}

type CodeExecutionResponse struct {
	SubmissionID  int    // Echoed back for tracking
	OverallStatus string // e.g., "Accepted", "Wrong Answer", "Runtime Error", "Time Limit Exceeded"
	Results       []TestCaseExecutionResult
	Error         string // Compiler/runtime error messages if any
}
