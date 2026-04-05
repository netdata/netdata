// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

import (
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// `sv` always output "warning: " at beginning of each line on stderr.
const svStderrPrefix = "warning: "

type svCliExec struct {
	timeout time.Duration
	log     *logger.Logger
}

func (e *svCliExec) StatusAll(dir string) ([]byte, error) {
	bs, err := ndexec.RunNDSudo(e.log, e.timeout, "sv-status-all", "--serviceDir", dir)
	if err != nil {
		// Exit codes:
		// - None: means context deadline exceeded.
		// - 1-6 are used by `ndsudo`: means permanent fatal error.
		// - 0-99 are used by `sv`: means (partially) okay.
		// - 100 is used by `sv`: means temporary fatal error.
		// - 126, 127 is used by `sh`: means missing `sv` executable.
		// - 129-255: means killed by a signal.
		// - Other: means unknown error.
		// Stderr output:
		// - `ndsudo` never begins line with "warning: ".
		// - `sv status` always begins line with "warning: ".

		errMsg := err.Error()

		// sv partial/temporary errors include output we can use.
		if len(bs) > 0 && !strings.Contains(errMsg, svStderrPrefix) {
			return bs, nil
		}

		return nil, fmt.Errorf("sv-status-all error: %w", err)
	}

	return bs, nil
}
