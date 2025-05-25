package api

import (
	"net/http"
	"time"
	"tle_zone_v2/internal/api/handler"
	"tle_zone_v2/internal/app/service"
	"tle_zone_v2/internal/common/security"

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
