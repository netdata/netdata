// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct LogicalDisk {
    DWORD id;
    STRING *name;
    DICTIONARY *counters;
};


struct logical_disk {
    COUNTER_DATA percentDiskFree;
    COUNTER_DATA freeMegabytes;

    COUNTER_DATA percentIdleTime;
    COUNTER_DATA percentDiskTime;
    COUNTER_DATA percentDiskReadTime;
    COUNTER_DATA percentDiskWriteTime;

    COUNTER_DATA currentDiskQueueLength;
    COUNTER_DATA averageDiskQueueLength;
    COUNTER_DATA averageDiskReadQueueLength;
    COUNTER_DATA averageDiskWriteQueueLength;

    COUNTER_DATA averageDiskSecondsPerTransfer;
    COUNTER_DATA averageDiskSecondsPerRead;
    COUNTER_DATA averageDiskSecondsPerWrite;

    COUNTER_DATA diskTransfersPerSec;
    COUNTER_DATA diskReadsPerSec;
    COUNTER_DATA diskWritesPerSec;

    COUNTER_DATA diskBytesPerSec;
    COUNTER_DATA diskReadBytesPerSec;
    COUNTER_DATA diskWriteBytesPerSec;

    COUNTER_DATA averageDiskBytesPerTransfer;
    COUNTER_DATA averageDiskBytesPerRead;
    COUNTER_DATA averageDiskBytesPerWrite;

    COUNTER_DATA splitIoPerSec;
};


int do_PerflibDisks(int update_every, usec_t dt __maybe_unused) {
    char name[4096];

    DWORD id = RegistryFindIDByName("LogicalDisk");
    if(id == REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    PERF_OBJECT_TYPE *pLogicalDisk = perflibFindObjectTypeByName(pDataBlock, "LogicalDisk");
    if(pLogicalDisk) {
        PERF_INSTANCE_DEFINITION *pi = NULL;
        for(LONG i = 0; i < pLogicalDisk->NumInstances ; i++) {
            pi = perflibForEachInstance(pDataBlock, pLogicalDisk, pi);
            if(!pi) break;

            if(!getInstanceName(pDataBlock, pLogicalDisk, pi, name, sizeof(name)))
                strncpyz(name, "[unknown]", sizeof(name) - 1);

            COUNTER_DATA diskFree = {
                .name = "% Free Space",
            }, freeMegabytes = {
                             .name = "Free Megabytes",
                         };

            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &diskFree);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &freeMegabytes);
        }
    }

    PERF_OBJECT_TYPE *pPhysicalDisk = perflibFindObjectTypeByName(pDataBlock, "PhysicalDisk");
    if(pPhysicalDisk) {

    }

    return 0;
}
