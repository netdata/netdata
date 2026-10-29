// SPDX-License-Identifier: GPL-3.0-or-later

package filelock

import (
	"os"
	"os/exec"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockerRejectsNonRegular(t *testing.T) {
	l := New(t.TempDir())
	require.NoError(t, os.Mkdir(l.filename("name"), 0700))
	ok, err := l.Lock("name")
	assert.False(t, ok)
	assert.ErrorIs(t, err, safefile.ErrNotRegular)
	assert.False(t, l.isLocked("name"))
}

func TestLockerCrossProcess(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	t.Cleanup(l.UnlockAll)
	ok, err := l.Lock("name")
	require.NoError(t, err)
	require.True(t, ok)
	check := func(want string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestLockerSubprocess$", "-test.count=1")
		cmd.Env = append(os.Environ(), "NETDATA_FILELOCK_TEST_DIR="+dir, "NETDATA_FILELOCK_TEST_ACQUIRED="+want)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	check("false")
	l.Unlock("name")
	check("true")
	// Closing the child lock also leaves a reusable regular lock file.
	ok, err = l.Lock("name")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestLockerSubprocess(t *testing.T) {
	dir := os.Getenv("NETDATA_FILELOCK_TEST_DIR")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	l := New(dir)
	defer l.UnlockAll()
	ok, err := l.Lock("name")
	require.NoError(t, err)
	assert.Equal(t, os.Getenv("NETDATA_FILELOCK_TEST_ACQUIRED") == "true", ok)
}
