package refreshtoken

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

var _ Repository = (*postgresRepository)(nil)

func (r *postgresRepository) Store(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (*RefreshToken, error) {

	const sql = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, expires_at, revoked_at, created_at`

	var t RefreshToken
	err := r.pool.QueryRow(ctx, sql, userID, tokenHash, expiresAt).
		Scan(&t.ID, &t.UserID, &t.Hash, &t.Expires, &t.RevokedAt, &t.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}
	return &t, nil
}

func (r *postgresRepository) ByHash(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	const sql = `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens WHERE token_hash = $1`

	var t RefreshToken
	err := r.pool.QueryRow(ctx, sql, tokenHash).
		Scan(&t.ID, &t.UserID, &t.Hash, &t.Expires, &t.RevokedAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find refresh token by hash: %w", err)
	}
	return &t, nil
}

func (r *postgresRepository) Revoke(ctx context.Context, tokenHash string) error {
	const sql = `UPDATE refresh_tokens SET revoked_at = $1 WHERE token_hash = $2`
	_, err := r.pool.Exec(ctx, sql, time.Now(), tokenHash)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}
