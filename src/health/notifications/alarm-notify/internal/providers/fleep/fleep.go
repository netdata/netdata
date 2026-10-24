// SPDX-License-Identifier: GPL-3.0-or-later

package fleep

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

type fleepMessage struct {
	Message string `json:"message"`
	User    string `json:"user,omitempty"`
}

func renderFleep(dst Config, event notifyevent.Event) fleepMessage {
	return fleepMessage{Message: notifymsg.PlainText(event, true), User: dst.Sender}
}

func (dst Config) validateFleep() error {
	if dst.Sender != "" && (strings.TrimSpace(dst.Sender) == "" || strings.Contains(dst.Sender, "${") ||
		strings.IndexFunc(dst.Sender, unicode.IsControl) >= 0) {
		return errors.New("fleep sender must be a nonempty literal name without controls")
	}
	reference, err := dst.Secrets.IsReference(dst.URL)
	if err != nil {
		return fmt.Errorf("fleep url: %w", err)
	}
	if !reference {
		if err := field.URL(dst.URL); err != nil {
			return err
		}
	}

	return nil
}

func postJSON(ctx context.Context, client *http.Client, dst Config, message any) error {
	endpoint, err := dst.Secrets.Resolve(ctx, dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if err := field.URL(endpoint); err != nil {
		return err
	}
	headers := http.Header{}
	response, err := httpclient.PostJSON(ctx, client, "fleep", endpoint, headers, message)
	if err != nil {
		return err
	}
	// These providers acknowledge delivery through HTTP status, not the response body.
	// Close without buffering or draining an arbitrary remote body.
	defer response.Body.Close()
	accepted := response.StatusCode == http.StatusOK
	if !accepted {
		return fmt.Errorf("fleep returned HTTP %d", response.StatusCode)
	}
	return nil
}

func sendFleep(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
	return postJSON(ctx, client, dst, renderFleep(dst, event))
}

type Config struct {
	Secrets secret.InputMode `yaml:"-"`
	URL     string           `yaml:"url,omitempty"`
	Sender  string           `yaml:"sender,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validateFleep(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("fleep HTTP client is required")
	}
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendFleep(ctx, s.config, event, s.client)
}
