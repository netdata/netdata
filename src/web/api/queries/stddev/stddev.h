// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERIES_STDDEV_H
#define NETDATA_API_QUERIES_STDDEV_H

#include "../query.h"
#include "../rrdr.h"

// this implementation comes from:
// https://www.johndcook.com/blog/standard_deviation/

struct tg_stddev {
    long count;
    NETDATA_DOUBLE m_oldM, m_newM, m_oldS, m_newS;
};

static inline void tg_stddev_create(RRDR *r, const char *options __maybe_unused) {
    r->time_grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct tg_stddev));
}

// resets when switches dimensions
// so, clear everything to restart
static inline void tg_stddev_reset(RRDR *r) {
    struct tg_stddev *g = (struct tg_stddev *)r->time_grouping.data;
    g->count = 0;
}

static inline void tg_stddev_free(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->time_grouping.data);
    r->time_grouping.data = NULL;
}

static inline void tg_stddev_add(RRDR *r, NETDATA_DOUBLE value) {
    struct tg_stddev *g = (struct tg_stddev *)r->time_grouping.data;

    g->count++;

    // See Knuth TAOCP vol 2, 3rd edition, page 232
    if (g->count == 1) {
        g->m_oldM = g->m_newM = value;
        g->m_oldS = 0.0;
    }
    else {
        g->m_newM = g->m_oldM + (value - g->m_oldM) / g->count;
        g->m_newS = g->m_oldS + (value - g->m_oldM) * (value - g->m_newM);

        // set up for next iteration
        g->m_oldM = g->m_newM;
        g->m_oldS = g->m_newS;
    }
}

static inline NETDATA_DOUBLE tg_stddev_mean(struct tg_stddev *g) {
    return (g->count > 0) ? g->m_newM : 0.0;
}

static inline NETDATA_DOUBLE tg_stddev_variance(struct tg_stddev *g) {
    return ( (g->count > 1) ? g->m_newS/(NETDATA_DOUBLE)(g->count - 1) : 0.0 );
}
static inline NETDATA_DOUBLE tg_stddev_stddev(struct tg_stddev *g) {
    return sqrtndd(tg_stddev_variance(g));
}

static inline NETDATA_DOUBLE tg_stddev_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_stddev *g = (struct tg_stddev *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(likely(g->count > 1)) {
        value = tg_stddev_stddev(g);

        if(!netdata_double_isnumber(value)) {
            value = 0.0;
            *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        }
    }
    else if(g->count == 1) {
        value = 0.0;
    }
    else {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }

    tg_stddev_reset(r);

    return  value;
}

// https://en.wikipedia.org/wiki/Coefficient_of_variation
static inline NETDATA_DOUBLE tg_stddev_coefficient_of_variation_flush(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct tg_stddev *g = (struct tg_stddev *)r->time_grouping.data;

    NETDATA_DOUBLE value;

    if(likely(g->count > 1)) {
        NETDATA_DOUBLE m = tg_stddev_mean(g);
        value = 100.0 * tg_stddev_stddev(g) / ((m < 0)? -m : m);

        if(unlikely(!netdata_double_isnumber(value))) {
            value = 0.0;
            *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        }
    }
    else if(g->count == 1) {
        // one value collected
        value = 0.0;
    }
    else {
        // no values collected
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }

    tg_stddev_reset(r);

    return  value;
}

#endif //NETDATA_API_QUERIES_STDDEV_H
