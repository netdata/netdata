// SPDX-License-Identifier: GPL-3.0-or-later

package scaleio

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/scaleio/client"

/*
Starting from version 3 of ScaleIO/VxFlex API numOfScsiInitiators property is removed from the system selectedStatisticsQuery.
Reference: VxFlex OS v3.x REST API Reference Guide.pdf
*/

var query = client.SelectedStatisticsQuery{
	List: []client.SelectedObject{
		{
			Type: "System",
			Properties: []string{
				"maxCapacityInKb",
				"thickCapacityInUseInKb",
				"thinCapacityInUseInKb",
				"snapCapacityInUseOccupiedInKb",
				"spareCapacityInKb",
				"capacityLimitInKb",

				"protectedCapacityInKb",
				"degradedHealthyCapacityInKb",
				"degradedFailedCapacityInKb",
				"failedCapacityInKb",
				"unreachableUnusedCapacityInKb",
				"inMaintenanceCapacityInKb",

				"capacityInUseInKb",
				"capacityAvailableForVolumeAllocationInKb",

				"numOfDevices",
				"numOfFaultSets",
				"numOfProtectionDomains",
				"numOfRfcacheDevices",
				"numOfSdc",
				"numOfSds",
				"numOfSnapshots",
				"numOfStoragePools",
				"numOfVolumes",
				"numOfVtrees",
				"numOfThickBaseVolumes",
				"numOfThinBaseVolumes",
				"numOfMappedToAllVolumes",
				"numOfUnmappedVolumes",

				"rebalanceReadBwc",
				"rebalanceWriteBwc",
				"pendingRebalanceCapacityInKb",

				"pendingNormRebuildCapacityInKb",
				"pendingBckRebuildCapacityInKb",
				"pendingFwdRebuildCapacityInKb",
				"normRebuildReadBwc",
				"normRebuildWriteBwc",
				"bckRebuildReadBwc",
				"bckRebuildWriteBwc",
				"fwdRebuildReadBwc",
				"fwdRebuildWriteBwc",

				"primaryReadBwc",
				"primaryWriteBwc",
				"secondaryReadBwc",
				"secondaryWriteBwc",
				"userDataReadBwc",
				"userDataWriteBwc",
				"totalReadBwc",
				"totalWriteBwc",
			},
		},
		{
			Type:   "StoragePool",
			AllIDs: true,
			Properties: []string{
				"maxCapacityInKb",
				"thickCapacityInUseInKb",
				"thinCapacityInUseInKb",
				"snapCapacityInUseOccupiedInKb",
				"spareCapacityInKb",
				"capacityLimitInKb",

				"protectedCapacityInKb",
				"degradedHealthyCapacityInKb",
				"degradedFailedCapacityInKb",
				"failedCapacityInKb",
				"unreachableUnusedCapacityInKb",
				"inMaintenanceCapacityInKb",

				"capacityInUseInKb",
				"capacityAvailableForVolumeAllocationInKb",

				"numOfDevices",
				"numOfVolumes",
				"numOfVtrees",
				"numOfSnapshots",
			},
		},
		{
			Type:   "Sdc",
			AllIDs: true,
			Properties: []string{
				"userDataReadBwc",
				"userDataWriteBwc",

				"numOfMappedVolumes",
			},
		},
	},
}
