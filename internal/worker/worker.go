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
	executors     executor.Registry
	credentials   app.CredentialService
	notifications *app.NotificationService
	workerID      string
}

// Options 控制 worker 注册哪些执行器。调用方显式传入启用状态；demo 公网部署时设
// SSHEnabled/K8sEnabled 为 false，只保留 mock，从代码层消除 SSH 跳板与 K8s 外联风险。
type Options struct {
	SSHEnabled bool
	K8sEnabled bool
}

func NewService(store repository.Store, credentials app.CredentialService, notifications *app.NotificationService, workerID string, opts Options) Service {
	executors := map[string]executor.Executor{
		"mock": executor.Mock{},
	}
	if opts.SSHEnabled {
		executors["ssh"] = executor.SSH{Credentials: credentials}
	}
	if opts.K8sEnabled {
		executors["k8s"] = executor.K8s{Credentials: credentials, Clusters: store}
	}
	return Service{
		store:         store,
		executors:     executor.NewRegistry(executors),
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
	recovered, err := s.store.RecoverExpiredDeploys(ctx)
	if err != nil {
		return err
	}
	for _, item := range recovered {
		_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
			ReleaseRequestID: item.ReleaseID,
			DeployRecordID:   item.RecordID,
			EventType:        "deploy_finished",
			ActorType:        "system",
			ActorID:          s.workerID,
			Message:          "发布因 Worker 租约过期失败",
		})
	}
	claimed, err := s.store.ClaimNextDeploy(ctx, s.workerID)
	if err != nil {
		return err
	}
	execCtx, stopHeartbeat, heartbeatErrors := s.startHeartbeat(ctx, claimed.Record.ID)
	defer stopHeartbeat()
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: claimed.Release.ID,
		DeployRecordID:   claimed.Record.ID,
		EventType:        "deploy_started",
		ActorType:        "system",
		ActorID:          s.workerID,
		Message:          "发布执行已开始",
	})
	failed := false
	for _, target := range claimed.ExecutionTargets {
		if failed {
			break
		}
		if err := s.store.MarkTargetRunning(ctx, claimed.Record.ID, target.RefID); err != nil {
			return err
		}
		result := s.execute(execCtx, claimed, target)
		if err := heartbeatError(heartbeatErrors); err != nil {
			return err
		}
		if err := s.store.MarkTargetFinished(ctx, claimed.Record.ID, target.RefID, result); err != nil {
			return err
		}
		_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
			ReleaseRequestID: claimed.Release.ID,
			DeployRecordID:   claimed.Record.ID,
			EventType:        "target_finished",
			ActorType:        "system",
			ActorID:          s.workerID,
			Message:          result.Status + ": " + target.Name,
		})
		failed = result.Status == "failed"
	}
	if failed {
		if err := s.store.MarkQueuedTargetsSkipped(ctx, claimed.Record.ID, "skipped after previous server failure"); err != nil {
			return err
		}
	}
	if err := heartbeatError(heartbeatErrors); err != nil {
		return err
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

func (s Service) startHeartbeat(ctx context.Context, deployRecordID string) (context.Context, func(), <-chan error) {
	execCtx, cancel := context.WithCancel(ctx)
	stop := make(chan struct{})
	done := make(chan struct{})
	errs := make(chan error, 1)
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				if err := s.store.HeartbeatDeploy(ctx, deployRecordID, s.workerID); err != nil {
					errs <- err
					cancel()
					return
				}
			}
		}
	}()
	return execCtx, func() {
		close(stop)
		<-done
		cancel()
	}, errs
}

func heartbeatError(errs <-chan error) error {
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func (s Service) execute(ctx context.Context, claimed repository.ClaimedDeploy, target repository.ExecutionTarget) repository.ServerResult {
	item, ok := s.executors.Get(claimed.Target.ExecutorType)
	if !ok {
		code := 1
		return repository.ServerResult{
			Status:       "failed",
			ExitCode:     &code,
			ErrorCode:    "internal_error",
			ErrorMessage: "unsupported executor: " + claimed.Target.ExecutorType,
		}
	}
	server, serverFound := findClaimedServer(claimed.Servers, target)
	var gateway *domain.Server
	if claimed.Target.ExecutorType == "ssh" && !serverFound {
		code := 1
		return repository.ServerResult{Status: "failed", ExitCode: &code, ErrorCode: "target_not_found", ErrorMessage: "server target is not available"}
	}
	if claimed.Target.ExecutorType == "ssh" && server.GatewayID != "" {
		item, err := s.store.GetServer(ctx, server.GatewayID)
		if err != nil {
			code := 1
			return repository.ServerResult{Status: "failed", ExitCode: &code, ErrorCode: "connect_failed", ErrorMessage: "gateway server is not available"}
		}
		gateway = &item
	}
	return item.Execute(ctx, executor.Request{
		Release:         claimed.Release,
		Record:          claimed.Record,
		Target:          claimed.Target,
		Version:         claimed.Version,
		ExecutionTarget: target,
		Server:          server,
		Gateway:         gateway,
	})
}

func findClaimedServer(servers []domain.Server, target repository.ExecutionTarget) (domain.Server, bool) {
	for _, server := range servers {
		if server.ID == target.RefID {
			return server, true
		}
	}
	return domain.Server{ID: target.RefID, Name: target.Name}, false
}
