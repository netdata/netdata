// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
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
	tests := map[string]struct {
		body     string
		timeout  time.Duration
		wantText string
	}{
		"nonzero exit": {
			body:     "printf '%s' 'SYNTHETIC_SECRET' >&2; exit 7",
			timeout:  time.Second,
			wantText: "exit status 7",
		},
		"timeout": {
			body:     "sleep 30",
			timeout:  50 * time.Millisecond,
			wantText: "deadline exceeded",
		},
		"over limit": {
			body:     fmt.Sprintf("head -c %d /dev/zero; sleep 30", maxMessageBytes+1),
			timeout:  5 * time.Second,
			wantText: "exceeds 64 MiB",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, _ := fixtureCollector(t, tc.body+"\n")
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			defer cancel()
			start := time.Now()
			data, err := runOneshot(ctx, c.definition.command, "collect", nil)
			require.ErrorContains(t, err, tc.wantText)
			assert.Nil(t, data)
			assert.NotContains(t, err.Error(), "SYNTHETIC_SECRET", "stderr is discarded")
			assert.Less(t, time.Since(start), tc.timeout+time.Second)
		})
	}
}

// The protocol boundary is tested independently of executable output chunking.
func TestLimitedBuffer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := limitedBuffer{
		cancel: cancel,
	}
	n, err := b.Write(make([]byte, maxMessageBytes))
	require.NoError(t, err)
	assert.Equal(t, maxMessageBytes, n)
	require.NoError(t, ctx.Err())
	_, err = b.Write([]byte{1})
	require.ErrorIs(t, err, errResponseTooLarge)
	assert.True(t, b.exceeded)
	assert.ErrorIs(t, ctx.Err(), context.Canceled, "exceeding the limit cancels the command")
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
			body:     fmt.Sprintf("printf '%%*s' %d x\nsleep 30\n", maxDescriptionBytes+1),
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
			data, err := runDescribe(ctx, c.definition.command)
			require.ErrorContains(t, err, tc.wantText)
			assert.Nil(t, data)
			assert.NotContains(t, err.Error(), "private-output", "stderr is discarded")
		})
	}
}

func TestRunDescribe_LargeDescription(t *testing.T) {
	setupRunner(t)
	const header = "version: v1\nchecks: [{id: ready, title: Ready}]\n"
	for _, size := range []int{(4 << 20) + 1, maxDescriptionBytes} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			body := fmt.Sprintf(
				"printf '%%s\\n' 'version: v1' 'checks: [{id: ready, title: Ready}]'\nprintf '%%*s' %d ''\n",
				size-len(header),
			)
			c, _ := fixtureCollector(t, body)
			data, err := runDescribe(context.Background(), c.definition.command)
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
	_, err := runDescribe(ctx, c.definition.command)
	require.ErrorIs(t, err, context.Canceled)
	requireNoFile(t, filepath.Join(dir, "started"))
}
