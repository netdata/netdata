// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const smseagleTestResponse = `[{"status":"queued","message":"OK","number":"+15005550009","id":1},{"status":"queued","message":"OK","number":"05005550009","id":2}]`

func smseagleTestDestination() Destination {
	return Destination{
		Type:        "smseagle",
		APIURL:      "https://example.com",
		AccessToken: "synthetic-token",
		Recipients:  []string{"+15005550009", "05005550009"},
	}
}

func smseagleTestFixture(t *testing.T, variant, mode, status string) map[string]any {
	t.Helper()
	data, err := os.ReadFile("testdata/smseagle-" + variant + ".json")
	require.NoError(t, err)
	var want map[string]any
	require.NoError(t, json.Unmarshal(data, &want))
	text := want["text"].(string)
	if status == "CRITICAL" {
		text = strings.ReplaceAll(text, "WARNING", "CRITICAL")
	}
	if status == "CLEAR" {
		text = strings.ReplaceAll(text, "CLEAR → WARNING", "CRITICAL → CLEAR")
	}
	want["text"] = text
	if mode == "ring" || mode == "tts" || mode == "tts_advanced" {
		delete(want, "encoding")
		want["duration"] = float64(10)
		want["text"] = strings.TrimSuffix(text, "\nhttps://example.com/alert?id=1&view=chart#details")
		if mode == "ring" {
			delete(want, "text")
		}
		if mode == "tts_advanced" {
			want["voice_id"] = float64(1)
		}
	}
	return want
}

func TestRenderSMSEagle(t *testing.T) {
	full := expectedEvent()
	full.URL = "https://example.com/alert?id=1&view=chart#details"
	critical, clear := full, full
	critical.Status = "CRITICAL"
	clear.Status, clear.PreviousStatus = "CLEAR", "CRITICAL"
	for mode, method := range map[string]string{"": "messages/sms", "sms": "messages/sms", "mms": "messages/mms", "ring": "calls/ring", "tts": "calls/tts", "tts_advanced": "calls/tts_advanced"} {
		for name, test := range map[string]struct {
			event   notifyevent.Event
			variant string
		}{
			"warning": {full, "full"}, "critical": {critical, "full"}, "clear": {clear, "full"},
			"minimal": {notifyevent.Event{Node: "node", Alert: "alert", Status: "WARNING", Summary: "summary", Timestamp: full.Timestamp}, "minimal"},
		} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				dst := smseagleTestDestination()
				dst.MessageType = mode
				path, message, err := renderSMSEagle(dst, test.event)
				require.NoError(t, err)
				data, err := json.Marshal(message)
				require.NoError(t, err)
				var got map[string]any
				require.NoError(t, json.Unmarshal(data, &got))
				assert.Equal(t, method, path)
				assert.Equal(t, smseagleTestFixture(t, test.variant, mode, test.event.Status), got)
				message.To[0] = "changed"
				assert.Equal(t, []string{"+15005550009", "05005550009"}, dst.Recipients)
			})
		}
	}
}

func TestSMSEagleEncoding(t *testing.T) {
	for name, test := range map[string]struct{ text, encoding string }{
		"empty": {"", "standard"}, "ASCII": {"Netdata WARNING 100%\r\n", "standard"},
		"default repertoire": {"@£$¥èéùìòÇØøÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ¤¡ÄÖÑÜ§¿äöñüà", "standard"},
		"extensions":         {"\f^{}\\[~]|€", "standard"}, "Unicode Greek": {"θερμοκρασία", "unicode"},
		"CJK and emoji": {"界😀", "unicode"}, "arrow": {"CLEAR → WARNING", "unicode"},
		"backtick": {"`", "unicode"}, "tab": {"\t", "unicode"}, "escape": {"\x1b", "unicode"},
		"nonbreaking space": {"\u00a0", "unicode"}, "no transliteration": {"ç", "unicode"},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, test.encoding, smseagleEncoding(test.text)) })
	}
	for field := range map[string]struct{}{"node": {}, "url": {}, "transition": {}} {
		t.Run("whole message/"+field, func(t *testing.T) {
			event := notifyevent.Event{
				Node:      "node",
				Alert:     "alert",
				Summary:   "summary",
				Status:    "WARNING",
				Timestamp: expectedEvent().Timestamp,
			}
			switch field {
			case "node":
				event.Node = "界"
			case "url":
				event.URL = "https://example.com/界"
			case "transition":
				event.PreviousStatus = "CLEAR"
			}
			_, got, err := renderSMSEagle(smseagleTestDestination(), event)
			require.NoError(t, err)
			assert.Equal(t, "unicode", got.Encoding)
		})
	}
}

