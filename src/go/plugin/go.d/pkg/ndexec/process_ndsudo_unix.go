// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package ndexec

// DenyNDSudoSignalsForTests makes processes that StartNDSudoProcess starts afterwards
// behave as root commands of an unprivileged caller: termination sends no signal, so a
// same-user test command ends only by exiting or by SIGPIPE. It returns a function that
// restores signaling. It sets process-wide state, so tests using it MUST NOT run in
// parallel. It is Unix-only: Windows termination also releases the Job Object.
func DenyNDSudoSignalsForTests() func() {
	previous := ndsudoSignalsDenied
	ndsudoSignalsDenied = true
	return func() { ndsudoSignalsDenied = previous }
}
