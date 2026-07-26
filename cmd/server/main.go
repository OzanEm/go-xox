package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ozanem/go-xox/internal/auth"
	"github.com/ozanem/go-xox/internal/config"
	"github.com/ozanem/go-xox/internal/httpapi"
	"github.com/ozanem/go-xox/internal/refreshtoken"
	"github.com/ozanem/go-xox/internal/server"
	"github.com/ozanem/go-xox/internal/user"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo, AddSource: true}))

	if err := run(logger); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	logger.Info("connected to postgres")

	users := user.NewPostgresRepository(pool)
	rfRepo := refreshtoken.NewPostgresRepository(pool)
	rf := auth.NewRefreshTokenIssuer(cfg.RefreshTokenTTL, rfRepo)
	tokens := auth.NewTokenIssuer(cfg.JWTSecret, cfg.JWTTTL)
	authService := auth.NewService(users, tokens, rf, cfg.BcryptCost)
	api := httpapi.New(authService, logger, users)

	gameManager := server.NewGameManager(users, logger)
	hub := server.NewHub(gameManager, logger)

	wsHandler := server.NewWSHandler(hub, tokens, logger)

	root := http.NewServeMux()
	root.Handle("/", api.Routes())
	root.Handle("/ws", wsHandler)

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: root,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	logger.Info("server stopped cleanly")
	return nil
}
