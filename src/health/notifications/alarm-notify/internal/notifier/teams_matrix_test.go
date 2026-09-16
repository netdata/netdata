// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func teamsMatrixTestEvent(status, variant string) Event {
	event := expectedEvent()
	if variant == "minimal" {
		event = Event{
			Version:    1,
			IncidentID: "test-incident",
			Timestamp:  event.Timestamp,
			Node:       "node",
			Alert:      "alert",
			Summary:    "summary",
		}
	} else {
		event.URL = "https://example.com/alert?id=1&view=chart#details"
		event.PreviousStatus = map[string]string{"WARNING": "CLEAR", "CRITICAL": "WARNING", "CLEAR": "CRITICAL"}[status]
	}
	event.Status = status
	return event
}

func teamsMatrixTestFixture(t *testing.T, provider, status, variant string) string {
	t.Helper()
	data, err := os.ReadFile(fmt.Sprintf("testdata/%s-%s-%s.json", provider, strings.ToLower(status), variant))
	require.NoError(t, err)
	return string(data)
}

func TestRenderTeamsMatrix(t *testing.T) {
	for provider := range map[string]struct{}{"msteams": {}, "matrix": {}} {
		for status := range map[string]struct{}{"WARNING": {}, "CRITICAL": {}, "CLEAR": {}} {
			for variant := range map[string]struct{}{"full": {}, "minimal": {}} {
				t.Run(provider+"/"+status+"/"+variant, func(t *testing.T) {
					event := teamsMatrixTestEvent(status, variant)
					var message any = renderMatrix(event)
					if provider == "msteams" {
						message = renderMSTeams(teamsMatrixTestDestination(provider), event)
					}
					data, err := json.Marshal(message)
					require.NoError(t, err)
					assert.JSONEq(t, teamsMatrixTestFixture(t, provider, status, variant), string(data))
				})
			}
		}
	}
}

func TestTeamsMatrixEscaping(t *testing.T) {
	event := teamsMatrixTestEvent("CLEAR", "minimal")
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
	assert.Equal(t, msTeamsMessage{
		Context: "http://schema.org/extensions", Type: "MessageCard", ThemeColor: "65A677", Title: "💚 Alert CLEAR from Netdata on node",
		Text: "\\[x\\]\\(bad\\) \\*hot\\* \\<b\\>\\&\"quote\"\\</b\\>  \n\\# next \\\\ end  \nNode: node  \nAlert: alert  \nStatus: CLEAR  \nTime: 2026\\-09\\-14T12:00:00Z\n\n[View alert](https://example.com/a%28b%29%3Cc%3E?q=%22x%22&y=1)",
	}, renderMSTeams(teamsMatrixTestDestination("msteams"), event))
}

func TestMSTeamsStyle(t *testing.T) {
	for name, test := range map[string]struct {
		icons, colors  map[string]string
		prefix, color  string
		node, nodeText string
	}{
		"defaults": {prefix: "⚠️ ", color: "FFA500"}, "custom": {icons: map[string]string{"warning": "*alarm*"}, colors: map[string]string{"warning": "abcDEF"}, prefix: "*alarm* ", color: "abcDEF"},
		"empty overrides":      {icons: map[string]string{"warning": ""}, colors: map[string]string{"warning": ""}},
		"other status":         {icons: map[string]string{"clear": "x"}, colors: map[string]string{"critical": "abcdef"}, prefix: "⚠️ ", color: "FFA500"},
		"hostname punctuation": {node: "prod-db-01.example.net", nodeText: `prod\-db\-01\.example\.net`, prefix: "⚠️ ", color: "FFA500"},
		"literal Markdown":     {node: "db_*.[east]", nodeText: `db\_\*\.\[east\]`, prefix: "⚠️ ", color: "FFA500"},
		"literal backslash":    {node: `node\branch`, nodeText: `node\\branch`, prefix: "⚠️ ", color: "FFA500"},
		"Unicode hostname":     {node: "δοκιμή-01", nodeText: `δοκιμή\-01`, prefix: "⚠️ ", color: "FFA500"},
	} {
		t.Run(name, func(t *testing.T) {
			dst := teamsMatrixTestDestination("msteams")
			dst.Icons, dst.Colors = test.icons, test.colors
			event := teamsMatrixTestEvent("WARNING", "minimal")
			var want msTeamsMessage
			require.NoError(
				t,
				json.Unmarshal([]byte(teamsMatrixTestFixture(t, "msteams", "WARNING", "minimal")), &want),
			)
			if test.node != "" {
				event.Node = test.node
				want.Text = strings.Replace(want.Text, "Node: node", "Node: "+test.nodeText, 1)
			}
			want.Title, want.ThemeColor = test.prefix+"Alert WARNING from Netdata on "+event.Node, test.color
			assert.Equal(t, want, renderMSTeams(dst, event))
		})
	}
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
		"boundary": {200, ack + strings.Repeat(" ", notificationResponseLimit-len(ack)), ""}, "over": {200, ack + strings.Repeat(" ", notificationResponseLimit-len(ack)+1), "256 KiB"},
	} {
		t.Run(name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			body := &telegramTestBody{Reader: reader}
			err := readMatrixResponse(&http.Response{StatusCode: test.status, Body: body})
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
			err := readMatrixResponse(&http.Response{StatusCode: 200, Body: body})
			require.ErrorContains(t, err, test.want)
			assert.NotContains(t, err.Error(), "synthetic-private-value")
			assert.True(t, body.closed)
		})
	}
}
