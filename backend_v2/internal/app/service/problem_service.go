package service

import (
	"context"
	"database/sql"
	"log"
	"tle_zone_v2/internal/common"
	"tle_zone_v2/internal/domain/model"
	"tle_zone_v2/internal/domain/repository"

	"github.com/google/uuid"
	"github.com/gosimple/slug" // For slug generation
)

type ProblemService struct {
	problemRepo repository.ProblemRepository
	// tagRepo        repository.TagRepository // Assuming a separate TagRepository
	execJobService *ExecutionJobService // For triggering validation
	db             *sql.DB              // For transactions
}

func NewProblemService(
	problemRepo repository.ProblemRepository,
	// tagRepo repository.TagRepository,
	execJobService *ExecutionJobService,
	db *sql.DB,
) *ProblemService {
	return &ProblemService{
		problemRepo: problemRepo,
		// tagRepo:        tagRepo,
		execJobService: execJobService,
		db:             db,
	}
}

type CreateProblemRequest struct {
	Title              string                  `json:"title"`
	Description        string                  `json:"description"`
	Difficulty         model.ProblemDifficulty `json:"difficulty"`
	SolutionCode       string                  `json:"solution_code"`
	SolutionLanguageID string                  `json:"solution_language_id"` // Or slug
	RuntimeLimitMs     int                     `json:"runtime_limit_ms"`
	MemoryLimitKb      int                     `json:"memory_limit_kb"`
	Tags               []string                `json:"tags"` // Tag names or slugs
	Examples           []model.Example         `json:"examples"`
	TestCases          []model.TestCase        `json:"test_cases"` // Hidden ones
}

type UpdateProblemRequest struct {
	// Similar to CreateProblemRequest, but fields are optional
	Title              *string                  `json:"title,omitempty"`
	Description        *string                  `json:"description,omitempty"`
	Difficulty         *model.ProblemDifficulty `json:"difficulty,omitempty"`
	SolutionCode       *string                  `json:"solution_code,omitempty"`
	SolutionLanguageID *string                  `json:"solution_language_id,omitempty"`
	RuntimeLimitMs     *int                     `json:"runtime_limit_ms,omitempty"`
	MemoryLimitKb      *int                     `json:"memory_limit_kb,omitempty"`
	Status             *model.ProblemStatus     `json:"status,omitempty"` // Admin can change status
	Tags               *[]string                `json:"tags,omitempty"`
	Examples           *[]model.Example         `json:"examples,omitempty"`
	TestCases          *[]model.TestCase        `json:"test_cases,omitempty"`
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
	if problem.RuntimeLimitMs == 0 {
		problem.RuntimeLimitMs = 2000
	} // Default
	if problem.MemoryLimitKb == 0 {
		problem.MemoryLimitKb = 256000
	} // Default

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
	problem.Examples = req.Examples   // Populate for response
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
	// for _, nameOrSlug := range tagNamesOrSlugs {
	// 	tag, err := s.tagRepo.FindBySlugOrName(ctx, tx, nameOrSlug) // TagRepo needs this method
	// 	if err != nil {
	// 		if errors.Is(err, common.ErrNotFound) { // Create if not exists
	// 			newTag := &model.Tag{
	// 				ID:   uuid.NewString(),
	// 				Name: nameOrSlug, // Assuming it's a name if not found by slug
	// 				Slug: slug.Make(nameOrSlug),
	// 			}
	// 			if errCreate := s.tagRepo.Create(ctx, tx, newTag); errCreate != nil {
	// 				return nil, fmt.Errorf("failed to create tag %s: %w", nameOrSlug, errCreate)
	// 			}
	// 			tag = newTag
	// 		} else {
	// 			return nil, fmt.Errorf("failed to find tag %s: %w", nameOrSlug, err)
	// 		}
	// 	}
	// 	tagIDs = append(tagIDs, tag.ID)
	// }
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
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	// For regular users, only show Published problems
	statusFilter := model.StatusPublished
	if userRole == model.RoleAdmin {
		statusFilter = "" // Admin can see all statuses, or we can add a status query param
	}

	// Convert tag names/slugs to tag IDs if tagRepo is available
	var tagIDs []string
	// if len(tags) > 0 && s.tagRepo != nil {
	// 	for _, tagNameOrSlug := range tags {
	// 		tag, err := s.tagRepo.FindBySlugOrName(ctx, nil, tagNameOrSlug) // nil for tx as it's a read
	// 		if err == nil && tag != nil {
	// 			tagIDs = append(tagIDs, tag.ID)
	// 		} else {
	// 			log.Printf("WARN: Tag '%s' not found, skipping in filter.", tagNameOrSlug)
	// 		}
	// 	}
	// 	if len(tags) > 0 && len(tagIDs) == 0 { // User filtered by tags but none found
	// 		return []model.Problem{}, 0, nil
	// 	}
	// }

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
