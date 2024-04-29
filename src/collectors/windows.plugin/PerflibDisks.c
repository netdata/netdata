// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct logical_disk {
    STRING *filesystem;

    RRDSET *st_disk_space;
    RRDDIM *rd_disk_space_used;
    RRDDIM *rd_disk_space_free;

    COUNTER_DATA percentDiskFree;
    // COUNTER_DATA freeMegabytes;
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

void dict_logical_disk_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct logical_disk *ld = value;

    ld->percentDiskFree.name = "% Free Space";
    // ld->freeMegabytes.name = "Free Megabytes";
    ld->percentIdleTime.name = "% Idle Time";
    ld->percentDiskTime.name = "% Disk Time";
    ld->percentDiskReadTime.name = "% Disk Read Time";
    ld->percentDiskWriteTime.name = "% Disk Write Time";
    ld->currentDiskQueueLength.name = "Current Disk Queue Length";
    ld->averageDiskQueueLength.name = "Avg. Disk Queue Length";
    ld->averageDiskReadQueueLength.name = "Avg. Disk Read Queue Length";
    ld->averageDiskWriteQueueLength.name = "Avg. Disk Write Queue Length";
    ld->averageDiskSecondsPerTransfer.name = "Avg. Disk sec/Transfer";
    ld->averageDiskSecondsPerRead.name = "Avg. Disk sec/Read";
    ld->averageDiskSecondsPerWrite.name = "Avg. Disk sec/Write";
    ld->diskTransfersPerSec.name = "Disk Transfers/sec";
    ld->diskReadsPerSec.name = "Disk Reads/sec";
    ld->diskWritesPerSec.name = "Disk Writes/sec";
    ld->diskBytesPerSec.name = "Disk Bytes/sec";
    ld->diskReadBytesPerSec.name = "Disk Read Bytes/sec";
    ld->diskWriteBytesPerSec.name = "Disk Write Bytes/sec";
    ld->averageDiskBytesPerTransfer.name = "Avg. Disk Bytes/Transfer";
    ld->averageDiskBytesPerRead.name = "Avg. Disk Bytes/Read";
    ld->averageDiskBytesPerWrite.name = "Avg. Disk Bytes/Write";
    ld->splitIoPerSec.name = "Split IO/Sec";
}

static DICTIONARY *logicalDisks = NULL;
static void initialize(void) {
    logicalDisks = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE |
                                                  DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct logical_disk));

    dictionary_register_insert_callback(logicalDisks, dict_logical_disk_insert_cb, NULL);
}

STRING *getFileSystemType(const char* diskName) {
    if (!diskName || !*diskName) return NULL;

    char fileSystemNameBuffer[128] = {0};  // Buffer for file system name
    char pathBuffer[256] = {0};            // Path buffer to accommodate different formats
    DWORD serialNumber = 0;
    DWORD maxComponentLength = 0;
    DWORD fileSystemFlags = 0;
    BOOL success;

    // Check if the input is likely a drive letter (e.g., "C:")
    if (isalpha((uint8_t)diskName[0]) && diskName[1] == ':' && diskName[2] == '\0')
        snprintf(pathBuffer, sizeof(pathBuffer), "%s\\", diskName);  // Format as "C:\\"
    else
        // Assume it's a Volume GUID path or a device path
        snprintf(pathBuffer, sizeof(pathBuffer), "\\\\.\\%s", diskName);  // Format as "\\.\HarddiskVolume1"

    // Attempt to get the volume information
    success = GetVolumeInformation(
        pathBuffer,                // Path to the disk
        NULL,                      // We don't need the volume name
        0,                         // Size of volume name buffer is 0
        &serialNumber,             // Volume serial number
        &maxComponentLength,       // Maximum component length
        &fileSystemFlags,          // File system flags
        fileSystemNameBuffer,      // File system name buffer
        sizeof(fileSystemNameBuffer) // Size of file system name buffer
    );

    if (success && fileSystemNameBuffer[0]) {
        char *s = fileSystemNameBuffer;
        while(*s) { *s = tolower((uint8_t)*s); s++; }
        return string_strdupz(fileSystemNameBuffer);  // Duplicate the file system name
    }
    else
        return string_strdupz("unknown");
}

