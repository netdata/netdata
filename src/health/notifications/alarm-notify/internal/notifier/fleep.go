// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

type fleepMessage struct {
	Message string `json:"message"`
	User    string `json:"user,omitempty"`
}

func renderFleep(dst Destination, event Event) fleepMessage {
	return fleepMessage{Message: notificationPlainText(event, true), User: dst.Sender}
}
