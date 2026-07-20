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
static LPBYTE getPerformanceData(const char *pwszSource, DWORD *bytes_used) {
    static __thread DWORD size = 0;
    static __thread LPBYTE buffer = NULL;

    if(bytes_used)
        *bytes_used = 0;

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
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR, "RegQueryValueEx failed with 0x%x.\n", status);
        return NULL;
    }

    if(bytes_used)
        *bytes_used = size;

    return buffer;
}

void perflibFreePerformanceData(void) {
    getPerformanceData((const char *)0x01, NULL);
}

// --------------------------------------------------------------------------------------------------------------------

// Retrieve the raw counter value and any supporting data needed to calculate
// a displayable counter value. Use the counter type to determine the information
// needed to calculate the value.

ALWAYS_INLINE
static size_t getCounterDataSize(PERF_COUNTER_DEFINITION* pCounter)
{
    switch (pCounter->CounterType) {
        case PERF_COUNTER_COUNTER:
        case PERF_COUNTER_QUEUELEN_TYPE:
        case PERF_SAMPLE_COUNTER:
        case PERF_OBJ_TIME_TIMER:
        case PERF_COUNTER_RAWCOUNT:
        case PERF_COUNTER_RAWCOUNT_HEX:
        case PERF_COUNTER_DELTA:
        case PERF_SAMPLE_FRACTION:
        case PERF_RAW_FRACTION:
            return sizeof(DWORD);

        case PERF_COUNTER_MULTI_TIMER:
        case PERF_COUNTER_MULTI_TIMER_INV:
        case PERF_100NSEC_MULTI_TIMER:
        case PERF_100NSEC_MULTI_TIMER_INV:
            return sizeof(ULONGLONG) +
                ((pCounter->CounterType & PERF_MULTI_COUNTER) == PERF_MULTI_COUNTER ? sizeof(DWORD) : 0);

        case PERF_COUNTER_100NS_QUEUELEN_TYPE:
        case PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE:
        case PERF_COUNTER_TIMER:
        case PERF_COUNTER_TIMER_INV:
        case PERF_COUNTER_BULK_COUNT:
        case PERF_COUNTER_LARGE_QUEUELEN_TYPE:
        case PERF_COUNTER_LARGE_RAWCOUNT:
        case PERF_COUNTER_LARGE_RAWCOUNT_HEX:
        case PERF_COUNTER_LARGE_DELTA:
        case PERF_100NSEC_TIMER:
        case PERF_100NSEC_TIMER_INV:
        case PERF_LARGE_RAW_FRACTION:
        case PERF_PRECISION_SYSTEM_TIMER:
        case PERF_PRECISION_100NS_TIMER:
        case PERF_PRECISION_OBJECT_TIMER:
        case PERF_AVERAGE_TIMER:
        case PERF_AVERAGE_BULK:
        case PERF_ELAPSED_TIME:
            return sizeof(ULONGLONG);

        default:
            return 0;
    }
}

ALWAYS_INLINE
static PVOID getCounterBlockData(
    PERF_COUNTER_BLOCK* pCounterDataBlock,
    PERF_COUNTER_DEFINITION* pCounter,
    size_t size)
{
    if(unlikely(!pCounterDataBlock || !pCounter || !size ||
                 size > pCounterDataBlock->ByteLength ||
                 pCounter->CounterOffset > pCounterDataBlock->ByteLength - size))
        return NULL;

    return (PVOID)((LPBYTE)pCounterDataBlock + pCounter->CounterOffset);
}

ALWAYS_INLINE
static BOOL isObjectSpanValid(PERF_OBJECT_TYPE* pObject, void *ptr, size_t length)
{
    if(!pObject || !ptr || !length)
        return FALSE;

    size_t total_length = pObject->TotalByteLength;
    if(unlikely(length > total_length))
        return FALSE;

    uintptr_t object = (uintptr_t)pObject;
    uintptr_t start = (uintptr_t)ptr;
    if(unlikely(start < object))
        return FALSE;

    size_t offset = start - object;
    return offset <= total_length && length <= total_length - offset;
}

