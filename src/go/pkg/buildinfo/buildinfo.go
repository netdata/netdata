// SPDX-License-Identifier: GPL-3.0-or-later

package buildinfo

// Version stores the agent's version number. It's set during the build process using build flags.
var Version = "v0.0.0"

// UserConfigDir stores the path to the user configuration directory.
// This value is set during the build process using build flags.
var UserConfigDir = ""

// StockConfigDir stores the path to the stock (default) configuration directory.
// This value is set during the build process using build flags.
var StockConfigDir = ""
