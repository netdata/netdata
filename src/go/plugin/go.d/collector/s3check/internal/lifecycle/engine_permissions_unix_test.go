// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package lifecycle

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstructorLoadsAbsentStateBeforePermissionRepair(t *testing.T) {
	root := t.TempDir()
	j := newTestJournal(t, root)
	t.Cleanup(j.Unlock)
	client, _ := newLifecycleClient()
	require.NoError(t, os.Chmod(root, 0100))
	t.Cleanup(func() { require.NoError(t, os.Chmod(root, 0700)) })
	engine, err := New(Options{
		Client:         client,
		Bucket:         "bucket",
		Journal:        j,
		Generator:      newGenerator(j.OwnerID()),
		RequestTimeout: time.Second,
		UpdateEvery:    time.Minute,
	})
	require.NoError(t, err)
	require.NoError(t, engine.Check(context.Background()))
	info, err := os.Stat(root)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0100), info.Mode().Perm(), "candidate validation must remain read-only")
	_, err = os.Stat(j.Path())
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Zero(t, client.Count("put"))

	require.NoError(t, engine.Collect(context.Background()).Err)
	info, err = os.Stat(root)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm(), "active ownership repairs permissions")
	assert.Positive(t, client.Count("put"))
}
