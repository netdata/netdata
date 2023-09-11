// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_MAX_H
#define NETDATA_API_QUERY_MAX_H

#include "../query.h"
#include "../rrdr.h"

struct tg_max {
    NETDATA_DOUBLE max;
    size_t count;
};

static inline void tg_max_create(RRDR *r, const char *options __maybe_unused) {
    r->time_grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_max));
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_max_reset(RRDR *r) {
    struct tg_max *g = (struct tg_max *)r->time_grouping.data;
    g->max = 0;
    g->count = 0;
}

static inline void tg_max_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_max_add(RRDR *r, NETDATA_DOUBLE value) {
    struct tg_max *g = (struct tg_max *)r->time_grouping.data;

    if(!g->count || fabsndd(value) > fabsndd(g->max)) {
        g->max = value;
        g->count++;
    }
}

static inline NETDATA_DOUBLE tg_max_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_max *g = (struct tg_max *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = g->max;
    }

    g->max = 0.0;
    g->count = 0;

    return value;
}

#endif //NETDATA_API_QUERY_MAX_H
