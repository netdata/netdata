// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package native

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDescribeContainsDescendants(t *testing.T) {
	setupRunner(t)
	for _, phase := range []string{"leader exits", "caller cancels"} {
		t.Run(phase, func(t *testing.T) {
			body := `dir=$(dirname "$0")
(sleep 1; printf escaped > "$dir/escaped") &
printf started > "$dir/started"
printf '%s\n' 'version: v1' 'checks: [{id: ready, title: Ready}]'
`
			if phase == "caller cancels" {
				body += "wait\n"
			}
			c, dir := fixtureCollector(t, body)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, err := describePackage(ctx, c.definition.Command); done <- err }()
			waitFile(t, filepath.Join(dir, "started"))
			if phase == "caller cancels" {
				cancel()
			}
			select {
			case err := <-done:
				if phase == "caller cancels" {
					require.ErrorIs(t, err, context.Canceled)
				} else {
					require.NoError(t, err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("description process did not finish")
			}
			time.Sleep(1100 * time.Millisecond)
			_, err := os.Stat(filepath.Join(dir, "escaped"))
			assert.ErrorIs(t, err, os.ErrNotExist, "contained descendant must not outlive description")
		})
	}
}
