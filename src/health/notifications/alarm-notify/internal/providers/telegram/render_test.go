// SPDX-License-Identifier: GPL-3.0-or-later

package telegram

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTelegram(t *testing.T) {
	full := testutil.ExpectedEvent()
	full.URL = "https://example.com/alert?id=1&view=chart#details"
	critical, recovery := full, full
	critical.Status = "CRITICAL"
	recovery.Status, recovery.PreviousStatus = "CLEAR", "CRITICAL"
	minimal := notifyevent.Event{Node: "node", Alert: "alert", Status: "WARNING", Summary: "summary", Timestamp: full.Timestamp}
	topic := field.Integer(7)
	fullConfig := Config{ChatID: "-100123", MessageThreadID: &topic}
	tests := map[string]struct {
		event        notifyevent.Event
		dst          Config
		fixture      string
		replacements []string
	}{
		"warning": {event: full, dst: fullConfig, fixture: "telegram-full.json"},
		"critical": {
			event:        critical,
			dst:          fullConfig,
			fixture:      "telegram-full.json",
			replacements: []string{"⚠️", "🔴", "WARNING", "CRITICAL"},
		},
		"silent recovery": {
			event:   recovery,
			dst:     fullConfig,
			fixture: "telegram-full.json",
			replacements: []string{
				"⚠️",
				"✅",
				"CLEAR → WARNING",
				"CRITICAL → CLEAR",
				"WARNING",
				"CLEAR",
				`"disable_notification": false`,
				`"disable_notification": true`,
			},
		},
		"unknown values and no topic": {
			event:   minimal,
			dst:     Config{ChatID: "@example_alerts"},
			fixture: "telegram-minimal.json",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := renderTelegram(test.dst, test.event)
			require.NoError(t, err)
			data, err := json.Marshal(got)
			require.NoError(t, err)
			want, err := os.ReadFile(filepath.Join("testdata", test.fixture))
			require.NoError(t, err)
			assert.JSONEq(t, strings.NewReplacer(test.replacements...).Replace(string(want)), string(data))
		})
	}
}

func TestTelegramEscaping(t *testing.T) {
	input := `<b>"hot" & 'cold'</b>`
	escaped := `&lt;b&gt;&#34;hot&#34; &amp; &#39;cold&#39;&lt;/b&gt;`
	value := 0.0
	event := notifyevent.Event{Node: input, Alert: input, Summary: input, Info: input, Chart: input, Context: input,
		Value: &value, Units: input, Status: "WARNING", Timestamp: testutil.ExpectedEvent().Timestamp,
		URL: `https://example.com/?a="&b='<tag>`}
	got, err := renderTelegram(Config{ChatID: "1"}, event)
	require.NoError(t, err)
	assert.Equal(t, telegramMessage{
		ChatID: "1", ParseMode: "HTML", LinkPreviewOptions: telegramLinkPreview{IsDisabled: true},
		Text: "⚠️ <b>WARNING: " + escaped + "</b>\n<b>Node:</b> " + escaped + "\n<b>Alert:</b> " + escaped +
			"\n<b>Status:</b> WARNING\n<b>Chart:</b> " + escaped + "\n<b>Context:</b> " + escaped +
			"\n<b>Value:</b> 0 " + escaped + "\n<b>Time:</b> 2026-09-14T12:00:00Z\n<i>" + escaped +
			`</i>` + "\n" + `<a href="https://example.com/?a=&#34;&amp;b=&#39;&lt;tag&gt;">View alert</a>`,
	}, got)
}

func TestTelegramTextLimits(t *testing.T) {
	const fixed = "⚠️ WARNING: \nNode: node\nAlert: alert\nStatus: WARNING\nTime: 2026-09-14T12:00:00Z"
	available := 4096 - utf8.RuneCountInString(fixed)
	for name, test := range map[string]struct {
		symbol string
		extra  int
	}{
		"ASCII boundary": {symbol: "a"}, "ASCII overflow": {symbol: "a", extra: 1},
		"Unicode boundary": {symbol: "界"}, "Unicode overflow": {symbol: "界", extra: 1},
		"non-BMP boundary": {symbol: "😀"}, "non-BMP overflow": {symbol: "😀", extra: 1},
		"entities do not consume visible space": {symbol: "&"}, "escaped overflow": {symbol: "&", extra: 1},
	} {
		t.Run(name, func(t *testing.T) {
			event := notifyevent.Event{Node: "node", Alert: "alert", Status: "WARNING", Timestamp: testutil.ExpectedEvent().Timestamp,
				Summary: strings.Repeat(test.symbol, available+test.extra)}
			got, err := renderTelegram(Config{ChatID: "1"}, event)
			if test.extra != 0 {
				require.ErrorContains(t, err, "4096-character")
				assert.Equal(t, telegramMessage{}, got)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, got.Text)
		})
	}
}

