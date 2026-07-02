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
	Gateway *domain.Server
}

type Executor interface {
	Execute(ctx context.Context, req Request) repository.ServerResult
}

type Registry struct {
	items map[string]Executor
}

func NewRegistry(items map[string]Executor) Registry {
	copied := make(map[string]Executor, len(items))
	for key, item := range items {
		copied[key] = item
	}
	return Registry{items: copied}
}

func (r Registry) Get(executorType string) (Executor, bool) {
	item, ok := r.items[executorType]
	return item, ok
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
	ssh := sshTarget(req.Target)
	return strings.Contains(ssh.EnvVars, "MOCK_FAIL")
}

type CredentialResolver interface {
	Secret(ctx context.Context, id string) (repository.CredentialSecret, error)
}

type SSH struct {
	Credentials CredentialResolver
	Command     func(context.Context, string, ...string) *exec.Cmd
}

func (s SSH) Check(ctx context.Context, server domain.Server, gateway *domain.Server) repository.ServerResult {
	start := time.Now()
	if server.CredentialRef == "" || server.AuthType == "none" {
		return failedResult(start, "auth_failed", "ssh credential is required", nil)
	}
	secret, err := s.Credentials.Secret(ctx, server.CredentialRef)
	if err != nil {
		return failedResult(start, "auth_failed", "credential is not available", nil)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	args, commandEnv, cleanup, err := sshAuth(server.AuthType, secret, "AI_PUB_SSH_ASKPASS_PASSWORD")
	if err != nil {
		return failedResult(start, "auth_failed", sanitize(err.Error(), secret.Secret), nil)
	}
	defer cleanup()
	proxyArgs, proxyEnv, proxyCleanup, proxySecret, err := s.gatewayProxy(ctx, gateway)
	if err != nil {
		return failedResult(start, "auth_failed", sanitize(err.Error(), secret.Secret), nil)
	}
	defer proxyCleanup()
	args = append(args, proxyArgs...)
	args = append(args,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=15",
		"-p", fmt.Sprintf("%d", server.Port),
		server.Username+"@"+server.Host,
		"true",
	)
	cmd := s.command(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), commandEnv...)
	cmd.Env = append(cmd.Env, proxyEnv...)
	err = cmd.Run()
	output := sanitizeAll(stdout.String()+"\n"+stderr.String(), secret.Secret, proxySecret)
	if err != nil {
		code := 1
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return failedResult(start, "connect_timeout", "ssh connection timed out", &code)
		}
		return repository.ServerResult{Status: "failed", ExitCode: &code, DurationMS: int(time.Since(start).Milliseconds()), LogOutput: output, ErrorCode: "connect_failed", ErrorMessage: sanitizeAll(err.Error(), secret.Secret, proxySecret)}
	}
	code := 0
	return repository.ServerResult{Status: "success", ExitCode: &code, DurationMS: int(time.Since(start).Milliseconds()), LogOutput: output}
}

func (s SSH) Execute(ctx context.Context, req Request) repository.ServerResult {
	start := time.Now()
	ssh := sshTarget(req.Target)
	if ssh.ScriptPath == "" {
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
	args, commandEnv, cleanup, err := sshAuth(req.Server.AuthType, secret, "AI_PUB_SSH_ASKPASS_PASSWORD")
	if err != nil {
		return failedResult(start, "auth_failed", sanitize(err.Error(), secret.Secret), nil)
	}
	defer cleanup()
	proxyArgs, proxyEnv, proxyCleanup, proxySecret, err := s.gatewayProxy(ctx, req.Gateway)
	if err != nil {
		return failedResult(start, "auth_failed", sanitize(err.Error(), secret.Secret), nil)
	}
	defer proxyCleanup()
	command := ssh.ScriptPath
	command = remoteEnvPrefix(req) + command
	if ssh.WorkingDir != "" {
		command = "cd " + shellQuote(ssh.WorkingDir) + " && " + command
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args = append(args,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(timeout.Seconds())),
		"-p", fmt.Sprintf("%d", req.Server.Port),
	)
	args = append(args, proxyArgs...)
	args = append(args, req.Server.Username+"@"+req.Server.Host, command)
	cmd := s.command(ctx, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), commandEnv...)
	cmd.Env = append(cmd.Env, proxyEnv...)
	for key, value := range executionEnv(req) {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	runErr := cmd.Run()
	output := sanitizeAll(stdout.String()+"\n"+stderr.String(), secret.Secret, proxySecret)
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
				ErrorMessage: sanitizeAll(runErr.Error(), secret.Secret, proxySecret),
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
			ErrorMessage: sanitizeAll(runErr.Error(), secret.Secret, proxySecret),
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

func (s SSH) command(ctx context.Context, args ...string) *exec.Cmd {
	if s.Command != nil {
		return s.Command(ctx, "ssh", args...)
	}
	return exec.CommandContext(ctx, "ssh", args...)
}

func sshAuth(authType string, secret repository.CredentialSecret, passwordEnv string) ([]string, []string, func(), error) {
	switch authType {
	case "private_key":
		keyPath, cleanup, err := privateKeyFile(authType, secret)
		if err != nil {
			return nil, nil, func() {}, err
		}
		return []string{"-o", "BatchMode=yes", "-i", keyPath}, nil, cleanup, nil
	case "password":
		if secret.Credential.Type != "password" {
			return nil, nil, func() {}, fmt.Errorf("credential type mismatch")
		}
		askpassPath, cleanup, err := askpassHelper(passwordEnv)
		if err != nil {
			return nil, nil, func() {}, err
		}
		return []string{
				"-o", "BatchMode=no",
				"-o", "NumberOfPasswordPrompts=1",
				"-o", "PreferredAuthentications=password,keyboard-interactive",
			}, []string{
				"SSH_ASKPASS=" + askpassPath,
				"SSH_ASKPASS_REQUIRE=force",
				"DISPLAY=ai-pub",
				passwordEnv + "=" + secret.Secret,
			}, cleanup, nil
	default:
		return nil, nil, func() {}, fmt.Errorf("unsupported ssh auth type %q", authType)
	}
}

func askpassHelper(passwordEnv string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "ai-pub-ssh-askpass-*")
	if err != nil {
		return "", func() {}, err
	}
	path := filepath.Join(dir, "askpass")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$"+passwordEnv+"\"\n"), 0o700); err != nil {
		os.RemoveAll(dir)
		return "", func() {}, err
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}

