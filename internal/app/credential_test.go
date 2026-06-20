package app

import (
	"context"
	"testing"

	"ai-pub/internal/crypto"
)

func TestCredentialServiceEncryptsAndListsWithoutSecret(t *testing.T) {
	_, store := newReleaseTestStore(t)
	service := NewCredentialService(store, crypto.NewBox("test-key"))

	created, err := service.Create(context.Background(), CreateCredentialInput{
		Name:   "deploy password",
		Type:   "password",
		Secret: "super-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("expected listed credential without secret, got %#v", list)
	}
	secret, err := service.Secret(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secret.Secret != "super-secret" {
		t.Fatalf("unexpected decrypted secret %q", secret.Secret)
	}
}
