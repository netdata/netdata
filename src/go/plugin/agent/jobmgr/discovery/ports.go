// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

type PreparedCommandPort interface {
	SubmitPreparedAndWait(context.Context, jobmgr.Request, jobmgr.WorkPlan) error
}

// SourceCandidate identifies one observed membership lifetime in a source slot.
type SourceCandidate struct {
	Config      confgroup.Config
	Source      string
	Incarnation uint64
}

// SourceInventory is the complete versioned source observation for one job.
// Empty candidates and sources together are an explicit removal observation.
type SourceInventory struct {
	FullName   string
	Version    uint64
	Candidates []SourceCandidate
	Sources    map[string]uint64 // observed per-source membership lifetimes, including malformed replacements
	AutoEnable bool
}

type PlanInventory func(SourceInventory) (jobmgr.WorkPlan, error)
