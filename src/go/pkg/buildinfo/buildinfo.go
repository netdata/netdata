// SPDX-License-Identifier: GPL-3.0-or-later

package buildinfo

import "fmt"

// The following variables are set at build time using linker flags.

// Version is the Netdata Agent version.
var Version = "v0.0.0"

// UserConfigDir is the path to the user configuration directory.
var UserConfigDir = ""

// StockConfigDir is the path to the stock (default) configuration directory.
var StockConfigDir = ""

// PluginsDir is the path to the installed plugins directory.
var PluginsDir = "/usr/libexec/netdata/plugins.d"

// NetdataBinDir is the path to the installed executables directory.
var NetdataBinDir = "/usr/sbin"

// Info returns all build information as a single line with snake_case keys.
func Info() string {
	return fmt.Sprintf(
		"version=%s user_config_dir=%s stock_config_dir=%s plugins_dir=%s netdata_bin_dir=%s",
		Version,
		UserConfigDir,
		StockConfigDir,
		PluginsDir,
		NetdataBinDir,
	)
}
