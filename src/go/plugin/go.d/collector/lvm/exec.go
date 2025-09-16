// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || netbsd

package lvm

import (
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
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
	return ndexec.RunNDSudo(
		e.Logger,
		e.timeout,
		"lvs-report-json",
		"--options",
		"vg_name,lv_name,lv_size,data_percent,metadata_percent,lv_attr",
	)
}
