// SPDX-License-Identifier: GPL-3.0-or-later

#include "median.h"


// ----------------------------------------------------------------------------
// median

struct grouping_median {
    size_t series_size;
    size_t next_pos;

    NETDATA_DOUBLE series[];
};

void grouping_create_median(RRDR *r, const char *options __maybe_unused) {
    long entries = r->group;
    if(entries < 0) entries = 0;

    struct grouping_median *g = (struct grouping_median *)callocz(1, sizeof(struct grouping_median) + entries * sizeof(NETDATA_DOUBLE));
    g->series_size = (size_t)entries;

    r->internal.grouping_data = g;
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_median(RRDR *r) {
    struct grouping_median *g = (struct grouping_median *)r->internal.grouping_data;
    g->next_pos = 0;
}

void grouping_free_median(RRDR *r) {
    freez(r->internal.grouping_data);
    r->internal.grouping_data = NULL;
}

void grouping_add_median(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_median *g = (struct grouping_median *)r->internal.grouping_data;

    if(unlikely(g->next_pos >= g->series_size)) {
        error("INTERNAL ERROR: median buffer overflow on chart '%s' - next_pos = %zu, series_size = %zu, r->group = %ld.", r->st->name, g->next_pos, g->series_size, r->group);
    }
    else
        g->series[g->next_pos++] = (NETDATA_DOUBLE)value;
}

NETDATA_DOUBLE grouping_flush_median(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_median *g = (struct grouping_median *)r->internal.grouping_data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->next_pos)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        if(g->next_pos > 1) {
            sort_series(g->series, g->next_pos);
            value = (NETDATA_DOUBLE)median_on_sorted_series(g->series, g->next_pos);
        }
        else
            value = (NETDATA_DOUBLE)g->series[0];

        if(!netdata_double_isnumber(value)) {
            value = 0.0;
            *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
        }

        //log_series_to_stderr(g->series, g->next_pos, value, "median");
    }

    g->next_pos = 0;

    return value;
}

