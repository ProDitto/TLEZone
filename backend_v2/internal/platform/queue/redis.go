package queue

import (
	"context"
	"fmt"
	"log"
	"tle_zone_v2/internal/platform/config"

	"github.com/redis/go-redis/v9"
)

var RDB *redis.Client

func ConnectRedis() {
	RDB = redis.NewClient(&redis.Options{
		Addr:     config.AppConfig.RedisAddr,
		Password: config.AppConfig.RedisPassword,
		DB:       config.AppConfig.RedisDB,
	})

	ctx := context.Background()
	_, err := RDB.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Could not connect to Redis: %v", err)
	}
	fmt.Println("Successfully connected to Redis!")
}

func CloseRedis() {
	if RDB != nil {
		RDB.Close()
		fmt.Println("Redis connection closed.")
	}
}
