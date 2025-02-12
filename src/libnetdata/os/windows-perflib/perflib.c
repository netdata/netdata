// SPDX-License-Identifier: GPL-3.0-or-later

#include "perflib.h"

#if defined(OS_WINDOWS)
// --------------------------------------------------------------------------------

// Retrieve a buffer that contains the specified performance data.
// The pwszSource parameter determines the data that GetRegistryBuffer returns.
//
// Typically, when calling RegQueryValueEx, you can specify zero for the size of the buffer
// and the RegQueryValueEx will set your size variable to the required buffer size. However,
// if the source is "Global" or one or more object index values, you will need to increment
// the buffer size in a loop until RegQueryValueEx does not return ERROR_MORE_DATA.
static LPBYTE getPerformanceData(const char *pwszSource) {
    static __thread DWORD size = 0;
    static __thread LPBYTE buffer = NULL;

    if(pwszSource == (const char *)0x01) {
        freez(buffer);
        buffer = NULL;
        size = 0;
        return NULL;
    }

    if(!size) {
        size = 32 * 1024;
        buffer = mallocz(size);
    }

    LONG status = ERROR_SUCCESS;
    while ((status = RegQueryValueEx(HKEY_PERFORMANCE_DATA, pwszSource,
                                     NULL, NULL, buffer, &size)) == ERROR_MORE_DATA) {
        size *= 2;
        buffer = reallocz(buffer, size);
    }

    if (status != ERROR_SUCCESS) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "RegQueryValueEx failed with 0x%x.\n", status);
        return NULL;
    }

    return buffer;
}

void perflibFreePerformanceData(void) {
    getPerformanceData((const char *)0x01);
}

// --------------------------------------------------------------------------------------------------------------------

// Retrieve the raw counter value and any supporting data needed to calculate
// a displayable counter value. Use the counter type to determine the information
// needed to calculate the value.

