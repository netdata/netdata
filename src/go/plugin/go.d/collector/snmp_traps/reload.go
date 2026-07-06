// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/snmptrapsfunc"
)

const (
	snmpTrapsLogsMethodID = snmptrapsfunc.LogsMethodID
	snmpTrapsFunctionName = snmptrapsfunc.FunctionName
)

func snmpTrapsMethods() []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{
		snmptrapsfunc.LogsFunctionConfig(directJournalLogsAvailable),
	}
}

func snmpTrapsMethodHandler(_ collectorapi.RuntimeJob) funcapi.MethodHandler {
	return snmptrapsfunc.NewHandler(journalBaseRoot())
}
