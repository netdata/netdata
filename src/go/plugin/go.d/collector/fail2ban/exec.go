// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package fail2ban

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

var errJailNotExist = errors.New("jail not exist")

const socketPathInDocker = "/host/var/run/fail2ban/fail2ban.sock"

type fail2banClientCli interface {
	status() ([]byte, error)
	jailStatus(s string) ([]byte, error)
}

func newFail2BanClientCliExec(timeout time.Duration, log *logger.Logger) *fail2banClientCliExec {
	_, err := os.Stat("/host/var/run")

	return &fail2banClientCliExec{
		Logger:         log,
		timeout:        timeout,
		isInsideDocker: err == nil,
	}
}

type fail2banClientCliExec struct {
	*logger.Logger

	timeout        time.Duration
	isInsideDocker bool
}

func (e *fail2banClientCliExec) status() ([]byte, error) {
	if e.isInsideDocker {
		return e.execute("fail2ban-client-status-socket",
			"--socket_path", socketPathInDocker,
		)
	}
	return e.execute("fail2ban-client-status")
}

func (e *fail2banClientCliExec) jailStatus(jail string) ([]byte, error) {
	if e.isInsideDocker {
		return e.execute("fail2ban-client-status-jail-socket",
			"--jail", jail,
			"--socket_path", socketPathInDocker,
		)
	}
	return e.execute("fail2ban-client-status-jail",
		"--jail", jail,
	)
}

func (e *fail2banClientCliExec) execute(cmd string, args ...string) ([]byte, error) {
	bs, err := ndexec.RunNDSudo(e.Logger, e.timeout, cmd, args...)
	if err != nil {
		if strings.HasPrefix(strings.TrimSpace(string(bs)), "Sorry but the jail") {
			return nil, errJailNotExist
		}
		return nil, err
	}
	return bs, err
}