static BOOL getCounterData(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE* pObject,
    PERF_COUNTER_DEFINITION* pCounter,
    PERF_COUNTER_BLOCK* pCounterDataBlock,
    PRAW_DATA pRawData)
{
    PVOID pData = NULL;
    UNALIGNED ULONGLONG* pullData = NULL;
    PERF_COUNTER_DEFINITION* pBaseCounter = NULL;
    BOOL fSuccess = TRUE;

    //Point to the raw counter data.
    pData = (PVOID)((LPBYTE)pCounterDataBlock + pCounter->CounterOffset);

    //Now use the PERF_COUNTER_DEFINITION.CounterType value to figure out what
    //other information you need to calculate a displayable value.
    switch (pCounter->CounterType) {

        case PERF_COUNTER_COUNTER:
        case PERF_COUNTER_QUEUELEN_TYPE:
        case PERF_SAMPLE_COUNTER:
            pRawData->Data = (ULONGLONG)(*(DWORD*)pData);
            pRawData->Time = pDataBlock->PerfTime.QuadPart;
            if (PERF_COUNTER_COUNTER == pCounter->CounterType || PERF_SAMPLE_COUNTER == pCounter->CounterType)
                pRawData->Frequency = pDataBlock->PerfFreq.QuadPart;
            break;

        case PERF_OBJ_TIME_TIMER:
            pRawData->Data = (ULONGLONG)(*(DWORD*)pData);
            pRawData->Time = pObject->PerfTime.QuadPart;
            break;

        case PERF_COUNTER_100NS_QUEUELEN_TYPE:
            pRawData->Data = *(UNALIGNED ULONGLONG *)pData;
            pRawData->Time = pDataBlock->PerfTime100nSec.QuadPart;
            break;

        case PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE:
            pRawData->Data = *(UNALIGNED ULONGLONG *)pData;
            pRawData->Time = pObject->PerfTime.QuadPart;
            break;

        case PERF_COUNTER_TIMER:
        case PERF_COUNTER_TIMER_INV:
        case PERF_COUNTER_BULK_COUNT:
        case PERF_COUNTER_LARGE_QUEUELEN_TYPE:
            pullData = (UNALIGNED ULONGLONG *)pData;
            pRawData->Data = *pullData;
            pRawData->Time = pDataBlock->PerfTime.QuadPart;
            if (pCounter->CounterType == PERF_COUNTER_BULK_COUNT)
                pRawData->Frequency = pDataBlock->PerfFreq.QuadPart;
            break;

        case PERF_COUNTER_MULTI_TIMER:
        case PERF_COUNTER_MULTI_TIMER_INV:
            pullData = (UNALIGNED ULONGLONG *)pData;
            pRawData->Data = *pullData;
            pRawData->Frequency = pDataBlock->PerfFreq.QuadPart;
            pRawData->Time = pDataBlock->PerfTime.QuadPart;

            //These counter types have a second counter value that is adjacent to
            //this counter value in the counter data block. The value is needed for
            //the calculation.
            if ((pCounter->CounterType & PERF_MULTI_COUNTER) == PERF_MULTI_COUNTER) {
                ++pullData;
                pRawData->MultiCounterData = *(DWORD*)pullData;
            }
            break;

        //These counters do not use any time reference.
        case PERF_COUNTER_RAWCOUNT:
        case PERF_COUNTER_RAWCOUNT_HEX:
        case PERF_COUNTER_DELTA:
            // some counters in these categories, have CounterSize = sizeof(ULONGLONG)
            // but the official documentation always uses them as sizeof(DWORD)
            pRawData->Data = (ULONGLONG)(*(DWORD*)pData);
            pRawData->Time = 0;
            break;

        case PERF_COUNTER_LARGE_RAWCOUNT:
        case PERF_COUNTER_LARGE_RAWCOUNT_HEX:
        case PERF_COUNTER_LARGE_DELTA:
            pRawData->Data = *(UNALIGNED ULONGLONG*)pData;
            pRawData->Time = 0;
            break;

        //These counters use the 100ns time base in their calculation.
        case PERF_100NSEC_TIMER:
        case PERF_100NSEC_TIMER_INV:
        case PERF_100NSEC_MULTI_TIMER:
        case PERF_100NSEC_MULTI_TIMER_INV:
            pullData = (UNALIGNED ULONGLONG*)pData;
            pRawData->Data = *pullData;
            pRawData->Time = pDataBlock->PerfTime100nSec.QuadPart;

            //These counter types have a second counter value that is adjacent to
            //this counter value in the counter data block. The value is needed for
            //the calculation.
            if ((pCounter->CounterType & PERF_MULTI_COUNTER) == PERF_MULTI_COUNTER) {
                ++pullData;
                pRawData->MultiCounterData = *(DWORD*)pullData;
            }
            break;

        //These counters use two data points, this value and one from this counter's
        //base counter. The base counter should be the next counter in the object's
        //list of counters.
        case PERF_SAMPLE_FRACTION:
        case PERF_RAW_FRACTION:
            pRawData->Data = (ULONGLONG)(*(DWORD*)pData);
            pBaseCounter = pCounter + 1;  //Get base counter
            if ((pBaseCounter->CounterType & PERF_COUNTER_BASE) == PERF_COUNTER_BASE) {
                pData = (PVOID)((LPBYTE)pCounterDataBlock + pBaseCounter->CounterOffset);
                pRawData->Time = (LONGLONG)(*(DWORD*)pData);
            }
            else
                fSuccess = FALSE;
            break;

        case PERF_LARGE_RAW_FRACTION:
        case PERF_PRECISION_SYSTEM_TIMER:
        case PERF_PRECISION_100NS_TIMER:
        case PERF_PRECISION_OBJECT_TIMER:
            pRawData->Data = *(UNALIGNED ULONGLONG*)pData;
            pBaseCounter = pCounter + 1;
            if ((pBaseCounter->CounterType & PERF_COUNTER_BASE) == PERF_COUNTER_BASE) {
                pData = (PVOID)((LPBYTE)pCounterDataBlock + pBaseCounter->CounterOffset);
                pRawData->Time = *(LONGLONG*)pData;
            }
            else
                fSuccess = FALSE;
            break;

        case PERF_AVERAGE_TIMER:
        case PERF_AVERAGE_BULK:
            pRawData->Data = *(UNALIGNED ULONGLONG*)pData;
            pBaseCounter = pCounter+1;
            if ((pBaseCounter->CounterType & PERF_COUNTER_BASE) == PERF_COUNTER_BASE) {
                pData = (PVOID)((LPBYTE)pCounterDataBlock + pBaseCounter->CounterOffset);
                pRawData->Time = *(DWORD*)pData;
            }
            else
                fSuccess = FALSE;

            if (pCounter->CounterType == PERF_AVERAGE_TIMER)
                pRawData->Frequency = pDataBlock->PerfFreq.QuadPart;
            break;

        //These are base counters and are used in calculations for other counters.
        //This case should never be entered.
        case PERF_SAMPLE_BASE:
        case PERF_AVERAGE_BASE:
        case PERF_COUNTER_MULTI_BASE:
        case PERF_RAW_BASE:
        case PERF_LARGE_RAW_BASE:
            pRawData->Data = 0;
            pRawData->Time = 0;
            fSuccess = FALSE;
            break;

        case PERF_ELAPSED_TIME:
            pRawData->Data = *(UNALIGNED ULONGLONG*)pData;
            pRawData->Time = pObject->PerfTime.QuadPart;
            pRawData->Frequency = pObject->PerfFreq.QuadPart;
            break;

        //These counters are currently not supported.
        case PERF_COUNTER_TEXT:
        case PERF_COUNTER_NODATA:
        case PERF_COUNTER_HISTOGRAM_TYPE:
        default: // unknown counter types
            pRawData->Data = 0;
            pRawData->Time = 0;
            fSuccess = FALSE;
            break;
    }

    return fSuccess;
}

