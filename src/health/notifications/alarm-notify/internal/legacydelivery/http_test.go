// SPDX-License-Identifier: GPL-3.0-or-later

package legacydelivery_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacyconfig"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacydelivery"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
)

type mappingTransport func(*http.Request) (*http.Response, error)

func (f mappingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Expectations describe wire-level settings independently of native provider
// constructors/renderers. Provider message-format tests own the remaining text.
type mappingRequest struct {
	method, endpoint string
	headers          map[string]string
	fields           map[string]any
}

func TestLegacyHTTPMappings(t *testing.T) {
	t.Setenv("LEGACY_MAPPING_SECRET", "must-not-be-interpolated")
	const key32 = "0123456789abcdef0123456789abcdef"
	const key40 = "0123456789abcdef0123456789abcdef01234567"
	const other40 = "abcdef0123456789abcdef0123456789abcdef01"
	const token30 = "012345678901234567890123456789"
	const user30 = "987654321098765432109876543210"
	// Construct a synthetic SID without a secret-scanner-shaped source literal.
	sid := "AC" + strings.Repeat("0", 32)
	key := fmt.Sprintf("%x", sha256.Sum256([]byte("mapping-incident")))
	basic := func(user, password string) string {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
	}
	post := func(endpoint string, headers map[string]string, fields map[string]any) mappingRequest {
		return mappingRequest{http.MethodPost, endpoint, headers, fields}
	}
	tests := map[string]struct {
		method, settings, recipients string
		roleOnly                     bool
		code                         int
		ack                          string
		requests                     []mappingRequest
	}{
		"alerta environments": {method: "alerta", settings: "ALERTA_WEBHOOK_URL='https://example.test/alerts/'\nALERTA_API_KEY='alerta-token'", recipients: "Production Testing", ack: `{"status":"ok","id":"accepted"}`, requests: []mappingRequest{
			post("https://example.test/alerts/alert", map[string]string{"Authorization": "Key alerta-token"}, map[string]any{"environment": "Production"}),
			post("https://example.test/alerts/alert", map[string]string{"Authorization": "Key alerta-token"}, map[string]any{"environment": "Testing"}),
		}},
		"discord once with literal reference": {method: "discord", settings: "DISCORD_WEBHOOK_URL='https://example.test/${env:LEGACY_MAPPING_SECRET}?wait=false'", recipients: "first second", requests: []mappingRequest{
			post("https://example.test/$%7Benv:LEGACY_MAPPING_SECRET%7D?wait=true", nil, map[string]any{"username": "netdata on mapping-node"}),
		}},
		"fleep hooks and sender": {method: "fleep", settings: "FLEEP_SENDER='monitor-bot'", recipients: "hook-one hook-two", requests: []mappingRequest{
			post("https://fleep.io/hook/hook-one", nil, map[string]any{"user": "monitor-bot"}),
			post("https://fleep.io/hook/hook-two", nil, map[string]any{"user": "monitor-bot"}),
		}},
		"flock once": {method: "flock", settings: "FLOCK_WEBHOOK_URL='https://example.test/flock?key=flock-token'", recipients: "first second", requests: []mappingRequest{
			post("https://example.test/flock?key=flock-token", nil, map[string]any{"sendAs": map[string]any{"name": "netdata on mapping-node"}}),
		}},
		"gotify once": {method: "gotify", settings: "GOTIFY_APP_URL='https://example.test/gotify/'\nGOTIFY_APP_TOKEN='gotify-token'", recipients: "first second", ack: `{"id":1}`, requests: []mappingRequest{
			post("https://example.test/gotify/message", map[string]string{"X-Gotify-Key": "gotify-token"}, map[string]any{"priority": float64(4)}),
		}},
		"kavenegar": {method: "kavenegar", settings: "KAVENEGAR_API_KEY='kavenegar-token'\nKAVENEGAR_SENDER='100000'", recipients: "+15005550009", ack: `{"return":{"status":200},"entries":[{"messageid":1}]}`, requests: []mappingRequest{
			post("https://api.kavenegar.com/v1/kavenegar-token/sms/send.json", nil, map[string]any{"sender": "100000", "receptor": "+15005550009"}),
		}},
		"matrix room": {method: "matrix", settings: "MATRIX_HOMESERVER='https://example.test/matrix/'\nMATRIX_ACCESSTOKEN='matrix-token'", recipients: "!room:example.test", ack: `{"event_id":"$accepted"}`, requests: []mappingRequest{
			{http.MethodPut, "https://example.test/matrix/_matrix/client/v3/rooms/%21room:example.test/send/m.room.message/nd_<transaction>", map[string]string{"Authorization": "Bearer matrix-token"}, map[string]any{"msgtype": "m.notice"}},
		}},
		"messagebird": {method: "messagebird", settings: "MESSAGEBIRD_ACCESS_KEY='messagebird-token'\nMESSAGEBIRD_NUMBER='Monitor'", recipients: "+15005550009", code: 201, ack: `{"id":"accepted"}`, requests: []mappingRequest{
			post("https://rest.messagebird.com/messages", map[string]string{"Authorization": "AccessKey messagebird-token"}, map[string]any{"originator": "Monitor", "recipients": "+15005550009"}),
		}},
		"ntfy role only basic wins": {method: "ntfy", settings: "NTFY_USERNAME='monitor'\nNTFY_PASSWORD='ntfy-password'\nNTFY_ACCESS_TOKEN='unused-token'", recipients: "https://example.test/topic", roleOnly: true, ack: `{"id":"accepted","event":"message"}`, requests: []mappingRequest{
			post("https://example.test/topic", map[string]string{"Authorization": basic("monitor", "ntfy-password")}, nil),
		}},
		"ntfy role overrides default and incomplete basic uses token": {method: "ntfy", settings: "DEFAULT_RECIPIENT_NTFY='https://example.test/wrong'\nNTFY_USERNAME='monitor'\nNTFY_ACCESS_TOKEN='ntfy-token'", recipients: "https://example.test/right", roleOnly: true, ack: `{"id":"accepted","event":"message"}`, requests: []mappingRequest{
			post("https://example.test/right", map[string]string{"Authorization": "Bearer ntfy-token"}, nil),
		}},
		"pagerduty role only defaults v1": {method: "pd", recipients: key32, roleOnly: true, ack: `{"status":"success","incident_key":"` + key + `"}`, requests: []mappingRequest{
			post("https://events.pagerduty.com/generic/2010-04-15/create_event.json", nil, map[string]any{"service_key": key32, "event_type": "trigger", "incident_key": key, "routing_key": nil}),
		}},
		"pagerduty v2": {method: "pd", settings: "USE_PD_VERSION='2'", recipients: key32, code: 202, ack: `{"status":"success","dedup_key":"` + key + `"}`, requests: []mappingRequest{
			post("https://events.pagerduty.com/v2/enqueue", nil, map[string]any{"routing_key": key32, "event_action": "trigger", "dedup_key": key, "service_key": nil}),
		}},
		"prowl role only batch": {method: "prowl", recipients: key40 + " " + other40, roleOnly: true, ack: `<prowl><success code="200"/></prowl>`, requests: []mappingRequest{
			post("https://api.prowlapp.com/publicapi/add", nil, map[string]any{"apikey": key40 + "," + other40, "application": "Netdata", "priority": "1"}),
		}},
		"pushbullet channel and email": {method: "pushbullet", settings: "PUSHBULLET_ACCESS_TOKEN='pushbullet-token'\nPUSHBULLET_SOURCE_DEVICE='source-device'", recipients: "#operations user@example.test", ack: `{"iden":"accepted"}`, requests: []mappingRequest{
			post("https://api.pushbullet.com/v2/pushes", map[string]string{"Access-Token": "pushbullet-token"}, map[string]any{"channel_tag": "operations", "email": nil, "source_device_iden": "source-device"}),
			post("https://api.pushbullet.com/v2/pushes", map[string]string{"Access-Token": "pushbullet-token"}, map[string]any{"email": "user@example.test", "channel_tag": nil, "source_device_iden": "source-device"}),
		}},
		"pushover": {method: "pushover", settings: "PUSHOVER_APP_TOKEN='" + token30 + "'", recipients: user30, ack: `{"status":1}`, requests: []mappingRequest{
			post("https://api.pushover.net/1/messages.json", nil, map[string]any{"token": token30, "user": user30}),
		}},
		"rocketchat channels": {method: "rocketchat", settings: "ROCKETCHAT_WEBHOOK_URL='https://example.test/rocket'", recipients: "operations alerts", ack: `{"success":true}`, requests: []mappingRequest{
			post("https://example.test/rocket", nil, map[string]any{"channel": "#operations"}),
			post("https://example.test/rocket", nil, map[string]any{"channel": "#alerts"}),
		}},
		"telegram topic and custom base": {method: "telegram", settings: "TELEGRAM_API_URL='https://example.test/telegram/'\nTELEGRAM_BOT_TOKEN='123:telegram-token'\nTELEGRAM_RETRIES_ON_LIMIT='2'", recipients: "-100123:42 -100456", ack: `{"ok":true}`, requests: []mappingRequest{
			post("https://example.test/telegram/bot123:telegram-token/sendMessage", nil, map[string]any{"chat_id": "-100123", "message_thread_id": float64(42)}),
			post("https://example.test/telegram/bot123:telegram-token/sendMessage", nil, map[string]any{"chat_id": "-100456", "message_thread_id": nil}),
		}},
		"twilio": {method: "twilio", settings: "TWILIO_ACCOUNT_SID='" + sid + "'\nTWILIO_ACCOUNT_TOKEN='twilio-token'\nTWILIO_NUMBER='+15005550006'", recipients: "+15005550009", code: 201, ack: `{"sid":"accepted"}`, requests: []mappingRequest{
			post("https://api.twilio.com/2010-04-01/Accounts/"+sid+"/Messages.json", map[string]string{"Authorization": basic(sid, "twilio-token")}, map[string]any{"From": "+15005550006", "To": "+15005550009"}),
		}},
		"kafka global": {method: "kafka", settings: "KAFKA_URL='https://example.test/kafka?key=kafka-token'\nKAFKA_SENDER_IP='192.0.2.45'", code: 204, requests: []mappingRequest{
			post("https://example.test/kafka?key=kafka-token", nil, map[string]any{"host_ip": "192.0.2.45", "name": "mapping_alert"}),
		}},
		"signl4 global": {method: "signl4", settings: "SIGNL4_WEBHOOK_URL='https://example.test/signl4'\nSEND_SIGNL4='YES'", requests: []mappingRequest{
			post("https://example.test/signl4", nil, map[string]any{"X-S4-ExternalID": "mapping-incident", "X-S4-Status": "new", "X-S4-SourceSystem": "Netdata"}),
		}},
	}
	for name, mode := range map[string]struct {
		path            string
		duration, voice any
		text            bool
	}{
		"sms": {"messages/sms", nil, nil, true}, "mms": {"messages/mms", nil, nil, true},
		"ring": {"calls/ring", float64(27), nil, false}, "tts": {"calls/tts", float64(27), nil, true},
		"tts_advanced": {"calls/tts_advanced", float64(27), float64(9), true},
	} {
		duration, voice := "27", "9"
		if mode.duration == nil {
			duration = "ignored-for-this-mode"
		}
		if mode.voice == nil {
			voice = "ignored-for-this-mode"
		}
		fields := map[string]any{"to": []any{"+15005550009", "+15005550010"}, "duration": mode.duration, "voice_id": mode.voice}
		if !mode.text {
			fields["text"] = nil
		}
		tests["smseagle "+name] = struct {
			method, settings, recipients string
			roleOnly                     bool
			code                         int
			ack                          string
			requests                     []mappingRequest
		}{method: "smseagle", settings: "SMSEAGLE_API_URL='https://example.test/eagle/'\nSMSEAGLE_API_ACCESSTOKEN='eagle-token'\nSMSEAGLE_MSG_TYPE='" + name + "'\nSMSEAGLE_CALL_DURATION='" + duration + "'\nSMSEAGLE_VOICE_ID='" + voice + "'", recipients: "+15005550009 +15005550010", ack: `[{"status":"queued","id":1},{"status":"queued","id":2}]`, requests: []mappingRequest{
			post("https://example.test/eagle/api/v2/"+mode.path, map[string]string{"Access-Token": "eagle-token"}, fields),
		}}
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			settings := test.settings + "\n"
			if test.roleOnly {
				settings += "role_recipients_" + test.method + "[operations]='" + test.recipients + "'\n"
			} else if test.recipients != "" {
				settings += "DEFAULT_RECIPIENT_" + strings.ToUpper(test.method) + "='" + test.recipients + "'\n"
			}
			count := 0
			client := &http.Client{Transport: mappingTransport(func(r *http.Request) (*http.Response, error) {
				if count >= len(test.requests) {
					t.Fatal("unexpected extra request")
				}
				want := test.requests[count]
				count++
				endpoint := r.URL.String()
				if test.method == "matrix" {
					prefix, suffix, ok := strings.Cut(endpoint, "/nd_")
					if !ok || suffix == "" || strings.ContainsAny(suffix, "/?#") {
						t.Fatal("missing matrix transaction identifier")
					}
					endpoint = prefix + "/nd_<transaction>"
				}
				if r.Method != want.method {
					t.Error("request method differs")
				}
				if endpoint != want.endpoint {
					t.Error("request endpoint differs")
				}
				for _, h := range []string{"Authorization", "Access-Token", "X-Gotify-Key"} {
					if r.Header.Get(h) != want.headers[h] {
						t.Errorf("request %s header differs", h)
					}
				}
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal("cannot read request body")
				}
				if len(data) == 0 {
					t.Error("empty notification body")
				}
				fields := map[string]any{}
				switch strings.Split(r.Header.Get("Content-Type"), ";")[0] {
				case "application/json":
					if json.Unmarshal(data, &fields) != nil {
						t.Fatal("invalid request JSON")
					}
				case "application/x-www-form-urlencoded":
					form, err := url.ParseQuery(string(data))
					if err != nil {
						t.Fatal("invalid form body")
					}
					for k, v := range form {
						if len(v) != 1 {
							t.Fatal("unexpected repeated form field")
						}
						fields[k] = v[0]
					}
				default:
					if len(want.fields) > 0 {
						t.Fatal("unexpected content type")
					}
				}
				for field, wantValue := range want.fields {
					if !reflect.DeepEqual(fields[field], wantValue) {
						t.Errorf("request field %s differs", field)
					}
				}
				code := test.code
				if code == 0 {
					code = http.StatusOK
				}
				return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(test.ack)), Header: make(http.Header)}, nil
			})}
			n := mappingNotification()
			program, err := legacyconfig.Parse(strings.NewReader(settings))
			if err != nil {
				t.Fatal("legacy settings did not parse")
			}
			plan, names, err := legacydelivery.Prepare(context.Background(), []*legacyconfig.Program{program}, []string{test.method}, []string{"operations"}, n, client, nil)
			if err != nil {
				t.Fatal("legacy mapping preparation failed")
			}
			if len(names) != len(test.requests) {
				t.Fatalf("destination count = %d, want %d", len(names), len(test.requests))
			}
			reports := 0
			err = plan.Deliver(context.Background(), names, n, func(result notifier.Result) {
				reports++
				if result.Err != nil || result.SkipReason != "" {
					t.Error("destination did not acknowledge delivery")
				}
			})
			if err != nil {
				t.Error("plan delivery failed")
			}
			if count != len(test.requests) || reports != len(names) {
				t.Error("delivery/request count differs")
			}
		})
	}
}