ALWAYS_INLINE
static BOOL isObjectCounterDefinitionValid(PERF_OBJECT_TYPE* pObject, PERF_COUNTER_DEFINITION* pCounter)
{
    if(!pObject || !pCounter)
        return FALSE;

    size_t header_length = pObject->HeaderLength;
    size_t definition_length = pObject->DefinitionLength;
    size_t total_length = pObject->TotalByteLength;
    if(unlikely(header_length > definition_length || definition_length > total_length))
        return FALSE;

    uintptr_t object = (uintptr_t)pObject;
    uintptr_t counter = (uintptr_t)pCounter;
    if(unlikely(counter < object))
        return FALSE;

    size_t counter_offset = counter - object;
    if(unlikely(counter_offset < header_length || counter_offset > definition_length))
        return FALSE;

    size_t remaining = definition_length - counter_offset;
    return sizeof(*pCounter) <= remaining &&
           pCounter->ByteLength >= sizeof(*pCounter) &&
           pCounter->ByteLength <= remaining;
}

ALWAYS_INLINE
static PERF_COUNTER_DEFINITION *getFollowingCounterDefinition(
    PERF_OBJECT_TYPE* pObject,
    PERF_COUNTER_DEFINITION* pCounter)
{
    if(unlikely(!isObjectCounterDefinitionValid(pObject, pCounter)))
        return NULL;

    PBYTE object = (PBYTE)pObject;
    PBYTE end = object + pObject->DefinitionLength;
    PBYTE next = (PBYTE)pCounter + pCounter->ByteLength;
    if(unlikely(sizeof(*pCounter) > (size_t)(end - next)))
        return NULL;

    PERF_COUNTER_DEFINITION *pBaseCounter = (PERF_COUNTER_DEFINITION *)next;
    if(unlikely(!isObjectCounterDefinitionValid(pObject, pBaseCounter)))
        return NULL;

    return pBaseCounter;
}

ALWAYS_INLINE
static PVOID getBaseCounterBlockData(
    PERF_OBJECT_TYPE* pObject,
    PERF_COUNTER_BLOCK* pCounterDataBlock,
    PERF_COUNTER_DEFINITION* pCounter,
    size_t size)
{
    PERF_COUNTER_DEFINITION* pBaseCounter = getFollowingCounterDefinition(pObject, pCounter);
    if(!pBaseCounter || (pBaseCounter->CounterType & PERF_COUNTER_BASE) != PERF_COUNTER_BASE)
        return NULL;

    return getCounterBlockData(pCounterDataBlock, pBaseCounter, size);
}

