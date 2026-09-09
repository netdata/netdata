// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"time"

	"github.com/vmware/govmomi/vim25/types"
)

const maxSnapshotTreeDepth = 64

type snapshotSummary struct {
	count            int64
	maxChainDepth    int64
	oldestCreateTime time.Time
}

func summarizeSnapshotInfo(info *types.VirtualMachineSnapshotInfo) snapshotSummary {
	if info == nil {
		return snapshotSummary{}
	}

	var summary snapshotSummary
	for i := range info.RootSnapshotList {
		walkSnapshotTree(info.RootSnapshotList[i], 1, &summary)
	}
	return summary
}

func walkSnapshotTree(node types.VirtualMachineSnapshotTree, depth int64, summary *snapshotSummary) {
	if depth > maxSnapshotTreeDepth {
		return
	}

	summary.count++
	if depth > summary.maxChainDepth {
		summary.maxChainDepth = depth
	}
	if !node.CreateTime.IsZero() && (summary.oldestCreateTime.IsZero() || node.CreateTime.Before(summary.oldestCreateTime)) {
		summary.oldestCreateTime = node.CreateTime
	}

	for i := range node.ChildSnapshotList {
		walkSnapshotTree(node.ChildSnapshotList[i], depth+1, summary)
	}
}
