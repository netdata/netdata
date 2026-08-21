// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

type retainedStateBudget struct {
	entries int
	members int
}

var (
	collectionProgressRetentionBudget = retainedStateBudget{
		entries: maxGraphResources,
		members: maxGraphResources,
	}
	graphMembershipRetentionBudget = retainedStateBudget{
		entries: maxGraphResources,
		members: maxGraphResources,
	}
	detailGateRetentionBudget = retainedStateBudget{
		entries: maxGraphResources,
		members: maxGraphResources,
	}
	aggregateRetentionBudget = retainedStateBudget{
		entries: maxGraphResources,
		// Aggregate snapshots retain one canonical member set plus the
		// authoritative collection-slice memberships used to prove completeness.
		members: maxGraphResources * 4,
	}
)

func retainedStateFits(
	entries int,
	members int,
	addedEntries int,
	addedMembers int,
	budget retainedStateBudget,
) bool {
	if entries < 0 || members < 0 || addedEntries < 0 || addedMembers < 0 {
		return false
	}
	return addedEntries <= budget.entries-entries &&
		addedMembers <= budget.members-members
}
