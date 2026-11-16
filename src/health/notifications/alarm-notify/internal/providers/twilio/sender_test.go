// SPDX-License-Identifier: GPL-3.0-or-later

package twilio

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSenderResolvesEachSendWithSuppliedClient(t *testing.T) {
	t.Setenv("TWILIO_SENDER_TEST_TOKEN", "first-token")
	tokens := []string{}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sid, token, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, twilioTestSID, sid)
		tokens = append(tokens, token)
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(`{"sid":"accepted"}`))}, nil
	})}
	cfg := Config{AccountSID: twilioTestSID, AuthToken: "${env:TWILIO_SENDER_TEST_TOKEN}", From: "12345", To: "+15005550009"}
	sender, err := New(cfg, client)
	require.NoError(t, err)
	require.NoError(t, sender.Send(context.Background(), testutil.ExpectedEvent()))
	t.Setenv("TWILIO_SENDER_TEST_TOKEN", "second-token")
	require.NoError(t, sender.Send(context.Background(), testutil.ExpectedEvent()))
	assert.Equal(t, []string{"first-token", "second-token"}, tokens)
}