func mappingNotification() notifier.Notification {
	return notifier.Notification{Event: event.Event{Version: 1, IncidentID: "mapping-incident", Timestamp: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC), Node: "mapping-node", Alert: "mapping_alert", Chart: "system.cpu", Context: "system.cpu", Status: "WARNING", PreviousStatus: "CLEAR", Summary: "CPU alert", URL: "https://example.test/alert"}}
}

func TestLegacyTelegramRetryMapping(t *testing.T) {
	for name, test := range map[string]struct {
		retries  string
		attempts int
	}{"disabled": {"0", 1}, "two retries": {"2", 3}} {
		t.Run(name, func(t *testing.T) {
			settings := "TELEGRAM_BOT_TOKEN='123:telegram-token'\nDEFAULT_RECIPIENT_TELEGRAM='-100123:7'\nTELEGRAM_RETRIES_ON_LIMIT='" + test.retries + "'\n"
			program, err := legacyconfig.Parse(strings.NewReader(settings))
			if err != nil {
				t.Fatal("settings did not parse")
			}
			attempts := 0
			client := &http.Client{Transport: mappingTransport(func(r *http.Request) (*http.Response, error) {
				attempts++
				if r.Method != http.MethodPost || r.URL.String() != "https://api.telegram.org/bot123:telegram-token/sendMessage" {
					t.Error("Telegram default endpoint differs")
				}
				return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"ok":false,"error_code":429,"parameters":{"retry_after":0}}`))}, nil
			})}
			n := mappingNotification()
			plan, names, err := legacydelivery.Prepare(context.Background(), []*legacyconfig.Program{program}, []string{"telegram"}, []string{"operations"}, n, client, nil)
			if err != nil {
				t.Fatal("Telegram preparation failed")
			}
			if len(names) != 1 {
				t.Fatal("expected one destination")
			}
			reports := 0
			err = plan.Deliver(context.Background(), names, n, func(result notifier.Result) {
				reports++
				if result.Err == nil {
					t.Error("rate limited delivery unexpectedly succeeded")
				} else if strings.Contains(result.Err.Error(), "telegram-token") {
					t.Error("delivery error exposed a configured credential")
				}
			})
			if err == nil || attempts != test.attempts || reports != 1 {
				t.Error("configured retry budget was not honored")
			}
		})
	}
}
