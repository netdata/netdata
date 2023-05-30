// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_INCREMENTAL_SUM_H
#define NETDATA_API_QUERY_INCREMENTAL_SUM_H

#include "../query.h"
#include "../rrdr.h"

struct tg_incremental_sum {
    NETDATA_DOUBLE first;
    NETDATA_DOUBLE last;
    size_t count;
};

static inline void tg_incremental_sum_create(RRDR *r, const char *options __maybe_unused) {
    r->time_grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_incremental_sum));
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_incremental_sum_reset(RRDR *r) {
    struct tg_incremental_sum *g = (struct tg_incremental_sum *)r->time_grouping.data;
    g->first = 0;
    g->last = 0;
    g->count = 0;
}

static inline void tg_incremental_sum_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_incremental_sum_add(RRDR *r, NETDATA_DOUBLE value) {
    struct tg_incremental_sum *g = (struct tg_incremental_sum *)r->time_grouping.data;

    if(unlikely(!g->count)) {
        g->first = value;
        g->count++;
    }
    else {
        g->last = value;
        g->count++;
    }
}

static inline NETDATA_DOUBLE tg_incremental_sum_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_incremental_sum *g = (struct tg_incremental_sum *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else if(unlikely(g->count == 1)) {
        value = 0.0;
    }
    else {
        value = g->last - g->first;
    }

    g->first = 0.0;
    g->last = 0.0;
    g->count = 0;

    return value;
}

#endif //NETDATA_API_QUERY_INCREMENTAL_SUM_H
