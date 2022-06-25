// SPDX-License-Identifier: GPL-3.0-or-later

#include "countif.h"

// ----------------------------------------------------------------------------
// countif

struct grouping_countif {
    size_t (*comparison)(NETDATA_DOUBLE, NETDATA_DOUBLE);
    NETDATA_DOUBLE target;
    size_t count;
    size_t matched;
};

static size_t countif_equal(NETDATA_DOUBLE v, NETDATA_DOUBLE target) {
    return (v == target);
}

static size_t countif_notequal(NETDATA_DOUBLE v, NETDATA_DOUBLE target) {
    return (v != target);
}

static size_t countif_less(NETDATA_DOUBLE v, NETDATA_DOUBLE target) {
    return (v < target);
}

static size_t countif_lessequal(NETDATA_DOUBLE v, NETDATA_DOUBLE target) {
    return (v <= target);
}

static size_t countif_greater(NETDATA_DOUBLE v, NETDATA_DOUBLE target) {
    return (v > target);
}

static size_t countif_greaterequal(NETDATA_DOUBLE v, NETDATA_DOUBLE target) {
    return (v >= target);
}

void grouping_create_countif(RRDR *r, const char *options __maybe_unused) {
    struct grouping_countif *g = callocz(1, sizeof(struct grouping_countif));
    r->internal.grouping_data = g;

    if(options && *options) {
        // skip any leading spaces
        while(isspace(*options)) options++;

        // find the comparison function
        switch(*options) {
            case '!':
                options++;
                if(*options != '=' && *options != ':')
                    options--;
                g->comparison = countif_notequal;
                break;

            case '>':
                options++;
                if(*options == '=' || *options == ':') {
                    g->comparison = countif_greaterequal;
                }
                else {
                    options--;
                    g->comparison = countif_greater;
                }
                break;

            case '<':
                options++;
                if(*options == '>') {
                    g->comparison = countif_notequal;
                }
                else if(*options == '=' || *options == ':') {
                    g->comparison = countif_lessequal;
                }
                else {
                    options--;
                    g->comparison = countif_less;
                }
                break;

            default:
            case '=':
            case ':':
                g->comparison = countif_equal;
                break;
        }
        if(*options) options++;

        // skip everything up to the first digit
        while(isspace(*options)) options++;

        g->target = str2ndd(options, NULL);
    }
    else {
        g->target = 0.0;
        g->comparison = countif_equal;
    }
}

// resets when switches dimensions
// so, clear everything to restart
void grouping_reset_countif(RRDR *r) {
    struct grouping_countif *g = (struct grouping_countif *)r->internal.grouping_data;
    g->matched = 0;
    g->count = 0;
}

void grouping_free_countif(RRDR *r) {
    freez(r->internal.grouping_data);
    r->internal.grouping_data = NULL;
}

void grouping_add_countif(RRDR *r, NETDATA_DOUBLE value) {
    struct grouping_countif *g = (struct grouping_countif *)r->internal.grouping_data;
    g->matched += g->comparison(value, g->target);
    g->count++;
}

NETDATA_DOUBLE grouping_flush_countif(RRDR *r,  RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_countif *g = (struct grouping_countif *)r->internal.grouping_data;

    NETDATA_DOUBLE value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = (NETDATA_DOUBLE)g->matched * 100 / (NETDATA_DOUBLE)g->count;
    }

    g->matched = 0;
    g->count = 0;

    return value;
}
