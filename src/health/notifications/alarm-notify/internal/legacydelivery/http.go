// SPDX-License-Identifier: GPL-3.0-or-later

package legacydelivery

import (
	"errors"
	"net/url"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/alerta"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/discord"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/fleep"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/flock"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/gotify"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/kafka"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/matrix"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/ntfy"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/pagerduty"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/prowl"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/pushbullet"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/pushover"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/rocketchat"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/signl4"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/telegram"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

func buildAlerta(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		return alerta.New(alerta.Config{Secrets: secret.LiteralInput, APIURL: b.values["ALERTA_WEBHOOK_URL"], APIKey: b.values["ALERTA_API_KEY"], Environment: r}, b.client)
	})
}
func buildDiscord(b *builder, _ []string) ([]notifier.Sender, error) {
	return one(discord.New(discord.Config{Secrets: secret.LiteralInput, URL: b.values["DISCORD_WEBHOOK_URL"]}, b.client))
}
func buildFlock(b *builder, _ []string) ([]notifier.Sender, error) {
	return one(flock.New(flock.Config{Secrets: secret.LiteralInput, URL: b.values["FLOCK_WEBHOOK_URL"]}, b.client))
}
func buildFleep(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		if r == "." || r == ".." {
			return nil, errors.New("Fleep hook must not be a dot path segment")
		}
		return fleep.New(fleep.Config{Secrets: secret.LiteralInput, URL: "https://fleep.io/hook/" + url.PathEscape(r), Sender: b.values["FLEEP_SENDER"]}, b.client)
	})
}
func buildGotify(b *builder, _ []string) ([]notifier.Sender, error) {
	return one(gotify.New(gotify.Config{Secrets: secret.LiteralInput, APIURL: b.values["GOTIFY_APP_URL"], AppToken: b.values["GOTIFY_APP_TOKEN"]}, b.client))
}
func buildMatrix(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		return matrix.New(matrix.Config{Secrets: secret.LiteralInput, APIURL: b.values["MATRIX_HOMESERVER"], AccessToken: b.values["MATRIX_ACCESSTOKEN"], RoomID: r}, b.client)
	})
}
func buildNtfy(b *builder, recipients []string) ([]notifier.Sender, error) {
	cfg := ntfy.Config{Secrets: secret.LiteralInput}
	// Bash prefers a complete basic-auth pair; incomplete pairs are not used.
	if b.values["NTFY_USERNAME"] != "" && b.values["NTFY_PASSWORD"] != "" {
		cfg.Username, cfg.Password = b.values["NTFY_USERNAME"], b.values["NTFY_PASSWORD"]
	} else {
		cfg.AccessToken = b.values["NTFY_ACCESS_TOKEN"]
	}
	return each(recipients, func(r string) (notifier.Sender, error) {
		cfg.URL = r
		return ntfy.New(cfg, b.client)
	})
}
func buildPagerDuty(b *builder, recipients []string) ([]notifier.Sender, error) {
	version := field.Integer(1)
	if b.values["USE_PD_VERSION"] == "2" {
		version = 2
	}
	return each(recipients, func(r string) (notifier.Sender, error) {
		return pagerduty.New(pagerduty.Config{Secrets: secret.LiteralInput, IntegrationKey: r, APIVersion: &version}, b.client)
	})
}
func buildProwl(b *builder, recipients []string) ([]notifier.Sender, error) {
	return one(prowl.New(prowl.Config{Secrets: secret.LiteralInput, APIKey: strings.Join(recipients, ",")}, b.client))
}
func buildPushbullet(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		cfg := pushbullet.Config{Secrets: secret.LiteralInput, AccessToken: b.values["PUSHBULLET_ACCESS_TOKEN"], SourceDeviceID: b.values["PUSHBULLET_SOURCE_DEVICE"]}
		if strings.HasPrefix(r, "#") {
			cfg.ChannelTag = strings.TrimPrefix(r, "#")
		} else {
			cfg.Email = r
		}
		return pushbullet.New(cfg, b.client)
	})
}
func buildPushover(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		return pushover.New(pushover.Config{Secrets: secret.LiteralInput, AppToken: b.values["PUSHOVER_APP_TOKEN"], UserKey: r}, b.client)
	})
}
func buildRocketChat(b *builder, recipients []string) ([]notifier.Sender, error) {
	return each(recipients, func(r string) (notifier.Sender, error) {
		return rocketchat.New(rocketchat.Config{Secrets: secret.LiteralInput, URL: b.values["ROCKETCHAT_WEBHOOK_URL"], Channel: "#" + r}, b.client)
	})
}
func buildTelegram(b *builder, recipients []string) ([]notifier.Sender, error) {
	retries, err := integer(b.values["TELEGRAM_RETRIES_ON_LIMIT"], "TELEGRAM_RETRIES_ON_LIMIT")
	if err != nil {
		return nil, err
	}
	return each(recipients, func(r string) (notifier.Sender, error) {
		chat, topic, hasTopic := strings.Cut(r, ":")
		thread, err := integer(topic, "Telegram topic")
		if err != nil {
			return nil, err
		}
		if hasTopic && thread == nil {
			return nil, errors.New("Telegram topic must not be empty after colon")
		}
		return telegram.New(telegram.Config{Secrets: secret.LiteralInput, BotToken: b.values["TELEGRAM_BOT_TOKEN"], APIURL: b.values["TELEGRAM_API_URL"], RetriesOnLimit: retries, ChatID: chat, MessageThreadID: thread}, b.client)
	})
}
func buildKafka(b *builder, _ []string) ([]notifier.Sender, error) {
	return one(kafka.New(kafka.Config{Secrets: secret.LiteralInput, URL: b.values["KAFKA_URL"], SenderIP: b.values["KAFKA_SENDER_IP"]}, b.client))
}
func buildSIGNL4(b *builder, _ []string) ([]notifier.Sender, error) {
	return one(signl4.New(signl4.Config{Secrets: secret.LiteralInput, URL: b.values["SIGNL4_WEBHOOK_URL"]}, b.client))
}
