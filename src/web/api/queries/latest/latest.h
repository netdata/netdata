// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_LATEST_H
#define NETDATA_API_QUERY_LATEST_H

#include "../query.h"
#include "../rrdr.h"

// keep the latest (most recent) non-empty value of each group;
// gap points never reach tg_latest_add(), so a group without any
// collected samples flushes as empty

struct tg_latest {
    NETDATA_DOUBLE latest;
    size_t count;
};

static inline void tg_latest_create(RRDR *r, const char *options __maybe_unused) {
    r->time_grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_latest));
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_latest_reset(RRDR *r) {
    struct tg_latest *g = (struct tg_latest *)r->time_grouping.data;
    g->latest = 0;
    g->count = 0;
}

static inline void tg_latest_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_latest_add(RRDR *r, NETDATA_DOUBLE value) {
    struct tg_latest *g = (struct tg_latest *)r->time_grouping.data;

    // values arrive in query direction order, oldest to newest,
    // so the last one added is the latest of the group
    g->latest = value;
    g->count++;
}

static inline NETDATA_DOUBLE tg_latest_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_latest *g = (struct tg_latest *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = g->latest;
    }

    g->latest = 0.0;
    g->count = 0;

    return value;
}

#endif //NETDATA_API_QUERY_LATEST_H
