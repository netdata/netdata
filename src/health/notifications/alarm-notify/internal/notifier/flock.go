// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

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

func renderFlock(event Event) flockMessage {
	return flockMessage{
		SendAs: flockSender{Name: "netdata on " + event.Node},
		Text:   event.Node + " " + event.Status + ": " + event.Summary,
		Attachments: []flockAttachment{{
			Title: event.Alert, Description: notificationPlainText(event, false),
			Color: chatWebhookColor(event.Status), URL: event.URL,
		}},
	}
}
