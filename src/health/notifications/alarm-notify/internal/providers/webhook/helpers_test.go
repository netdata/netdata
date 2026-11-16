// SPDX-License-Identifier: GPL-3.0-or-later

package webhook

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSenderResolvesSecretsForEachSend(t *testing.T) {
	t.Setenv("WEBHOOK_TEST_URL", "https://first.example/hook")
	t.Setenv("WEBHOOK_TEST_TOKEN", "first-token")
	var endpoints, auth []string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		endpoints = append(endpoints, r.URL.String())
		auth = append(auth, r.Header.Get("Authorization"))
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	sender, err := New(Config{URL: "${env:WEBHOOK_TEST_URL}", BearerToken: "${env:WEBHOOK_TEST_TOKEN}"}, client)
	require.NoError(t, err)
	require.NoError(t, sender.Send(context.Background(), testutil.ExpectedEvent()))
	t.Setenv("WEBHOOK_TEST_URL", "https://second.example/hook")
	t.Setenv("WEBHOOK_TEST_TOKEN", "second-token")
	require.NoError(t, sender.Send(context.Background(), testutil.ExpectedEvent()))
	require.Equal(t, []string{"https://first.example/hook", "https://second.example/hook"}, endpoints)
	require.Equal(t, []string{"Bearer first-token", "Bearer second-token"}, auth)
}
