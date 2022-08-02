// SPDX-License-Identifier: GPL-3.0-or-later

#include "percentile.h"

// ----------------------------------------------------------------------------
// median

struct grouping_percentile {
    size_t series_size;
    size_t next_pos;
    NETDATA_DOUBLE percent;

    NETDATA_DOUBLE *series;
};

static void grouping_create_percentile_internal(RRDR *r, const char *options, NETDATA_DOUBLE def) {
    long entries = r->group;
    if(entries < 10) entries = 10;

    struct grouping_percentile *g = (struct grouping_percentile *)onewayalloc_callocz(r->internal.owa, 1, sizeof(struct grouping_percentile));
    g->series = onewayalloc_mallocz(r->internal.owa, entries * sizeof(NETDATA_DOUBLE));
    g->series_size = (size_t)entries;

    g->percent = def;
    if(options && *options) {
        g->percent = str2ndd(options, NULL);
        if(!netdata_double_isnumber(g->percent)) g->percent = 0.0;
        if(g->percent < 0.0) g->percent = 0.0;
        if(g->percent > 100.0) g->percent = 100.0;
    }

    g->percent = 1.0 - g->percent / 100.0;
    r->internal.grouping_data = g;
}

void grouping_create_percentile25(RRDR *r, const char *options) {
    grouping_create_percentile_internal(r, options, 25.0);
}
void grouping_create_percentile50(RRDR *r, const char *options) {
    grouping_create_percentile_internal(r, options, 50.0);
}
void grouping_create_percentile75(RRDR *r, const char *options) {
    grouping_create_percentile_internal(r, options, 75.0);
}
void grouping_create_percentile80(RRDR *r, const char *options) {
    grouping_create_percentile_internal(r, options, 80.0);
}
void grouping_create_percentile90(RRDR *r, const char *options) {
    grouping_create_percentile_internal(r, options, 90.0);
}
void grouping_create_percentile95(RRDR *r, const char *options) {
    grouping_create_percentile_internal(r, options, 95.0);
}
void grouping_create_percentile97(RRDR *r, const char *options) {
    grouping_create_percentile_internal(r, options, 97.0);
}
void grouping_create_percentile98(RRDR *r, const char *options) {
    grouping_create_percentile_internal(r, options, 98.0);
}
void grouping_create_percentile99(RRDR *r, const char *options) {
    grouping_create_percentile_internal(r, options, 99.0);
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_percentile(RRDR *r) {
    struct grouping_percentile *g = (struct grouping_percentile *)r->internal.grouping_data;
    g->next_pos = 0;
}

void grouping_free_percentile(RRDR *r) {
    struct grouping_percentile *g = (struct grouping_percentile *)r->internal.grouping_data;
    if(g) onewayalloc_freez(r->internal.owa, g->series);

    onewayalloc_freez(r->internal.owa, r->internal.grouping_data);
    r->internal.grouping_data = NULL;
}

void grouping_add_percentile(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_percentile *g = (struct grouping_percentile *)r->internal.grouping_data;

    if(unlikely(g->next_pos >= g->series_size)) {
        g->series = onewayalloc_doublesize( r->internal.owa, g->series, g->series_size * sizeof(NETDATA_DOUBLE));
        g->series_size *= 2;
    }

    g->series[g->next_pos++] = value;
}

NETDATA_DOUBLE grouping_flush_percentile(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_percentile *g = (struct grouping_percentile *)r->internal.grouping_data;

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
            size_t counted = 1;

            if (min <= 0.0 && max <= 0.0) {
                NETDATA_DOUBLE wanted_min = min - (min - max) * g->percent;

                // fprintf(stderr, "min = %f, max = %f, wanted_min = %f, percent = %f\n", min, max, wanted_min, g->percent);

                value = g->series[available_slots - 1];
                for (int slot = (int)available_slots - 2; slot >= 0; slot--) {
                    NETDATA_DOUBLE v = g->series[slot];
                    if (likely(v >= wanted_min)) {
                        value += v;
                        counted++;
                    } else
                        break;
                }
            }
            else {
                NETDATA_DOUBLE wanted_max = max - (max - min) * g->percent;

                // fprintf(stderr, "min = %f, max = %f, wanted_max = %f, percent = %f\n", min, max, wanted_max, g->percent);

                value = g->series[0];
                for (int slot = 1; slot < (int)available_slots; slot++) {
                    NETDATA_DOUBLE v = g->series[slot];
                    if (likely(v <= wanted_max)) {
                        value += v;
                        counted++;
                    } else
                        break;
                }
            }

            value = value / (NETDATA_DOUBLE)counted;
        }
        else
            value = g->series[0];
    }

    if(unlikely(!netdata_double_isnumber(value))) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }

    //log_series_to_stderr(g->series, g->next_pos, value, "percentile");

    g->next_pos = 0;

    return value;
}
