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
	oldLogDir := buildinfo.LogDir
	buildinfo.CacheDir = dir
	logDir := filepath.Join(dir, "log")
	buildinfo.LogDir = logDir
	t.Setenv(netdataLogDirEnv, logDir)
	if err := os.MkdirAll(logDir, 0750); err != nil {
		t.Fatalf("create test Netdata log dir: %v", err)
	}
	t.Cleanup(func() {
		buildinfo.CacheDir = oldCacheDir
		buildinfo.LogDir = oldLogDir
		_ = os.RemoveAll(dir)
	})

	return dir
}

func withNetdataLogDir(t testing.TB, root string) {
	t.Helper()

	oldLogDir := buildinfo.LogDir
	buildinfo.LogDir = root
	t.Setenv(netdataLogDirEnv, root)
	t.Cleanup(func() {
		buildinfo.LogDir = oldLogDir
	})
}
