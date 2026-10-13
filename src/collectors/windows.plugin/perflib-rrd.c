// SPDX-License-Identifier: GPL-3.0-or-later

#include "perflib-rrd.h"

#include "libnetdata/libnetdata.h"

#define COLLECTED_NUMBER_PRECISION 10000

bool perflib_counter_type_is_incremental(uint32_t counter_type)
{
    switch (counter_type) {
        case PERF_COUNTER_COUNTER:
        case PERF_SAMPLE_COUNTER:
        case PERF_COUNTER_BULK_COUNT:
        case PERF_COUNTER_QUEUELEN_TYPE:
        case PERF_COUNTER_100NS_QUEUELEN_TYPE:
        case PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE:
        case PERF_COUNTER_LARGE_QUEUELEN_TYPE:
        case PERF_AVERAGE_BULK:
        case PERF_AVERAGE_TIMER:
        case PERF_OBJ_TIME_TIMER:
        case PERF_COUNTER_TIMER:
        case PERF_100NSEC_TIMER:
        case PERF_PRECISION_SYSTEM_TIMER:
        case PERF_PRECISION_100NS_TIMER:
        case PERF_PRECISION_OBJECT_TIMER:
        case PERF_SAMPLE_FRACTION:
            return true;
        default:
            return false;
    }
}

bool perflib_counter_type_is_32bit(uint32_t counter_type)
{
    switch (counter_type) {
        case PERF_COUNTER_COUNTER:
        case PERF_SAMPLE_COUNTER:
        case PERF_COUNTER_QUEUELEN_TYPE:
        case PERF_OBJ_TIME_TIMER:
        case PERF_COUNTER_RAWCOUNT:
        case PERF_COUNTER_RAWCOUNT_HEX:
        case PERF_COUNTER_DELTA:
        case PERF_SAMPLE_FRACTION:
        case PERF_RAW_FRACTION:
            return true;
        default:
            return false;
    }
}

bool perflib_counter_type_is_32bit_rate(uint32_t counter_type)
{
    return counter_type == PERF_COUNTER_COUNTER || counter_type == PERF_SAMPLE_COUNTER;
}

uint64_t perflib_counter_delta(uint64_t previous, uint64_t current, bool is_32bit)
{
    if (!is_32bit)
        // A decrease means the 64-bit counter reset; use its new value as this interval's delta.
        return current >= previous ? current - previous : current;

    uint32_t previous32 = (uint32_t)previous;
    uint32_t current32 = (uint32_t)current;
    uint64_t delta = current32 >= previous32 ? (uint64_t)(current32 - previous32) :
                                               (uint64_t)UINT32_MAX - previous32 + current32 + 1;
    // A wrap of at least half the range is indistinguishable from a reset or stale sample; ignore it.
    return delta < (UINT64_C(1) << 31) ? delta : 0;
}

void perflib_aggregate_instance_sample(
    COUNTER_DATA *aggregate,
    const COUNTER_DATA *sample,
    bool *has_previous,
    enum perflib_aggregate_mode mode)
{
    if (mode == PERFLIB_AGGREGATE_INCREMENTAL) {
        if (*has_previous)
            aggregate->current.Data += perflib_counter_delta(
                sample->previous.Data,
                sample->current.Data,
                perflib_counter_type_is_32bit(sample->current.CounterType));
        *has_previous = true;
    } else if (mode == PERFLIB_AGGREGATE_MAXIMUM) {
        if (!aggregate->updated || sample->current.Data > aggregate->current.Data)
            aggregate->current.Data = sample->current.Data;
        *has_previous = true;
    } else
        aggregate->current.Data += sample->current.Data;

    if (!aggregate->updated) {
        aggregate->current.CounterType = sample->current.CounterType;
        aggregate->current.Time = sample->current.Time;
        aggregate->current.Frequency = sample->current.Frequency;
    }
    aggregate->updated = true;
    aggregate->id = sample->id;
}

static size_t perflib_worker_counters_offset(size_t counter_count)
{
    size_t offset = counter_count * sizeof(bool);
    size_t alignment = _Alignof(COUNTER_DATA);
    return (offset + alignment - 1) & ~(alignment - 1);
}

static size_t perflib_worker_state_size(size_t counter_count)
{
    return offsetof(PERFLIB_WORKER_STATE, data) + perflib_worker_counters_offset(counter_count) +
           counter_count * sizeof(COUNTER_DATA);
}

