// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct logical_disk {
    bool collected_metadata;

    STRING *filesystem;

    RRDSET *st_disk_space;
    RRDDIM *rd_disk_space_used;
    RRDDIM *rd_disk_space_free;

    COUNTER_DATA percentDiskFree;
    // COUNTER_DATA freeMegabytes;
};

struct physical_disk {
    bool collected_metadata;

    STRING *device_type;
    STRING *model;
    STRING *serial;
    STRING *id;

    RRDSET *st_io;
    RRDDIM *rd_io_reads;
    RRDDIM *rd_io_writes;
    COUNTER_DATA diskReadBytesPerSec;
    COUNTER_DATA diskWriteBytesPerSec;

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
    COUNTER_DATA averageDiskBytesPerTransfer;
    COUNTER_DATA averageDiskBytesPerRead;
    COUNTER_DATA averageDiskBytesPerWrite;
    COUNTER_DATA splitIoPerSec;
};

struct physical_disk system_physical_total = {
    .collected_metadata = true,
};

void dict_logical_disk_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct logical_disk *ld = value;

    ld->percentDiskFree.key = "% Free Space";
    // ld->freeMegabytes.key = "Free Megabytes";
}

void initialize_physical_disk(struct physical_disk *pd) {
    pd->percentIdleTime.key = "% Idle Time";
    pd->percentDiskTime.key = "% Disk Time";
    pd->percentDiskReadTime.key = "% Disk Read Time";
    pd->percentDiskWriteTime.key = "% Disk Write Time";
    pd->currentDiskQueueLength.key = "Current Disk Queue Length";
    pd->averageDiskQueueLength.key = "Avg. Disk Queue Length";
    pd->averageDiskReadQueueLength.key = "Avg. Disk Read Queue Length";
    pd->averageDiskWriteQueueLength.key = "Avg. Disk Write Queue Length";
    pd->averageDiskSecondsPerTransfer.key = "Avg. Disk sec/Transfer";
    pd->averageDiskSecondsPerRead.key = "Avg. Disk sec/Read";
    pd->averageDiskSecondsPerWrite.key = "Avg. Disk sec/Write";
    pd->diskTransfersPerSec.key = "Disk Transfers/sec";
    pd->diskReadsPerSec.key = "Disk Reads/sec";
    pd->diskWritesPerSec.key = "Disk Writes/sec";
    pd->diskBytesPerSec.key = "Disk Bytes/sec";
    pd->diskReadBytesPerSec.key = "Disk Read Bytes/sec";
    pd->diskWriteBytesPerSec.key = "Disk Write Bytes/sec";
    pd->averageDiskBytesPerTransfer.key = "Avg. Disk Bytes/Transfer";
    pd->averageDiskBytesPerRead.key = "Avg. Disk Bytes/Read";
    pd->averageDiskBytesPerWrite.key = "Avg. Disk Bytes/Write";
    pd->splitIoPerSec.key = "Split IO/Sec";
}

void dict_physical_disk_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct physical_disk *pd = value;
    initialize_physical_disk(pd);
}

static DICTIONARY *logicalDisks = NULL, *physicalDisks = NULL;
static void initialize(void) {
    initialize_physical_disk(&system_physical_total);

    logicalDisks = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE |
                                                  DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct logical_disk));

    dictionary_register_insert_callback(logicalDisks, dict_logical_disk_insert_cb, NULL);

    physicalDisks = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE |
                                                  DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct physical_disk));

    dictionary_register_insert_callback(physicalDisks, dict_physical_disk_insert_cb, NULL);
}

static STRING *getFileSystemType(const char* diskName) {
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
        return NULL;
}

static char buffer[4096];

