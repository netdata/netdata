// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PERFLIB_H
#define NETDATA_PERFLIB_H

#include "windows_plugin.h"
#include "windows-internals.h"

const char *RegistryFindNameByID(DWORD id);
const char *RegistryFindHelpByID(DWORD id);

typedef struct _rawdata {
    DWORD CounterType;
    ULONGLONG Data;          // Raw counter data
    LONGLONG Time;           // Is a time value or a base value
    DWORD MultiCounterData;  // Second raw counter value for multi-valued counters
    LONGLONG Frequency;
} RAW_DATA, *PRAW_DATA;

typedef bool (*perflib_data_cb)(PERF_DATA_BLOCK *pDataBlock, void *data);
typedef bool (*perflib_object_cb)(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, void *data);
typedef bool (*perflib_instance_cb)(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, void *data);
typedef bool (*perflib_instance_counter_cb)(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, PERF_COUNTER_DEFINITION *pCounter, RAW_DATA *sample, void *data);
typedef bool (*perflib_counter_cb)(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_COUNTER_DEFINITION *pCounter, RAW_DATA *sample, void *data);

int perflib_query_and_traverse(DWORD id,
                               perflib_data_cb dataCb,
                               perflib_object_cb objectCb,
                               perflib_instance_cb instanceCb,
                               perflib_instance_counter_cb instanceCounterCb,
                               perflib_counter_cb counterCb,
                               void *data);

bool ObjectTypeHasInstances(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType);

BOOL getInstanceName(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance,
                     char *buffer, size_t bufferLen);

PERF_OBJECT_TYPE *getObjectTypeByIndex(PERF_DATA_BLOCK *pDataBlock, DWORD ObjectNameTitleIndex);

PERF_INSTANCE_DEFINITION *getInstanceByPosition(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    DWORD instancePosition);

#endif //NETDATA_PERFLIB_H
