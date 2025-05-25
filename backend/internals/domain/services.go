package domain

import "tle-zone/internals/model"

type UserService interface {
	Register(user *model.UserRegistration) error
	GetUserByID(id int) (*model.User, error)
	UpdateProfile(userID int, update *model.UserUpdate) error
}

type AuthService interface {
	Authenticate(username, password string) (model.User, error)
	GenerateToken(user *model.User) (string, error)
	ValidateToken(token string) (*model.User, error)
}

type ProblemService interface {
	GetProblemByID(id int) (*model.ProblemDetail, error)
	ListProblems(filter *model.ProblemFilter) ([]*model.ProblemInfo, error)
}

type SubmissionService interface {
	CreateSubmission(sub *model.Submission) (int, error)
	ProcessExecutionResult(result *model.ExecutionResult) error
	GetUserSubmissions(userID int) ([]*model.Submission, error)
	GetSubmissionByID(submissionId, userId int) (*model.Submission, error)
}

type ContestService interface {
	StartContest(contest *model.Contest) error
	GetContestStatus(contestID int) (string, error)
	EndContest(contestID int) error
	CalculateContestResults(contestID int) error // Async
}

type LeaderboardService interface {
	UpdateUserScore(userID int, delta int) error // Event-driven
	GetLeaderboard(scope model.LeaderboardScope) ([]*model.LeaderboardEntry, error)
	GetUserRank(userID int, scope model.LeaderboardScope) (int, error)
}

type CodeExecutionService interface {
	ExecuteCode(req *model.CodeExecutionRequest) (*model.CodeExecutionResponse, error)
}

type AdminService interface {
	CreateProblem(problem *model.ProblemDetail) error
	UpdateProblem(problem *model.ProblemDetail) error
	InvalidateProblem(problem *model.ProblemDetail) error
	BanUser(userID int) error
}
