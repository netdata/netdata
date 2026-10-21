// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type svCliExec struct {
	timeout time.Duration
	log     *logger.Logger
}

// StatusAll runs sv-status-all: ndsudo execs `sh -c`, which execs `sv status` for every service.
//
// Exit codes, for reference (codes of different programs overlap, so only stdout tells them apart):
//   - 0: sv reported all services.
//   - 1-7 from ndsudo: it did not run the command
//     (1 wrong argument count, 2 invalid parameters, 3 unknown command, 4 `sh` not in PATH,
//     5 missing parameters, 6 execve failure, 7 no root privileges).
//   - 2 from sh: invalid option or syntax.
//   - 126, 127 from sh: `sv` is not executable or not found.
//   - 1-99 from sv: the number of services it reported as failed, capped at 99.
//   - 100 from sv: fatal error (usage, unreadable current directory, failed return to it).
//   - -1 (no exit code): killed by a signal; there is no parent shell to turn it into 128+N.
//   - None, with context.DeadlineExceeded: the timeout expired and the command was killed.
//
// Stdout:
//   - ndsudo and sh never write to it for this command.
//   - sv writes a status line per service,
//     or a `fail:` or `warning:` line for a service it could not report.
//
// Stderr:
//   - ndsudo and sh write their errors to it, never starting with "warning: ".
//   - sv writes `fatal:` and usage errors to it,
//     and "warning: unable to stat .../down" without counting it in the exit code;
//     the status line of that service in stdout then lacks its status (see servicesCli).
func (e *svCliExec) StatusAll(dir string) ([]byte, error) {
	bs, err := ndexec.RunNDSudo(e.log, e.timeout, "sv-status-all", "--serviceDir", dir)
	// sv exits non-zero when it fails to report some services but still reports the others,
	// and output cut by a timeout or a signal still holds valid statuses.
	// Errors of ndsudo and the shell go to stderr only, so empty stdout has nothing to use.
	if err != nil && len(bs) == 0 {
		return nil, fmt.Errorf("sv-status-all error: %w", err)
	}

	return bs, nil
}
