// SPDX-License-Identifier: GPL-3.0-or-later

package buildinfo

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInfoReleaseIdentity(t *testing.T) {
	originalVersion, originalChannel, originalRef := ReleaseVersion, ReleaseChannel, ReleaseRef
	t.Cleanup(func() { ReleaseVersion, ReleaseChannel, ReleaseRef = originalVersion, originalChannel, originalRef })
	for name, tc := range map[string]struct {
		version string
		channel string
		ref     string
		want    string
	}{
		"developer": {},
		"partial":   {version: "v2.12.0"},
		"stable": {version: "v2.12.0", channel: "stable", ref: strings.Repeat("a", 40),
			want: " release_version=v2.12.0 release_channel=stable release_ref=" + strings.Repeat("a", 40)},
		"nightly": {version: "v2.12.0-9-nightly", channel: "nightly", ref: strings.Repeat("b", 40),
			want: " release_version=v2.12.0-9-nightly release_channel=nightly release_ref=" + strings.Repeat("b", 40)},
	} {
		t.Run(name, func(t *testing.T) {
			ReleaseVersion, ReleaseChannel, ReleaseRef = tc.version, tc.channel, tc.ref
			info := Info()
			_, tuple, present := strings.Cut(info, " release_version=")
			got := ""
			if present {
				got = " release_version=" + tuple
			}
			assert.Equal(t, tc.want, got)
			assert.Contains(t, info, "version="+Version+" ")
		})
	}
}

func TestInfoStockDataDir(t *testing.T) {
	original := StockDataDir
	t.Cleanup(func() { StockDataDir = original })
	StockDataDir = "/custom/share/netdata"
	assert.Contains(t, Info(), " stock_data_dir=/custom/share/netdata ")
}

func TestRewriteWindowsPaths(t *testing.T) {
	originalUser, originalStock, originalData := UserConfigDir, StockConfigDir, StockDataDir
	originalPlugins, originalBin := PluginsDir, NetdataBinDir
	originalCache, originalLib, originalLog := CacheDir, VarLibDir, LogDir
	t.Cleanup(func() {
		UserConfigDir, StockConfigDir, StockDataDir = originalUser, originalStock, originalData
		PluginsDir, NetdataBinDir = originalPlugins, originalBin
		CacheDir, VarLibDir, LogDir = originalCache, originalLib, originalLog
	})

	for _, tc := range []struct {
		name string
		exec string
		data string
		want string
	}{
		{
			name: "installed default data root",
			exec: "C:/Program Files/Netdata/usr/libexec/netdata/plugins.d",
			data: "/usr/share/netdata",
			want: "C:/Program Files/Netdata/usr/share/netdata",
		},
		{
			name: "installed custom data root",
			exec: "C:/Netdata/usr/libexec/netdata/plugins.d",
			data: "/custom/data",
			want: "C:/Netdata/custom/data",
		},
		{
			name: "empty remains empty",
			exec: "C:/Netdata/usr/libexec/netdata/plugins.d",
		},
		{
			name: "development layout remains unchanged",
			exec: "C:/development/bin",
			data: "/usr/share/netdata",
			want: "/usr/share/netdata",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			PluginsDir = "/usr/libexec/netdata/plugins.d"
			StockDataDir = tc.data
			rewriteWindowsPaths(tc.exec)
			assert.Equal(t, tc.want, filepath.ToSlash(StockDataDir))
		})
	}
}
