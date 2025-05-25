package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"
	"tle_zone_v2/internal/platform/config"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

var DB *sql.DB

func Connect() {
	var err error
	DB, err = sql.Open("pgx", config.AppConfig.DBConnStr)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(25)
	DB.SetConnMaxLifetime(5 * time.Minute)

	// Verify connection
	if err = DB.Ping(); err != nil {
		log.Fatalf("Error connecting to database: %v", err)
	}

	fmt.Println("Successfully connected to PostgreSQL database!")
}

func Close() {
	if DB != nil {
		DB.Close()
		fmt.Println("Database connection closed.")
	}
}
