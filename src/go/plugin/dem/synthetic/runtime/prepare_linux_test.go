// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestImportRejectsFIFOWithoutBlocking(t *testing.T) {
	s, _ := testStore(t)
	m, _ := bundleFixture(t, "fifo")
	path := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, unix.Mkfifo(path, 0600))
	result := make(chan error, 1)
	go func() { result <- s.importUnselected(context.Background(), m, path) }()
	select {
	case err := <-result:
		require.ErrorContains(t, err, "regular")
	case <-time.After(time.Second):
		writer, err := os.OpenFile(path, os.O_RDWR|unix.O_NONBLOCK, 0)
		require.NoError(t, err)
		<-result
		require.NoError(t, writer.Close())
		t.Fatal("archive open blocked on a FIFO")
	}
}

func TestImportFollowsRegularArchiveSymlink(t *testing.T) {
	s, _ := testStore(t)
	m, archive := bundleFixture(t, "symlink")
	dir := t.TempDir()
	target, link := filepath.Join(dir, "archive"), filepath.Join(dir, "link")
	require.NoError(t, os.WriteFile(target, archive, 0600))
	require.NoError(t, os.Symlink(target, link))
	require.NoError(t, s.importUnselected(context.Background(), m, link))
	requirePrepared(t, s, m)
}

func TestPublicationReportsGenerationReadFailure(t *testing.T) {
	s, _ := testStore(t)
	m, archive := bundleFixture(t, "publication")
	err := s.prepare(context.Background(), m, func() (io.ReadCloser, error) {
		name := filepath.Join(s.path, "generations", m.Archive.SHA256)
		if err := os.Mkdir(name, 0700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(name, "prepared.json"), []byte("broken"), 0600); err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(archive)), nil
	})
	require.ErrorContains(t, err, "invalid character")
	require.NotContains(t, err.Error(), "appeared during preparation")
}
