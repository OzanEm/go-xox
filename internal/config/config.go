package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	JWTSecret       []byte
	JWTTTL          time.Duration
	BcryptCost      int
	ShutdownTimeout time.Duration
	RefreshTokenTTL time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        envString("HTTP_ADDR", ":8080"),
		DatabaseURL:     envString("DATABASE_URL", "postgres://xox:xox@localhost:5432/xox?sslmode=disable"),
		JWTSecret:       []byte(os.Getenv("JWT_SECRET")),
		ShutdownTimeout: 10 * time.Second,
	}

	if len(cfg.JWTSecret) == 0 {
		return Config{}, fmt.Errorf("JWT_SECRET must be set")
	}

	var err error
	if cfg.JWTTTL, err = envDuration("JWT_TTL", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if cfg.BcryptCost, err = envInt("BCRYPT_COST", 12); err != nil {
		return Config{}, err
	}
	if cfg.RefreshTokenTTL, err = envDuration("REFRESH_TOKEN_TTL", 24*time.Hour); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return v, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return v, nil
}
