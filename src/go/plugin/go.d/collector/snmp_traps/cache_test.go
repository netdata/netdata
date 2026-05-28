// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"os"
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
	buildinfo.CacheDir = dir
	t.Cleanup(func() {
		buildinfo.CacheDir = oldCacheDir
		_ = os.RemoveAll(dir)
	})

	return dir
}
