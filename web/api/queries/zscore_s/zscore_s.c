// SPDX-License-Identifier: GPL-3.0-or-later

#include "zscore_s.h"

// ----------------------------------------------------------------------------
// stddev

// this implementation comes from:
// https://www.johndcook.com/blog/standard_deviation/

struct stats_zscore {
    long count;
    calculated_number m_oldM, m_newM, m_oldS, m_newS, value;
};

void *stats_create_zscore(RRDR *r) {
    UNUSED (r);
    return callocz(1, sizeof(struct stats_zscore));
}

// resets when switches dimensions
// so, clear everything to restart
void stats_reset_zscore(RRDR *r, int index) {
    struct stats_zscore *g = (struct stats_zscore *)r->stats[index].stat_data;
    g->count = 0;
}

void stats_free_zscore(RRDR *r, int index) {
    freez(r->stats[index].stat_data);
    r->stats[index].stat_data = NULL;
}

void stats_add_zscore(RRDR *r, calculated_number value, int index) {
    struct stats_zscore *g = (struct stats_zscore *)r->stats[index].stat_data;

    if(calculated_number_isnumber(value)) {
        g->count++;
        g->value = value;
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

static inline calculated_number value(struct stats_zscore *g) {
    return (g->value > 0) ? g->value : 0.0;
}

static inline calculated_number mean(struct stats_zscore *g) {
    return (g->count > 0) ? g->m_newM : 0.0;
}

static inline calculated_number variance(struct stats_zscore *g) {
    return ( (g->count > 1) ? g->m_newS/(g->count - 1) : 0.0 );
}
static inline calculated_number zscore(struct stats_zscore *g) {
    calculated_number sigma = sqrtl(variance(g));
    if(!sigma)
        return 0;
    return (value(g) - mean(g)) / sqrtl(variance(g));
}

calculated_number stats_flush_zscore(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr, int index) {
    struct stats_zscore *g = (struct stats_zscore *)r->stats[index].stat_data;

    calculated_number value;

    if(likely(g->count > 1)) {
        value = zscore(g);

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

    stats_reset_zscore(r, index);

    return  value;
}
