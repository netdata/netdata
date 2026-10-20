// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

func jobAttemptIdentity(namespace jobmgr.ProcessAttemptNamespace, fullName string) jobmgr.ProcessAttemptIdentity {
	return jobmgr.ProcessAttemptIdentity{
		Namespace: namespace,
		Key: jobmgr.ProcessAttemptIdentityKey(
			"collector-job",
			fullName,
		),
		Resource: candidateDiagnosticResource(fullName),
	}
}

func jobTestAttemptIdentity(kind configOperationKind, config confgroup.Config) jobmgr.ProcessAttemptIdentity {
	return jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptJobTest,
		Key: jobmgr.ProcessAttemptIdentityKey(
			"collector-job-test",
			strconv.Itoa(int(kind)),
			config.FullName(),
			strconv.FormatUint(config.Hash(), 10),
		),
		Resource: candidateDiagnosticResource(config.FullName()),
	}
}

func candidateDiagnosticResource(name string) string {
	return jobmgr.ProcessAttemptDiagnosticResource(name, "collector job")
}
