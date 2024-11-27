// SPDX-License-Identifier: GPL-3.0-or-later

package postfix

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
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
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.binPath, "-p")
	p.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
