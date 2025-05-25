package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
	"tle_zone_v2/internal/app/service"
	"tle_zone_v2/internal/domain/model"
	"tle_zone_v2/internal/domain/repository"
	"tle_zone_v2/internal/platform/config"

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
	JobID          string             `json:"job_id"` // Our internal ExecutionJob ID
	LanguageSlug   string             `json:"language_slug"`
	Code           string             `json:"code"`
	TestCases      []ExecutorTestCase `json:"test_cases"`
	RuntimeLimitMs int                `json:"runtime_limit_ms"`
	MemoryLimitKb  int                `json:"memory_limit_kb"`
	WebhookURL     string             `json:"webhook_url"`
	IsValidation   bool               `json:"is_validation"`             // To differentiate problem validation runs
	IsRunCode      bool               `json:"is_run_code"`               // To differentiate simple run code
	RunCodeInputs  []string           `json:"run_code_inputs,omitempty"` // For run_code type
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
			if sErr != nil {
				log.Printf("ERROR: Failed to update submission status to SystemError for %s: %v", submission.ID, sErr)
			}
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
			if sErr != nil {
				log.Printf("ERROR: Failed to update submission status to SystemError for %s: %v", *job.SubmissionID, sErr)
			}
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
					Input:           input,
					TestCaseID:      func() *string { s := "runcode_input_" + strconv.Itoa(i); return &s }(),
					Status:          model.StatusAccepted,
					ActualOutput:    func() *string { s := "Mock output for " + input; return &s }(),
					ExecutionTimeMs: func() *int { t := 50; return &t }(),
					MemoryKb:        func() *int { m := 10240; return &m }(),
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
					allPassed = true                   // only fail one for simple demo
				}
				tcTime := 50   // ms
				tcMem := 10240 // kb
				totalTime += tcTime
				if tcMem > maxMemory {
					maxMemory = tcMem
				}

				mockResult.TestCaseResults = append(mockResult.TestCaseResults, service.TestCaseExecutionResult{
					TestCaseID:      &tc.ID,
					Input:           tc.Input,
					ActualOutput:    &tc.ExpectedOutput, // Assume correct for accepted
					Status:          tcStatus,
					ExecutionTimeMs: &tcTime,
					MemoryKb:        &tcMem,
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
