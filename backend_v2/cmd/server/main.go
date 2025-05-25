package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"tle_zone_v2/internal/api"
	"tle_zone_v2/internal/app/service"
	"tle_zone_v2/internal/app/worker"
	"tle_zone_v2/internal/common/security"
	"tle_zone_v2/internal/domain/repository"
	"tle_zone_v2/internal/platform/config"
	"tle_zone_v2/internal/platform/database"
	"tle_zone_v2/internal/platform/queue"
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
	// var tagRepo repository.TagRepository                          // = repository.NewPgTagRepository(database.DB)               // Implement fully
	var submissionRepo repository.SubmissionRepository // = repository.NewPgSubmissionRepository(database.DB) // Implement fully
	var execJobRepo repository.ExecutionJobRepository  // = repository.NewPgExecutionJobRepository(database.DB)  // Implement fully
	// ... other repos

	// 6. Initialize Services
	authService := service.NewAuthService(userRepo)
	execJobService := service.NewExecutionJobService(execJobRepo, submissionRepo, queue.RDB, database.DB)
	// problemService := service.NewProblemService(problemRepo, tagRepo, execJobService, database.DB)
	problemService := service.NewProblemService(problemRepo, execJobService, database.DB)
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
		Addr:         ":" + config.AppConfig.APIPort,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
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
