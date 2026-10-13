// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// The fixture uses real SDK metadata, instrumentations, hooks and wire bodies.
// Its stdout is also consumable by receiver integration tests.
func TestPinnedSDKCaptureContract(t *testing.T) {
	requireNode(t)
	for _, enabled := range []bool{false, true} {
		name := "false"
		if enabled {
			name = "true"
		}
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("node", "testdata/capture-sdk.cjs", name)
			cmd.Stdin = strings.NewReader(Bootstrap("shop", BootstrapOptions{
				MeasureRate:        1,
				EventLogs:          enabled,
				ConsoleLogs:        enabled,
				FrustrationSignals: enabled,
				Tracing:            enabled,
			}))
			raw, err := cmd.Output()
			if exit, ok := err.(*exec.ExitError); ok {
				t.Log(string(exit.Stderr))
			}
			require.NoError(t, err)
			var bodies []json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &bodies))
			require.NotEmpty(t, bodies)
			events := map[string]*beacon.Beacon{}
			var views []beacon.Event
			var errors []beacon.Error
			var resources []beacon.Resource
			var vitals []beacon.Vital
			var spans []beacon.Span
			var logs []beacon.Log
			var navigation *beacon.Navigation
			for _, body := range bodies {
				b, err := Decode(body, Options{
					Now:                now,
					ConsoleLogs:        enabled,
					EventLogs:          enabled,
					FrustrationSignals: enabled,
					Tracing:            enabled,
				})
				require.NoError(t, err)
				for _, event := range b.Events {
					events[event.Name] = b
					if event.Kind == beacon.EventView {
						views = append(views, event)
					}
				}
				errors = append(errors, b.Errors...)
				resources = append(resources, b.Resources...)
				vitals = append(vitals, b.Vitals...)
				spans = append(spans, b.Spans...)
				logs = append(logs, b.Logs...)
				if b.Navigation != nil {
					navigation = b.Navigation
				}
			}
			for event, user := range map[string]string{"identified": "user-1", "anonymous": "", "second_user": "user-2"} {
				b := events[event]
				require.NotNil(t, b, event)
				assert.Equal(t, user, b.UserID)
				assert.Equal(t, "/products/:id", b.Path)
				assert.NotEmpty(t, b.ExperienceID)
				assert.Equal(t, "Chrome", b.Browser)
				assert.Equal(t, "release-1", b.AppVersion)
				assert.Equal(t, "test", b.Environment)
			}
			attrs := events["identified"].Events[0].Attrs
			if enabled {
				assert.Equal(t, map[string]string{"custom": "retained only in event logs", "url.full": "https://shop.example.org/api"}, attrs)
			} else {
				assert.Empty(t, attrs)
			}
			require.Len(t, views, 2)
			assert.Equal(t, "/orders/:id", views[1].Attrs["fromView"])
			assert.Equal(t, "/orders/:id", views[1].Attrs["toView"])
			assert.NotEmpty(t, views[1].ID)
			assert.Greater(t, views[1].Revision, views[0].Revision)
			require.NotNil(t, navigation)
			assert.Positive(t, navigation.Revision)
			assert.Equal(t, float64(90), navigation.LoadMS)
			assert.True(t, navigation.HasLoad)
			assert.Equal(t, float64(5), navigation.DCLMS)
			assert.True(t, navigation.HasDCL)
			require.Len(t, resources, 1)
			assert.NotEmpty(t, resources[0].ID)
			assert.Equal(t, beacon.Resource{
				ID:          resources[0].ID,
				HasDuration: true,
				Host:        "cdn.example.org",
				DurationMS:  23,
				TransferB:   456,
				Initiator:   "fetch",
			}, resources[0])
			require.Len(t, vitals, 1)
			assert.Equal(t, beacon.LCP, vitals[0].Name)
			assert.Equal(t, float64(1200), vitals[0].Value)
			assert.Equal(t, "button#pay", vitals[0].Element)
			require.Len(t, errors, 1)
			assert.Equal(t, "TypeError", errors[0].Type)
			assert.Equal(t, "render (https://shop.example.org/app.js:12:3)\nboot (https://shop.example.org/main.js:22:4)\n", errors[0].Stack)
			if enabled {
				require.Len(t, spans, 1)
				assert.Equal(t, []beacon.SpanAttr{{Key: "http.url", Str: "https://api.example.org/orders"}, {Key: "name", Str: "checkout"}}, spans[0].Attrs)
				require.Len(t, logs, 1)
				assert.Equal(t, errors[0].Stack, logs[0].Stack)
			} else {
				assert.Empty(t, spans)
				assert.Empty(t, logs)
			}
		})
	}
}
