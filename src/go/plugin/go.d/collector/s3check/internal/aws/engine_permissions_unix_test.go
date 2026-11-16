// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package aws

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/journal"
)

func TestRestartLoadsOwnedStateBeforePermissionRepair(t *testing.T) {
	source, destination, _ := newAWSClients()
	root := t.TempDir()
	fingerprint := journal.Fingerprint("aws_replication", "", "source", "netdata-s3check/", "", "destination", "exact-key")
	j, err := journal.New(root, "agent", "job", fingerprint)
	require.NoError(t, err)
	t.Cleanup(j.Unlock)
	var opts Options
	engine := newAWSEngineWithJournal(t, source, destination, nil, j, func(options *Options) { opts = *options })
	require.NoError(t, engine.Collect(context.Background()).Err)
	require.NotEmpty(t, engine.state.Entries)
	j.Unlock()

	require.NoError(t, os.Chmod(root, 0300))
	t.Cleanup(func() { require.NoError(t, os.Chmod(root, 0700)) })
	fresh, err := journal.New(root, "agent", "job", fingerprint)
	require.NoError(t, err)
	t.Cleanup(fresh.Unlock)
	opts.Journal = fresh
	restarted, err := New(opts)
	require.NoError(t, err)
	require.NoError(t, restarted.Check(context.Background()))
	assert.Len(t, restarted.state.Entries, len(engine.state.Entries))
	assert.Equal(t, engine.state.ActiveKey, restarted.state.ActiveKey)
	info, err := os.Stat(root)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0300), info.Mode().Perm(), "candidate validation must remain read-only")

	putsBefore := source.Count("put")
	require.NoError(t, restarted.Collect(context.Background()).Err)
	info, err = os.Stat(root)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm(), "takeover repairs permissions before mutation")
	assert.Equal(t, engine.state.ActiveKey, restarted.state.ActiveKey)
	assert.Equal(t, putsBefore, source.Count("put"), "takeover must retain existing ownership")
}
