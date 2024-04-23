// SPDX-License-Identifier: GPL-3.0-or-later

package fail2ban

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/logger"
)

var errJailNotExist = errors.New("jail not exist")

func newFail2BanClientCliExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *fail2banClientCliExec {
	return &fail2banClientCliExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type fail2banClientCliExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *fail2banClientCliExec) status() ([]byte, error) {
	return e.execute("fail2ban-client-status")
}

func (e *fail2banClientCliExec) jailStatus(jail string) ([]byte, error) {
	return e.execute("fail2ban-client-status-jail", "--jail", jail)
}

func (e *fail2banClientCliExec) execute(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.ndsudoPath, args...)
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		if strings.HasPrefix(strings.TrimSpace(string(bs)), "Sorry but the jail") {
			return nil, errJailNotExist
		}
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