static bool do_logical_disk(PERF_DATA_BLOCK *pDataBlock, int update_every) {
    DICTIONARY *dict = logicalDisks;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "LogicalDisk");
    if(!pObjectType) return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for(LONG i = 0; i < pObjectType->NumInstances ; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if(!pi) break;

        if(!getInstanceName(pDataBlock, pObjectType, pi, buffer, sizeof(buffer)))
            strncpyz(buffer, "[unknown]", sizeof(buffer) - 1);

        if(strcasecmp(buffer, "_Total") == 0)
            continue;

        struct logical_disk *d = dictionary_set(dict, buffer, NULL, sizeof(*d));

        if(!d->collected_metadata) {
            d->filesystem = getFileSystemType(buffer);
            d->collected_metadata = true;
        }

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->percentDiskFree);
        // perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->freeMegabytes);

        if(!d->st_disk_space) {
            d->st_disk_space = rrdset_create_localhost(
                "disk_space"
                , buffer
                , NULL
                , buffer
                , "disk.space"
                , "Disk Space Usage"
                , "GiB"
                , PLUGIN_WINDOWS_NAME
                , "PerflibStorage"
                , NETDATA_CHART_PRIO_DISKSPACE_SPACE
                , update_every
                , RRDSET_TYPE_STACKED
            );

            rrdlabels_add(d->st_disk_space->rrdlabels, "mount_point", buffer, RRDLABEL_SRC_AUTO);
            // rrdlabels_add(d->st->rrdlabels, "mount_root", name, RRDLABEL_SRC_AUTO);

            if(d->filesystem)
                rrdlabels_add(d->st_disk_space->rrdlabels, "filesystem", string2str(d->filesystem), RRDLABEL_SRC_AUTO);

            d->rd_disk_space_free = rrddim_add(d->st_disk_space, "avail", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
            d->rd_disk_space_used = rrddim_add(d->st_disk_space, "used", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }

        // percentDiskFree has the free space in Data and the size of the disk in Time, in MiB.
        rrddim_set_by_pointer(d->st_disk_space, d->rd_disk_space_free, (collected_number)d->percentDiskFree.current.Data);
        rrddim_set_by_pointer(d->st_disk_space, d->rd_disk_space_used, (collected_number)(d->percentDiskFree.current.Time - d->percentDiskFree.current.Data));
        rrdset_done(d->st_disk_space);
    }

    return true;
}

static bool do_physical_disk(PERF_DATA_BLOCK *pDataBlock, int update_every) {
    DICTIONARY *dict = physicalDisks;

    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "PhysicalDisk");
    if(!pObjectType) return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, buffer, sizeof(buffer)))
            strncpyz(buffer, "[unknown]", sizeof(buffer) - 1);

        char *device = buffer;
        char *mount_point = NULL;

        if((mount_point = strchr(device, ' '))) {
            *mount_point = '\0';
            mount_point++;
        }

        struct physical_disk *d;
        bool is_system;
        if (strcasecmp(buffer, "_Total") == 0) {
            d = &system_physical_total;
            is_system = true;
        }
        else {
            d = dictionary_set(dict, device, NULL, sizeof(*d));
            is_system = false;
        }

        if (!d->collected_metadata) {
            // TODO collect metadata - device_type, serial, id
            d->collected_metadata = true;
        }

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->diskReadBytesPerSec) &&
            perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->diskWriteBytesPerSec)) {
            if (!d->st_io) {
                d->st_io = rrdset_create_localhost(
                    is_system ? "system" : "disk_io",
                    is_system ? "io" : device,
                    NULL,
                    is_system ? "disk" : device,
                    is_system ? "system.io" : "disk.io",
                    "Disk I/O Bandwidth",
                    "KiB/s",
                    PLUGIN_WINDOWS_NAME,
                    "PerflibStorage",
                    is_system ? NETDATA_CHART_PRIO_SYSTEM_IO : NETDATA_CHART_PRIO_DISK_IO,
                    update_every,
                    RRDSET_TYPE_AREA);

                if(!is_system) {
                    rrdlabels_add(d->st_io->rrdlabels, "device", device, RRDLABEL_SRC_AUTO);

                    if (mount_point)
                        rrdlabels_add(d->st_io->rrdlabels, "mount_point", mount_point, RRDLABEL_SRC_AUTO);
                }

                d->rd_io_reads = perflib_rrddim_add(d->st_io, "reads", NULL, 1, 1024, &d->diskReadBytesPerSec);
                d->rd_io_writes = perflib_rrddim_add(d->st_io, "writes", NULL, -1, 1024, &d->diskWriteBytesPerSec);
            }

            perflib_rrddim_set_by_pointer(d->st_io, d->rd_io_reads, &d->diskReadBytesPerSec);
            perflib_rrddim_set_by_pointer(d->st_io, d->rd_io_writes, &d->diskWriteBytesPerSec);
            rrdset_done(d->st_io);
        }

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->percentIdleTime);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->percentDiskTime);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->percentDiskReadTime);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->percentDiskWriteTime);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->currentDiskQueueLength);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskQueueLength);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskReadQueueLength);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskWriteQueueLength);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskSecondsPerTransfer);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskSecondsPerRead);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskSecondsPerWrite);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->diskTransfersPerSec);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->diskReadsPerSec);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->diskWritesPerSec);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->diskBytesPerSec);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskBytesPerTransfer);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskBytesPerRead);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->averageDiskBytesPerWrite);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &d->splitIoPerSec);
    }

    return true;
}

int do_PerflibStorage(int update_every, usec_t dt __maybe_unused) {
    static bool initialized = false;

    if(unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("LogicalDisk");
    if(id == REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) return -1;

    do_logical_disk(pDataBlock, update_every);
    do_physical_disk(pDataBlock, update_every);

    return 0;
}
