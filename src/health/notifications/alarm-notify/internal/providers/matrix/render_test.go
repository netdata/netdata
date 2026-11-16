// SPDX-License-Identifier: GPL-3.0-or-later

package matrix

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTeamsMatrix(t *testing.T) {
	for provider := range map[string]struct{}{"matrix": {}} {
		for status := range map[string]struct{}{"WARNING": {}, "CRITICAL": {}, "CLEAR": {}} {
			for variant := range map[string]struct{}{"full": {}, "minimal": {}} {
				t.Run(provider+"/"+status+"/"+variant, func(t *testing.T) {
					event := testutil.EventForStatus(status, variant)
					message := renderMatrix(event)
					data, err := json.Marshal(message)
					require.NoError(t, err)
					assert.JSONEq(t, testutil.ReadStatusFixture(t, "testdata", provider, status, variant), string(data))
				})
			}
		}
	}
}

func TestTeamsMatrixEscaping(t *testing.T) {
	event := testutil.EventForStatus("CLEAR", "minimal")
	event.Summary = "[x](bad) *hot* <b>&\"quote\"</b>\n# next \\ end"
	event.URL = "https://example.com/a(b)<c>?q=\"x\"&y=1"
	wantText := "✅ [x](bad) *hot* <b>&\"quote\"</b>\n# next \\ end\nNode: node\nAlert: alert\nStatus: CLEAR\nTime: 2026-09-14T12:00:00Z\nhttps://example.com/a(b)<c>?q=\"x\"&y=1"
	wantHTML := "✅ [x](bad) *hot* &lt;b&gt;&amp;&#34;quote&#34;&lt;/b&gt;<br># next \\ end<br>Node: node<br>Alert: alert<br>Status: CLEAR<br>Time: 2026-09-14T12:00:00Z<br><a href=\"https://example.com/a(b)&lt;c&gt;?q=&#34;x&#34;&amp;y=1\">https://example.com/a(b)&lt;c&gt;?q=&#34;x&#34;&amp;y=1</a>"
	assert.Equal(
		t,
		matrixMessage{
			MessageType:   "m.notice",
			Body:          wantText,
			Format:        "org.matrix.custom.html",
			FormattedBody: wantHTML,
		},
		renderMatrix(event),
	)
}
func TestReadMatrixResponse(t *testing.T) {
	const ack = `{"event_id":"$synthetic-event:example.org"}`
	for name, test := range map[string]struct {
		status    int
		body, err string
	}{
		"accepted": {200, ack, ""}, "opaque id": {200, `{"event_id":"$opaque/hash+id"}`, ""},
		"wrong success": {202, ack, "HTTP 202"}, "unauthorized": {401, "synthetic-private-value", "HTTP 401"}, "limit": {429, "", "HTTP 429"}, "redirect": {307, "", "HTTP 307"},
		"missing id": {200, `{}`, "acknowledgment"}, "empty id": {200, `{"event_id":""}`, "acknowledgment"}, "sigil only": {200, `{"event_id":"$"}`, "acknowledgment"},
		"wrong sigil": {200, `{"event_id":"!room"}`, "acknowledgment"}, "whitespace": {200, `{"event_id":"$id\n"}`, "acknowledgment"}, "wrong type": {200, `{"event_id":1}`, "invalid"},
		"error": {200, `{"event_id":"$id","error":"synthetic-private-value"}`, "acknowledgment"}, "error code": {200, `{"event_id":"$id","errcode":"M_UNKNOWN"}`, "acknowledgment"},
		"empty": {200, "", "invalid"}, "null": {200, "null", "acknowledgment"}, "array": {200, "[]", "invalid"}, "malformed": {200, "synthetic-private-value", "invalid"}, "trailing": {200, ack + ack, "invalid"},
		"boundary": {200, ack + strings.Repeat(" ", httpclient.ResponseLimit-len(ack)), ""}, "over": {200, ack + strings.Repeat(" ", httpclient.ResponseLimit-len(ack)+1), "256 KiB"},
	} {
		t.Run(name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			body := &testutil.TrackingBody{Reader: reader}
			err := readMatrixResponse(&http.Response{StatusCode: test.status, Body: body})
			if test.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
			}
			assert.True(t, body.Closed)
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
			body := &testutil.TrackingBody{Reader: testutil.ErrorReader{Err: test.err}}
			err := readMatrixResponse(&http.Response{StatusCode: 200, Body: body})
			require.ErrorContains(t, err, test.want)
			assert.NotContains(t, err.Error(), "synthetic-private-value")
			assert.True(t, body.Closed)
		})
	}
}
