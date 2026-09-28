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

func TestCancellationContainsDescendant(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(
		t,
		"dir=$(dirname \"$0\")\n(sleep 1; printf escaped > \"$dir/escaped\") &\nprintf started > \"$dir/started\"\nwait\n",
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := runCommand(ctx, 5*time.Second, c.definition.Command); done <- err }()
	require.Eventually(
		t,
		func() bool { _, err := os.Stat(filepath.Join(dir, "started")); return err == nil },
		2*time.Second,
		10*time.Millisecond,
	)
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("command did not stop")
	}
	// The descendant has passed its scheduled write time. A surviving child would
	// create this marker even after the parent and command's pipes were closed.
	time.Sleep(1100 * time.Millisecond)
	_, err := os.Stat(filepath.Join(dir, "escaped"))
	assert.True(t, os.IsNotExist(err), "descendant survived cancellation")
}
