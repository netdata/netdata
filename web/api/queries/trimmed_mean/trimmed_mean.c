// SPDX-License-Identifier: GPL-3.0-or-later

#include "trimmed_mean.h"

// ----------------------------------------------------------------------------
// median

struct grouping_trimmed_mean {
    size_t series_size;
    size_t next_pos;
    NETDATA_DOUBLE percent;

    NETDATA_DOUBLE *series;
};

static void grouping_create_trimmed_mean_internal(RRDR *r, const char *options, NETDATA_DOUBLE def) {
    long entries = r->view.group;
    if(entries < 10) entries = 10;

    struct grouping_trimmed_mean *g = (struct grouping_trimmed_mean *)onewayalloc_callocz(r->internal.owa, 1, sizeof(struct grouping_trimmed_mean));
    g->series = onewayalloc_mallocz(r->internal.owa, entries * sizeof(NETDATA_DOUBLE));
    g->series_size = (size_t)entries;

    g->percent = def;
    if(options && *options) {
        g->percent = str2ndd(options, NULL);
        if(!netdata_double_isnumber(g->percent)) g->percent = 0.0;
        if(g->percent < 0.0) g->percent = 0.0;
        if(g->percent > 50.0) g->percent = 50.0;
    }

    g->percent = 1.0 - ((g->percent / 100.0) * 2.0);
    r->grouping.data = g;
}

void grouping_create_trimmed_mean1(RRDR *r, const char *options) {
    grouping_create_trimmed_mean_internal(r, options, 1.0);
}
void grouping_create_trimmed_mean2(RRDR *r, const char *options) {
    grouping_create_trimmed_mean_internal(r, options, 2.0);
}
void grouping_create_trimmed_mean3(RRDR *r, const char *options) {
    grouping_create_trimmed_mean_internal(r, options, 3.0);
}
void grouping_create_trimmed_mean5(RRDR *r, const char *options) {
    grouping_create_trimmed_mean_internal(r, options, 5.0);
}
void grouping_create_trimmed_mean10(RRDR *r, const char *options) {
    grouping_create_trimmed_mean_internal(r, options, 10.0);
}
void grouping_create_trimmed_mean15(RRDR *r, const char *options) {
    grouping_create_trimmed_mean_internal(r, options, 15.0);
}
void grouping_create_trimmed_mean20(RRDR *r, const char *options) {
    grouping_create_trimmed_mean_internal(r, options, 20.0);
}
void grouping_create_trimmed_mean25(RRDR *r, const char *options) {
    grouping_create_trimmed_mean_internal(r, options, 25.0);
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_trimmed_mean(RRDR *r) {
    struct grouping_trimmed_mean *g = (struct grouping_trimmed_mean *)r->grouping.data;
    g->next_pos = 0;
}

void grouping_free_trimmed_mean(RRDR *r) {
    struct grouping_trimmed_mean *g = (struct grouping_trimmed_mean *)r->grouping.data;
    if(g) onewayalloc_freez(r->internal.owa, g->series);

    onewayalloc_freez(r->internal.owa, r->grouping.data);
    r->grouping.data = NULL;
}

void grouping_add_trimmed_mean(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_trimmed_mean *g = (struct grouping_trimmed_mean *)r->grouping.data;

    if(unlikely(g->next_pos >= g->series_size)) {
        g->series = onewayalloc_doublesize( r->internal.owa, g->series, g->series_size * sizeof(NETDATA_DOUBLE));
        g->series_size *= 2;
    }

    g->series[g->next_pos++] = value;
}

NETDATA_DOUBLE grouping_flush_trimmed_mean(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_trimmed_mean *g = (struct grouping_trimmed_mean *)r->grouping.data;

    NETDATA_DOUBLE value;
    size_t available_slots = g->next_pos;

    if(unlikely(!available_slots)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else if(available_slots == 1) {
        value = g->series[0];
    }
    else {
        sort_series(g->series, available_slots);

        NETDATA_DOUBLE min = g->series[0];
        NETDATA_DOUBLE max = g->series[available_slots - 1];

        if (min != max) {
            size_t slots_to_use = (size_t)((NETDATA_DOUBLE)available_slots * g->percent);
            if(!slots_to_use) slots_to_use = 1;

            NETDATA_DOUBLE percent_to_use = (NETDATA_DOUBLE)slots_to_use / (NETDATA_DOUBLE)available_slots;
            NETDATA_DOUBLE percent_delta = g->percent - percent_to_use;

            NETDATA_DOUBLE percent_interpolation_slot = 0.0;
            NETDATA_DOUBLE percent_last_slot = 0.0;
            if(percent_delta > 0.0) {
                NETDATA_DOUBLE percent_to_use_plus_1_slot = (NETDATA_DOUBLE)(slots_to_use + 1) / (NETDATA_DOUBLE)available_slots;
                NETDATA_DOUBLE percent_1slot = percent_to_use_plus_1_slot - percent_to_use;

                percent_interpolation_slot = percent_delta / percent_1slot;
                percent_last_slot = 1 - percent_interpolation_slot;
            }

            int start_slot, stop_slot, step, last_slot, interpolation_slot;
            if(min >= 0.0 && max >= 0.0) {
                start_slot = (int)((available_slots - slots_to_use) / 2);
                stop_slot = start_slot + (int)slots_to_use;
                last_slot = stop_slot - 1;
                interpolation_slot = stop_slot;
                step = 1;
            }
            else {
                start_slot = (int)available_slots - 1 - (int)((available_slots - slots_to_use) / 2);
                stop_slot = start_slot - (int)slots_to_use;
                last_slot = stop_slot + 1;
                interpolation_slot = stop_slot;
                step = -1;
            }

            value = 0.0;
            for(int slot = start_slot; slot != stop_slot ; slot += step)
                value += g->series[slot];

            size_t counted = slots_to_use;
            if(percent_interpolation_slot > 0.0 && interpolation_slot >= 0 && interpolation_slot < (int)available_slots) {
                value += g->series[interpolation_slot] * percent_interpolation_slot;
                value += g->series[last_slot] * percent_last_slot;
                counted++;
            }

            value = value / (NETDATA_DOUBLE)counted;
        }
        else
            value = min;
    }

    if(unlikely(!netdata_double_isnumber(value))) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }

    //log_series_to_stderr(g->series, g->next_pos, value, "trimmed_mean");

    g->next_pos = 0;

    return value;
}
