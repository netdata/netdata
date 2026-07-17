// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
)

// CommandPort is the job lifecycle adapter's command submission capability.
type CommandPort interface {
	Submit(context.Context, jobmgr.Request) error
}
