// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type gotifyMessage struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

func (dst Destination) validateGotify() error {
	if dst != (Destination{Type: dst.Type, APIURL: dst.APIURL, AppToken: dst.AppToken}) {
		return errors.New("gotify destinations support api_url and app_token only")
	}
	for _, field := range []struct{ name, value string }{{"api_url", dst.APIURL}, {"app_token", dst.AppToken}} {
		reference, err := secretReference(field.value)
		if err != nil {
			return fmt.Errorf("gotify %s: %w", field.name, err)
		}
		if !reference {
			if err := validateGotifyField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateGotifyField(name, value string) error {
	if name == "api_url" {
		if value == "" {
			return errors.New("gotify api_url is required")
		}
		return validateAPIBase(value, "gotify", "")
	}
	return validateToken(value, "gotify", "app_token")
}

func sendGotify(ctx context.Context, dst Destination, event Event, timeout time.Duration) error {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"api_url", &dst.APIURL}, {"app_token", &dst.AppToken},
	} {
		value, err := resolveSecret(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("gotify %s: %w", field.name, err)
		}
		if err := validateGotifyField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	client := notificationHTTPClient(timeout)
	defer client.CloseIdleConnections()
	response, err := postNotificationJSON(ctx, client, "gotify", strings.TrimRight(dst.APIURL, "/")+"/message",
		http.Header{"X-Gotify-Key": {dst.AppToken}}, renderGotify(event))
	if err != nil {
		return err
	}
	return readGotifyResponse(response)
}

func renderGotify(event Event) gotifyMessage {
	priority := 1
	switch event.Status {
	case "WARNING":
		priority = 4
	case "CRITICAL":
		priority = 10
	}
	return gotifyMessage{Title: event.Node + " " + event.Status + ": " + event.Summary,
		Message: notificationPlainText(event, true), Priority: priority}
}

func readGotifyResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("gotify returned HTTP %d", response.StatusCode)
	}
	var result struct {
		ID uint64 `json:"id"`
	}
	if err := decodeNotificationResponse("gotify", response.Body, &result); err != nil {
		return err
	}
	if result.ID == 0 {
		return errors.New("invalid gotify response")
	}
	return nil
}
