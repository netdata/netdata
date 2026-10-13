// SPDX-License-Identifier: GPL-3.0-or-later

// Package providers composes the explicitly registered built-in destinations.
package providers

import (
	"net/http"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/alerta"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/awssns"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/command"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/discord"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/dynatrace"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/email"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/fleep"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/flock"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/gotify"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/ilert"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/irc"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/kafka"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/kavenegar"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/matrix"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/messagebird"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/msteams"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/ntfy"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/opsgenie"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/pagerduty"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/prowl"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/pushbullet"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/pushover"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/rocketchat"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/signl4"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/slack"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/smseagle"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/smstools3"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/syslog"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/telegram"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/twilio"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/webhook"
)

func Builtin(client *http.Client, processes *commandexec.Runner) config.Registry {
	return config.Registry{
		"webhook":     config.Factory(func(c webhook.Config) (notifier.Sender, error) { return webhook.New(c, client) }),
		"slack":       config.Factory(func(c slack.Config) (notifier.Sender, error) { return slack.New(c, client) }),
		"discord":     config.Factory(func(c discord.Config) (notifier.Sender, error) { return discord.New(c, client) }),
		"telegram":    config.Factory(func(c telegram.Config) (notifier.Sender, error) { return telegram.New(c, client) }),
		"pushover":    config.Factory(func(c pushover.Config) (notifier.Sender, error) { return pushover.New(c, client) }),
		"pushbullet":  config.Factory(func(c pushbullet.Config) (notifier.Sender, error) { return pushbullet.New(c, client) }),
		"twilio":      config.Factory(func(c twilio.Config) (notifier.Sender, error) { return twilio.New(c, client) }),
		"messagebird": config.Factory(func(c messagebird.Config) (notifier.Sender, error) { return messagebird.New(c, client) }),
		"gotify":      config.Factory(func(c gotify.Config) (notifier.Sender, error) { return gotify.New(c, client) }),
		"ntfy":        config.Factory(func(c ntfy.Config) (notifier.Sender, error) { return ntfy.New(c, client) }),
		"rocketchat":  config.Factory(func(c rocketchat.Config) (notifier.Sender, error) { return rocketchat.New(c, client) }),
		"flock":       config.Factory(func(c flock.Config) (notifier.Sender, error) { return flock.New(c, client) }),
		"fleep":       config.Factory(func(c fleep.Config) (notifier.Sender, error) { return fleep.New(c, client) }),
		"ilert":       config.Factory(func(c ilert.Config) (notifier.Sender, error) { return ilert.New(c, client) }),
		"signl4":      config.Factory(func(c signl4.Config) (notifier.Sender, error) { return signl4.New(c, client) }),
		"alerta":      config.Factory(func(c alerta.Config) (notifier.Sender, error) { return alerta.New(c, client) }),
		"dynatrace":   config.Factory(func(c dynatrace.Config) (notifier.Sender, error) { return dynatrace.New(c, client) }),
		"prowl":       config.Factory(func(c prowl.Config) (notifier.Sender, error) { return prowl.New(c, client) }),
		"kavenegar":   config.Factory(func(c kavenegar.Config) (notifier.Sender, error) { return kavenegar.New(c, client) }),
		"smseagle":    config.Factory(func(c smseagle.Config) (notifier.Sender, error) { return smseagle.New(c, client) }),
		"pagerduty":   config.Factory(func(c pagerduty.Config) (notifier.Sender, error) { return pagerduty.New(c, client) }),
		"opsgenie":    config.Factory(func(c opsgenie.Config) (notifier.Sender, error) { return opsgenie.New(c, client) }),
		"msteams":     config.Factory(func(c msteams.Config) (notifier.Sender, error) { return msteams.New(c, client) }),
		"matrix":      config.Factory(func(c matrix.Config) (notifier.Sender, error) { return matrix.New(c, client) }),
		"kafka":       config.Factory(func(c kafka.Config) (notifier.Sender, error) { return kafka.New(c, client) }),
		"command":     config.Factory(func(c command.Config) (notifier.Sender, error) { return command.New(c, processes) }),
		"smstools3":   config.Factory(func(c smstools3.Config) (notifier.Sender, error) { return smstools3.New(c, processes) }),
		"syslog":      config.Factory(func(c syslog.Config) (notifier.Sender, error) { return syslog.New(c, processes) }),
		"awssns":      config.Factory(func(c awssns.Config) (notifier.Sender, error) { return awssns.New(c, processes) }),
		"email":       config.Factory(func(c email.Config) (notifier.Sender, error) { return email.New(c, processes) }),
		"irc":         config.Factory(func(c irc.Config) (notifier.Sender, error) { return irc.New(c, processes) }),
	}
}
