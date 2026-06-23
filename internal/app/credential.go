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

// UpdateCredentialInput 描述可编辑字段；type 与 secret 创建后不可改。
type UpdateCredentialInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Enabled     *bool   `json:"enabled"`
}

func (s CredentialService) Update(ctx context.Context, id string, input UpdateCredentialInput) (domain.Credential, error) {
	existing, err := s.store.GetCredential(ctx, id)
	if err != nil {
		return domain.Credential{}, err
	}
	if input.Name != nil {
		existing.Name = *input.Name
	}
	if input.Description != nil {
		existing.Description = *input.Description
	}
	if input.Enabled != nil {
		existing.Enabled = *input.Enabled
	}
	return s.store.UpdateCredential(ctx, id, existing)
}

// Delete 删除凭据；若仍被服务器引用返回 ErrCredentialInUse，由 HTTP 层映射 409。
func (s CredentialService) Delete(ctx context.Context, id string) error {
	if _, err := s.store.GetCredential(ctx, id); err != nil {
		return err
	}
	return s.store.DeleteCredential(ctx, id)
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
