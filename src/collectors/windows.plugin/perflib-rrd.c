// SPDX-License-Identifier: GPL-3.0-or-later

#include "perflib-rrd.h"

#define COLLECTED_NUMBER_PRECISION 10000

RRDDIM *perflib_rrddim_add(
    RRDSET *st,
    const char *id,
    const char *name,
    collected_number multiplier,
    collected_number divider,
    COUNTER_DATA *cd)
{
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
        case PERF_AVERAGE_BULK: // normally not displayed
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

#define VALID_DELTA(cd)                                                                                                \
    ((cd)->previous.Time > 0 && (cd)->current.Data >= (cd)->previous.Data && (cd)->current.Time > (cd)->previous.Time)

collected_number perflib_rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, COUNTER_DATA *cd)
{
    ULONGLONG numerator = 0;
    LONGLONG denominator = 0;
    double doubleValue = 0.0;
    collected_number value;

    switch (cd->current.CounterType) {
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
        case PERF_AVERAGE_BULK: // normally not displayed
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
            if (!VALID_DELTA(cd))
                return 0;
            numerator = cd->current.Data - cd->previous.Data;
            denominator = cd->current.Time - cd->previous.Time;
            doubleValue = 100.0 * (1.0 - ((double)numerator / (double)denominator));
            // printf("Display value is (timer-inv): %f%%\n", doubleValue);
            value = (collected_number)(doubleValue * COLLECTED_NUMBER_PRECISION);
            break;

        case PERF_COUNTER_MULTI_TIMER:
            // 100 * ((N1 - N0) / ((D1 - D0) / TB)) / B1
            if (!VALID_DELTA(cd))
                return 0;
            numerator = cd->current.Data - cd->previous.Data;
            denominator = cd->current.Time - cd->previous.Time;
            denominator /= cd->current.Frequency;
            doubleValue = 100.0 * ((double)numerator / (double)denominator) / cd->current.MultiCounterData;
            // printf("Display value is (multi-timer): %f%%\n", doubleValue);
            value = (collected_number)(doubleValue * COLLECTED_NUMBER_PRECISION);
            break;

        case PERF_100NSEC_MULTI_TIMER:
            // 100 * ((N1 - N0) / (D1 - D0)) / B1
            if (!VALID_DELTA(cd))
                return 0;
            numerator = cd->current.Data - cd->previous.Data;
            denominator = cd->current.Time - cd->previous.Time;
            doubleValue = 100.0 * ((double)numerator / (double)denominator) / (double)cd->current.MultiCounterData;
            // printf("Display value is (100ns multi-timer): %f%%\n", doubleValue);
            value = (collected_number)(doubleValue * COLLECTED_NUMBER_PRECISION);
            break;

        case PERF_COUNTER_MULTI_TIMER_INV:
        case PERF_100NSEC_MULTI_TIMER_INV:
            // 100 * (B1 - ((N1 - N0) / (D1 - D0)))
            if (!VALID_DELTA(cd))
                return 0;
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
            if (!VALID_DELTA(cd))
                return 0;
            value = (collected_number)(cd->current.Data - cd->previous.Data);
            break;

        case PERF_RAW_FRACTION:
        case PERF_LARGE_RAW_FRACTION:
            // 100 * N / B
            if (!cd->current.Time)
                return 0;
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