// --------------------------------------------------------------------------------------------------------------------

ALWAYS_INLINE
static BOOL isValidPointer(PERF_DATA_BLOCK *pDataBlock, void *ptr) {
    return
        !pDataBlock ||
        (PBYTE)ptr < (PBYTE)pDataBlock ||
        (PBYTE)ptr >= (PBYTE)pDataBlock + pDataBlock->TotalByteLength ? FALSE : TRUE;
}

ALWAYS_INLINE
static BOOL isValidStructure(PERF_DATA_BLOCK *pDataBlock, void *ptr, size_t length) {
    return
        !pDataBlock ||
        !length || length > pDataBlock->TotalByteLength ||
        (PBYTE)ptr < (PBYTE)pDataBlock ||
        (PBYTE)ptr > (PBYTE)ptr + length || // Check for pointer arithmetic overflow
        (PBYTE)ptr + length > (PBYTE)pDataBlock + pDataBlock->TotalByteLength ? FALSE : TRUE;
}

static inline PERF_DATA_BLOCK *getDataBlock(BYTE *pBuffer) {
    PERF_DATA_BLOCK *pDataBlock = (PERF_DATA_BLOCK *)pBuffer;

    static WCHAR signature[] = { 'P', 'E', 'R', 'F' };

    if(memcmp(pDataBlock->Signature, signature, sizeof(signature)) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PERFLIB: Invalid data block signature.");
        return NULL;
    }

    if(!isValidPointer(pDataBlock, (PBYTE)pDataBlock + pDataBlock->SystemNameOffset) ||
        !isValidStructure(pDataBlock, (PBYTE)pDataBlock + pDataBlock->SystemNameOffset, pDataBlock->SystemNameLength)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PERFLIB: Invalid system name array.");
        return NULL;
    }

    return pDataBlock;
}

ALWAYS_INLINE
static PERF_OBJECT_TYPE *getObjectType(PERF_DATA_BLOCK* pDataBlock, PERF_OBJECT_TYPE *lastObjectType) {
    PERF_OBJECT_TYPE* pObjectType = NULL;

    if(!lastObjectType)
        pObjectType = (PERF_OBJECT_TYPE *)((PBYTE)pDataBlock + pDataBlock->HeaderLength);
    else if (lastObjectType->TotalByteLength != 0)
        pObjectType = (PERF_OBJECT_TYPE *)((PBYTE)lastObjectType + lastObjectType->TotalByteLength);

    if(pObjectType && (!isValidPointer(pDataBlock, pObjectType) || !isValidStructure(pDataBlock, pObjectType, pObjectType->TotalByteLength))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "WINDOWS: PERFLIB: %s(): Invalid ObjectType!", __FUNCTION__);
        pObjectType = NULL;
    }

    return pObjectType;
}

