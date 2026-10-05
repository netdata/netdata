// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// FARO_SDK_BUNDLE points at the unchanged CDN bundle, pinned by SHA256 in the
// fixture. This opt-in check requires Node, but no browser or npm dependencies:
// https://cdn.jsdelivr.net/npm/@grafana/faro-web-sdk@2.11.0/dist/bundle/faro-web-sdk.iife.js
func TestPinnedSDKConsoleContract(t *testing.T) {
	if os.Getenv("FARO_SDK_BUNDLE") == "" {
		t.Skip("set FARO_SDK_BUNDLE to the pinned SDK IIFE to execute the upstream oracle")
	}
	for _, enabled := range []bool{false, true} {
		name := "false"
		if enabled {
			name = "true"
		}
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("node", "testdata/console-sdk.cjs", name)
			cmd.Stdin = strings.NewReader(
				Bootstrap("shop", "https://rum.example.org", BootstrapOptions{
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

func TestConsoleContextNormalization(t *testing.T) {
	for name, ctx := range map[string]any{
		"absent": nil, "wrong context": "malformed", "wrong type": map[string]any{"type": 42, "stackFrames": true},
		"null frame":         map[string]any{"type": "TypeError", "stackFrames": "null"},
		"bad frames":         map[string]any{"type": "TypeError", "stackFrames": "{not json}"},
		"wrong frame fields": map[string]any{"type": "TypeError", "stackFrames": `{"filename":123}`},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(
				map[string]any{
					"logs":       []any{map[string]any{"message": "message", "level": "error", "context": ctx}},
					"exceptions": []any{map[string]any{"type": "Error", "value": "core"}},
				},
			)
			require.NoError(t, err)
			b, err := Decode(raw, Options{
				Now:         now,
				ConsoleLogs: true,
			})
			require.NoError(t, err)
			require.Len(t, b.Logs, 1)
			require.Len(t, b.Errors, 1)
			assert.Empty(t, b.Logs[0].Stack)
			off, err := Decode(raw, Options{
				Now: now,
			})
			require.NoError(t, err)
			assert.Empty(t, off.Logs)
			require.Len(t, off.Errors, 1)
		})
	}
	frame, _ := json.Marshal(
		map[string]any{
			"filename": "https://example.org/users/123456/app.js?password=urlsecret",
			"function": "token=framesecret",
			"lineno":   12,
			"colno":    3,
		},
	)
	raw, err := json.Marshal(
		map[string]any{
			"logs": []any{
				map[string]any{
					"message": "password=messagesecret",
					"context": map[string]any{
						"type":        "token=typesecret",
						"stackFrames": string(frame),
						"arbitrary":   "discard me",
					},
				},
			},
		},
	)
	require.NoError(t, err)
	b, err := Decode(raw, Options{
		Now:         now,
		ConsoleLogs: true,
	})
	require.NoError(t, err)
	require.Len(t, b.Logs, 1)
	assert.Equal(t, "password=[REDACTED]", b.Logs[0].Message)
	assert.Equal(t, "token=[REDACTED]", b.Logs[0].ErrorType)
	assert.Equal(t, "token=[REDACTED] (https://example.org/users/:id/app.js:12:3)\n", b.Logs[0].Stack)
	frame, _ = json.Marshal(map[string]any{"filename": strings.Repeat("a", maxStackLen*2)})
	raw, _ = json.Marshal(
		map[string]any{
			"logs": []any{
				map[string]any{
					"message": strings.Repeat("m", maxMessageLen*2),
					"context": map[string]any{"type": strings.Repeat("t", maxNameLen*2), "stackFrames": string(frame)},
				},
			},
		},
	)
	b, err = Decode(raw, Options{
		Now:         now,
		ConsoleLogs: true,
	})
	require.NoError(t, err)
	require.Len(t, b.Logs, 1)
	assert.Len(t, b.Logs[0].Message, maxMessageLen)
	assert.Len(t, b.Logs[0].ErrorType, maxNameLen)
	assert.Len(t, b.Logs[0].Stack, maxStackLen)
	assert.Empty(t, b.Errors)
}
