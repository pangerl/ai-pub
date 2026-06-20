package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-pub/internal/crypto"
	"ai-pub/internal/domain"
	"ai-pub/internal/notification"
	"ai-pub/internal/repository"
)

type NotificationService struct {
	store  repository.Store
	box    crypto.Box
	sender notification.WeComRobot
}

type CreateNotificationConfigInput struct {
	Name       string `json:"name"`
	Channel    string `json:"channel"`
	WebhookURL string `json:"webhook_url"`
	Secret     string `json:"secret"`
}

type PatchNotificationConfigInput struct {
	Name       string `json:"name"`
	WebhookURL string `json:"webhook_url"`
	Secret     string `json:"secret"`
	Enabled    *bool  `json:"enabled"`
}

type NotificationEvent struct {
	EventType        string
	ReleaseRequestID string
	DeployRecordID   string
	Content          string
}

func NewNotificationService(store repository.Store, box crypto.Box, client *http.Client) NotificationService {
	return NotificationService{store: store, box: box, sender: notification.WeComRobot{Client: client}}
}

func (s NotificationService) CreateConfig(ctx context.Context, input CreateNotificationConfigInput) (domain.NotificationConfig, error) {
	if input.Channel == "" {
		input.Channel = "wecom_robot"
	}
	if input.Channel != "wecom_robot" {
		return domain.NotificationConfig{}, errors.New("only wecom_robot is supported")
	}
	if input.WebhookURL == "" {
		return domain.NotificationConfig{}, errors.New("webhook_url is required")
	}
	webhookEnc, err := s.box.Encrypt(input.WebhookURL)
	if err != nil {
		return domain.NotificationConfig{}, err
	}
	secretEnc := ""
	if input.Secret != "" {
		secretEnc, err = s.box.Encrypt(input.Secret)
		if err != nil {
			return domain.NotificationConfig{}, err
		}
	}
	return s.store.CreateNotificationConfig(ctx, domain.NotificationConfig{
		Name:    input.Name,
		Channel: input.Channel,
	}, webhookEnc, secretEnc)
}

func (s NotificationService) ListConfigs(ctx context.Context) ([]domain.NotificationConfig, error) {
	return s.store.ListNotificationConfigs(ctx)
}

func (s NotificationService) ListDeliveries(ctx context.Context) ([]domain.NotificationDelivery, error) {
	return s.store.ListNotificationDeliveries(ctx)
}

func (s NotificationService) PatchConfig(ctx context.Context, id string, input PatchNotificationConfigInput) (domain.NotificationConfig, error) {
	patch := repository.NotificationConfigPatch{
		Name:    input.Name,
		Enabled: input.Enabled,
	}
	if input.WebhookURL != "" {
		webhookEnc, err := s.box.Encrypt(input.WebhookURL)
		if err != nil {
			return domain.NotificationConfig{}, err
		}
		patch.WebhookURL = webhookEnc
	}
	if input.Secret != "" {
		secretEnc, err := s.box.Encrypt(input.Secret)
		if err != nil {
			return domain.NotificationConfig{}, err
		}
		patch.Secret = secretEnc
	}
	return s.store.UpdateNotificationConfig(ctx, id, patch)
}

func (s NotificationService) Test(ctx context.Context, configID string) (domain.NotificationDelivery, error) {
	config, err := s.decryptConfig(ctx, configID)
	if err != nil {
		return domain.NotificationDelivery{}, err
	}
	return s.sendOne(ctx, config, NotificationEvent{
		EventType: "notification_test",
		Content:   "ai-pub 通知测试",
	})
}

func (s NotificationService) NotifyAll(ctx context.Context, event NotificationEvent) {
	configs, err := s.store.ListEnabledNotificationSecrets(ctx)
	if err != nil {
		return
	}
	for _, config := range configs {
		decrypted, err := s.decryptSecret(config)
		if err != nil {
			delivery, _ := s.store.CreateNotificationDelivery(ctx, domain.NotificationDelivery{
				ConfigID:         config.Config.ID,
				EventType:        event.EventType,
				ReleaseRequestID: event.ReleaseRequestID,
				DeployRecordID:   event.DeployRecordID,
				Status:           "failed",
				AttemptCount:     1,
				LastError:        err.Error(),
			})
			s.recordDeliveryEvent(ctx, delivery)
			continue
		}
		_, _ = s.sendOne(ctx, decrypted, event)
	}
}

