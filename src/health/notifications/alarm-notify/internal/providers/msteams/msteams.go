// SPDX-License-Identifier: GPL-3.0-or-later

package msteams

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"unicode"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

const msTeamsPayloadLimit = 28 * 1024

func (dst Config) validateMSTeams() error {
	for name, values := range map[string]map[string]string{"icons": dst.Icons, "colors": dst.Colors} {
		for status, value := range values {
			if status != "warning" && status != "critical" && status != "clear" {
				return fmt.Errorf("msteams %s keys must be warning, critical or clear", name)
			}
			if name == "colors" {
				if value != "" && (len(value) != 6 || strings.Trim(value, "0123456789abcdefABCDEF") != "") {
					return errors.New("msteams colors must be six hexadecimal digits or empty")
				}
			} else if strings.Contains(value, "${") || strings.IndexFunc(value, unicode.IsControl) >= 0 {
				return errors.New("msteams icons must be literal text without controls")
			}
		}
	}
	reference, err := dst.Secrets.IsReference(dst.URL)
	if err != nil {
		return fmt.Errorf("msteams url: %w", err)
	}
	if !reference {
		return field.URL(dst.URL)
	}
	return nil
}

type msTeamsMessage struct {
	Context    string `json:"@context"`
	Type       string `json:"@type"`
	ThemeColor string `json:"themeColor,omitempty"`
	Title      string `json:"title"`
	Text       string `json:"text"`
}

func renderMSTeams(dst Config, event notifyevent.Event) msTeamsMessage {
	icon, color := "⚠️", "FFA500"
	switch event.Status {
	case "CRITICAL":
		icon, color = "🔥", "D93F3C"
	case "CLEAR":
		icon, color = "💚", "65A677"
	}
	if value, ok := dst.Icons[strings.ToLower(event.Status)]; ok {
		icon = value
	}
	if value, ok := dst.Colors[strings.ToLower(event.Status)]; ok {
		color = value
	}
	title := "Alert " + event.Status + " from Netdata on " + event.Node
	if icon != "" {
		title = icon + " " + title
	}
	text := strings.ReplaceAll(escapeMSTeamsMarkdown(notifymsg.PlainText(event, false)), "\n", "  \n")
	if event.URL != "" {
		// Parentheses, backslashes and whitespace must not terminate the Markdown link target.
		var target strings.Builder
		for _, r := range event.URL {
			if strings.ContainsRune("()\\<>\"", r) || unicode.IsSpace(r) {
				for _, b := range []byte(string(r)) {
					fmt.Fprintf(&target, "%%%02X", b)
				}
			} else {
				target.WriteRune(r)
			}
		}
		text += "\n\n[View alert](" + target.String() + ")"
	}
	// Teams renders the title as plain text; Markdown escaping applies only to the body.
	return msTeamsMessage{
		Context: "http://schema.org/extensions", Type: "MessageCard", ThemeColor: color,
		Title: title, Text: text,
	}
}

func escapeMSTeamsMarkdown(value string) string {
	var escaped strings.Builder
	for _, r := range value {
		if strings.ContainsRune("\\`*_{}[]()#+-.!|>~<&", r) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(r)
	}
	return escaped.String()
}

func sendMSTeams(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
	endpoint, err := dst.Secrets.Resolve(ctx, dst.URL)
	if err != nil {
		return fmt.Errorf("msteams url: %w", err)
	}
	if err := field.URL(endpoint); err != nil {
		return err
	}
	// Validate the complete serialized card, including JSON escapes and UTF-8 bytes.
	payload, err := json.Marshal(renderMSTeams(dst, event))
	if err != nil {
		return errors.New("could not encode msteams notification")
	}
	if len(payload) > msTeamsPayloadLimit {
		return errors.New("msteams payload exceeds the 28 KiB limit")
	}
	response, err := httpclient.Post(
		ctx,
		client,
		"msteams",
		endpoint,
		"application/json",
		nil,
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("msteams returned HTTP %d", response.StatusCode)
	}
	return nil
}

type Config struct {
	Secrets secret.InputMode  `yaml:"-"`
	URL     string            `yaml:"url,omitempty"`
	Icons   map[string]string `yaml:"icons,omitempty"`
	Colors  map[string]string `yaml:"colors,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validateMSTeams(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("msteams HTTP client is required")
	}
	cfg.Icons = maps.Clone(cfg.Icons)
	cfg.Colors = maps.Clone(cfg.Colors)
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendMSTeams(ctx, s.config, event, s.client)
}
