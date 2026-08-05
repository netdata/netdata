// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

func defaultProfileCatalogPaths() catalog.Paths {
	if dir := filepath.Join(executable.Directory, "../config/go.d/snmp.trap-profiles/default"); profilecatalog.DirExists(dir) {
		return catalog.Paths{StockDir: dir}
	}

	var userDirs []string
	for _, dir := range pluginconfig.CollectorsUserDirs() {
		userDirs = append(userDirs, filepath.Join(dir, "snmp.trap-profiles"))
	}
	return catalog.Paths{
		UserDirs: userDirs,
		StockDir: filepath.Join(pluginconfig.CollectorsStockDir(), "snmp.trap-profiles", "default"),
	}
}
