// SPDX-License-Identifier: GPL-3.0-or-later

package rocketchat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

type rocketChatMessage struct {
	Channel     string                 `json:"channel,omitempty"`
	Alias       string                 `json:"alias"`
	Text        string                 `json:"text"`
	ParseURLs   bool                   `json:"parseUrls"`
	Attachments []rocketChatAttachment `json:"attachments"`
}

type rocketChatAttachment struct {
	Color     string            `json:"color"`
	Title     string            `json:"title"`
	TitleLink string            `json:"title_link,omitempty"`
	Text      string            `json:"text,omitempty"`
	Timestamp string            `json:"ts"`
	Fields    []rocketChatField `json:"fields"`
}

type rocketChatField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

func renderRocketChat(dst Config, event notifyevent.Event) rocketChatMessage {
	attachment := rocketChatAttachment{
		Color: notifymsg.StatusColor(event.Status), Title: event.Alert, TitleLink: event.URL,
		Text: event.Info, Timestamp: event.Timestamp.Format(time.RFC3339),
	}
	for _, field := range notifymsg.Fields(event) {
		attachment.Fields = append(
			attachment.Fields,
			rocketChatField{Title: field.Name, Value: field.Value, Short: true},
		)
	}
	return rocketChatMessage{
		Channel: dst.Channel, Alias: "netdata on " + event.Node,
		Text:        event.Node + " " + event.Status + ": " + event.Summary,
		Attachments: []rocketChatAttachment{attachment},
	}
}

func readRocketChatResponse(response *http.Response) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("rocketchat returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Success   bool   `json:"success"`
		Error     string `json:"error"`
		Responses []struct {
			Error string `json:"error"`
		} `json:"responses"`
	}
	if err := httpclient.DecodeResponse("rocketchat", response.Body, &result); err != nil {
		return err
	}
	if !result.Success || result.Error != "" {
		return errors.New("rocketchat did not acknowledge the notification")
	}
	// A server integration script can return separate room results, including partial failures.
	for _, room := range result.Responses {
		if room.Error != "" {
			return errors.New("rocketchat did not acknowledge every destination room")
		}
	}
	return nil
}

func (dst Config) validateRocketChat() error {
	if dst.Channel != "" && (len(dst.Channel) < 2 || !strings.ContainsAny(dst.Channel[:1], "#@") ||
		strings.ContainsAny(dst.Channel[1:], ",#@") || strings.Contains(dst.Channel, "${") ||
		strings.IndexFunc(dst.Channel, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0) {
		return errors.New("rocketchat channel must select one #channel or @user without whitespace")
	}
	reference, err := dst.Secrets.IsReference(dst.URL)
	if err != nil {
		return fmt.Errorf("rocketchat url: %w", err)
	}
	if !reference {
		if err := field.URL(dst.URL); err != nil {
			return err
		}
	}

	return nil
}

func postJSON(ctx context.Context, client *http.Client, dst Config, message any) error {
	endpoint, err := dst.Secrets.Resolve(ctx, dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if err := field.URL(endpoint); err != nil {
		return err
	}
	headers := http.Header{}
	response, err := httpclient.PostJSON(ctx, client, "rocketchat", endpoint, headers, message)
	if err != nil {
		return err
	}
	return readRocketChatResponse(response)
}

func sendRocketChat(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
	return postJSON(ctx, client, dst, renderRocketChat(dst, event))
}

type Config struct {
	Secrets secret.InputMode `yaml:"-"`
	URL     string           `yaml:"url,omitempty"`
	Channel string           `yaml:"channel,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validateRocketChat(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("rocketchat HTTP client is required")
	}
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendRocketChat(ctx, s.config, event, s.client)
}
