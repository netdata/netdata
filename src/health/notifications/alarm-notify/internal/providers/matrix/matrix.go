// SPDX-License-Identifier: GPL-3.0-or-later

package matrix

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

func (dst Config) validateMatrix() error {
	if !validMatrixID(dst.RoomID, '!') || strings.Contains(dst.RoomID, "${") {
		return errors.New("matrix room_id must be a literal !-prefixed room ID without whitespace or controls")
	}
	for _, field := range []struct{ name, value string }{{"api_url", dst.APIURL}, {"access_token", dst.AccessToken}} {
		reference, err := dst.Secrets.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("matrix %s: %w", field.name, err)
		}
		if !reference {
			if err := validateMatrixField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validMatrixID(value string, sigil byte) bool {
	return len(value) > 1 && value[0] == sigil && strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) == -1
}

func validateMatrixField(name, value string) error {
	if name == "api_url" {
		if value == "" {
			return errors.New("matrix api_url is required")
		}
		for _, host := range []string{"matrix.org", "matrix-client.matrix.org"} {
			if err := field.APIBase(value, "matrix", host); err != nil {
				return err
			}
		}
		return nil
	}
	return field.Token(value, "matrix", "access_token")
}

type matrixMessage struct {
	MessageType   string `json:"msgtype"`
	Body          string `json:"body"`
	Format        string `json:"format"`
	FormattedBody string `json:"formatted_body"`
}

func renderMatrix(event notifyevent.Event) matrixMessage {
	icon := "⚠️"
	switch event.Status {
	case "CRITICAL":
		icon = "🔴"
	case "CLEAR":
		icon = "✅"
	}
	text := icon + " " + notifymsg.PlainText(event, false)
	formatted := strings.ReplaceAll(html.EscapeString(text), "\n", "<br>")
	if event.URL != "" {
		text += "\n" + event.URL
		formatted += `<br><a href="` + html.EscapeString(event.URL) + `">` + html.EscapeString(event.URL) + `</a>`
	}
	return matrixMessage{
		MessageType:   "m.notice",
		Body:          text,
		Format:        "org.matrix.custom.html",
		FormattedBody: formatted,
	}
}

func sendMatrix(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{"api_url", &dst.APIURL}, {"access_token", &dst.AccessToken}} {
		value, err := dst.Secrets.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("matrix %s: %w", field.name, err)
		}
		if err := validateMatrixField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	// Transaction IDs identify individual sends, not the alert lifecycle.
	endpoint := strings.TrimRight(dst.APIURL, "/") + "/_matrix/client/v3/rooms/" + url.PathEscape(dst.RoomID) +
		"/send/m.room.message/nd_" + rand.Text()
	response, err := httpclient.RequestJSON(
		ctx,
		client,
		"matrix",
		http.MethodPut,
		endpoint,
		http.Header{
			"Authorization": {"Bearer " + dst.AccessToken},
			"Accept":        {"application/json"},
		},
		renderMatrix(event),
	)
	if err != nil {
		return err
	}
	return readMatrixResponse(response)
}

func readMatrixResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("matrix returned HTTP %d", response.StatusCode)
	}
	var result struct {
		EventID   string `json:"event_id"`
		ErrorCode string `json:"errcode"`
		Error     string `json:"error"`
	}
	if err := httpclient.DecodeResponse("matrix", response.Body, &result); err != nil {
		return err
	}
	if !validMatrixID(result.EventID, '$') || result.ErrorCode != "" || result.Error != "" {
		return errors.New("invalid matrix acknowledgment")
	}
	return nil
}

type Config struct {
	Secrets     secret.InputMode `yaml:"-"`
	APIURL      string           `yaml:"api_url,omitempty"`
	AccessToken string           `yaml:"access_token,omitempty"`
	RoomID      string           `yaml:"room_id,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validateMatrix(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("matrix HTTP client is required")
	}
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendMatrix(ctx, s.config, event, s.client)
}
