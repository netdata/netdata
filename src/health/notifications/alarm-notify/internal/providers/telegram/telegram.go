// SPDX-License-Identifier: GPL-3.0-or-later

package telegram

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

const telegramTextLimit = 4096 // After parsing HTML: https://core.telegram.org/bots/api#sendmessage

type telegramMessage struct {
	ChatID              string              `json:"chat_id"`
	MessageThreadID     *int64              `json:"message_thread_id,omitempty"`
	Text                string              `json:"text"`
	ParseMode           string              `json:"parse_mode"`
	DisableNotification bool                `json:"disable_notification"`
	LinkPreviewOptions  telegramLinkPreview `json:"link_preview_options"`
}

type telegramLinkPreview struct {
	IsDisabled bool `json:"is_disabled"`
}

type telegramResponse struct {
	OK         *bool `json:"ok"`
	ErrorCode  int   `json:"error_code"`
	Parameters struct {
		RetryAfter *int64 `json:"retry_after"`
	} `json:"parameters"`
}

func renderTelegram(dst Config, event notifyevent.Event) (telegramMessage, error) {
	emoji := "⚠️"
	switch event.Status {
	case "CRITICAL":
		emoji = "🔴"
	case "CLEAR":
		emoji = "✅"
	}
	title := event.Status + ": " + event.Summary
	plain := []string{emoji + " " + title}
	formatted := []string{emoji + " <b>" + html.EscapeString(title) + "</b>"}
	fields := append(notifymsg.Fields(event), notifymsg.Field{Name: "Time", Value: event.Timestamp.Format(time.RFC3339)})
	for _, field := range fields {
		plain = append(plain, field.Name+": "+field.Value)
		formatted = append(formatted, "<b>"+field.Name+":</b> "+html.EscapeString(field.Value))
	}
	if event.Info != "" {
		plain = append(plain, event.Info)
		formatted = append(formatted, "<i>"+html.EscapeString(event.Info)+"</i>")
	}
	if event.URL != "" {
		plain = append(plain, "View alert")
		formatted = append(formatted, `<a href="`+html.EscapeString(event.URL)+`">View alert</a>`)
	}
	// Count visible text, including separators; generated tags and escaped entities do not add characters.
	if utf8.RuneCountInString(strings.Join(plain, "\n")) > telegramTextLimit {
		return telegramMessage{}, errors.New("telegram text exceeds the 4096-character message limit")
	}
	return telegramMessage{
		ChatID: dst.ChatID, MessageThreadID: (*int64)(dst.MessageThreadID),
		Text: strings.Join(formatted, "\n"), ParseMode: "HTML",
		DisableNotification: event.Status == "CLEAR", LinkPreviewOptions: telegramLinkPreview{IsDisabled: true},
	}, nil
}

func sendTelegram(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
	message, err := renderTelegram(dst, event)
	if err != nil {
		return err
	}
	token, err := dst.Secrets.Resolve(ctx, dst.BotToken)
	if err != nil {
		return fmt.Errorf("telegram bot_token: %w", err)
	}
	base, err := dst.Secrets.Resolve(ctx, dst.APIURL)
	if err != nil {
		return fmt.Errorf("telegram api_url: %w", err)
	}
	endpoint, err := telegramEndpoint(base, token)
	if err != nil {
		return err
	}
	retries := field.Integer(0)
	if dst.RetriesOnLimit != nil {
		retries = *dst.RetriesOnLimit
	}
	for {
		response, err := httpclient.PostJSON(ctx, client, "telegram", endpoint, nil, message)
		if err != nil {
			return err
		}
		result, err := readTelegramResponse(response)
		if err != nil {
			return err
		}
		if response.StatusCode == http.StatusOK && *result.OK {
			return nil
		}
		if response.StatusCode != http.StatusTooManyRequests || *result.OK || retries == 0 {
			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("telegram returned HTTP %d", response.StatusCode)
			}
			return fmt.Errorf("telegram API rejected notification (error code %d)", result.ErrorCode)
		}
		delay := int64(1)
		if result.Parameters.RetryAfter != nil {
			delay = *result.Parameters.RetryAfter
		}
		if err := waitTelegramRetry(ctx, delay); err != nil {
			return err
		}
		retries--
	}
}

func readTelegramResponse(response *http.Response) (telegramResponse, error) {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusTooManyRequests {
		return telegramResponse{}, fmt.Errorf("telegram returned HTTP %d", response.StatusCode)
	}
	var result telegramResponse
	if err := httpclient.DecodeResponse("telegram", response.Body, &result); err != nil {
		return telegramResponse{}, err
	}
	if result.OK == nil {
		return telegramResponse{}, errors.New("invalid telegram response")
	}
	if result.Parameters.RetryAfter != nil && *result.Parameters.RetryAfter < 0 {
		return telegramResponse{}, errors.New("invalid telegram retry delay")
	}
	return result, nil
}

func waitTelegramRetry(ctx context.Context, seconds int64) error {
	if err := ctx.Err(); err != nil {
		return httpclient.SafeError("telegram", err)
	}
	if seconds < 0 {
		return errors.New("invalid telegram retry delay")
	}
	// Check before converting server-controlled seconds to a nanosecond duration.
	if seconds > int64((1<<63-1)/time.Second) {
		return errors.New("telegram retry delay exceeds the remaining deadline")
	}
	delay := time.Duration(seconds) * time.Second
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
		return errors.New("telegram retry delay exceeds the remaining deadline")
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return httpclient.SafeError("telegram", ctx.Err())
	case <-timer.C:
		if err := ctx.Err(); err != nil {
			return httpclient.SafeError("telegram", err)
		}
		return nil
	}
}

type Config struct {
	Secrets         secret.InputMode `yaml:"-"`
	BotToken        string           `yaml:"bot_token,omitempty"`
	ChatID          string           `yaml:"chat_id,omitempty"`
	MessageThreadID *field.Integer   `yaml:"message_thread_id,omitempty"`
	APIURL          string           `yaml:"api_url,omitempty"`
	RetriesOnLimit  *field.Integer   `yaml:"retries_on_limit,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validateTelegram(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("telegram HTTP client is required")
	}
	if cfg.MessageThreadID != nil {
		value := *cfg.MessageThreadID
		cfg.MessageThreadID = &value
	}
	if cfg.RetriesOnLimit != nil {
		value := *cfg.RetriesOnLimit
		cfg.RetriesOnLimit = &value
	}
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendTelegram(ctx, s.config, event, s.client)
}
