package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueViolation is the Postgres SQLSTATE code for a unique-constraint conflict
const uniqueViolation = "23505"

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

var _ Repository = (*PostgresRepository)(nil)

func (r *PostgresRepository) ByUsername(ctx context.Context, username string) (*User, error) {
	const query = `
		SELECT id, username, password_hash, created_at
		FROM users
		WHERE username = $1`

	var u User
	err := r.pool.QueryRow(ctx, query, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query user by username: %w", err)
	}

	return &u, nil
}

func (r *PostgresRepository) Create(ctx context.Context, username, passwordHash string) (*User, error) {
	const query = `
		INSERT INTO users (username, password_hash)
		VALUES ($1, $2)
		RETURNING id, username, password_hash, created_at`

	var u User
	err := r.pool.QueryRow(ctx, query, username, passwordHash).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return nil, ErrUsernameTaken
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}

	return &u, nil
}

func (r *PostgresRepository) ByUserID(ctx context.Context, userID int64) (*User, error) {
	const query = `
		SELECT id, username, password_hash, created_at
		FROM users
		WHERE id = $1`

	var u User
	err := r.pool.QueryRow(ctx, query, userID).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query user by id: %w", err)
	}

	return &u, nil
}

func (r *PostgresRepository) RecordDecisive(ctx context.Context, winnerID, loserID int64) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE users SET wins = wins + 1 WHERE id = $1`, winnerID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE users SET losses = losses + 1 WHERE id = $1`, loserID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
func (r *PostgresRepository) RecordDraw(ctx context.Context, aID, bID int64) error {
	sql := `UPDATE users SET draws = draws + 1 WHERE id = $1 OR id = $2`
	_, err := r.pool.Exec(ctx, sql, aID, bID)
	if err != nil {
		return err
	}
	return nil
}

func (r *PostgresRepository) GetLeaderBoard(ctx context.Context) ([]User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, username, wins, losses, draws
		FROM users
		ORDER BY wins DESC
		LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		err := rows.Scan(&u.ID, &u.Username, &u.Wins, &u.Losses, &u.Draws)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}
