// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type PreparedCommandPort interface {
	SubmitPreparedAndWait(context.Context, jobmgr.Request, jobmgr.WorkPlan) error
}

type DiscoveredChange struct {
	Config confgroup.Config // the winning discovered config
	Status dyncfg.Status    // target status (Accepted / Running)
	Remove bool             // remove the job rather than install it
}

type PlanDiscovered func(DiscoveredChange) (jobmgr.WorkPlan, error)
