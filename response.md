Okay, this is a very comprehensive and well-defined project. Building the *entire* backend in "one go" to a production-ready state is a monumental task. I will provide a robust foundation covering all the core features, emphasizing the requested patterns and technologies.

**Assumptions & Simplifications for this "One Go" Build:**

1.  **External Code Execution API:** Interaction will be mocked. The worker will log the request it *would* send. The webhook handler will be present to receive (simulated) callbacks.
2.  **Leaderboard Logic:** Basic implementation (e.g., number of problems solved). Complex scoring like "time to solve" with penalties will be simplified.
3.  **Code Diff:** Will store submission history. The actual "diff" endpoint might return the two code blobs to be diffed on the client, or a very basic text diff.
4.  **Error Handling:** Will use a structured JSON error response. Error wrapping will be used.
5.  **Security:** JWT for auth. Webhook security will be noted but basic. No advanced rate limiting, XSS, CSRF protection beyond Chi defaults.
6.  **Testing:** No unit/integration tests will be written in this output.
7.  **Migrations:** A single SQL schema file. `golang-migrate/migrate` setup is good practice but adds overhead for this single output.
8.  **Configuration:** `.env` file for all settings.
9.  **"Run Code" API:** Will follow the same async execution path as submissions but won't persist submission records for leaderboard/history.
10. **Search/Filtering for Problems:** Basic implementation for tags and difficulty. Full-text search is out of scope.
11. **One Active Execution Job:** This will be managed using a Redis distributed lock.

Let's begin!

---
**1. Project Setup & `.env`**
---

Create `go.mod`:
```bash
go mod init leetcode-clone-scalable
go get github.com/go-chi/chi/v5
go get github.com/go-chi/jwtauth/v5
go get github.com/golang-jwt/jwt/v5 # jwtauth dependency
go get github.com/joho/godotenv
go get github.com/jackc/pgx/v5/stdlib # For database/sql
go get github.com/jackc/pgx/v5/pgconn # For error handling
go get github.com/redis/go-redis/v9
go get golang.org/x/crypto/bcrypt
go get github.com/google/uuid # For UUIDs
go get github.com/lib/pq # Alternative pg driver, pgx/v5/stdlib is preferred
```
*(Note: `go mod tidy` will clean this up later)*

Create `.env` file:
```.env
# API Configuration
API_PORT=8080
JWT_SECRET="your-very-strong-and-long-jwt-secret-key"
JWT_EXPIRATION_HOURS=72

# PostgreSQL Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=youruser
DB_PASSWORD=yourpassword
DB_NAME=leetcode_clone_db
DB_SSLMODE=disable # adjust as needed, e.g., require

# Redis Configuration
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=
REDIS_DB=0

# Worker Configuration
EXECUTION_QUEUE_NAME="execution_jobs_queue"
EXECUTION_LOCK_KEY="execution_job_lock"
EXECUTION_LOCK_TTL_SECONDS=300 # Time for which the lock is held (should be > max job execution time)
MOCK_EXECUTOR_WEBHOOK_URL="http://localhost:8080/api/v1/webhook/execution" # Where the mock executor calls back

# For Problem Validation via Execution Service
PROBLEM_VALIDATION_LANGUAGE_ID="1" # Assume a default language ID for problem solution validation
```

---
**2. Directory Structure (High-Level)**
---
```
leetcode-clone-scalable/
├── cmd/server/main.go
├── cmd/worker/main.go  # Optional: if worker runs as separate process
├── internal/
│   ├── api/             # HTTP handlers, router, middlewares
│   │   ├── handler/
│   │   ├── middleware/
│   │   └── router.go
│   ├── app/             # Core application logic (services)
│   │   ├── service/
│   │   └── worker/      # Execution job worker
│   ├── domain/          # Core business models and repository interfaces
│   │   ├── model/
│   │   └── repository/
│   ├── platform/        # Infrastructure concerns
│   │   ├── config/
│   │   ├── database/
│   │   └── queue/       # Redis client setup
│   └── common/          # Shared utilities (errors, responses, etc.)
│       ├── errors.go
│       ├── response.go
│       └── security/    # Hashing, JWT
├── migrations/
│   └── 001_initial_schema.sql
├── go.mod
├── go.sum
└── .env
```

---
**3. `migrations/001_initial_schema.sql`**
---
```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    hashed_password TEXT NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'user', -- 'user', 'admin'
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE languages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(50) UNIQUE NOT NULL, -- e.g., 'Go', 'Python 3', 'JavaScript'
    slug VARCHAR(50) UNIQUE NOT NULL, -- e.g., 'go', 'python3', 'javascript' (for API usage)
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TYPE problem_difficulty AS ENUM ('Easy', 'Medium', 'Hard');
CREATE TYPE problem_status AS ENUM ('Draft', 'PendingValidation', 'Published', 'Rejected');

CREATE TABLE problems (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL, -- auto-generated from title
    description TEXT NOT NULL,
    difficulty problem_difficulty NOT NULL,
    status problem_status NOT NULL DEFAULT 'Draft',
    solution_code TEXT, -- Admin's reference solution
    solution_language_id UUID REFERENCES languages(id) ON DELETE SET NULL,
    runtime_limit_ms INTEGER DEFAULT 2000, -- Default runtime limit in milliseconds
    memory_limit_kb INTEGER DEFAULT 256000, -- Default memory limit in kilobytes
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    validated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE examples ( -- Public/sample test cases
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    input TEXT NOT NULL,
    expected_output TEXT, -- Can be null if it's just an input example
    explanation TEXT,
    is_runnable BOOLEAN DEFAULT TRUE, -- Whether users can run code against this example
    sort_order SERIAL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE test_cases ( -- Hidden/private test cases for judging
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    input TEXT NOT NULL,
    expected_output TEXT NOT NULL,
    is_hidden BOOLEAN DEFAULT TRUE,
    sort_order SERIAL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE tags (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(50) UNIQUE NOT NULL,
    slug VARCHAR(50) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE problem_tags (
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (problem_id, tag_id)
);

CREATE TYPE submission_status AS ENUM (
    'Pending', 'InQueue', 'Processing', 'Accepted', 'WrongAnswer',
    'TimeLimitExceeded', 'MemoryLimitExceeded', 'CompilationError',
    'RuntimeError', 'SystemError', 'ValidationError' -- For problem validation
);

CREATE TABLE submissions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    language_id UUID NOT NULL REFERENCES languages(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    status submission_status DEFAULT 'Pending',
    execution_time_ms INTEGER,
    memory_kb INTEGER,
    is_run_code_attempt BOOLEAN DEFAULT FALSE, -- True if it was a "Run Code" action, not a full submission
    submitted_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Stores results for each hidden test case of a submission
CREATE TABLE submission_test_case_results (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    submission_id UUID NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
    test_case_id UUID NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    actual_output TEXT,
    status submission_status NOT NULL, -- Status for this specific test case
    execution_time_ms INTEGER,
    memory_kb INTEGER,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Tracks jobs in the execution queue (optional, can also infer from submission status)
CREATE TABLE execution_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    submission_id UUID UNIQUE REFERENCES submissions(id) ON DELETE CASCADE, -- Can be NULL if it's a problem validation job not tied to a user submission
    problem_id UUID REFERENCES problems(id) ON DELETE CASCADE, -- For problem validation jobs
    job_type VARCHAR(50) NOT NULL, -- 'submission_evaluation', 'problem_validation', 'run_code'
    payload JSONB, -- Details like code, language, test cases (if not fetching from problem/submission)
    status VARCHAR(50) DEFAULT 'Queued', -- Queued, Processing, SentToExecutor, Completed, Failed
    attempts INTEGER DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE user_solved_problems (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    problem_id UUID NOT NULL REFERENCES problems(id) ON DELETE CASCADE,
    first_accepted_submission_id UUID REFERENCES submissions(id) ON DELETE SET NULL,
    solved_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, problem_id)
);

-- Indexes
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_problems_slug ON problems(slug);
CREATE INDEX idx_problems_difficulty ON problems(difficulty);
CREATE INDEX idx_problems_status ON problems(status);
CREATE INDEX idx_examples_problem_id ON examples(problem_id);
CREATE INDEX idx_test_cases_problem_id ON test_cases(problem_id);
CREATE INDEX idx_tags_slug ON tags(slug);
CREATE INDEX idx_submissions_user_problem ON submissions(user_id, problem_id);
CREATE INDEX idx_submissions_status ON submissions(status);
CREATE INDEX idx_execution_jobs_status ON execution_jobs(status);
CREATE INDEX idx_execution_jobs_type ON execution_jobs(job_type);


-- Trigger to update updated_at columns
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_problems_updated_at BEFORE UPDATE ON problems FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_examples_updated_at BEFORE UPDATE ON examples FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_test_cases_updated_at BEFORE UPDATE ON test_cases FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_submissions_updated_at BEFORE UPDATE ON submissions FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_execution_jobs_updated_at BEFORE UPDATE ON execution_jobs FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Seed initial data
INSERT INTO languages (name, slug, is_active) VALUES
('Go', 'go', TRUE),
('Python 3', 'python3', TRUE),
('JavaScript', 'javascript', TRUE),
('Java', 'java', TRUE),
('C++', 'cpp', TRUE);

-- Seed an admin user (password: adminpassword)
INSERT INTO users (username, email, hashed_password, role)
VALUES ('admin', 'admin@example.com', '$2a$10$D9y226952x8h4wGSFLrv9e3sHwM5rD.07m0gSbo9nDU2wP6yVAtUW', 'admin');
```

---
**4. `internal/platform/config/config.go`**
---
```go
package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	APIPort string
	JWTKey  []byte
	JWTExp  time.Duration

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSslMode  string
	DBConnStr  string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	ExecutionQueueName      string
	ExecutionLockKey        string
	ExecutionLockTTLSeconds int
	MockExecutorWebhookURL  string
	ProblemValidationLangID string
}

var AppConfig *Config

func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	AppConfig = &Config{
		APIPort:                 getEnv("API_PORT", "8080"),
		JWTKey:                  []byte(getEnv("JWT_SECRET", "defaultsecret")),
		JWTExp:                  time.Duration(getEnvAsInt("JWT_EXPIRATION_HOURS", 72)) * time.Hour,
		DBHost:                  getEnv("DB_HOST", "localhost"),
		DBPort:                  getEnv("DB_PORT", "5432"),
		DBUser:                  getEnv("DB_USER", "user"),
		DBPassword:              getEnv("DB_PASSWORD", "password"),
		DBName:                  getEnv("DB_NAME", "leetcode_clone_db"),
		DBSslMode:               getEnv("DB_SSLMODE", "disable"),
		RedisAddr:               getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:           getEnv("REDIS_PASSWORD", ""),
		RedisDB:                 getEnvAsInt("REDIS_DB", 0),
		ExecutionQueueName:      getEnv("EXECUTION_QUEUE_NAME", "execution_jobs_queue"),
		ExecutionLockKey:        getEnv("EXECUTION_LOCK_KEY", "execution_job_lock"),
		ExecutionLockTTLSeconds: getEnvAsInt("EXECUTION_LOCK_TTL_SECONDS", 300),
		MockExecutorWebhookURL:  getEnv("MOCK_EXECUTOR_WEBHOOK_URL", "http://localhost:8080/api/v1/webhook/execution"),
		ProblemValidationLangID: getEnv("PROBLEM_VALIDATION_LANGUAGE_ID", ""),
	}

	AppConfig.DBConnStr = "host=" + AppConfig.DBHost +
		" port=" + AppConfig.DBPort +
		" user=" + AppConfig.DBUser +
		" password=" + AppConfig.DBPassword +
		" dbname=" + AppConfig.DBName +
		" sslmode=" + AppConfig.DBSslMode
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}
```

