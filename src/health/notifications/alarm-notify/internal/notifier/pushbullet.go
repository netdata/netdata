// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

const pushbulletDefaultAPI = "https://api.pushbullet.com"

type pushbulletMessage struct {
	Type           string `json:"type"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	URL            string `json:"url,omitempty"`
	Email          string `json:"email,omitempty"`
	ChannelTag     string `json:"channel_tag,omitempty"`
	SourceDeviceID string `json:"source_device_iden,omitempty"`
}

func (dst Destination) validatePushbullet() error {
	if dst.URL != "" || dst.BearerToken != "" || dst.BotToken != "" || dst.AppToken != "" || dst.UserKey != "" ||
		dst.ChatID != "" || dst.MessageThreadID != nil || dst.RetriesOnLimit != nil {
		return errors.New(
			"pushbullet destinations support access_token, email, channel_tag, source_device_id and api_url only",
		)
	}
	if (dst.Email == "") == (dst.ChannelTag == "") {
		return errors.New("pushbullet requires exactly one email or channel_tag")
	}
	if dst.Email != "" {
		address, err := mail.ParseAddress(dst.Email)
		if err != nil || address.Name != "" || address.Address != dst.Email || strings.ContainsAny(dst.Email, "\r\n") {
			return errors.New("pushbullet email must be a single address without a display name")
		}
	}
	if dst.ChannelTag != "" && (!validPushbulletIdentifier(dst.ChannelTag) || strings.HasPrefix(dst.ChannelTag, "#")) {
		return errors.New("pushbullet channel_tag must be one tag without a leading # or whitespace")
	}
	if dst.SourceDeviceID != "" && !validPushbulletIdentifier(dst.SourceDeviceID) {
		return errors.New("pushbullet source_device_id must be one identifier without whitespace")
	}
	for _, field := range []struct{ name, value string }{
		{"access_token", dst.AccessToken}, {"api_url", dst.APIURL},
	} {
		reference, err := secret.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("pushbullet %s: %w", field.name, err)
		}
		if !reference {
			if err := validatePushbulletField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validPushbulletIdentifier(value string) bool {
	return !strings.Contains(value, "${") && strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) == -1
}

func validatePushbulletField(name, value string) error {
	if name == "api_url" {
		return validateAPIBase(value, "pushbullet", "api.pushbullet.com")
	}
	return validateToken(value, "pushbullet", "access_token")
}

func sendPushbullet(ctx context.Context, dst Destination, event notifyevent.Event, timeout time.Duration) error {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"access_token", &dst.AccessToken}, {"api_url", &dst.APIURL},
	} {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("pushbullet %s: %w", field.name, err)
		}
		if err := validatePushbulletField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	base := dst.APIURL
	if base == "" {
		base = pushbulletDefaultAPI
	}
	endpoint := strings.TrimRight(base, "/") + "/v2/pushes"
	client := httpclient.New(timeout)
	defer client.CloseIdleConnections()
	headers := http.Header{"Access-Token": {dst.AccessToken}}
	response, err := httpclient.PostJSON(ctx, client, "pushbullet", endpoint, headers, renderPushbullet(dst, event))
	if err != nil {
		return err
	}
	return readPushbulletResponse(response)
}

func readPushbulletResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("pushbullet returned HTTP %d", response.StatusCode)
	}
	var result struct {
		ID    string          `json:"iden"`
		Error json.RawMessage `json:"error"`
	}
	if err := httpclient.DecodeResponse("pushbullet", response.Body, &result); err != nil {
		return err
	}
	if len(result.Error) > 0 && string(result.Error) != "null" {
		return errors.New("pushbullet API rejected notification")
	}
	if strings.TrimSpace(result.ID) == "" {
		return errors.New("invalid pushbullet response")
	}
	return nil
}

func renderPushbullet(dst Destination, event notifyevent.Event) pushbulletMessage {
	message := pushbulletMessage{Type: "note", Title: event.Node + " " + event.Status + ": " + event.Summary,
		Email: dst.Email, ChannelTag: dst.ChannelTag, SourceDeviceID: dst.SourceDeviceID}
	if event.URL != "" {
		message.Type, message.URL = "link", event.URL
	}
	message.Body = notifymsg.PlainText(event, false)
	return message
}
