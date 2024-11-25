// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || netbsd

package lvm

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

type lvmCLI interface {
	lvsReportJson() ([]byte, error)
}

func newLVMCLIExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *lvmCLIExec {
	return &lvmCLIExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type lvmCLIExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *lvmCLIExec) lvsReportJson() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		e.ndsudoPath,
		"lvs-report-json",
		"--options",
		"vg_name,lv_name,lv_size,data_percent,metadata_percent,lv_attr",
	)
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
