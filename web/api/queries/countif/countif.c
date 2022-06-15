// SPDX-License-Identifier: GPL-3.0-or-later

#include "countif.h"

// ----------------------------------------------------------------------------
// countif

struct grouping_countif {
    size_t (*comparison)(calculated_number, calculated_number);
    calculated_number target;
    size_t count;
    size_t matched;
};

static size_t countif_equal(calculated_number v, calculated_number target) {
    return (v == target);
}

static size_t countif_notequal(calculated_number v, calculated_number target) {
    return (v != target);
}

static size_t countif_less(calculated_number v, calculated_number target) {
    return (v < target);
}

static size_t countif_lessequal(calculated_number v, calculated_number target) {
    return (v <= target);
}

static size_t countif_greater(calculated_number v, calculated_number target) {
    return (v > target);
}

static size_t countif_greaterequal(calculated_number v, calculated_number target) {
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

        g->target = str2ld(options, NULL);
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

void grouping_add_countif(RRDR *r, calculated_number value) {
    struct grouping_countif *g = (struct grouping_countif *)r->internal.grouping_data;
    g->matched += g->comparison(value, g->target);
    g->count++;
}

calculated_number grouping_flush_countif(RRDR *r,  RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_countif *g = (struct grouping_countif *)r->internal.grouping_data;

    calculated_number value;

    if(unlikely(!g->count)) {
        value = 0.0;
        *rrdr_value_options_ptr |= RRDR_VALUE_EMPTY;
    }
    else {
        value = (calculated_number)g->matched / (calculated_number)g->count;
    }

    g->matched = 0;
    g->count = 0;

    return value;
}
