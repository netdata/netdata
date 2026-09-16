// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

const telegramDefaultAPI = "https://api.telegram.org"

var (
	telegramTokenPattern = regexp.MustCompile(`^[0-9]+:[A-Za-z0-9_-]+$`)
	telegramChatPattern  = regexp.MustCompile(`^(-?[0-9]+|@[A-Za-z0-9_]+)$`)
)

func (dst Destination) validateTelegram() error {
	if dst.URL != "" || dst.BearerToken != "" {
		return errors.New("telegram destinations use bot_token and api_url; url and bearer_token are not supported")
	}
	if !telegramChatPattern.MatchString(dst.ChatID) {
		return errors.New("telegram chat_id must be a nonzero numeric ID or @username")
	}
	if !strings.HasPrefix(dst.ChatID, "@") {
		id, err := strconv.ParseInt(dst.ChatID, 10, 64)
		if err != nil || id == 0 {
			return errors.New("telegram chat_id must be a nonzero signed 64-bit ID")
		}
	}
	if dst.MessageThreadID != nil && *dst.MessageThreadID <= 0 {
		return errors.New("telegram message_thread_id must be positive")
	}
	if dst.RetriesOnLimit != nil && *dst.RetriesOnLimit < 0 {
		return errors.New("telegram retries_on_limit must not be negative")
	}
	for _, field := range []struct {
		name, value string
		validate    func(string) error
	}{
		{"bot_token", dst.BotToken, validateTelegramToken},
		{"api_url", dst.APIURL, validateTelegramAPI},
	} {
		reference, err := secret.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("telegram %s: %w", field.name, err)
		}
		if !reference {
			if err := field.validate(field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateTelegramToken(token string) error {
	if !telegramTokenPattern.MatchString(token) {
		return errors.New(
			"telegram bot_token must have the form numeric-id:token with letters, digits, underscores or hyphens",
		)
	}
	return nil
}

func validateTelegramAPI(endpoint string) error {
	return validateAPIBase(endpoint, "telegram", "api.telegram.org")
}

func telegramEndpoint(base, token string) (string, error) {
	if err := validateTelegramAPI(base); err != nil {
		return "", err
	}
	if err := validateTelegramToken(token); err != nil {
		return "", err
	}
	if base == "" {
		base = telegramDefaultAPI
	}
	return strings.TrimRight(base, "/") + "/bot" + token + "/sendMessage", nil
}