func (s NotificationService) decryptConfig(ctx context.Context, id string) (repository.NotificationSecret, error) {
	config, err := s.store.GetNotificationSecret(ctx, id)
	if err != nil {
		return repository.NotificationSecret{}, err
	}
	return s.decryptSecret(config)
}

func (s NotificationService) decryptSecret(config repository.NotificationSecret) (repository.NotificationSecret, error) {
	webhook, err := s.box.Decrypt(config.WebhookURL)
	if err != nil {
		return repository.NotificationSecret{}, err
	}
	secret := ""
	if config.Secret != "" {
		secret, err = s.box.Decrypt(config.Secret)
		if err != nil {
			return repository.NotificationSecret{}, err
		}
	}
	config.WebhookURL = webhook
	config.Secret = secret
	return config, nil
}

func (s NotificationService) sendOne(ctx context.Context, config repository.NotificationSecret, event NotificationEvent) (domain.NotificationDelivery, error) {
	err := s.sender.Send(ctx, config.WebhookURL, event.Content)
	if err != nil {
		err = errors.New(sanitizeNotificationError(err.Error(), config.WebhookURL, config.Secret))
	}
	delivery := domain.NotificationDelivery{
		ConfigID:         config.Config.ID,
		EventType:        event.EventType,
		ReleaseRequestID: event.ReleaseRequestID,
		DeployRecordID:   event.DeployRecordID,
		AttemptCount:     1,
	}
	if err != nil {
		delivery.Status = "failed"
		delivery.LastError = err.Error()
		saved, saveErr := s.store.CreateNotificationDelivery(ctx, delivery)
		if saveErr != nil {
			return domain.NotificationDelivery{}, saveErr
		}
		s.recordDeliveryEvent(ctx, saved)
		return saved, err
	}
	delivery.Status = "sent"
	delivery.SentAt = time.Now().UTC()
	saved, err := s.store.CreateNotificationDelivery(ctx, delivery)
	if err != nil {
		return domain.NotificationDelivery{}, err
	}
	s.recordDeliveryEvent(ctx, saved)
	return saved, nil
}

func sanitizeNotificationError(message string, webhookURL string, secret string) string {
	for _, value := range []string{webhookURL, secret} {
		if value != "" {
			message = strings.ReplaceAll(message, value, "[redacted]")
		}
	}
	return message
}

func (s NotificationService) recordDeliveryEvent(ctx context.Context, delivery domain.NotificationDelivery) {
	if delivery.ReleaseRequestID == "" {
		return
	}
	eventType := "notification_sent"
	message := fmt.Sprintf("通知已发送：%s", delivery.EventType)
	if delivery.Status == "failed" {
		eventType = "notification_failed"
		message = fmt.Sprintf("通知发送失败：%s", delivery.EventType)
		if delivery.LastError != "" {
			message += "：" + delivery.LastError
		}
	}
	_, _ = s.store.CreateReleaseEvent(ctx, domain.ReleaseEvent{
		ReleaseRequestID: delivery.ReleaseRequestID,
		DeployRecordID:   delivery.DeployRecordID,
		EventType:        eventType,
		ActorType:        "system",
		ActorID:          "notification",
		Message:          message,
		Metadata:         fmt.Sprintf(`{"notification_delivery_id":%q,"notification_event_type":%q}`, delivery.ID, delivery.EventType),
	})
}

func FailureContent(releaseID string, recordID string, summary string) string {
	return fmt.Sprintf("**发布失败**\n\n发布单：%s\n\n发布记录：%s\n\n摘要：%s", releaseID, recordID, summary)
}

type PendingConfirmData struct {
	ServiceName     string
	EnvironmentName string
	Version         string
	CreatedBy       string
	ReleaseID       string
}

func PendingConfirmContent(data PendingConfirmData) string {
	return fmt.Sprintf("【发布待确认】\n服务：%s\n环境：%s\n版本：%s\n申请人：%s\n发布单：%s\n请管理员进入发布中心确认。",
		data.ServiceName, data.EnvironmentName, data.Version, data.CreatedBy, data.ReleaseID)
}

type RollbackData struct {
	ServiceName       string
	EnvironmentName   string
	RollbackVersion   string
	OriginalReleaseID string
	ReleaseID         string
}

func RollbackContent(data RollbackData) string {
	return fmt.Sprintf("【回滚申请】\n服务：%s\n环境：%s\n回滚版本：%s\n原发布单：%s\n新发布单：%s",
		data.ServiceName, data.EnvironmentName, data.RollbackVersion, data.OriginalReleaseID, data.ReleaseID)
}
