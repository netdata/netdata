// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct LogicalDisk {
    DWORD id;
    STRING *name;
    DICTIONARY *counters;
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


        }
    }

    PERF_OBJECT_TYPE *pPhysicalDisk = perflibFindObjectTypeByName(pDataBlock, "PhysicalDisk");
    if(pPhysicalDisk) {

    }

    return 0;
}
