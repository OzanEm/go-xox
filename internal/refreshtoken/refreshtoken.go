// Package refreshtoken owns the refresh-token record and its persistence.
package refreshtoken

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrNotFound            = errors.New("refresh token not found")
)

// RefreshToken is a stored refresh token. Only the SHA-256 hash of the raw
// token is persisted; the raw value exists solely in the client's hands.
type RefreshToken struct {
	ID        int64
	UserID    int64
	Hash      string
	Expires   time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// Repository is the persistence boundary for refresh tokens.
type Repository interface {
	Store(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) (*RefreshToken, error)
	ByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	Revoke(ctx context.Context, tokenHash string) error
}
