// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"

	"github.com/tidwall/gjson"
)

type smartctlCli interface {
	scan(open bool) (*gjson.Result, error)
	deviceInfo(deviceName, deviceType, powerMode string) (*gjson.Result, error)
}

// ndsudoSmartctlCli executes smartctl via ndsudo (Linux)
type ndsudoSmartctlCli struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func newNdsudoSmartctlCli(ndsudoPath string, timeout time.Duration, log *logger.Logger) *ndsudoSmartctlCli {
	return &ndsudoSmartctlCli{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

func (e *ndsudoSmartctlCli) scan(open bool) (*gjson.Result, error) {
	if open {
		return e.execute("smartctl-json-scan-open")
	}
	return e.execute("smartctl-json-scan")
}

func (e *ndsudoSmartctlCli) deviceInfo(deviceName, deviceType, powerMode string) (*gjson.Result, error) {
	return e.execute("smartctl-json-device-info",
		"--deviceName", deviceName,
		"--deviceType", deviceType,
		"--powerMode", powerMode,
	)
}

func (e *ndsudoSmartctlCli) execute(args ...string) (*gjson.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	command := ndexec.CommandNDSudo(ctx, e.Logger, args...)

	bs, err := command.Output()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isExecExitCode(err, 1) || len(bs) == 0 {
			return nil, fmt.Errorf("'%s' execution failed: %v", command, err)
		}
	}

	return parseOutput(command.String(), bs, args, e.Logger)
}

// directSmartctlCli executes smartctl directly (Windows, macOS, etc.)
type directSmartctlCli struct {
	*logger.Logger

	smartctlPath string
	timeout      time.Duration
}

func newDirectSmartctlCli(smartctlPath string, timeout time.Duration, log *logger.Logger) *directSmartctlCli {
	return &directSmartctlCli{
		Logger:       log,
		smartctlPath: smartctlPath,
		timeout:      timeout,
	}
}

func (e *directSmartctlCli) scan(open bool) (*gjson.Result, error) {
	args := []string{"--json", "--scan"}
	if open {
		args = append(args, "--scan-open")
	}
	return e.execute(args...)
}

func (e *directSmartctlCli) deviceInfo(deviceName, deviceType, powerMode string) (*gjson.Result, error) {
	args := []string{
		"--json",
		"--xall",
		"--device", deviceType,
		"--nocheck", powerMode,
		deviceName,
	}
	return e.execute(args...)
}

func (e *directSmartctlCli) execute(args ...string) (*gjson.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := ndexec.CommandUnprivileged(ctx, e.Logger, args...)

	bs, err := cmd.Output()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isExecExitCode(err, 1) || len(bs) == 0 {
			return nil, fmt.Errorf("'%s' execution failed: %v", cmd, err)
		}
	}

	return parseOutput(cmd.String(), bs, args, e.Logger)
}

// Common output parsing function
func parseOutput(cmdStr string, bs []byte, args []string, log *logger.Logger) (*gjson.Result, error) {
	if len(bs) == 0 {
		return nil, fmt.Errorf("'%s' returned no output", cmdStr)
	}

	if logger.Level.Enabled(slog.LevelDebug) {
		var buf bytes.Buffer
		if err := json.Compact(&buf, bs); err == nil {
			log.Debugf("exec: %v, resp: %s", args, buf.String())
		}
	}

	if !gjson.ValidBytes(bs) {
		return nil, fmt.Errorf("'%s' returned invalid JSON output", cmdStr)
	}

	res := gjson.ParseBytes(bs)
	if !res.Get("smartctl.exit_status").Exists() {
		return nil, fmt.Errorf("'%s' returned unexpected data", cmdStr)
	}

	for _, msg := range res.Get("smartctl.messages").Array() {
		if msg.Get("severity").String() == "error" {
			return &res, fmt.Errorf("'%s' reported an error: %s", cmdStr, msg.Get("string"))
		}
	}

	return &res, nil
}

func isExecExitCode(err error, exitCode int) bool {
	var v *exec.ExitError
	return errors.As(err, &v) && v.ExitCode() == exitCode
}
