package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gocraft/dbr/v2"
	_ "github.com/lib/pq"
	"go.uber.org/zap"

	"pr-reviewer-service/internal/config"
	"pr-reviewer-service/internal/repository"
	"pr-reviewer-service/internal/server"
	"pr-reviewer-service/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger, err := config.NewLogger(cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	conn, err := dbr.Open("postgres", cfg.DSN(), nil)
	if err != nil {
		logger.Fatal("failed to open database", zap.Error(err))
	}
	defer conn.Close()

	if err := conn.Ping(); err != nil {
		logger.Fatal("failed to ping database", zap.Error(err))
	}

	sess := conn.NewSession(nil)

	repo := repository.New(sess, logger)
	svc := service.New(repo, logger)
	srv := server.New(svc, logger)

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logger.Info("shutting down server")
		srv.Shutdown()
	}()

	addr := fmt.Sprintf(":%s", cfg.ServerPort)
	logger.Info("starting server", zap.String("addr", addr))
	if err := srv.Listen(addr); err != nil {
		logger.Error("server stopped", zap.Error(err))
	}
}