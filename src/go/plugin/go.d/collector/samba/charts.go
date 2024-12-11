// SPDX-License-Identifier: GPL-3.0-or-later

package samba

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioSyscallCalls = module.Priority + iota
	prioSyscallTransferredData

	prioSmb2CallCalls
	prioSmb2CallTransferredData
)

var (
	syscallCallsChartTmpl = module.Chart{
		ID:       "syscall_%s_calls",
		Title:    "Syscalls Count",
		Units:    "calls/s",
		Fam:      "syscalls",
		Ctx:      "samba.syscall_calls",
		Priority: prioSyscallCalls,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "syscall_%s_count", Name: "syscalls", Algo: module.Incremental},
		},
	}
	syscallTransferredDataChartTmpl = module.Chart{
		ID:       "syscall_%s_transferred_data",
		Title:    "Syscall Transferred Data",
		Units:    "bytes/s",
		Fam:      "syscalls",
		Ctx:      "samba.syscall_transferred_data",
		Priority: prioSyscallTransferredData,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "syscall_%s_bytes", Name: "transferred", Algo: module.Incremental},
		},
	}

	smb2CallCallsChartTmpl = module.Chart{
		ID:       "smb2_call_%s_calls",
		Title:    "SMB2 Calls Count",
		Units:    "calls/s",
		Fam:      "smb2 calls",
		Ctx:      "samba.smb2_call_calls",
		Priority: prioSmb2CallCalls,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "smb2_%s_count", Name: "smb2", Algo: module.Incremental},
		},
	}
	smb2CallTransferredDataChartTmpl = module.Chart{
		ID:       "smb2_call_%s_transferred_data",
		Title:    "SMB2 Call Transferred Data",
		Units:    "bytes/s",
		Fam:      "smb2 calls",
		Ctx:      "samba.smb2_call_transferred_data",
		Priority: prioSmb2CallTransferredData,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "smb2_%s_inbytes", Name: "in", Algo: module.Incremental},
			{ID: "smb2_%s_outbytes", Name: "out", Algo: module.Incremental, Mul: -1},
		},
	}
)

func (c *Collector) addCharts(mx map[string]int64) {
	for k := range mx {
		if name, ok := extractCallName(k, "syscall_", "_count"); ok {
			c.addSysCallChart(name, syscallCallsChartTmpl.Copy())
		} else if name, ok := extractCallName(k, "syscall_", "_bytes"); ok {
			c.addSysCallChart(name, syscallTransferredDataChartTmpl.Copy())
		} else if name, ok := extractCallName(k, "smb2_", "_count"); ok {
			c.addSmb2CallChart(name, smb2CallCallsChartTmpl.Copy())
			// all smb2* metrics have inbytes and outbytes
			c.addSmb2CallChart(name, smb2CallTransferredDataChartTmpl.Copy())
		}
	}
}

func (c *Collector) addSysCallChart(syscall string, chart *module.Chart) {
	chart = chart.Copy()
	chart.ID = fmt.Sprintf(chart.ID, syscall)
	chart.Labels = []module.Label{
		{Key: "syscall", Value: syscall},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, syscall)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addSmb2CallChart(smb2Call string, chart *module.Chart) {
	chart = chart.Copy()
	chart.ID = fmt.Sprintf(chart.ID, smb2Call)
	chart.Labels = []module.Label{
		{Key: "smb2call", Value: smb2Call},
	}
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, smb2Call)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func extractCallName(s, prefix, suffix string) (string, bool) {
	if !(strings.HasPrefix(s, prefix) && strings.HasSuffix(s, suffix)) {
		return "", false
	}
	name := strings.TrimPrefix(s, prefix)
	name = strings.TrimSuffix(name, suffix)
	return name, true
}
