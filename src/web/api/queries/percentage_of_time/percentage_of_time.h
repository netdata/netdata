// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_PERCENTAGE_OF_TIME_H
#define NETDATA_API_QUERIES_PERCENTAGE_OF_TIME_H

#include "../query.h"
#include "../rrdr.h"
#include "../tg-expression.h"

// The share of TIME a condition held, as opposed to the share of samples
// (percentage-of-samples). The two differ whenever sample density is not
// uniform - a changed update_every, or a higher tier where one stored
// point stands for many seconds - which is exactly the fleet SLO case.
//
// Durations arrive already clamped to the bucket by the query engine, so
// they always sum to the bucket's own width and a gap simply fills what
// the collected points left uncovered.

struct tg_percentage_of_time {
    TG_EXPRESSION expr;
    NETDATA_DOUBLE matched;
    NETDATA_DOUBLE total;
};

static inline void tg_percentage_of_time_create(RRDR *r, const char *options) {
    struct tg_percentage_of_time *g =
        onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_percentage_of_time));
    // the API has never rejected a malformed condition here
    (void)tg_expression_parse(&g->expr, options);
    r->time_grouping.data = g;

    // ALWAYS - whatever the condition compares against.
    //
    // The denominator of this grouping is the selected duration, not the
    // collected part of it. Uncollected time is time during which the
    // condition did not hold, so it belongs in the denominator: one
    // collected sample reading 1 followed by ninety-nine missing ones is
    // 1% of the window at `==1`, not 100% of the only sample that was
    // there. Every other grouping stays blind to gaps unless its condition
    // names one; this one cannot, or it stops measuring time.
    r->time_grouping.wants_gaps = true;
}

// resets when the query switches dimensions
static inline void tg_percentage_of_time_reset(RRDR *r) {
    struct tg_percentage_of_time *g = (struct tg_percentage_of_time *)r->time_grouping.data;
    g->matched = 0.0;
    g->total = 0.0;
    tg_expression_reset(&g->expr);
}

static inline void tg_percentage_of_time_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_percentage_of_time_add_point(RRDR *r, const TG_POINT *p) {
    struct tg_percentage_of_time *g = (struct tg_percentage_of_time *)r->time_grouping.data;

    // Above tier 0 the share is a fraction of the window, not a yes/no.
    // A gap contributes 0 to the numerator for every condition that does
    // not name one, and its duration to the denominator either way.
    //
    // A point that covers no time still passes through here: it adds
    // nothing to either side, but the condition must see it, or this
    // grouping's predecessor drifts away from the one the other three
    // carry over the same data.
    NETDATA_DOUBLE share = tg_expression_share(&g->expr, p);

    if(unlikely(p->duration <= 0))
        return;

    g->matched += share * (NETDATA_DOUBLE)p->duration;
    g->total += (NETDATA_DOUBLE)p->duration;
}

// the registry's generic entry point (the dispatcher's default arm and
// rrdr_set_grouping_function need every row to have one)
static inline void tg_percentage_of_time_add(RRDR *r, NETDATA_DOUBLE value) {
    TG_POINT p = { .value = value, .min = value, .max = value, .count = 1,
                   .sum = value, .duration = 1, .samples = 1,
                   .is_gap = false, .first = true };
    tg_percentage_of_time_add_point(r, &p);
}

static inline NETDATA_DOUBLE tg_percentage_of_time_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_percentage_of_time *g = (struct tg_percentage_of_time *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(g->total <= 0.0)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else
        value = g->matched * 100.0 / g->total;

    g->matched = 0.0;
    g->total = 0.0;

    // the predecessor deliberately survives the flush - a condition
    // relative to the last collected sample spans bucket boundaries

    return value;
}

#endif //NETDATA_API_QUERIES_PERCENTAGE_OF_TIME_H
