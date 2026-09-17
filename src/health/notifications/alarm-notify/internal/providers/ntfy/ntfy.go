// SPDX-License-Identifier: GPL-3.0-or-later

package ntfy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"unicode"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

type ntfyAction struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	URL    string `json:"url"`
	Clear  bool   `json:"clear"`
}

func (dst Config) validateNtfy() error {
	if (dst.Username == "") != (dst.Password == "") {
		return errors.New("ntfy username and password must be configured together")
	}
	if dst.AccessToken != "" && dst.Username != "" {
		return errors.New("ntfy requires either access_token or username/password, not both")
	}
	for _, field := range []struct{ name, value string }{
		{"url", dst.URL}, {"access_token", dst.AccessToken}, {"username", dst.Username}, {"password", dst.Password},
	} {
		reference, err := secret.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("ntfy %s: %w", field.name, err)
		}
		if !reference {
			if err := validateNtfyField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateNtfyField(name, value string) error {
	if name == "url" {
		if !httpclient.ValidURL(value, false) || strings.Contains(value, "#") {
			return errors.New("ntfy url must be an absolute HTTP(S) topic URL without user information or fragment")
		}
		return nil
	}
	if value == "" {
		return nil // Authentication is optional; references cannot resolve to an empty value.
	}
	if name == "access_token" {
		return field.Token(value, "ntfy", name)
	}
	if strings.Contains(value, "${") || strings.IndexFunc(value, unicode.IsControl) != -1 ||
		(name == "username" && strings.Contains(value, ":")) {
		return fmt.Errorf(
			"ntfy %s must not contain controls or secret interpolation; username must not contain a colon",
			name,
		)
	}
	return nil
}

func sendNtfy(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
	for _, field := range []struct {
		name  string
		value *string
	}{
		{"url", &dst.URL}, {"access_token", &dst.AccessToken}, {"username", &dst.Username}, {"password", &dst.Password},
	} {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("ntfy %s: %w", field.name, err)
		}
		if err := validateNtfyField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	headers := renderNtfyHeaders(event)
	if dst.AccessToken != "" {
		headers.Set("Authorization", "Bearer "+dst.AccessToken)
	} else if dst.Username != "" {
		headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(dst.Username+":"+dst.Password)))
	}
	response, err := httpclient.Post(ctx, client, "ntfy", dst.URL, "text/plain; charset=utf-8", headers,
		strings.NewReader(notifymsg.PlainText(event, false)))
	if err != nil {
		return err
	}
	return readNtfyResponse(response)
}

func renderNtfyHeaders(event notifyevent.Event) http.Header {
	priority, tag := "default", "white_check_mark"
	switch event.Status {
	case "WARNING":
		priority, tag = "high", "warning"
	case "CRITICAL":
		priority, tag = "urgent", "red_circle"
	}
	headers := http.Header{
		"Title":    {encodeNtfyHeader(event.Node + ": " + strings.ReplaceAll(event.Alert, "_", " "))},
		"Priority": {priority}, "Tags": {tag},
	}
	if event.URL != "" {
		// JSON preserves delimiters inside URLs; ntfy also accepts it in the Actions header.
		actions, _ := json.Marshal([]ntfyAction{{Action: "view", Label: "View node", URL: event.URL, Clear: true}})
		headers.Set("Actions", encodeNtfyHeader(string(actions)))
	}
	return headers
}

func encodeNtfyHeader(value string) string {
	encoded := mime.BEncoding.Encode("UTF-8", value)
	if encoded != value || !strings.Contains(value, "=?") {
		return encoded
	}
	// The MIME encoder leaves printable ASCII unchanged, but ntfy decodes literal encoded words too.
	// This remaining ASCII case needs explicit encoding, with each word below the MIME 75-byte limit.
	var words []string
	for len(value) > 0 {
		n := min(len(value), 45)
		words = append(words, "=?UTF-8?B?"+base64.StdEncoding.EncodeToString([]byte(value[:n]))+"?=")
		value = value[n:]
	}
	return strings.Join(words, " ")
}

func readNtfyResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("ntfy returned HTTP %d", response.StatusCode)
	}
	var result struct {
		ID    string `json:"id"`
		Event string `json:"event"`
	}
	if err := httpclient.DecodeResponse("ntfy", response.Body, &result); err != nil {
		return err
	}
	if strings.TrimSpace(result.ID) == "" || result.Event != "message" {
		return errors.New("invalid ntfy response")
	}
	return nil
}

type Config struct {
	URL         string `yaml:"url,omitempty"`
	AccessToken string `yaml:"access_token,omitempty"`
	Username    string `yaml:"username,omitempty"`
	Password    string `yaml:"password,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validateNtfy(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("ntfy HTTP client is required")
	}
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendNtfy(ctx, s.config, event, s.client)
}
