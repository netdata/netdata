// SPDX-License-Identifier: GPL-3.0-or-later

package kavenegar

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const kavenegarTestResponse = `{"return":{"status":200,"message":"تایید شد"},"entries":[{"messageid":8792343,"status":1,"receptor":"+15005550009"}]}`

func formTestConfig(provider string) Config {

	return Config{APIKey: "synthetic-key", Sender: "+15005550006", Recipient: "+15005550009"}
}

func formTestFixture(t *testing.T, provider, variant string) url.Values {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", provider+"-"+variant+".json"))
	require.NoError(t, err)
	var want url.Values
	require.NoError(t, json.Unmarshal(data, &want))
	return want
}

func TestRenderFormProviders(t *testing.T) {
	full := testutil.ExpectedEvent()
	full.URL = "https://example.com/alert?id=1&view=chart#details"
	critical, clear := full, full
	critical.Status = "CRITICAL"
	clear.Status, clear.PreviousStatus = "CLEAR", "CRITICAL"
	for provider := range map[string]struct{}{"kavenegar": {}} {
		for name, test := range map[string]struct {
			event   notifyevent.Event
			variant string
		}{
			"warning": {full, "full"}, "critical": {critical, "full"}, "clear": {clear, "full"},
			"minimal": {notifyevent.Event{Node: "node", Alert: "alert", Status: "WARNING", Summary: "summary", Timestamp: full.Timestamp}, "minimal"},
		} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				want := formTestFixture(t, provider, test.variant)
				if name == "critical" || name == "clear" {
					for field, values := range want {
						for i, value := range values {
							if name == "critical" {
								values[i] = strings.ReplaceAll(value, "WARNING", "CRITICAL")
							} else {
								values[i] = strings.NewReplacer("CLEAR → WARNING", "CRITICAL → CLEAR", "WARNING:", "CLEAR:").Replace(value)
							}
						}
						want[field] = values
					}

				}
				dst := formTestConfig(provider)
				got := renderKavenegar(dst, test.event)
				decoded, err := url.ParseQuery(got.Encode())
				require.NoError(t, err)
				assert.Equal(t, want, decoded)
			})
		}
	}
}

func TestFormTextEncoding(t *testing.T) {
	for provider := range map[string]struct{}{"kavenegar": {}} {
		t.Run(provider, func(t *testing.T) {
			event := notifyevent.Event{
				Node:      "node",
				Alert:     "alert",
				Status:    "WARNING",
				Summary:   `<b>"quoted" & + = % 'text'</b>`,
				Info:      "line\nفارسی 界😀",
				Timestamp: testutil.ExpectedEvent().Timestamp,
			}
			text := event.Summary + "\n" + event.Info + "\nNode: node\nAlert: alert\nStatus: WARNING\nTime: 2026-09-14T12:00:00Z"
			dst := formTestConfig(provider)
			want := url.Values{"sender": {dst.Sender}, "receptor": {dst.Recipient}, "message": {text}}
			got := renderKavenegar(dst, event)

			decoded, err := url.ParseQuery(got.Encode())
			require.NoError(t, err)
			assert.Equal(t, want, decoded)
		})
	}
}

func TestReadFormResponses(t *testing.T) {
	for provider, success := range map[string]string{"kavenegar": kavenegarTestResponse} {
		cases := map[string]struct {
			status    int
			body, err string
		}{
			"accepted": {200, success, ""}, "empty": {200, "", "invalid"}, "null": {200, "null", "invalid"},
			"malformed": {
				200,
				"synthetic-private-value",
				"invalid",
			}, "trailing data": {200, success + "synthetic-private-value", "invalid"},
			"trailing document": {
				200,
				success + success,
				"invalid",
			}, "HTTP error": {401, "synthetic-private-value", "HTTP 401"},
			"redirect": {302, "synthetic-private-value", "HTTP 302"}, "wrong success code": {201, success, "HTTP 201"},
			"no content": {
				204,
				"",
				"HTTP 204",
			}, "limit": {200, success + strings.Repeat(" ", httpclient.ResponseLimit-len(success)), ""},
			"over limit": {200, success + strings.Repeat(" ", httpclient.ResponseLimit-len(success)+1), "256 KiB"},
		}

		for name, body := range map[string]string{
			"API error":      `{"return":{"status":403,"message":"synthetic-private-value"},"entries":[{"messageid":1}]}`,
			"missing return": `{"entries":[{"messageid":1}]}`, "missing entries": `{"return":{"status":200}}`,
			"empty entries": `{"return":{"status":200},"entries":[]}`, "null entry": `{"return":{"status":200},"entries":[null]}`,
			"zero ID": `{"return":{"status":200},"entries":[{"messageid":0}]}`, "negative ID": `{"return":{"status":200},"entries":[{"messageid":-1}]}`,
			"wrong ID type": `{"return":{"status":200},"entries":[{"messageid":"1"}]}`,
			"extra entry":   `{"return":{"status":200},"entries":[{"messageid":1},{"messageid":2}]}`,
		} {
			cases[name] = struct {
				status    int
				body, err string
			}{200, body, "invalid"}
		}

		for name, test := range cases {
			t.Run(provider+"/"+name, func(t *testing.T) {
				reader := strings.NewReader(test.body)
				body := &testBody{Reader: reader}
				response := &http.Response{StatusCode: test.status, Body: body}
				var err error

				err = readKavenegarResponse(response)

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
	}
}

func TestReadFormResponseErrors(t *testing.T) {
	for provider := range map[string]struct{}{"kavenegar": {}} {
		for name, test := range map[string]struct {
			err  error
			want string
		}{
			"read": {errors.New("synthetic-private-value"), "transport failed"}, "canceled": {context.Canceled, "canceled"}, "deadline": {context.DeadlineExceeded, "timed out"},
		} {
			t.Run(provider+"/"+name, func(t *testing.T) {
				body := &testBody{Reader: formErrorReader{test.err}}
				response := &http.Response{StatusCode: 200, Body: body}
				var err error

				err = readKavenegarResponse(response)

				require.ErrorContains(t, err, test.want)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.True(t, body.closed)
			})
		}
	}
}

type formErrorReader struct{ err error }

func (r formErrorReader) Read([]byte) (int, error) { return 0, r.err }

var _ io.Reader = formErrorReader{}

func TestKavenegarLongText(t *testing.T) {
	text := strings.Repeat("فارسی 界😀 & +\n", 2000)
	event := notifyevent.Event{
		Node:      "node",
		Alert:     "alert",
		Status:    "WARNING",
		Summary:   "summary",
		Info:      text,
		Timestamp: testutil.ExpectedEvent().Timestamp,
	}
	want := url.Values{
		"sender":   {"+15005550006"},
		"receptor": {"+15005550009"},
		"message":  {"summary\n" + text + "\nNode: node\nAlert: alert\nStatus: WARNING\nTime: 2026-09-14T12:00:00Z"},
	}
	decoded, err := url.ParseQuery(renderKavenegar(formTestConfig("kavenegar"), event).Encode())
	require.NoError(t, err)
	assert.Equal(t, want, decoded)
}
