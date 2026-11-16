// SPDX-License-Identifier: GPL-3.0-or-later

package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConstructorOwnsNumericOptions(t *testing.T) {
	topic, retries := field.Integer(7), field.Integer(0)
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var payload telegramMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, int64(7), *payload.MessageThreadID)
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"ok":false,"error_code":429,"parameters":{"retry_after":0}}`))}, nil
	})}
	sender, err := New(Config{BotToken: "123:token", ChatID: "1", MessageThreadID: &topic, RetriesOnLimit: &retries}, client)
	require.NoError(t, err)
	topic, retries = 9, 1
	// The zero retry budget captured at construction must return this rejection immediately.
	require.ErrorContains(t, sender.Send(context.Background(), testutil.ExpectedEvent()), "HTTP 429")
	require.Equal(t, 1, calls)
}
