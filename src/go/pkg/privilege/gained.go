// SPDX-License-Identifier: GPL-3.0-or-later

// Package privilege identifies privilege transitions for configuration selection.
package privilege

import "os"

// GainedAtExec reports setuid/setgid transitions and Linux capability gains.
// Direct root execution is not a transition. Callers must handle probe errors
// before trusting caller-selected configuration. The boolean retains the legacy
// eBPF interpretation even on malformed capability data.
func GainedAtExec() (bool, error) {
	return gainedAtExec(os.Getuid(), os.Geteuid(), os.Getgid(), os.Getegid(), capabilitiesAtExec)
}

func gainedAtExec(uid, euid, gid, egid int, capabilities func() (bool, error)) (bool, error) {
	if uid != euid || gid != egid {
		return true, nil
	}
	if euid == 0 {
		return false, nil
	}
	return capabilities()
}
