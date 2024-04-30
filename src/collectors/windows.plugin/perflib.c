// SPDX-License-Identifier: GPL-3.0-or-later

#include "perflib.h"
#include "windows-internals.h"

#include <windows.h>
#include <string.h>
#include <wchar.h>
#include <stdlib.h>
#include <strsafe.h>
#include <stdio.h>

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
            pRawData->Data = 0;
            pRawData->Time = 0;
            fSuccess = FALSE;
            break;

        //Encountered an unidentified counter.
        default:
            pRawData->Data = 0;
            pRawData->Time = 0;
            fSuccess = FALSE;
            break;
    }

    return fSuccess;
}

#define COLLECTED_NUMBER_PRECISION 10000

RRDDIM *perflib_rrddim_add(RRDSET *st, const char *id, const char *name, collected_number multiplier, collected_number divider, COUNTER_DATA *cd) {
    RRD_ALGORITHM algorithm = RRD_ALGORITHM_ABSOLUTE;

    switch (cd->current.CounterType) {
        case PERF_COUNTER_COUNTER:
        case PERF_SAMPLE_COUNTER:
        case PERF_COUNTER_BULK_COUNT:
            // (N1 - N0) / ((D1 - D0) / F)
            // multiplier *= cd->current.Frequency / 10000000;
            // tested, the frequency is not that useful for netdata
            // we get right results without it.
            algorithm = RRD_ALGORITHM_INCREMENTAL;
            break;

        case PERF_COUNTER_QUEUELEN_TYPE:
        case PERF_COUNTER_100NS_QUEUELEN_TYPE:
        case PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE:
        case PERF_COUNTER_LARGE_QUEUELEN_TYPE:
        case PERF_AVERAGE_BULK:  // normally not displayed
            // (N1 - N0) / (D1 - D0)
            algorithm = RRD_ALGORITHM_INCREMENTAL;
            break;

        case PERF_OBJ_TIME_TIMER:
        case PERF_COUNTER_TIMER:
        case PERF_100NSEC_TIMER:
        case PERF_PRECISION_SYSTEM_TIMER:
        case PERF_PRECISION_100NS_TIMER:
        case PERF_PRECISION_OBJECT_TIMER:
        case PERF_SAMPLE_FRACTION:
            // 100 * (N1 - N0) / (D1 - D0)
            multiplier *= 100;
            algorithm = RRD_ALGORITHM_INCREMENTAL;
            break;

        case PERF_COUNTER_TIMER_INV:
        case PERF_100NSEC_TIMER_INV:
            // 100 * (1 - ((N1 - N0) / (D1 - D0)))
            divider *= COLLECTED_NUMBER_PRECISION;
            algorithm = RRD_ALGORITHM_ABSOLUTE;
            break;

        case PERF_COUNTER_MULTI_TIMER:
            // 100 * ((N1 - N0) / ((D1 - D0) / TB)) / B1
            divider *= COLLECTED_NUMBER_PRECISION;
            algorithm = RRD_ALGORITHM_ABSOLUTE;
            break;

        case PERF_100NSEC_MULTI_TIMER:
            // 100 * ((N1 - N0) / (D1 - D0)) / B1
            divider *= COLLECTED_NUMBER_PRECISION;
            algorithm = RRD_ALGORITHM_ABSOLUTE;
            break;

        case PERF_COUNTER_MULTI_TIMER_INV:
        case PERF_100NSEC_MULTI_TIMER_INV:
            // 100 * (B1 - ((N1 - N0) / (D1 - D0)))
            divider *= COLLECTED_NUMBER_PRECISION;
            algorithm = RRD_ALGORITHM_ABSOLUTE;
            break;

        case PERF_COUNTER_RAWCOUNT:
        case PERF_COUNTER_LARGE_RAWCOUNT:
            // N as decimal
            algorithm = RRD_ALGORITHM_ABSOLUTE;
            break;

        case PERF_COUNTER_RAWCOUNT_HEX:
        case PERF_COUNTER_LARGE_RAWCOUNT_HEX:
            // N as hexadecimal
            algorithm = RRD_ALGORITHM_ABSOLUTE;
            break;

        case PERF_COUNTER_DELTA:
        case PERF_COUNTER_LARGE_DELTA:
            // N1 - N0
            algorithm = RRD_ALGORITHM_ABSOLUTE;
            break;

        case PERF_RAW_FRACTION:
        case PERF_LARGE_RAW_FRACTION:
            // 100 * N / B
            algorithm = RRD_ALGORITHM_ABSOLUTE;
            divider *= COLLECTED_NUMBER_PRECISION;
            break;

        case PERF_AVERAGE_TIMER:
            // ((N1 - N0) / TB) / (B1 - B0)
            // divider *= cd->current.Frequency / 10000000;
            algorithm = RRD_ALGORITHM_INCREMENTAL;
            break;

        case PERF_ELAPSED_TIME:
            // (D0 - N0) / F
            algorithm = RRD_ALGORITHM_ABSOLUTE;
            break;

        case PERF_COUNTER_TEXT:
        case PERF_SAMPLE_BASE:
        case PERF_AVERAGE_BASE:
        case PERF_COUNTER_MULTI_BASE:
        case PERF_RAW_BASE:
        case PERF_COUNTER_NODATA:
        case PERF_PRECISION_TIMESTAMP:
        default:
            break;
    }

    return rrddim_add(st, id, name, multiplier, divider, algorithm);
}

