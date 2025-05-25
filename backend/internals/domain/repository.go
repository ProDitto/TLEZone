package domain

import "tle-zone/internals/model"

// UserRepository handles persistence for users.
type UserRepository interface {
	Create(user *model.UserDB) error
	FindByID(id int) (*model.User, error)
	FindByEmail(email string) (*model.User, error)
	FindByCreds(username, hashedPassword string) (*model.User, error)
	Update(user *model.User) error
	BanUser(userID int) error
}

// AuthRepository handles token persistence (optional).
type AuthRepository interface {
	StoreRefreshToken(userID int, token string) error
	GetUserByToken(token string) (*model.UserDB, error)
	RevokeToken(token string) error
}

// ProblemRepository handles CRUD for problems.
type ProblemRepository interface {
	Create(problem *model.ProblemDB) error
	Update(problem *model.ProblemDB) error
	Invalidate(problemID int) error
	GetByID(id int) (*model.ProblemDB, error)
	List(filter *model.ProblemFilter) ([]*model.ProblemInfo, error)
	GetTags(problemID int) ([]model.Tag, error)
	GetExamples(problemID int) ([]model.ProblemExampleDB, error)
	GetTestCases(problemID int) ([]model.TestCaseDB, error)
}

// TagRepository handles tags (optional).
type TagRepository interface {
	GetAll() ([]model.TagDB, error)
	GetByIDs(ids []int) ([]model.TagDB, error)
}

// SubmissionRepository handles user submissions.
type SubmissionRepository interface {
	Create(submission *model.SubmissionDB) (int, error)
	UpdateResult(result *model.ExecutionResult) error
	GetByID(id int) (*model.SubmissionDB, error)
	GetByUserID(userID int) ([]*model.SubmissionDB, error)
	GetByProblemID(problemID int) ([]*model.SubmissionDB, error)
}

// ContestRepository handles contests.
type ContestRepository interface {
	Create(contest *model.Contest) error
	Update(contest *model.Contest) error
	GetByID(id int) (*model.Contest, error)
	List() ([]*model.Contest, error)
}

// LeaderboardRepository tracks scores and ranks.
type LeaderboardRepository interface {
	UpdateScore(userID int, scoreDelta int, contestID *int) error
	GetLeaderboard(contestID *int) ([]*model.LeaderboardEntry, error)
	GetUserRank(userID int, contestID *int) (int, error)
}

// LanguageRepository maps language IDs to names and vice versa.
type LanguageRepository interface {
	GetByID(id int) (string, error)
	GetIDByName(name string) (int, error)
	GetAll() ([]model.ProgrammingLanguageDB, error)
}
