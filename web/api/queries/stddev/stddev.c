// SPDX-License-Identifier: GPL-3.0-or-later

#include "stddev.h"


// ----------------------------------------------------------------------------
// stddev

// this implementation comes from:
// https://www.johndcook.com/blog/standard_deviation/

struct grouping_stddev {
    long count;
    calculated_number m_oldM, m_newM, m_oldS, m_newS;
};

void *grouping_create_stddev(RRDR *r) {
    long entries = r->group;
    if(entries < 0) entries = 0;

    return callocz(1, sizeof(struct grouping_stddev) + entries * sizeof(LONG_DOUBLE));
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_stddev(RRDR *r) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->internal.grouping_data;
    g->count = 0;
}

void grouping_free_stddev(RRDR *r) {
    freez(r->internal.grouping_data);
    r->internal.grouping_data = NULL;
}

void grouping_add_stddev(RRDR *r, calculated_number value) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->internal.grouping_data;

    if(calculated_number_isnumber(value)) {
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
}

static inline calculated_number mean(struct grouping_stddev *g) {
    return (g->count > 0) ? g->m_newM : 0.0;
}

static inline calculated_number variance(struct grouping_stddev *g) {
    return ( (g->count > 1) ? g->m_newS/(g->count - 1) : 0.0 );
}
static inline calculated_number stddev(struct grouping_stddev *g) {
    return sqrtl(variance(g));
}

calculated_number grouping_flush_stddev(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->internal.grouping_data;

    calculated_number value;

    if(likely(g->count > 1)) {
        value = stddev(g);

        if(!calculated_number_isnumber(value)) {
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
calculated_number grouping_flush_coefficient_of_variation(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->internal.grouping_data;

    calculated_number value;

    if(likely(g->count > 1)) {
        calculated_number m = mean(g);
        value = 100.0 * stddev(g) / ((m < 0)? -m : m);

        if(unlikely(!calculated_number_isnumber(value))) {
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
calculated_number grouping_flush_mean(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->internal.grouping_data;

    calculated_number value;

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
calculated_number grouping_flush_variance(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->internal.grouping_data;

    calculated_number value;

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