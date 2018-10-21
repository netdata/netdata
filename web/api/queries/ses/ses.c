// SPDX-License-Identifier: GPL-3.0-or-later

#include "ses.h"


// ----------------------------------------------------------------------------
// single exponential smoothing

struct grouping_ses {
    size_t series_size;
    size_t next_pos;

    LONG_DOUBLE series[];
};

void *grouping_init_ses(RRDR *r) {
    long entries = (r->group > r->group_points) ? r->group : r->group_points;
    if(entries < 0) entries = 0;

    struct grouping_ses *g = (struct grouping_ses *)callocz(1, sizeof(struct grouping_ses) + entries * sizeof(LONG_DOUBLE));
    g->series_size = (size_t)entries;

    return g;
}

void grouping_reset_ses(RRDR *r) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;
    g->next_pos = 0;
}

void grouping_free_ses(RRDR *r) {
    freez(r->grouping_data);
}

void grouping_add_ses(RRDR *r, calculated_number value) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;

    if(unlikely(g->next_pos >= g->series_size)) {
        error("INTERNAL ERROR: single exponential smoothing buffer overflow on chart '%s' - next_pos = %zu, series_size = %zu, r->group = %ld, r->group_points = %ld.", r->st->name, g->next_pos, g->series_size, r->group, r->group_points);
    }
    else {
        if(isnormal(value))
            g->series[g->next_pos++] = (LONG_DOUBLE)value;
    }
}

void grouping_flush_ses(RRDR *r, calculated_number *rrdr_value_ptr, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_ses *g = (struct grouping_ses *)r->grouping_data;

    if(unlikely(!g->next_pos)) {
        *rrdr_value_ptr = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        calculated_number value = single_exponential_smoothing_reverse(g->series, g->next_pos, 1.0 / g->next_pos / 2);

        if(!isnormal(value)) {
            *rrdr_value_ptr = 0.0;
            *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        }
        else {
            *rrdr_value_ptr = value;
        }

        //log_series_to_stderr(g->series, g->next_pos, *rrdr_value_ptr, "ses");
    }

    g->next_pos = 0;
}

