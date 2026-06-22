package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

type fakeCredentialResolver struct {
	secret repository.CredentialSecret
}

func (f fakeCredentialResolver) Secret(context.Context, string) (repository.CredentialSecret, error) {
	return f.secret, nil
}

type credentialResolverByID map[string]repository.CredentialSecret

func (f credentialResolverByID) Secret(_ context.Context, id string) (repository.CredentialSecret, error) {
	return f[id], nil
}

func TestSSHExecutorPasswordUsesAskpassWithoutLeakingSecret(t *testing.T) {
	sshPath := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(sshPath, []byte(`#!/bin/sh
set -eu
test "$SSH_ASKPASS_REQUIRE" = force
test "$("$SSH_ASKPASS")" = "$AI_PUB_SSH_ASKPASS_PASSWORD"
printf 'password authentication succeeded\n'
`), 0o700); err != nil {
		t.Fatal(err)
	}
	exec := SSH{Credentials: fakeCredentialResolver{secret: repository.CredentialSecret{
		Credential: domain.Credential{Type: "password"},
		Secret:     "top-secret",
	}}, Command: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, sshPath, args...)
	}}
	result := exec.Execute(context.Background(), Request{
		Target: domain.DeploymentTarget{ScriptPath: "echo ok", TimeoutSeconds: 1},
		Server: domain.Server{Host: "example.test", Port: 22, Username: "deploy", AuthType: "password", CredentialRef: "cred_1"},
	})
	if result.Status != "success" || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("expected successful password authentication, got %#v", result)
	}
	if result.ErrorMessage == "top-secret" || result.LogOutput == "top-secret" {
		t.Fatalf("secret leaked in result %#v", result)
	}
}

func TestSSHExecutorPrivateKeyConnectionFailureIsReadable(t *testing.T) {
	exec := SSH{Credentials: fakeCredentialResolver{secret: repository.CredentialSecret{
		Credential: domain.Credential{Type: "private_key"},
		Secret:     "not-a-real-private-key",
	}}}
	result := exec.Execute(context.Background(), Request{
		Target: domain.DeploymentTarget{ScriptPath: "echo ok", TimeoutSeconds: 1},
		Server: domain.Server{
			Name:          "local-unused",
			Host:          "127.0.0.1",
			Port:          1,
			Username:      "deploy",
			AuthType:      "private_key",
			CredentialRef: "cred_1",
		},
	})
	if result.Status != "failed" {
		t.Fatalf("expected failed result, got %#v", result)
	}
}

func TestSSHExecutorUsesGatewayTunnelWithSeparateCredentials(t *testing.T) {
	var captured []string
	sshPath := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(sshPath, []byte(`#!/bin/sh
set -eu
test "$AI_PUB_SSH_ASKPASS_PASSWORD" = application-secret
test "$AI_PUB_SSH_GATEWAY_ASKPASS_PASSWORD" = gateway-secret
`), 0o700); err != nil {
		t.Fatal(err)
	}
	exec := SSH{Credentials: credentialResolverByID{
		"app-cred": {
			Credential: domain.Credential{Type: "password"},
			Secret:     "application-secret",
		},
		"gateway-cred": {
			Credential: domain.Credential{Type: "password"},
			Secret:     "gateway-secret",
		},
	}, Command: func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		captured = append([]string(nil), args...)
		return exec.CommandContext(ctx, sshPath, args...)
	}}
	result := exec.Execute(context.Background(), Request{
		Target: domain.DeploymentTarget{ScriptPath: "echo ok", TimeoutSeconds: 1},
		Server: domain.Server{
			Host: "app.internal", Port: 22, Username: "app-user", AuthType: "password", CredentialRef: "app-cred",
		},
		Gateway: &domain.Server{
			Role: "gateway", Host: "gateway.example", Port: 2202, Username: "gateway-user", AuthType: "password", CredentialRef: "gateway-cred", Enabled: true,
		},
	})
	if result.Status != "success" {
		t.Fatalf("expected successful command setup, got %#v", result)
	}
	command := strings.Join(captured, " ")
	for _, want := range []string{"ProxyCommand=env", "SSH_ASKPASS=", "gateway-user@gateway.example", "-W", "%h:%p", "app-user@app.internal"} {
		if !strings.Contains(command, want) {
			t.Fatalf("expected %q in ssh arguments: %s", want, command)
		}
	}
	if strings.Contains(command, "application-secret") || strings.Contains(command, "gateway-secret") {
		t.Fatalf("ssh arguments must not contain credentials: %s", command)
	}
}

func TestExecutionEnvInjectsServiceVersionAndTargetVars(t *testing.T) {
	env := executionEnv(Request{
		Release: domain.ReleaseRequest{
			ID:               "rel_1",
			ServiceID:        "svc_1",
			EnvironmentID:    "env_1",
			ServiceVersionID: "ver_1",
		},
		Record: domain.DeployRecord{ID: "deploy_1"},
		Target: domain.DeploymentTarget{
			EnvVars: `{"CUSTOM":"ok","AI_PUB_VERSION":"target-must-not-override","COUNT":2}`,
		},
		Version: domain.ServiceVersion{
			ID:          "ver_1",
			Version:     "v1.2.3",
			CommitSHA:   "abc123",
			ArtifactURL: "https://artifact.example/app.tar.gz",
		},
		Server: domain.Server{ID: "srv_1"},
	})
	for key, want := range map[string]string{
		"CUSTOM":                    "ok",
		"COUNT":                     "2",
		"AI_PUB_RELEASE_ID":         "rel_1",
		"AI_PUB_DEPLOY_ID":          "deploy_1",
		"AI_PUB_SERVER_ID":          "srv_1",
		"AI_PUB_SERVICE_ID":         "svc_1",
		"AI_PUB_ENVIRONMENT_ID":     "env_1",
		"AI_PUB_SERVICE_VERSION_ID": "ver_1",
		"AI_PUB_VERSION":            "v1.2.3",
		"AI_PUB_COMMIT_SHA":         "abc123",
		"AI_PUB_ARTIFACT_URL":       "https://artifact.example/app.tar.gz",
	} {
		if got := env[key]; got != want {
			t.Fatalf("expected %s=%q, got %q in %#v", key, want, got, env)
		}
	}
}