func TestTelegramEndpoint(t *testing.T) {
	for name, test := range map[string]struct{ base, token, want, err string }{
		"official":               {token: "123:synthetic-private-value", want: "https://api.telegram.org/bot123:synthetic-private-value/sendMessage"},
		"local server prefix":    {base: "http://localhost:8081/proxy/", token: "123:synthetic-private-value", want: "http://localhost:8081/proxy/bot123:synthetic-private-value/sendMessage"},
		"token path injection":   {token: "123:synthetic-private-value/other", err: "bot_token"},
		"token query injection":  {token: "123:synthetic-private-value?x", err: "bot_token"},
		"query":                  {base: "https://example.com/?secret=synthetic-private-value", err: "query"},
		"empty fragment":         {base: "https://example.com/#", token: "123:synthetic-private-value", err: "fragment"},
		"encoded hash in prefix": {base: "http://localhost:8081/proxy%23/", token: "123:synthetic-private-value", want: "http://localhost:8081/proxy%23/bot123:synthetic-private-value/sendMessage"},
		"userinfo":               {base: "https://user:synthetic-private-value@example.com", err: "without user information"},
		"official plaintext":     {base: "http://API.TELEGRAM.ORG./", err: "requires HTTPS"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := telegramEndpoint(test.base, test.token)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestReadTelegramResponse(t *testing.T) {
	ok, failed, delay := true, false, int64(2)
	limited := telegramResponse{OK: &failed, ErrorCode: 429}
	limited.Parameters.RetryAfter = &delay
	for name, test := range map[string]struct {
		status int
		body   string
		want   telegramResponse
		err    string
	}{
		"success":                {status: 200, body: `{"ok":true,"result":{"message_id":1,"text":"synthetic-private-value"}}`, want: telegramResponse{OK: &ok}},
		"API error":              {status: 200, body: `{"ok":false,"error_code":400,"description":"synthetic-private-value"}`, want: telegramResponse{OK: &failed, ErrorCode: 400}},
		"rate limited":           {status: 429, body: `{"ok":false,"error_code":429,"parameters":{"retry_after":2}}`, want: limited},
		"missing retry delay":    {status: 429, body: `{"ok":false}`, want: telegramResponse{OK: &failed}},
		"missing ok":             {status: 200, body: `{}`, err: "invalid telegram response"},
		"null":                   {status: 200, body: `null`, err: "invalid telegram response"},
		"null ok":                {status: 200, body: `{"ok":null}`, err: "invalid telegram response"},
		"wrong ok":               {status: 200, body: `{"ok":"synthetic-private-value"}`, err: "invalid telegram response"},
		"malformed":              {status: 200, body: `synthetic-private-value`, err: "invalid telegram response"},
		"trailing document":      {status: 200, body: `{"ok":true} {}`, err: "invalid telegram response"},
		"negative delay":         {status: 429, body: `{"ok":false,"parameters":{"retry_after":-1}}`, err: "invalid telegram retry delay"},
		"delay integer overflow": {status: 429, body: `{"ok":false,"parameters":{"retry_after":9223372036854775808}}`, err: "invalid telegram response"},
		"response boundary":      {status: 200, body: `{"ok":true}` + strings.Repeat(" ", httpclient.ResponseLimit-len(`{"ok":true}`)), want: telegramResponse{OK: &ok}},
		"oversized":              {status: 200, body: strings.Repeat(" ", httpclient.ResponseLimit+1), err: "256 KiB limit"},
		"unexpected HTTP status": {status: 503, body: `synthetic-private-value`, err: "HTTP 503"},
	} {
		t.Run(name, func(t *testing.T) {
			body := &testutil.TrackingBody{Reader: strings.NewReader(test.body)}
			got, err := readTelegramResponse(&http.Response{StatusCode: test.status, Body: body})
			assert.True(t, body.Closed)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Equal(t, telegramResponse{}, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestWaitTelegramRetry(t *testing.T) {
	for name, test := range map[string]struct {
		seconds                       int64
		timeout, cancelAfter, elapsed time.Duration
		err                           string
	}{
		"server delay":      {seconds: 2, timeout: 10 * time.Second, elapsed: 2 * time.Second},
		"zero delay":        {timeout: time.Second},
		"cannot fit":        {seconds: 2, timeout: time.Second, err: "remaining deadline"},
		"exact deadline":    {seconds: 2, timeout: 2 * time.Second, err: "remaining deadline"},
		"duration overflow": {seconds: math.MaxInt64, timeout: time.Second, err: "remaining deadline"},
		"negative":          {seconds: -1, timeout: time.Second, err: "invalid telegram retry delay"},
		"cancel wait":       {seconds: 2, timeout: 10 * time.Second, cancelAfter: time.Second, elapsed: time.Second, err: "notification canceled"},
		"already canceled":  {seconds: 2, timeout: 10 * time.Second, cancelAfter: -1, err: "notification canceled"},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), test.timeout)
				defer cancel()
				if test.cancelAfter < 0 {
					cancel()
				}
				if test.cancelAfter > 0 {
					go func() { time.Sleep(test.cancelAfter); cancel() }()
				}
				start := time.Now()
				err := waitTelegramRetry(ctx, test.seconds)
				assert.Equal(t, test.elapsed, time.Since(start))
				if test.err != "" {
					require.ErrorContains(t, err, test.err)
				} else {
					require.NoError(t, err)
				}
			})
		})
	}
}
