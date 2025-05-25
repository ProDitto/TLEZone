package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"tle_zone_v2/internal/common"
	"tle_zone_v2/internal/domain/model"

	"github.com/jackc/pgx/v5/pgconn"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	FindByID(ctx context.Context, id string) (*model.User, error)
}

type pgUserRepository struct {
	db *sql.DB
}

func NewPgUserRepository(db *sql.DB) UserRepository {
	return &pgUserRepository{db: db}
}

func (r *pgUserRepository) Create(ctx context.Context, user *model.User) error {
	query := `INSERT INTO users (id, username, email, hashed_password, role)
	          VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.ExecContext(ctx, query, user.ID, user.Username, user.Email, user.HashedPassword, user.Role)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // Unique constraint violation
			return fmt.Errorf("user with given username or email already exists: %w", common.ErrConflict)
		}
		return fmt.Errorf("pgUserRepository.Create: %w", err)
	}
	return nil
}

func (r *pgUserRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	query := `SELECT id, username, email, hashed_password, role, created_at, updated_at
	          FROM users WHERE email = $1`
	user := &model.User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("pgUserRepository.FindByEmail: %w", err)
	}
	return user, nil
}

// FindByUsername and FindByID are similar to FindByEmail
// ... (implement FindByUsername, FindByID)
func (r *pgUserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	query := `SELECT id, username, email, hashed_password, role, created_at, updated_at
	          FROM users WHERE username = $1`
	user := &model.User{}
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("pgUserRepository.FindByUsername: %w", err)
	}
	return user, nil
}

func (r *pgUserRepository) FindByID(ctx context.Context, id string) (*model.User, error) {
	query := `SELECT id, username, email, hashed_password, role, created_at, updated_at
	          FROM users WHERE id = $1`
	user := &model.User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.HashedPassword, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, common.ErrNotFound
		}
		return nil, fmt.Errorf("pgUserRepository.FindByID: %w", err)
	}
	return user, nil
}
