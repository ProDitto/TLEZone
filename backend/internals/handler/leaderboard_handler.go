package handler

import (
	"net/http"
	"tle-zone/internals/domain"
)

type leaderboardHandler struct {
	leaderboardService domain.LeaderboardService
}

func NewLeaderboardHandler(ls domain.LeaderboardService) domain.LeaderboardHandler {
	return &leaderboardHandler{leaderboardService: ls}
}

func (lh *leaderboardHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (lh *leaderboardHandler) GetUserRank(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
