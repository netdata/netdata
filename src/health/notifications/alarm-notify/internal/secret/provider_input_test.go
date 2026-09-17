// SPDX-License-Identifier: GPL-3.0-or-later

package secret_test

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/alerta"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/discord"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/fleep"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/flock"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/gotify"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/kafka"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/kavenegar"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/matrix"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/messagebird"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/ntfy"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/pagerduty"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/prowl"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/pushbullet"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/pushover"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/rocketchat"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/signl4"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/smseagle"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/telegram"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/twilio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderInputModeCannotBeSetThroughYAML(t *testing.T) {
	for name, test := range map[string]struct{ factory config.FactoryFunc }{
		"alerta":      {factory: config.Factory(func(alerta.Config) (notifier.Sender, error) { return nil, nil })},
		"discord":     {factory: config.Factory(func(discord.Config) (notifier.Sender, error) { return nil, nil })},
		"fleep":       {factory: config.Factory(func(fleep.Config) (notifier.Sender, error) { return nil, nil })},
		"flock":       {factory: config.Factory(func(flock.Config) (notifier.Sender, error) { return nil, nil })},
		"gotify":      {factory: config.Factory(func(gotify.Config) (notifier.Sender, error) { return nil, nil })},
		"kavenegar":   {factory: config.Factory(func(kavenegar.Config) (notifier.Sender, error) { return nil, nil })},
		"matrix":      {factory: config.Factory(func(matrix.Config) (notifier.Sender, error) { return nil, nil })},
		"messagebird": {factory: config.Factory(func(messagebird.Config) (notifier.Sender, error) { return nil, nil })},
		"ntfy":        {factory: config.Factory(func(ntfy.Config) (notifier.Sender, error) { return nil, nil })},
		"pagerduty":   {factory: config.Factory(func(pagerduty.Config) (notifier.Sender, error) { return nil, nil })},
		"prowl":       {factory: config.Factory(func(prowl.Config) (notifier.Sender, error) { return nil, nil })},
		"pushbullet":  {factory: config.Factory(func(pushbullet.Config) (notifier.Sender, error) { return nil, nil })},
		"pushover":    {factory: config.Factory(func(pushover.Config) (notifier.Sender, error) { return nil, nil })},
		"rocketchat":  {factory: config.Factory(func(rocketchat.Config) (notifier.Sender, error) { return nil, nil })},
		"telegram":    {factory: config.Factory(func(telegram.Config) (notifier.Sender, error) { return nil, nil })},
		"twilio":      {factory: config.Factory(func(twilio.Config) (notifier.Sender, error) { return nil, nil })},
		"smseagle":    {factory: config.Factory(func(smseagle.Config) (notifier.Sender, error) { return nil, nil })},
		"kafka":       {factory: config.Factory(func(kafka.Config) (notifier.Sender, error) { return nil, nil })},
		"signl4":      {factory: config.Factory(func(signl4.Config) (notifier.Sender, error) { return nil, nil })},
	} {
		t.Run(name, func(t *testing.T) {
			for key, value := range map[string]string{"secrets": "1", "Secrets": "1", "input_mode": "1"} {
				t.Run(key, func(t *testing.T) {
					data := "version: 1\ndestinations:\n  dev:\n    type: " + name + "\n    " + key + ": " + value + "\n"
					got, err := config.Read(strings.NewReader(data), config.Registry{name: test.factory})
					require.ErrorContains(t, err, "invalid YAML")
					assert.Equal(t, notifier.Plan{}, got)
				})
			}
		})
	}
}
