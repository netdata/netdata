// SPDX-License-Identifier: GPL-3.0-or-later

package client

// https://github.com/dell/goscaleio/blob/master/types/v1/types.go

// For all 4xx and 5xx return codes, the body may contain an apiError instance
// with more specifics about the failure.
type apiError struct {
	Message        string
	HTTPStatusCode int
	ErrorCode      int
}

func (e apiError) Error() string {
	return e.Message
}

// Version represents API version.
type Version struct {
	Major int64
	Minor int64
}

// Bwc Bwc.
type Bwc struct {
	NumOccured      int64
	NumSeconds      int64
	TotalWeightInKb int64
}

// Sdc represents ScaleIO Data Client.
type Sdc struct {
	ID                 string
	SdcIp              string
	MdmConnectionState string
}

// StoragePool represents ScaleIO Storage Pool.
type StoragePool struct {
	ID                             string
	Name                           string
	SparePercentage                int64
	CapacityAlertCriticalThreshold int64
	CapacityAlertHighThreshold     int64
}

// Instances represents '/api/instances' response.
type Instances struct {
	StoragePoolList []StoragePool
	SdcList         []Sdc
}

type (
	// SelectedStatisticsQuery represents '/api/instances/querySelectedStatistics' query.
	SelectedStatisticsQuery struct {
		List []SelectedObject `json:"selectedStatisticsList"`
	}
	// SelectedObject represents '/api/instances/querySelectedStatistics' query object.
	SelectedObject struct {
		Type string `json:"type"` // object type (System, ProtectionDomain, Sds, StoragePool, Device, Volume, VTree, Sdc, FaultSet, RfcacheDevice).

		// the following parameters are not relevant to the System type and can be omitted:
		IDs    []string `json:"ids,omitempty"`    // list of objects ids
		AllIDs allIds   `json:"allIds,omitempty"` // all available objects

		Properties []string `json:"properties"` // list of properties to fetch
	}
	allIds bool
)

func (b allIds) MarshalJSON() ([]byte, error) {
	// should be set to empty value if AllIDs is true.
	if b {
		return []byte("[]"), nil
	}
	return nil, nil
}
func (b *allIds) UnmarshalJSON([]byte) error {
	*b = true
	return nil
}

// SelectedStatistics represents '/api/instances/querySelectedStatistics' response.
type SelectedStatistics struct {
	System      SystemStatistics
	Sdc         map[string]SdcStatistics
	StoragePool map[string]StoragePoolStatistics
}

