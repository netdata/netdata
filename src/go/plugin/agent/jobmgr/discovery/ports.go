// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type PreparedCommandPort interface {
	SubmitPreparedAndWait(
		context.Context,
		jobmgr.Request,
		jobmgr.WorkPlan,
	) error
}

type DiscoveredChange struct {
	Config confgroup.Config
	Status dyncfg.Status
	Remove bool
}

type PlanDiscovered func(DiscoveredChange) (jobmgr.WorkPlan, error)
