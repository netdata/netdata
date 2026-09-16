// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

const messagebirdDefaultAPI = "https://rest.messagebird.com"

func (dst Destination) validateMessageBird() error {
	if dst.URL != "" || dst.BearerToken != "" || dst.BotToken != "" || dst.AppToken != "" || dst.UserKey != "" ||
		dst.AccessToken != "" || dst.Email != "" || dst.ChannelTag != "" || dst.SourceDeviceID != "" ||
		dst.ChatID != "" || dst.MessageThreadID != nil || dst.RetriesOnLimit != nil ||
		dst.AccountSID != "" || dst.AuthToken != "" || dst.From != "" || dst.To != "" {
		return errors.New("messagebird destinations support access_key, originator, recipient and api_url only")
	}
	if dst.Originator == "" || strings.TrimSpace(dst.Originator) != dst.Originator ||
		strings.Contains(dst.Originator, "${") || strings.IndexFunc(dst.Originator, unicode.IsControl) != -1 {
		return errors.New(
			"messagebird originator must be a nonempty literal sender without surrounding whitespace or controls",
		)
	}
	if err := validatePhoneNumber(dst.Recipient, "messagebird", "recipient"); err != nil {
		return err
	}
	for _, field := range []struct{ name, value string }{{"access_key", dst.AccessKey}, {"api_url", dst.APIURL}} {
		reference, err := secret.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("messagebird %s: %w", field.name, err)
		}
		if !reference {
			if err := validateMessageBirdField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateMessageBirdField(name, value string) error {
	if name == "api_url" {
		return validateAPIBase(value, "messagebird", "rest.messagebird.com")
	}
	return validateToken(value, "messagebird", "access_key")
}

func sendMessageBird(ctx context.Context, dst Destination, event notifyevent.Event, timeout time.Duration) error {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"access_key", &dst.AccessKey}, {"api_url", &dst.APIURL},
	} {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("messagebird %s: %w", field.name, err)
		}
		if err := validateMessageBirdField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	base := dst.APIURL
	if base == "" {
		base = messagebirdDefaultAPI
	}
	endpoint := strings.TrimRight(base, "/") + "/messages"
	client := httpclient.New(timeout)
	defer client.CloseIdleConnections()
	headers := http.Header{"Authorization": {"AccessKey " + dst.AccessKey}, "Accept": {"application/json"}}
	response, err := httpclient.Post(
		ctx,
		client,
		"messagebird",
		endpoint,
		"application/x-www-form-urlencoded",
		headers,
		strings.NewReader(renderMessageBird(dst, event).Encode()),
	)
	if err != nil {
		return err
	}
	return readMessageBirdResponse(response)
}

func renderMessageBird(dst Destination, event notifyevent.Event) url.Values {
	return url.Values{"originator": {dst.Originator}, "recipients": {dst.Recipient},
		"body": {notifymsg.PlainText(event, true)}, "datacoding": {"auto"}}
}

func readMessageBirdResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("messagebird returned HTTP %d", response.StatusCode)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := httpclient.DecodeResponse("messagebird", response.Body, &result); err != nil {
		return err
	}
	if strings.TrimSpace(result.ID) == "" {
		return errors.New("invalid messagebird response")
	}
	return nil
}