ALWAYS_INLINE
PERF_OBJECT_TYPE *getObjectTypeByIndex(PERF_DATA_BLOCK *pDataBlock, DWORD ObjectNameTitleIndex) {
    PERF_OBJECT_TYPE *po = NULL;
    for(DWORD o = 0; o < pDataBlock->NumObjectTypes ; o++) {
        po = getObjectType(pDataBlock, po);
        if(!po) break;

        if(po->ObjectNameTitleIndex == ObjectNameTitleIndex)
            return po;
    }

    return NULL;
}

ALWAYS_INLINE
static PERF_INSTANCE_DEFINITION *getInstance(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_COUNTER_BLOCK *lastCounterBlock
) {
    if(unlikely(!pObjectType))
        return NULL;

    PERF_INSTANCE_DEFINITION *pInstance;
    if(!lastCounterBlock)
        pInstance = (PERF_INSTANCE_DEFINITION *)((PBYTE)pObjectType + pObjectType->DefinitionLength);
    else
        pInstance = (PERF_INSTANCE_DEFINITION *)((PBYTE)lastCounterBlock + lastCounterBlock->ByteLength);

    if(pInstance && (!isValidPointer(pDataBlock, pInstance) || !isValidStructure(pDataBlock, pInstance, pInstance->ByteLength))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "WINDOWS: PERFLIB: %s(): Invalid Instance Definition!", __FUNCTION__);
        pInstance = NULL;
    }

    return pInstance;
}

ALWAYS_INLINE
static PERF_COUNTER_BLOCK *getObjectTypeCounterBlock(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType
) {
    if(unlikely(!pObjectType))
        return NULL;

    PERF_COUNTER_BLOCK *pCounterBlock = (PERF_COUNTER_BLOCK *)((PBYTE)pObjectType + pObjectType->DefinitionLength);

    if(pCounterBlock && (!isValidPointer(pDataBlock, pCounterBlock) || !isValidStructure(pDataBlock, pCounterBlock, pCounterBlock->ByteLength))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "WINDOWS: PERFLIB: %s(): Invalid ObjectType CounterBlock!", __FUNCTION__);
        pCounterBlock = NULL;
    }

    return pCounterBlock;
}

ALWAYS_INLINE
static PERF_COUNTER_BLOCK *getInstanceCounterBlock(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType __maybe_unused,
    PERF_INSTANCE_DEFINITION *pInstance
) {
    if(unlikely(!pInstance))
        return NULL;

    PERF_COUNTER_BLOCK *pCounterBlock = (PERF_COUNTER_BLOCK *)((PBYTE)pInstance + pInstance->ByteLength);

    if(pCounterBlock && (!isValidPointer(pDataBlock, pCounterBlock) || !isValidStructure(pDataBlock, pCounterBlock, pCounterBlock->ByteLength))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "WINDOWS: PERFLIB: %s(): Invalid Instance CounterBlock!", __FUNCTION__);
        pCounterBlock = NULL;
    }

    return pCounterBlock;
}

ALWAYS_INLINE
PERF_INSTANCE_DEFINITION *getInstanceByPosition(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, DWORD instancePosition) {
    PERF_INSTANCE_DEFINITION *pi = NULL;
    PERF_COUNTER_BLOCK *pc = NULL;
    for(DWORD i = 0; i <= instancePosition ;i++) {
        pi = getInstance(pDataBlock, pObjectType, pc);
        pc = getInstanceCounterBlock(pDataBlock, pObjectType, pi);
    }
    return pi;
}

ALWAYS_INLINE
static PERF_COUNTER_DEFINITION *getCounterDefinition(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_COUNTER_DEFINITION *lastCounterDefinition) {
    if(unlikely(!pObjectType))
        return NULL;

    PERF_COUNTER_DEFINITION *pCounterDefinition = NULL;
    if(!lastCounterDefinition)
        pCounterDefinition = (PERF_COUNTER_DEFINITION *)((PBYTE)pObjectType + pObjectType->HeaderLength);
    else
        pCounterDefinition = (PERF_COUNTER_DEFINITION *)((PBYTE)lastCounterDefinition +	lastCounterDefinition->ByteLength);

    if(pCounterDefinition && (!isValidPointer(pDataBlock, pCounterDefinition) || !isValidStructure(pDataBlock, pCounterDefinition, pCounterDefinition->ByteLength))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "WINDOWS: PERFLIB: %s(): Invalid Counter Definition!", __FUNCTION__);
        pCounterDefinition = NULL;
    }

    return pCounterDefinition;
}