#define VALID_DELTA(cd) \
    ((cd)->previous.Time > 0 && (cd)->current.Data >= (cd)->previous.Data && (cd)->current.Time > (cd)->previous.Time)

collected_number perflib_rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, COUNTER_DATA *cd) {
    ULONGLONG numerator = 0;
    LONGLONG denominator = 0;
    double doubleValue = 0.0;
    collected_number value;

    switch(cd->current.CounterType) {
        case PERF_COUNTER_COUNTER:
        case PERF_SAMPLE_COUNTER:
        case PERF_COUNTER_BULK_COUNT:
            // (N1 - N0) / ((D1 - D0) / F)
            value = (collected_number)cd->current.Data;
            break;

        case PERF_COUNTER_QUEUELEN_TYPE:
        case PERF_COUNTER_100NS_QUEUELEN_TYPE:
        case PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE:
        case PERF_COUNTER_LARGE_QUEUELEN_TYPE:
        case PERF_AVERAGE_BULK:  // normally not displayed
            // (N1 - N0) / (D1 - D0)
            value = (collected_number)cd->current.Data;
            break;

        case PERF_OBJ_TIME_TIMER:
        case PERF_COUNTER_TIMER:
        case PERF_100NSEC_TIMER:
        case PERF_PRECISION_SYSTEM_TIMER:
        case PERF_PRECISION_100NS_TIMER:
        case PERF_PRECISION_OBJECT_TIMER:
        case PERF_SAMPLE_FRACTION:
            // 100 * (N1 - N0) / (D1 - D0)
            value = (collected_number)cd->current.Data;
            break;

        case PERF_COUNTER_TIMER_INV:
        case PERF_100NSEC_TIMER_INV:
            // 100 * (1 - ((N1 - N0) / (D1 - D0)))
            if(!VALID_DELTA(cd)) return 0;
            numerator = cd->current.Data - cd->previous.Data;
            denominator = cd->current.Time - cd->previous.Time;
            doubleValue = 100.0 * (1.0 - ((double)numerator / (double)denominator));
            // printf("Display value is (timer-inv): %f%%\n", doubleValue);
            value = (collected_number)(doubleValue * COLLECTED_NUMBER_PRECISION);
            break;

        case PERF_COUNTER_MULTI_TIMER:
            // 100 * ((N1 - N0) / ((D1 - D0) / TB)) / B1
            if(!VALID_DELTA(cd)) return 0;
            numerator = cd->current.Data - cd->previous.Data;
            denominator = cd->current.Time - cd->previous.Time;
            denominator /= cd->current.Frequency;
            doubleValue = 100.0 * ((double)numerator / (double)denominator) / cd->current.MultiCounterData;
            // printf("Display value is (multi-timer): %f%%\n", doubleValue);
            value = (collected_number)(doubleValue * COLLECTED_NUMBER_PRECISION);
            break;

        case PERF_100NSEC_MULTI_TIMER:
            // 100 * ((N1 - N0) / (D1 - D0)) / B1
            if(!VALID_DELTA(cd)) return 0;
            numerator = cd->current.Data - cd->previous.Data;
            denominator = cd->current.Time - cd->previous.Time;
            doubleValue = 100.0 * ((double)numerator / (double)denominator) / (double)cd->current.MultiCounterData;
            // printf("Display value is (100ns multi-timer): %f%%\n", doubleValue);
            value = (collected_number)(doubleValue * COLLECTED_NUMBER_PRECISION);
            break;

        case PERF_COUNTER_MULTI_TIMER_INV:
        case PERF_100NSEC_MULTI_TIMER_INV:
            // 100 * (B1 - ((N1 - N0) / (D1 - D0)))
            if(!VALID_DELTA(cd)) return 0;
            numerator = cd->current.Data - cd->previous.Data;
            denominator = cd->current.Time - cd->previous.Time;
            doubleValue = 100.0 * ((double)cd->current.MultiCounterData - ((double)numerator / (double)denominator));
            // printf("Display value is (multi-timer-inv): %f%%\n", doubleValue);
            value = (collected_number)(doubleValue * COLLECTED_NUMBER_PRECISION);
            break;

        case PERF_COUNTER_RAWCOUNT:
        case PERF_COUNTER_LARGE_RAWCOUNT:
            // N as decimal
            value = (collected_number)cd->current.Data;
            break;

        case PERF_COUNTER_RAWCOUNT_HEX:
        case PERF_COUNTER_LARGE_RAWCOUNT_HEX:
            // N as hexadecimal
            value = (collected_number)cd->current.Data;
            break;

        case PERF_COUNTER_DELTA:
        case PERF_COUNTER_LARGE_DELTA:
            if(!VALID_DELTA(cd)) return 0;
            value = (collected_number)(cd->current.Data - cd->previous.Data);
            break;

        case PERF_RAW_FRACTION:
        case PERF_LARGE_RAW_FRACTION:
            // 100 * N / B
            if(!cd->current.Time) return 0;
            doubleValue = 100.0 * (double)cd->current.Data / (double)cd->current.Time;
            // printf("Display value is (fraction): %f%%\n", doubleValue);
            value = (collected_number)(doubleValue * COLLECTED_NUMBER_PRECISION);
            break;

        default:
            return 0;
    }

    return rrddim_set_by_pointer(st, rd, value);
}

