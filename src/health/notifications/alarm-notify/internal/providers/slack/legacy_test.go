// SPDX-License-Identifier: GPL-3.0-or-later

package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyOverridesValidation(t *testing.T) {
	for name, tt := range map[string]struct {
		overrides LegacyOverrides
		err       string
	}{
		"default channel":  {overrides: LegacyOverrides{Username: "netdata on node"}},
		"channel":          {overrides: LegacyOverrides{Channel: "#ops"}},
		"user":             {overrides: LegacyOverrides{Channel: "@user"}},
		"empty target":     {overrides: LegacyOverrides{Channel: "@"}, err: "identify one channel or user"},
		"channel space":    {overrides: LegacyOverrides{Channel: "#synthetic-private-value\u00a0other"}, err: "without whitespace"},
		"channel control":  {overrides: LegacyOverrides{Channel: "#synthetic-private-value\x00"}, err: "controls"},
		"username control": {overrides: LegacyOverrides{Username: "synthetic-private-value\n"}, err: "controls"},
		"icon URL":         {overrides: LegacyOverrides{IconURL: "https://example.test/icon.png"}},
		"relative icon":    {overrides: LegacyOverrides{IconURL: "/synthetic-private-value"}, err: "absolute HTTP(S)"},
		"icon credentials": {overrides: LegacyOverrides{IconURL: "https://user:synthetic-private-value@example.test/icon.png"}, err: "user information"},
		"icon boundary":    {overrides: LegacyOverrides{IconURL: "https://example.test/" + strings.Repeat("x", 255-len("https://example.test/"))}},
		"icon too long":    {overrides: LegacyOverrides{IconURL: "https://example.test/" + strings.Repeat("x", 256-len("https://example.test/"))}, err: "255-character"},
	} {
		t.Run(name, func(t *testing.T) {
			sender, err := New(Config{URL: "https://example.test", Legacy: &tt.overrides}, http.DefaultClient)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				assert.NotContains(t, err.Error(), "synthetic-private-value")
				assert.Nil(t, sender)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, sender)
			}
		})
	}
}

func TestConstructorOwnsLegacyOverrides(t *testing.T) {
	var received slackMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	overrides := &LegacyOverrides{Channel: "#ops", Username: "notifier", IconURL: "https://example.test/icon.png"}
	sender, err := New(Config{URL: server.URL, Legacy: overrides}, server.Client())
	require.NoError(t, err)
	*overrides = LegacyOverrides{Channel: "#other", Username: "changed", IconURL: "invalid"}
	event := testutil.ExpectedEvent()
	require.NoError(t, sender.Send(context.Background(), event))
	want, err := renderSlack(event)
	require.NoError(t, err)
	want.Channel, want.Username, want.IconURL = "#ops", "notifier", "https://example.test/icon.png"
	assert.Equal(t, want, received)
}
