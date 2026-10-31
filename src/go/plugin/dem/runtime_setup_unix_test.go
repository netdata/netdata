//go:build unix

// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestInstalledReleaseRejectsFIFOWithoutBlocking(t *testing.T) {
	cfg, _, _ := signedSetupFixture(t)
	path := filepath.Join(t.TempDir(), "release")
	require.NoError(t, unix.Mkfifo(path, 0600))
	result := make(chan error, 1)
	go func() { _, err := cfg.ReleaseLoader(path, false)(context.Background()); result <- err }()
	select {
	case err := <-result:
		require.ErrorContains(t, err, "regular")
	case <-time.After(time.Second):
		// Unblock the old implementation before reporting its failure.
		writer, err := os.OpenFile(path, os.O_RDWR|unix.O_NONBLOCK, 0)
		require.NoError(t, err)
		<-result
		require.NoError(t, writer.Close())
		t.Fatal("release open blocked on a FIFO")
	}
}

func TestInstalledReleaseFollowsRegularFileSymlink(t *testing.T) {
	cfg, release, raw := signedSetupFixture(t)
	dir := t.TempDir()
	target, link := filepath.Join(dir, "release"), filepath.Join(dir, "link")
	require.NoError(t, os.WriteFile(target, raw, 0600))
	require.NoError(t, os.Symlink(target, link))
	got, err := cfg.ReleaseLoader(link, false)(context.Background())
	require.NoError(t, err)
	require.Equal(t, release, got)
}

func TestReleaseKeyRejectsFIFOAndAcceptsRegularSymlink(t *testing.T) {
	cfg, _, _ := signedSetupFixture(t)
	link := filepath.Join(t.TempDir(), "key-link")
	require.NoError(t, os.Symlink(cfg.OfflineKeyring, link))
	key, err := runtime.ReadReleaseKeyring(link)
	require.NoError(t, err)
	require.NotNil(t, key)
	fifo := filepath.Join(t.TempDir(), "key-fifo")
	require.NoError(t, unix.Mkfifo(fifo, 0600))
	result := make(chan error, 1)
	go func() { _, err := runtime.ReadReleaseKeyring(fifo); result <- err }()
	select {
	case err := <-result:
		require.ErrorContains(t, err, "regular")
	case <-time.After(time.Second):
		writer, err := os.OpenFile(fifo, os.O_RDWR|unix.O_NONBLOCK, 0)
		require.NoError(t, err)
		<-result
		require.NoError(t, writer.Close())
		t.Fatal("verification key open blocked on a FIFO")
	}
}