func (s SSH) gatewayProxy(ctx context.Context, gateway *domain.Server) ([]string, []string, func(), string, error) {
	if gateway == nil {
		return nil, nil, func() {}, "", nil
	}
	if gateway.Role != "gateway" || gateway.GatewayID != "" || !gateway.Enabled {
		return nil, nil, func() {}, "", fmt.Errorf("gateway server is not available")
	}
	if gateway.CredentialRef == "" || gateway.AuthType == "none" {
		return nil, nil, func() {}, "", fmt.Errorf("gateway ssh credential is required")
	}
	secret, err := s.Credentials.Secret(ctx, gateway.CredentialRef)
	if err != nil {
		return nil, nil, func() {}, "", fmt.Errorf("gateway credential is not available")
	}
	authArgs, commandEnv, cleanup, err := sshAuth(gateway.AuthType, secret, "AI_PUB_SSH_GATEWAY_ASKPASS_PASSWORD")
	if err != nil {
		return nil, nil, func() {}, "", err
	}
	proxyArgs := append([]string{"ssh"}, authArgs...)
	proxyArgs = append(proxyArgs,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=15",
		"-W", "%h:%p",
		gateway.Username+"@"+gateway.Host,
	)
	proxyCommand := "env " + shellQuoteJoin(gatewayProxyEnv(commandEnv)) + " " + shellQuoteJoin(proxyArgs)
	return []string{"-o", "ProxyCommand=" + proxyCommand}, gatewayProcessEnv(commandEnv), cleanup, secret.Secret, nil
}

func gatewayProxyEnv(commandEnv []string) []string {
	result := make([]string, 0, len(commandEnv))
	for _, value := range commandEnv {
		if !strings.HasPrefix(value, "AI_PUB_SSH_GATEWAY_ASKPASS_PASSWORD=") {
			result = append(result, value)
		}
	}
	return result
}

func gatewayProcessEnv(commandEnv []string) []string {
	result := make([]string, 0, len(commandEnv))
	for _, value := range commandEnv {
		if strings.HasPrefix(value, "AI_PUB_SSH_GATEWAY_ASKPASS_PASSWORD=") {
			result = append(result, value)
		}
	}
	return result
}

func shellQuoteJoin(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, shellQuote(value))
	}
	return strings.Join(parts, " ")
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
	env := targetEnv(sshTarget(req.Target).EnvVars)
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

func sshTarget(target domain.DeploymentTarget) domain.SSHDeploymentTarget {
	if target.SSH == nil {
		return domain.SSHDeploymentTarget{
			TargetType:  target.TargetType,
			TargetRefID: target.TargetRefID,
			ScriptPath:  target.ScriptPath,
			WorkingDir:  target.WorkingDir,
			EnvVars:     target.EnvVars,
		}
	}
	return *target.SSH
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

func sanitizeAll(value string, secrets ...string) string {
	for _, secret := range secrets {
		value = sanitize(value, secret)
	}
	return value
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
