package user

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("user not found")

var ErrUsernameTaken = errors.New("username already taken")

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	Wins         int64
	Losses       int64
	Draws        int64
}

type Repository interface {
	ByUsername(ctx context.Context, username string) (*User, error)
	ByUserID(ctx context.Context, userID int64) (*User, error)
	Create(ctx context.Context, username, passwordHash string) (*User, error)
}
