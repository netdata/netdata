// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PERFLIB_H
#define NETDATA_PERFLIB_H

#include "libnetdata/libnetdata.h"

#if defined(OS_WINDOWS)

typedef uint32_t DWORD;
typedef long long  LONGLONG;
typedef unsigned long long ULONGLONG;
typedef int BOOL;

struct _PERF_DATA_BLOCK;
typedef struct _PERF_DATA_BLOCK PERF_DATA_BLOCK;
struct _PERF_OBJECT_TYPE;
typedef struct _PERF_OBJECT_TYPE PERF_OBJECT_TYPE;
struct _PERF_INSTANCE_DEFINITION;
typedef struct _PERF_INSTANCE_DEFINITION PERF_INSTANCE_DEFINITION;
struct _PERF_COUNTER_DEFINITION;
typedef struct _PERF_COUNTER_DEFINITION PERF_COUNTER_DEFINITION;

const char *RegistryFindNameByID(DWORD id);
const char *RegistryFindHelpByID(DWORD id);
DWORD RegistryFindIDByName(const char *name);
#define PERFLIB_REGISTRY_NAME_NOT_FOUND (DWORD)-1
#define PERFLIB_MAX_NAME_LENGTH 1024

PERF_DATA_BLOCK *perflibGetPerformanceData(DWORD id);
void perflibFreePerformanceData(void);
PERF_OBJECT_TYPE *perflibFindObjectTypeByName(PERF_DATA_BLOCK *pDataBlock, const char *name);
PERF_INSTANCE_DEFINITION *perflibForEachInstance(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *lastInstance);

typedef struct _rawdata {
    DWORD CounterType;
    DWORD MultiCounterData;  // Second raw counter value for multi-valued counters
    ULONGLONG Data;          // Raw counter data
    LONGLONG Time;           // Is a time value or a base value
    LONGLONG Frequency;
} RAW_DATA, *PRAW_DATA;

typedef struct _counterdata {
    DWORD id;
    bool updated;
    uint8_t failures;           // counts the number of failures to find this key
    const char *key;
    DWORD OverwriteCounterType; // if set, the counter type will be overwritten once read
    RAW_DATA current;
    RAW_DATA previous;
} COUNTER_DATA;

#define PERFLIB_MAX_FAILURES_TO_FIND_METRIC 10

#define RAW_DATA_EMPTY (RAW_DATA){ 0 }

bool perflibGetInstanceCounter(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, COUNTER_DATA *cd);
bool perflibGetObjectCounter(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, COUNTER_DATA *cd);

typedef bool (*perflib_data_cb)(PERF_DATA_BLOCK *pDataBlock, void *data);
typedef bool (*perflib_object_cb)(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, void *data);
typedef bool (*perflib_instance_cb)(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, void *data);
typedef bool (*perflib_instance_counter_cb)(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, PERF_COUNTER_DEFINITION *pCounter, RAW_DATA *sample, void *data);
typedef bool (*perflib_counter_cb)(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_COUNTER_DEFINITION *pCounter, RAW_DATA *sample, void *data);

int perflibQueryAndTraverse(DWORD id,
                               perflib_data_cb dataCb,
                               perflib_object_cb objectCb,
                               perflib_instance_cb instanceCb,
                               perflib_instance_counter_cb instanceCounterCb,
                               perflib_counter_cb counterCb,
                               void *data);

BOOL ObjectTypeHasInstances(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType);

BOOL getInstanceName(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance,
                     char *buffer, size_t bufferLen);

BOOL getSystemName(PERF_DATA_BLOCK *pDataBlock, char *buffer, size_t bufferLen);

PERF_OBJECT_TYPE *getObjectTypeByIndex(PERF_DATA_BLOCK *pDataBlock, DWORD ObjectNameTitleIndex);

PERF_INSTANCE_DEFINITION *getInstanceByPosition(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    DWORD instancePosition);

void PerflibNamesRegistryInitialize(void);
void PerflibNamesRegistryUpdate(void);

#endif // OS_WINDOWS
#endif //NETDATA_PERFLIB_H
