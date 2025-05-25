package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	APIPort string
	JWTKey  []byte
	JWTExp  time.Duration

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSslMode  string
	DBConnStr  string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	ExecutionQueueName      string
	ExecutionLockKey        string
	ExecutionLockTTLSeconds int
	MockExecutorWebhookURL  string
	ProblemValidationLangID string

	ProblemValidationDefaultRuntimeLimitMs int
	ProblemValidationDefaultMemoryLimitKb  int
}

var AppConfig *Config

func Load() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	AppConfig = &Config{
		APIPort:                 getEnv("API_PORT", "8080"),
		JWTKey:                  []byte(getEnv("JWT_SECRET", "defaultsecret")),
		JWTExp:                  time.Duration(getEnvAsInt("JWT_EXPIRATION_HOURS", 72)) * time.Hour,
		DBHost:                  getEnv("DB_HOST", "localhost"),
		DBPort:                  getEnv("DB_PORT", "5432"),
		DBUser:                  getEnv("DB_USER", "user"),
		DBPassword:              getEnv("DB_PASSWORD", "password"),
		DBName:                  getEnv("DB_NAME", "leetcode_clone_db"),
		DBSslMode:               getEnv("DB_SSLMODE", "disable"),
		RedisAddr:               getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:           getEnv("REDIS_PASSWORD", ""),
		RedisDB:                 getEnvAsInt("REDIS_DB", 0),
		ExecutionQueueName:      getEnv("EXECUTION_QUEUE_NAME", "execution_jobs_queue"),
		ExecutionLockKey:        getEnv("EXECUTION_LOCK_KEY", "execution_job_lock"),
		ExecutionLockTTLSeconds: getEnvAsInt("EXECUTION_LOCK_TTL_SECONDS", 300),
		MockExecutorWebhookURL:  getEnv("MOCK_EXECUTOR_WEBHOOK_URL", "http://localhost:8080/api/v1/webhook/execution"),
		ProblemValidationLangID: getEnv("PROBLEM_VALIDATION_LANGUAGE_ID", ""),

		ProblemValidationDefaultRuntimeLimitMs: getEnvAsInt("Problem_Validation_Default_Runtime_Limit_Ms", 5000),
		ProblemValidationDefaultMemoryLimitKb:  getEnvAsInt("Problem_Validation_Default_Memory_Limit_Kb", 5000),
	}

	AppConfig.DBConnStr = "host=" + AppConfig.DBHost +
		" port=" + AppConfig.DBPort +
		" user=" + AppConfig.DBUser +
		" password=" + AppConfig.DBPassword +
		" dbname=" + AppConfig.DBName +
		" sslmode=" + AppConfig.DBSslMode
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}
