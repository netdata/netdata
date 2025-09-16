// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type postqueueBinary interface {
	list() ([]byte, error)
}

func newPostqueueExec(binPath string, timeout time.Duration) *postqueueExec {
	return &postqueueExec{
		binPath: binPath,
		timeout: timeout,
	}
}

type postqueueExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (p *postqueueExec) list() ([]byte, error) {
	return ndexec.RunUnprivileged(p.Logger, p.timeout, p.binPath, "-p")
}