static BOOL getCounterData(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE* pObject,
    PERF_COUNTER_DEFINITION* pCounter,
    PERF_COUNTER_BLOCK* pCounterDataBlock,
    PRAW_DATA pRawData)
{
    PVOID pData = NULL;
    UNALIGNED ULONGLONG* pullData = NULL;
    BOOL fSuccess = TRUE;

    if(!pCounterDataBlock)
        return FALSE;

    size_t size = getCounterDataSize(pCounter);
    if(size) {
        pData = getCounterBlockData(pCounterDataBlock, pCounter, size);
        if(!pData) {
            pRawData->Data = 0;
            pRawData->Time = 0;
            return FALSE;
        }
    }

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
            pData = getBaseCounterBlockData(pObject, pCounterDataBlock, pCounter, sizeof(DWORD));
            if (pData)
                pRawData->Time = (LONGLONG)(*(DWORD*)pData);
            else
                fSuccess = FALSE;
            break;

        case PERF_LARGE_RAW_FRACTION:
        case PERF_PRECISION_SYSTEM_TIMER:
        case PERF_PRECISION_100NS_TIMER:
        case PERF_PRECISION_OBJECT_TIMER:
            pRawData->Data = *(UNALIGNED ULONGLONG*)pData;
            pData = getBaseCounterBlockData(pObject, pCounterDataBlock, pCounter, sizeof(LONGLONG));
            if (pData)
                pRawData->Time = *(LONGLONG*)pData;
            else
                fSuccess = FALSE;
            break;

        case PERF_AVERAGE_TIMER:
        case PERF_AVERAGE_BULK:
            pRawData->Data = *(UNALIGNED ULONGLONG*)pData;
            pData = getBaseCounterBlockData(pObject, pCounterDataBlock, pCounter, sizeof(DWORD));
            if (pData)
                pRawData->Time = *(DWORD*)pData;
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

ALWAYS_INLINE
static BOOL isValidVariableStructure(
    PERF_DATA_BLOCK *pDataBlock,
    void *ptr,
    size_t minimum_length,
    DWORD *length)
{
    DWORD byte_length;

    if(!length)
        return FALSE;

    if(unlikely(!isValidStructure(pDataBlock, ptr, sizeof(byte_length))))
        return FALSE;

    memcpy(&byte_length, ptr, sizeof(byte_length));
    if(unlikely(byte_length < minimum_length || !isValidStructure(pDataBlock, ptr, byte_length)))
        return FALSE;

    *length = byte_length;
    return TRUE;
}

ALWAYS_INLINE
static BOOL isValidObjectType(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType) {
    DWORD total_byte_length;

    if(unlikely(!isValidVariableStructure(pDataBlock, pObjectType, sizeof(*pObjectType), &total_byte_length) ||
                 pObjectType->HeaderLength < sizeof(*pObjectType) ||
                 pObjectType->HeaderLength > pObjectType->DefinitionLength ||
                 pObjectType->DefinitionLength > total_byte_length))
        return FALSE;

    return TRUE;
}

ALWAYS_INLINE
static BOOL isValidInstanceDefinition(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pInstance)
{
    DWORD byte_length = 0;

    if(unlikely(!isValidVariableStructure(pDataBlock, pInstance, sizeof(*pInstance), &byte_length)))
        return FALSE;

    if(unlikely(pObjectType && !isObjectSpanValid(pObjectType, pInstance, byte_length)))
        return FALSE;

    if(unlikely(pInstance->NameLength > byte_length ||
                pInstance->NameOffset > byte_length - pInstance->NameLength))
        return FALSE;

    // when a name exists, it must live after the fixed header - otherwise
    // header bytes would be decoded as the instance name
    // (NameLength == 0 with NameOffset == 0 is valid for unnamed instances)
    if(unlikely(pInstance->NameLength && pInstance->NameOffset < sizeof(*pInstance)))
        return FALSE;

    return TRUE;
}

ALWAYS_INLINE
static BOOL isValidCounterBlock(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_COUNTER_BLOCK *pCounterBlock)
{
    DWORD byte_length = 0;

    if(unlikely(!isValidVariableStructure(pDataBlock, pCounterBlock, sizeof(*pCounterBlock), &byte_length)))
        return FALSE;

    if(unlikely(pObjectType && !isObjectSpanValid(pObjectType, pCounterBlock, byte_length)))
        return FALSE;

    return TRUE;
}

ALWAYS_INLINE
static BOOL isValidCounterDefinition(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_COUNTER_DEFINITION *pCounterDefinition)
{
    DWORD byte_length = 0;

    if(unlikely(!isValidVariableStructure(pDataBlock, pCounterDefinition, sizeof(*pCounterDefinition), &byte_length)))
        return FALSE;

    if(unlikely(pObjectType && !isObjectCounterDefinitionValid(pObjectType, pCounterDefinition)))
        return FALSE;

    return TRUE;
}

static inline PERF_DATA_BLOCK *getDataBlock(BYTE *pBuffer, DWORD bytes_used) {
    if(unlikely(!pBuffer || bytes_used < sizeof(PERF_DATA_BLOCK))) {
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PERFLIB: Performance data block is too small.");
        return NULL;
    }

    PERF_DATA_BLOCK *pDataBlock = (PERF_DATA_BLOCK *)pBuffer;

    static WCHAR signature[] = { 'P', 'E', 'R', 'F' };

    if(unlikely(pDataBlock->TotalByteLength > bytes_used))
        pDataBlock->TotalByteLength = bytes_used;

    if(unlikely(pDataBlock->TotalByteLength < sizeof(*pDataBlock))) {
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PERFLIB: Invalid data block length.");
        return NULL;
    }

    if(memcmp(pDataBlock->Signature, signature, sizeof(signature)) != 0) {
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PERFLIB: Invalid data block signature.");
        return NULL;
    }

    if(!isValidPointer(pDataBlock, (PBYTE)pDataBlock + pDataBlock->SystemNameOffset) ||
        !isValidStructure(pDataBlock, (PBYTE)pDataBlock + pDataBlock->SystemNameOffset, pDataBlock->SystemNameLength)) {
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
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

    if(pObjectType && (!isValidPointer(pDataBlock, pObjectType) || !isValidObjectType(pDataBlock, pObjectType))) {
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR, "WINDOWS: PERFLIB: %s(): Invalid ObjectType!", __FUNCTION__);
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

    if(pInstance && (!isValidPointer(pDataBlock, pInstance) || !isValidInstanceDefinition(pDataBlock, pObjectType, pInstance))) {
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR, "WINDOWS: PERFLIB: %s(): Invalid Instance Definition!", __FUNCTION__);
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

    if(pCounterBlock && (!isValidPointer(pDataBlock, pCounterBlock) || !isValidCounterBlock(pDataBlock, pObjectType, pCounterBlock))) {
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR, "WINDOWS: PERFLIB: %s(): Invalid ObjectType CounterBlock!", __FUNCTION__);
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

    if(pCounterBlock && (!isValidPointer(pDataBlock, pCounterBlock) || !isValidCounterBlock(pDataBlock, pObjectType, pCounterBlock))) {
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR, "WINDOWS: PERFLIB: %s(): Invalid Instance CounterBlock!", __FUNCTION__);
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
        if(!pi)
            return NULL;

        if(i == instancePosition)
            return pi;

        pc = getInstanceCounterBlock(pDataBlock, pObjectType, pi);
        if(!pc)
            return NULL;
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

    if(pCounterDefinition && (!isValidPointer(pDataBlock, pCounterDefinition) || !isValidCounterDefinition(pDataBlock, pObjectType, pCounterDefinition))) {
        nd_log_limit_static_global_var(erl, 60, 0);
        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR, "WINDOWS: PERFLIB: %s(): Invalid Counter Definition!", __FUNCTION__);
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

    if(unlikely(!dst || !dst_len || dst_len > INT_MAX || length > INT_MAX))
        return FALSE;

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
    if(unlikely(charsCopied > INT_MAX))
        return FALSE;

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
    if (!pObjectType || !pInstance || !buffer || !bufferLen)
        return FALSE;

    if(!isValidInstanceDefinition(pDataBlock, pObjectType, pInstance))
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

// Dedup for the "giving up on metric" log line. One COUNTER_DATA exists per
// (instance x counter), so a counter missing across N instances would otherwise
// emit N identical error lines. We log once per distinct metric key and re-arm
// after the metric recovers.
static SPINLOCK perflib_giveup_spinlock = SPINLOCK_INITIALIZER;
static DICTIONARY *perflib_giveup_logged = NULL;

static bool perflib_giveup_log_should(const char *key) {
    // A non-NULL marker value is required: dictionary_set(..., NULL, 0) stores a
    // NULL value, which dictionary_get() cannot distinguish from "absent".
    static const uint8_t marker = 1;
    bool should = false;
    spinlock_lock(&perflib_giveup_spinlock);
    if(unlikely(!perflib_giveup_logged))
        perflib_giveup_logged = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    if(!dictionary_get(perflib_giveup_logged, key)) {
        dictionary_set(perflib_giveup_logged, key, (void *)&marker, sizeof(marker));
        should = true;
    }
    spinlock_unlock(&perflib_giveup_spinlock);
    return should;
}

// Returns true only for the first caller that clears the key, so the recovery
// notice is logged once per metric key (not once per instance).
static bool perflib_giveup_log_clear(const char *key) {
    bool cleared = false;
    spinlock_lock(&perflib_giveup_spinlock);
    if(perflib_giveup_logged)
        cleared = dictionary_del(perflib_giveup_logged, key);
    spinlock_unlock(&perflib_giveup_spinlock);
    return cleared;
}

// ----------------------------------------------------------------------------
// Park / re-probe accounting, shared by instance- and object-counter lookups.
//
// After PERFLIB_MAX_FAILURES_TO_FIND_METRIC consecutive misses we stop scanning
// counter definitions every cycle, but we re-probe every PERFLIB_REPROBE_INTERVAL
// collections so a transiently-missing counter resumes on its own (no restart).

// Returns true when the metric is parked and still inside its backoff window
// (the caller should skip the search this cycle). On the re-probe cycle it also
// forgets the cached counter id, so a counter that came back under a new
// CounterNameTitleIndex is resolved again by name.
static bool perflib_counter_backoff_skip(COUNTER_DATA *cd) {
    if(likely(cd->failures < PERFLIB_MAX_FAILURES_TO_FIND_METRIC))
        return false;

    // `--cd->backoff` is only evaluated while backoff > 0, so it never underflows;
    // re-probing when it reaches 0 makes the period exactly PERFLIB_REPROBE_INTERVAL.
    if(cd->backoff && --cd->backoff)
        return true;

    cd->id = 0;
    return false;
}

// Record a successful read; if the metric had been parked, log recovery once.
static void perflib_counter_record_success(COUNTER_DATA *cd, bool parked) {
    if(unlikely(parked) && perflib_giveup_log_clear(cd->key))
        nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
               "WINDOWS: PERFLIB: Metric '%s' is available again; resuming collection.", cd->key);
    cd->updated = true;
    cd->failures = 0;
    cd->backoff = 0;
}

// Record a failed read; on the transition to parked, log give-up once per key.
static void perflib_counter_record_failure(COUNTER_DATA *cd, bool parked) {
    if(parked) {
        // the re-probe failed; keep it parked until the next re-probe window
        cd->backoff = PERFLIB_REPROBE_INTERVAL;
        return;
    }

    cd->failures++;
    if(cd->failures >= PERFLIB_MAX_FAILURES_TO_FIND_METRIC) {
        if(perflib_giveup_log_should(cd->key))
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "WINDOWS: PERFLIB: Giving up on metric '%s' after %u attempts; "
                   "will re-probe every %u collections.",
                   cd->key, cd->failures, PERFLIB_REPROBE_INTERVAL);
        cd->backoff = PERFLIB_REPROBE_INTERVAL;
    }
}

// Shared implementation for instance- and object-counter lookups. They differ
// only in how the counter block is located: pass the instance for an instance
// counter, or NULL for an object-level (single-instance) counter.
static bool perflib_get_counter(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, COUNTER_DATA *cd) {
    internal_fatal(cd->key == NULL, "You have to set a key for this call.");

    if(unlikely(!pObjectType))
        goto failed; // missing object (not a missing counter) — do not park on it

    bool parked = cd->failures >= PERFLIB_MAX_FAILURES_TO_FIND_METRIC;
    if(unlikely(perflib_counter_backoff_skip(cd)))
        goto failed;

    DWORD id = cd->id; // read after backoff_skip, which may reset it on a re-probe
    PERF_COUNTER_DEFINITION *pCounterDefinition = NULL;
    for(DWORD c = 0; c < pObjectType->NumCounters ;c++) {
        pCounterDefinition = getCounterDefinition(pDataBlock, pObjectType, pCounterDefinition);
        if(!pCounterDefinition) {
            nd_log_limit_static_global_var(erl, 60, 0);
            nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
                   "WINDOWS: PERFLIB: Cannot read counter definition No %u (out of %u)",
                   c, pObjectType->NumCounters);
            break;
        }

        if(id) {
            if(id != pCounterDefinition->CounterNameTitleIndex)
                continue;
        }
        else {
            if(strcmp(RegistryFindNameByID(pCounterDefinition->CounterNameTitleIndex), cd->key) != 0)
                continue;

            cd->id = pCounterDefinition->CounterNameTitleIndex;
        }

        cd->current.CounterType = cd->OverwriteCounterType ? cd->OverwriteCounterType : pCounterDefinition->CounterType;
        PERF_COUNTER_BLOCK *pCounterBlock = pInstance ?
            getInstanceCounterBlock(pDataBlock, pObjectType, pInstance) :
            getObjectTypeCounterBlock(pDataBlock, pObjectType);

        cd->previous = cd->current;
        if(likely(getCounterData(pDataBlock, pObjectType, pCounterDefinition, pCounterBlock, &cd->current))) {
            perflib_counter_record_success(cd, parked);
            return true;
        }

        // counter matched but was unreadable: stop scanning and count it as a miss
        break;
    }

    perflib_counter_record_failure(cd, parked);

failed:
    cd->previous = cd->current;
    cd->current = RAW_DATA_EMPTY;
    cd->updated = false;
    return false;
}

