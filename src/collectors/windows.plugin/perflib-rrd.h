// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PERFLIB_RRD_H
#define NETDATA_PERFLIB_RRD_H

#include "database/rrd.h"

typedef struct perflib_worker_state {
    bool initialized;
    bool seen;
    size_t counter_count;
    max_align_t data[];
} PERFLIB_WORKER_STATE;

bool perflib_counter_type_is_incremental(uint32_t counter_type);
bool perflib_counter_type_is_32bit(uint32_t counter_type);
bool perflib_counter_type_is_32bit_rate(uint32_t counter_type);
uint64_t perflib_counter_delta(uint64_t previous, uint64_t current, bool is_32bit);
enum perflib_aggregate_mode {
    PERFLIB_AGGREGATE_SUM,
    PERFLIB_AGGREGATE_INCREMENTAL,
    PERFLIB_AGGREGATE_MAXIMUM,
};
void perflib_aggregate_instance_sample(
    COUNTER_DATA *aggregate,
    const COUNTER_DATA *sample,
    bool *has_previous,
    enum perflib_aggregate_mode mode);
DICTIONARY *perflib_worker_dictionary_create(size_t counter_count);
PERFLIB_WORKER_STATE *perflib_worker_state_get(DICTIONARY *workers, const char *key, size_t counter_count);
bool *perflib_worker_state_has_sample(PERFLIB_WORKER_STATE *worker);
COUNTER_DATA *perflib_worker_state_counters(PERFLIB_WORKER_STATE *worker);
void perflib_worker_state_mark_all_unseen(DICTIONARY *workers);
void perflib_worker_state_remove_unseen(DICTIONARY *workers);

RRDDIM *perflib_rrddim_add(
    RRDSET *st,
    const char *id,
    const char *name,
    collected_number multiplier,
    collected_number divider,
    COUNTER_DATA *cd);
collected_number perflib_rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, COUNTER_DATA *cd);

#endif //NETDATA_PERFLIB_RRD_H
