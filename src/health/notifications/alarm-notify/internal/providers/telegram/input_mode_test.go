// SPDX-License-Identifier: GPL-3.0-or-later

package telegram

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInputModeValidationAndSend(t *testing.T) {
	t.Setenv("NOTIFIER_TELEGRAM_TOKEN", "123:token")
	zero := field.Integer(0)
	for name, test := range map[string]struct {
		cfg Config
		err string
	}{
		"literal valid":                    {cfg: Config{Secrets: secret.LiteralInput, BotToken: "123:token", ChatID: "1"}},
		"native token reference":           {cfg: Config{BotToken: "${env:NOTIFIER_TELEGRAM_TOKEN}", ChatID: "1"}},
		"literal token reference rejected": {cfg: Config{Secrets: secret.LiteralInput, BotToken: "${env:NOTIFIER_TELEGRAM_TOKEN}", ChatID: "1"}, err: "bot_token must have the form"},
		"literal zero chat rejected":       {cfg: Config{Secrets: secret.LiteralInput, BotToken: "123:token", ChatID: "0"}, err: "nonzero signed 64-bit"},
		"literal chat overflow rejected":   {cfg: Config{Secrets: secret.LiteralInput, BotToken: "123:token", ChatID: "9223372036854775808"}, err: "nonzero signed 64-bit"},
		"literal zero topic rejected":      {cfg: Config{Secrets: secret.LiteralInput, BotToken: "123:token", ChatID: "1", MessageThreadID: &zero}, err: "message_thread_id must be positive"},
	} {
		t.Run(name, func(t *testing.T) {
			var paths []string
			client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":1}}`))}, nil
			})}
			sender, err := New(test.cfg, client)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.Nil(t, sender)
				assert.Empty(t, paths)
				return
			}
			require.NoError(t, err)
			require.NoError(t, sender.Send(t.Context(), testutil.ExpectedEvent()))
			assert.Equal(t, []string{"/bot123:token/sendMessage"}, paths)
		})
	}
}
