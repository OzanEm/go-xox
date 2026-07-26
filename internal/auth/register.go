package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/ozanem/go-xox/internal/user"
)

func (s *Service) Register(ctx context.Context, username, password string) error {
	hash, err := s.HashPassword(password)
	if err != nil {
		return err
	}

	_, err = s.users.Create(ctx, username, hash)
	switch {
	case errors.Is(err, user.ErrUsernameTaken):
		return ErrUsernameTaken
	case err != nil:
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}
