// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/privilege"
)

// The install directories this build was configured for. The build sets them
// with -ldflags -X from the same CMake *_DIR values the C plugin gets through
// config.h, so they name what the daemon exports to us as NETDATA_*_DIR. They are
// what secureGetenv() falls back to; the defaults are a native install's layout.
var (
	netdataPluginsDir     = "/usr/libexec/netdata/plugins.d"
	netdataUserConfigDir  = "/etc/netdata"
	netdataStockConfigDir = "/usr/lib/netdata/conf.d"
)

// secureGetenv is os.Getenv() for a process that may be running with more
// privileges than the one that started it.
//
// ebpf-go.plugin is installed setuid root and executable by the netdata group, so
// when it is started by anything other than the daemon its environment comes from
// a caller with fewer privileges than it runs with. Every variable read through
// here names a directory the plugin then loads eBPF objects from or reads
// configuration out of as root, so while privileged the environment is ignored and
// the caller falls back to its compiled-in default - the same directory the daemon
// would have pointed us at, since both derive from the install prefix.
//
// This must answer the same question as nd_secure_getenv() on the C side, so that
// the two plugins obey one contract.
//
// fallback is the compiled-in directory used when the environment cannot be
// trusted. Ignoring the environment is the normal case, not an attack - the daemon
// drops to its own user before spawning us, so the exec is privileged on every
// start - and it costs nothing while the two agree. It is reported only when they
// disagree, which is when an operator's override is being dropped.
func secureGetenv(name, fallback string) string {
	value := os.Getenv(name)

	if !gainedPrivilegesAtExec() {
		if value != "" {
			return value
		}

		return fallback
	}

	// Report a dropped override only when it actually changes the directory we use,
	// and once per variable: the collectors resolve the same directories again and
	// again during start-up. The value itself is deliberately not printed: it is the
	// part a caller controls, and it would let them write arbitrary bytes, newlines
	// included, into our log.
	if value != "" && !sameDir(value, fallback) {
		if _, reported := reportedDroppedOverrides.LoadOrStore(name, true); reported {
			return fallback
		}

		fmt.Fprintf(os.Stderr,
			"ebpf-go.plugin: ignoring %s because we run with elevated privileges and it points elsewhere,"+
				" using %q; relocating a directory of a setuid plugin has to be done through the"+
				" installation prefix\n",
			name, fallback)
	}

	return fallback
}

// reportedDroppedOverrides holds the variables whose dropped override secureGetenv()
// has already reported.
var reportedDroppedOverrides sync.Map

// sameDir answers the question the message above needs: a trailing slash is the
// common way to write the same directory differently and must not count as a
// difference. Anything subtler, like a symlinked spelling, is not worth resolving:
// being wrong costs one log line.
func sameDir(a, b string) bool {
	return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
}

// Keep this plugin's existing boolean/error policy while sharing the detector.
func gainedPrivilegesAtExec() bool {
	gained, _ := privilege.GainedAtExec()
	return gained
}
