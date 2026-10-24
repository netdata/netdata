// SPDX-License-Identifier: GPL-3.0-or-later

package msteams

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendPayloadLimit(t *testing.T) {
	for name, symbol := range map[string]string{"ASCII": "x", "Unicode": "界", "JSON escaping": "\"", "Markdown": "*", "HTML": "<"} {
		for suffix, extra := range map[string]int{"boundary": 0, "over": 1} {
			t.Run(name+"/"+suffix, func(t *testing.T) {
				sizes := make(chan int, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					data, _ := io.ReadAll(r.Body)
					sizes <- len(data)
					w.WriteHeader(202)
				}))
				defer server.Close()
				dst := testDestination()
				dst.URL = server.URL
				event := testutil.EventForStatus("WARNING", "minimal")
				event.Info = "x"
				baseline, err := json.Marshal(renderMSTeams(dst, event))
				require.NoError(t, err)
				event.Info = symbol
				sample, err := json.Marshal(renderMSTeams(dst, event))
				require.NoError(t, err)
				width := len(sample) - len(baseline) + 1
				remaining := msTeamsPayloadLimit - len(baseline) + 1
				event.Info = strings.Repeat(symbol, remaining/width) + strings.Repeat("x", remaining%width+extra)
				sender, err := New(dst, server.Client())
				require.NoError(t, err)
				err = sender.Send(context.Background(), event)

				if extra == 0 {
					require.NoError(t, err)
					require.Len(t, sizes, 1)
					assert.Equal(t, msTeamsPayloadLimit, <-sizes)
				} else {
					require.Error(t, err)
					assert.Empty(t, sizes)
					assert.ErrorContains(t, err, "28 KiB")
				}
			})
		}
	}
}

func TestConstructorOwnsStyleMaps(t *testing.T) {
	icons, colors := map[string]string{"warning": "custom"}, map[string]string{"warning": "123456"}
	var received msTeamsMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	sender, err := New(Config{URL: server.URL, Icons: icons, Colors: colors}, server.Client())
	require.NoError(t, err)
	icons["warning"], colors["warning"] = "changed", "654321"
	require.NoError(t, sender.Send(context.Background(), testutil.EventForStatus("WARNING", "minimal")))
	require.Equal(t, "123456", received.ThemeColor)
	require.Equal(t, "custom Alert WARNING from Netdata on node", received.Title)
}