---
**5. `internal/platform/database/postgres.go`**
---
```go
package database

import (
	"database/sql"
	"fmt"
	"leetcode-clone-scalable/internal/platform/config"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

var DB *sql.DB

func Connect() {
	var err error
	DB, err = sql.Open("pgx", config.AppConfig.DBConnStr)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(25)
	DB.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection
	if err = DB.Ping(); err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	fmt.Println("Successfully connected to PostgreSQL database!")
}

func Close() {
	if DB != nil {
		DB.Close()
		fmt.Println("Database connection closed.")
	}
}
```

---
**6. `internal/platform/queue/redis.go`**
---
```go
package queue

import (
	"context"
	"fmt"
	"leetcode-clone-scalable/internal/platform/config"
	"log"

	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client

func ConnectRedis() {
	RDB = redis.NewClient(&redis.Options{
		Addr:     config.AppConfig.RedisAddr,
		Password: config.AppConfig.RedisPassword,
		DB:       config.AppConfig.RedisDB,
	})

	ctx := context.Background()
	_, err := RDB.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	fmt.Println("Successfully connected to Redis!")
}

func CloseRedis() {
	if RDB != nil {
		RDB.Close()
		fmt.Println("Redis connection closed.")
	}
}
```

---
**7. Models (`internal/domain/model/*.go`)**
---
*(Due to length, I'll show `user.go` and `problem.go` as examples, others will follow a similar pattern)*

**`internal/domain/model/user.go`**
```go
package model

import (
	"time"
)

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

type User struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	Email          string    `json:"email"`
	HashedPassword string    `json:"-"` // Not exposed
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
```

**`internal/domain/model/problem.go`**
```go
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
	ID                    string            `json:"id"`
	Title                 string            `json:"title"`
	Slug                  string            `json:"slug"`
	Description           string            `json:"description"`
	Difficulty            ProblemDifficulty `json:"difficulty"`
	Status                ProblemStatus     `json:"status"`
	SolutionCode          *string           `json:"solution_code,omitempty"` // Admin only view
	SolutionLanguageID    *string           `json:"solution_language_id,omitempty"`
	RuntimeLimitMs        int               `json:"runtime_limit_ms"`
	MemoryLimitKb         int               `json:"memory_limit_kb"`
	CreatedByID           *string           `json:"created_by_id,omitempty"`
	ValidatedByID         *string           `json:"validated_by_id,omitempty"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
	Tags                  []Tag             `json:"tags,omitempty"`
	Examples              []Example         `json:"examples,omitempty"`       // Public test cases
	TestCases             []TestCase        `json:"test_cases,omitempty"`     // Hidden test cases (admin only view)
	SolutionLanguageSlug  *string           `json:"solution_language_slug,omitempty"` // For convenience
	CreatedByUsername     *string           `json:"created_by_username,omitempty"`    // For display
	ValidatedByUsername   *string           `json:"validated_by_username,omitempty"`  // For display
}

type Example struct {
	ID              string    `json:"id"`
	ProblemID       string    `json:"problem_id"`
	Input           string    `json:"input"`
	ExpectedOutput  *string   `json:"expected_output,omitempty"`
	Explanation     *string   `json:"explanation,omitempty"`
	IsRunnable      bool      `json:"is_runnable"`
	SortOrder       int       `json:"sort_order"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
```

**`internal/domain/model/tag.go`**
```go
package model

import "time"

type Tag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
}
```

**`internal/domain/model/language.go`**
```go
package model

import "time"

type Language struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"` // For API usage
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}
```

**`internal/domain/model/submission.go`**
```go
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
```

**`internal/domain/model/execution_job.go`**
```go
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
```

**`internal/domain/model/leaderboard.go`**
```go
package model

