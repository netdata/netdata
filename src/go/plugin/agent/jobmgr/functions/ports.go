// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"

// CommandPort is the Function ingress view of command orchestration.
type CommandPort interface {
	jobmgr.AdmissionCommandPort
}
