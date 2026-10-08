// SPDX-License-Identifier: GPL-3.0-or-later

package buildinfo

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
