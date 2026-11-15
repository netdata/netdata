// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunOneshot(t *testing.T) {
	setupRunner(t)
	large := bytes.Repeat([]byte("x"), testPipeBytes)
	tests := map[string]struct {
		body        string
		input       []byte
		timeout     time.Duration
		limit       int
		wantOutput  []byte
		wantErrText string
	}{
		"large input and output": {
			body:       "cat",
			input:      large,
			timeout:    10 * time.Second,
			wantOutput: large,
		},
		"unread input": {
			body:       "printf done",
			input:      large,
			timeout:    5 * time.Second,
			wantOutput: []byte("done"),
		},
		"exact limit": {
			body:       fmt.Sprintf("head -c %d /dev/zero", testMessageLimit),
			timeout:    10 * time.Second,
			wantOutput: make([]byte, testMessageLimit),
			limit:      testMessageLimit,
		},
		"over limit": {
			body:        fmt.Sprintf("head -c %d /dev/zero; sleep 30", testMessageLimit+1),
			timeout:     5 * time.Second,
			wantErrText: "exceeds 64 MiB",
			limit:       testMessageLimit,
		},
		"over limit with unread input": {
			body:        fmt.Sprintf("head -c %d /dev/zero; sleep 30", testMessageLimit+1),
			input:       large,
			timeout:     5 * time.Second,
			wantErrText: "exceeds 64 MiB",
			limit:       testMessageLimit,
		},
		"timeout with unread input": {
			body:        "sleep 30",
			input:       large,
			timeout:     50 * time.Millisecond,
			wantErrText: "deadline exceeded",
		},
		"nonzero exit": {
			body:        "printf '%s' 'SYNTHETIC_SECRET' >&2; exit 7",
			timeout:     time.Second,
			wantErrText: "exit status 7",
		},
		"timeout": {
			body:        "sleep 30",
			timeout:     50 * time.Millisecond,
			wantErrText: "deadline exceeded",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, _ := fixtureCollector(t, tc.body+"\n")
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			defer cancel()
			start := time.Now()
			limit := tc.limit
			if limit == 0 {
				limit = maxMessageBytes
			}
			data, err := runOneshot(ctx, c.definition.command, opCollect, tc.input, limit)
			assert.Less(t, time.Since(start), tc.timeout+time.Second)
			if tc.wantErrText != "" {
				require.ErrorContains(t, err, tc.wantErrText)
				assert.Nil(t, data)
				assert.NotContains(t, err.Error(), "SYNTHETIC_SECRET", "stderr is discarded")
				return
			}
			require.NoError(t, err)
			assert.Len(t, data, len(tc.wantOutput))
			assert.True(t, bytes.Equal(tc.wantOutput, data), "output must match")
		})
	}
}

func TestRunDescribe(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		body     string
		timeout  time.Duration // caller deadline; zero uses only the describe budget
		wantText string
	}{
		"nonzero exit": {
			body:     "printf 'private-output' >&2\nexit 7\n",
			wantText: "status 7",
		},
		"oversized": {
			body:     fmt.Sprintf("printf '%%*s' %d x\nsleep 30\n", testDescriptionLimit+1),
			wantText: "exceeds 64 MiB",
		},
		"caller deadline": {
			body:     "sleep 30\n",
			timeout:  100 * time.Millisecond,
			wantText: "deadline exceeded",
		},
		"describe deadline after closing stdout": {
			body:     "exec 1>&-\nsleep 30\n",
			wantText: "deadline exceeded",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, _ := fixtureCollector(t, tc.body)
			ctx := context.Background()
			if tc.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.timeout)
				defer cancel()
			}
			data, err := runDescribe(ctx, c.definition.command, testDescriptionLimit)
			require.ErrorContains(t, err, tc.wantText)
			assert.Nil(t, data)
			assert.NotContains(t, err.Error(), "private-output", "stderr is discarded")
		})
	}
}

func TestRunDescribe_LargeDescription(t *testing.T) {
	setupRunner(t)
	const header = "version: v1\n"
	tests := map[string]struct{ size, limit int }{
		"default budget across reads": {
			size:  testTransferBytes,
			limit: maxDescriptionBytes,
		},
		"exact configured budget": {
			size:  testDescriptionLimit,
			limit: testDescriptionLimit,
		},
	}
	for name, tc := range tests {
		size := tc.size
		t.Run(name, func(t *testing.T) {
			body := fmt.Sprintf(
				"printf '%%s\\n' 'version: v1'\nprintf '%%*s' %d ''\n",
				size-len(header),
			)
			c, _ := fixtureCollector(t, body)
			data, err := runDescribe(context.Background(), c.definition.command, tc.limit)
			require.NoError(t, err)
			assert.Len(t, data, size)
			_, err = parseDescription(data, c.definition.command)
			require.NoError(t, err, "large metadata must still compile as a valid package")
		})
	}
}

func TestRunDescribe_CanceledBeforeStart(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, "touch \"$(dirname \"$0\")/started\"\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runDescribe(ctx, c.definition.command, maxDescriptionBytes)
	require.ErrorIs(t, err, context.Canceled)
	requireNoFile(t, filepath.Join(dir, "started"))
}
