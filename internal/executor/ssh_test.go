package executor

import (
	"context"
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

func TestSSHExecutorPasswordIsExplicitlyUnsupportedForCommandBackend(t *testing.T) {
	exec := SSH{Credentials: fakeCredentialResolver{secret: repository.CredentialSecret{
		Credential: domain.Credential{Type: "password"},
		Secret:     "top-secret",
	}}}
	result := exec.Execute(context.Background(), Request{
		Target: domain.DeploymentTarget{ScriptPath: "echo ok", TimeoutSeconds: 1},
		Server: domain.Server{AuthType: "password", CredentialRef: "cred_1"},
	})
	if result.Status != "failed" || result.ErrorCode != "auth_failed" {
		t.Fatalf("expected auth_failed, got %#v", result)
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
