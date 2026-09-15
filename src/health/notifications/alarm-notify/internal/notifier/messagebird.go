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
	digits := strings.TrimPrefix(dst.Recipient, "+")
	if digits == "" || strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
		return errors.New("messagebird recipient must be one phone number using digits and an optional leading +")
	}
	for _, field := range []struct{ name, value string }{{"access_key", dst.AccessKey}, {"api_url", dst.APIURL}} {
		reference, err := secretReference(field.value)
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

func sendMessageBird(ctx context.Context, dst Destination, event Event, timeout time.Duration) error {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"access_key", &dst.AccessKey}, {"api_url", &dst.APIURL},
	} {
		value, err := resolveSecret(ctx, *field.value)
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
	client := notificationHTTPClient(timeout)
	defer client.CloseIdleConnections()
	headers := http.Header{"Authorization": {"AccessKey " + dst.AccessKey}, "Accept": {"application/json"}}
	response, err := postNotification(
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

func renderMessageBird(dst Destination, event Event) url.Values {
	return url.Values{"originator": {dst.Originator}, "recipients": {dst.Recipient},
		"body": {notificationPlainText(event, true)}, "datacoding": {"auto"}}
}

func readMessageBirdResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		return fmt.Errorf("messagebird returned HTTP %d", response.StatusCode)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := decodeNotificationResponse("messagebird", response.Body, &result); err != nil {
		return err
	}
	if strings.TrimSpace(result.ID) == "" {
		return errors.New("invalid messagebird response")
	}
	return nil
}
