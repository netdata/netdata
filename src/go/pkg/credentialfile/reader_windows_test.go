// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package credentialfile

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/require"
)

func TestReaderLocalPolicies(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "credential")
	require.NoError(t, os.WriteFile(p, []byte("first"), 0600))
	got, err := Read(ctx, p)
	require.NoError(t, err)
	require.Equal(t, "first", string(got))
	require.NoError(t, os.WriteFile(p, []byte("rotated"), 0600))
	got, err = Read(ctx, p)
	require.NoError(t, err)
	require.Equal(t, "rotated", string(got))
	info, err := os.Stat(p)
	require.NoError(t, err)
	mt, err := Stat(ctx, p)
	require.NoError(t, err)
	require.True(t, mt.Equal(info.ModTime()))
	require.NoError(t, os.WriteFile(p, make([]byte, safefile.MaxSize+1), 0600))
	_, err = Read(ctx, p)
	require.ErrorIs(t, err, safefile.ErrTooLarge)
	got, err = ReadAll(ctx, p)
	require.NoError(t, err)
	require.Len(t, got, int(safefile.MaxSize+1))
	stream, err := Open(ctx, p)
	require.NoError(t, err)
	got, err = io.ReadAll(stream)
	require.NoError(t, err)
	require.NoError(t, stream.Close())
	require.Len(t, got, int(safefile.MaxSize+1))
	_, err = Read(ctx, filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = Read(canceled, p)
	require.ErrorIs(t, err, context.Canceled)
}
