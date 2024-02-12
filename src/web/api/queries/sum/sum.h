// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_SUM_H
#define NETDATA_API_QUERY_SUM_H

#include "../query.h"
#include "../rrdr.h"

struct tg_sum {
    NETDATA_DOUBLE sum;
    size_t count;
};

static inline void tg_sum_create(RRDR *r, const char *options __maybe_unused) {
    r->time_grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_sum));
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_sum_reset(RRDR *r) {
    struct tg_sum *g = (struct tg_sum *)r->time_grouping.data;
    g->sum = 0;
    g->count = 0;
}

static inline void tg_sum_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_sum_add(RRDR *r, NETDATA_DOUBLE value) {
    struct tg_sum *g = (struct tg_sum *)r->time_grouping.data;
    g->sum += value;
    g->count++;
}

static inline NETDATA_DOUBLE tg_sum_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_sum *g = (struct tg_sum *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = g->sum;
    }

    g->sum = 0.0;
    g->count = 0;

    return value;
}

#endif //NETDATA_API_QUERY_SUM_H
