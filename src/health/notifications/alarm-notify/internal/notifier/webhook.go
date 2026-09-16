// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

func sendWebhook(ctx context.Context, dst Destination, event notifyevent.Event, timeout time.Duration) error {
	return postJSON(ctx, dst, event, timeout)
}

func postJSON(ctx context.Context, dst Destination, message any, timeout time.Duration) error {
	endpoint, err := secret.Resolve(ctx, dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if err := validateURL(endpoint); err != nil {
		return err
	}
	if dst.Type == "discord" {
		endpoint, err = discordEndpoint(endpoint)
		if err != nil {
			return err
		}
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
	client := httpclient.New(timeout)
	defer client.CloseIdleConnections()
	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	response, err := httpclient.PostJSON(ctx, client, dst.Type, endpoint, headers, message)
	if err != nil {
		return err
	}
	if dst.Type == "rocketchat" {
		return readRocketChatResponse(response)
	}
	// These providers acknowledge delivery through HTTP status, not the response body.
	// Close without buffering or draining an arbitrary remote body.
	defer response.Body.Close()
	accepted := response.StatusCode == http.StatusOK
	switch dst.Type {
	case "webhook":
		accepted = response.StatusCode >= 200 && response.StatusCode < 300
	case "signl4":
		accepted = accepted || response.StatusCode == http.StatusCreated || response.StatusCode == http.StatusAccepted
	case "kafka":
		accepted = response.StatusCode == http.StatusNoContent
	}
	if !accepted {
		return fmt.Errorf("%s returned HTTP %d", dst.Type, response.StatusCode)
	}
	return nil
}
