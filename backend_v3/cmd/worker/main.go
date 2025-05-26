package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

// Task struct
type Task struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func main() {
	log.Println("👷 Worker service starting...")

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Graceful shutdown on SIGINT or SIGTERM
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Redis connection
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379", // Use env var in real setup
	})
	defer rdb.Close()

	// Start worker
	startWorker(ctx, rdb, &wg)

	// Wait for signal
	<-sigs
	log.Println("🔻 Shutdown signal received.")
	cancel()

	// Wait for worker to finish
	wg.Wait()
	log.Println("✅ Worker exited cleanly.")
}

// startWorker listens to the process_queue, processes tasks, and sends results to results_queue
func startWorker(ctx context.Context, rdb *redis.Client, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("🛠️ Worker started...")

		for {
			select {
			case <-ctx.Done():
				log.Println("🛑 Worker context canceled. Exiting...")
				return
			default:
				res, err := rdb.BLPop(ctx, 5*time.Second, "process_queue").Result()
				if err != nil {
					if err == redis.Nil {
						continue
					}
					if ctx.Err() != nil {
						log.Println("Context canceled during BLPOP")
						return
					}
					log.Printf("BLPOP error: %v", err)
					time.Sleep(1 * time.Second)
					continue
				}

				var task Task
				if err := json.Unmarshal([]byte(res[1]), &task); err != nil {
					log.Printf("Invalid task JSON: %v", err)
					continue
				}

				log.Printf("🔧 Processing task ID %d: %s", task.ID, task.Name)

				// Simulate work
				time.Sleep(2 * time.Second)

				result := Task{
					ID:   task.ID,
					Name: fmt.Sprintf("Processed: %s", task.Name),
				}

				data, _ := json.Marshal(result)
				if err := rdb.RPush(ctx, "results_queue", data).Err(); err != nil {
					log.Printf("❌ Failed to push result: %v", err)
				} else {
					log.Printf("✅ Pushed result for task ID %d", task.ID)
				}
			}
		}
	}()
}
