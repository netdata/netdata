// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"os"
	"path/filepath"
	"testing"

	buildinfo "github.com/netdata/netdata/go/plugins/pkg/buildinfo"
)

func withTestCacheDir(t testing.TB) string {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "snmp-traps-test-cache-*")
	if err != nil {
		t.Fatalf("create test cache dir: %v", err)
	}

	oldCacheDir := buildinfo.CacheDir
	oldJournalRoot := persistentSystemdJournalRoot
	buildinfo.CacheDir = dir
	persistentSystemdJournalRoot = filepath.Join(dir, "journal")
	if err := os.MkdirAll(persistentSystemdJournalRoot, 0750); err != nil {
		t.Fatalf("create test persistent journal root: %v", err)
	}
	t.Cleanup(func() {
		buildinfo.CacheDir = oldCacheDir
		persistentSystemdJournalRoot = oldJournalRoot
		_ = os.RemoveAll(dir)
	})

	return dir
}

func withPersistentJournalRoot(t testing.TB, root string) {
	t.Helper()

	oldJournalRoot := persistentSystemdJournalRoot
	persistentSystemdJournalRoot = root
	t.Cleanup(func() {
		persistentSystemdJournalRoot = oldJournalRoot
	})
}
