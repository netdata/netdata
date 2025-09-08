// SPDX-License-Identifier: GPL-3.0-or-later

package buildinfo

// The variables in this file are set during the build process using linker flags.

// Version stores the agent's version number.
var Version = "v0.0.0"

// UserConfigDir stores the path to the user configuration directory.
var UserConfigDir = ""

// StockConfigDir stores the path to the stock (default) configuration directory.
var StockConfigDir = ""

// NetdataBinDir stores the directory where executables were installed at build time.
var NetdataBinDir = "/usr/sbin"
