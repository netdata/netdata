// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
)

type fleepMessage struct {
	Message string `json:"message"`
	User    string `json:"user,omitempty"`
}

func renderFleep(dst Destination, event notifyevent.Event) fleepMessage {
	return fleepMessage{Message: notifymsg.PlainText(event, true), User: dst.Sender}
}
