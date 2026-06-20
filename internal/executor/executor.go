package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ai-pub/internal/domain"
	"ai-pub/internal/repository"
)

type Request struct {
	Release domain.ReleaseRequest
	Record  domain.DeployRecord
	Target  domain.DeploymentTarget
	Version domain.ServiceVersion
	Server  domain.Server
}

type Executor interface {
	Execute(ctx context.Context, req Request) repository.ServerResult
}

type Mock struct{}

func (Mock) Execute(ctx context.Context, req Request) repository.ServerResult {
	start := time.Now()
	select {
	case <-ctx.Done():
		code := 1
		return repository.ServerResult{
			Status:       "failed",
			ExitCode:     &code,
			DurationMS:   int(time.Since(start).Milliseconds()),
			ErrorCode:    "internal_error",
			ErrorMessage: "mock execution cancelled",
			LogOutput:    "mock execution cancelled",
		}
	default:
	}
	if mockShouldFail(req) {
		code := 1
		return repository.ServerResult{
			Status:       "failed",
			ExitCode:     &code,
			DurationMS:   int(time.Since(start).Milliseconds()),
			ErrorCode:    "exit_non_zero",
			ErrorMessage: "mock deployment failed",
			LogOutput:    "mock deployment failed on " + req.Server.Name,
		}
	}
	code := 0
	version := req.Version.Version
	if version == "" {
		version = req.Release.ServiceVersionID
	}
	return repository.ServerResult{
		Status:     "success",
		ExitCode:   &code,
		DurationMS: int(time.Since(start).Milliseconds()),
		LogOutput:  "mock deployment succeeded on " + req.Server.Name + " version " + version,
	}
}

func mockShouldFail(req Request) bool {
	env := executionEnv(req)
	if env["MOCK_FAIL_SERVER_ID"] != "" {
		return env["MOCK_FAIL_SERVER_ID"] == req.Server.ID
	}
	return strings.Contains(req.Target.EnvVars, "MOCK_FAIL")
}

type CredentialResolver interface {
	Secret(ctx context.Context, id string) (repository.CredentialSecret, error)
}

type SSH struct {
	Credentials CredentialResolver
}

func (s SSH) Execute(ctx context.Context, req Request) repository.ServerResult {
	start := time.Now()
	if req.Target.ScriptPath == "" {
		return failedResult(start, "script_not_found", "script_path is required", nil)
	}
	secret, err := s.Credentials.Secret(ctx, req.Server.CredentialRef)
	if err != nil {
		return failedResult(start, "auth_failed", "credential is not available", nil)
	}
	timeout := time.Duration(req.Target.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if req.Server.AuthType == "password" {
		return failedResult(start, "auth_failed", "password ssh execution requires an interactive askpass helper and is not enabled", nil)
	}
	keyPath, cleanup, err := privateKeyFile(req.Server.AuthType, secret)
	if err != nil {
		return failedResult(start, "auth_failed", sanitize(err.Error(), secret.Secret), nil)
	}
	defer cleanup()
	command := req.Target.ScriptPath
	command = remoteEnvPrefix(req) + command
	if req.Target.WorkingDir != "" {
		command = "cd " + shellQuote(req.Target.WorkingDir) + " && " + command
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(timeout.Seconds())),
		"-p", fmt.Sprintf("%d", req.Server.Port),
	}
	if keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	args = append(args, req.Server.Username+"@"+req.Server.Host, command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	for key, value := range executionEnv(req) {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	runErr := cmd.Run()
	output := sanitize(stdout.String()+"\n"+stderr.String(), secret.Secret)
	if runErr != nil {
		exitCode := 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			errorCode := "exit_non_zero"
			lowerOutput := strings.ToLower(output)
			if strings.Contains(lowerOutput, "permission denied") {
				errorCode = "auth_failed"
			} else if strings.Contains(lowerOutput, "connection refused") ||
				strings.Contains(lowerOutput, "could not resolve") ||
				strings.Contains(lowerOutput, "operation timed out") ||
				strings.Contains(lowerOutput, "no route to host") {
				errorCode = "connect_failed"
			}
			return repository.ServerResult{
				Status:       "failed",
				ExitCode:     &exitCode,
				DurationMS:   int(time.Since(start).Milliseconds()),
				LogOutput:    output,
				ErrorCode:    errorCode,
				ErrorMessage: sanitize(runErr.Error(), secret.Secret),
			}
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return failedResult(start, "command_timeout", "ssh command timed out", &exitCode)
		}
		return repository.ServerResult{
			Status:       "failed",
			ExitCode:     &exitCode,
			DurationMS:   int(time.Since(start).Milliseconds()),
			LogOutput:    output,
			ErrorCode:    "internal_error",
			ErrorMessage: sanitize(runErr.Error(), secret.Secret),
		}
	}
	exitCode := 0
	return repository.ServerResult{
		Status:     "success",
		ExitCode:   &exitCode,
		DurationMS: int(time.Since(start).Milliseconds()),
		LogOutput:  output,
	}
}

func privateKeyFile(authType string, secret repository.CredentialSecret) (string, func(), error) {
	switch authType {
	case "private_key":
		if secret.Credential.Type != "private_key" {
			return "", func() {}, fmt.Errorf("credential type mismatch")
		}
		dir, err := os.MkdirTemp("", "ai-pub-ssh-*")
		if err != nil {
			return "", func() {}, err
		}
		keyPath := filepath.Join(dir, "key")
		if err := os.WriteFile(keyPath, []byte(secret.Secret), 0o600); err != nil {
			os.RemoveAll(dir)
			return "", func() {}, err
		}
		return keyPath, func() { _ = os.RemoveAll(dir) }, nil
	default:
		return "", func() {}, fmt.Errorf("unsupported ssh auth type %q", authType)
	}
}

func failedResult(start time.Time, code string, message string, exitCode *int) repository.ServerResult {
	return repository.ServerResult{
		Status:       "failed",
		ExitCode:     exitCode,
		DurationMS:   int(time.Since(start).Milliseconds()),
		ErrorCode:    code,
		ErrorMessage: message,
		LogOutput:    message,
	}
}

func executionEnv(req Request) map[string]string {
	env := targetEnv(req.Target.EnvVars)
	env["AI_PUB_RELEASE_ID"] = req.Release.ID
	env["AI_PUB_DEPLOY_ID"] = req.Record.ID
	env["AI_PUB_SERVER_ID"] = req.Server.ID
	env["AI_PUB_SERVICE_ID"] = req.Release.ServiceID
	env["AI_PUB_ENVIRONMENT_ID"] = req.Release.EnvironmentID
	env["AI_PUB_SERVICE_VERSION_ID"] = req.Release.ServiceVersionID
	env["AI_PUB_VERSION"] = req.Version.Version
	env["AI_PUB_COMMIT_SHA"] = req.Version.CommitSHA
	env["AI_PUB_ARTIFACT_URL"] = req.Version.ArtifactURL
	return env
}

func targetEnv(raw string) map[string]string {
	env := map[string]string{}
	if raw == "" {
		return env
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return env
	}
	for key, value := range values {
		switch typed := value.(type) {
		case string:
			env[key] = typed
		case float64, bool:
			env[key] = fmt.Sprint(typed)
		}
	}
	return env
}

func remoteEnvPrefix(req Request) string {
	env := executionEnv(req)
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, key := range keys {
		value := env[key]
		parts = append(parts, key+"="+shellQuote(value))
	}
	return strings.Join(parts, " ") + " "
}

func sanitize(value string, secret string) string {
	if secret == "" {
		return value
	}
	return strings.ReplaceAll(value, secret, "[redacted]")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