type LeaderboardEntry struct {
	Rank           int    `json:"rank"`
	UserID         string `json:"user_id"`
	Username       string `json:"username"`
	ProblemsSolved int    `json:"problems_solved"`
	// Accuracy       float64 `json:"accuracy"` // (Accepted Submissions / Total Submissions)
	// AvgTimeToSolve int64 `json:"avg_time_to_solve_seconds"` // Complex to calculate
}
```

---
**8. `internal/common/security/*.go`**
---
**`internal/common/security/password.go`**
```go
package security

import "golang.org/x/crypto/bcrypt"

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
```

**`internal/common/security/jwt.go`**
```go
package security

import (
	"errors"
	"leetcode-clone-scalable/internal/platform/config"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/golang-jwt/jwt/v5"
)

var TokenAuth *jwtauth.JWTAuth

func InitJWT() {
	TokenAuth = jwtauth.New("HS256", config.AppConfig.JWTKey, nil)
}

func GenerateToken(userID, role string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"exp":     time.Now().Add(config.AppConfig.JWTExp).Unix(),
		"iat":     time.Now().Unix(),
	}
	_, tokenString, err := TokenAuth.Encode(claims)
	return tokenString, err
}

// Helper functions to extract claims, can be used in middleware or services
func GetUserIDFromClaims(claims jwt.MapClaims) (string, error) {
	id, ok := claims["user_id"].(string)
	if !ok {
		return "", errors.New("user_id claim is missing or not a string")
	}
	return id, nil
}

func GetUserRoleFromClaims(claims jwt.MapClaims) (string, error) {
	role, ok := claims["role"].(string)
	if !ok {
		return "", errors.New("role claim is missing or not a string")
	}
	return role, nil
}
```

---
**9. `internal/common/response.go` & `errors.go`**
---
**`internal/common/response.go`**
```go
package common

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error string `json:"error"`
	// Details interface{} `json:"details,omitempty"` // For more detailed validation errors
}

func RespondWithError(w http.ResponseWriter, code int, message string) {
	RespondWithJSON(w, code, ErrorResponse{Error: message})
}

func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Failed to marshal JSON response"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}
```

**`internal/common/errors.go`**
```go
package common

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound            = errors.New("requested resource not found")
	ErrUnauthorized        = errors.New("unauthorized access")
	ErrForbidden           = errors.New("forbidden access")
	ErrBadRequest          = errors.New("bad request")
	ErrConflict            = errors.New("resource conflict") // e.g., username already exists
	ErrInternalServer      = errors.New("internal server error")
	ErrValidation          = errors.New("validation failed")
	ErrServiceUnavailable  = errors.New("service unavailable") // e.g. external executor down
	ErrJobLockFailed       = errors.New("failed to acquire job lock")
)

// HTTPStatusFromError maps domain errors to HTTP status codes.
func HTTPStatusFromError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, ErrUnauthorized) {
		return http.StatusUnauthorized
	}
	if errors.Is(err, ErrForbidden) {
		return http.StatusForbidden
	}
	if errors.Is(err, ErrBadRequest) || errors.Is(err, ErrValidation) {
		return http.StatusBadRequest
	}
	if errors.Is(err, ErrConflict) {
		return http.StatusConflict
	}
	if errors.Is(err, ErrServiceUnavailable) {
		return http.StatusServiceUnavailable
	}
	if errors.Is(err, ErrJobLockFailed) {
		return http.StatusConflict // Or 503 if it's transient
	}


	// Check for pgx specific errors (example for unique constraint)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" { // Unique violation
			return http.StatusConflict
		}
	}

	return http.StatusInternalServerError
}

// Errorf creates a new error with formatting, useful for wrapping.
func Errorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
```

---
**10. Repositories (`internal/domain/repository/*.go`)**
---
*(Interfaces first, then Pg implementations. I'll show `user_repository.go` and parts of `problem_repository.go` due to extreme length. Others will follow similar patterns.)*

**`internal/domain/repository/user_repository.go`**
```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"leetcode-clone-scalable/internal/domain/model"
	"leetcode-clone-scalable/internal/platform/database"

	"github.com/jackc/pgx/v5/pgconn"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	FindByID(ctx context.Context, id string) (*model.User, error)
}

type pgUserRepository struct {
	db *sql.DB
}

func NewPgUserRepository(db *sql.DB) UserRepository {
	return &pgUserRepository{db: db}
}

func (r *pgUserRepository) Create(ctx context.Context, user *model.User) error {
	query := `INSERT INTO users (id, username, email, hashed_password, role)
	          VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.ExecContext(ctx, query, user.ID, user.Username, user.Email, user.HashedPassword, user.Role)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // Unique constraint violation
			return fmt.Errorf("user with given username or email already exists: %w", common.ErrConflict)
		}
		return fmt.Errorf("pgUserRepository.Create: %w", err)
	}
	return nil
}

func (r *pgUserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT id, username, email, hashed_password, role, created_at, updated_at
	          FROM users WHERE email = $1`
	user := &model.User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("pgUserRepository.FindByEmail: %w", err)
	}
	return user, nil
}
// FindByUsername and FindByID are similar to FindByEmail
// ... (implement FindByUsername, FindByID)
func (r *pgUserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	query := `SELECT id, username, email, hashed_password, role, created_at, updated_at
	          FROM users WHERE username = $1`
	user := &model.User{}
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("pgUserRepository.FindByUsername: %w", err)
	}
	return user, nil
}

func (r *pgUserRepository) FindByID(ctx context.Context, id string) (*model.User, error) {
	query := `SELECT id, username, email, hashed_password, role, created_at, updated_at
	          FROM users WHERE id = $1`
	user := &model.User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("pgUserRepository.FindByID: %w", err)
	}
	return user, nil
}
```

**`internal/domain/repository/problem_repository.go` (Interface and partial Pg impl.)**
```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"leetcode-clone-scalable/internal/domain/model"
	"leetcode-clone-scalable/internal/platform/database"
	"strings"
	"github.com/lib/pq" // For array handling if needed, though pgx is generally better
)

type ProblemRepository interface {
	CreateProblem(ctx context.Context, tx *sql.Tx, problem *model.Problem) error
	UpdateProblem(ctx context.Context, tx *sql.Tx, problem *model.Problem) error
	FindProblemByID(ctx context.Context, id string) (*model.Problem, error)
	FindProblemBySlug(ctx context.Context, slug string) (*model.Problem, error)
	ListProblems(ctx context.Context, limit, offset int, difficulty model.ProblemDifficulty, tagIDs []string, status model.ProblemStatus, searchTerm string) ([]model.Problem, int, error)

	AddExamplesToProblem(ctx context.Context, tx *sql.Tx, problemID string, examples []model.Example) error
	GetExamplesByProblemID(ctx context.Context, problemID string) ([]model.Example, error)
    DeleteExamplesByProblemID(ctx context.Context, tx *sql.Tx, problemID string) error


	AddTestCasesToProblem(ctx context.Context, tx *sql.Tx, problemID string, testCases []model.TestCase) error
	GetTestCasesByProblemID(ctx context.Context, problemID string) ([]model.TestCase, error) // For judging/admin
    DeleteTestCasesByProblemID(ctx context.Context, tx *sql.Tx, problemID string) error

	AddTagsToProblem(ctx context.Context, tx *sql.Tx, problemID string, tagIDs []string) error
	GetTagsByProblemID(ctx context.Context, problemID string) ([]model.Tag, error)
    RemoveTagsFromProblem(ctx context.Context, tx *sql.Tx, problemID string, tagIDs []string) error
    ClearProblemTags(ctx context.Context, tx *sql.Tx, problemID string) error

	GetLanguageByID(ctx context.Context, id string) (*model.Language, error)
	GetLanguageBySlug(ctx context.Context, slug string) (*model.Language, error)
    ListLanguages(ctx context.Context) ([]model.Language, error)
}

type pgProblemRepository struct {
	db *sql.DB
}

func NewPgProblemRepository(db *sql.DB) ProblemRepository {
	return &pgProblemRepository{db: db}
}

func (r *pgProblemRepository) CreateProblem(ctx context.Context, tx *sql.Tx, p *model.Problem) error {
	query := `INSERT INTO problems (id, title, slug, description, difficulty, status, solution_code, solution_language_id, runtime_limit_ms, memory_limit_kb, created_by)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, p.ID, p.Title, p.Slug, p.Description, p.Difficulty, p.Status, p.SolutionCode, p.SolutionLanguageID, p.RuntimeLimitMs, p.MemoryLimitKb, p.CreatedByID)
	} else {
		_, err = r.db.ExecContext(ctx, query, p.ID, p.Title, p.Slug, p.Description, p.Difficulty, p.Status, p.SolutionCode, p.SolutionLanguageID, p.RuntimeLimitMs, p.MemoryLimitKb, p.CreatedByID)
	}

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // Unique constraint for slug
			return fmt.Errorf("problem with this slug already exists: %w", common.ErrConflict)
		}
		return fmt.Errorf("pgProblemRepository.CreateProblem: %w", err)
	}
	return nil
}

func (r *pgProblemRepository) UpdateProblem(ctx context.Context, tx *sql.Tx, p *model.Problem) error {
    query := `UPDATE problems SET 
                title = $1, slug = $2, description = $3, difficulty = $4, status = $5, 
                solution_code = $6, solution_language_id = $7, runtime_limit_ms = $8, 
                memory_limit_kb = $9, validated_by = $10, updated_at = CURRENT_TIMESTAMP
              WHERE id = $11`
    
    var err error
    if tx != nil {
        _, err = tx.ExecContext(ctx, query, p.Title, p.Slug, p.Description, p.Difficulty, p.Status, p.SolutionCode, p.SolutionLanguageID, p.RuntimeLimitMs, p.MemoryLimitKb, p.ValidatedByID, p.ID)
    } else {
        _, err = r.db.ExecContext(ctx, query, p.Title, p.Slug, p.Description, p.Difficulty, p.Status, p.SolutionCode, p.SolutionLanguageID, p.RuntimeLimitMs, p.MemoryLimitKb, p.ValidatedByID, p.ID)
    }
    if err != nil {
        return fmt.Errorf("pgProblemRepository.UpdateProblem: %w", err)
    }
    return nil
}


func (r *pgProblemRepository) FindProblemByID(ctx context.Context, id string) (*model.Problem, error) {
	query := `
        SELECT p.id, p.title, p.slug, p.description, p.difficulty, p.status, 
               p.solution_code, p.solution_language_id, sol_lang.slug as solution_language_slug,
               p.runtime_limit_ms, p.memory_limit_kb, 
               p.created_by, cb_user.username as created_by_username, 
               p.validated_by, vb_user.username as validated_by_username,
               p.created_at, p.updated_at
        FROM problems p
        LEFT JOIN languages sol_lang ON p.solution_language_id = sol_lang.id
        LEFT JOIN users cb_user ON p.created_by = cb_user.id
        LEFT JOIN users vb_user ON p.validated_by = vb_user.id
        WHERE p.id = $1`
	
	problem := &model.Problem{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&problem.ID, &problem.Title, &problem.Slug, &problem.Description, &problem.Difficulty, &problem.Status,
		&problem.SolutionCode, &problem.SolutionLanguageID, &problem.SolutionLanguageSlug,
		&problem.RuntimeLimitMs, &problem.MemoryLimitKb,
		&problem.CreatedByID, &problem.CreatedByUsername,
		&problem.ValidatedByID, &problem.ValidatedByUsername,
		&problem.CreatedAt, &problem.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("pgProblemRepository.FindProblemByID: %w", err)
	}
	return problem, nil
}
// FindProblemBySlug is similar
// ...

// ListProblems implementation is complex due to filters, requires dynamic query building or CTEs
func (r *pgProblemRepository) ListProblems(ctx context.Context, limit, offset int, difficulty model.ProblemDifficulty, tagIDs []string, status model.ProblemStatus, searchTerm string) ([]model.Problem, int, error) {
    var baseQuery strings.Builder
    baseQuery.WriteString(`
        SELECT DISTINCT p.id, p.title, p.slug, p.description, p.difficulty, p.status,
               p.runtime_limit_ms, p.memory_limit_kb, p.created_at, p.updated_at
        FROM problems p`)

    var countQueryBuilder strings.Builder
    countQueryBuilder.WriteString(`SELECT COUNT(DISTINCT p.id) FROM problems p`)

    var conditions []string
    var args []interface{}
    argID := 1

    if len(tagIDs) > 0 {
        baseQuery.WriteString(" JOIN problem_tags pt ON p.id = pt.problem_id JOIN tags t ON pt.tag_id = t.id")
        countQueryBuilder.WriteString(" JOIN problem_tags pt ON p.id = pt.problem_id JOIN tags t ON pt.tag_id = t.id")
        
        // Placeholders for tag IDs like ($1, $2, $3)
        tagPlaceholders := make([]string, len(tagIDs))
        for i := range tagIDs {
            tagPlaceholders[i] = fmt.Sprintf("$%d", argID)
            args = append(args, tagIDs[i])
            argID++
        }
        conditions = append(conditions, fmt.Sprintf("t.id IN (%s)", strings.Join(tagPlaceholders, ",")))
    }

    if difficulty != "" {
        conditions = append(conditions, fmt.Sprintf("p.difficulty = $%d", argID))
        args = append(args, difficulty)
        argID++
    }
    
    if status != "" { // Filter by status, e.g. only "Published" for users
        conditions = append(conditions, fmt.Sprintf("p.status = $%d", argID))
        args = append(args, status)
        argID++
    }

    if searchTerm != "" {
        conditions = append(conditions, fmt.Sprintf("(p.title ILIKE $%d OR p.description ILIKE $%d)", argID, argID+1))
        likeTerm := "%" + searchTerm + "%"
        args = append(args, likeTerm, likeTerm)
        argID += 2
    }

    if len(conditions) > 0 {
        whereClause := " WHERE " + strings.Join(conditions, " AND ")
        baseQuery.WriteString(whereClause)
        countQueryBuilder.WriteString(whereClause)
    }

    // Count total matching problems (without limit/offset)
    var total int
    // Use a new args slice for count query if tagIDs were present, as their placeholders might differ
    // This part needs careful handling of arg indices if query structure differs significantly
    // For simplicity here, assume args match for count if conditions are the same.
    // If you use pq.Array for tagIDs, arg indexing is simpler.
    err := r.db.QueryRowContext(ctx, countQueryBuilder.String(), args...).Scan(&total)
    if err != nil {
        return nil, 0, fmt.Errorf("pgProblemRepository.ListProblems count: %w", err)
    }


    baseQuery.WriteString(fmt.Sprintf(" ORDER BY p.created_at DESC LIMIT $%d OFFSET $%d", argID, argID+1))
    args = append(args, limit, offset)

    rows, err := r.db.QueryContext(ctx, baseQuery.String(), args...)
    if err != nil {
        return nil, 0, fmt.Errorf("pgProblemRepository.ListProblems query: %w", err)
    }
    defer rows.Close()

    problems := []model.Problem{}
    for rows.Next() {
        var p model.Problem
        if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.Description, &p.Difficulty, &p.Status,
            &p.RuntimeLimitMs, &p.MemoryLimitKb, &p.CreatedAt, &p.UpdatedAt); err != nil {
            return nil, 0, fmt.Errorf("pgProblemRepository.ListProblems scan: %w", err)
        }
        // Optionally, fetch tags for each problem here (N+1 problem) or do it in service layer
        problems = append(problems, p)
    }
    if err = rows.Err(); err != nil {
        return nil, 0, fmt.Errorf("pgProblemRepository.ListProblems rows.Err: %w", err)
    }

    return problems, total, nil
}


func (r *pgProblemRepository) AddExamplesToProblem(ctx context.Context, tx *sql.Tx, problemID string, examples []model.Example) error {
	if len(examples) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO examples (id, problem_id, input, expected_output, explanation, is_runnable, sort_order) VALUES ($1, $2, $3, $4, $5, $6, $7)`)
	if err != nil {
		return fmt.Errorf("pgProblemRepository.AddExamplesToProblem prepare: %w", err)
	}
	defer stmt.Close()

	for i, ex := range examples {
		ex.SortOrder = i + 1 // Auto-assign sort order
		_, err := stmt.ExecContext(ctx, ex.ID, problemID, ex.Input, ex.ExpectedOutput, ex.Explanation, ex.IsRunnable, ex.SortOrder)
		if err != nil {
			return fmt.Errorf("pgProblemRepository.AddExamplesToProblem exec for example %s: %w", ex.ID, err)
		}
	}
	return nil
}

func (r *pgProblemRepository) GetExamplesByProblemID(ctx context.Context, problemID string) ([]model.Example, error) {
    query := `SELECT id, problem_id, input, expected_output, explanation, is_runnable, sort_order, created_at, updated_at
              FROM examples WHERE problem_id = $1 ORDER BY sort_order ASC`
    rows, err := r.db.QueryContext(ctx, query, problemID)
    if err != nil {
        return nil, fmt.Errorf("pgProblemRepository.GetExamplesByProblemID query: %w", err)
    }
    defer rows.Close()

    var examples []model.Example
    for rows.Next() {
        var ex model.Example
        if err := rows.Scan(&ex.ID, &ex.ProblemID, &ex.Input, &ex.ExpectedOutput, &ex.Explanation, &ex.IsRunnable, &ex.SortOrder, &ex.CreatedAt, &ex.UpdatedAt); err != nil {
            return nil, fmt.Errorf("pgProblemRepository.GetExamplesByProblemID scan: %w", err)
        }
        examples = append(examples, ex)
    }
    if err = rows.Err(); err != nil {
        return nil, fmt.Errorf("pgProblemRepository.GetExamplesByProblemID rows.Err: %w", err)
    }
    return examples, nil
}

// DeleteExamplesByProblemID, AddTestCasesToProblem, GetTestCasesByProblemID, DeleteTestCasesByProblemID are similar
// AddTagsToProblem, GetTagsByProblemID, RemoveTagsFromProblem, ClearProblemTags manage the problem_tags join table
// ...
// Language methods
func (r *pgProblemRepository) GetLanguageBySlug(ctx context.Context, slug string) (*model.Language, error) {
	query := `SELECT id, name, slug, is_active, created_at FROM languages WHERE slug = $1 AND is_active = TRUE`
	lang := &model.Language{}
	err := r.db.QueryRowContext(ctx, query, slug).Scan(
		&lang.ID, &lang.Name, &lang.Slug, &lang.IsActive, &lang.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("pgProblemRepository.GetLanguageBySlug: %w", err)
	}
	return lang, nil
}

// Other repository interfaces (SubmissionRepository, TagRepository, ExecutionJobRepository, LeaderboardRepository)
// and their Pg implementations would follow. This is already extremely long.
// I will sketch out interfaces and key methods.
```

**`internal/domain/repository/submission_repository.go` (Interface Sketch)**
```go
package repository

import (
	"context"
	"database/sql"
	"leetcode-clone-scalable/internal/domain/model"
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
```

**`internal/domain/repository/execution_job_repository.go` (Interface Sketch)**
```go
package repository

import (
	"context"
	"database/sql"
	"leetcode-clone-scalable/internal/domain/model"
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
```

---
**11. Services (`internal/app/service/*.go`)**
---
*(Services use repositories and contain business logic. Showing `auth_service.go` and parts of `submission_service.go` and `problem_service.go`)*

**`internal/app/service/auth_service.go`**
```go
package service

import (
	"context"
	"fmt"
	"leetcode-clone-scalable/internal/common"
	"leetcode-clone-scalable/internal/common/security"
	"leetcode-clone-scalable/internal/domain/model"
	"leetcode-clone-scalable/internal/domain/repository"

	"github.com/google/uuid"
)

type AuthService struct {
	userRepo repository.UserRepository
}

func NewAuthService(userRepo repository.UserRepository) *AuthService {
	return &AuthService{userRepo: userRepo}
}

type SignupRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	LoginField string `json:"login_field"` // Can be username or email
	Password   string `json:"password"`
}

type AuthResponse struct {
	User  *model.User `json:"user"`
	Token string      `json:"token"`
}

func (s *AuthService) Signup(ctx context.Context, req SignupRequest) (*AuthResponse, error) {
	if req.Username == "" || req.Email == "" || req.Password == "" {
		return nil, common.ErrBadRequest
	}
	// Add more validation (email format, password strength etc.)

	hashedPassword, err := security.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	user := &model.User{
		ID:             uuid.NewString(),
		Username:       req.Username,
		Email:          req.Email,
		HashedPassword: hashedPassword,
		Role:           model.RoleUser, // Default role
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		// Repo might return common.ErrConflict
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	token, err := security.GenerateToken(user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	user.HashedPassword = "" // Clear password before returning
	return &AuthResponse{User: user, Token: token}, nil
}

func (s *AuthService) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	if req.LoginField == "" || req.Password == "" {
		return nil, common.ErrBadRequest
	}

	var user *model.User
	var err error

	// Try finding by email first, then by username
	user, err = s.userRepo.FindByEmail(ctx, req.LoginField)
	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			user, err = s.userRepo.FindByUsername(ctx, req.LoginField)
		}
	}

	if err != nil {
		if errors.Is(err, common.ErrNotFound) {
			return nil, common.ErrUnauthorized // Generic message for security
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	if !security.CheckPasswordHash(req.Password, user.HashedPassword) {
		return nil, common.ErrUnauthorized
	}

	token, err := security.GenerateToken(user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}
	user.HashedPassword = ""
	return &AuthResponse{User: user, Token: token}, nil
}
```

**`internal/app/service/problem_service.go` (Sketch)**
```go
package service

import (
	"context"
	"database/sql"
	"fmt"
	"leetcode-clone-scalable/internal/common"
	"leetcode-clone-scalable/internal/domain/model"
	"leetcode-clone-scalable/internal/domain/repository"
	"leetcode-clone-scalable/internal/platform/config"
	"leetcode-clone-scalable/internal/platform/database" // For DB instance
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/gosimple/slug" // For slug generation
)


type ProblemService struct {
	problemRepo    repository.ProblemRepository
	tagRepo        repository.TagRepository // Assuming a separate TagRepository
	execJobService *ExecutionJobService     // For triggering validation
	db             *sql.DB                  // For transactions
}

func NewProblemService(
	problemRepo repository.ProblemRepository, 
	tagRepo repository.TagRepository,
	execJobService *ExecutionJobService,
	db *sql.DB,
) *ProblemService {
	return &ProblemService{
		problemRepo:    problemRepo,
		tagRepo:        tagRepo,
		execJobService: execJobService,
		db:             db,
	}
}

type CreateProblemRequest struct {
	Title              string                `json:"title"`
	Description        string                `json:"description"`
	Difficulty         model.ProblemDifficulty `json:"difficulty"`
	SolutionCode       string                `json:"solution_code"`
	SolutionLanguageID string                `json:"solution_language_id"` // Or slug
	RuntimeLimitMs     int                   `json:"runtime_limit_ms"`
	MemoryLimitKb      int                   `json:"memory_limit_kb"`
	Tags               []string              `json:"tags"` // Tag names or slugs
	Examples           []model.Example       `json:"examples"`
	TestCases          []model.TestCase      `json:"test_cases"` // Hidden ones
}

type UpdateProblemRequest struct {
    // Similar to CreateProblemRequest, but fields are optional
	Title              *string                `json:"title,omitempty"`
	Description        *string                `json:"description,omitempty"`
	Difficulty         *model.ProblemDifficulty `json:"difficulty,omitempty"`
	SolutionCode       *string                `json:"solution_code,omitempty"`
	SolutionLanguageID *string                `json:"solution_language_id,omitempty"`
	RuntimeLimitMs     *int                   `json:"runtime_limit_ms,omitempty"`
	MemoryLimitKb      *int                   `json:"memory_limit_kb,omitempty"`
	Status             *model.ProblemStatus   `json:"status,omitempty"` // Admin can change status
	Tags               *[]string              `json:"tags,omitempty"`
	Examples           *[]model.Example       `json:"examples,omitempty"`
	TestCases          *[]model.TestCase      `json:"test_cases,omitempty"`
}


func (s *ProblemService) CreateProblem(ctx context.Context, userID string, req CreateProblemRequest) (*model.Problem, error) {
	// Validate request
	if req.Title == "" || req.Description == "" || req.Difficulty == "" || req.SolutionCode == "" || req.SolutionLanguageID == "" || len(req.TestCases) == 0 {
		return nil, common.Errorf("missing required fields for problem creation: %w", common.ErrBadRequest)
	}

	// Get actual language ID from slug or ID
	// lang, err := s.problemRepo.GetLanguageBySlugOrID(ctx, req.SolutionLanguageID)
	// For now, assume SolutionLanguageID is actual ID
	// if err != nil { ... }

	problem := &model.Problem{
		ID:                 uuid.NewString(),
		Title:              req.Title,
		Slug:               slug.Make(req.Title), // Ensure uniqueness later or handle conflict
		Description:        req.Description,
		Difficulty:         req.Difficulty,
		Status:             model.StatusPendingValidation, // Initial status
		SolutionCode:       &req.SolutionCode,
		SolutionLanguageID: &req.SolutionLanguageID,
		RuntimeLimitMs:     req.RuntimeLimitMs,
		MemoryLimitKb:      req.MemoryLimitKb,
		CreatedByID:        &userID,
	}
	if problem.RuntimeLimitMs == 0 { problem.RuntimeLimitMs = 2000 } // Default
	if problem.MemoryLimitKb == 0 { problem.MemoryLimitKb = 256000 } // Default


	// Transaction for creating problem and its related entities
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, common.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback if not committed

	if err := s.problemRepo.CreateProblem(ctx, tx, problem); err != nil {
		return nil, common.Errorf("failed to create problem in DB: %w", err)
	}

	// Handle Tags: find or create tags, then link to problem
	tagIDs, err := s.findOrCreateTags(ctx, tx, req.Tags)
	if err != nil {
		return nil, common.Errorf("failed to process tags: %w", err)
	}
	if len(tagIDs) > 0 {
		if err := s.problemRepo.AddTagsToProblem(ctx, tx, problem.ID, tagIDs); err != nil {
			return nil, common.Errorf("failed to add tags to problem: %w", err)
		}
	}
	
	// Add Examples
	for i := range req.Examples { // Ensure examples have UUIDs if not provided
		if req.Examples[i].ID == "" {
			req.Examples[i].ID = uuid.NewString()
		}
	}
	if len(req.Examples) > 0 {
		if err := s.problemRepo.AddExamplesToProblem(ctx, tx, problem.ID, req.Examples); err != nil {
			return nil, common.Errorf("failed to add examples to problem: %w", err)
		}
	}

	// Add Test Cases
	for i := range req.TestCases { // Ensure test cases have UUIDs
		if req.TestCases[i].ID == "" {
			req.TestCases[i].ID = uuid.NewString()
		}
	}
	if err := s.problemRepo.AddTestCasesToProblem(ctx, tx, problem.ID, req.TestCases); err != nil {
		return nil, common.Errorf("failed to add test cases to problem: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, common.Errorf("failed to commit transaction: %w", err)
	}

	// Enqueue for validation
	_, err = s.execJobService.EnqueueProblemValidationJob(ctx, problem.ID)
	if err != nil {
		// Problem created, but validation job failed. Log this.
		log.Printf("ERROR: Failed to enqueue validation job for problem %s: %v", problem.ID, err)
		// Could update problem status to Draft or something to indicate issue.
	}
	problem.Examples = req.Examples // Populate for response
	problem.TestCases = req.TestCases // Populate for response (admin view)
	// Fetch created tags to return them
	// createdTags, _ := s.problemRepo.GetTagsByProblemID(ctx, problem.ID)
	// problem.Tags = createdTags

	return problem, nil
}

// findOrCreateTags is a helper
func (s *ProblemService) findOrCreateTags(ctx context.Context, tx *sql.Tx, tagNamesOrSlugs []string) ([]string, error) {
    if len(tagNamesOrSlugs) == 0 {
        return nil, nil
    }
    var tagIDs []string
    for _, nameOrSlug := range tagNamesOrSlugs {
        tag, err := s.tagRepo.FindBySlugOrName(ctx, tx, nameOrSlug) // TagRepo needs this method
        if err != nil {
            if errors.Is(err, common.ErrNotFound) { // Create if not exists
                newTag := &model.Tag{
                    ID:   uuid.NewString(),
                    Name: nameOrSlug, // Assuming it's a name if not found by slug
                    Slug: slug.Make(nameOrSlug),
                }
                if errCreate := s.tagRepo.Create(ctx, tx, newTag); errCreate != nil {
                    return nil, fmt.Errorf("failed to create tag %s: %w", nameOrSlug, errCreate)
                }
                tag = newTag
            } else {
                return nil, fmt.Errorf("failed to find tag %s: %w", nameOrSlug, err)
            }
        }
        tagIDs = append(tagIDs, tag.ID)
    }
    return tagIDs, nil
}

func (s *ProblemService) GetProblemDetails(ctx context.Context, problemSlug string, userRole string) (*model.Problem, error) {
	problem, err := s.problemRepo.FindProblemBySlug(ctx, problemSlug)
	if err != nil {
		return nil, err // common.ErrNotFound or other errors
	}

	// Filter problem content based on status and user role
	if problem.Status != model.StatusPublished && userRole != model.RoleAdmin {
		return nil, common.ErrNotFound // Treat non-published as not found for regular users
	}

	examples, err := s.problemRepo.GetExamplesByProblemID(ctx, problem.ID)
	if err != nil {
		log.Printf("WARN: Failed to fetch examples for problem %s: %v", problem.ID, err)
		// Continue, but examples will be missing
	}
	problem.Examples = examples

	tags, err := s.problemRepo.GetTagsByProblemID(ctx, problem.ID)
    if err != nil {
        log.Printf("WARN: Failed to fetch tags for problem %s: %v", problem.ID, err)
    }
    problem.Tags = tags

	if userRole == model.RoleAdmin {
		testCases, err := s.problemRepo.GetTestCasesByProblemID(ctx, problem.ID)
		if err != nil {
			log.Printf("WARN: Failed to fetch test cases for problem %s: %v", problem.ID, err)
		}
		problem.TestCases = testCases
	} else {
		// Regular users don't see solution code or hidden test cases
		problem.SolutionCode = nil
		// problem.SolutionLanguageID = nil // maybe keep language ID for display
		problem.TestCases = nil
	}
	return problem, nil
}

func (s *ProblemService) ListProblems(ctx context.Context, page, pageSize int, difficulty model.ProblemDifficulty, tags []string, userRole string) ([]model.Problem, int, error) {
    limit := pageSize
    offset := (page -1) * pageSize
    if offset < 0 { offset = 0 }

    // For regular users, only show Published problems
    statusFilter := model.StatusPublished
    if userRole == model.RoleAdmin {
        statusFilter = "" // Admin can see all statuses, or we can add a status query param
    }

    // Convert tag names/slugs to tag IDs if tagRepo is available
    var tagIDs []string
    if len(tags) > 0 && s.tagRepo != nil {
        for _, tagNameOrSlug := range tags {
            tag, err := s.tagRepo.FindBySlugOrName(ctx, nil, tagNameOrSlug) // nil for tx as it's a read
            if err == nil && tag != nil {
                tagIDs = append(tagIDs, tag.ID)
            } else {
                log.Printf("WARN: Tag '%s' not found, skipping in filter.", tagNameOrSlug)
            }
        }
		if len(tags) > 0 && len(tagIDs) == 0 { // User filtered by tags but none found
			return []model.Problem{}, 0, nil
		}
    }


    problems, total, err := s.problemRepo.ListProblems(ctx, limit, offset, difficulty, tagIDs, statusFilter, "") // Add searchTerm later
    if err != nil {
        return nil, 0, err
    }

    // For each problem, fetch its tags (could be optimized)
    for i := range problems {
        pTags, err := s.problemRepo.GetTagsByProblemID(ctx, problems[i].ID)
        if err == nil {
            problems[i].Tags = pTags
        }
    }
    return problems, total, nil
}

// UpdateProblem is complex, involves checking existing problem, updating fields, re-associating tags/examples/testcases
// ... (UpdateProblem method sketch)
// It would also involve transactions and potentially re-triggering validation if critical fields change.
```

**`internal/app/service/submission_service.go` (Sketch)**
```go
package service

import (
	"context"
	"database/sql"
	"fmt"
	"leetcode-clone-scalable/internal/common"
	"leetcode-clone-scalable/internal/domain/model"
	"leetcode-clone-scalable/internal/domain/repository"
	"leetcode-clone-scalable/internal/platform/config"
	"log"

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
	memoryLimit := config.AppConfig.ProblemValidationDefaultMemoryLimitKb  // Default


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
    if req.RuntimeLimitMs != nil { runtimeLimit = *req.RuntimeLimitMs }
    if req.MemoryLimitKb != nil { memoryLimit = *req.MemoryLimitKb }


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
```

**`internal/app/service/execution_job_service.go` (Central to queuing)**
```go
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"leetcode-clone-scalable/internal/common"
	"leetcode-clone-scalable/internal/domain/model"
	"leetcode-clone-scalable/internal/domain/repository"
	"leetcode-clone-scalable/internal/platform/config"
	"leetcode-clone-scalable/internal/platform/queue" // Redis client
	"log"

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
		ID:      uuid.NewString(),
		JobType: model.JobTypeRunCode,
		Payload: payloadBytes,
		Status:  model.JobStatusQueued,
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
```

**`internal/app/service/webhook_service.go` (Handles results from executor)**
```go
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"leetcode-clone-scalable/internal/common"
	"leetcode-clone-scalable/internal/domain/model"
	"leetcode-clone-scalable/internal/domain/repository"
	"log"
	"time"
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
	JobID             string                         `json:"job_id"` // The ID of our ExecutionJob
	OverallStatus     model.SubmissionStatus         `json:"overall_status"`
	ExecutionTimeMs   *int                           `json:"execution_time_ms,omitempty"` // Overall if applicable
	MemoryKb          *int                           `json:"memory_kb,omitempty"`         // Overall if applicable
	CompilationOutput *string                        `json:"compilation_output,omitempty"`
	ErrorOutput       *string                        `json:"error_output,omitempty"`       // For runtime errors
	TestCaseResults   []TestCaseExecutionResult `json:"test_case_results,omitempty"` // Results for each test case
}

type TestCaseExecutionResult struct {
	Input           string                 `json:"input"` // To match against our test cases if IDs aren't passed back
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
		
		testCaseMap := make(map[string]model.TestCase) // map by input for simple matching
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
	// payload.TestCaseResults can be inspected.

	return nil
}

```

---
**12. Worker (`internal/app/worker/execution_worker.go`)**
---
```go
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"leetcode-clone-scalable/internal/common"
	"leetcode-clone-scalable/internal/domain/model"
	"leetcode-clone-scalable/internal/domain/repository"
	"leetcode-clone-scalable/internal/platform/config"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type ExecutionWorker struct {
	rdb            *redis.Client
	jobRepo        repository.ExecutionJobRepository
	problemRepo    repository.ProblemRepository
	submissionRepo repository.SubmissionRepository
	// db for updating job status within transactions
}

func NewExecutionWorker(rdb *redis.Client, jobRepo repository.ExecutionJobRepository, probRepo repository.ProblemRepository, subRepo repository.SubmissionRepository) *ExecutionWorker {
	return &ExecutionWorker{
		rdb:            rdb,
		jobRepo:        jobRepo,
		problemRepo:    probRepo,
		submissionRepo: subRepo,
	}
}

// ExternalExecutorRequest is the format sent to the (mocked) external service
type ExternalExecutorRequest struct {
	JobID             string            `json:"job_id"` // Our internal ExecutionJob ID
	LanguageSlug      string            `json:"language_slug"`
	Code              string            `json:"code"`
	TestCases         []ExecutorTestCase `json:"test_cases"`
	RuntimeLimitMs    int               `json:"runtime_limit_ms"`
	MemoryLimitKb     int               `json:"memory_limit_kb"`
	WebhookURL        string            `json:"webhook_url"`
	IsValidation      bool              `json:"is_validation"`       // To differentiate problem validation runs
	IsRunCode         bool              `json:"is_run_code"`         // To differentiate simple run code
	RunCodeInputs     []string          `json:"run_code_inputs,omitempty"` // For run_code type
}

type ExecutorTestCase struct {
	ID             string `json:"id"` // Original TestCase.ID for easier mapping on webhook
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"` // Optional for executor, but good for it to have
}

func (w *ExecutionWorker) Start(ctx context.Context) {
	log.Println("Execution worker started, listening to queue:", config.AppConfig.ExecutionQueueName)
	for {
		select {
		case <-ctx.Done():
			log.Println("Execution worker stopping...")
			return
		default:
			// Blocking pop from Redis queue
			jobID, err := w.rdb.BRPop(ctx, 0*time.Second, config.AppConfig.ExecutionQueueName).Result()
			if err != nil {
				if errors.Is(err, redis.Nil) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					// redis.Nil means timeout (0 means infinite, but good to handle)
					// context errors mean worker is shutting down
					log.Printf("Worker BRPop exiting or timed out: %v", err)
					time.Sleep(1 * time.Second) // Avoid busy-looping on certain errors
					continue
				}
				log.Printf("ERROR: Failed to BRPop from Redis queue '%s': %v", config.AppConfig.ExecutionQueueName, err)
				time.Sleep(5 * time.Second) // Wait before retrying on other errors
				continue
			}
			
			// jobID is an array: [queueName, value]
			if len(jobID) < 2 || jobID[1] == "" {
				log.Println("WARN: BRPop returned empty job ID.")
				continue
			}
			actualJobID := jobID[1]
			log.Printf("Worker picked up job ID: %s", actualJobID)

			// Process the job in a separate goroutine to not block the BRPop loop for long.
			// However, the requirement is "Only 1 job can run at a time".
			// So, we must process synchronously here after acquiring a lock.
			w.processJobWithLock(ctx, actualJobID)
		}
	}
}

func (w *ExecutionWorker) processJobWithLock(ctx context.Context, jobID string) {
	// 1. Acquire Distributed Lock
	lockValue := uuid.NewString() // Unique value for this lock instance
	lockTTL := time.Duration(config.AppConfig.ExecutionLockTTLSeconds) * time.Second
	
	// Try to acquire the lock
	// SET key value NX PX milliseconds
	ok, err := w.rdb.SetNX(ctx, config.AppConfig.ExecutionLockKey, lockValue, lockTTL).Result()
	if err != nil {
		log.Printf("ERROR: Failed to attempt lock acquisition for job %s: %v", jobID, err)
		// Re-queue the job? Or just let it be picked up later.
		// For now, log and skip. A robust system might re-queue with backoff.
		w.requeueJob(ctx, jobID) // Potentially requeue
		return
	}
	if !ok {
		log.Printf("INFO: Could not acquire execution lock for job %s, another worker is busy. Re-queueing.", jobID)
		w.requeueJob(ctx, jobID) // Re-queue job
		return
	}
	log.Printf("INFO: Acquired execution lock for job %s (lock value: %s)", jobID, lockValue)

	// Defer lock release
	defer func() {
		// Release lock only if we still hold it (check value)
		// Use Lua script for CAS delete:
		// if redis.call("get",KEYS[1]) == ARGV[1] then
		//     return redis.call("del",KEYS[1])
		// else
		//     return 0
		// end
		script := redis.NewScript(`
            if redis.call("get", KEYS[1]) == ARGV[1] then
                return redis.call("del", KEYS[1])
            else
                return 0
            end
        `)
		deleted, err := script.Run(ctx, w.rdb, []string{config.AppConfig.ExecutionLockKey}, lockValue).Result()
		if err != nil {
			log.Printf("ERROR: Failed to release lock for key %s (job %s): %v", config.AppConfig.ExecutionLockKey, jobID, err)
		} else if deleted.(int64) == 1 {
			log.Printf("INFO: Released execution lock for job %s", jobID)
		} else {
			log.Printf("WARN: Did not release lock for job %s; it might have expired or been taken by another.", jobID)
		}
	}()
	
	// 2. Process the job (now that lock is acquired)
	w.handleJobProcessing(ctx, jobID)
}

func (w *ExecutionWorker) requeueJob(ctx context.Context, jobID string) {
    // Re-queue to the head of the list for immediate retry, or tail for later.
    // For simplicity, push to tail. Consider a dead-letter queue after N retries.
    if err := w.rdb.RPush(ctx, config.AppConfig.ExecutionQueueName, jobID).Err(); err != nil {
        log.Printf("ERROR: Failed to re-queue job %s: %v", jobID, err)
    } else {
        log.Printf("INFO: Job %s re-queued.", jobID)
    }
}


func (w *ExecutionWorker) handleJobProcessing(ctx context.Context, jobID string) {
	// Fetch job details from DB
	job, err := w.jobRepo.GetJobByID(ctx, jobID)
	if err != nil {
		log.Printf("ERROR: Failed to fetch job %s from DB: %v", jobID, err)
		// Cannot proceed without job details. This job might be stuck.
		// A monitoring system should catch this.
		return
	}

	// Update job status to Processing
	if err := w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusProcessing, nil); err != nil { // nil for tx
		log.Printf("ERROR: Failed to update job %s status to Processing: %v", job.ID, err)
		// Continue processing, but status might be stale.
	}

	var externalReq ExternalExecutorRequest
	externalReq.JobID = job.ID // Pass our job ID to executor
	externalReq.WebhookURL = config.AppConfig.MockExecutorWebhookURL

	switch job.JobType {
	case model.JobTypeSubmissionEvaluation:
		if job.SubmissionID == nil {
			errMsg := "SubmissionID is nil for submission evaluation job"
			log.Printf("ERROR: %s (Job ID: %s)", errMsg, job.ID)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		submission, err := w.submissionRepo.GetSubmissionByID(ctx, *job.SubmissionID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to fetch submission %s: %v", *job.SubmissionID, err)
			log.Printf("ERROR: %s (Job ID: %s)", errMsg, job.ID)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		problem, err := w.problemRepo.FindProblemByID(ctx, submission.ProblemID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to fetch problem %s for submission %s: %v", submission.ProblemID, submission.ID, err)
			log.Printf("ERROR: %s (Job ID: %s)", errMsg, job.ID)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		language, err := w.problemRepo.GetLanguageByID(ctx, submission.LanguageID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to fetch language %s: %v", submission.LanguageID, err)
			log.Printf("ERROR: %s (Job ID: %s)", errMsg, job.ID)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		
		testCases, err := w.problemRepo.GetTestCasesByProblemID(ctx, problem.ID)
		if err != nil || len(testCases) == 0 {
			errMsg := fmt.Sprintf("Failed to fetch test cases for problem %s or no test cases found: %v", problem.ID, err)
			log.Printf("ERROR: %s (Job ID: %s)", errMsg, job.ID)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			// Potentially update submission to SystemError
			sErr := w.submissionRepo.UpdateSubmissionStatus(ctx, nil, submission.ID, model.StatusSystemError, nil, nil)
			if sErr != nil { log.Printf("ERROR: Failed to update submission status to SystemError for %s: %v", submission.ID, sErr)}
			return
		}

		externalReq.LanguageSlug = language.Slug
		externalReq.Code = submission.Code
		externalReq.RuntimeLimitMs = problem.RuntimeLimitMs
		externalReq.MemoryLimitKb = problem.MemoryLimitKb
		for _, tc := range testCases {
			externalReq.TestCases = append(externalReq.TestCases, ExecutorTestCase{ID: tc.ID, Input: tc.Input, ExpectedOutput: tc.ExpectedOutput})
		}
		externalReq.IsValidation = false
		externalReq.IsRunCode = false

	case model.JobTypeProblemValidation:
		if job.ProblemID == nil {
			errMsg := "ProblemID is nil for problem validation job"
			log.Printf("ERROR: %s (Job ID: %s)", errMsg, job.ID)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		problem, err := w.problemRepo.FindProblemByID(ctx, *job.ProblemID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to fetch problem %s for validation: %v", *job.ProblemID, err)
			log.Printf("ERROR: %s (Job ID: %s)", errMsg, job.ID)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		if problem.SolutionCode == nil || problem.SolutionLanguageID == nil {
			errMsg := fmt.Sprintf("Problem %s has no solution code or language for validation", problem.ID)
			log.Printf("ERROR: %s (Job ID: %s)", errMsg, job.ID)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		language, err := w.problemRepo.GetLanguageByID(ctx, *problem.SolutionLanguageID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to fetch solution language %s for problem %s: %v", *problem.SolutionLanguageID, problem.ID, err)
			log.Printf("ERROR: %s (Job ID: %s)", errMsg, job.ID)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		testCases, err := w.problemRepo.GetTestCasesByProblemID(ctx, problem.ID)
		if err != nil || len(testCases) == 0 {
			errMsg := fmt.Sprintf("Failed to fetch test cases for problem %s validation or no test cases found: %v", problem.ID, err)
			log.Printf("ERROR: %s (Job ID: %s)", errMsg, job.ID)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		externalReq.LanguageSlug = language.Slug
		externalReq.Code = *problem.SolutionCode
		externalReq.RuntimeLimitMs = problem.RuntimeLimitMs
		externalReq.MemoryLimitKb = problem.MemoryLimitKb
		for _, tc := range testCases {
			externalReq.TestCases = append(externalReq.TestCases, ExecutorTestCase{ID: tc.ID, Input: tc.Input, ExpectedOutput: tc.ExpectedOutput})
		}
		externalReq.IsValidation = true
		externalReq.IsRunCode = false

	case model.JobTypeRunCode:
		var payload model.RunCodePayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			errMsg := fmt.Sprintf("Failed to unmarshal RunCodePayload for job %s: %v", job.ID, err)
			log.Printf("ERROR: %s", errMsg)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		language, err := w.problemRepo.GetLanguageByID(ctx, payload.LanguageID)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to fetch language %s for run_code job %s: %v", payload.LanguageID, job.ID, err)
			log.Printf("ERROR: %s", errMsg)
			w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
			return
		}
		externalReq.LanguageSlug = language.Slug
		externalReq.Code = payload.Code
		externalReq.RuntimeLimitMs = payload.RuntimeLimitMs
		externalReq.MemoryLimitKb = payload.MemoryLimitKb
		externalReq.RunCodeInputs = payload.Inputs // Send custom inputs to executor
		externalReq.IsValidation = false
		externalReq.IsRunCode = true
		// For run_code, test cases are effectively the inputs themselves.
		// The executor might handle this differently, or we adapt the ExecutorTestCase.
		// For simplicity, let's assume executor can take raw inputs for run_code.
		// If not, we'd create dummy ExecutorTestCases from RunCodeInputs.
		// For this example, let's send them as `RunCodeInputs` and leave `TestCases` empty.
		// Alternatively, if executor always expects TestCases:
		/*
		for i, inputStr := range payload.Inputs {
			externalReq.TestCases = append(externalReq.TestCases, ExecutorTestCase{
				ID: "runcode_input_"+strconv.Itoa(i), // Dummy ID
				Input: inputStr,
				// ExpectedOutput is not relevant for user-provided run code inputs usually
			})
		}
		*/


	default:
		errMsg := fmt.Sprintf("Unknown job type '%s' for job ID %s", job.JobType, job.ID)
		log.Printf("ERROR: %s", errMsg)
		w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg)
		return
	}

	// Send to external executor (mocked)
	if err := w.sendToExternalExecutor(ctx, externalReq); err != nil {
		errMsg := fmt.Sprintf("Failed to send job %s to external executor: %v", job.ID, err)
		log.Printf("ERROR: %s", errMsg)
		w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusFailed, &errMsg) // Or a retry status
		// If it's a submission, update its status to SystemError
		if job.JobType == model.JobTypeSubmissionEvaluation && job.SubmissionID != nil {
			sErr := w.submissionRepo.UpdateSubmissionStatus(ctx, nil, *job.SubmissionID, model.StatusSystemError, nil, nil)
			if sErr != nil { log.Printf("ERROR: Failed to update submission status to SystemError for %s: %v", *job.SubmissionID, sErr)}
		}
		return
	}

	// Update job status to SentToExecutor
	if err := w.jobRepo.UpdateJobStatus(ctx, nil, job.ID, model.JobStatusSentToExecutor, nil); err != nil {
		log.Printf("ERROR: Failed to update job %s status to SentToExecutor: %v", job.ID, err)
	}
	log.Printf("INFO: Job %s (type: %s) successfully sent to external executor.", job.ID, job.JobType)
}