/*
double perflibCalculateValue(RAW_DATA *current, RAW_DATA *previous) {
    ULONGLONG numerator = 0;
    LONGLONG denominator = 0;
    double doubleValue = 0.0;
    DWORD dwordValue = 0;

    if (NULL == previous) {
        // Return error if the counter type requires two samples to calculate the value.
        switch (current->CounterType) {
            default:
                if (PERF_DELTA_COUNTER != (current->CounterType & PERF_DELTA_COUNTER))
                    break;
                __fallthrough;
                // fallthrough

            case PERF_AVERAGE_TIMER: // Special case.
            case PERF_AVERAGE_BULK:  // Special case.
                // printf(" > The counter type requires two samples but only one sample was provided.\n");
                return NAN;
        }
    }
    else {
        if (current->CounterType != previous->CounterType) {
            // printf(" > The samples have inconsistent counter types.\n");
            return NAN;
        }

        // Check for integer overflow or bad data from provider (the data from
        // sample 2 must be greater than the data from sample 1).
        if (current->Data < previous->Data)
        {
            // Can happen for various reasons. Commonly occurs with the Process counterset when
            // multiple processes have the same name and one of them starts or stops.
            // Normally you'll just drop the older sample and continue.
            // printf("> current (%llu) is smaller than previous (%llu).\n", current->Data, previous->Data);
            return NAN;
        }
    }

    switch (current->CounterType) {
        case PERF_COUNTER_COUNTER:
        case PERF_SAMPLE_COUNTER:
        case PERF_COUNTER_BULK_COUNT:
            // (N1 - N0) / ((D1 - D0) / F)
            numerator = current->Data - previous->Data;
            denominator = current->Time - previous->Time;
            dwordValue = (DWORD)(numerator / ((double)denominator / current->Frequency));
            //printf("Display value is (counter): %lu%s\n", (unsigned long)dwordValue,
            //       (previous->CounterType == PERF_SAMPLE_COUNTER) ? "" : "/sec");
            return (double)dwordValue;

        case PERF_COUNTER_QUEUELEN_TYPE:
        case PERF_COUNTER_100NS_QUEUELEN_TYPE:
        case PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE:
        case PERF_COUNTER_LARGE_QUEUELEN_TYPE:
        case PERF_AVERAGE_BULK:  // normally not displayed
            // (N1 - N0) / (D1 - D0)
            numerator = current->Data - previous->Data;
            denominator = current->Time - previous->Time;
            doubleValue = (double)numerator / denominator;
            if (previous->CounterType != PERF_AVERAGE_BULK) {
                // printf("Display value is (queuelen): %f\n", doubleValue);
                return doubleValue;
            }
            return NAN;

        case PERF_OBJ_TIME_TIMER:
        case PERF_COUNTER_TIMER:
        case PERF_100NSEC_TIMER:
        case PERF_PRECISION_SYSTEM_TIMER:
        case PERF_PRECISION_100NS_TIMER:
        case PERF_PRECISION_OBJECT_TIMER:
        case PERF_SAMPLE_FRACTION:
            // 100 * (N1 - N0) / (D1 - D0)
            numerator = current->Data - previous->Data;
            denominator = current->Time - previous->Time;
            doubleValue = (double)(100 * numerator) / denominator;
            // printf("Display value is (timer): %f%%\n", doubleValue);
            return doubleValue;

        case PERF_COUNTER_TIMER_INV:
            // 100 * (1 - ((N1 - N0) / (D1 - D0)))
            numerator = current->Data - previous->Data;
            denominator = current->Time - previous->Time;
            doubleValue = 100 * (1 - ((double)numerator / denominator));
            // printf("Display value is (timer-inv): %f%%\n", doubleValue);
            return doubleValue;

        case PERF_100NSEC_TIMER_INV:
            // 100 * (1- (N1 - N0) / (D1 - D0))
            numerator = current->Data - previous->Data;
            denominator = current->Time - previous->Time;
            doubleValue = 100 * (1 - (double)numerator / denominator);
            // printf("Display value is (100ns-timer-inv): %f%%\n", doubleValue);
            return doubleValue;

        case PERF_COUNTER_MULTI_TIMER:
            // 100 * ((N1 - N0) / ((D1 - D0) / TB)) / B1
            numerator = current->Data - previous->Data;
            denominator = current->Time - previous->Time;
            denominator /= current->Frequency;
            doubleValue = 100 * ((double)numerator / denominator) / current->MultiCounterData;
            // printf("Display value is (multi-timer): %f%%\n", doubleValue);
            return doubleValue;

        case PERF_100NSEC_MULTI_TIMER:
            // 100 * ((N1 - N0) / (D1 - D0)) / B1
            numerator = current->Data - previous->Data;
            denominator = current->Time - previous->Time;
            doubleValue = 100 * ((double)numerator / (double)denominator) / (double)current->MultiCounterData;
            // printf("Display value is (100ns multi-timer): %f%%\n", doubleValue);
            return doubleValue;

        case PERF_COUNTER_MULTI_TIMER_INV:
        case PERF_100NSEC_MULTI_TIMER_INV:
            // 100 * (B1 - ((N1 - N0) / (D1 - D0)))
            numerator = current->Data - previous->Data;
            denominator = current->Time - previous->Time;
            doubleValue = 100.0 * ((double)current->MultiCounterData - ((double)numerator / (double)denominator));
            // printf("Display value is (multi-timer-inv): %f%%\n", doubleValue);
            return doubleValue;

        case PERF_COUNTER_RAWCOUNT:
        case PERF_COUNTER_LARGE_RAWCOUNT:
            // N as decimal
            // printf("Display value is (rawcount): %llu\n", current->Data);
            return (double)current->Data;

        case PERF_COUNTER_RAWCOUNT_HEX:
        case PERF_COUNTER_LARGE_RAWCOUNT_HEX:
            // N as hexadecimal
            // printf("Display value is (hex): 0x%llx\n", current->Data);
            return (double)current->Data;

        case PERF_COUNTER_DELTA:
        case PERF_COUNTER_LARGE_DELTA:
            // N1 - N0
            // printf("Display value is (delta): %llu\n", current->Data - previous->Data);
            return (double)(current->Data - previous->Data);

        case PERF_RAW_FRACTION:
        case PERF_LARGE_RAW_FRACTION:
            // 100 * N / B
            doubleValue = 100.0 * (double)current->Data / (double)current->Time;
            // printf("Display value is (fraction): %f%%\n", doubleValue);
            return doubleValue;

        case PERF_AVERAGE_TIMER:
            // ((N1 - N0) / TB) / (B1 - B0)
            numerator = current->Data - previous->Data;
            denominator = current->Time - previous->Time;
            doubleValue = (double)numerator / (double)current->Frequency / (double)denominator;
            // printf("Display value is (average timer): %f seconds\n", doubleValue);
            return doubleValue;

        case PERF_ELAPSED_TIME:
            // (D0 - N0) / F
            doubleValue = (double)(current->Time - current->Data) / (double)current->Frequency;
            // printf("Display value is (elapsed time): %f seconds\n", doubleValue);
            return doubleValue;

        case PERF_COUNTER_TEXT:
        case PERF_SAMPLE_BASE:
        case PERF_AVERAGE_BASE:
        case PERF_COUNTER_MULTI_BASE:
        case PERF_RAW_BASE:
        case PERF_COUNTER_NODATA:
        case PERF_PRECISION_TIMESTAMP:
            // printf(" > Non-printing counter type: 0x%08x\n", current->CounterType);
            return NAN;
            break;

        default:
            // printf(" > Unrecognized counter type: 0x%08x\n", current->CounterType);
            return NAN;
            break;
    }
}
*/

