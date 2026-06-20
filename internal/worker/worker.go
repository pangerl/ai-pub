package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"ai-pub/internal/app"
	"ai-pub/internal/domain"
	"ai-pub/internal/executor"
	"ai-pub/internal/repository"
)

type Service struct {
	store         repository.Store
	mock          executor.Mock
	ssh           executor.SSH
	credentials   app.CredentialService
	notifications *app.NotificationService
	workerID      string
}

func NewService(store repository.Store, credentials app.CredentialService, notifications *app.NotificationService, workerID string) Service {
	return Service{
		store:         store,
		mock:          executor.Mock{},
		ssh:           executor.SSH{Credentials: credentials},
		credentials:   credentials,
		notifications: notifications,
		workerID:      workerID,
	}
}

func (s Service) RunLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := s.RunOnce(ctx); err != nil && !errors.Is(err, repository.ErrNotFound) {
			slog.Error("worker tick failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s Service) RunOnce(ctx context.Context) error {
	claimed, err := s.store.ClaimNextDeploy(ctx, s.workerID)
	if err != nil {
		return err
	}
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: claimed.Release.ID,
		DeployRecordID:   claimed.Record.ID,
		EventType:        "deploy_started",
		ActorType:        "system",
		ActorID:          s.workerID,
		Message:          "发布执行已开始",
	})
	failed := false
	for _, server := range claimed.Servers {
		if failed {
			break
		}
		result := s.execute(ctx, claimed, server)
		if err := s.store.MarkServerFinished(ctx, claimed.Record.ID, server.ID, result); err != nil {
			return err
		}
		_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
			ReleaseRequestID: claimed.Release.ID,
			DeployRecordID:   claimed.Record.ID,
			EventType:        "server_finished",
			ActorType:        "system",
			ActorID:          s.workerID,
			Message:          result.Status + ": " + server.Name,
		})
		failed = result.Status == "failed"
	}
	if failed {
		if err := s.store.MarkQueuedServersSkipped(ctx, claimed.Record.ID, "skipped after previous server failure"); err != nil {
			return err
		}
	}
	record, err := s.store.FinishDeploy(ctx, claimed.Record.ID)
	if err != nil {
		return err
	}
	if s.notifications != nil && (record.Status == "failed" || record.Status == "partial") {
		s.notifications.NotifyAll(ctx, app.NotificationEvent{
			EventType:        "deploy_failed",
			ReleaseRequestID: claimed.Release.ID,
			DeployRecordID:   claimed.Record.ID,
			Content:          app.FailureContent(claimed.Release.ID, claimed.Record.ID, record.Status),
		})
	}
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: claimed.Release.ID,
		DeployRecordID:   claimed.Record.ID,
		EventType:        "deploy_finished",
		ActorType:        "system",
		ActorID:          s.workerID,
		Message:          "发布执行结束：" + record.Status,
	})
	return nil
}

func (s Service) execute(ctx context.Context, claimed repository.ClaimedDeploy, server domain.Server) repository.ServerResult {
	switch claimed.Target.ExecutorType {
	case "mock":
		return s.mock.Execute(ctx, executor.Request{
			Release: claimed.Release,
			Record:  claimed.Record,
			Target:  claimed.Target,
			Version: claimed.Version,
			Server:  server,
		})
	case "ssh":
		return s.ssh.Execute(ctx, executor.Request{
			Release: claimed.Release,
			Record:  claimed.Record,
			Target:  claimed.Target,
			Version: claimed.Version,
			Server:  server,
		})
	default:
		code := 1
		return repository.ServerResult{
			Status:       "failed",
			ExitCode:     &code,
			ErrorCode:    "internal_error",
			ErrorMessage: "unsupported executor: " + claimed.Target.ExecutorType,
		}
	}
}