// --------------------------------------------------------------------------------------------------------------------

ALWAYS_INLINE
static BOOL getEncodedStringToUTF8(char *dst, size_t dst_len, DWORD CodePage, char *start, DWORD length) {
    static __thread wchar_t unicode[PERFLIB_MAX_NAME_LENGTH];

    WCHAR *tempBuffer;  // Temporary buffer for Unicode data
    DWORD charsCopied = 0;

    if (CodePage == 0) {
        // Input is already Unicode (UTF-16)
        tempBuffer = (WCHAR *)start;
        charsCopied = length / sizeof(WCHAR);  // Convert byte length to number of WCHARs
    }
    else {
        tempBuffer = unicode;
        charsCopied = any_to_utf16(CodePage, unicode, _countof(unicode), start, (int)length, NULL);
        if(!charsCopied) return FALSE;
    }

    // Now convert from Unicode (UTF-16) to UTF-8
    int bytesCopied = WideCharToMultiByte(CP_UTF8, 0, tempBuffer, (int)charsCopied, dst, (int)dst_len, NULL, NULL);
    if (bytesCopied == 0) {
        dst[0] = '\0'; // Ensure the buffer is null-terminated even on failure
        return FALSE;
    }

    dst[bytesCopied - 1] = '\0'; // Ensure buffer is null-terminated
    return TRUE;
}

ALWAYS_INLINE
BOOL getInstanceName(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance,
                     char *buffer, size_t bufferLen) {
    (void)pDataBlock;
    if (!pObjectType || !pInstance || !buffer || !bufferLen)
        return FALSE;

    return getEncodedStringToUTF8(buffer, bufferLen, pObjectType->CodePage,
                                  ((char *)pInstance + pInstance->NameOffset), pInstance->NameLength);
}

ALWAYS_INLINE
BOOL getSystemName(PERF_DATA_BLOCK *pDataBlock, char *buffer, size_t bufferLen) {
    return getEncodedStringToUTF8(buffer, bufferLen, 0,
                                  ((char *)pDataBlock + pDataBlock->SystemNameOffset), pDataBlock->SystemNameLength);
}

ALWAYS_INLINE
BOOL ObjectTypeHasInstances(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType) {
    (void)pDataBlock;
    return pObjectType && pObjectType->NumInstances != PERF_NO_INSTANCES && pObjectType->NumInstances > 0 ? TRUE : FALSE;
}

PERF_OBJECT_TYPE *perflibFindObjectTypeByName(PERF_DATA_BLOCK *pDataBlock, const char *name) {
    PERF_OBJECT_TYPE* pObjectType = NULL;
    for(DWORD o = 0; o < pDataBlock->NumObjectTypes; o++) {
        pObjectType = getObjectType(pDataBlock, pObjectType);
        if(!pObjectType) break;
        if(strcmp(name, RegistryFindNameByID(pObjectType->ObjectNameTitleIndex)) == 0)
            return pObjectType;
    }

    return NULL;
}

PERF_INSTANCE_DEFINITION *perflibForEachInstance(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *lastInstance) {
    if(!ObjectTypeHasInstances(pDataBlock, pObjectType))
        return NULL;

    return getInstance(pDataBlock, pObjectType,
                       lastInstance ?
                           getInstanceCounterBlock(pDataBlock, pObjectType, lastInstance) :
                           NULL );
}