// --------------------------------------------------------------------------------------------------------------------

static inline BOOL isValidPointer(PERF_DATA_BLOCK *pDataBlock __maybe_unused, void *ptr __maybe_unused) {
#ifdef NETDATA_INTERNAL_CHECKS
    return (PBYTE)ptr >= (PBYTE)pDataBlock + pDataBlock->TotalByteLength ? FALSE : TRUE;
#else
    return TRUE;
#endif
}

static inline BOOL isValidStructure(PERF_DATA_BLOCK *pDataBlock __maybe_unused, void *ptr __maybe_unused, size_t length __maybe_unused) {
#ifdef NETDATA_INTERNAL_CHECKS
    return (PBYTE)ptr + length > (PBYTE)pDataBlock + pDataBlock->TotalByteLength ? FALSE : TRUE;
#else
    return TRUE;
#endif
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

static inline PERF_OBJECT_TYPE *getObjectType(PERF_DATA_BLOCK* pDataBlock, PERF_OBJECT_TYPE *lastObjectType) {
    PERF_OBJECT_TYPE* pObjectType = NULL;

    if(unlikely(!lastObjectType))
        pObjectType = (PERF_OBJECT_TYPE *)((PBYTE)pDataBlock + pDataBlock->HeaderLength);
    else if (likely(lastObjectType->TotalByteLength != 0))
        pObjectType = (PERF_OBJECT_TYPE *)((PBYTE)lastObjectType + lastObjectType->TotalByteLength);

    if(unlikely(pObjectType && (!isValidPointer(pDataBlock, pObjectType) || !isValidStructure(pDataBlock, pObjectType, pObjectType->TotalByteLength)))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PERFLIB: Invalid ObjectType!");
        pObjectType = NULL;
    }

    return pObjectType;
}

inline PERF_OBJECT_TYPE *getObjectTypeByIndex(PERF_DATA_BLOCK *pDataBlock, DWORD ObjectNameTitleIndex) {
    PERF_OBJECT_TYPE *po = NULL;
    for(DWORD o = 0; o < pDataBlock->NumObjectTypes ; o++) {
        po = getObjectType(pDataBlock, po);
        if(unlikely(po->ObjectNameTitleIndex == ObjectNameTitleIndex))
            return po;
    }

    return NULL;
}

static inline PERF_INSTANCE_DEFINITION *getInstance(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_COUNTER_BLOCK *lastCounterBlock
) {
    PERF_INSTANCE_DEFINITION *pInstance;

    if(unlikely(!lastCounterBlock))
        pInstance = (PERF_INSTANCE_DEFINITION *)((PBYTE)pObjectType + pObjectType->DefinitionLength);
    else
        pInstance = (PERF_INSTANCE_DEFINITION *)((PBYTE)lastCounterBlock + lastCounterBlock->ByteLength);

    if(unlikely(pInstance && (!isValidPointer(pDataBlock, pInstance) || !isValidStructure(pDataBlock, pInstance, pInstance->ByteLength)))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PERFLIB: Invalid Instance Definition!");
        pInstance = NULL;
    }

    return pInstance;
}

static inline PERF_COUNTER_BLOCK *getObjectTypeCounterBlock(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType
) {
    PERF_COUNTER_BLOCK *pCounterBlock = (PERF_COUNTER_BLOCK *)((PBYTE)pObjectType + pObjectType->DefinitionLength);

    if(unlikely(pCounterBlock && (!isValidPointer(pDataBlock, pCounterBlock) || !isValidStructure(pDataBlock, pCounterBlock, pCounterBlock->ByteLength)))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PERFLIB: Invalid ObjectType CounterBlock!");
        pCounterBlock = NULL;
    }

    return pCounterBlock;
}

static inline PERF_COUNTER_BLOCK *getInstanceCounterBlock(
    PERF_DATA_BLOCK *pDataBlock,
    PERF_OBJECT_TYPE *pObjectType,
    PERF_INSTANCE_DEFINITION *pInstance
) {
    (void)pObjectType;
    PERF_COUNTER_BLOCK *pCounterBlock = (PERF_COUNTER_BLOCK *)((PBYTE)pInstance + pInstance->ByteLength);

    if(unlikely(pCounterBlock && (!isValidPointer(pDataBlock, pCounterBlock) || !isValidStructure(pDataBlock, pCounterBlock, pCounterBlock->ByteLength)))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PERFLIB: Invalid Instance CounterBlock!");
        pCounterBlock = NULL;
    }

    return pCounterBlock;
}

inline PERF_INSTANCE_DEFINITION *getInstanceByPosition(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, DWORD instancePosition) {
    PERF_INSTANCE_DEFINITION *pi = NULL;
    PERF_COUNTER_BLOCK *pc = NULL;
    for(DWORD i = 0; i <= instancePosition ;i++) {
        pi = getInstance(pDataBlock, pObjectType, pc);
        pc = getInstanceCounterBlock(pDataBlock, pObjectType, pi);
    }
    return pi;
}

static inline PERF_COUNTER_DEFINITION *getCounterDefinition(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_COUNTER_DEFINITION *lastCounterDefinition) {
    PERF_COUNTER_DEFINITION *pCounterDefinition = NULL;

    if(unlikely(!lastCounterDefinition))
        pCounterDefinition = (PERF_COUNTER_DEFINITION *)((PBYTE)pObjectType + pObjectType->HeaderLength);
    else
        pCounterDefinition = (PERF_COUNTER_DEFINITION *)((PBYTE)lastCounterDefinition +	lastCounterDefinition->ByteLength);

    if(unlikely(pCounterDefinition && (!isValidPointer(pDataBlock, pCounterDefinition) || !isValidStructure(pDataBlock, pCounterDefinition, pCounterDefinition->ByteLength)))) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "WINDOWS: PERFLIB: Invalid Counter Definition!");
        pCounterDefinition = NULL;
    }

    return pCounterDefinition;
}

