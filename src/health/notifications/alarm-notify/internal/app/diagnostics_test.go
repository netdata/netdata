// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigOpenDiagnostics(t *testing.T) {
	for name, test := range map[string]struct {
		unreadable bool
		cause      error
	}{
		"missing":           {cause: os.ErrNotExist},
		"permission denied": {unreadable: true, cause: os.ErrPermission},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "synthetic-private-value")
			if test.unreadable {
				require.NoError(t, os.WriteFile(path, []byte("A=ok\n"), 0000))
			}
			file, openErr := os.Open(path)
			if file != nil {
				require.NoError(t, file.Close())
				if test.unreadable {
					t.Skip("environment permits reading a file with mode 0000")
				}
			}
			require.ErrorIs(t, openErr, test.cause)
			var pathErr *os.PathError
			require.ErrorAs(t, openErr, &pathErr)

			for command, target := range map[string]struct {
				args    []string
				call    func() error
				message string
			}{
				"check-legacy": {
					args: []string{"--config", writeConfig(t, "A=ok"), "--config", path},
					call: func() error {
						return checkLegacy(context.Background(), []string{writeConfig(t, "A=ok"), path})
					},
					message: "legacy configuration file 2: could not open file",
				},
				"validate": {
					args: []string{"--config", path},
					call: func() error {
						return execute(context.Background(), nil, path, true, "", nil, panicReader{}, nil)
					},
					message: "could not open configuration file",
				},
				"send": {
					args: []string{"--config", path, "--destination", "dev"},
					call: func() error {
						return execute(context.Background(), nil, path, false, "dev", nil, panicReader{}, nil)
					},
					message: "could not open configuration file",
				},
			} {
				t.Run(command, func(t *testing.T) {
					err := target.call()
					assert.ErrorIs(t, err, test.cause)
					want := fmt.Sprintf("%s: %s", target.message, pathErr.Err)
					assert.EqualError(t, err, want)
					var stdout, stderr bytes.Buffer
					code := Run(context.Background(), append([]string{command}, target.args...), panicReader{}, &stdout, &stderr)
					assert.Equal(t, 1, code)
					assert.Empty(t, stdout.String())
					prefix := ""
					if command == "send" {
						prefix = "alarm-notify: delivery summary: 0 succeeded, 0 failed, 0 skipped\n"
					}
					assert.Equal(t, prefix+"alarm-notify: "+want+"\n", stderr.String())
					assert.NotContains(t, stderr.String(), path)
					assert.NotContains(t, stderr.String(), "synthetic-private-value")
				})
			}
		})
	}
}

func TestRunCancellationDiagnostics(t *testing.T) {
	for command, test := range map[string]struct {
		args    []string
		subject string
	}{
		"check-legacy": {subject: "legacy configuration check"},
		"validate":     {subject: "configuration validation"},
		"send":         {args: []string{"--destination", "dev"}, subject: "notification"},
	} {
		t.Run(command, func(t *testing.T) {
			for name, failure := range map[string]struct {
				deadline bool
				message  string
			}{
				"canceled": {message: "canceled"},
				"deadline": {deadline: true, message: "timed out"},
			} {
				t.Run(name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					if failure.deadline {
						cancel()
						ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
					}
					defer cancel()
					cancel()
					var stdout, stderr bytes.Buffer
					args := append([]string{command, "--config", "synthetic-private-value"}, test.args...)
					assert.Equal(t, 1, Run(ctx, args, panicReader{}, &stdout, &stderr))
					assert.Empty(t, stdout.String())
					prefix := ""
					if command == "send" {
						prefix = "alarm-notify: delivery summary: 0 succeeded, 0 failed, 0 skipped\n"
					}
					assert.Equal(t, prefix+"alarm-notify: "+test.subject+" "+failure.message+"\n", stderr.String())
				})
			}
		})
	}
}
