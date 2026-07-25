// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_NUMBER_OF_TIMES_H
#define NETDATA_API_QUERIES_NUMBER_OF_TIMES_H

#include "../query.h"
#include "../rrdr.h"
#include "../tg-expression.h"

// How many samples matched - occurrence counting.
//
// Distinct from number-of-flaps: consecutive matches count individually,
// so a node that rebooted twice in a row is 2 times but 1 flap. With the
// predecessor keyword this is the uptime query - `<previous` counts the
// times a counter went backwards, i.e. reboots - and `==previous` counts
// the times a metric did not move at all.

struct tg_number_of_times {
    TG_EXPRESSION expr;
    size_t times;       // emitted and zeroed by flush()
    size_t count;       // points in this bucket, to tell empty from zero
};

static inline void tg_number_of_times_create(RRDR *r, const char *options) {
    struct tg_number_of_times *g =
        onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_number_of_times));
    // the API has never rejected a malformed condition here
    (void)tg_expression_parse(&g->expr, options);
    r->time_grouping.data = g;
    r->time_grouping.wants_gaps = tg_expression_wants_gaps(&g->expr);
}

// resets when the query switches dimensions
static inline void tg_number_of_times_reset(RRDR *r) {
    struct tg_number_of_times *g = (struct tg_number_of_times *)r->time_grouping.data;
    g->times = 0;
    g->count = 0;
    tg_expression_reset(&g->expr);
}

static inline void tg_number_of_times_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_number_of_times_add_point(
    RRDR *r, NETDATA_DOUBLE value, bool is_gap, time_t duration __maybe_unused, size_t samples) {
    struct tg_number_of_times *g = (struct tg_number_of_times *)r->time_grouping.data;

    // a gap stands for every sample slot it covers, not for one point
    if(tg_expression_eval(&g->expr, value, is_gap))
        g->times += samples;

    g->count += samples;
}

static inline void tg_number_of_times_add(RRDR *r, NETDATA_DOUBLE value) {
    tg_number_of_times_add_point(r, value, false, 1, 1);
}

static inline NETDATA_DOUBLE tg_number_of_times_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_number_of_times *g = (struct tg_number_of_times *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else
        value = (NETDATA_DOUBLE)g->times;

    g->times = 0;
    g->count = 0;

    // the predecessor survives the flush: a drop across a bucket boundary
    // is still a drop

    return value;
}

#endif //NETDATA_API_QUERIES_NUMBER_OF_TIMES_H
