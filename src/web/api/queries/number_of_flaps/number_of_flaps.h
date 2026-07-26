// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_NUMBER_OF_FLAPS_H
#define NETDATA_API_QUERIES_NUMBER_OF_FLAPS_H

#include "../query.h"
#include "../rrdr.h"
#include "../tg-expression.h"

// How many times a condition flipped from false to true - link flapping,
// a service bouncing, an alert condition oscillating.
//
// Only OBSERVED transitions count: a series that is already true when the
// query window opens contributes nothing until it goes false and returns.
// The boolean state carries across bucket boundaries (the ses/des
// precedent) while the counter itself is per-bucket.

struct tg_number_of_flaps {
    TG_EXPRESSION expr;

    size_t flaps;       // emitted and zeroed by flush()
    size_t count;       // points in this bucket, to tell empty from zero

    bool state;         // carried across buckets
    bool has_state;
};

static inline void tg_number_of_flaps_create(RRDR *r, const char *options) {
    struct tg_number_of_flaps *g =
        onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_number_of_flaps));
    // the API has never rejected a malformed condition here
    (void)tg_expression_parse(&g->expr, options);
    r->time_grouping.data = g;
    r->time_grouping.wants_gaps = tg_expression_wants_gaps(&g->expr);
}

// resets when the query switches dimensions - the carried state belongs
// to one series
static inline void tg_number_of_flaps_reset(RRDR *r) {
    struct tg_number_of_flaps *g = (struct tg_number_of_flaps *)r->time_grouping.data;
    g->flaps = 0;
    g->count = 0;
    g->state = false;
    g->has_state = false;
    tg_expression_reset(&g->expr);
}

static inline void tg_number_of_flaps_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_number_of_flaps_add_point(RRDR *r, const TG_POINT *p) {
    struct tg_number_of_flaps *g = (struct tg_number_of_flaps *)r->time_grouping.data;

    NETDATA_DOUBLE share = tg_expression_share(&g->expr, p);

    if(unlikely(!p->first))
        // the same stored point cannot flip the state twice
        return;

    if(p->count > 1 && share > 0.0 && share < 1.0) {
        // the condition was both true and false inside this stored window,
        // so it changed at least once - counted as one, because the window
        // keeps no ordering. This is an approximation, not a lower bound:
        // [true,false] and [false,true] leave identical statistics behind.
        g->flaps++;
        g->state = true;
    }
    else {
        bool now = (share > 0.0);

        if(likely(g->has_state) && !g->state && now)
            g->flaps++;

        // a contiguous hole is ONE state change however many slots it spans
        g->state = now;
    }

    g->has_state = true;
    g->count += p->samples;
}

static inline void tg_number_of_flaps_add(RRDR *r, NETDATA_DOUBLE value) {
    TG_POINT p = { .value = value, .min = value, .max = value, .count = 1,
                   .sum = value, .duration = 1, .samples = 1,
                   .is_gap = false, .first = true };
    tg_number_of_flaps_add_point(r, &p);
}

static inline NETDATA_DOUBLE tg_number_of_flaps_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_number_of_flaps *g = (struct tg_number_of_flaps *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else
        value = (NETDATA_DOUBLE)g->flaps;

    g->flaps = 0;
    g->count = 0;

    // g->state and the predecessor deliberately survive: a flip that
    // straddles two buckets is still one flip

    return value;
}

#endif //NETDATA_API_QUERIES_NUMBER_OF_FLAPS_H
