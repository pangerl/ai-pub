package app

import (
	"context"
	"errors"

	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

// 版本登记相关错误。
var (
	ErrVersionConflict = errors.New("version conflict")
	ErrServiceNotFound = errors.New("service not found")
	ErrServiceDisabled = errors.New("service disabled")
)

// VersionRegistrationInput 是外部 CI 登记版本的输入。
type VersionRegistrationInput struct {
	ProjectKey    string
	ServiceKey    string
	Version       string
	CommitSHA     string
	ArtifactURL   string
	Metadata      string
	IdempotencyKey string
	APIKeyID      string
}

// VersionRegistrationResult 是登记结果。
type VersionRegistrationResult struct {
	Version   domain.ServiceVersion
	Created   bool
}

// RegisterVersion 处理外部 CI 版本登记的幂等与冲突逻辑。
func (s ReleaseService) RegisterVersion(ctx context.Context, input VersionRegistrationInput) (VersionRegistrationResult, error) {
	// 按 project slug + service slug 定位既有服务，不自动创建。
	service, err := s.store.GetServiceByProjectSlugAndServiceSlug(ctx, input.ProjectKey, input.ServiceKey)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return VersionRegistrationResult{}, ErrServiceNotFound
		}
		return VersionRegistrationResult{}, err
	}
	if !service.Enabled {
		return VersionRegistrationResult{}, ErrServiceDisabled
	}

	requestHash := repository.RegistrationRequestHash(input.Version, input.CommitSHA, input.ArtifactURL)

	// 携带幂等键时优先按幂等键去重：CI 重试场景。
	if input.IdempotencyKey != "" {
		existing, err := s.store.FindServiceVersionByIdempotency(ctx, service.ID, input.IdempotencyKey)
		if err == nil {
			// 同幂等键、同指纹 → 返回已有版本（200）；不同指纹 → 409。
			if existing.RegistrationRequestHash != requestHash {
				return VersionRegistrationResult{}, ErrIdempotencyConflict
			}
			s.recordVersionRegisteredEvent(ctx, existing, input, false)
			return VersionRegistrationResult{Version: existing, Created: false}, nil
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return VersionRegistrationResult{}, err
		}
	}

	// 不同幂等键（或无幂等键）下，按 (service_id, version) 去重与冲突判断。
	if existing, err := s.store.FindServiceVersionByServiceAndVersion(ctx, service.ID, input.Version); err == nil {
		// 同版本、同 commit 与制品（指纹一致）→ 返回已有版本（200）。
		if existing.RegistrationRequestHash == requestHash {
			s.recordVersionRegisteredEvent(ctx, existing, input, false)
			return VersionRegistrationResult{Version: existing, Created: false}, nil
		}
		// 同版本但 commit 或制品不同 → 409。
		return VersionRegistrationResult{}, ErrVersionConflict
	} else if !errors.Is(err, repository.ErrNotFound) {
		return VersionRegistrationResult{}, err
	}

	// 首次登记：创建版本。
	version := domain.ServiceVersion{
		ServiceID:                  service.ID,
		Version:                    input.Version,
		CommitSHA:                  input.CommitSHA,
		ArtifactURL:                input.ArtifactURL,
		Source:                     "ci",
		Metadata:                   input.Metadata,
		CreatedByType:              "api_key",
		CreatedByID:                input.APIKeyID,
		RegistrationIdempotencyKey: input.IdempotencyKey,
		RegistrationRequestHash:    requestHash,
	}
	created, err := s.store.CreateServiceVersion(ctx, version)
	if err != nil {
		return VersionRegistrationResult{}, err
	}
	s.recordVersionRegisteredEvent(ctx, created, input, true)
	return VersionRegistrationResult{Version: created, Created: true}, nil
}

// recordVersionRegisteredEvent 写入 version_registered 审计事件。
func (s ReleaseService) recordVersionRegisteredEvent(ctx context.Context, version domain.ServiceVersion, input VersionRegistrationInput, created bool) {
	message := "外部 CI 登记版本"
	if !created {
		message = "外部 CI 版本登记命中幂等/同版本，返回已有版本"
	}
	_, _ = s.store.CreateServiceVersionEvent(ctx, domain.ServiceVersionEvent{
		ServiceVersionID:           version.ID,
		EventType:                  "version_registered",
		ActorType:                  "api_key",
		ActorID:                    input.APIKeyID,
		APIKeyID:                   input.APIKeyID,
		RegistrationIdempotencyKey: input.IdempotencyKey,
		Message:                    message,
		Metadata:                   version.Metadata,
	})
}
