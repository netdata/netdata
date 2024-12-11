// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package smartctl

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"

	"github.com/tidwall/gjson"
)

type smartctlCli interface {
	scan(open bool) (*gjson.Result, error)
	deviceInfo(deviceName, deviceType, powerMode string) (*gjson.Result, error)
}

func newSmartctlCliExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *smartctlCliExec {
	return &smartctlCliExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type smartctlCliExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *smartctlCliExec) scan(open bool) (*gjson.Result, error) {
	if open {
		return e.execute("smartctl-json-scan-open")
	}
	return e.execute("smartctl-json-scan")
}

func (e *smartctlCliExec) deviceInfo(deviceName, deviceType, powerMode string) (*gjson.Result, error) {
	return e.execute("smartctl-json-device-info",
		"--deviceName", deviceName,
		"--deviceType", deviceType,
		"--powerMode", powerMode,
	)
}

func (e *smartctlCliExec) execute(args ...string) (*gjson.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.ndsudoPath, args...)
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isExecExitCode(err, 1) || len(bs) == 0 {
			return nil, fmt.Errorf("'%s' execution failed: %v", cmd, err)
		}
	}
	if len(bs) == 0 {
		return nil, fmt.Errorf("'%s' returned no output", cmd)
	}

	if !gjson.ValidBytes(bs) {
		return nil, fmt.Errorf("'%s' returned invalid JSON output", cmd)
	}

	res := gjson.ParseBytes(bs)
	if !res.Get("smartctl.exit_status").Exists() {
		return nil, fmt.Errorf("'%s' returned unexpected data", cmd)
	}

	for _, msg := range res.Get("smartctl.messages").Array() {
		if msg.Get("severity").String() == "error" {
			return &res, fmt.Errorf("'%s' reported an error: %s", cmd, msg.Get("string"))
		}
	}

	return &res, nil
}

func isExecExitCode(err error, exitCode int) bool {
	var v *exec.ExitError
	return errors.As(err, &v) && v.ExitCode() == exitCode
}