bool perflibGetInstanceCounter(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, COUNTER_DATA *cd) {
    DWORD id = cd->id;
    const char *key = cd->key;
    internal_fatal(key == NULL, "You have to set a key for this call.");

    if(unlikely(cd->failures >= PERFLIB_MAX_FAILURES_TO_FIND_METRIC)) {
        // we don't want to lookup and compare strings all the time
        // when a metric is not there, so we try to find it for
        // XX times, and then we give up.

        if(cd->failures == PERFLIB_MAX_FAILURES_TO_FIND_METRIC) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "WINDOWS: PERFLIB: Giving up on metric '%s' (tried to find it %u times).",
                   cd->key, cd->failures);

            cd->failures++; // increment it once, so that we will not log this again
        }

        goto failed;
    }

    PERF_COUNTER_DEFINITION *pCounterDefinition = NULL;
    for(DWORD c = 0; c < pObjectType->NumCounters ;c++) {
        pCounterDefinition = getCounterDefinition(pDataBlock, pObjectType, pCounterDefinition);
        if(!pCounterDefinition) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "WINDOWS: PERFLIB: Cannot read counter definition No %u (out of %u)",
                   c, pObjectType->NumCounters);
            break;
        }

        if(id) {
            if(id != pCounterDefinition->CounterNameTitleIndex)
                continue;
        }
        else {
            const char *name = RegistryFindNameByID(pCounterDefinition->CounterNameTitleIndex);
            if(strcmp(name, key) != 0)
                continue;

            cd->id = pCounterDefinition->CounterNameTitleIndex;
        }

        cd->current.CounterType = cd->OverwriteCounterType ? cd->OverwriteCounterType : pCounterDefinition->CounterType;
        PERF_COUNTER_BLOCK *pCounterBlock = getInstanceCounterBlock(pDataBlock, pObjectType, pInstance);

        cd->previous = cd->current;
        if(likely(getCounterData(pDataBlock, pObjectType, pCounterDefinition, pCounterBlock, &cd->current))) {
            cd->updated = true;
            cd->failures = 0;
            return true;
        }
    }

    cd->failures++;

failed:
    cd->previous = cd->current;
    cd->current = RAW_DATA_EMPTY;
    cd->updated = false;
    return false;
}

bool perflibGetObjectCounter(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, COUNTER_DATA *cd) {
    if (unlikely(!pObjectType))
        goto cleanup;

    PERF_COUNTER_DEFINITION *pCounterDefinition = NULL;
    for(DWORD c = 0; c < pObjectType->NumCounters ;c++) {
        pCounterDefinition = getCounterDefinition(pDataBlock, pObjectType, pCounterDefinition);
        if(!pCounterDefinition) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "WINDOWS: PERFLIB: Cannot read counter definition No %u (out of %u)",
                   c, pObjectType->NumCounters);
            break;
        }

        if(cd->id) {
            if(cd->id != pCounterDefinition->CounterNameTitleIndex)
                continue;
        }
        else {
            if(strcmp(RegistryFindNameByID(pCounterDefinition->CounterNameTitleIndex), cd->key) != 0)
                continue;

            cd->id = pCounterDefinition->CounterNameTitleIndex;
        }

        cd->current.CounterType = cd->OverwriteCounterType ? cd->OverwriteCounterType : pCounterDefinition->CounterType;
        PERF_COUNTER_BLOCK *pCounterBlock = getObjectTypeCounterBlock(pDataBlock, pObjectType);

        cd->previous = cd->current;
        cd->updated = getCounterData(pDataBlock, pObjectType, pCounterDefinition, pCounterBlock, &cd->current);
        return cd->updated;
    }

cleanup:
    cd->previous = cd->current;
    cd->current = RAW_DATA_EMPTY;
    cd->updated = false;
    return false;
}

PERF_DATA_BLOCK *perflibGetPerformanceData(DWORD id) {
    char source[24];
    snprintfz(source, sizeof(source), "%u", id);

    LPBYTE pData = (LPBYTE)getPerformanceData((id > 0) ? source : NULL);
    if (!pData) return NULL;

    PERF_DATA_BLOCK *pDataBlock = getDataBlock(pData);
    if(!pDataBlock) return NULL;

    return pDataBlock;
}

