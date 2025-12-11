// SPDX-License-Identifier: GPL-3.0-or-later

package buildinfo

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

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

func init() {
	if runtime.GOOS != "windows" {
		return
	}

	execDir := executable.Directory
	if execDir == "" || PluginsDir == "" {
		return
	}

	// ----------------------------------------------------------------------------
	// 1. Detect install prefix on Windows
	//
	// We assume that on Windows the *running binary* lives inside PluginsDir.
	//
	// Example:
	//   execDir    = "C:/Program Files/Netdata/usr/libexec/netdata/plugins.d"
	//   PluginsDir = "/usr/libexec/netdata/plugins.d"
	//
	// By normalizing both paths to forward-slash format, we can test:
	//
	//   strings.HasSuffix(execDir, PluginsDir)  → true
	//
	// From that, we strip the suffix and recover the actual installation prefix:
	//
	//   prefix = "C:/Program Files/Netdata"
	//
	// If execDir does *not* end with PluginsDir, we simply do nothing — this keeps
	// development/testing environments safe where binaries are run outside the
	// expected layout.
	// ----------------------------------------------------------------------------
	normalized := filepath.ToSlash(execDir)
	suffix := filepath.ToSlash(PluginsDir)

	if !strings.HasSuffix(normalized, suffix) {
		return
	}

	// Extract the prefix by removing the suffix.
	//
	// Example:
	//   normalized = "C:/Program Files/Netdata/usr/libexec/netdata/plugins.d"
	//   suffix     = "/usr/libexec/netdata/plugins.d"
	//
	// → prefix = "C:/Program Files/Netdata"
	prefix := strings.TrimSuffix(normalized, suffix)
	prefix = strings.TrimSuffix(prefix, "/")

	installPrefix := filepath.FromSlash(prefix)

	// ----------------------------------------------------------------------------
	// 2. Rewrite all buildinfo paths as:
	//        <installPrefix> + <original buildinfo path as relative suffix>
	//
	// Example:
	//   NetdataBinDir build-time: "/usr/sbin"
	//   After rewrite:
	//       "C:\Program Files\Netdata\usr\sbin"
	//
	// Notes:
	//   • We must remove the *leading slash* from the build-time path, otherwise
	//     filepath.Join would treat it as absolute and ignore the prefix.
	//
	// Example of what we avoid:
	//     filepath.Join("C:\\Program Files\\Netdata", "/usr/sbin") →
	//         "\usr\sbin"   (WRONG — prefix lost!)
	//
	//   • Paths that were empty at build time should remain empty.
	// ----------------------------------------------------------------------------
	rebuild := func(p string) string {
		if p == "" {
			return ""
		}

		// Convert to slash form and trim the leading '/' so it becomes relative.
		//
		// Example:
		//   p  = "/usr/sbin"
		//   → s = "usr/sbin"
		//
		s := filepath.ToSlash(p)
		s = strings.TrimPrefix(s, "/")

		// Now prefix + relative suffix works reliably on Windows.
		//
		// Example:
		//   installPrefix = "C:\\Program Files\\Netdata"
		//   s = "usr/sbin"
		//   → result = "C:\\Program Files\\Netdata\\usr\\sbin"
		return filepath.Join(installPrefix, s)
	}

	UserConfigDir = rebuild(UserConfigDir)
	StockConfigDir = rebuild(StockConfigDir)
	PluginsDir = rebuild(PluginsDir)
	NetdataBinDir = rebuild(NetdataBinDir)
}
