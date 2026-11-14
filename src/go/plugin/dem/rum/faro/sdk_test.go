// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is required to execute the shipped SDK browser fixtures")
	}
}

func TestLoaderURLAndFailureContract(t *testing.T) {
	requireNode(t)
	payload, err := json.Marshal(map[string]string{
		"core": Bootstrap("shop", BootstrapOptions{
			MeasureRate: 1,
		}),
		"tracing": Bootstrap("shop", BootstrapOptions{
			MeasureRate: 1,
			Tracing:     true,
		}),
		"disabled": Bootstrap("shop", BootstrapOptions{}),
	})
	require.NoError(t, err)
	cmd := exec.Command("node", "testdata/loader.cjs")
	cmd.Stdin = bytes.NewReader(payload)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

func TestPinnedSDKDocumentIdentity(t *testing.T) {
	requireNode(t)
	cmd := exec.Command("node", "testdata/identity-sdk.cjs")
	cmd.Stdin = strings.NewReader(Bootstrap("shop", BootstrapOptions{
		MeasureRate: 1,
	}))
	raw, err := cmd.Output()
	if exit, ok := err.(*exec.ExitError); ok {
		t.Log(string(exit.Stderr))
	}
	require.NoError(t, err)
	var bodies []json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &bodies))
	var documents, views int
	for _, body := range bodies {
		b, err := Decode(body, Options{
			Now: now,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, b.ExperienceID)
		assert.Equal(t, "/entry", b.PageGroup)
		for _, event := range b.Events {
			switch event.Kind {
			case beacon.EventSession:
				assert.NotEmpty(t, event.ID)
				assert.Positive(t, event.Revision)
				assert.Equal(t, event.ID, event.Attrs["observation_id"])
			case beacon.EventDocument:
				documents++
				assert.Equal(t, b.ExperienceID, event.ID)
				assert.Positive(t, event.Revision)
			case beacon.EventView:
				views++
				assert.Equal(t, b.ViewID, event.ID)
				assert.Positive(t, event.Revision)
			}
		}
		for _, vital := range b.Vitals {
			assert.NotEmpty(t, vital.ID)
			assert.Positive(t, vital.Revision)
		}
		if b.Navigation != nil {
			assert.Positive(t, b.Navigation.Revision)
		}
		for _, resource := range b.Resources {
			assert.NotEmpty(t, resource.ID)
			assert.True(t, resource.HasDuration)
			assert.Zero(t, resource.DurationMS)
		}
	}
	assert.Equal(t, 2, documents)
	assert.Equal(t, 2, views)
}

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

// The shipped, checksum-pinned SDK is exercised with Node and synthetic browser APIs.
func TestPinnedSDKConsoleContract(t *testing.T) {
	requireNode(t)
	for _, enabled := range []bool{false, true} {
		name := "false"
		if enabled {
			name = "true"
		}
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("node", "testdata/console-sdk.cjs", name)
			cmd.Stdin = strings.NewReader(
				Bootstrap("shop", BootstrapOptions{
					MeasureRate: 1,
					ConsoleLogs: enabled,
				}),
			)
			raw, err := cmd.Output()
			if exit, ok := err.(*exec.ExitError); ok {
				t.Log(string(exit.Stderr))
			}
			require.NoError(t, err)
			b, err := Decode(raw, Options{
				Now:         now,
				ConsoleLogs: enabled,
			})
			require.NoError(t, err)
			require.Len(t, b.Errors, 2)
			assert.Equal(t, "real exception", b.Errors[0].Message)
			assert.Equal(t, "real rejection", b.Errors[1].Message)
			if !enabled {
				assert.Empty(t, b.Logs)
				return
			}
			require.Len(t, b.Logs, 4)
			assert.Equal(t, "console.error: string error", b.Logs[2].Message)
			assert.Empty(t, b.Logs[2].ErrorType)
			assert.Empty(t, b.Logs[2].Stack)
			assert.Equal(t, "TypeError", b.Logs[3].ErrorType)
			assert.Equal(t, "console.error: object error", b.Logs[3].Message)
			assert.Equal(
				t,
				"render (https://shop.example.org/app.js:12:3)\nboot (https://shop.example.org/main.js:22:4)\n",
				b.Logs[3].Stack,
			)
		})
	}
}

// Like the console oracle, this executes the unchanged shipped SDK bundle.
// The browser surface and transport sink are fixtures; initialization,
// session persistence, sampling and transport filtering are the real SDK.
func TestPinnedSDKSamplingContract(t *testing.T) {
	requireNode(t)
	bootstraps := map[string]string{}
	for name, rate := range map[string]float64{"zero": 0, "fraction": 0.25, "full": 1} {
		bootstraps[name] = Bootstrap("shop", BootstrapOptions{
			MeasureRate: rate,
		})
	}
	encoded, err := json.Marshal(bootstraps)
	require.NoError(t, err)
	cmd := exec.Command("node", "testdata/sampling-sdk.cjs")
	cmd.Stdin = strings.NewReader(string(encoded))
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
}

func TestPinnedSDKBotTransportURLExclusion(t *testing.T) {
	requireNode(t)
	cmd := exec.Command("node", "testdata/identity-sdk.cjs", "bot")
	cmd.Stdin = strings.NewReader(Bootstrap("shop", BootstrapOptions{
		MeasureRate: 1,
		IncludeBots: true,
	}))
	_, err := cmd.Output()
	if exit, ok := err.(*exec.ExitError); ok {
		t.Log(string(exit.Stderr))
	}
	require.NoError(t, err)
}
