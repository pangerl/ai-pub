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

func NewInfrastructureService(store repository.Store, credentials CredentialService) InfrastructureService {
	return InfrastructureService{store: store, credentials: credentials}
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
