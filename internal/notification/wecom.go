package notification

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type WeComRobot struct {
	Client *http.Client
}

func (s WeComRobot) Send(ctx context.Context, webhookURL string, secret string, content string) error {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	body, err := json.Marshal(map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": content,
		},
	})
	if err != nil {
		return err
	}
	if secret != "" {
		webhookURL, err = signedWebhookURL(webhookURL, secret, time.Now())
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("wecom webhook returned %d", resp.StatusCode)
	}
	var result struct {
		ErrCode *int   `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return fmt.Errorf("invalid wecom webhook response: %w", err)
	}
	if result.ErrCode == nil {
		return fmt.Errorf("invalid wecom webhook response: errcode is required")
	}
	if *result.ErrCode != 0 {
		return fmt.Errorf("wecom webhook error %d: %s", *result.ErrCode, result.ErrMsg)
	}
	return nil
}

func signedWebhookURL(webhookURL string, secret string, now time.Time) (string, error) {
	parsed, err := url.Parse(webhookURL)
	if err != nil {
		return "", fmt.Errorf("parse wecom webhook: %w", err)
	}
	timestamp := strconv.FormatInt(now.UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "\n" + secret))
	query := parsed.Query()
	query.Set("timestamp", timestamp)
	query.Set("sign", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
