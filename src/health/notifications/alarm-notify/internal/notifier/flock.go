// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
)

type flockMessage struct {
	SendAs      flockSender       `json:"sendAs"`
	Text        string            `json:"text"`
	Attachments []flockAttachment `json:"attachments"`
}

type flockSender struct {
	Name string `json:"name"`
}

type flockAttachment struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       string `json:"color"`
	URL         string `json:"url,omitempty"`
}

func renderFlock(event notifyevent.Event) flockMessage {
	return flockMessage{
		SendAs: flockSender{Name: "netdata on " + event.Node},
		Text:   event.Node + " " + event.Status + ": " + event.Summary,
		Attachments: []flockAttachment{{
			Title: event.Alert, Description: notifymsg.PlainText(event, false),
			Color: chatWebhookColor(event.Status), URL: event.URL,
		}},
	}
}
