package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"ai-pub/internal/app"
	"ai-pub/internal/config"
	"ai-pub/internal/crypto"
	"ai-pub/internal/httpapi"
	"ai-pub/internal/migration"
	"ai-pub/internal/repository"
	"ai-pub/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		return err
	}

	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	runner := migration.NewRunner(db, "mysql", os.DirFS("."))
	report, err := runner.Run(context.Background(), cfg.MigrationCheckOnly)
	if err != nil {
		return err
	}
	if cfg.MigrationCheckOnly {
		slog.Info("migration check complete", "pending", len(report.Pending), "applied", len(report.Applied))
		return nil
	}
	slog.Info("migration complete", "applied", len(report.Applied))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cfg.WorkerEnabled {
		store := repository.NewStore(db)
		box := crypto.NewBox(cfg.AppEncryptionKey)
		credentialService := app.NewCredentialService(store, box)
		notificationService := app.NewNotificationService(store, box, nil)
		workerService := worker.NewService(store, credentialService, &notificationService, "worker_builtin")
		go workerService.RunLoop(ctx, 2*time.Second)
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(httpapi.Dependencies{DB: db, Config: cfg}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.HTTPAddr)
		errCh <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		slog.Info("shutdown requested", "signal", sig.String())
		cancel()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	return server.Shutdown(shutdownCtx)
}

func openDB(cfg config.Config) (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.MySQLDSN)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return db, nil
}
