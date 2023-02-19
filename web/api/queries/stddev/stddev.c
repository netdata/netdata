// SPDX-License-Identifier: GPL-3.0-or-later

#include "stddev.h"


// ----------------------------------------------------------------------------
// stddev

// this implementation comes from:
// https://www.johndcook.com/blog/standard_deviation/

struct grouping_stddev {
    long count;
    NETDATA_DOUBLE m_oldM, m_newM, m_oldS, m_newS;
};

void grouping_create_stddev(RRDR *r, const char *options __maybe_unused) {
    r->grouping.data = onewayalloc_callocz(r->internal.owa, 1, sizeof(struct grouping_stddev));
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_stddev(RRDR *r) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping.data;
    g->count = 0;
}

void grouping_free_stddev(RRDR *r) {
    onewayalloc_freez(r->internal.owa, r->grouping.data);
    r->grouping.data = NULL;
}

void grouping_add_stddev(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping.data;

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

static inline NETDATA_DOUBLE mean(struct grouping_stddev *g) {
    return (g->count > 0) ? g->m_newM : 0.0;
}

static inline NETDATA_DOUBLE variance(struct grouping_stddev *g) {
    return ( (g->count > 1) ? g->m_newS/(NETDATA_DOUBLE)(g->count - 1) : 0.0 );
}
static inline NETDATA_DOUBLE stddev(struct grouping_stddev *g) {
    return sqrtndd(variance(g));
}

NETDATA_DOUBLE grouping_flush_stddev(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping.data;

    NETDATA_DOUBLE value;

    if(likely(g->count > 1)) {
        value = stddev(g);

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

    grouping_reset_stddev(r);

    return  value;
}

// https://en.wikipedia.org/wiki/Coefficient_of_variation
NETDATA_DOUBLE grouping_flush_coefficient_of_variation(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping.data;

    NETDATA_DOUBLE value;

    if(likely(g->count > 1)) {
        NETDATA_DOUBLE m = mean(g);
        value = 100.0 * stddev(g) / ((m < 0)? -m : m);

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

    grouping_reset_stddev(r);

    return  value;
}


/*
 * Mean = average
 *
NETDATA_DOUBLE grouping_flush_mean(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping.grouping_data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = mean(g);

        if(!isnormal(value)) {
            value = 0.0;
            *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        }
    }

    grouping_reset_stddev(r);

    return  value;
}
 */

/*
 * It is not advised to use this version of variance directly
 *
NETDATA_DOUBLE grouping_flush_variance(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping.grouping_data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = variance(g);

        if(!isnormal(value)) {
            value = 0.0;
            *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        }
    }

    grouping_reset_stddev(r);

    return  value;
}
*/