func (w *ExecutionWorker) sendToExternalExecutor(ctx context.Context, req ExternalExecutorRequest) error {
	// MOCK IMPLEMENTATION: Log and simulate a successful send.
	// In a real system, this would be an HTTP POST to the executor service.
	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request for external executor: %w", err)
	}

	log.Printf("INFO: [WORKER] Mock sending to external executor: URL: %s, Payload: %s", "http://fake-executor.com/execute", string(jsonData))
	
	// For testing the webhook, we can make a request to our own webhook endpoint
	// This should be done carefully and ideally replaced by a true external call.
	// This simulates the external executor calling back.
	go func() {
		time.Sleep(3 * time.Second) // Simulate execution time

		// Construct a mock ExecutionResultPayload based on what was sent
		mockResult := service.ExecutionResultPayload{
			JobID: req.JobID,
			// Populate other fields based on a mock successful or failed execution
		}

		if req.IsRunCode {
			mockResult.OverallStatus = model.StatusAccepted // Or based on some dummy logic
			mockResult.ExecutionTimeMs = func() *int { t := 50; return &t }()
			mockResult.MemoryKb = func() *int { m := 10240; return &m }()
			for i, input := range req.RunCodeInputs {
				mockResult.TestCaseResults = append(mockResult.TestCaseResults, service.TestCaseExecutionResult{
					Input: input,
					TestCaseID: func() *string { s := "runcode_input_"+strconv.Itoa(i); return &s }(),
					Status: model.StatusAccepted,
					ActualOutput: func() *string { s := "Mock output for " + input; return &s }(),
					ExecutionTimeMs: func() *int { t := 50; return &t }(),
					MemoryKb: func() *int { m := 10240; return &m }(),
				})
			}
		} else { // For submissions and validations
			// Simulate a simple pass/fail for all test cases
			allPassed := true // Change this to false to simulate failure
			mockResult.OverallStatus = model.StatusAccepted
			if !allPassed {
				mockResult.OverallStatus = model.StatusWrongAnswer
			}

			totalTime := 0
			maxMemory := 0

			for _, tc := range req.TestCases {
				tcStatus := model.StatusAccepted
				if !allPassed {
					tcStatus = model.StatusWrongAnswer // Make first one fail for example
					allPassed = true // only fail one for simple demo
				}
				tcTime := 50 // ms
				tcMem := 10240 // kb
				totalTime += tcTime
				if tcMem > maxMemory { maxMemory = tcMem }

				mockResult.TestCaseResults = append(mockResult.TestCaseResults, service.TestCaseExecutionResult{
					TestCaseID: &tc.ID,
					Input: tc.Input,
					ActualOutput: &tc.ExpectedOutput, // Assume correct for accepted
					Status: tcStatus,
					ExecutionTimeMs: &tcTime,
					MemoryKb: &tcMem,
				})
			}
			mockResult.ExecutionTimeMs = &totalTime
			mockResult.MemoryKb = &maxMemory
		}


		resultBytes, _ := json.Marshal(mockResult)
		
		// Post to our own webhook
		httpReq, err := http.NewRequestWithContext(context.Background(), "POST", config.AppConfig.MockExecutorWebhookURL, bytes.NewBuffer(resultBytes))
		if err != nil {
			log.Printf("ERROR: [WORKER-SIMULATION] Failed to create webhook request for job %s: %v", req.JobID, err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		// Add shared secret for webhook auth if implemented
		// httpReq.Header.Set("X-Webhook-Secret", "your-webhook-secret")

		client := http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(httpReq)
		if err != nil {
			log.Printf("ERROR: [WORKER-SIMULATION] Failed to call webhook for job %s: %v", req.JobID, err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("ERROR: [WORKER-SIMULATION] Webhook call for job %s returned status %d", req.JobID, resp.StatusCode)
		} else {
			log.Printf("INFO: [WORKER-SIMULATION] Successfully simulated webhook callback for job %s.", req.JobID)
		}
	}()


	return nil // Mocked: always successful send
}
```

---
**13. API Layer (`internal/api/*`)**
---
*(Middleware, Handlers, Router. Showing key parts.)*

**`internal/api/middleware/auth.go`**
```go
package middleware

import (
	"context"
	"leetcode-clone-scalable/internal/common"
	"leetcode-clone-scalable/internal/common/security"
	"leetcode-clone-scalable/internal/domain/model"
	"net/http"
	"strings"

	"github.com/go-chi/jwtauth/v5"
)

type contextKey string

const (
	UserIDCtxKey contextKey = "userID"
	UserRoleCtxKey contextKey = "userRole"
)

func Authenticator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, claims, err := jwtauth.FromContext(r.Context()) // Extracts token from Authorization header

		if err != nil {
			if strings.Contains(err.Error(), "token not found") || token == nil {
				common.RespondWithError(w, http.StatusUnauthorized, "Authorization token required")
			} else {
				common.RespondWithError(w, http.StatusUnauthorized, "Invalid token: "+err.Error())
			}
			return
		}

		if token == nil || !token.Valid {
			common.RespondWithError(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		userID, err := security.GetUserIDFromClaims(claims)
		if err != nil {
			common.RespondWithError(w, http.StatusUnauthorized, "Invalid token claims: "+err.Error())
			return
		}
		userRole, err := security.GetUserRoleFromClaims(claims)
		if err != nil {
			common.RespondWithError(w, http.StatusUnauthorized, "Invalid token claims: "+err.Error())
			return
		}

		ctx := context.WithValue(r.Context(), UserIDCtxKey, userID)
		ctx = context.WithValue(ctx, UserRoleCtxKey, userRole)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func AdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, ok := r.Context().Value(UserRoleCtxKey).(string)
		if !ok || role != model.RoleAdmin {
			common.RespondWithError(w, http.StatusForbidden, "Admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Helper to get user ID from context
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDCtxKey).(string)
	return userID, ok
}

// Helper to get user role from context
func GetUserRoleFromContext(ctx context.Context) (string, bool) {
	userRole, ok := ctx.Value(UserRoleCtxKey).(string)
	return userRole, ok
}
```

**`internal/api/handler/auth_handler.go`**
```go
package handler

import (
	"encoding/json"
	"leetcode-clone-scalable/internal/app/service"
	"leetcode-clone-scalable/internal/common"
	"net/http"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Post("/signup", h.signup)
	r.Post("/login", h.login)
}

func (h *AuthHandler) signup(w http.ResponseWriter, r *http.Request) {
	var req service.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.RespondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}

	resp, err := h.authService.Signup(r.Context(), req)
	if err != nil {
		common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
		return
	}
	common.RespondWithJSON(w, http.StatusCreated, resp)
}

func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.RespondWithError(w, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}
	resp, err := h.authService.Login(r.Context(), req)
	if err != nil {
		common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
		return
	}
	common.RespondWithJSON(w, http.StatusOK, resp)
}
```

**`internal/api/handler/problem_handler.go` (Sketch)**
```go
package handler

// ... imports ...
import (
	"encoding/json"
	"leetcode-clone-scalable/internal/api/middleware"
	"leetcode-clone-scalable/internal/app/service"
	"leetcode-clone-scalable/internal/common"
	"leetcode-clone-scalable/internal/domain/model"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)


type ProblemHandler struct {
	problemService *service.ProblemService
}

func NewProblemHandler(ps *service.ProblemService) *ProblemHandler {
	return &ProblemHandler{problemService: ps}
}

func (h *ProblemHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.listProblems) // GET /api/v1/problems
	r.Get("/{problemSlug}", h.getProblem) // GET /api/v1/problems/two-sum

	r.Group(func(adminRouter chi.Router) {
		adminRouter.Use(middleware.Authenticator)
		adminRouter.Use(middleware.AdminOnly)
		adminRouter.Post("/", h.createProblem)         // POST /api/v1/problems
		adminRouter.Put("/{problemID}", h.updateProblem) // PUT /api/v1/problems/{id}
		// Add routes for managing examples, test cases, etc. for a problem by ID
	})
}

func (h *ProblemHandler) createProblem(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
    if !ok {
        common.RespondWithError(w, http.StatusUnauthorized, "Missing user context")
        return
    }

	var req service.CreateProblemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.RespondWithError(w, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	problem, err := h.problemService.CreateProblem(r.Context(), userID, req)
	if err != nil {
		common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
		return
	}
	common.RespondWithJSON(w, http.StatusCreated, problem)
}

func (h *ProblemHandler) listProblems(w http.ResponseWriter, r *http.Request) {
    pageStr := r.URL.Query().Get("page")
    pageSizeStr := r.URL.Query().Get("pageSize")
    difficultyStr := r.URL.Query().Get("difficulty")
    tagsStr := r.URL.Query().Get("tags") // comma-separated list of tag slugs/names

    page, _ := strconv.Atoi(pageStr)
    if page <= 0 { page = 1 }
    pageSize, _ := strconv.Atoi(pageSizeStr)
    if pageSize <= 0 || pageSize > 100 { pageSize = 20 }

    var tagsFilter []string
    if tagsStr != "" {
        tagsFilter = strings.Split(tagsStr, ",")
    }
    
    userRole, _ := middleware.GetUserRoleFromContext(r.Context()) // Role might be empty if not authenticated for this public route

    problems, total, err := h.problemService.ListProblems(r.Context(), page, pageSize, model.ProblemDifficulty(difficultyStr), tagsFilter, userRole)
    if err != nil {
        common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
        return
    }

    // Paginated response structure
    type PaginatedProblemsResponse struct {
        Problems []model.Problem `json:"problems"`
        Total    int             `json:"total"`
        Page     int             `json:"page"`
        PageSize int             `json:"page_size"`
    }
    common.RespondWithJSON(w, http.StatusOK, PaginatedProblemsResponse{
        Problems: problems,
        Total:    total,
        Page:     page,
        PageSize: pageSize,
    })
}

func (h *ProblemHandler) getProblem(w http.ResponseWriter, r *http.Request) {
    problemSlug := chi.URLParam(r, "problemSlug")
    userRole, _ := middleware.GetUserRoleFromContext(r.Context())

    problem, err := h.problemService.GetProblemDetails(r.Context(), problemSlug, userRole)
    if err != nil {
        common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
        return
    }
    common.RespondWithJSON(w, http.StatusOK, problem)
}

// updateProblem handler
// ...
```

**`internal/api/handler/submission_handler.go` (Sketch)**
```go
package handler
// ... imports ...
type SubmissionHandler struct {
	submissionService *service.SubmissionService
}
func NewSubmissionHandler(ss *service.SubmissionService) *SubmissionHandler {
	return &SubmissionHandler{submissionService: ss}
}
func (h *SubmissionHandler) RegisterRoutes(r chi.Router) {
	r.Use(middleware.Authenticator) // All submission routes require auth
	r.Post("/", h.createSubmission)
	r.Post("/run", h.runCode)
	// GET /me (my submissions)
	// GET /{submissionID}
	// GET /problem/{problemID}/history (user's history for a problem)
	// GET /{submissionID1}/diff/{submissionID2}
}

func (h *SubmissionHandler) createSubmission(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok { /* ... error ... */ return }

	var req service.CreateSubmissionRequest
	// ... decode ... error ...
	
	submission, err := h.submissionService.CreateSubmission(r.Context(), userID, req)
	// ... error handling ...
	common.RespondWithJSON(w, http.StatusAccepted, submission) // Accepted (202) as it's async
}

