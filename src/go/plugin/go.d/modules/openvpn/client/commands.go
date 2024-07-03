// SPDX-License-Identifier: GPL-3.0-or-later

package client

/*
https://openvpn.net/community-resources/management-interface/

OUTPUT FORMAT
-------------

(1) Command success/failure indicated by "SUCCESS: [text]" or
    "ERROR: [text]".

(2) For commands which print multiple lines of output,
    the last line will be "END".

(3) Real-time messages will be in the form ">[source]:[text]",
    where source is "CLIENT", "ECHO", "FATAL", "HOLD", "INFO", "LOG",
    "NEED-OK", "PASSWORD", or "STATE".
*/

var (
	// Close the management session, and resume listening on the
	// management port for connections from other clients. Currently,
	// the OpenVPN daemon can at most support a single management client
	// any one time.
	commandExit = "exit\n"

	// Show current daemon status information, in the same format as
	// that produced by the OpenVPN --status directive.
	commandStatus3 = "status 3\n"

	// no description in docs ¯\(°_o)/¯
	commandLoadStats = "load-stats\n"

	// Show the current OpenVPN and Management Interface versions.
	commandVersion = "version\n"
)
