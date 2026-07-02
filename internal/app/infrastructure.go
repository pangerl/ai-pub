package app

import (
	"context"
	"errors"

	"ai-pub/internal/domain"
	"ai-pub/internal/executor"
	"ai-pub/internal/repository"
)

type InfrastructureService struct {
	store       repository.Store
	credentials CredentialService
}

type CreateK8sClusterInput struct {
	Name          string `json:"name"`
	CredentialRef string `json:"credential_ref"`
}

type UpdateK8sClusterInput struct {
	Name          *string `json:"name"`
	CredentialRef *string `json:"credential_ref"`
	Enabled       *bool   `json:"enabled"`
}

func NewInfrastructureService(store repository.Store, credentials CredentialService) InfrastructureService {
	return InfrastructureService{store: store, credentials: credentials}
}

func (s InfrastructureService) ListK8sClusters(ctx context.Context) ([]domain.K8sCluster, error) {
	return s.store.ListK8sClusters(ctx)
}

func (s InfrastructureService) CreateK8sCluster(ctx context.Context, input CreateK8sClusterInput) (domain.K8sCluster, error) {
	return s.store.CreateK8sCluster(ctx, domain.K8sCluster{
		Name:          input.Name,
		CredentialRef: input.CredentialRef,
	})
}

func (s InfrastructureService) UpdateK8sCluster(ctx context.Context, id string, input UpdateK8sClusterInput) (domain.K8sCluster, error) {
	existing, err := s.store.GetK8sCluster(ctx, id)
	if err != nil {
		return domain.K8sCluster{}, err
	}
	if input.Name != nil {
		existing.Name = *input.Name
	}
	if input.CredentialRef != nil {
		existing.CredentialRef = *input.CredentialRef
	}
	if input.Enabled != nil {
		existing.Enabled = *input.Enabled
	}
	return s.store.UpdateK8sCluster(ctx, id, existing)
}

func (s InfrastructureService) DeleteK8sCluster(ctx context.Context, id string) error {
	return s.store.DeleteK8sCluster(ctx, id)
}

func (s InfrastructureService) TestServer(ctx context.Context, id string) (domain.Server, repository.ServerResult, error) {
	server, err := s.store.GetServer(ctx, id)
	if err != nil {
		return domain.Server{}, repository.ServerResult{}, err
	}
	if server.AuthType != "private_key" && server.AuthType != "password" {
		return domain.Server{}, repository.ServerResult{}, errors.New("server must use password or private_key authentication")
	}
	var gateway *domain.Server
	if server.GatewayID != "" {
		item, err := s.store.GetServer(ctx, server.GatewayID)
		if err != nil {
			return domain.Server{}, repository.ServerResult{}, errors.New("gateway server is not available")
		}
		gateway = &item
	}
	result := (executor.SSH{Credentials: s.credentials}).Check(ctx, server, gateway)
	status := result.Status
	if _, err := s.store.UpdateServerCheck(ctx, id, status); err != nil {
		return domain.Server{}, repository.ServerResult{}, err
	}
	updated, err := s.store.GetServer(ctx, id)
	return updated, result, err
}

// TestServerConfig 对未落库的服务器配置做一次性连接校验，不更新 last_check 状态。
func (s InfrastructureService) TestServerConfig(ctx context.Context, server domain.Server) (repository.ServerResult, error) {
	if server.AuthType != "private_key" && server.AuthType != "password" {
		return repository.ServerResult{}, errors.New("server must use password or private_key authentication")
	}
	var gateway *domain.Server
	if server.GatewayID != "" {
		item, err := s.store.GetServer(ctx, server.GatewayID)
		if err != nil {
			return repository.ServerResult{}, errors.New("gateway server is not available")
		}
		gateway = &item
	}
	result := (executor.SSH{Credentials: s.credentials}).Check(ctx, server, gateway)
	return result, nil
}
