package app

import (
	"context"
	"errors"

	"ai-pub/internal/crypto"
	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

type CredentialService struct {
	store repository.Store
	box   crypto.Box
}

type CreateCredentialInput struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Secret      string `json:"secret"`
	Description string `json:"description"`
}

func NewCredentialService(store repository.Store, box crypto.Box) CredentialService {
	return CredentialService{store: store, box: box}
}

func (s CredentialService) Create(ctx context.Context, input CreateCredentialInput) (domain.Credential, error) {
	if input.Secret == "" {
		return domain.Credential{}, errors.New("secret is required")
	}
	if input.Type != "password" && input.Type != "private_key" {
		return domain.Credential{}, errors.New("credential type must be password or private_key")
	}
	enc, err := s.box.Encrypt(input.Secret)
	if err != nil {
		return domain.Credential{}, err
	}
	return s.store.CreateCredential(ctx, domain.Credential{
		Name:        input.Name,
		Type:        input.Type,
		Description: input.Description,
	}, enc)
}

func (s CredentialService) List(ctx context.Context) ([]domain.Credential, error) {
	return s.store.ListCredentials(ctx)
}

func (s CredentialService) Secret(ctx context.Context, id string) (repository.CredentialSecret, error) {
	item, err := s.store.GetCredentialSecret(ctx, id)
	if err != nil {
		return repository.CredentialSecret{}, err
	}
	secret, err := s.box.Decrypt(item.Secret)
	if err != nil {
		return repository.CredentialSecret{}, err
	}
	item.Secret = secret
	return item, nil
}
