// SPDX-License-Identifier: GPL-3.0-or-later

package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

func (dst Config) validateWebhook() error {
	reference, err := secret.IsReference(dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if !reference {
		if err := field.URL(dst.URL); err != nil {
			return err
		}
	}
	if _, err := secret.IsReference(dst.BearerToken); err != nil {
		return fmt.Errorf("destination.bearer_token: %w", err)
	}
	if strings.ContainsAny(dst.BearerToken, "\r\n") {
		return errors.New("destination.bearer_token must not contain line breaks")
	}
	return nil
}

func postJSON(ctx context.Context, client *http.Client, dst Config, message any) error {
	endpoint, err := secret.Resolve(ctx, dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if err := field.URL(endpoint); err != nil {
		return err
	}
	var token string
	if dst.BearerToken != "" {
		token, err = secret.Resolve(ctx, dst.BearerToken)
		if err != nil {
			return fmt.Errorf("destination.bearer_token: %w", err)
		}
		if strings.ContainsAny(token, "\r\n") {
			return errors.New("destination.bearer_token must not contain line breaks")
		}
	}
	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	response, err := httpclient.PostJSON(ctx, client, "webhook", endpoint, headers, message)
	if err != nil {
		return err
	}
	// These providers acknowledge delivery through HTTP status, not the response body.
	// Close without buffering or draining an arbitrary remote body.
	defer response.Body.Close()
	accepted := response.StatusCode >= 200 && response.StatusCode < 300
	if !accepted {
		return fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}

func sendWebhook(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
	return postJSON(ctx, client, dst, event)
}

type Config struct {
	URL         string `yaml:"url,omitempty"`
	BearerToken string `yaml:"bearer_token,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validateWebhook(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("webhook HTTP client is required")
	}
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendWebhook(ctx, s.config, event, s.client)
}
