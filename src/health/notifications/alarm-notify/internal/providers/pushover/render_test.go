// SPDX-License-Identifier: GPL-3.0-or-later

package pushover

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pushoverTestToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const pushoverTestUser = "UUUUUUUUUUUUUUUUUUUUUUUUUUUUUU"

func TestRenderPushover(t *testing.T) {
	full := testutil.ExpectedEvent()
	full.URL = "https://example.com/alert?id=1&view=chart#details"
	critical, recovery := full, full
	critical.Status = "CRITICAL"
	recovery.Status, recovery.PreviousStatus = "CLEAR", "CRITICAL"
	minimal := notifyevent.Event{Node: "node", Alert: "alert", Status: "WARNING", Summary: "summary", Timestamp: full.Timestamp}
	for name, test := range map[string]struct {
		event        notifyevent.Event
		fixture      string
		replacements []string
	}{
		"warning":                    {event: full, fixture: "pushover-full.json"},
		"critical":                   {event: critical, fixture: "pushover-full.json", replacements: []string{"WARNING", "CRITICAL", `"priority": 0`, `"priority": 1`}},
		"quiet recovery":             {event: recovery, fixture: "pushover-full.json", replacements: []string{"CLEAR → WARNING", "CRITICAL → CLEAR", "WARNING", "CLEAR", `"priority": 0`, `"priority": -1`}},
		"unknown values and no link": {event: minimal, fixture: "pushover-minimal.json"},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(
				renderPushover(Config{AppToken: pushoverTestToken, UserKey: pushoverTestUser}, test.event),
			)
			require.NoError(t, err)
			want, err := os.ReadFile(filepath.Join("testdata", test.fixture))
			require.NoError(t, err)
			assert.JSONEq(t, strings.NewReplacer(test.replacements...).Replace(string(want)), string(data))
		})
	}
}

func TestPushoverEscaping(t *testing.T) {
	input := `<b>"hot" & 'cold'</b>`
	escaped := `&lt;b&gt;&#34;hot&#34; &amp; &#39;cold&#39;&lt;/b&gt;`
	zero := 0.0
	event := notifyevent.Event{Node: input, Alert: input, Summary: input, Info: input, Chart: input, Context: input,
		Value: &zero, Units: input, Status: "WARNING", Timestamp: testutil.ExpectedEvent().Timestamp,
		URL: `https://example.com/?a="&b='<tag>`}
	assert.Equal(t, pushoverMessage{
		Token: pushoverTestToken, User: pushoverTestUser, HTML: 1, Timestamp: 1789387200,
		Title: input + " WARNING: " + input,
		Message: "<b>" + escaped + "</b>\n<i>" + escaped + "</i>\nNode: " + escaped + "\nAlert: " + escaped +
			"\nStatus: WARNING\nChart: " + escaped + "\nContext: " + escaped + "\nValue: 0 " + escaped,
		URL: event.URL, URLTitle: "View Netdata",
	}, renderPushover(Config{AppToken: pushoverTestToken, UserKey: pushoverTestUser}, event))
}

func TestPushoverShortening(t *testing.T) {
	for name, test := range map[string]struct {
		parts []pushoverText
		want  string
	}{
		"exact limit":             {parts: []pushoverText{{text: strings.Repeat("a", 1024)}}, want: strings.Repeat("a", 1024)},
		"one over":                {parts: []pushoverText{{text: strings.Repeat("a", 1025)}}, want: strings.Repeat("a", 1021) + "..."},
		"unicode":                 {parts: []pushoverText{{text: strings.Repeat("界", 1025)}}, want: strings.Repeat("界", 1021) + "..."},
		"non BMP and closing tag": {parts: []pushoverText{{text: strings.Repeat("😀", 1025), tag: "b"}}, want: "<b>" + strings.Repeat("😀", 1014) + "...</b>"},
		"whole entities":          {parts: []pushoverText{{text: strings.Repeat("&", 300), tag: "b"}}, want: "<b>" + strings.Repeat("&amp;", 202) + "...</b>"},
		"exact encoded limit":     {parts: []pushoverText{{text: strings.Repeat("&", 203) + "ab", tag: "b"}}, want: "<b>" + strings.Repeat("&amp;", 203) + "ab</b>"},
		"no room for next tag":    {parts: []pushoverText{{text: strings.Repeat("a", 1019)}, {text: "abcdef", tag: "i"}}, want: strings.Repeat("a", 1019) + "..."},
		"truncate second part":    {parts: []pushoverText{{text: "summary", tag: "b"}, {text: strings.Repeat("x", 1100), tag: "i"}}, want: "<b>summary</b>\n<i>" + strings.Repeat("x", 999) + "...</i>"},
	} {
		t.Run(name, func(t *testing.T) {
			got := renderPushoverText(test.parts)
			assert.Equal(t, test.want, got)
			assert.True(t, utf8.ValidString(got))
			assert.LessOrEqual(t, utf8.RuneCountInString(got), 1024)
		})
	}
}

