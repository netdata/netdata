// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_MIN_H
#define NETDATA_API_QUERY_MIN_H

#include "../query.h"
#include "../rrdr.h"

struct tg_min {
    NETDATA_DOUBLE min;
    size_t count;
};

static inline void tg_min_create(RRDR *r, const char *options __maybe_unused) {
    r->time_grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_min));
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_min_reset(RRDR *r) {
    struct tg_min *g = (struct tg_min *)r->time_grouping.data;
    g->min = 0;
    g->count = 0;
}

static inline void tg_min_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_min_add(RRDR *r, NETDATA_DOUBLE value) {
    struct tg_min *g = (struct tg_min *)r->time_grouping.data;

    if(!g->count || fabsndd(value) < fabsndd(g->min)) {
        g->min = value;
        g->count++;
    }
}

static inline NETDATA_DOUBLE tg_min_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_min *g = (struct tg_min *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = g->min;
    }

    g->min = 0.0;
    g->count = 0;

    return value;
}

#endif //NETDATA_API_QUERY_MIN_H
