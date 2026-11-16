// SPDX-License-Identifier: GPL-3.0-or-later

package pushover

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

const pushoverDefaultAPI = "https://api.pushover.net"

var pushoverKeyPattern = regexp.MustCompile(`^[A-Za-z0-9]{30}$`)

type pushoverMessage struct {
	Token     string `json:"token"`
	User      string `json:"user"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	HTML      int    `json:"html"`
	Priority  int    `json:"priority"`
	Timestamp int64  `json:"timestamp"`
	URL       string `json:"url,omitempty"`
	URLTitle  string `json:"url_title,omitempty"`
}

func (dst Config) validatePushover() error {
	for _, field := range []struct{ name, value string }{
		{"app_token", dst.AppToken}, {"user_key", dst.UserKey}, {"api_url", dst.APIURL},
	} {
		reference, err := dst.Secrets.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("pushover %s: %w", field.name, err)
		}
		if reference {
			continue
		}
		if err := validatePushoverField(field.name, field.value); err != nil {
			return err
		}
	}
	return nil
}

func validatePushoverField(name, value string) error {
	if name == "api_url" {
		return field.APIBase(value, "pushover", "api.pushover.net")
	}
	if !pushoverKeyPattern.MatchString(value) {
		return fmt.Errorf("pushover %s must contain 30 ASCII letters or digits", name)
	}
	return nil
}

func sendPushover(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"app_token", &dst.AppToken}, {"user_key", &dst.UserKey}, {"api_url", &dst.APIURL},
	} {
		value, err := dst.Secrets.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("pushover %s: %w", field.name, err)
		}
		if err := validatePushoverField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	base := dst.APIURL
	if base == "" {
		base = pushoverDefaultAPI
	}
	endpoint := strings.TrimRight(base, "/") + "/1/messages.json"
	response, err := httpclient.PostJSON(ctx, client, "pushover", endpoint, nil, renderPushover(dst, event))
	if err != nil {
		return err
	}
	return readPushoverResponse(response)
}

func readPushoverResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("pushover returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Status *int `json:"status"`
	}
	if err := httpclient.DecodeResponse("pushover", response.Body, &result); err != nil {
		return err
	}
	if result.Status == nil {
		return errors.New("invalid pushover response")
	}
	if *result.Status != 1 {
		return errors.New("pushover API rejected notification")
	}
	return nil
}

func renderPushover(dst Config, event notifyevent.Event) pushoverMessage {
	message := pushoverMessage{
		Token: dst.AppToken, User: dst.UserKey, HTML: 1, Timestamp: event.Timestamp.Unix(),
		Title: event.Node + " " + event.Status + ": " + event.Summary,
	}
	switch event.Status {
	case "CLEAR":
		message.Priority = -1
	case "CRITICAL":
		message.Priority = 1
	}
	if utf8.RuneCountInString(message.Title) > 250 {
		message.Title = string([]rune(message.Title)[:247]) + "..."
	}
	parts := []pushoverText{{text: event.Summary, tag: "b"}}
	if event.Info != "" {
		parts = append(parts, pushoverText{text: event.Info, tag: "i"})
	}
	for _, field := range notifymsg.Fields(event) {
		parts = append(parts, pushoverText{text: field.Name + ": " + field.Value})
	}
	message.Message = renderPushoverText(parts)
	if event.URL != "" && utf8.RuneCountInString(event.URL) <= 512 {
		message.URL, message.URLTitle = event.URL, "View Netdata"
	}
	return message
}

type pushoverText struct{ text, tag string }

func (part pushoverText) tags() (string, string) {
	if part.tag == "" {
		return "", ""
	}
	return "<" + part.tag + ">", "</" + part.tag + ">"
}

func renderPushoverText(parts []pushoverText) string {
	const limit = 1024
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		open, close := part.tags()
		lines = append(lines, open+html.EscapeString(part.text)+close)
	}
	full := strings.Join(lines, "\n")
	if utf8.RuneCountInString(full) <= limit {
		return full
	}

	// Preserve Bash's shortening while reserving room for the ellipsis and each closing tag.
	remaining := limit - 3
	var shortened strings.Builder
	for i, part := range parts {
		open, close := part.tags()
		separator := ""
		if i > 0 {
			separator = "\n"
		}
		overhead := len(separator + open + close) // Generated markup is ASCII.
		if overhead > remaining {
			break
		}
		remaining -= overhead
		shortened.WriteString(separator + open)
		for _, character := range part.text {
			escaped := html.EscapeString(string(character))
			size := utf8.RuneCountInString(escaped)
			if size > remaining {
				shortened.WriteString("..." + close)
				return shortened.String()
			}
			shortened.WriteString(escaped)
			remaining -= size
		}
		shortened.WriteString(close)
	}
	shortened.WriteString("...")
	return shortened.String()
}

type Config struct {
	Secrets  secret.InputMode `yaml:"-"`
	AppToken string           `yaml:"app_token,omitempty"`
	UserKey  string           `yaml:"user_key,omitempty"`
	APIURL   string           `yaml:"api_url,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validatePushover(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("pushover HTTP client is required")
	}
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendPushover(ctx, s.config, event, s.client)
}