int perflibQueryAndTraverse(DWORD id,
                               perflib_data_cb dataCb,
                               perflib_object_cb objectCb,
                               perflib_instance_cb instanceCb,
                               perflib_instance_counter_cb instanceCounterCb,
                               perflib_counter_cb counterCb,
                               void *data) {
    int counters = -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if(!pDataBlock) goto cleanup;

    bool do_data = true;
    if(dataCb)
        do_data = dataCb(pDataBlock, data);

    PERF_OBJECT_TYPE* pObjectType = NULL;
    for(DWORD o = 0; do_data && o < pDataBlock->NumObjectTypes; o++) {
        pObjectType = getObjectType(pDataBlock, pObjectType);
        if(!pObjectType) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "WINDOWS: PERFLIB: Cannot read object type No %d (out of %d)",
                   o, pDataBlock->NumObjectTypes);
            break;
        }

        bool do_object = true;
        if(objectCb)
            do_object = objectCb(pDataBlock, pObjectType, data);

        if(!do_object)
            continue;

        if(ObjectTypeHasInstances(pDataBlock, pObjectType)) {
            PERF_INSTANCE_DEFINITION *pInstance = NULL;
            PERF_COUNTER_BLOCK *pCounterBlock = NULL;
            for(LONG i = 0; i < pObjectType->NumInstances ;i++) {
                pInstance = getInstance(pDataBlock, pObjectType, pCounterBlock);
                if(!pInstance) {
                    nd_log(NDLS_COLLECTORS, NDLP_ERR,
                           "WINDOWS: PERFLIB: Cannot read Instance No %d (out of %d)",
                           i, pObjectType->NumInstances);
                    break;
                }

                pCounterBlock = getInstanceCounterBlock(pDataBlock, pObjectType, pInstance);
                if(!pCounterBlock) {
                    nd_log(NDLS_COLLECTORS, NDLP_ERR,
                           "WINDOWS: PERFLIB: Cannot read CounterBlock of instance No %d (out of %d)",
                           i, pObjectType->NumInstances);
                    break;
                }

                bool do_instance = true;
                if(instanceCb)
                    do_instance = instanceCb(pDataBlock, pObjectType, pInstance, data);

                if(!do_instance)
                    continue;

                PERF_COUNTER_DEFINITION *pCounterDefinition = NULL;
                for(DWORD c = 0; c < pObjectType->NumCounters ;c++) {
                    pCounterDefinition = getCounterDefinition(pDataBlock, pObjectType, pCounterDefinition);
                    if(!pCounterDefinition) {
                        nd_log(NDLS_COLLECTORS, NDLP_ERR,
                               "WINDOWS: PERFLIB: Cannot read counter definition No %u (out of %u)",
                               c, pObjectType->NumCounters);
                        break;
                    }

                    RAW_DATA sample = {
                        .CounterType = pCounterDefinition->CounterType,
                    };
                    if(getCounterData(pDataBlock, pObjectType, pCounterDefinition, pCounterBlock, &sample)) {
                        // DisplayCalculatedValue(&sample, &sample);

                        if(instanceCounterCb) {
                            instanceCounterCb(pDataBlock, pObjectType, pInstance, pCounterDefinition, &sample, data);
                            counters++;
                        }
                    }
                }

                if(instanceCb)
                    instanceCb(pDataBlock, pObjectType, NULL, data);
            }
        }
        else {
            PERF_COUNTER_BLOCK *pCounterBlock = getObjectTypeCounterBlock(pDataBlock, pObjectType);
            PERF_COUNTER_DEFINITION *pCounterDefinition = NULL;
            for(DWORD c = 0; c < pObjectType->NumCounters ;c++) {
                pCounterDefinition = getCounterDefinition(pDataBlock, pObjectType, pCounterDefinition);
                if(!pCounterDefinition) {
                    nd_log(NDLS_COLLECTORS, NDLP_ERR,
                           "WINDOWS: PERFLIB: Cannot read counter definition No %u (out of %u)",
                           c, pObjectType->NumCounters);
                    break;
                }

                RAW_DATA sample = {
                    .CounterType = pCounterDefinition->CounterType,
                };
                if(getCounterData(pDataBlock, pObjectType, pCounterDefinition, pCounterBlock, &sample)) {
                    // DisplayCalculatedValue(&sample, &sample);

                    if(counterCb) {
                        counterCb(pDataBlock, pObjectType, pCounterDefinition, &sample, data);
                        counters++;
                    }
                }
            }
        }

        if(objectCb)
            objectCb(pDataBlock, NULL, data);
    }

cleanup:
    return counters;
}

#endif // OS_WINDOWS