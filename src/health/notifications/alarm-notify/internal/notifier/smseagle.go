// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

func (dst Destination) validateSMSEagle() error {
	allowed := Destination{Type: dst.Type, APIURL: dst.APIURL, AccessToken: dst.AccessToken,
		Recipients: dst.Recipients, MessageType: dst.MessageType, CallDuration: dst.CallDuration, VoiceID: dst.VoiceID}
	if !reflect.DeepEqual(dst, allowed) {
		return errors.New("smseagle destination contains fields for another provider")
	}
	switch dst.MessageType {
	case "", "sms", "mms", "ring", "tts", "tts_advanced":
	default:
		return errors.New("smseagle message_type must be sms, mms, ring, tts or tts_advanced")
	}
	if len(dst.Recipients) == 0 {
		return errors.New("smseagle requires at least one recipient")
	}
	seen := make(map[string]struct{}, len(dst.Recipients))
	for _, number := range dst.Recipients {
		if err := validatePhoneNumber(number, "smseagle", "recipients"); err != nil {
			return err
		}
		if _, ok := seen[number]; ok {
			return errors.New("smseagle recipients must not contain duplicates")
		}
		seen[number] = struct{}{}
	}
	if dst.CallDuration != nil {
		if dst.MessageType != "ring" && dst.MessageType != "tts" && dst.MessageType != "tts_advanced" {
			return errors.New("smseagle call_duration requires a call message_type")
		}
		if *dst.CallDuration <= 0 {
			return errors.New("smseagle call_duration must be a positive integer of seconds")
		}
	}
	if dst.VoiceID != nil {
		if dst.MessageType != "tts_advanced" {
			return errors.New("smseagle voice_id requires message_type: tts_advanced")
		}
		if *dst.VoiceID <= 0 {
			return errors.New("smseagle voice_id must be a positive integer")
		}
	}
	for _, field := range []struct{ name, value string }{{"api_url", dst.APIURL}, {"access_token", dst.AccessToken}} {
		reference, err := secret.IsReference(field.value)
		if err != nil {
			return fmt.Errorf("smseagle %s: %w", field.name, err)
		}
		if !reference {
			if err := validateSMSEagleField(field.name, field.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSMSEagleField(name, value string) error {
	if name == "api_url" {
		if value == "" {
			return errors.New("smseagle api_url is required")
		}
		return validateAPIBase(value, "smseagle", "")
	}
	return validateToken(value, "smseagle", name)
}

type smseagleMessage struct {
	To       []string `json:"to"`
	Text     string   `json:"text,omitempty"`
	Encoding string   `json:"encoding,omitempty"`
	Duration int64    `json:"duration,omitempty"`
	VoiceID  int64    `json:"voice_id,omitempty"`
}

func renderSMSEagle(dst Destination, event notifyevent.Event) (string, smseagleMessage, error) {
	message := smseagleMessage{To: slices.Clone(dst.Recipients)}
	mode := dst.MessageType
	if mode == "" {
		mode = "sms"
	}
	if mode == "sms" || mode == "mms" {
		message.Text = notifymsg.PlainText(event, true)
		message.Encoding = smseagleEncoding(message.Text)
		return "messages/" + mode, message, nil
	}
	message.Duration = 10
	if dst.CallDuration != nil {
		message.Duration = int64(*dst.CallDuration)
	}
	if mode == "tts" || mode == "tts_advanced" {
		message.Text = notifymsg.PlainText(event, false)
		if utf8.RuneCountInString(message.Text) > 960 {
			return "", smseagleMessage{}, errors.New("smseagle TTS text exceeds the 960-character limit")
		}
	}
	if mode == "tts_advanced" {
		message.VoiceID = 1
		if dst.VoiceID != nil {
			message.VoiceID = int64(*dst.VoiceID)
		}
	}
	return "calls/" + mode, message, nil
}

func smseagleEncoding(text string) string {
	// GSM-7 default and extension alphabets (ETSI TS 123 038, 6.2.1 and 6.2.1.1).
	// Do not transliterate unsupported characters or treat a bare ESC as message text.
	const alphabet = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?" +
		"¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà\f^{}\\[~]|€"
	for _, r := range text {
		if !strings.ContainsRune(alphabet, r) {
			return "unicode"
		}
	}
	return "standard"
}

func sendSMSEagle(ctx context.Context, dst Destination, event notifyevent.Event, timeout time.Duration) error {
	for _, field := range []struct {
		name  string
		value *string
	}{{"api_url", &dst.APIURL}, {"access_token", &dst.AccessToken}} {
		value, err := secret.Resolve(ctx, *field.value)
		if err != nil {
			return fmt.Errorf("smseagle %s: %w", field.name, err)
		}
		if err := validateSMSEagleField(field.name, value); err != nil {
			return err
		}
		*field.value = value
	}
	method, message, err := renderSMSEagle(dst, event)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(dst.APIURL, "/") + "/api/v2/" + method
	client := httpclient.New(timeout)
	defer client.CloseIdleConnections()
	response, err := httpclient.PostJSON(ctx, client, "smseagle", endpoint,
		http.Header{"Access-Token": {dst.AccessToken}, "Accept": {"application/json"}}, message)
	if err != nil {
		return err
	}
	return readSMSEagleResponse(response, len(message.To))
}

func readSMSEagleResponse(response *http.Response, recipients int) error {
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("smseagle returned HTTP %d", response.StatusCode)
	}
	var results []struct {
		Status string `json:"status"`
		ID     int64  `json:"id"`
	}
	if err := httpclient.DecodeResponse("smseagle", response.Body, &results); err != nil {
		return err
	}
	if len(results) != recipients || recipients == 0 {
		return errors.New("smseagle response did not acknowledge every recipient")
	}
	queued := 0
	for _, result := range results {
		if result.Status == "queued" && result.ID > 0 {
			queued++
		}
	}
	if queued != recipients {
		return fmt.Errorf("smseagle queued %d of %d recipients", queued, recipients)
	}
	return nil
}
