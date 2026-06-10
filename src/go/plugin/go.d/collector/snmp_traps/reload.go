// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	snmpTrapsLogsMethodID = "logs"
	snmpTrapsFunctionName = "snmp:traps"
)

func snmpTrapsMethods() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		snmpTrapsLogsMethodConfig(),
	}
}

func snmpTrapsLogsMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           snmpTrapsLogsMethodID,
		FunctionName: snmpTrapsFunctionName,
		Name:         "SNMP Trap Logs",
		UpdateEvery:  1,
		Help:         "Query SNMP trap journal entries received by SNMP trap listener jobs",
		RequireCloud: true,
		Tags:         "logs",
		ResponseType: "logs",
		Available:    directJournalLogsAvailable,
		RawRequest:   true,
		AgentWide:    true,
	}
}

func snmpTrapsMethodHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return newSNMPTrapsFunctionHandler(c)
}