// --------------------------------------------------------------------------------------------------------------------

static inline BOOL getEncodedStringToUTF8(char *dst, size_t dst_len, DWORD CodePage, char *start, DWORD length) {
    WCHAR *tempBuffer;  // Temporary buffer for Unicode data
    DWORD charsCopied = 0;
    BOOL free_tempBuffer;

    if (likely(CodePage == 0)) {
        // Input is already Unicode (UTF-16)
        tempBuffer = (WCHAR *)start;
        charsCopied = length / sizeof(WCHAR);  // Convert byte length to number of WCHARs
        free_tempBuffer = FALSE;
    }
    else {
        // Convert the multi-byte instance name to Unicode (UTF-16)
        // Calculate maximum possible characters in UTF-16

        int charCount = MultiByteToWideChar(CodePage, 0, start, (int)length, NULL, 0);
        tempBuffer = (WCHAR *)malloc(charCount * sizeof(WCHAR));
        if (!tempBuffer) return FALSE;

        charsCopied = MultiByteToWideChar(CodePage, 0, start, (int)length, tempBuffer, charCount);
        if (charsCopied == 0) {
            free(tempBuffer);
            dst[0] = '\0';
            return FALSE;
        }

        free_tempBuffer = TRUE;
    }

    // Now convert from Unicode (UTF-16) to UTF-8
    int bytesCopied = WideCharToMultiByte(CP_UTF8, 0, tempBuffer, (int)charsCopied, dst, (int)dst_len, NULL, NULL);
    if (bytesCopied == 0) {
        if (free_tempBuffer) free(tempBuffer);
        dst[0] = '\0'; // Ensure the buffer is null-terminated even on failure
        return FALSE;
    }

    dst[bytesCopied] = '\0'; // Ensure buffer is null-terminated
    if (free_tempBuffer) free(tempBuffer); // Free temporary buffer if used
    return TRUE;
}

