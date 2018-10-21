// SPDX-License-Identifier: GPL-3.0-or-later

#include "stddev.h"


// ----------------------------------------------------------------------------
// stddev

struct grouping_stddev {
    size_t series_size;
    size_t next_pos;

    LONG_DOUBLE series[];
};

void *grouping_init_stddev(RRDR *r) {
    long entries = (r->group > r->group_points) ? r->group : r->group_points;
    if(entries < 0) entries = 0;

    struct grouping_stddev *g = (struct grouping_stddev *)callocz(1, sizeof(struct grouping_stddev) + entries * sizeof(LONG_DOUBLE));
    g->series_size = (size_t)entries;

    return g;
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_stddev(RRDR *r) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping_data;
    g->next_pos = 0;
}

void grouping_free_stddev(RRDR *r) {
    freez(r->grouping_data);
}

void grouping_add_stddev(RRDR *r, calculated_number value) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping_data;

    if(unlikely(g->next_pos >= g->series_size)) {
        error("INTERNAL ERROR: stddev buffer overflow on chart '%s' - next_pos = %zu, series_size = %zu, r->group = %ld, r->group_points = %ld.", r->st->name, g->next_pos, g->series_size, r->group, r->group_points);
    }
    else {
        if(isnormal(value))
            g->series[g->next_pos++] = (LONG_DOUBLE)value;
    }
}

calculated_number grouping_flush_stddev(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping_data;

    calculated_number value;

    if(unlikely(!g->next_pos)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = standard_deviation(g->series, g->next_pos);

        if(!isnormal(value)) {
            value = 0.0;
            *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        }

        //log_series_to_stderr(g->series, g->next_pos, value, "stddev");
    }

    g->next_pos = 0;

    return  value;
}

