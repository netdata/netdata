//go:build unix

// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestInstalledManifestRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest")
	require.NoError(t, unix.Mkfifo(path, 0600))
	result := make(chan error, 1)
	go func() { _, err := LoadRuntimeManifest(path); result <- err }()
	select {
	case err := <-result:
		require.ErrorContains(t, err, "regular")
	case <-time.After(time.Second):
		// Unblock the old implementation before reporting its failure.
		writer, err := os.OpenFile(path, os.O_RDWR|unix.O_NONBLOCK, 0)
		require.NoError(t, err)
		<-result
		require.NoError(t, writer.Close())
		t.Fatal("manifest open blocked on a FIFO")
	}
}

func TestInstalledManifestFollowsRegularFileSymlink(t *testing.T) {
	manifest := trustedFixture()
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	dir := t.TempDir()
	target, link := filepath.Join(dir, "manifest"), filepath.Join(dir, "link")
	require.NoError(t, os.WriteFile(target, raw, 0600))
	require.NoError(t, os.Symlink(target, link))
	got, err := LoadRuntimeManifest(link)
	require.NoError(t, err)
	require.Equal(t, manifest, *got)
}