func (h *SubmissionHandler) runCode(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok { /* ... error ... */ return }

	var req service.RunCodeRequest
	// ... decode ... error ...

	jobID, err := h.submissionService.RunCode(r.Context(), userID, req)
	// ... error handling ...
	common.RespondWithJSON(w, http.StatusAccepted, map[string]string{"job_id": jobID})
}
// ... other submission handlers ...
```

**`internal/api/handler/webhook_handler.go`**
```go
package handler

import (
	"encoding/json"
	"leetcode-clone-scalable/internal/app/service"
	"leetcode-clone-scalable/internal/common"
	"log"
	"net/http"
)

type WebhookHandler struct {
	webhookService *service.WebhookService
}

func NewWebhookHandler(ws *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{webhookService: ws}
}

func (h *WebhookHandler) RegisterRoutes(r chi.Router) {
	// This endpoint should be secured, e.g., with a shared secret in a header
	// or by checking the source IP of the executor service.
	r.Post("/execution", h.handleExecutionResult)
}

func (h *WebhookHandler) handleExecutionResult(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement webhook security (e.g., X-Webhook-Secret header check)
	// secret := r.Header.Get("X-Webhook-Secret")
	// if secret != config.AppConfig.WebhookSecret {
	//    common.RespondWithError(w, http.StatusUnauthorized, "Invalid webhook secret")
	//    return
	// }

	var payload service.ExecutionResultPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Printf("ERROR: Webhook: Invalid payload: %v", err)
		common.RespondWithError(w, http.StatusBadRequest, "Invalid webhook payload")
		return
	}
	defer r.Body.Close()

	if err := h.webhookService.HandleExecutionResult(r.Context(), payload); err != nil {
		log.Printf("ERROR: Webhook: Error handling result for JobID %s: %v", payload.JobID, err)
		common.RespondWithError(w, common.HTTPStatusFromError(err), err.Error())
		return
	}
	common.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Webhook processed for job " + payload.JobID})
}
```


**`internal/api/router.go`**
```go
package api

