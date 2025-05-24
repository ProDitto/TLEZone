package domain

import "net/http"

type AuthHandler interface {
	Login(http.ResponseWriter, *http.Request)  // POST /auth/login
	Signup(http.ResponseWriter, *http.Request) // POST /auth/signup
	Logout(http.ResponseWriter, *http.Request) // POST /auth/logout (optional)
}

type UserHandler interface {
	GetProfile(http.ResponseWriter, *http.Request)          // GET /users/{id}
	UpdateProfile(http.ResponseWriter, *http.Request)       // PUT /users/{id}
	ListUserSubmissions(http.ResponseWriter, *http.Request) // GET /users/{id}/submissions
}

type ProblemHandler interface {
	ListProblems(http.ResponseWriter, *http.Request)   // GET /problems
	GetProblemByID(http.ResponseWriter, *http.Request) // GET /problems/{id}
}

type SubmissionHandler interface {
	SubmitCode(http.ResponseWriter, *http.Request)          // POST /submissions
	GetSubmissionResult(http.ResponseWriter, *http.Request) // GET /submissions/{id}
}

type ContestHandler interface {
	CreateContest(http.ResponseWriter, *http.Request)    // POST /contests
	JoinContest(http.ResponseWriter, *http.Request)      // POST /contests/{id}/join
	GetContestStatus(http.ResponseWriter, *http.Request) // GET /contests/{id}/status
	ListContests(http.ResponseWriter, *http.Request)     // GET /contests
}

type LeaderboardHandler interface {
	GetLeaderboard(http.ResponseWriter, *http.Request) // GET /leaderboard?contestID=
	GetUserRank(http.ResponseWriter, *http.Request)    // GET /leaderboard/{userID}/rank
}

type AdminHandler interface {
	CreateProblem(http.ResponseWriter, *http.Request)     // POST /admin/problems
	UpdateProblem(http.ResponseWriter, *http.Request)     // PUT /admin/problems/{id}
	InvalidateProblem(http.ResponseWriter, *http.Request) // DELETE /admin/problems/{id}
	BanUser(http.ResponseWriter, *http.Request)           // POST /admin/users/{id}/ban
}
