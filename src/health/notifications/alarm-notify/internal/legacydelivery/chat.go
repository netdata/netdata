// SPDX-License-Identifier: GPL-3.0-or-later

package legacydelivery

import (
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/msteams"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/slack"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

func buildSlack(b *builder, recipients []string) ([]notifier.Sender, error) {
	images := b.values["images_base_url"]
	if images == "" {
		images = "https://registry.my-netdata.io"
	}
	return each(recipients, func(r string) (notifier.Sender, error) {
		if !strings.HasPrefix(r, "#") && !strings.HasPrefix(r, "@") {
			r = "#" + r
		}
		if r == "#" {
			r = "" // Use the destination configured in the legacy webhook.
		}
		return slack.New(slack.Config{
			Secrets: secret.LiteralInput, URL: b.values["SLACK_WEBHOOK_URL"],
			Legacy: &slack.LegacyOverrides{
				Channel: r, Username: "netdata on " + b.values["host"],
				IconURL: images + "/images/banner-icon-144x144.png",
			},
		}, b.client)
	})
}

func buildMSTeams(b *builder, recipients []string) ([]notifier.Sender, error) {
	cfg := msteams.Config{Secrets: secret.LiteralInput, Icons: map[string]string{}, Colors: map[string]string{}}
	for _, status := range []string{"warning", "critical", "clear"} {
		if value, ok := b.values["MSTEAMS_ICON_"+strings.ToUpper(status)]; ok {
			cfg.Icons[status] = value
		}
		if value, ok := b.values["MSTEAMS_COLOR_"+strings.ToUpper(status)]; ok {
			cfg.Colors[status] = value
		}
	}
	var urls []string
	seen := make(map[string]bool)
	for _, recipient := range recipients {
		endpoint := strings.ReplaceAll(b.values["MSTEAMS_WEBHOOK_URL"], "CHANNEL", recipient)
		// Routing has already applied every recipient's policies. Compare exact URLs
		// without normalizing paths or queries that may be covered by a signature.
		if !seen[endpoint] {
			seen[endpoint] = true
			urls = append(urls, endpoint)
		}
	}
	return each(urls, func(endpoint string) (notifier.Sender, error) {
		cfg.URL = endpoint
		return msteams.New(cfg, b.client)
	})
}
