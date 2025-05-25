package model

type LeaderboardEntry struct {
	Rank           int    `json:"rank"`
	UserID         string `json:"user_id"`
	Username       string `json:"username"`
	ProblemsSolved int    `json:"problems_solved"`
	// Accuracy       float64 `json:"accuracy"` // (Accepted Submissions / Total Submissions)
	// AvgTimeToSolve int64 `json:"avg_time_to_solve_seconds"` // Complex to calculate
}