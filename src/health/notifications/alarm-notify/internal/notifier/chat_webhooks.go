// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

func (dst Destination) validateChatWebhook() error {
	allowed := Destination{Type: dst.Type, URL: dst.URL}
	switch dst.Type {
	case "rocketchat":
		allowed.Channel = dst.Channel
		if dst.Channel != "" && (len(dst.Channel) < 2 || !strings.ContainsAny(dst.Channel[:1], "#@") ||
			strings.ContainsAny(dst.Channel[1:], ",#@") || strings.Contains(dst.Channel, "${") ||
			strings.IndexFunc(dst.Channel, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0) {
			return errors.New("rocketchat channel must select one #channel or @user without whitespace")
		}
	case "fleep":
		allowed.Sender = dst.Sender
		if dst.Sender != "" && (strings.TrimSpace(dst.Sender) == "" || strings.Contains(dst.Sender, "${") ||
			strings.IndexFunc(dst.Sender, unicode.IsControl) >= 0) {
			return errors.New("fleep sender must be a nonempty literal name without controls")
		}
	}
	if !reflect.DeepEqual(dst, allowed) {
		return fmt.Errorf("%s destination contains fields for another provider", dst.Type)
	}
	reference, err := secretReference(dst.URL)
	if err != nil {
		return fmt.Errorf("%s url: %w", dst.Type, err)
	}
	if !reference {
		return validateURL(dst.URL)
	}
	return nil
}

func chatWebhookColor(status string) string {
	switch status {
	case "WARNING":
		return "#f0ad4e"
	case "CRITICAL":
		return "#d9534f"
	default:
		return "#5cb85c"
	}
}
