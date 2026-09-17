// SPDX-License-Identifier: GPL-3.0-or-later

package prowl

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const prowlTestKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const prowlTestKeys = prowlTestKey + ",BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"

const prowlTestResponse = `<?xml version="1.0" encoding="UTF-8"?><prowl><success code="200" remaining="999" resetdate="1234567890"/></prowl>`

func formTestConfig(provider string) Config {

	return Config{APIKey: prowlTestKeys}

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
	for provider := range map[string]struct{}{"prowl": {}} {
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

					if name == "critical" {
						want.Set("priority", "2")
					} else {
						want.Set("priority", "0")
					}

				}
				dst := formTestConfig(provider)
				var got url.Values
				var err error

				got, err = renderProwl(dst, test.event)

				require.NoError(t, err)
				decoded, err := url.ParseQuery(got.Encode())
				require.NoError(t, err)
				assert.Equal(t, want, decoded)
			})
		}
	}
}

func TestFormTextEncoding(t *testing.T) {
	for provider := range map[string]struct{}{"prowl": {}} {
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

			want := url.Values{
				"apikey":      {prowlTestKeys},
				"application": {"Netdata"},
				"priority":    {"1"},
				"event":       {"node WARNING: " + event.Summary},
				"description": {text},
				"url":         {""},
			}
			got, err := renderProwl(dst, event)
			require.NoError(t, err)

			decoded, err := url.ParseQuery(got.Encode())
			require.NoError(t, err)
			assert.Equal(t, want, decoded)
		})
	}
}

func TestProwlByteLimits(t *testing.T) {
	for field, limit := range map[string]int{"event": 1024, "description": 10000, "url": 512} {
		for name, extra := range map[string]int{"boundary": 0, "too long": 1} {
			t.Run(field+"/"+name, func(t *testing.T) {
				event := notifyevent.Event{
					Node:      "node",
					Alert:     "alert",
					Status:    "WARNING",
					Summary:   "summary",
					Timestamp: testutil.ExpectedEvent().Timestamp,
				}
				switch field {
				case "event":
					event.Node = strings.Repeat("界", (limit-len(" WARNING: summary"))/3)
					event.Node += strings.Repeat("x", limit-len(" WARNING: summary")-len(event.Node)+extra)
				case "description":
					event.Info = "界"
					form, err := renderProwl(formTestConfig("prowl"), event)
					require.NoError(t, err)
					event.Info += strings.Repeat("x", limit-len(form.Get(field))+extra)
				case "url":
					event.URL = "https://example.com/"
					event.URL += strings.Repeat("x", limit-len(event.URL)+extra)
				}
				form, err := renderProwl(formTestConfig("prowl"), event)
				if extra > 0 {
					require.ErrorContains(t, err, field)
					assert.Nil(t, form)
				} else {
					require.NoError(t, err)
					assert.Len(t, form.Get(field), limit)
				}
			})
		}
	}
}

func TestReadFormResponses(t *testing.T) {
	for provider, success := range map[string]string{"prowl": prowlTestResponse} {
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
			"wrong root": `<other><success code="200"/></other>`, "missing code": `<prowl><success/></prowl>`,
			"non numeric code": `<prowl><success code="synthetic-private-value"/></prowl>`, "error": `<prowl><error code="401">synthetic-private-value</error></prowl>`,
			"success and error": `<prowl><success code="200"/><error code="401"/></prowl>`, "duplicate success": `<prowl><success code="200"/><success code="200"/></prowl>`,
			"wrong code": `<prowl><success code="401"/></prowl>`, "missing success": `<prowl/>`, "prefix data": "synthetic-private-value" + success,
			"entity": `<!DOCTYPE prowl [<!ENTITY secret SYSTEM "file:///synthetic-private-value">]><prowl><success code="200"/>&secret;</prowl>`,
		} {
			cases[name] = struct {
				status    int
				body, err string
			}{200, body, "invalid"}
		}
		cases["comments"] = struct {
			status    int
			body, err string
		}{200, "<!-- comment -->" + success + "<!-- comment -->", ""}

		for name, test := range cases {
			t.Run(provider+"/"+name, func(t *testing.T) {
				reader := strings.NewReader(test.body)
				body := &testBody{Reader: reader}
				response := &http.Response{StatusCode: test.status, Body: body}
				var err error

				err = readProwlResponse(response)

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
	for provider := range map[string]struct{}{"prowl": {}} {
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

				err = readProwlResponse(response)

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
