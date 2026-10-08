// SPDX-License-Identifier: GPL-3.0-or-later

package httpsd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/stretchr/testify/require"
)

func useCredentialHelper(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses service-account file reads without a helper")
	}
	helper := os.Getenv("NETDATA_TEST_ND_RUN")
	if helper == "" {
		t.Skip("set NETDATA_TEST_ND_RUN to a prebuilt nd-run for credential-file integration tests")
	}
	helper, err := filepath.Abs(helper)
	require.NoError(t, err)
	dir := credentialTestDir(t)
	require.NoError(t, os.Symlink(helper, filepath.Join(dir, "nd-run")))
	old := buildinfo.NetdataBinDir
	buildinfo.NetdataBinDir = dir
	t.Cleanup(func() { buildinfo.NetdataBinDir = old })
}

func TestCredentialReadFailsClosedWithoutHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses service-account file reads without a helper")
	}
	old := buildinfo.NetdataBinDir
	buildinfo.NetdataBinDir = t.TempDir()
	t.Cleanup(func() { buildinfo.NetdataBinDir = old })
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("private-token-sentinel"), 0600))
	d, err := NewDiscoverer(Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{
		URL: "http://example.invalid", BearerTokenFile: path,
	}}})
	require.NoError(t, err)
	err = d.Test(t.Context())
	require.ErrorIs(t, err, safefile.ErrFile)
	require.NotContains(t, err.Error(), "private-token-sentinel")
}

func credentialTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "netdata-httpsd-credential-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	require.NoError(t, os.Chmod(dir, 0755))
	return dir
}
