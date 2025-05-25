package model

import "time"

type Language struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"` // For API usage
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}