// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
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

// gainedPrivilegesAtExec reports whether this process is more privileged than the
// one that exec'd it - what the kernel records as AT_SECURE and libc answers from
// secure_getenv(). Go has neither, and AT_SECURE can only be read from
// /proc/self/auxv, which is mode 0400 and owned by root once gaining privileges at
// exec clears dumpable. Reading our own is not blocked by the ptrace check, so a
// setuid-root process could do it on euid 0 alone - but one carrying only file
// capabilities cannot, and that is a case this has to answer. So the kernel's own
// conditions are re-evaluated instead, from /proc/self/status, which stays world
// readable either way.
func gainedPrivilegesAtExec() bool {
	// a setuid or setgid binary
	if os.Geteuid() != os.Getuid() || os.Getegid() != os.Getgid() {
		return true
	}

	// A root process is not a privilege boundary with its caller, and the kernel
	// likewise only weighs capabilities for a non-root exec.
	if os.Geteuid() == 0 {
		return false
	}

	// A file capability granted to this executable. /proc/self/status is world
	// readable even for a privileged process, unlike /proc/self/auxv, so an
	// unreadable one means we are not on a Linux system that grants capabilities
	// at all and there is nothing here to protect against.
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}

	return gainedCapabilitiesAtExec(status)
}

// gainedCapabilitiesAtExec answers the capability half of the kernel's AT_SECURE
// decision for a non-root exec (cap_bprm_creds_from_file()), which has two
// independent triggers.
//
// First: the permitted set grew past the ambient set the caller passed down. Read
// permitted, not effective - a file marked "cap_bpf=p" lands in permitted only,
// leaving CapEff empty, and the kernel still marks that exec secure, so judging by
// CapEff alone would trust an environment the C side rejects.
//
// Second: the file asked for its capabilities to be effective at exec. The kernel
// counts that on its own, even when those capabilities add nothing to what the
// caller already had. That bit is not recoverable from /proc/self/status, because
// CapEff == CapPrm looks identical whether the file set it or the caller's ambient
// set did - so any effective capability held by a non-root process is treated as a
// gain. The single cell where this is stricter than the kernel is a caller that
// passed ambient capabilities down to us; there this errs toward the compiled-in
// directories, and netdata does not spawn plugins that way.
func gainedCapabilitiesAtExec(status []byte) bool {
	permitted, found := capabilitySet(status, "CapPrm:")
	if !found {
		return false
	}

	// Kernels older than 4.3 have no ambient set; there, any permitted capability
	// a non-root process holds came from the executable.
	ambient, _ := capabilitySet(status, "CapAmb:")
	if permitted&^ambient != 0 {
		return true
	}

	effective, _ := capabilitySet(status, "CapEff:")

	return effective != 0
}

// capabilitySet reads one hex capability mask out of /proc/self/status.
func capabilitySet(status []byte, field string) (mask uint64, found bool) {
	for line := range strings.SplitSeq(string(status), "\n") {
		value, ok := strings.CutPrefix(line, field)
		if !ok {
			continue
		}

		mask, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
		if err != nil {
			return 0, false
		}

		return mask, true
	}

	return 0, false
}