// Those commented out structure fields are not deleted on purpose. We need them to see what other metrics can be collected.
type (
	// CapacityStatistics is System/StoragePool capacity statistics.
	CapacityStatistics struct {
		CapacityAvailableForVolumeAllocationInKb int64
		MaxCapacityInKb                          int64
		CapacityLimitInKb                        int64
		ProtectedCapacityInKb                    int64
		DegradedFailedCapacityInKb               int64
		DegradedHealthyCapacityInKb              int64
		SpareCapacityInKb                        int64
		FailedCapacityInKb                       int64
		UnreachableUnusedCapacityInKb            int64
		InMaintenanceCapacityInKb                int64
		ThinCapacityAllocatedInKb                int64
		ThinCapacityInUseInKb                    int64
		ThickCapacityInUseInKb                   int64
		SnapCapacityInUseOccupiedInKb            int64
		CapacityInUseInKb                        int64
	}
	SystemStatistics struct {
		CapacityStatistics

		NumOfDevices            int64
		NumOfFaultSets          int64
		NumOfProtectionDomains  int64
		NumOfRfcacheDevices     int64
		NumOfSdc                int64
		NumOfSds                int64
		NumOfSnapshots          int64
		NumOfStoragePools       int64
		NumOfVolumes            int64
		NumOfVtrees             int64
		NumOfThickBaseVolumes   int64
		NumOfThinBaseVolumes    int64
		NumOfMappedToAllVolumes int64
		NumOfUnmappedVolumes    int64

		RebalanceReadBwc             Bwc
		RebalanceWriteBwc            Bwc
		PendingRebalanceCapacityInKb int64

		PendingNormRebuildCapacityInKb int64
		PendingBckRebuildCapacityInKb  int64
		PendingFwdRebuildCapacityInKb  int64
		NormRebuildReadBwc             Bwc // TODO: ???
		NormRebuildWriteBwc            Bwc // TODO: ???
		BckRebuildReadBwc              Bwc // failed node/disk is back alive
		BckRebuildWriteBwc             Bwc // failed node/disk is back alive
		FwdRebuildReadBwc              Bwc // node/disk fails
		FwdRebuildWriteBwc             Bwc // node/disk fails

		PrimaryReadBwc    Bwc // Backend (SDSs + Devices) Primary - Mater MDM
		PrimaryWriteBwc   Bwc // Backend (SDSs + Devices) Primary - Mater MDM
		SecondaryReadBwc  Bwc // Backend (SDSs + Devices, 2nd) Secondary - Slave MDM
		SecondaryWriteBwc Bwc // Backend (SDSs + Devices, 2nd) Secondary - Slave MDM
		UserDataReadBwc   Bwc // Frontend (Volumes + SDCs)
		UserDataWriteBwc  Bwc // Frontend (Volumes + SDCs)
		TotalReadBwc      Bwc // *ReadBwc
		TotalWriteBwc     Bwc // *WriteBwc

		//SnapCapacityInUseInKb                           int64
		//BackgroundScanCompareCount                      int64
		//BackgroundScannedInMB                           int64
		//ActiveBckRebuildCapacityInKb                    int64
		//ActiveFwdRebuildCapacityInKb                    int64
		//ActiveMovingCapacityInKb                        int64
		//ActiveMovingInBckRebuildJobs                    int64
		//ActiveMovingInFwdRebuildJobs                    int64
		//ActiveMovingInNormRebuildJobs                   int64
		//ActiveMovingInRebalanceJobs                     int64
		//ActiveMovingOutBckRebuildJobs                   int64
		//ActiveMovingOutFwdRebuildJobs                   int64
		//ActiveMovingOutNormRebuildJobs                  int64
		//ActiveMovingRebalanceJobs                       int64
		//ActiveNormRebuildCapacityInKb                   int64
		//ActiveRebalanceCapacityInKb                     int64
		//AtRestCapacityInKb                              int64
		//BckRebuildCapacityInKb                          int64
		//DegradedFailedVacInKb                           int64
		//DegradedHealthyVacInKb                          int64
		//FailedVacInKb                                   int64
		//FixedReadErrorCount                             int64
		//FwdRebuildCapacityInKb                          int64
		//InMaintenanceVacInKb                            int64
		//InUseVacInKb                                    int64
		//MovingCapacityInKb                              int64
		//NormRebuildCapacityInKb                         int64
		//NumOfScsiInitiators                             int64 // removed from version 3 of ScaleIO/VxFlex API
		//PendingMovingCapacityInKb                       int64
		//PendingMovingInBckRebuildJobs                   int64
		//PendingMovingInFwdRebuildJobs                   int64
		//PendingMovingInNormRebuildJobs                  int64
		//PendingMovingInRebalanceJobs                    int64
		//PendingMovingOutBckRebuildJobs                  int64
		//PendingMovingOutFwdRebuildJobs                  int64
		//PendingMovingOutNormrebuildJobs                 int64
		//PendingMovingRebalanceJobs                      int64
		//PrimaryReadFromDevBwc                           int64
		//PrimaryReadFromRmcacheBwc                       int64
		//PrimaryVacInKb                                  int64
		//ProtectedVacInKb                                int64
		//ProtectionDomainIds                             int64
		//RebalanceCapacityInKb                           int64
		//RebalancePerReceiveJobNetThrottlingInKbps       int64
		//RebalanceWaitSendQLength                        int64
		//RebuildPerReceiveJobNetThrottlingInKbps         int64
		//RebuildWaitSendQLength                          int64
		//RfacheReadHit                                   int64
		//RfacheWriteHit                                  int64
		//RfcacheAvgReadTime                              int64
		//RfcacheAvgWriteTime                             int64
		//RfcacheFdAvgReadTime                            int64
		//RfcacheFdAvgWriteTime                           int64
		//RfcacheFdCacheOverloaded                        int64
		//RfcacheFdInlightReads                           int64
		//RfcacheFdInlightWrites                          int64
		//RfcacheFdIoErrors                               int64
		//RfcacheFdMonitorErrorStuckIo                    int64
		//RfcacheFdReadTimeGreater1Min                    int64
		//RfcacheFdReadTimeGreater1Sec                    int64
		//RfcacheFdReadTimeGreater500Millis               int64
		//RfcacheFdReadTimeGreater5Sec                    int64
		//RfcacheFdReadsReceived                          int64
		//RfcacheFdWriteTimeGreater1Min                   int64
		//RfcacheFdWriteTimeGreater1Sec                   int64
		//RfcacheFdWriteTimeGreater500Millis              int64
		//RfcacheFdWriteTimeGreater5Sec                   int64
		//RfcacheFdWritesReceived                         int64
		//RfcacheIoErrors                                 int64
		//RfcacheIosOutstanding                           int64
		//RfcacheIosSkipped                               int64
		//RfcachePooIosOutstanding                        int64
		//RfcachePoolCachePages                           int64
		//RfcachePoolEvictions                            int64
		//RfcachePoolInLowMemoryCondition                 int64
		//RfcachePoolIoTimeGreater1Min                    int64
		//RfcachePoolLockTimeGreater1Sec                  int64
		//RfcachePoolLowResourcesInitiatedPassthroughMode int64
		//RfcachePoolNumCacheDevs                         int64
		//RfcachePoolNumSrcDevs                           int64
		//RfcachePoolPagesInuse                           int64
		//RfcachePoolReadHit                              int64
		//RfcachePoolReadMiss                             int64
		//RfcachePoolReadPendingG10Millis                 int64
		//RfcachePoolReadPendingG1Millis                  int64
		//RfcachePoolReadPendingG1Sec                     int64
		//RfcachePoolReadPendingG500Micro                 int64
		//RfcachePoolReadsPending                         int64
		//RfcachePoolSize                                 int64
		//RfcachePoolSourceIdMismatch                     int64
		//RfcachePoolSuspendedIos                         int64
		//RfcachePoolSuspendedPequestsRedundantSearchs    int64
		//RfcachePoolWriteHit                             int64
		//RfcachePoolWriteMiss                            int64
		//RfcachePoolWritePending                         int64
		//RfcachePoolWritePendingG10Millis                int64
		//RfcachePoolWritePendingG1Millis                 int64
		//RfcachePoolWritePendingG1Sec                    int64
		//RfcachePoolWritePendingG500Micro                int64
		//RfcacheReadMiss                                 int64
		//RfcacheReadsFromCache                           int64
		//RfcacheReadsPending                             int64
		//RfcacheReadsReceived                            int64
		//RfcacheReadsSkipped                             int64
		//RfcacheReadsSkippedAlignedSizeTooLarge          int64
		//RfcacheReadsSkippedHeavyLoad                    int64
		//RfcacheReadsSkippedInternalError                int64
		//RfcacheReadsSkippedLockIos                      int64
		//RfcacheReadsSkippedLowResources                 int64
		//RfcacheReadsSkippedMaxIoSize                    int64
		//RfcacheReadsSkippedStuckIo                      int64
		//RfcacheSkippedUnlinedWrite                      int64
		//RfcacheSourceDeviceReads                        int64
		//RfcacheSourceDeviceWrites                       int64
		//RfcacheWriteMiss                                int64
		//RfcacheWritePending                             int64
		//RfcacheWritesReceived                           int64
		//RfcacheWritesSkippedCacheMiss                   int64
		//RfcacheWritesSkippedHeavyLoad                   int64
		//RfcacheWritesSkippedInternalError               int64
		//RfcacheWritesSkippedLowResources                int64
		//RfcacheWritesSkippedMaxIoSize                   int64
		//RfcacheWritesSkippedStuckIo                     int64
		//RmPendingAllocatedInKb                          int64
		//Rmcache128kbEntryCount                          int64
		//Rmcache16kbEntryCount                           int64
		//Rmcache32kbEntryCount                           int64
		//Rmcache4kbEntryCount                            int64
		//Rmcache64kbEntryCount                           int64
		//Rmcache8kbEntryCount                            int64
		//RmcacheBigBlockEvictionCount                    int64
		//RmcacheBigBlockEvictionSizeCountInKb            int64
		//RmcacheCurrNumOf128kbEntries                    int64
		//RmcacheCurrNumOf16kbEntries                     int64
		//RmcacheCurrNumOf32kbEntries                     int64
		//RmcacheCurrNumOf4kbEntries                      int64
		//RmcacheCurrNumOf64kbEntries                     int64
		//RmcacheCurrNumOf8kbEntries                      int64
		//RmcacheEntryEvictionCount                       int64
		//RmcacheEntryEvictionSizeCountInKb               int64
		//RmcacheNoEvictionCount                          int64
		//RmcacheSizeInKb                                 int64
		//RmcacheSizeInUseInKb                            int64
		//RmcacheSkipCountCacheAllBusy                    int64
		//RmcacheSkipCountLargeIo                         int64
		//RmcacheSkipCountUnaligned4kbIo                  int64
		//ScsiInitiatorIds                                int64
		//SdcIds                                          int64
		//SecondaryReadFromDevBwc                         int64
		//SecondaryReadFromRmcacheBwc                     int64
		//SecondaryVacInKb                                int64
		//SemiProtectedCapacityInKb                       int64
		//SemiProtectedVacInKb                            int64
		//SnapCapacityInUseOccupiedInKb                   int64
		//UnusedCapacityInKb                              int64
	}
	SdcStatistics struct {
		NumOfMappedVolumes int64
		UserDataReadBwc    Bwc
		UserDataWriteBwc   Bwc
		//VolumeIds          int64
	}
	StoragePoolStatistics struct {
		CapacityStatistics

		NumOfDevices   int64
		NumOfVolumes   int64
		NumOfVtrees    int64
		NumOfSnapshots int64

		//SnapCapacityInUseInKb                  int64
		//BackgroundScanCompareCount             int64
		//BackgroundScannedInMB                  int64
		//ActiveBckRebuildCapacityInKb           int64
		//ActiveFwdRebuildCapacityInKb           int64
		//ActiveMovingCapacityInKb               int64
		//ActiveMovingInBckRebuildJobs           int64
		//ActiveMovingInFwdRebuildJobs           int64
		//ActiveMovingInNormRebuildJobs          int64
		//ActiveMovingInRebalanceJobs            int64
		//ActiveMovingOutBckRebuildJobs          int64
		//ActiveMovingOutFwdRebuildJobs          int64
		//ActiveMovingOutNormRebuildJobs         int64
		//ActiveMovingRebalanceJobs              int64
		//ActiveNormRebuildCapacityInKb          int64
		//ActiveRebalanceCapacityInKb            int64
		//AtRestCapacityInKb                     int64
		//BckRebuildCapacityInKb                 int64
		//BckRebuildReadBwc                      int64
		//BckRebuildWriteBwc                     int64
		//DegradedFailedVacInKb                  int64
		//DegradedHealthyVacInKb                 int64
		//DeviceIds                              int64
		//FailedVacInKb                          int64
		//FixedReadErrorCount                    int64
		//FwdRebuildCapacityInKb                 int64
		//FwdRebuildReadBwc                      int64
		//FwdRebuildWriteBwc                     int64
		//InMaintenanceVacInKb                   int64
		//InUseVacInKb                           int64
		//MovingCapacityInKb                     int64
		//NormRebuildCapacityInKb                int64
		//NormRebuildReadBwc                     int64
		//NormRebuildWriteBwc                    int64
		//NumOfMappedToAllVolumes                int64
		//NumOfThickBaseVolumes                  int64
		//NumOfThinBaseVolumes                   int64
		//NumOfUnmappedVolumes                   int64
		//NumOfVolumesInDeletion                 int64
		//PendingBckRebuildCapacityInKb          int64
		//PendingFwdRebuildCapacityInKb          int64
		//PendingMovingCapacityInKb              int64
		//PendingMovingInBckRebuildJobs          int64
		//PendingMovingInFwdRebuildJobs          int64
		//PendingMovingInNormRebuildJobs         int64
		//PendingMovingInRebalanceJobs           int64
		//PendingMovingOutBckRebuildJobs         int64
		//PendingMovingOutFwdRebuildJobs         int64
		//PendingMovingOutNormrebuildJobs        int64
		//PendingMovingRebalanceJobs             int64
		//PendingNormRebuildCapacityInKb         int64
		//PendingRebalanceCapacityInKb           int64
		//PrimaryReadBwc                         int64
		//PrimaryReadFromDevBwc                  int64
		//PrimaryReadFromRmcacheBwc              int64
		//PrimaryVacInKb                         int64
		//PrimaryWriteBwc                        int64
		//ProtectedVacInKb                       int64
		//RebalanceCapacityInKb                  int64
		//RebalanceReadBwc                       int64
		//RebalanceWriteBwc                      int64
		//RfacheReadHit                          int64
		//RfacheWriteHit                         int64
		//RfcacheAvgReadTime                     int64
		//RfcacheAvgWriteTime                    int64
		//RfcacheIoErrors                        int64
		//RfcacheIosOutstanding                  int64
		//RfcacheIosSkipped                      int64
		//RfcacheReadMiss                        int64
		//RfcacheReadsFromCache                  int64
		//RfcacheReadsPending                    int64
		//RfcacheReadsReceived                   int64
		//RfcacheReadsSkipped                    int64
		//RfcacheReadsSkippedAlignedSizeTooLarge int64
		//RfcacheReadsSkippedHeavyLoad           int64
		//RfcacheReadsSkippedInternalError       int64
		//RfcacheReadsSkippedLockIos             int64
		//RfcacheReadsSkippedLowResources        int64
		//RfcacheReadsSkippedMaxIoSize           int64
		//RfcacheReadsSkippedStuckIo             int64
		//RfcacheSkippedUnlinedWrite             int64
		//RfcacheSourceDeviceReads               int64
		//RfcacheSourceDeviceWrites              int64
		//RfcacheWriteMiss                       int64
		//RfcacheWritePending                    int64
		//RfcacheWritesReceived                  int64
		//RfcacheWritesSkippedCacheMiss          int64
		//RfcacheWritesSkippedHeavyLoad          int64
		//RfcacheWritesSkippedInternalError      int64
		//RfcacheWritesSkippedLowResources       int64
		//RfcacheWritesSkippedMaxIoSize          int64
		//RfcacheWritesSkippedStuckIo            int64
		//RmPendingAllocatedInKb                 int64
		//SecondaryReadBwc                       int64
		//SecondaryReadFromDevBwc                int64
		//SecondaryReadFromRmcacheBwc            int64
		//SecondaryVacInKb                       int64
		//SecondaryWriteBwc                      int64
		//SemiProtectedCapacityInKb              int64
		//SemiProtectedVacInKb                   int64
		//SnapCapacityInUseOccupiedInKb          int64
		//TotalReadBwc                           int64
		//TotalWriteBwc                          int64
		//UnusedCapacityInKb                     int64
		//UserDataReadBwc                        int64
		//UserDataWriteBwc                       int64
		//VolumeIds                              int64
		//VtreeIds                               int64
	}
	DeviceStatistic struct {
		//	BackgroundScanCompareCount             int64
		//	BackgroundScannedInMB                  int64
		//	ActiveMovingInBckRebuildJobs           int64
		//	ActiveMovingInFwdRebuildJobs           int64
		//	ActiveMovingInNormRebuildJobs          int64
		//	ActiveMovingInRebalanceJobs            int64
		//	ActiveMovingOutBckRebuildJobs          int64
		//	ActiveMovingOutFwdRebuildJobs          int64
		//	ActiveMovingOutNormRebuildJobs         int64
		//	ActiveMovingRebalanceJobs              int64
		//	AvgReadLatencyInMicrosec               int64
		//	AvgReadSizeInBytes                     int64
		//	AvgWriteLatencyInMicrosec              int64
		//	AvgWriteSizeInBytes                    int64
		//	BckRebuildReadBwc                      int64
		//	BckRebuildWriteBwc                     int64
		//	CapacityInUseInKb                      int64
		//	CapacityLimitInKb                      int64
		//	DegradedFailedVacInKb                  int64
		//	DegradedHealthyVacInKb                 int64
		//	FailedVacInKb                          int64
		//	FixedReadErrorCount                    int64
		//	FwdRebuildReadBwc                      int64
		//	FwdRebuildWriteBwc                     int64
		//	InMaintenanceVacInKb                   int64
		//	InUseVacInKb                           int64
		//	MaxCapacityInKb                        int64
		//	NormRebuildReadBwc                     int64
		//	NormRebuildWriteBwc                    int64
		//	PendingMovingInBckRebuildJobs          int64
		//	PendingMovingInFwdRebuildJobs          int64
		//	PendingMovingInNormRebuildJobs         int64
		//	PendingMovingInRebalanceJobs           int64
		//	PendingMovingOutBckRebuildJobs         int64
		//	PendingMovingOutFwdRebuildJobs         int64
		//	PendingMovingOutNormrebuildJobs        int64
		//	PendingMovingRebalanceJobs             int64
		//	PrimaryReadBwc                         int64
		//	PrimaryReadFromDevBwc                  int64
		//	PrimaryReadFromRmcacheBwc              int64
		//	PrimaryVacInKb                         int64
		//	PrimaryWriteBwc                        int64
		//	ProtectedVacInKb                       int64
		//	RebalanceReadBwc                       int64
		//	RebalanceWriteBwc                      int64
		//	RfacheReadHit                          int64
		//	RfacheWriteHit                         int64
		//	RfcacheAvgReadTime                     int64
		//	RfcacheAvgWriteTime                    int64
		//	RfcacheIoErrors                        int64
		//	RfcacheIosOutstanding                  int64
		//	RfcacheIosSkipped                      int64
		//	RfcacheReadMiss                        int64
		//	RfcacheReadsFromCache                  int64
		//	RfcacheReadsPending                    int64
		//	RfcacheReadsReceived                   int64
		//	RfcacheReadsSkipped                    int64
		//	RfcacheReadsSkippedAlignedSizeTooLarge int64
		//	RfcacheReadsSkippedHeavyLoad           int64
		//	RfcacheReadsSkippedInternalError       int64
		//	RfcacheReadsSkippedLockIos             int64
		//	RfcacheReadsSkippedLowResources        int64
		//	RfcacheReadsSkippedMaxIoSize           int64
		//	RfcacheReadsSkippedStuckIo             int64
		//	RfcacheSkippedUnlinedWrite             int64
		//	RfcacheSourceDeviceReads               int64
		//	RfcacheSourceDeviceWrites              int64
		//	RfcacheWriteMiss                       int64
		//	RfcacheWritePending                    int64
		//	RfcacheWritesReceived                  int64
		//	RfcacheWritesSkippedCacheMiss          int64
		//	RfcacheWritesSkippedHeavyLoad          int64
		//	RfcacheWritesSkippedInternalError      int64
		//	RfcacheWritesSkippedLowResources       int64
		//	RfcacheWritesSkippedMaxIoSize          int64
		//	RfcacheWritesSkippedStuckIo            int64
		//	RmPendingAllocatedInKb                 int64
		//	SecondaryReadBwc                       int64
		//	SecondaryReadFromDevBwc                int64
		//	SecondaryReadFromRmcacheBwc            int64
		//	SecondaryVacInKb                       int64
		//	SecondaryWriteBwc                      int64
		//	SemiProtectedVacInKb                   int64
		//	SnapCapacityInUseInKb                  int64
		//	SnapCapacityInUseOccupiedInKb          int64
		//	ThickCapacityInUseInKb                 int64
		//	ThinCapacityAllocatedInKb              int64
		//	ThinCapacityInUseInKb                  int64
		//	TotalReadBwc                           int64
		//	TotalWriteBwc                          int64
		//	UnreachableUnusedCapacityInKb          int64
		//	UnusedCapacityInKb                     int64
	}
	FaultSetStatistics struct {
		//	BackgroundScanCompareCount                      int64
		//	BackgroundScannedInMB                           int64
		//	ActiveMovingInBckRebuildJobs                    int64
		//	ActiveMovingInFwdRebuildJobs                    int64
		//	ActiveMovingInNormRebuildJobs                   int64
		//	ActiveMovingInRebalanceJobs                     int64
		//	ActiveMovingOutBckRebuildJobs                   int64
		//	ActiveMovingOutFwdRebuildJobs                   int64
		//	ActiveMovingOutNormRebuildJobs                  int64
		//	ActiveMovingRebalanceJobs                       int64
		//	BckRebuildReadBwc                               int64
		//	BckRebuildWriteBwc                              int64
		//	CapacityInUseInKb                               int64
		//	CapacityLimitInKb                               int64
		//	DegradedFailedVacInKb                           int64
		//	DegradedHealthyVacInKb                          int64
		//	FailedVacInKb                                   int64
		//	FixedReadErrorCount                             int64
		//	FwdRebuildReadBwc                               int64
		//	FwdRebuildWriteBwc                              int64
		//	InMaintenanceVacInKb                            int64
		//	InUseVacInKb                                    int64
		//	MaxCapacityInKb                                 int64
		//	NormRebuildReadBwc                              int64
		//	NormRebuildWriteBwc                             int64
		//	NumOfSds                                        int64
		//	PendingMovingInBckRebuildJobs                   int64
		//	PendingMovingInFwdRebuildJobs                   int64
		//	PendingMovingInNormRebuildJobs                  int64
		//	PendingMovingInRebalanceJobs                    int64
		//	PendingMovingOutBckRebuildJobs                  int64
		//	PendingMovingOutFwdRebuildJobs                  int64
		//	PendingMovingOutNormrebuildJobs                 int64
		//	PendingMovingRebalanceJobs                      int64
		//	PrimaryReadBwc                                  int64
		//	PrimaryReadFromDevBwc                           int64
		//	PrimaryReadFromRmcacheBwc                       int64
		//	PrimaryVacInKb                                  int64
		//	PrimaryWriteBwc                                 int64
		//	ProtectedVacInKb                                int64
		//	RebalancePerReceiveJobNetThrottlingInKbps       int64
		//	RebalanceReadBwc                                int64
		//	RebalanceWaitSendQLength                        int64
		//	RebalanceWriteBwc                               int64
		//	RebuildPerReceiveJobNetThrottlingInKbps         int64
		//	RebuildWaitSendQLength                          int64
		//	RfacheReadHit                                   int64
		//	RfacheWriteHit                                  int64
		//	RfcacheAvgReadTime                              int64
		//	RfcacheAvgWriteTime                             int64
		//	RfcacheFdAvgReadTime                            int64
		//	RfcacheFdAvgWriteTime                           int64
		//	RfcacheFdCacheOverloaded                        int64
		//	RfcacheFdInlightReads                           int64
		//	RfcacheFdInlightWrites                          int64
		//	RfcacheFdIoErrors                               int64
		//	RfcacheFdMonitorErrorStuckIo                    int64
		//	RfcacheFdReadTimeGreater1Min                    int64
		//	RfcacheFdReadTimeGreater1Sec                    int64
		//	RfcacheFdReadTimeGreater500Millis               int64
		//	RfcacheFdReadTimeGreater5Sec                    int64
		//	RfcacheFdReadsReceived                          int64
		//	RfcacheFdWriteTimeGreater1Min                   int64
		//	RfcacheFdWriteTimeGreater1Sec                   int64
		//	RfcacheFdWriteTimeGreater500Millis              int64
		//	RfcacheFdWriteTimeGreater5Sec                   int64
		//	RfcacheFdWritesReceived                         int64
		//	RfcacheIoErrors                                 int64
		//	RfcacheIosOutstanding                           int64
		//	RfcacheIosSkipped                               int64
		//	RfcachePooIosOutstanding                        int64
		//	RfcachePoolCachePages                           int64
		//	RfcachePoolEvictions                            int64
		//	RfcachePoolInLowMemoryCondition                 int64
		//	RfcachePoolIoTimeGreater1Min                    int64
		//	RfcachePoolLockTimeGreater1Sec                  int64
		//	RfcachePoolLowResourcesInitiatedPassthroughMode int64
		//	RfcachePoolNumCacheDevs                         int64
		//	RfcachePoolNumSrcDevs                           int64
		//	RfcachePoolPagesInuse                           int64
		//	RfcachePoolReadHit                              int64
		//	RfcachePoolReadMiss                             int64
		//	RfcachePoolReadPendingG10Millis                 int64
		//	RfcachePoolReadPendingG1Millis                  int64
		//	RfcachePoolReadPendingG1Sec                     int64
		//	RfcachePoolReadPendingG500Micro                 int64
		//	RfcachePoolReadsPending                         int64
		//	RfcachePoolSize                                 int64
		//	RfcachePoolSourceIdMismatch                     int64
		//	RfcachePoolSuspendedIos                         int64
		//	RfcachePoolSuspendedPequestsRedundantSearchs    int64
		//	RfcachePoolWriteHit                             int64
		//	RfcachePoolWriteMiss                            int64
		//	RfcachePoolWritePending                         int64
		//	RfcachePoolWritePendingG10Millis                int64
		//	RfcachePoolWritePendingG1Millis                 int64
		//	RfcachePoolWritePendingG1Sec                    int64
		//	RfcachePoolWritePendingG500Micro                int64
		//	RfcacheReadMiss                                 int64
		//	RfcacheReadsFromCache                           int64
		//	RfcacheReadsPending                             int64
		//	RfcacheReadsReceived                            int64
		//	RfcacheReadsSkipped                             int64
		//	RfcacheReadsSkippedAlignedSizeTooLarge          int64
		//	RfcacheReadsSkippedHeavyLoad                    int64
		//	RfcacheReadsSkippedInternalError                int64
		//	RfcacheReadsSkippedLockIos                      int64
		//	RfcacheReadsSkippedLowResources                 int64
		//	RfcacheReadsSkippedMaxIoSize                    int64
		//	RfcacheReadsSkippedStuckIo                      int64
		//	RfcacheSkippedUnlinedWrite                      int64
		//	RfcacheSourceDeviceReads                        int64
		//	RfcacheSourceDeviceWrites                       int64
		//	RfcacheWriteMiss                                int64
		//	RfcacheWritePending                             int64
		//	RfcacheWritesReceived                           int64
		//	RfcacheWritesSkippedCacheMiss                   int64
		//	RfcacheWritesSkippedHeavyLoad                   int64
		//	RfcacheWritesSkippedInternalError               int64
		//	RfcacheWritesSkippedLowResources                int64
		//	RfcacheWritesSkippedMaxIoSize                   int64
		//	RfcacheWritesSkippedStuckIo                     int64
		//	RmPendingAllocatedInKb                          int64
		//	Rmcache128kbEntryCount                          int64
		//	Rmcache16kbEntryCount                           int64
		//	Rmcache32kbEntryCount                           int64
		//	Rmcache4kbEntryCount                            int64
		//	Rmcache64kbEntryCount                           int64
		//	Rmcache8kbEntryCount                            int64
		//	RmcacheBigBlockEvictionCount                    int64
		//	RmcacheBigBlockEvictionSizeCountInKb            int64
		//	RmcacheCurrNumOf128kbEntries                    int64
		//	RmcacheCurrNumOf16kbEntries                     int64
		//	RmcacheCurrNumOf32kbEntries                     int64
		//	RmcacheCurrNumOf4kbEntries                      int64
		//	RmcacheCurrNumOf64kbEntries                     int64
		//	RmcacheCurrNumOf8kbEntries                      int64
		//	RmcacheEntryEvictionCount                       int64
		//	RmcacheEntryEvictionSizeCountInKb               int64
		//	RmcacheNoEvictionCount                          int64
		//	RmcacheSizeInKb                                 int64
		//	RmcacheSizeInUseInKb                            int64
		//	RmcacheSkipCountCacheAllBusy                    int64
		//	RmcacheSkipCountLargeIo                         int64
		//	RmcacheSkipCountUnaligned4kbIo                  int64
		//	SdsIds                                          int64
		//	SecondaryReadBwc                                int64
		//	SecondaryReadFromDevBwc                         int64
		//	SecondaryReadFromRmcacheBwc                     int64
		//	SecondaryVacInKb                                int64
		//	SecondaryWriteBwc                               int64
		//	SemiProtectedVacInKb                            int64
		//	SnapCapacityInUseInKb                           int64
		//	SnapCapacityInUseOccupiedInKb                   int64
		//	ThickCapacityInUseInKb                          int64
		//	ThinCapacityAllocatedInKb                       int64
		//	ThinCapacityInUseInKb                           int64
		//	TotalReadBwc                                    int64
		//	TotalWriteBwc                                   int64
		//	UnreachableUnusedCapacityInKb                   int64
		//	UnusedCapacityInKb                              int64
	}
	ProtectionDomainStatistics struct {
		//	BackgroundScanCompareCount                      int64
		//	BackgroundScannedInMB                           int64
		//	ActiveBckRebuildCapacityInKb                    int64
		//	ActiveFwdRebuildCapacityInKb                    int64
		//	ActiveMovingCapacityInKb                        int64
		//	ActiveMovingInBckRebuildJobs                    int64
		//	ActiveMovingInFwdRebuildJobs                    int64
		//	ActiveMovingInNormRebuildJobs                   int64
		//	ActiveMovingInRebalanceJobs                     int64
		//	ActiveMovingOutBckRebuildJobs                   int64
		//	ActiveMovingOutFwdRebuildJobs                   int64
		//	ActiveMovingOutNormRebuildJobs                  int64
		//	ActiveMovingRebalanceJobs                       int64
		//	ActiveNormRebuildCapacityInKb                   int64
		//	ActiveRebalanceCapacityInKb                     int64
		//	AtRestCapacityInKb                              int64
		//	BckRebuildCapacityInKb                          int64
		//	BckRebuildReadBwc                               int64
		//	BckRebuildWriteBwc                              int64
		//	CapacityAvailableForVolumeAllocationInKb        int64
		//	CapacityInUseInKb                               int64
		//	CapacityLimitInKb                               int64
		//	DegradedFailedCapacityInKb                      int64
		//	DegradedFailedVacInKb                           int64
		//	DegradedHealthyCapacityInKb                     int64
		//	DegradedHealthyVacInKb                          int64
		//	FailedCapacityInKb                              int64
		//	FailedVacInKb                                   int64
		//	FaultSetIds                                     int64
		//	FixedReadErrorCount                             int64
		//	FwdRebuildCapacityInKb                          int64
		//	FwdRebuildReadBwc                               int64
		//	FwdRebuildWriteBwc                              int64
		//	InMaintenanceCapacityInKb                       int64
		//	InMaintenanceVacInKb                            int64
		//	InUseVacInKb                                    int64
		//	MaxCapacityInKb                                 int64
		//	MovingCapacityInKb                              int64
		//	NormRebuildCapacityInKb                         int64
		//	NormRebuildReadBwc                              int64
		//	NormRebuildWriteBwc                             int64
		//	NumOfFaultSets                                  int64
		//	NumOfMappedToAllVolumes                         int64
		//	NumOfSds                                        int64
		//	NumOfSnapshots                                  int64
		//	NumOfStoragePools                               int64
		//	NumOfThickBaseVolumes                           int64
		//	NumOfThinBaseVolumes                            int64
		//	NumOfUnmappedVolumes                            int64
		//	NumOfVolumesInDeletion                          int64
		//	PendingBckRebuildCapacityInKb                   int64
		//	PendingFwdRebuildCapacityInKb                   int64
		//	PendingMovingCapacityInKb                       int64
		//	PendingMovingInBckRebuildJobs                   int64
		//	PendingMovingInFwdRebuildJobs                   int64
		//	PendingMovingInNormRebuildJobs                  int64
		//	PendingMovingInRebalanceJobs                    int64
		//	PendingMovingOutBckRebuildJobs                  int64
		//	PendingMovingOutFwdRebuildJobs                  int64
		//	PendingMovingOutNormrebuildJobs                 int64
		//	PendingMovingRebalanceJobs                      int64
		//	PendingNormRebuildCapacityInKb                  int64
		//	PendingRebalanceCapacityInKb                    int64
		//	PrimaryReadBwc                                  int64
		//	PrimaryReadFromDevBwc                           int64
		//	PrimaryReadFromRmcacheBwc                       int64
		//	PrimaryVacInKb                                  int64
		//	PrimaryWriteBwc                                 int64
		//	ProtectedCapacityInKb                           int64
		//	ProtectedVacInKb                                int64
		//	RebalanceCapacityInKb                           int64
		//	RebalancePerReceiveJobNetThrottlingInKbps       int64
		//	RebalanceReadBwc                                int64
		//	RebalanceWaitSendQLength                        int64
		//	RebalanceWriteBwc                               int64
		//	RebuildPerReceiveJobNetThrottlingInKbps         int64
		//	RebuildWaitSendQLength                          int64
		//	RfacheReadHit                                   int64
		//	RfacheWriteHit                                  int64
		//	RfcacheAvgReadTime                              int64
		//	RfcacheAvgWriteTime                             int64
		//	RfcacheFdAvgReadTime                            int64
		//	RfcacheFdAvgWriteTime                           int64
		//	RfcacheFdCacheOverloaded                        int64
		//	RfcacheFdInlightReads                           int64
		//	RfcacheFdInlightWrites                          int64
		//	RfcacheFdIoErrors                               int64
		//	RfcacheFdMonitorErrorStuckIo                    int64
		//	RfcacheFdReadTimeGreater1Min                    int64
		//	RfcacheFdReadTimeGreater1Sec                    int64
		//	RfcacheFdReadTimeGreater500Millis               int64
		//	RfcacheFdReadTimeGreater5Sec                    int64
		//	RfcacheFdReadsReceived                          int64
		//	RfcacheFdWriteTimeGreater1Min                   int64
		//	RfcacheFdWriteTimeGreater1Sec                   int64
		//	RfcacheFdWriteTimeGreater500Millis              int64
		//	RfcacheFdWriteTimeGreater5Sec                   int64
		//	RfcacheFdWritesReceived                         int64
		//	RfcacheIoErrors                                 int64
		//	RfcacheIosOutstanding                           int64
		//	RfcacheIosSkipped                               int64
		//	RfcachePooIosOutstanding                        int64
		//	RfcachePoolCachePages                           int64
		//	RfcachePoolEvictions                            int64
		//	RfcachePoolInLowMemoryCondition                 int64
		//	RfcachePoolIoTimeGreater1Min                    int64
		//	RfcachePoolLockTimeGreater1Sec                  int64
		//	RfcachePoolLowResourcesInitiatedPassthroughMode int64
		//	RfcachePoolNumCacheDevs                         int64
		//	RfcachePoolNumSrcDevs                           int64
		//	RfcachePoolPagesInuse                           int64
		//	RfcachePoolReadHit                              int64
		//	RfcachePoolReadMiss                             int64
		//	RfcachePoolReadPendingG10Millis                 int64
		//	RfcachePoolReadPendingG1Millis                  int64
		//	RfcachePoolReadPendingG1Sec                     int64
		//	RfcachePoolReadPendingG500Micro                 int64
		//	RfcachePoolReadsPending                         int64
		//	RfcachePoolSize                                 int64
		//	RfcachePoolSourceIdMismatch                     int64
		//	RfcachePoolSuspendedIos                         int64
		//	RfcachePoolSuspendedPequestsRedundantSearchs    int64
		//	RfcachePoolWriteHit                             int64
		//	RfcachePoolWriteMiss                            int64
		//	RfcachePoolWritePending                         int64
		//	RfcachePoolWritePendingG10Millis                int64
		//	RfcachePoolWritePendingG1Millis                 int64
		//	RfcachePoolWritePendingG1Sec                    int64
		//	RfcachePoolWritePendingG500Micro                int64
		//	RfcacheReadMiss                                 int64
		//	RfcacheReadsFromCache                           int64
		//	RfcacheReadsPending                             int64
		//	RfcacheReadsReceived                            int64
		//	RfcacheReadsSkipped                             int64
		//	RfcacheReadsSkippedAlignedSizeTooLarge          int64
		//	RfcacheReadsSkippedHeavyLoad                    int64
		//	RfcacheReadsSkippedInternalError                int64
		//	RfcacheReadsSkippedLockIos                      int64
		//	RfcacheReadsSkippedLowResources                 int64
		//	RfcacheReadsSkippedMaxIoSize                    int64
		//	RfcacheReadsSkippedStuckIo                      int64
		//	RfcacheSkippedUnlinedWrite                      int64
		//	RfcacheSourceDeviceReads                        int64
		//	RfcacheSourceDeviceWrites                       int64
		//	RfcacheWriteMiss                                int64
		//	RfcacheWritePending                             int64
		//	RfcacheWritesReceived                           int64
		//	RfcacheWritesSkippedCacheMiss                   int64
		//	RfcacheWritesSkippedHeavyLoad                   int64
		//	RfcacheWritesSkippedInternalError               int64
		//	RfcacheWritesSkippedLowResources                int64
		//	RfcacheWritesSkippedMaxIoSize                   int64
		//	RfcacheWritesSkippedStuckIo                     int64
		//	RmPendingAllocatedInKb                          int64
		//	Rmcache128kbEntryCount                          int64
		//	Rmcache16kbEntryCount                           int64
		//	Rmcache32kbEntryCount                           int64
		//	Rmcache4kbEntryCount                            int64
		//	Rmcache64kbEntryCount                           int64
		//	Rmcache8kbEntryCount                            int64
		//	RmcacheBigBlockEvictionCount                    int64
		//	RmcacheBigBlockEvictionSizeCountInKb            int64
		//	RmcacheCurrNumOf128kbEntries                    int64
		//	RmcacheCurrNumOf16kbEntries                     int64
		//	RmcacheCurrNumOf32kbEntries                     int64
		//	RmcacheCurrNumOf4kbEntries                      int64
		//	RmcacheCurrNumOf64kbEntries                     int64
		//	RmcacheCurrNumOf8kbEntries                      int64
		//	RmcacheEntryEvictionCount                       int64
		//	RmcacheEntryEvictionSizeCountInKb               int64
		//	RmcacheNoEvictionCount                          int64
		//	RmcacheSizeInKb                                 int64
		//	RmcacheSizeInUseInKb                            int64
		//	RmcacheSkipCountCacheAllBusy                    int64
		//	RmcacheSkipCountLargeIo                         int64
		//	RmcacheSkipCountUnaligned4kbIo                  int64
		//	SdsIds                                          int64
		//	SecondaryReadBwc                                int64
		//	SecondaryReadFromDevBwc                         int64
		//	SecondaryReadFromRmcacheBwc                     int64
		//	SecondaryVacInKb                                int64
		//	SecondaryWriteBwc                               int64
		//	SemiProtectedCapacityInKb                       int64
		//	SemiProtectedVacInKb                            int64
		//	SnapCapacityInUseInKb                           int64
		//	SnapCapacityInUseOccupiedInKb                   int64
		//	SpareCapacityInKb                               int64
		//	StoragePoolIds                                  int64
		//	ThickCapacityInUseInKb                          int64
		//	ThinCapacityAllocatedInKb                       int64
		//	ThinCapacityInUseInKb                           int64
		//	TotalReadBwc                                    int64
		//	TotalWriteBwc                                   int64
		//	UnreachableUnusedCapacityInKb                   int64
		//	UnusedCapacityInKb                              int64
		//	UserDataReadBwc                                 int64
		//	UserDataWriteBwc                                int64
	}
	RFCacheDeviceStatistics struct {
		//	RfcacheFdAvgReadTime               int64
		//	RfcacheFdAvgWriteTime              int64
		//	RfcacheFdCacheOverloaded           int64
		//	RfcacheFdInlightReads              int64
		//	RfcacheFdInlightWrites             int64
		//	RfcacheFdIoErrors                  int64
		//	RfcacheFdMonitorErrorStuckIo       int64
		//	RfcacheFdReadTimeGreater1Min       int64
		//	RfcacheFdReadTimeGreater1Sec       int64
		//	RfcacheFdReadTimeGreater500Millis  int64
		//	RfcacheFdReadTimeGreater5Sec       int64
		//	RfcacheFdReadsReceived             int64
		//	RfcacheFdWriteTimeGreater1Min      int64
		//	RfcacheFdWriteTimeGreater1Sec      int64
		//	RfcacheFdWriteTimeGreater500Millis int64
		//	RfcacheFdWriteTimeGreater5Sec      int64
		//	RfcacheFdWritesReceived            int64
	}
	SdsStatistics struct {
		//	BackgroundScanCompareCount                      int64
		//	BackgroundScannedInMB                           int64
		//	ActiveMovingInBckRebuildJobs                    int64
		//	ActiveMovingInFwdRebuildJobs                    int64
		//	ActiveMovingInNormRebuildJobs                   int64
		//	ActiveMovingInRebalanceJobs                     int64
		//	ActiveMovingOutBckRebuildJobs                   int64
		//	ActiveMovingOutFwdRebuildJobs                   int64
		//	ActiveMovingOutNormRebuildJobs                  int64
		//	ActiveMovingRebalanceJobs                       int64
		//	BckRebuildReadBwc                               int64
		//	BckRebuildWriteBwc                              int64
		//	CapacityInUseInKb                               int64
		//	CapacityLimitInKb                               int64
		//	DegradedFailedVacInKb                           int64
		//	DegradedHealthyVacInKb                          int64
		//	DeviceIds                                       int64
		//	FailedVacInKb                                   int64
		//	FixedReadErrorCount                             int64
		//	FwdRebuildReadBwc                               int64
		//	FwdRebuildWriteBwc                              int64
		//	InMaintenanceVacInKb                            int64
		//	InUseVacInKb                                    int64
		//	MaxCapacityInKb                                 int64
		//	NormRebuildReadBwc                              int64
		//	NormRebuildWriteBwc                             int64
		//	NumOfDevices                                    int64
		//	NumOfRfcacheDevices                             int64
		//	PendingMovingInBckRebuildJobs                   int64
		//	PendingMovingInFwdRebuildJobs                   int64
		//	PendingMovingInNormRebuildJobs                  int64
		//	PendingMovingInRebalanceJobs                    int64
		//	PendingMovingOutBckRebuildJobs                  int64
		//	PendingMovingOutFwdRebuildJobs                  int64
		//	PendingMovingOutNormrebuildJobs                 int64
		//	PendingMovingRebalanceJobs                      int64
		//	PrimaryReadBwc                                  int64
		//	PrimaryReadFromDevBwc                           int64
		//	PrimaryReadFromRmcacheBwc                       int64
		//	PrimaryVacInKb                                  int64
		//	PrimaryWriteBwc                                 int64
		//	ProtectedVacInKb                                int64
		//	RebalancePerReceiveJobNetThrottlingInKbps       int64
		//	RebalanceReadBwc                                int64
		//	RebalanceWaitSendQLength                        int64
		//	RebalanceWriteBwc                               int64
		//	RebuildPerReceiveJobNetThrottlingInKbps         int64
		//	RebuildWaitSendQLength                          int64
		//	RfacheReadHit                                   int64
		//	RfacheWriteHit                                  int64
		//	RfcacheAvgReadTime                              int64
		//	RfcacheAvgWriteTime                             int64
		//	RfcacheDeviceIds                                int64
		//	RfcacheFdAvgReadTime                            int64
		//	RfcacheFdAvgWriteTime                           int64
		//	RfcacheFdCacheOverloaded                        int64
		//	RfcacheFdInlightReads                           int64
		//	RfcacheFdInlightWrites                          int64
		//	RfcacheFdIoErrors                               int64
		//	RfcacheFdMonitorErrorStuckIo                    int64
		//	RfcacheFdReadTimeGreater1Min                    int64
		//	RfcacheFdReadTimeGreater1Sec                    int64
		//	RfcacheFdReadTimeGreater500Millis               int64
		//	RfcacheFdReadTimeGreater5Sec                    int64
		//	RfcacheFdReadsReceived                          int64
		//	RfcacheFdWriteTimeGreater1Min                   int64
		//	RfcacheFdWriteTimeGreater1Sec                   int64
		//	RfcacheFdWriteTimeGreater500Millis              int64
		//	RfcacheFdWriteTimeGreater5Sec                   int64
		//	RfcacheFdWritesReceived                         int64
		//	RfcacheIoErrors                                 int64
		//	RfcacheIosOutstanding                           int64
		//	RfcacheIosSkipped                               int64
		//	RfcachePooIosOutstanding                        int64
		//	RfcachePoolCachePages                           int64
		//	RfcachePoolContinuosMem                         int64
		//	RfcachePoolEvictions                            int64
		//	RfcachePoolInLowMemoryCondition                 int64
		//	RfcachePoolIoTimeGreater1Min                    int64
		//	RfcachePoolLockTimeGreater1Sec                  int64
		//	RfcachePoolLowResourcesInitiatedPassthroughMode int64
		//	RfcachePoolMaxIoSize                            int64
		//	RfcachePoolNumCacheDevs                         int64
		//	RfcachePoolNumOfDriverTheads                    int64
		//	RfcachePoolNumSrcDevs                           int64
		//	RfcachePoolOpmode                               int64
		//	RfcachePoolPageSize                             int64
		//	RfcachePoolPagesInuse                           int64
		//	RfcachePoolReadHit                              int64
		//	RfcachePoolReadMiss                             int64
		//	RfcachePoolReadPendingG10Millis                 int64
		//	RfcachePoolReadPendingG1Millis                  int64
		//	RfcachePoolReadPendingG1Sec                     int64
		//	RfcachePoolReadPendingG500Micro                 int64
		//	RfcachePoolReadsPending                         int64
		//	RfcachePoolSize                                 int64
		//	RfcachePoolSourceIdMismatch                     int64
		//	RfcachePoolSuspendedIos                         int64
		//	RfcachePoolSuspendedIosMax                      int64
		//	RfcachePoolSuspendedPequestsRedundantSearchs    int64
		//	RfcachePoolWriteHit                             int64
		//	RfcachePoolWriteMiss                            int64
		//	RfcachePoolWritePending                         int64
		//	RfcachePoolWritePendingG10Millis                int64
		//	RfcachePoolWritePendingG1Millis                 int64
		//	RfcachePoolWritePendingG1Sec                    int64
		//	RfcachePoolWritePendingG500Micro                int64
		//	RfcacheReadMiss                                 int64
		//	RfcacheReadsFromCache                           int64
		//	RfcacheReadsPending                             int64
		//	RfcacheReadsReceived                            int64
		//	RfcacheReadsSkipped                             int64
		//	RfcacheReadsSkippedAlignedSizeTooLarge          int64
		//	RfcacheReadsSkippedHeavyLoad                    int64
		//	RfcacheReadsSkippedInternalError                int64
		//	RfcacheReadsSkippedLockIos                      int64
		//	RfcacheReadsSkippedLowResources                 int64
		//	RfcacheReadsSkippedMaxIoSize                    int64
		//	RfcacheReadsSkippedStuckIo                      int64
		//	RfcacheSkippedUnlinedWrite                      int64
		//	RfcacheSourceDeviceReads                        int64
		//	RfcacheSourceDeviceWrites                       int64
		//	RfcacheWriteMiss                                int64
		//	RfcacheWritePending                             int64
		//	RfcacheWritesReceived                           int64
		//	RfcacheWritesSkippedCacheMiss                   int64
		//	RfcacheWritesSkippedHeavyLoad                   int64
		//	RfcacheWritesSkippedInternalError               int64
		//	RfcacheWritesSkippedLowResources                int64
		//	RfcacheWritesSkippedMaxIoSize                   int64
		//	RfcacheWritesSkippedStuckIo                     int64
		//	RmPendingAllocatedInKb                          int64
		//	Rmcache128kbEntryCount                          int64
		//	Rmcache16kbEntryCount                           int64
		//	Rmcache32kbEntryCount                           int64
		//	Rmcache4kbEntryCount                            int64
		//	Rmcache64kbEntryCount                           int64
		//	Rmcache8kbEntryCount                            int64
		//	RmcacheBigBlockEvictionCount                    int64
		//	RmcacheBigBlockEvictionSizeCountInKb            int64
		//	RmcacheCurrNumOf128kbEntries                    int64
		//	RmcacheCurrNumOf16kbEntries                     int64
		//	RmcacheCurrNumOf32kbEntries                     int64
		//	RmcacheCurrNumOf4kbEntries                      int64
		//	RmcacheCurrNumOf64kbEntries                     int64
		//	RmcacheCurrNumOf8kbEntries                      int64
		//	RmcacheEntryEvictionCount                       int64
		//	RmcacheEntryEvictionSizeCountInKb               int64
		//	RmcacheNoEvictionCount                          int64
		//	RmcacheSizeInKb                                 int64
		//	RmcacheSizeInUseInKb                            int64
		//	RmcacheSkipCountCacheAllBusy                    int64
		//	RmcacheSkipCountLargeIo                         int64
		//	RmcacheSkipCountUnaligned4kbIo                  int64
		//	SecondaryReadBwc                                int64
		//	SecondaryReadFromDevBwc                         int64
		//	SecondaryReadFromRmcacheBwc                     int64
		//	SecondaryVacInKb                                int64
		//	SecondaryWriteBwc                               int64
		//	SemiProtectedVacInKb                            int64
		//	SnapCapacityInUseInKb                           int64
		//	SnapCapacityInUseOccupiedInKb                   int64
		//	ThickCapacityInUseInKb                          int64
		//	ThinCapacityAllocatedInKb                       int64
		//	ThinCapacityInUseInKb                           int64
		//	TotalReadBwc                                    int64
		//	TotalWriteBwc                                   int64
		//	UnreachableUnusedCapacityInKb                   int64
		//	UnusedCapacityInKb                              int64
	}
	VolumeStatistics struct {
		//	ChildVolumeIds            int64
		//	DescendantVolumeIds       int64
		//	MappedSdcIds              int64
		//	NumOfChildVolumes         int64
		//	NumOfDescendantVolumes    int64
		//	NumOfMappedScsiInitiators int64
		//	NumOfMappedSdcs           int64
		//	UserDataReadBwc           int64
		//	UserDataWriteBwc          int64
	}
	VTreeStatistics struct {
		//	BaseNetCapacityInUseInKb int64
		//	NetCapacityInUseInKb     int64
		//	NumOfVolumes             int64
		//	SnapNetCapacityInUseInKb int64
		//	TrimmedCapacityInKb      int64
		//	VolumeIds                int64
	}
)