import (
	"leetcode-clone-scalable/internal/api/handler"
	"leetcode-clone-scalable/internal/api/middleware"
	"leetcode-clone-scalable/internal/app/service"
	"leetcode-clone-scalable/internal/common/security"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/jwtauth/v5"
)

func NewRouter(
	authService *service.AuthService,
	problemService *service.ProblemService,
	submissionService *service.SubmissionService,
	webhookService *service.WebhookService,
	// ... other services
) http.Handler {
	r := chi.NewRouter()

	// Base Middlewares
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Logger) // Chi's logger
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(60 * time.Second))
	// CORS middleware if frontend is on a different domain
	// r.Use(cors.Handler(cors.Options{...}))

	// JWT Auth Middleware Setup
	// This makes jwtauth.Verifier and jwtauth.Authenticator work with the token found in context.
	// It will search for a token in "Authorization: Bearer T".
	r.Use(jwtauth.Verifier(security.TokenAuth)) // Verifies token, puts claims in context

	// Public health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// API v1 Routes
	r.Route("/api/v1", func(v1 chi.Router) {
		// Auth routes (public)
		authHandler := handler.NewAuthHandler(authService)
		v1.Group(func(publicAuth chi.Router) {
			authHandler.RegisterRoutes(publicAuth)
		})

		// Problem routes (some public, some admin)
		problemHandler := handler.NewProblemHandler(problemService)
		v1.Route("/problems", problemHandler.RegisterRoutes)
		
		// Submission routes (authenticated)
		submissionHandler := handler.NewSubmissionHandler(submissionService)
		v1.Route("/submissions", submissionHandler.RegisterRoutes)

		// Webhook routes (public, but should be secured)
		webhookHandler := handler.NewWebhookHandler(webhookService)
		v1.Route("/webhook", webhookHandler.RegisterRoutes)
		
		// Leaderboard routes (public)
		// leaderboardHandler := handler.NewLeaderboardHandler(leaderboardService)
		// v1.Route("/leaderboard", leaderboardHandler.RegisterRoutes)
	})

	return r
}
```

---
**14. Main Application (`cmd/server/main.go`)**
---
```go
package main

