// SPDX-License-Identifier: GPL-3.0-or-later

#include "median.h"

// ----------------------------------------------------------------------------
// median

struct grouping_median {
    size_t series_size;
    size_t next_pos;
    NETDATA_DOUBLE percent;

    NETDATA_DOUBLE *series;
};

void grouping_create_median_internal(RRDR *r, const char *options, NETDATA_DOUBLE def) {
    long entries = r->view.group;
    if(entries < 10) entries = 10;

    struct grouping_median *g = (struct grouping_median *)onewayalloc_callocz(r->internal.owa, 1, sizeof(struct grouping_median));
    g->series = onewayalloc_mallocz(r->internal.owa, entries * sizeof(NETDATA_DOUBLE));
    g->series_size = (size_t)entries;

    g->percent = def;
    if(options && *options) {
        g->percent = str2ndd(options, NULL);
        if(!netdata_double_isnumber(g->percent)) g->percent = 0.0;
        if(g->percent < 0.0) g->percent = 0.0;
        if(g->percent > 50.0) g->percent = 50.0;
    }

    g->percent = g->percent / 100.0;
    r->grouping.data = g;
}

void grouping_create_median(RRDR *r, const char *options) {
    grouping_create_median_internal(r, options, 0.0);
}
void grouping_create_trimmed_median1(RRDR *r, const char *options) {
    grouping_create_median_internal(r, options, 1.0);
}
void grouping_create_trimmed_median2(RRDR *r, const char *options) {
    grouping_create_median_internal(r, options, 2.0);
}
void grouping_create_trimmed_median3(RRDR *r, const char *options) {
    grouping_create_median_internal(r, options, 3.0);
}
void grouping_create_trimmed_median5(RRDR *r, const char *options) {
    grouping_create_median_internal(r, options, 5.0);
}
void grouping_create_trimmed_median10(RRDR *r, const char *options) {
    grouping_create_median_internal(r, options, 10.0);
}
void grouping_create_trimmed_median15(RRDR *r, const char *options) {
    grouping_create_median_internal(r, options, 15.0);
}
void grouping_create_trimmed_median20(RRDR *r, const char *options) {
    grouping_create_median_internal(r, options, 20.0);
}
void grouping_create_trimmed_median25(RRDR *r, const char *options) {
    grouping_create_median_internal(r, options, 25.0);
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_median(RRDR *r) {
    struct grouping_median *g = (struct grouping_median *)r->grouping.data;
    g->next_pos = 0;
}

void grouping_free_median(RRDR *r) {
    struct grouping_median *g = (struct grouping_median *)r->grouping.data;
    if(g) onewayalloc_freez(r->internal.owa, g->series);

    onewayalloc_freez(r->internal.owa, r->grouping.data);
    r->grouping.data = NULL;
}

void grouping_add_median(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_median *g = (struct grouping_median *)r->grouping.data;

    if(unlikely(g->next_pos >= g->series_size)) {
        g->series = onewayalloc_doublesize( r->internal.owa, g->series, g->series_size * sizeof(NETDATA_DOUBLE));
        g->series_size *= 2;
    }

    g->series[g->next_pos++] = value;
}

NETDATA_DOUBLE grouping_flush_median(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_median *g = (struct grouping_median *)r->grouping.data;

    size_t available_slots = g->next_pos;
    NETDATA_DOUBLE value;

    if(unlikely(!available_slots)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else if(available_slots == 1) {
        value = g->series[0];
    }
    else {
        sort_series(g->series, available_slots);

        size_t start_slot = 0;
        size_t end_slot = available_slots - 1;

        if(g->percent > 0.0) {
            NETDATA_DOUBLE min = g->series[0];
            NETDATA_DOUBLE max = g->series[available_slots - 1];
            NETDATA_DOUBLE delta = (max - min) * g->percent;

            NETDATA_DOUBLE wanted_min = min + delta;
            NETDATA_DOUBLE wanted_max = max - delta;

            for (start_slot = 0; start_slot < available_slots; start_slot++)
                if (g->series[start_slot] >= wanted_min) break;

            for (end_slot = available_slots - 1; end_slot > start_slot; end_slot--)
                if (g->series[end_slot] <= wanted_max) break;
        }

        if(start_slot == end_slot)
            value = g->series[start_slot];
        else
            value = median_on_sorted_series(&g->series[start_slot], end_slot - start_slot + 1);
    }

    if(unlikely(!netdata_double_isnumber(value))) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }

    //log_series_to_stderr(g->series, g->next_pos, value, "median");

    g->next_pos = 0;

    return value;
}
