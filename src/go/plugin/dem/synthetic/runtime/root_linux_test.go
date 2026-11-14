// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalRootMatchesPublishedStore(t *testing.T) {
	dir := t.TempDir()
	real, alias := filepath.Join(dir, "real"), filepath.Join(dir, "alias")
	require.NoError(t, os.Mkdir(real, 0700))
	require.NoError(t, os.Symlink(real, alias))
	path := filepath.Join(alias, "new", "runtime")
	expected, err := CanonicalRoot(path)
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(real, "new"))
	actual, root, err := openRuntimeRoot(path, managedRootLayout)
	require.NoError(t, err)
	require.NoError(t, root.Close())
	assert.Equal(t, expected, actual)
	after, err := CanonicalRoot(path)
	require.NoError(t, err)
	assert.Equal(t, expected, after)
}

func TestEnsureRuntimeRootRejectsNonDirectory(t *testing.T) {
	dir := t.TempDir()
	file, alias := filepath.Join(dir, "file"), filepath.Join(dir, "alias")
	require.NoError(t, os.WriteFile(file, []byte("preserved"), 0600))
	require.NoError(t, os.Symlink(file, alias))
	for _, layout := range []runtimeRootLayout{managedRootLayout, ownershipRootLayout} {
		for _, path := range []string{file, alias, filepath.Join(file, "child"), filepath.Join(alias, "child")} {
			canonical, err := ensureRuntimeRoot(path, layout)
			require.Error(t, err, "%s", path)
			assert.Empty(t, canonical)
		}
	}
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "preserved", string(data))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestOpenDoesNotRecreateLostSafetyStructure(t *testing.T) {
	for _, name := range []string{layoutName, "coordination.lock", "installation.lock", "claims", "uses", "staging", "initializing", "generations", "trash"} {
		t.Run(name, func(t *testing.T) {
			s, _ := testStore(t)
			installFixture(t, s, "installed")
			require.NoError(t, os.Rename(filepath.Join(s.path, name), filepath.Join(t.TempDir(), "lost")))
			other, err := Open(s.config)
			if other != nil {
				require.NoError(t, other.Close())
			}
			require.Error(t, err)
			_, err = os.Lstat(filepath.Join(s.path, name))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestConcurrentStoreBootstrapAndCanonicalRoot(t *testing.T) {
	for _, precreated := range []bool{false, true} {
		t.Run(fmt.Sprintf("precreated=%t", precreated), func(t *testing.T) {
			testConcurrentStoreBootstrapAndCanonicalRoot(t, precreated)
		})
	}
}

func testConcurrentStoreBootstrapAndCanonicalRoot(t *testing.T, precreated bool) {
	target := filepath.Join(t.TempDir(), "new", "parent", "store")
	if precreated {
		require.NoError(t, os.MkdirAll(target, 0700))
	}
	config := DefaultConfig(target)
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < cap(errs); i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			s, err := Open(config)
			if err == nil {
				err = s.Close()
			}
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(target, alias))
	s, err := Open(DefaultConfig(alias))
	require.NoError(t, err)
	require.Equal(t, target, s.path)
	require.NoError(t, s.Close())
	info, err := os.Lstat(alias)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink)
}

func TestStoreBootstrapResumesIncompleteLayout(t *testing.T) {
	for _, phase := range []string{"temporary", "record", "locks", "directories", "selection"} {
		t.Run(phase, func(t *testing.T) {
			path := t.TempDir()
			require.NoError(t, os.Chmod(path, 0700))
			root, err := os.OpenRoot(path)
			require.NoError(t, err)
			defer root.Close()
			if phase == "temporary" {
				id, err := randomID()
				require.NoError(t, err)
				// A caller may exit while writing the temporary before publication.
				require.NoError(t, writeNewFile(root, layoutTemporaryPrefix+id, []byte("{partial"), 0600))
			} else {
				require.NoError(t, createInitialLayout(root, storeLayout{
					Format: 1,
					Phase:  "initializing",
				}))
				if phase != "record" {
					for _, name := range storeLocks {
						require.NoError(t, ensureLock(root, name))
					}
				}
				if phase == "directories" || phase == "selection" {
					for _, name := range storeDirectories {
						require.NoError(t, root.Mkdir(name, 0700))
					}
				}
				if phase == "selection" {
					require.NoError(t, saveJSON(root, "selection.json", selection{}))
				}
			}
			s, err := Open(DefaultConfig(path))
			require.NoError(t, err)
			selected := installFixture(t, s, "resumed")
			require.NoError(t, s.Close())
			s, err = Open(DefaultConfig(path))
			require.NoError(t, err)
			defer s.Close()
			status, err := s.Inspect()
			require.NoError(t, err)
			require.Equal(t, selected, *status.Selected, "reopening ready must not reset selection")
		})
	}
}

func TestBootstrapExitBeforeParentSyncProcess(t *testing.T) {
	path := os.Getenv("DEM_BOOTSTRAP_EXIT_STORE")
	if path == "" {
		return
	}
	_, err := ensureRuntimeRoot(path, managedRootLayout)
	require.NoError(t, err)
	os.Exit(helperCrashExit)
}

func TestOpenReconcilesPublishedBootstrapAfterExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "nested", "store")
	output, status := runHelper(t, "TestBootstrapExitBeforeParentSyncProcess", "DEM_BOOTSTRAP_EXIT_STORE="+path)
	require.Equal(t, helperCrashExit, status, string(output))
	before, err := os.Stat(path)
	require.NoError(t, err)
	s, err := Open(DefaultConfig(path))
	require.NoError(t, err)
	require.NoError(t, s.Close())
	after, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, os.SameFile(before, after))
}