import (
	"context"
	"fmt"
	"leetcode-clone-scalable/internal/api"
	"leetcode-clone-scalable/internal/app/service"
	"leetcode-clone-scalable/internal/app/worker"
	"leetcode-clone-scalable/internal/common/security"
	"leetcode-clone-scalable/internal/domain/repository"
	"leetcode-clone-scalable/internal/platform/config"
	"leetcode-clone-scalable/internal/platform/database"
	"leetcode-clone-scalable/internal/platform/queue"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 1. Load Configuration
	config.Load()
	fmt.Println("Configuration loaded.")

	// 2. Initialize JWT
	security.InitJWT()
	fmt.Println("JWT initialized.")

	// 3. Initialize Database
	database.Connect()
	defer database.Close()
	fmt.Println("Database connected.")

	// 4. Initialize Redis
	queue.ConnectRedis()
	defer queue.CloseRedis()
	fmt.Println("Redis connected.")

	// 5. Initialize Repositories
	userRepo := repository.NewPgUserRepository(database.DB)
	problemRepo := repository.NewPgProblemRepository(database.DB) // Implement fully
	tagRepo := repository.NewPgTagRepository(database.DB)         // Implement fully
	submissionRepo := repository.NewPgSubmissionRepository(database.DB) // Implement fully
	execJobRepo := repository.NewPgExecutionJobRepository(database.DB) // Implement fully
	// ... other repos

	// 6. Initialize Services
	authService := service.NewAuthService(userRepo)
	execJobService := service.NewExecutionJobService(execJobRepo, submissionRepo, queue.RDB, database.DB)
	problemService := service.NewProblemService(problemRepo, tagRepo, execJobService, database.DB)
	submissionService := service.NewSubmissionService(submissionRepo, problemRepo, execJobService, database.DB)
	webhookService := service.NewWebhookService(submissionRepo, problemRepo, execJobRepo, database.DB)
	// ... other services

	// 7. Initialize Execution Worker (as a goroutine)
	executionWorker := worker.NewExecutionWorker(queue.RDB, execJobRepo, problemRepo, submissionRepo)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go executionWorker.Start(workerCtx)
	fmt.Println("Execution worker started.")

	// 8. Initialize Router & HTTP Server
	router := api.NewRouter(authService, problemService, submissionService, webhookService /* ... other services */)
	
	server := &http.Server{
		Addr:    ":" + config.AppConfig.APIPort,
		Handler: router,
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout: 120 * time.Second,
	}

	// 9. Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server starting on port %s", config.AppConfig.APIPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Could not listen on %s: %v\n", config.AppConfig.APIPort, err)
		}
	}()
	log.Println("Server started successfully.")

	<-stop // Wait for interrupt signal

	log.Println("Shutting down server...")
	workerCancel() // Signal worker to stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server and worker stopped gracefully.")
}
```

---
**15. `README.md` (Setup Instructions - Brief)**
---
```markdown
# Scalable LeetCode Clone Backend