int do_PerflibDisks(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    char name[4096];

    DWORD id = RegistryFindIDByName("LogicalDisk");
    if(id == REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) return -1;

    PERF_OBJECT_TYPE *pLogicalDisk = perflibFindObjectTypeByName(pDataBlock, "LogicalDisk");
    if(pLogicalDisk) {
        PERF_INSTANCE_DEFINITION *pi = NULL;
        for(LONG i = 0; i < pLogicalDisk->NumInstances ; i++) {
            pi = perflibForEachInstance(pDataBlock, pLogicalDisk, pi);
            if(!pi) break;

            if(!getInstanceName(pDataBlock, pLogicalDisk, pi, name, sizeof(name)))
                strncpyz(name, "[unknown]", sizeof(name) - 1);

            if(strcasecmp(name, "_Total") == 0)
                continue;

            struct logical_disk *ld = dictionary_set(logicalDisks, name, NULL, sizeof(struct logical_disk));

            if(!ld->filesystem)
                ld->filesystem = getFileSystemType(name);

            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->percentDiskFree);
            // perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->freeMegabytes);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->percentIdleTime);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->percentDiskTime);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->percentDiskReadTime);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->percentDiskWriteTime);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->currentDiskQueueLength);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->averageDiskQueueLength);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->averageDiskReadQueueLength);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->averageDiskWriteQueueLength);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->averageDiskSecondsPerTransfer);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->averageDiskSecondsPerRead);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->averageDiskSecondsPerWrite);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->diskTransfersPerSec);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->diskReadsPerSec);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->diskWritesPerSec);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->diskBytesPerSec);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->diskReadBytesPerSec);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->diskWriteBytesPerSec);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->averageDiskBytesPerTransfer);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->averageDiskBytesPerRead);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->averageDiskBytesPerWrite);
            perflibGetInstanceCounter(pDataBlock, pLogicalDisk, pi, &ld->splitIoPerSec);

            if(!ld->st_disk_space) {
                ld->st_disk_space = rrdset_create_localhost(
                    "disk_space"
                    , name
                    , NULL
                    , name
                    , "disk.space"
                    , "Disk Space Usage"
                    , "GiB"
                    , PLUGIN_WINDOWS_NAME
                    , "PerflibDisks"
                    , NETDATA_CHART_PRIO_DISKSPACE_SPACE
                    , update_every
                    , RRDSET_TYPE_STACKED
                );

                rrdlabels_add(ld->st_disk_space->rrdlabels, "mount_point", name, RRDLABEL_SRC_AUTO);
                rrdlabels_add(ld->st_disk_space->rrdlabels, "mount_root", name, RRDLABEL_SRC_AUTO);
                rrdlabels_add(ld->st_disk_space->rrdlabels, "filesystem", string2str(ld->filesystem), RRDLABEL_SRC_AUTO);

                ld->rd_disk_space_free = rrddim_add(ld->st_disk_space, "avail", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
                ld->rd_disk_space_used = rrddim_add(ld->st_disk_space, "used", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            }

            // percentDiskFree has the free space in Data and the size of the disk in Time, in MiB.
            rrddim_set_by_pointer(ld->st_disk_space, ld->rd_disk_space_free, (collected_number)ld->percentDiskFree.current.Time);
            rrddim_set_by_pointer(ld->st_disk_space, ld->rd_disk_space_used, (collected_number)ld->percentDiskFree.current.Data);
            rrdset_done(ld->st_disk_space);
        }
    }

//    PERF_OBJECT_TYPE *pPhysicalDisk = perflibFindObjectTypeByName(pDataBlock, "PhysicalDisk");
//    if(pPhysicalDisk) {
//
//    }

    return 0;
}
