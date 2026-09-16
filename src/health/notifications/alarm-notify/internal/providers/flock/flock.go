// SPDX-License-Identifier: GPL-3.0-or-later

package flock

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

type flockMessage struct {
	SendAs      flockSender       `json:"sendAs"`
	Text        string            `json:"text"`
	Attachments []flockAttachment `json:"attachments"`
}

type flockSender struct {
	Name string `json:"name"`
}

type flockAttachment struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       string `json:"color"`
	URL         string `json:"url,omitempty"`
}

func renderFlock(event notifyevent.Event) flockMessage {
	return flockMessage{
		SendAs: flockSender{Name: "netdata on " + event.Node},
		Text:   event.Node + " " + event.Status + ": " + event.Summary,
		Attachments: []flockAttachment{{
			Title: event.Alert, Description: notifymsg.PlainText(event, false),
			Color: notifymsg.StatusColor(event.Status), URL: event.URL,
		}},
	}
}

func (dst Config) validateFlock() error {
	reference, err := secret.IsReference(dst.URL)
	if err != nil {
		return fmt.Errorf("flock url: %w", err)
	}
	if !reference {
		if err := field.URL(dst.URL); err != nil {
			return err
		}
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
	headers := http.Header{}
	response, err := httpclient.PostJSON(ctx, client, "flock", endpoint, headers, message)
	if err != nil {
		return err
	}
	// These providers acknowledge delivery through HTTP status, not the response body.
	// Close without buffering or draining an arbitrary remote body.
	defer response.Body.Close()
	accepted := response.StatusCode == http.StatusOK
	if !accepted {
		return fmt.Errorf("flock returned HTTP %d", response.StatusCode)
	}
	return nil
}

func sendFlock(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
	return postJSON(ctx, client, dst, renderFlock(event))
}

type Config struct {
	URL string `yaml:"url,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validateFlock(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("flock HTTP client is required")
	}
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendFlock(ctx, s.config, event, s.client)
}
