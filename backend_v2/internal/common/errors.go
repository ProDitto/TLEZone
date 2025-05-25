package common

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound           = errors.New("requested resource not found")
	ErrUnauthorized       = errors.New("unauthorized access")
	ErrForbidden          = errors.New("forbidden access")
	ErrBadRequest         = errors.New("bad request")
	ErrConflict           = errors.New("resource conflict") // e.g., username already exists
	ErrInternalServer     = errors.New("internal server error")
	ErrValidation         = errors.New("validation failed")
	ErrServiceUnavailable = errors.New("service unavailable") // e.g. external executor down
	ErrJobLockFailed      = errors.New("failed to acquire job lock")
)

// HTTPStatusFromError maps domain errors to HTTP status codes.
func HTTPStatusFromError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, ErrUnauthorized) {
		return http.StatusUnauthorized
	}
	if errors.Is(err, ErrForbidden) {
		return http.StatusForbidden
	}
	if errors.Is(err, ErrBadRequest) || errors.Is(err, ErrValidation) {
		return http.StatusBadRequest
	}
	if errors.Is(err, ErrConflict) {
		return http.StatusConflict
	}
	if errors.Is(err, ErrServiceUnavailable) {
		return http.StatusServiceUnavailable
	}
	if errors.Is(err, ErrJobLockFailed) {
		return http.StatusConflict // Or 503 if it's transient
	}

	// Check for pgx specific errors (example for unique constraint)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" { // Unique violation
			return http.StatusConflict
		}
	}

	return http.StatusInternalServerError
}

// Errorf creates a new error with formatting, useful for wrapping.
func Errorf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}
