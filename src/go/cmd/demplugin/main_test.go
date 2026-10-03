// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebugHistoryLeavesLiveDirectoryUntouched(t *testing.T) {
	dir := t.TempDir()
	path, err := historyPath(dir, true)
	require.NoError(t, err)
	history, err := store.Open(context.Background(), path)
	require.NoError(t, err)
	require.NoError(t, history.Close())
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}
func TestDaemonRejectsMissingStateDirectory(t *testing.T) {
	_, err := historyPath("", false)
	assert.Error(t, err)
}
