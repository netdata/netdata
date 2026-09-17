// SPDX-License-Identifier: GPL-3.0-or-later

package pushbullet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"unicode"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
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

func (dst Config) validatePushbullet() error {
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
		return field.APIBase(value, "pushbullet", "api.pushbullet.com")
	}
	return field.Token(value, "pushbullet", "access_token")
}

func sendPushbullet(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
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

func renderPushbullet(dst Config, event notifyevent.Event) pushbulletMessage {
	message := pushbulletMessage{Type: "note", Title: event.Node + " " + event.Status + ": " + event.Summary,
		Email: dst.Email, ChannelTag: dst.ChannelTag, SourceDeviceID: dst.SourceDeviceID}
	if event.URL != "" {
		message.Type, message.URL = "link", event.URL
	}
	message.Body = notifymsg.PlainText(event, false)
	return message
}

type Config struct {
	AccessToken    string `yaml:"access_token,omitempty"`
	Email          string `yaml:"email,omitempty"`
	ChannelTag     string `yaml:"channel_tag,omitempty"`
	SourceDeviceID string `yaml:"source_device_id,omitempty"`
	APIURL         string `yaml:"api_url,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validatePushbullet(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("pushbullet HTTP client is required")
	}
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendPushbullet(ctx, s.config, event, s.client)
}