func TestPushoverTitleAndURLLimits(t *testing.T) {
	for name, test := range map[string]struct {
		symbol string
		extra  int
	}{
		"ASCII boundary": {symbol: "a"}, "ASCII overflow": {symbol: "a", extra: 1},
		"Unicode boundary": {symbol: "界"}, "Unicode overflow": {symbol: "界", extra: 1},
		"non BMP boundary": {symbol: "😀"}, "non BMP overflow": {symbol: "😀", extra: 1},
	} {
		t.Run(name, func(t *testing.T) {
			const titlePrefix = "node WARNING: "
			const urlPrefix = "https://example.com/"
			event := notifyevent.Event{Node: "node", Alert: "alert", Status: "WARNING", Timestamp: testutil.ExpectedEvent().Timestamp,
				Summary: strings.Repeat(test.symbol, 250-len(titlePrefix)+test.extra),
				URL:     urlPrefix + strings.Repeat(test.symbol, 512-len(urlPrefix)+test.extra)}
			want := pushoverMessage{Token: pushoverTestToken, User: pushoverTestUser, HTML: 1, Timestamp: 1789387200,
				Title: titlePrefix + event.Summary, Message: "<b>" + event.Summary + "</b>\nNode: node\nAlert: alert\nStatus: WARNING",
				URL: event.URL, URLTitle: "View Netdata"}
			if test.extra != 0 {
				want.Title = titlePrefix + strings.Repeat(test.symbol, 247-len(titlePrefix)) + "..."
				want.URL, want.URLTitle = "", ""
			}
			assert.Equal(
				t,
				want,
				renderPushover(Config{AppToken: pushoverTestToken, UserKey: pushoverTestUser}, event),
			)
		})
	}
}

func TestReadPushoverResponse(t *testing.T) {
	for name, test := range map[string]struct {
		status    int
		body, err string
	}{
		"queued":            {status: 200, body: `{"status":1,"request":"synthetic-private-value"}`},
		"rejected":          {status: 200, body: `{"status":0,"errors":["synthetic-private-value"]}`, err: "API rejected"},
		"unexpected status": {status: 200, body: `{"status":2}`, err: "API rejected"},
		"missing status":    {status: 200, body: `{}`, err: "invalid pushover response"},
		"null status":       {status: 200, body: `{"status":null}`, err: "invalid pushover response"},
		"string status":     {status: 200, body: `{"status":"1"}`, err: "invalid pushover response"},
		"null":              {status: 200, body: `null`, err: "invalid pushover response"},
		"invalid JSON":      {status: 200, body: `synthetic-private-value`, err: "invalid pushover response"},
		"trailing JSON":     {status: 200, body: `{"status":1}{}`, err: "invalid pushover response"},
		"size boundary":     {status: 200, body: `{"status":1}` + strings.Repeat(" ", httpclient.ResponseLimit-12)},
		"oversized":         {status: 200, body: `{"status":1}` + strings.Repeat(" ", httpclient.ResponseLimit), err: "256 KiB"},
		"unconfirmed":       {status: 204, err: "HTTP 204"},
		"quota":             {status: 429, body: `{"status":1}`, err: "HTTP 429"},
		"server error":      {status: 503, body: `synthetic-private-value`, err: "HTTP 503"},
	} {
		t.Run(name, func(t *testing.T) {
			reader := strings.NewReader(test.body)
			body := &testutil.TrackingBody{Reader: reader}
			err := readPushoverResponse(&http.Response{StatusCode: test.status, Body: body})
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
}
