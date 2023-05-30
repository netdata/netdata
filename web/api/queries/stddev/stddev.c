// SPDX-License-Identifier: GPL-3.0-or-later

#include "stddev.h"


// ----------------------------------------------------------------------------
// stddev

/*
 * Mean = average
 *
NETDATA_DOUBLE grouping_flush_mean(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping.grouping_data;

    NETDATA_DOUBLE value;

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
NETDATA_DOUBLE grouping_flush_variance(RRDR *r, RRDR_VALUE_FLAGS *rrdr_value_options_ptr) {
    struct grouping_stddev *g = (struct grouping_stddev *)r->grouping.grouping_data;

    NETDATA_DOUBLE value;

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