inline BOOL getInstanceName(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, PERF_INSTANCE_DEFINITION *pInstance,
                     char *buffer, size_t bufferLen) {
    (void)pDataBlock;
    if (unlikely(!pInstance || !buffer || !bufferLen)) return FALSE;

    return getEncodedStringToUTF8(buffer, bufferLen, pObjectType->CodePage,
                                  ((char *)pInstance + pInstance->NameOffset), pInstance->NameLength);
}

inline BOOL getSystemName(PERF_DATA_BLOCK *pDataBlock, char *buffer, size_t bufferLen) {
    return getEncodedStringToUTF8(buffer, bufferLen, 0,
                                  ((char *)pDataBlock + pDataBlock->SystemNameOffset), pDataBlock->SystemNameLength);
}

inline bool ObjectTypeHasInstances(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType) {
    (void)pDataBlock;
    return pObjectType->NumInstances != PERF_NO_INSTANCES && pObjectType->NumInstances > 0;
}

PERF_OBJECT_TYPE *perflibFindObjectTypeByName(PERF_DATA_BLOCK *pDataBlock, const char *name) {
    PERF_OBJECT_TYPE* pObjectType = NULL;
    for(DWORD o = 0; o < pDataBlock->NumObjectTypes; o++) {
        pObjectType = getObjectType(pDataBlock, pObjectType);
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
        PERF_COUNTER_BLOCK *pCounterBlock = getInstanceCounterBlock(pDataBlock, pObjectType, pInstance);

        cd->previous = cd->current;
        cd->updated = getCounterData(pDataBlock, pObjectType, pCounterDefinition, pCounterBlock, &cd->current);
        return cd->updated;
    }

    cd->previous = cd->current;
    cd->current = RAW_DATA_EMPTY;
    cd->updated = false;
    return false;
}

bool perflibGetObjectCounter(PERF_DATA_BLOCK *pDataBlock, PERF_OBJECT_TYPE *pObjectType, COUNTER_DATA *cd) {
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
