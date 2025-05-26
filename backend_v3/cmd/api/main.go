package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

// --- Models ---

type Task struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// --- Main ---

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup

	// Setup Redis
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // Or use os.Getenv("REDIS_ADDR")
	})
	defer rdb.Close()

	// Start background result worker
	startResultWorker(ctx, rdb, &wg)

	// Setup signal handler for graceful shutdown
	go handleShutdown(cancel)

	// Setup HTTP server
	r := chi.NewRouter()
	r.Get("/health", healthHandler)
	r.Post("/submit", submitCodeHandler(rdb))

	server := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	log.Println("🚀 Server running on :8080")

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	log.Println("🔻 Shutting down HTTP server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	// Wait for background workers to finish
	wg.Wait()
	log.Println("✅ Backend exited cleanly.")
}

// --- Handlers ---

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func submitCodeHandler(rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var task Task
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}

		if task.ID == 0 {
			task.ID = int(time.Now().Unix()) // fallback ID
		}

		log.Printf("🌟 Adding task %d: %s", task.ID, task.Name)
		if err := enqueueTask(r.Context(), rdb, task); err != nil {
			log.Printf("❌ Failed to enqueue task: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue"})
			return
		}

		writeJSON(w, http.StatusAccepted, map[string]string{
			"message": "task accepted",
			"task_id": strconv.Itoa(task.ID),
		})
	}
}

// --- Redis Task Queue ---

func enqueueTask(ctx context.Context, rdb *redis.Client, task Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return rdb.RPush(ctx, "process_queue", data).Err()
}

// --- Background Worker ---

func startResultWorker(ctx context.Context, rdb *redis.Client, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("🛠️ Result worker started...")
		for {
			select {
			case <-ctx.Done():
				log.Println("🛑 Result worker shutting down...")
				return
			default:
				res, err := rdb.BLPop(ctx, 5*time.Second, "results_queue").Result()
				if err != nil {
					if err == redis.Nil {
						continue // no result yet
					}
					if ctx.Err() != nil {
						log.Println("Context canceled, exiting worker...")
						return
					}
					log.Printf("Redis BLPOP error: %v", err)
					time.Sleep(1 * time.Second)
					continue
				}

				var result Task
				if err := json.Unmarshal([]byte(res[1]), &result); err != nil {
					log.Printf("Invalid result JSON: %v", err)
					continue
				}

				// Simulate saving to DB
				log.Printf("✅ Received result for task ID %d: %s", result.ID, result.Name)
			}
		}
	}()
}

// --- Graceful Shutdown Signal ---

func handleShutdown(cancel context.CancelFunc) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	log.Println("🔻 Shutdown signal received.")
	cancel()
}

// --- JSON Response Helper ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
