// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
)

func modulePlanAttemptIdentity(module string) jobmgr.ProcessAttemptIdentity {
	return jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptFunctionBundle,
		Key: jobmgr.ProcessAttemptIdentityKey(
			"collector-function-agent",
			module,
		),
		Resource: candidateFunctionResource(module),
	}
}

func jobFunctionAttemptKey(epoch uint64, fullName string) string {
	return jobmgr.ProcessAttemptIdentityKey(
		"collector-function-job",
		strconv.FormatUint(epoch, 10),
		fullName,
	)
}

func candidateFunctionResource(module string) string {
	return jobmgr.ProcessAttemptDiagnosticResource(module, "collector module")
}
