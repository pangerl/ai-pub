package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

type ReleaseService struct {
	store    repository.Store
	notifier releaseNotifier
}

type Actor struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	APIKeyID string `json:"api_key_id,omitempty"`
}

type releaseNotifier interface {
	NotifyAll(context.Context, NotificationEvent)
}

type PreflightInput struct {
	ServiceID          string `json:"service_id"`
	EnvironmentID      string `json:"environment_id"`
	ServiceVersionID   string `json:"service_version_id"`
	DeploymentTargetID string `json:"deployment_target_id"`
}

type PreflightResult struct {
	Result      string          `json:"result"`
	NextAction  string          `json:"next_action"`
	ConfirmMode string          `json:"confirm_mode"`
	Items       []PreflightItem `json:"items"`
}

type PreflightItem struct {
	Code    string `json:"code"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

type CreateReleaseInput struct {
	PreflightInput
	Source             string `json:"source"`
	IdempotencyKey     string `json:"idempotency_key"`
	CreatedByType      string `json:"created_by_type"`
	CreatedByID        string `json:"created_by_id"`
	AuthorizedByUserID string `json:"authorized_by_user_id"`
	APIKeyID           string `json:"api_key_id"`
	Metadata           string `json:"metadata"`
}

type ConfirmInput struct {
	UserID   string `json:"user_id"`
	APIKeyID string `json:"api_key_id"`
	Actor    Actor  `json:"-"`
}

type RejectInput struct {
	UserID   string `json:"user_id"`
	Reason   string `json:"reason"`
	APIKeyID string `json:"api_key_id"`
	Actor    Actor  `json:"-"`
}

type CancelInput struct {
	UserID   string `json:"user_id"`
	APIKeyID string `json:"api_key_id"`
	Actor    Actor  `json:"-"`
}

type RollbackInput struct {
	ServiceVersionID string `json:"service_version_id"`
	Source           string `json:"source"`
	CreatedByType    string `json:"created_by_type"`
	CreatedByID      string `json:"created_by_id"`
	IdempotencyKey   string `json:"idempotency_key"`
	APIKeyID         string `json:"api_key_id"`
}

type RetryInput struct {
	Source         string `json:"source"`
	CreatedByType  string `json:"created_by_type"`
	CreatedByID    string `json:"created_by_id"`
	IdempotencyKey string `json:"idempotency_key"`
	APIKeyID       string `json:"api_key_id"`
}

var ErrPreflightBlocked = errors.New("preflight blocked")
var ErrIdempotencyConflict = errors.New("idempotency key conflict")

func NewReleaseService(store repository.Store, notifiers ...releaseNotifier) ReleaseService {
	var notifier releaseNotifier
	if len(notifiers) > 0 {
		notifier = notifiers[0]
	}
	return ReleaseService{store: store, notifier: notifier}
}

func (s ReleaseService) Preflight(ctx context.Context, input PreflightInput) (PreflightResult, error) {
	env, err := s.store.GetEnvironment(ctx, input.EnvironmentID)
	if err != nil {
		return PreflightResult{}, err
	}
	version, err := s.store.GetServiceVersion(ctx, input.ServiceVersionID)
	if err != nil {
		return PreflightResult{}, err
	}
	target, err := s.store.GetDeploymentTarget(ctx, input.DeploymentTargetID)
	if err != nil {
		return PreflightResult{}, err
	}
	result := PreflightResult{
		Result:      "pass",
		ConfirmMode: "self_confirm",
		NextAction:  "self_confirm",
		Items:       []PreflightItem{},
	}
	if env.IsProduction {
		result.ConfirmMode = "admin_confirm"
		result.NextAction = "admin_confirm"
	}

	if version.ServiceID != input.ServiceID {
		result.block("version_service_mismatch", "版本不属于目标服务")
	}
	if target.ServiceID != input.ServiceID || target.EnvironmentID != input.EnvironmentID {
		result.block("target_mismatch", "部署目标与服务或环境不匹配")
	}
	if conflicts := reservedEnvConflicts(target.EnvVars); len(conflicts) > 0 {
		result.block("reserved_env_var", "部署目标环境变量不能覆盖系统变量: "+strings.Join(conflicts, ", "))
	}
	if version.ArtifactURL == "" {
		result.Items = append(result.Items, PreflightItem{
			Code:    "artifact_url_missing",
			Level:   "warning",
			Message: "版本未配置制品地址，部署脚本需要自行根据版本号解析制品",
		})
	}
	if env.ReleaseFrozen {
		result.block("environment_frozen", "当前环境已冻结发布")
	}
	running, err := s.store.CountRunningReleases(ctx, input.ServiceID, input.EnvironmentID)
	if err != nil {
		return PreflightResult{}, err
	}
	if running > 0 {
		result.block("running_conflict", "同服务同环境已有运行中发布")
	}
	if result.Result == "pass" {
		result.Items = append(result.Items, PreflightItem{Code: "target_ready", Level: "pass", Message: "部署目标配置完整"})
	}
	return result, nil
}

func reservedEnvConflicts(raw string) []string {
	if raw == "" {
		return nil
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	conflicts := []string{}
	for key := range values {
		if strings.HasPrefix(key, "AI_PUB_") {
			conflicts = append(conflicts, key)
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

func (s ReleaseService) PreflightExisting(ctx context.Context, id string, actor Actor) (PreflightResult, error) {
	release, err := s.store.GetReleaseRequest(ctx, id)
	if err != nil {
		return PreflightResult{}, err
	}
	result, err := s.Preflight(ctx, PreflightInput{
		ServiceID:          release.ServiceID,
		EnvironmentID:      release.EnvironmentID,
		ServiceVersionID:   release.ServiceVersionID,
		DeploymentTargetID: release.DeploymentTargetID,
	})
	if err != nil {
		return PreflightResult{}, err
	}
	actorType := chooseString(actor.Type, "system")
	actorID := chooseString(actor.ID, "preflight")
	s.recordPreflightEvent(ctx, release.ID, result, Actor{Type: actorType, ID: actorID, APIKeyID: actor.APIKeyID})
	return result, nil
}

func (s ReleaseService) Create(ctx context.Context, input CreateReleaseInput) (domain.ReleaseRequest, PreflightResult, error) {
	if input.IdempotencyKey != "" {
		existing, err := s.store.FindReleaseByIdempotency(ctx, input.IdempotencyKey)
		if err == nil {
			if input.Source == "" {
				input.Source = "web"
			}
			if input.CreatedByType == "" {
				input.CreatedByType = "user"
			}
			if !sameIdempotentCreate(existing, input) {
				return domain.ReleaseRequest{}, PreflightResult{}, ErrIdempotencyConflict
			}
			preflight, _ := s.Preflight(ctx, input.PreflightInput)
			return existing, preflight, nil
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return domain.ReleaseRequest{}, PreflightResult{}, err
		}
	}
	preflight, err := s.Preflight(ctx, input.PreflightInput)
	if err != nil {
		return domain.ReleaseRequest{}, PreflightResult{}, err
	}
	if preflight.Result == "block" {
		return domain.ReleaseRequest{}, preflight, ErrPreflightBlocked
	}
	service, err := s.store.GetService(ctx, input.ServiceID)
	if err != nil {
		return domain.ReleaseRequest{}, PreflightResult{}, err
	}
	if input.Source == "" {
		input.Source = "web"
	}
	if input.CreatedByType == "" {
		input.CreatedByType = "user"
	}
	item, err := s.store.CreateReleaseRequest(ctx, domain.ReleaseRequest{
		ProjectID:          service.ProjectID,
		ServiceID:          input.ServiceID,
		EnvironmentID:      input.EnvironmentID,
		ServiceVersionID:   input.ServiceVersionID,
		DeploymentTargetID: input.DeploymentTargetID,
		Source:             input.Source,
		IdempotencyKey:     input.IdempotencyKey,
		CreatedByType:      input.CreatedByType,
		CreatedByID:        input.CreatedByID,
		AuthorizedByUserID: input.AuthorizedByUserID,
		Metadata:           input.Metadata,
	})
	if err != nil {
		return domain.ReleaseRequest{}, preflight, err
	}
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: item.ID,
		EventType:        "release_created",
		ActorType:        input.CreatedByType,
		ActorID:          input.CreatedByID,
		APIKeyID:         input.APIKeyID,
		Message:          "发布单已创建",
	})
	s.recordPreflightEvent(ctx, item.ID, preflight, Actor{Type: input.CreatedByType, ID: input.CreatedByID, APIKeyID: input.APIKeyID})
	if preflight.ConfirmMode == "admin_confirm" {
		s.notifyProductionPending(ctx, item)
	}
	return item, preflight, nil
}

func sameIdempotentCreate(existing domain.ReleaseRequest, input CreateReleaseInput) bool {
	return existing.ServiceID == input.ServiceID &&
		existing.EnvironmentID == input.EnvironmentID &&
		existing.ServiceVersionID == input.ServiceVersionID &&
		existing.DeploymentTargetID == input.DeploymentTargetID &&
		existing.Source == input.Source &&
		existing.CreatedByType == input.CreatedByType &&
		existing.CreatedByID == input.CreatedByID
}

func (s ReleaseService) List(ctx context.Context) ([]domain.ReleaseRequest, error) {
	return s.store.ListReleaseRequests(ctx)
}

func (s ReleaseService) Get(ctx context.Context, id string) (domain.ReleaseRequest, error) {
	return s.store.GetReleaseRequest(ctx, id)
}

func (s ReleaseService) Confirm(ctx context.Context, id string, input ConfirmInput) (domain.ReleaseRequest, error) {
	release, err := s.store.GetReleaseRequest(ctx, id)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	if release.Status != "pending_confirm" {
		switch release.Status {
		case "queued", "running", "success", "failed", "partial":
			return release, nil
		default:
			return domain.ReleaseRequest{}, errors.New("release is not pending_confirm")
		}
	}
	preflight, err := s.Preflight(ctx, PreflightInput{
		ServiceID:          release.ServiceID,
		EnvironmentID:      release.EnvironmentID,
		ServiceVersionID:   release.ServiceVersionID,
		DeploymentTargetID: release.DeploymentTargetID,
	})
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	if preflight.Result == "block" {
		return domain.ReleaseRequest{}, ErrPreflightBlocked
	}
	actor := input.actor()
	confirmedByUserID := ""
	switch actor.Type {
	case "user":
		user, err := s.store.GetUser(ctx, actor.ID)
		if err != nil {
			return domain.ReleaseRequest{}, err
		}
		if !user.Enabled {
			return domain.ReleaseRequest{}, errors.New("user is disabled")
		}
		if preflight.ConfirmMode == "self_confirm" && (release.CreatedByType != "user" || release.CreatedByID != user.ID) {
			return domain.ReleaseRequest{}, errors.New("non-production release requires creator self confirmation")
		}
		if preflight.ConfirmMode == "admin_confirm" && user.Role != "admin" {
			return domain.ReleaseRequest{}, errors.New("production release requires admin confirmation")
		}
		confirmedByUserID = user.ID
	case "api_key":
		if preflight.ConfirmMode == "admin_confirm" {
			return domain.ReleaseRequest{}, errors.New("production release requires admin session confirmation")
		}
		if release.CreatedByType != "api_key" || release.CreatedByID != actor.ID {
			return domain.ReleaseRequest{}, errors.New("api key may only confirm its own release")
		}
	default:
		return domain.ReleaseRequest{}, errors.New("release actor is required")
	}
	target, err := s.store.GetDeploymentTarget(ctx, release.DeploymentTargetID)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	updated, record, err := s.store.ConfirmAndQueueRelease(ctx, id, confirmedByUserID, target)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: updated.ID,
		DeployRecordID:   record.ID,
		EventType:        "release_confirmed",
		ActorType:        actor.Type,
		ActorID:          actor.ID,
		APIKeyID:         actor.APIKeyID,
		Message:          "发布单已确认并入队",
	})
	return updated, nil
}

func (s ReleaseService) Reject(ctx context.Context, id string, input RejectInput) (domain.ReleaseRequest, error) {
	release, err := s.store.GetReleaseRequest(ctx, id)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	actor := input.actor()
	if err := validateReleaseActionActor(release, actor); err != nil {
		return domain.ReleaseRequest{}, err
	}
	updated, err := s.store.RejectRelease(ctx, id, actionUserID(actor), input.Reason)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: updated.ID,
		EventType:        "release_rejected",
		ActorType:        actor.Type,
		ActorID:          actor.ID,
		APIKeyID:         actor.APIKeyID,
		Message:          input.Reason,
	})
	return updated, nil
}

func (s ReleaseService) Cancel(ctx context.Context, id string, input CancelInput) (domain.ReleaseRequest, error) {
	current, err := s.store.GetReleaseRequest(ctx, id)
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	if current.Status == "cancelled" {
		return current, nil
	}
	if current.Status != "pending_confirm" && current.Status != "queued" {
		return current, nil
	}
	actor := input.actor()
	if err := validateReleaseActionActor(current, actor); err != nil {
		return domain.ReleaseRequest{}, err
	}
	updated, err := s.store.CancelRelease(ctx, id, actionUserID(actor))
	if err != nil {
		return domain.ReleaseRequest{}, err
	}
	if updated.Status != "cancelled" {
		return updated, nil
	}
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: updated.ID,
		EventType:        "release_cancelled",
		ActorType:        actor.Type,
		ActorID:          actor.ID,
		APIKeyID:         actor.APIKeyID,
		Message:          "发布单已取消",
	})
	return updated, nil
}

func (input ConfirmInput) actor() Actor {
	return releaseActionActor(input.Actor, input.UserID, input.APIKeyID)
}

func (input RejectInput) actor() Actor {
	return releaseActionActor(input.Actor, input.UserID, input.APIKeyID)
}

func (input CancelInput) actor() Actor {
	return releaseActionActor(input.Actor, input.UserID, input.APIKeyID)
}

func releaseActionActor(actor Actor, userID, apiKeyID string) Actor {
	if actor.Type != "" {
		return actor
	}
	if apiKeyID != "" {
		return Actor{Type: "api_key", ID: apiKeyID, APIKeyID: apiKeyID}
	}
	return Actor{Type: "user", ID: userID, APIKeyID: apiKeyID}
}

func validateReleaseActionActor(release domain.ReleaseRequest, actor Actor) error {
	if actor.Type == "user" && actor.ID != "" {
		return nil
	}
	if actor.Type == "api_key" && release.CreatedByType == "api_key" && release.CreatedByID == actor.ID {
		return nil
	}
	return errors.New("api key may only act on its own release")
}

func actionUserID(actor Actor) string {
	if actor.Type == "user" {
		return actor.ID
	}
	return ""
}

func (s ReleaseService) ListEvents(ctx context.Context, releaseID string) ([]domain.ReleaseEvent, error) {
	return s.store.ListReleaseEvents(ctx, releaseID)
}

func (s ReleaseService) RollbackCandidates(ctx context.Context, releaseID string) ([]domain.ServiceVersion, error) {
	return s.store.RollbackCandidates(ctx, releaseID)
}

func (s ReleaseService) CreateRollback(ctx context.Context, releaseID string, input RollbackInput) (domain.ReleaseRequest, PreflightResult, error) {
	original, err := s.store.GetReleaseRequest(ctx, releaseID)
	if err != nil {
		return domain.ReleaseRequest{}, PreflightResult{}, err
	}
	versionID := input.ServiceVersionID
	if versionID == "" {
		candidates, err := s.RollbackCandidates(ctx, releaseID)
		if err != nil {
			return domain.ReleaseRequest{}, PreflightResult{}, err
		}
		if len(candidates) == 0 {
			return domain.ReleaseRequest{}, PreflightResult{}, errors.New("no rollback candidates")
		}
		versionID = candidates[0].ID
	}
	item, preflight, err := s.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          original.ServiceID,
			EnvironmentID:      original.EnvironmentID,
			ServiceVersionID:   versionID,
			DeploymentTargetID: original.DeploymentTargetID,
		},
		Source:         chooseString(input.Source, "web"),
		IdempotencyKey: input.IdempotencyKey,
		CreatedByType:  chooseString(input.CreatedByType, "user"),
		CreatedByID:    input.CreatedByID,
		APIKeyID:       input.APIKeyID,
		Metadata:       `{"type":"rollback"}`,
	})
	if err != nil {
		return domain.ReleaseRequest{}, preflight, err
	}
	item.RollbackOfID = releaseID
	item, err = s.store.SetReleaseRollbackOf(ctx, item.ID, releaseID)
	if err != nil {
		return domain.ReleaseRequest{}, preflight, err
	}
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: item.ID,
		EventType:        "rollback_requested",
		ActorType:        chooseString(input.CreatedByType, "user"),
		ActorID:          input.CreatedByID,
		APIKeyID:         input.APIKeyID,
		Message:          "回滚发布单已创建",
	})
	s.notifyRollbackRequested(ctx, original, item)
	return item, preflight, nil
}

func (s ReleaseService) Retry(ctx context.Context, releaseID string, input RetryInput) (domain.ReleaseRequest, PreflightResult, error) {
	original, err := s.store.GetReleaseRequest(ctx, releaseID)
	if err != nil {
		return domain.ReleaseRequest{}, PreflightResult{}, err
	}
	if original.Status != "failed" && original.SummaryStatus != "partial" {
		return domain.ReleaseRequest{}, PreflightResult{}, errors.New("only failed or partial releases can be retried")
	}
	item, preflight, err := s.Create(ctx, CreateReleaseInput{
		PreflightInput: PreflightInput{
			ServiceID:          original.ServiceID,
			EnvironmentID:      original.EnvironmentID,
			ServiceVersionID:   original.ServiceVersionID,
			DeploymentTargetID: original.DeploymentTargetID,
		},
		Source:         chooseString(input.Source, "web"),
		IdempotencyKey: input.IdempotencyKey,
		CreatedByType:  chooseString(input.CreatedByType, "user"),
		CreatedByID:    input.CreatedByID,
		APIKeyID:       input.APIKeyID,
		Metadata:       fmt.Sprintf(`{"type":"retry","retry_of_id":%q}`, releaseID),
	})
	if err != nil {
		return domain.ReleaseRequest{}, preflight, err
	}
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: item.ID,
		EventType:        "release_retried",
		ActorType:        chooseString(input.CreatedByType, "user"),
		ActorID:          input.CreatedByID,
		APIKeyID:         input.APIKeyID,
		Message:          "重新发布单已创建",
	})
	return item, preflight, nil
}

func (r *PreflightResult) block(code string, message string) {
	r.Result = "block"
	r.Items = append(r.Items, PreflightItem{Code: code, Level: "block", Message: message})
}

func chooseString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (s ReleaseService) recordPreflightEvent(ctx context.Context, releaseID string, result PreflightResult, actor Actor) {
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: releaseID,
		EventType:        "preflight_checked",
		ActorType:        actor.Type,
		ActorID:          actor.ID,
		APIKeyID:         actor.APIKeyID,
		Message:          fmt.Sprintf("预检完成：%s", result.Result),
		Metadata:         fmt.Sprintf(`{"result":%q,"next_action":%q,"confirm_mode":%q}`, result.Result, result.NextAction, result.ConfirmMode),
	})
}

func (s ReleaseService) notifyProductionPending(ctx context.Context, release domain.ReleaseRequest) {
	if s.notifier == nil {
		return
	}
	labels := s.notificationLabels(ctx, release)
	s.notifier.NotifyAll(ctx, NotificationEvent{
		EventType:        "prod_pending_confirm",
		ReleaseRequestID: release.ID,
		Content: PendingConfirmContent(PendingConfirmData{
			ServiceName:     labels.serviceName,
			EnvironmentName: labels.environmentName,
			Version:         labels.version,
			CreatedBy:       labels.createdBy,
			ReleaseID:       release.ID,
		}),
	})
}

func (s ReleaseService) notifyRollbackRequested(ctx context.Context, original domain.ReleaseRequest, rollback domain.ReleaseRequest) {
	if s.notifier == nil {
		return
	}
	labels := s.notificationLabels(ctx, rollback)
	s.notifier.NotifyAll(ctx, NotificationEvent{
		EventType:        "rollback_requested",
		ReleaseRequestID: rollback.ID,
		Content: RollbackContent(RollbackData{
			ServiceName:       labels.serviceName,
			EnvironmentName:   labels.environmentName,
			RollbackVersion:   labels.version,
			OriginalReleaseID: original.ID,
			ReleaseID:         rollback.ID,
		}),
	})
}

type notificationLabels struct {
	serviceName     string
	environmentName string
	version         string
	createdBy       string
}

func (s ReleaseService) notificationLabels(ctx context.Context, release domain.ReleaseRequest) notificationLabels {
	labels := notificationLabels{
		serviceName:     release.ServiceID,
		environmentName: release.EnvironmentID,
		version:         release.ServiceVersionID,
		createdBy:       release.CreatedByID,
	}
	if service, err := s.store.GetService(ctx, release.ServiceID); err == nil && service.Name != "" {
		labels.serviceName = service.Name
	}
	if env, err := s.store.GetEnvironment(ctx, release.EnvironmentID); err == nil && env.Name != "" {
		labels.environmentName = env.Name
	}
	if version, err := s.store.GetServiceVersion(ctx, release.ServiceVersionID); err == nil && version.Version != "" {
		labels.version = version.Version
	}
	if release.CreatedByType == "user" && release.CreatedByID != "" {
		if user, err := s.store.GetUser(ctx, release.CreatedByID); err == nil {
			labels.createdBy = chooseString(user.DisplayName, user.Username)
		}
	}
	return labels
}
