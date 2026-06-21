package domain

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Service struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ServiceVersion struct {
	ID            string    `json:"id"`
	ServiceID     string    `json:"service_id"`
	Version       string    `json:"version"`
	CommitSHA     string    `json:"commit_sha"`
	ArtifactURL   string    `json:"artifact_url"`
	Source        string    `json:"source"`
	Metadata      string    `json:"metadata"`
	CreatedByType string    `json:"created_by_type"`
	CreatedByID   string    `json:"created_by_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type Environment struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	IsProduction bool      `json:"is_production"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Server struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Host            string     `json:"host"`
	Port            int        `json:"port"`
	Username        string     `json:"username"`
	AuthType        string     `json:"auth_type"`
	CredentialRef   string     `json:"credential_ref"`
	GatewayID       string     `json:"gateway_id"`
	Enabled         bool       `json:"enabled"`
	LastCheckStatus string     `json:"last_check_status"`
	LastCheckAt     *time.Time `json:"last_check_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ServerGroup struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	ServerIDs   []string  `json:"server_ids"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DeploymentTarget struct {
	ID             string    `json:"id"`
	ServiceID      string    `json:"service_id"`
	EnvironmentID  string    `json:"environment_id"`
	ExecutorType   string    `json:"executor_type"`
	TargetType     string    `json:"target_type"`
	TargetRefID    string    `json:"target_ref_id"`
	ScriptPath     string    `json:"script_path"`
	WorkingDir     string    `json:"working_dir"`
	EnvVars        string    `json:"env_vars"`
	TimeoutSeconds int       `json:"timeout_seconds"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type User struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	DisplayName    string    `json:"display_name"`
	Role           string    `json:"role"`
	Enabled        bool      `json:"enabled"`
	PasswordHash   string    `json:"-"`
	SessionVersion int       `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	OwnerType  string     `json:"owner_type"`
	OwnerID    string     `json:"owner_id"`
	Scopes     string     `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Enabled    bool       `json:"enabled"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Credential struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ReleasePolicy struct {
	ID                       string    `json:"id"`
	ScopeType                string    `json:"scope_type"`
	ScopeID                  string    `json:"scope_id"`
	ConfirmMode              string    `json:"confirm_mode"`
	ManualFreezeEnabled      bool      `json:"manual_freeze_enabled"`
	SSHRealtimeCheckRequired bool      `json:"ssh_realtime_check_required"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type ReleaseRequest struct {
	ID                 string    `json:"id"`
	ProjectID          string    `json:"project_id"`
	ServiceID          string    `json:"service_id"`
	EnvironmentID      string    `json:"environment_id"`
	ServiceVersionID   string    `json:"service_version_id"`
	DeploymentTargetID string    `json:"deployment_target_id"`
	Status             string    `json:"status"`
	Source             string    `json:"source"`
	IdempotencyKey     string    `json:"idempotency_key,omitempty"`
	CreatedByType      string    `json:"created_by_type"`
	CreatedByID        string    `json:"created_by_id"`
	AuthorizedByUserID string    `json:"authorized_by_user_id,omitempty"`
	ConfirmedByUserID  string    `json:"confirmed_by_user_id,omitempty"`
	ConfirmedAt        time.Time `json:"confirmed_at,omitempty"`
	RejectedByUserID   string    `json:"rejected_by_user_id,omitempty"`
	RejectedReason     string    `json:"rejected_reason,omitempty"`
	RollbackOfID       string    `json:"rollback_of_id,omitempty"`
	SummaryStatus      string    `json:"summary_status"`
	SummaryMessage     string    `json:"summary_message"`
	Metadata           string    `json:"metadata"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type DeployRecord struct {
	ID               string    `json:"id"`
	ReleaseRequestID string    `json:"release_request_id"`
	Status           string    `json:"status"`
	ExecutorType     string    `json:"executor_type"`
	TargetSnapshot   string    `json:"target_snapshot"`
	TotalServers     int       `json:"total_servers"`
	SuccessServers   int       `json:"success_servers"`
	FailedServers    int       `json:"failed_servers"`
	SkippedServers   int       `json:"skipped_servers"`
	WorkerID         string    `json:"worker_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ServerDeployLog struct {
	ID             string    `json:"id"`
	DeployRecordID string    `json:"deploy_record_id"`
	ServerID       string    `json:"server_id"`
	Status         string    `json:"status"`
	ExitCode       *int      `json:"exit_code,omitempty"`
	StartedAt      time.Time `json:"started_at,omitempty"`
	FinishedAt     time.Time `json:"finished_at,omitempty"`
	DurationMS     int       `json:"duration_ms"`
	LogOutput      string    `json:"log_output"`
	ErrorCode      string    `json:"error_code"`
	ErrorMessage   string    `json:"error_message"`
}

type ServerDeploymentState struct {
	ID               string    `json:"id"`
	ServiceID        string    `json:"service_id"`
	EnvironmentID    string    `json:"environment_id"`
	ServerID         string    `json:"server_id"`
	ServiceVersionID string    `json:"service_version_id"`
	DeployRecordID   string    `json:"deploy_record_id"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ReleaseEvent struct {
	ID               string    `json:"id"`
	ReleaseRequestID string    `json:"release_request_id,omitempty"`
	DeployRecordID   string    `json:"deploy_record_id,omitempty"`
	EventType        string    `json:"event_type"`
	ActorType        string    `json:"actor_type"`
	ActorID          string    `json:"actor_id"`
	AuthorizedUserID string    `json:"authorized_user_id,omitempty"`
	APIKeyID         string    `json:"api_key_id,omitempty"`
	SourceIP         string    `json:"source_ip,omitempty"`
	Message          string    `json:"message"`
	Metadata         string    `json:"metadata"`
	CreatedAt        time.Time `json:"created_at"`
}

type NotificationConfig struct {
	ID        string    `json:"id"`
	Channel   string    `json:"channel"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type NotificationDelivery struct {
	ID               string    `json:"id"`
	ConfigID         string    `json:"config_id"`
	EventType        string    `json:"event_type"`
	ReleaseRequestID string    `json:"release_request_id,omitempty"`
	DeployRecordID   string    `json:"deploy_record_id,omitempty"`
	Status           string    `json:"status"`
	AttemptCount     int       `json:"attempt_count"`
	LastError        string    `json:"last_error"`
	SentAt           time.Time `json:"sent_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func NewID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return prefix + "_unknown"
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}
