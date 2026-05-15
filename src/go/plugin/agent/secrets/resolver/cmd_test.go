// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveCmd(t *testing.T) {
	tests := map[string]struct {
		onWindowsSkip   bool
		buildCfg        func(t *testing.T) map[string]any
		wantErrContains string
		wantValue       string
		field           string
	}{
		"success": {
			onWindowsSkip: true,
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"password": "${cmd:/bin/echo hello}"}
			},
			field:     "password",
			wantValue: "hello",
		},
		"trims output": {
			onWindowsSkip: true,
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"val": "${cmd:/bin/echo secretval}"}
			},
			field:     "val",
			wantValue: "secretval",
		},
		"relative path rejected": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"val": "${cmd:echo hello}"}
			},
			wantErrContains: "command path must be absolute",
		},
		"empty command": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"val": "${cmd:}"}
			},
			wantErrContains: "empty command",
		},
		"nonexistent command": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"val": "${cmd:/nonexistent/command}"}
			},
			wantErrContains: "command failed",
		},
		"command with args": {
			onWindowsSkip: true,
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"val": "${cmd:/usr/bin/printf %s secret}"}
			},
			field:     "val",
			wantValue: "secret",
		},
		"script file": {
			onWindowsSkip: true,
			buildCfg: func(t *testing.T) map[string]any {
				dir := t.TempDir()
				script := filepath.Join(dir, "secret.sh")
				require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho mysecret\n"), 0700))
				return map[string]any{"val": "${cmd:" + script + "}"}
			},
			field:     "val",
			wantValue: "mysecret",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.onWindowsSkip && runtime.GOOS == "windows" {
				t.Skip("skipping on windows")
			}

			resolver := New()
			cfg := tc.buildCfg(t)
			err := resolver.Resolve(cfg)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantValue, cfg[tc.field])
		})
	}
}
