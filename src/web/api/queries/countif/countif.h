// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_COUNTIF_H
#define NETDATA_API_QUERY_COUNTIF_H

#include "../query.h"
#include "../rrdr.h"
#include "../tg-expression.h"

// percentage-of-samples (historically `countif`, still accepted as an
// alias): the share of the samples in each bucket that matched the
// condition.
//
// The values it returns are unchanged; what changed is that the condition
// is now parsed by the shared grammar, so this understands gap tokens and
// the predecessor keywords, and a bare number finally means what it says.

struct tg_countif {
    TG_EXPRESSION expr;
    size_t count;
    size_t matched;
};

static inline void tg_countif_create(RRDR *r, const char *options) {
    struct tg_countif *g = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_countif));
    // the API has never rejected a malformed condition here
    (void)tg_expression_parse(&g->expr, options);
    r->time_grouping.data = g;
    r->time_grouping.wants_gaps = tg_expression_wants_gaps(&g->expr);
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_countif_reset(RRDR *r) {
    struct tg_countif *g = (struct tg_countif *)r->time_grouping.data;
    g->matched = 0;
    g->count = 0;
    tg_expression_reset(&g->expr);
}

static inline void tg_countif_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_countif_add_point(RRDR *r, const TG_POINT *p) {
    struct tg_countif *g = (struct tg_countif *)r->time_grouping.data;

    // this one deliberately keeps its historical behaviour: a delivered
    // point IS a sample here, so the condition is evaluated on it and every
    // delivery counts - including a wide point re-delivered to the buckets
    // it spans, which is what the query engine has always fed this
    // grouping. Skipping repeats would leave those buckets EMPTY and change
    // results that predate this work. percentage-of-time is the grouping
    // that reasons about a window instead.
    // a gap stands for every sample slot it covers, not for one point
    if(tg_expression_eval(&g->expr, p->value, p->is_gap))
        g->matched += p->samples;

    g->count += p->samples;
}

static inline void tg_countif_add(RRDR *r, NETDATA_DOUBLE value) {
    TG_POINT p = { .value = value, .min = value, .max = value, .count = 1,
                   .sum = value, .duration = 1, .samples = 1,
                   .is_gap = false, .first = true };
    tg_countif_add_point(r, &p);
}

static inline NETDATA_DOUBLE tg_countif_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_countif *g = (struct tg_countif *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = (NETDATA_DOUBLE)g->matched * 100 / (NETDATA_DOUBLE)g->count;
    }

    g->matched = 0;
    g->count = 0;

    // the predecessor survives the flush, so `<previous` spans buckets

    return value;
}

#endif //NETDATA_API_QUERY_COUNTIF_H