func TestSMSEagleTTSLimitsAndOptions(t *testing.T) {
	for mode := range map[string]struct{}{"sms": {}, "mms": {}, "ring": {}, "tts": {}, "tts_advanced": {}} {
		for name, test := range map[string]struct {
			char string
			size int
		}{"boundary": {"x", 960}, "over": {"x", 961}, "Unicode boundary": {"界", 960}, "Unicode over": {"界", 961}} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				const suffix = "\nNode: node\nAlert: alert\nStatus: WARNING\nTime: 2026-09-14T12:00:00Z"
				event := notifyevent.Event{
					Node:      "node",
					Alert:     "alert",
					Status:    "WARNING",
					Timestamp: expectedEvent().Timestamp,
					URL:       "https://example.com/" + strings.Repeat("x", 1000),
				}
				event.Summary = strings.Repeat(test.char, test.size-utf8.RuneCountInString(suffix))
				dst := smseagleTestDestination()
				dst.MessageType = mode
				call := mode == "ring" || mode == "tts" || mode == "tts_advanced"
				duration, voice := configInteger(25), configInteger(7)
				if call {
					dst.CallDuration = &duration
				}
				if mode == "tts_advanced" {
					dst.VoiceID = &voice
				}
				path, got, err := renderSMSEagle(dst, event)
				if (mode == "tts" || mode == "tts_advanced") && test.size > 960 {
					require.ErrorContains(t, err, "960-character")
					assert.Empty(t, path)
					assert.Equal(t, smseagleMessage{}, got)
					return
				}
				require.NoError(t, err)
				want := smseagleMessage{To: dst.Recipients, Text: event.Summary + suffix}
				if call {
					want.Duration = 25
					if mode == "ring" {
						want.Text = ""
					}
					if mode == "tts_advanced" {
						want.VoiceID = 7
					}
				} else {
					want.Text += "\n" + event.URL
					want.Encoding = "standard"
					if test.char == "界" {
						want.Encoding = "unicode"
					}
				}
				assert.Equal(t, want, got)
			})
		}
	}
}

func TestReadSMSEagleResponse(t *testing.T) {
	for name, test := range map[string]struct {
		status    int
		body, err string
	}{
		"queued": {200, smseagleTestResponse, ""}, "HTTP error": {401, "synthetic-private-value", "HTTP 401"},
		"wrong success": {201, smseagleTestResponse, "HTTP 201"}, "redirect": {307, "", "HTTP 307"},
		"empty": {200, "", "invalid"}, "malformed": {200, "synthetic-private-value", "invalid"},
		"object": {200, `{}`, "invalid"}, "null": {200, `null`, "every recipient"}, "empty array": {200, `[]`, "every recipient"},
		"missing recipient": {200, `[{"status":"queued","id":1}]`, "every recipient"},
		"extra recipient":   {200, `[{"status":"queued","id":1},{"status":"queued","id":2},{"status":"queued","id":3}]`, "every recipient"},
		"partial":           {200, `[{"status":"queued","id":1},{"status":"error","message":"synthetic-private-value"}]`, "queued 1 of 2"},
		"null entry":        {200, `[null,{"status":"queued","id":2}]`, "queued 1 of 2"},
		"missing IDs":       {200, `[{"status":"queued"},{"status":"queued","id":0}]`, "queued 0 of 2"},
		"negative ID":       {200, `[{"status":"queued","id":-1},{"status":"queued","id":2}]`, "queued 1 of 2"},
		"wrong ID type":     {200, `[{"status":"queued","id":"1"},{"status":"queued","id":2}]`, "invalid"},
		"missing status":    {200, `[{"id":1},{"id":2}]`, "queued 0 of 2"},
		"trailing data":     {200, smseagleTestResponse + "synthetic-private-value", "invalid"},
		"second document":   {200, smseagleTestResponse + smseagleTestResponse, "invalid"},
		"limit":             {200, smseagleTestResponse + strings.Repeat(" ", httpclient.ResponseLimit-len(smseagleTestResponse)), ""},
		"over limit":        {200, smseagleTestResponse + strings.Repeat(" ", httpclient.ResponseLimit-len(smseagleTestResponse)+1), "256 KiB"},
	} {
		t.Run(name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			body := &telegramTestBody{Reader: reader}
			err := readSMSEagleResponse(&http.Response{StatusCode: test.status, Body: body}, 2)
			if test.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
			}
			assert.True(t, body.closed)
			if test.status != 200 {
				assert.Equal(t, len(test.body), reader.Len())
			}
		})
	}
	for name, test := range map[string]struct {
		err  error
		want string
	}{"read": {errors.New("synthetic-private-value"), "transport failed"}, "cancel": {context.Canceled, "canceled"}, "deadline": {context.DeadlineExceeded, "timed out"}} {
		t.Run(name, func(t *testing.T) {
			body := &telegramTestBody{Reader: formErrorReader{test.err}}
			err := readSMSEagleResponse(&http.Response{StatusCode: 200, Body: body}, 2)
			require.ErrorContains(t, err, test.want)
			assert.NotContains(t, err.Error(), "synthetic-private-value")
			assert.True(t, body.closed)
		})
	}
}
