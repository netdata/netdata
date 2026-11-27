// SPDX-License-Identifier: GPL-3.0-or-later

package windows

// terminalServicesCounters are the Terminal Services counter set's point values,
// which the native Windows Agent charts as the host's Remote Desktop sessions:
// sessions with a connected user and all others, such as disconnected
// sessions, the services session and a console nobody is logged on to.
var terminalServicesCounters = [...]rawCounter{
	{property: "ActiveSessions", metric: "terminal_services_active_sessions"},
	{property: "InactiveSessions", metric: "terminal_services_inactive_sessions"},
}

// The Local Session Manager provides the Terminal Services counter set, so it
// exists without the Remote Desktop Services role.
var queryTerminalServices = counterQuery(
	"Win32_PerfRawData_LocalSessionManager_TerminalServices",
	terminalServicesCounters[:],
)

func (o *observations) readTerminalServices(rows []map[string]any) bool {
	if len(rows) != 1 {
		return false
	}
	return readCounters(rows[0], terminalServicesCounters[:], o.terminalServices[:])
}