DICTIONARY *perflib_worker_dictionary_create(size_t counter_count)
{
    return dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, perflib_worker_state_size(counter_count));
}

PERFLIB_WORKER_STATE *perflib_worker_state_get(DICTIONARY *workers, const char *key, size_t counter_count)
{
    size_t value_size = perflib_worker_state_size(counter_count);
    PERFLIB_WORKER_STATE *worker = dictionary_set(workers, key, NULL, value_size);
    if (!worker)
        return NULL;

    if (worker->counter_count && worker->counter_count != counter_count)
        return NULL;
    worker->counter_count = counter_count;
    return worker;
}

bool *perflib_worker_state_has_sample(PERFLIB_WORKER_STATE *worker)
{
    return (bool *)worker->data;
}

COUNTER_DATA *perflib_worker_state_counters(PERFLIB_WORKER_STATE *worker)
{
    return (COUNTER_DATA *)((uint8_t *)worker->data + perflib_worker_counters_offset(worker->counter_count));
}

void perflib_worker_state_mark_all_unseen(DICTIONARY *workers)
{
    if (!workers)
        return;

    PERFLIB_WORKER_STATE *worker;
    dfe_start_write(workers, worker) worker->seen = false;
    dfe_done(worker);
}

void perflib_worker_state_remove_unseen(DICTIONARY *workers)
{
    if (!workers)
        return;

    PERFLIB_WORKER_STATE *worker;
    dfe_start_write(workers, worker)
    {
        if (!worker->seen)
            dictionary_del(workers, worker_dfe.name);
    }
    dfe_done(worker);
    dictionary_garbage_collect(workers);
}

RRDDIM *perflib_rrddim_add(
    RRDSET *st,
    const char *id,
    const char *name,
    collected_number multiplier,
    collected_number divider,
    COUNTER_DATA *cd)
{
    RRD_ALGORITHM algorithm = perflib_counter_type_is_incremental(cd->current.CounterType) ? RRD_ALGORITHM_INCREMENTAL :
                                                                                             RRD_ALGORITHM_ABSOLUTE;

    switch (cd->current.CounterType) {
        case PERF_COUNTER_COUNTER:
        case PERF_SAMPLE_COUNTER:
        case PERF_COUNTER_BULK_COUNT:
            // (N1 - N0) / ((D1 - D0) / F)
            // multiplier *= cd->current.Frequency / 10000000;
            // tested, the frequency is not that useful for netdata
            // we get right results without it.
            break;

        case PERF_COUNTER_QUEUELEN_TYPE:
        case PERF_COUNTER_100NS_QUEUELEN_TYPE:
        case PERF_COUNTER_OBJ_TIME_QUEUELEN_TYPE:
        case PERF_COUNTER_LARGE_QUEUELEN_TYPE:
        case PERF_AVERAGE_BULK: // normally not displayed
            // (N1 - N0) / (D1 - D0)
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
            break;

        case PERF_ELAPSED_TIME:
            // (D0 - N0) / F
            algorithm = RRD_ALGORITHM_ABSOLUTE;
            divider *= COLLECTED_NUMBER_PRECISION;
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
            if (!VALID_DELTA(cd) || !cd->current.Frequency || !cd->current.MultiCounterData)
                return 0;
            numerator = cd->current.Data - cd->previous.Data;
            denominator = cd->current.Time - cd->previous.Time;
            doubleValue = 100.0 * ((double)numerator / ((double)denominator / (double)cd->current.Frequency)) /
                          (double)cd->current.MultiCounterData;
            // printf("Display value is (multi-timer): %f%%\n", doubleValue);
            value = (collected_number)(doubleValue * COLLECTED_NUMBER_PRECISION);
            break;

        case PERF_100NSEC_MULTI_TIMER:
            // 100 * ((N1 - N0) / (D1 - D0)) / B1
            if (!VALID_DELTA(cd) || !cd->current.MultiCounterData)
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
            if (!VALID_DELTA(cd) || !cd->current.MultiCounterData)
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

        case PERF_ELAPSED_TIME:
            if (!cd->current.Frequency || !cd->current.Data || cd->current.Time < (LONGLONG)cd->current.Data)
                return 0;
            doubleValue = (double)(cd->current.Time - cd->current.Data) / (double)cd->current.Frequency;
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