func TestBootstrapPrecreatedRootNeedsNoParentWriteAccess(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	parent := t.TempDir()
	path := filepath.Join(parent, "cache")
	require.NoError(t, os.Mkdir(path, 0700))
	require.NoError(t, os.Chmod(parent, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(parent, 0700)) })
	s, err := Open(DefaultConfig(path))
	require.NoError(t, err)
	require.NoError(t, s.Close())
}

func TestOpenRequiresOnlyTraversalOfExistingAncestors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	parent := t.TempDir()
	path := filepath.Join(parent, "private", "cache")
	s, err := Open(DefaultConfig(path))
	require.NoError(t, err)
	require.NoError(t, s.Close())
	require.NoError(t, os.Chmod(parent, 0100))
	t.Cleanup(func() { require.NoError(t, os.Chmod(parent, 0700)) })
	s, err = Open(DefaultConfig(path))
	require.NoError(t, err)
	require.NoError(t, s.Close())
}

func TestPrecreatedMountedStoreRoot(t *testing.T) {
	path := os.Getenv("DEM_RUNTIME_TEST_PRECREATED_ROOT")
	if path == "" {
		t.Skip("requires an isolated private mountpoint")
	}
	s, err := Open(DefaultConfig(path))
	require.NoError(t, err)
	m := installFixture(t, s, "mounted")
	require.NoError(t, s.Close())
	s, err = Open(DefaultConfig(path))
	require.NoError(t, err)
	defer s.Close()
	candidate, err := s.beginFixture(context.Background(), m, RepairRelease)
	require.NoError(t, err)
	require.NoError(t, candidate.Close())
}

func TestSharedCreatedAncestorRetainsPublicationBarrier(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	parent := t.TempDir()
	first := filepath.Join(parent, "new", "nested", "first")
	// The first initializer exits after publication, before syncing parent.
	_, err := ensureRuntimeRoot(first, managedRootLayout)
	require.NoError(t, err)
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(filepath.Dir(first), alias))
	second := filepath.Join(alias, "second")
	// Make the unresolved parent barrier fail while leaving traversal available.
	require.NoError(t, os.Chmod(parent, 0100))
	t.Cleanup(func() { require.NoError(t, os.Chmod(parent, 0700)) })
	s, err := Open(DefaultConfig(second))
	if s != nil {
		require.NoError(t, s.Close())
	}
	require.ErrorIs(
		t,
		err,
		os.ErrPermission,
		"a different store must inherit the created ancestor's publication barrier",
	)
	require.NoError(t, os.Chmod(parent, 0700))
	s, err = Open(DefaultConfig(second))
	require.NoError(t, err)
	require.NoError(t, s.Close())
	s, err = Open(DefaultConfig(first))
	require.NoError(t, err)
	require.NoError(t, s.Close())
	// Completed setup leaves no stale obligation to reopen the old ancestor.
	require.NoError(t, os.Chmod(parent, 0100))
	s, err = Open(DefaultConfig(filepath.Join(alias, "third")))
	require.NoError(t, err)
	require.NoError(t, s.Close())
}

func TestConcurrentStoresShareCreatedAncestors(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "new", "nested")
	first := filepath.Join(parent, "first")
	_, err := ensureRuntimeRoot(first, managedRootLayout)
	require.NoError(t, err)
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for i := range cap(errs) {
		group.Add(1)
		go func() {
			defer group.Done()
			s, err := Open(DefaultConfig(filepath.Join(parent, fmt.Sprintf("store-%d", i))))
			if err == nil {
				err = s.Close()
			}
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	for _, path := range []string{parent, filepath.Dir(parent)} {
		require.NoFileExists(t, filepath.Join(path, publicationName))
	}
	s, err := Open(DefaultConfig(first))
	require.NoError(t, err)
	require.NoError(t, s.Close())
}
