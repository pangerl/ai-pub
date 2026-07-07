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
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"

	"ai-pub/internal/app"
	"ai-pub/internal/auth"
	"ai-pub/internal/config"
	"ai-pub/internal/crypto"
	"ai-pub/internal/domain"
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
	if cfg.AppEnv != "prod" && cfg.JWTSecret == "dev-secret-change-me" {
		slog.Warn("JWT_SECRET 使用不安全默认值；共享开发环境前请设置独立密钥")
	}

	db, err := openDB(cfg)
	if err != nil {
		return err
	}
	defer db.Close()

	if cfg.MigrationCheckOnly {
		runner := migration.NewRunner(db, cfg.DBDialect, os.DirFS("."))
		report, err := runner.Run(context.Background(), true)
		if err != nil {
			return err
		}
		slog.Info("migration check complete", "pending", len(report.Pending), "applied", len(report.Applied))
		return nil
	}
	if cfg.MigrationAuto {
		runner := migration.NewRunner(db, cfg.DBDialect, os.DirFS("."))
		report, err := runner.Run(context.Background(), false)
		if err != nil {
			return err
		}
		slog.Info("migration complete", "applied", len(report.Applied))
	} else {
		slog.Info("migration skipped", "reason", "MIGRATION_AUTO=false")
	}
	if err := ensureBootstrapAdmin(context.Background(), repository.NewStore(db), cfg); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cfg.WorkerEnabled {
		store := repository.NewStore(db)
		box := crypto.NewBox(cfg.AppEncryptionKey)
		credentialService := app.NewCredentialService(store, box)
		notificationService := app.NewNotificationService(store, box, nil)
		workerService := worker.NewService(store, credentialService, &notificationService, "worker_builtin", worker.Options{
			SSHEnabled: !cfg.ExecutorSSHDisabled,
			K8sEnabled: !cfg.ExecutorK8sDisabled,
		})
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

func ensureBootstrapAdmin(ctx context.Context, store repository.Store, cfg config.Config) error {
	admin, err := store.GetUserByUsername(ctx, cfg.BootstrapAdminUsername)
	if err == nil && admin.PasswordHash != "" {
		return nil
	}
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return fmt.Errorf("find bootstrap admin: %w", err)
	}
	if cfg.BootstrapAdminPassword == "" {
		return errors.New("BOOTSTRAP_ADMIN_PASSWORD is required until the bootstrap administrator has a password")
	}
	hash, err := auth.HashPassword(cfg.BootstrapAdminPassword)
	if err != nil {
		return err
	}
	if err == nil && admin.ID != "" {
		if err := store.SetUserPassword(ctx, admin.ID, hash); err != nil {
			return fmt.Errorf("set bootstrap admin password: %w", err)
		}
		slog.Info("bootstrap admin password initialized", "username", cfg.BootstrapAdminUsername)
		return nil
	}
	_, err = store.CreateUserWithPassword(ctx, domain.User{Username: cfg.BootstrapAdminUsername, DisplayName: "Administrator", Role: "admin", PasswordHash: hash})
	if err != nil {
		return fmt.Errorf("create bootstrap admin: %w", err)
	}
	slog.Info("bootstrap admin created", "username", cfg.BootstrapAdminUsername)
	return nil
}

func openDB(cfg config.Config) (*sql.DB, error) {
	if cfg.DBDialect == "sqlite" {
		return openSQLite(cfg.SQLitePath)
	}
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

func openSQLite(path string) (*sql.DB, error) {
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("create sqlite directory: %w", err)
			}
		}
	}
	dsn := path
	if path != ":memory:" {
		dsn += "?_foreign_keys=on&_busy_timeout=5000"
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable sqlite foreign keys: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set sqlite busy timeout: %w", err)
	}
	if path != ":memory:" {
		if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
			db.Close()
			return nil, fmt.Errorf("enable sqlite wal: %w", err)
		}
	}
	return db, nil
}
