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