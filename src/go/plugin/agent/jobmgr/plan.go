// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func RejectionPlan(status lifecycle.ControlStatus, claims ...string) WorkPlan {
	ownedClaims := slices.Clone(claims)
	return WorkPlan{
		Claims: ownedClaims,
		Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			return lifecycle.NewControlResult(status)
		}),
		CooperativeCancel: true, CooperativeDeadline: true,
	}
}