## Setup

1.  **Prerequisites:**
    *   Go (version 1.18+)
    *   PostgreSQL
    *   Redis

2.  **Configuration:**
    *   Copy `.env.example` to `.env`.
    *   Update `.env` with your PostgreSQL and Redis connection details, JWT secret, etc.
    ```bash
    cp .env.example .env
    # Edit .env
    ```

3.  **Database Setup:**
    *   Create a PostgreSQL database (e.g., `leetcode_clone_db`).
    *   Apply the schema:
        ```bash
        psql -U youruser -d yourdbname -a -f migrations/001_initial_schema.sql
        ```
        (Replace `youruser` and `yourdbname` accordingly)

4.  **Install Dependencies:**
    ```bash
    go mod tidy
    ```

5.  **Run the Application:**
    ```bash
    go run cmd/server/main.go
    ```
    The server will start (default: `http://localhost:8080`). The execution worker also starts as a goroutine within this process.

## API Endpoints

*   (To be documented: Key endpoints for auth, problems, submissions, admin, webhook)

## Notes

*   The code execution is mocked. The worker logs what it would send to an external executor and simulates a callback to the webhook.
*   Leaderboard, code history/diff features are sketched but may require further implementation.
```

---

**Final Thoughts & Next Steps:**

This is a massive codebase, and while I've tried to cover the core structure and requirements, many repository methods and service logic details are sketched or simplified for brevity.

*   **Complete Repository Implementations:** All `// ... (implement ...)` comments in repository files need full pg implementations.
*   **Complete Service Logic:** Flesh out all service methods (e.g., `UpdateProblem`, complex leaderboard calculations, code diff).
*   **Input Validation:** Add thorough input validation in handlers/services (e.g., using a library like `go-playground/validator`).
*   **Robust Error Handling:** Refine error types and ensure consistent error responses.
*   **Testing:** Crucial for a real application. Unit tests for services/repos, integration tests for API endpoints.
*   **Real External Executor:** Integrate with an actual code execution sandbox.
*   **Webhook Security:** Implement robust webhook authentication.
*   **Scalability Considerations:**
    *   The worker is currently a single goroutine. For true scalability, it could be a separate deployable service, and multiple instances could run, relying on the Redis distributed lock.
    *   Database connection pooling is basic; tune it for production.
    *   Consider read replicas for the database for read-heavy operations like listing problems or leaderboards.
*   **Full API Documentation:** Use OpenAPI/Swagger.

This provides a very strong architectural foundation to build upon.