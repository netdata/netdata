// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vmware/govmomi/vim25/types"
)

func TestSummarizeSnapshotInfo(t *testing.T) {
	now := time.Now().UTC()
	older := now.Add(-48 * time.Hour)
	newer := now.Add(-2 * time.Hour)
	oldest := now.Add(-72 * time.Hour)

	tests := map[string]struct {
		info *types.VirtualMachineSnapshotInfo
		want snapshotSummary
	}{
		"nil snapshot info": {},
		"empty root list": {
			info: &types.VirtualMachineSnapshotInfo{},
		},
		"single snapshot": {
			info: &types.VirtualMachineSnapshotInfo{
				RootSnapshotList: []types.VirtualMachineSnapshotTree{
					{CreateTime: older},
				},
			},
			want: snapshotSummary{
				count:            1,
				maxChainDepth:    1,
				oldestCreateTime: older,
			},
		},
		"siblings and nested chain": {
			info: &types.VirtualMachineSnapshotInfo{
				RootSnapshotList: []types.VirtualMachineSnapshotTree{
					{
						CreateTime: newer,
						ChildSnapshotList: []types.VirtualMachineSnapshotTree{
							{
								CreateTime: older,
								ChildSnapshotList: []types.VirtualMachineSnapshotTree{
									{CreateTime: oldest},
								},
							},
						},
					},
					{CreateTime: now},
				},
			},
			want: snapshotSummary{
				count:            4,
				maxChainDepth:    3,
				oldestCreateTime: oldest,
			},
		},
		"zero create time still counts": {
			info: &types.VirtualMachineSnapshotInfo{
				RootSnapshotList: []types.VirtualMachineSnapshotTree{
					{},
				},
			},
			want: snapshotSummary{
				count:         1,
				maxChainDepth: 1,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, summarizeSnapshotInfo(tc.info))
		})
	}
}

func TestSummarizeSnapshotInfoCapsTraversalDepth(t *testing.T) {
	root := types.VirtualMachineSnapshotTree{}
	node := &root
	for i := int64(1); i < maxSnapshotTreeDepth+10; i++ {
		node.ChildSnapshotList = []types.VirtualMachineSnapshotTree{{}}
		node = &node.ChildSnapshotList[0]
	}

	summary := summarizeSnapshotInfo(&types.VirtualMachineSnapshotInfo{
		RootSnapshotList: []types.VirtualMachineSnapshotTree{root},
	})

	assert.EqualValues(t, maxSnapshotTreeDepth, summary.count)
	assert.EqualValues(t, maxSnapshotTreeDepth, summary.maxChainDepth)
}
