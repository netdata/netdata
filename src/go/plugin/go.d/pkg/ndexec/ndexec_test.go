// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunner_run(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh scripts")
	}

	tmp := t.TempDir()

	writeExe := func(path, body string) {
		require.NoError(t, os.WriteFile(path, []byte(body), 0o755), "write %s", path)
	}

	// Target scripts the helper will exec into.
	echoArgs := filepath.Join(tmp, "echoargs.sh")
	writeExe(echoArgs, `#!/bin/sh
printf '%s|' "$@"
echo
`)

	longErr := filepath.Join(tmp, "longerr.sh")
	long := strings.Repeat("x", 9000) // > stderrLimit
	writeExe(longErr, `#!/bin/sh
printf '`+long+`' 1>&2
exit 17
`)

	sleeper := filepath.Join(tmp, "sleep.sh")
	writeExe(sleeper, `#!/bin/sh
sleep "$1"
`)

	// Fake helper (acts like nd-run/ndsudo): replaces itself with the target.
	helper := filepath.Join(tmp, "helper.sh")
	writeExe(helper, `#!/bin/sh
exec "$@"
`)

	type tc struct {
		helperPath  string
		argv        []string
		timeout     time.Duration
		wantOut     string
		wantErr     bool
		errContains []string
		check       func(t *testing.T, out []byte, err error)
	}

	tests := map[string]tc{
		"success_echo_args": {
			helperPath: helper,
			argv:       []string{echoArgs, `a b`, `c"d`},
			timeout:    time.Second,
			wantOut:    "a b|c\"d|\n",
		},
		"nonzero_with_trimmed_stderr": {
			helperPath:  helper,
			argv:        []string{longErr},
			timeout:     time.Second,
			wantErr:     true,
			errContains: []string{"stderr:", "truncated"},
		},
		"timeout": {
			helperPath:  helper,
			argv:        []string{sleeper, "2"},
			timeout:     200 * time.Millisecond,
			wantErr:     true,
			errContains: []string{"deadline"},
			check: func(t *testing.T, _ []byte, err error) {
				// Either errors.Is(err, context.DeadlineExceeded) or message contains it.
				assert.ErrorIs(t, err, context.DeadlineExceeded)
			},
		},
		"helper_missing": {
			helperPath:  filepath.Join(tmp, "missing", "helper.sh"),
			argv:        []string{echoArgs},
			timeout:     time.Second,
			wantErr:     true,
			errContains: []string{"no such file", "helper.sh"},
		},
	}

	r := &runner{} // we pass helperPath directly to run()

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			out, _, err := r.run(nil, tt.timeout, tt.helperPath, "RunTest", tt.argv...)

			if tt.wantErr {
				require.Error(t, err)
				for _, frag := range tt.errContains {
					assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(frag))
				}
			} else {
				require.NoError(t, err)
				if tt.wantOut != "" {
					assert.Equal(t, tt.wantOut, string(out))
				}
			}

			if tt.check != nil {
				tt.check(t, out, err)
			}
		})
	}
}