bool perflibGetInstanceCounter(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance, COUNTER_DATA *cd) {
    return perflib_get_counter(pDataBlock, pObjectType, pInstance, cd);
}

bool perflibGetObjectCounter(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, COUNTER_DATA *cd) {
    return perflib_get_counter(pDataBlock, pObjectType, NULL, cd);
}

PERF_DATA_BLOCK *perflibGetPerformanceData(DWORD id) {
    char source[24];
    snprintfz(source, sizeof(source), "%u", id);

    DWORD bytes_used = 0;
    LPBYTE pData = (LPBYTE)getPerformanceData((id > 0) ? source : NULL, &bytes_used);
    if (!pData) return NULL;

    PERF_DATA_BLOCK *pDataBlock = getDataBlock(pData, bytes_used);
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
            nd_log_limit_static_global_var(erl, 60, 0);
            nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
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
                    nd_log_limit_static_global_var(erl, 60, 0);
                    nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
                           "WINDOWS: PERFLIB: Cannot read Instance No %d (out of %d)",
                           i, pObjectType->NumInstances);
                    break;
                }

                pCounterBlock = getInstanceCounterBlock(pDataBlock, pObjectType, pInstance);
                if(!pCounterBlock) {
                    nd_log_limit_static_global_var(erl, 60, 0);
                    nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
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
                        nd_log_limit_static_global_var(erl, 60, 0);
                        nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
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
                    nd_log_limit_static_global_var(erl, 60, 0);
                    nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_ERR,
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
