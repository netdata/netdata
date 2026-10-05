// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"

// mustDynCfgMessage builds an expected DynCfg message result.
func mustDynCfgMessage(code int, message string) lifecycle.SealedResult {
	return messageResult(code, message)
}
