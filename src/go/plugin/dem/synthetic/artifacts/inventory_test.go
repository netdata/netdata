// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fail one real directory enumeration without replacing publication, manifest
// reads, retention selection or deletion. Metadata can survive a filesystem I/O error.
type failedReadDirFS struct {
	fs.FS
	directory string
}

func (f failedReadDirFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == f.directory {
		return nil, &os.PathError{
			Op:   "readdir",
			Path: name,
			Err:  syscall.EIO,
		}
	}
	return fs.ReadDir(f.FS, name)
}

func TestUnreadableOwnedSetDefersRetentionUntilInventoryRecovers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s, err := Open(t.TempDir())
		require.NoError(t, err)
		defer s.Close()
		publish := func(job string, size int) Set {
			var raw [16]byte
			_, err := rand.Read(raw[:])
			require.NoError(t, err)
			id := hex.EncodeToString(raw[:])
			work, err := s.Begin(id, job)
			require.NoError(t, err)
			require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
			require.NoError(
				t,
				os.WriteFile(filepath.Join(work, "output", "report"), []byte(strings.Repeat("x", size)), 0600),
			)
			_, err = s.Finalize(
				ctx,
				id,
				[]synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report"}},
			)
			require.NoError(t, err)
			page, err := s.Catalogue(ctx, Filter{
				Limit: 1,
			})
			require.NoError(t, err)
			require.Len(t, page.Sets, 1)
			require.Equal(t, id, page.Sets[0].RunID)
			return page.Sets[0]
		}
		quiet := publish("lighthouse:quiet", 64)
		time.Sleep(time.Second)
		old := publish("lighthouse:busy", 256)
		time.Sleep(time.Second)
		latest := publish("lighthouse:busy", 64)
		budget := quiet.Bytes + latest.Bytes
		require.Less(t, old.Bytes, budget)
		require.Greater(t, quiet.Bytes+old.Bytes, budget)
		realFS := s.inventoryFS
		s.inventoryFS = failedReadDirFS{
			FS:        realFS,
			directory: "runs/" + latest.RunID,
		}
		_, _, err = s.Fetch(ctx, latest.JobID, latest.RunID, "report")
		require.NoError(t, err, "directory enumeration failure does not imply lost ownership or payload")
		status, err := s.Enforce(ctx, 7, budget)
		require.Error(t, err)
		assert.Nil(t, status.Inventory.Retained.Bytes)
		assert.Contains(t, status.Inventory.Retained.Error, syscall.EIO.Error())
		for _, set := range []Set{quiet, old, latest} {
			_, err = s.Manifest(ctx, set.JobID, set.RunID)
			assert.NoError(t, err, "incomplete selection must not evict %s", set.JobID)
		}
		require.NotNil(t, status.Inventory.Unclassified.Bytes)
		assert.Zero(t, *status.Inventory.Unclassified.Bytes, "valid ownership does not become unclassified")
		page, err := s.Catalogue(ctx, Filter{
			Limit: 10,
		})
		require.NoError(t, err)
		assert.Nil(t, page.Status.Inventory.Retained.Bytes)
		assert.Contains(t, page.Status.Inventory.Retained.Error, syscall.EIO.Error())
		s.inventoryFS = realFS
		status, err = s.Enforce(ctx, 7, budget)
		require.NoError(t, err)
		require.NotNil(t, status.Inventory.Retained.Bytes)
		assert.Equal(t, budget, *status.Inventory.Retained.Bytes)
		assert.Empty(t, status.Cleanup.Error)
		_, err = s.Manifest(ctx, old.JobID, old.RunID)
		assert.ErrorIs(t, err, ErrNotFound)
		for _, set := range []Set{quiet, latest} {
			_, _, err = s.Fetch(ctx, set.JobID, set.RunID, "report")
			require.NoError(t, err)
		}
	})
}
