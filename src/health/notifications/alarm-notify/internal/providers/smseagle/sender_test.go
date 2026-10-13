// SPDX-License-Identifier: GPL-3.0-or-later

package smseagle

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSenderOwnsMutableConfig(t *testing.T) {
	duration, voice := configfield.Integer(11), configfield.Integer(2)
	cfg := Config{APIURL: "http://example.com", AccessToken: "synthetic-token", Recipients: []string{"12345"}, MessageType: "tts_advanced", CallDuration: &duration, VoiceID: &voice}
	var got map[string]any
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[{"status":"queued","id":1}]`))}, nil
	})}
	sender, err := New(cfg, client)
	require.NoError(t, err)
	cfg.Recipients[0] = "98765"
	duration, voice = 99, 99
	require.NoError(t, sender.Send(context.Background(), testutil.ExpectedEvent()))
	assert.Equal(t, []any{"12345"}, got["to"])
	assert.Equal(t, float64(11), got["duration"])
	assert.Equal(t, float64(2), got["voice_id"])
}
