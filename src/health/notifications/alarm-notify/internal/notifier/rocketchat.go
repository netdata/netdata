// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
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

func renderRocketChat(dst Destination, event notifyevent.Event) rocketChatMessage {
	attachment := rocketChatAttachment{
		Color: chatWebhookColor(event.Status), Title: event.Alert, TitleLink: event.URL,
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
