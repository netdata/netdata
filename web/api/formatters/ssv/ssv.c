// SPDX-License-Identifier: GPL-3.0-or-later

#include "ssv.h"

void rrdr2ssv(RRDR *r, BUFFER *wb, RRDR_OPTIONS options, const char *prefix, const char *separator, const char *suffix) {
    //netdata_log_info("RRD2SSV(): %s: BEGIN", r->st->id);
    long i;

    buffer_strcat(wb, prefix);
    long start = 0, end = rrdr_rows(r), step = 1;
    if(!(options & RRDR_OPTION_REVERSED)) {
        start = rrdr_rows(r) - 1;
        end = -1;
        step = -1;
    }

    // for each line in the array
    for(i = start; i != end ;i += step) {
        int all_values_are_null = 0;
        NETDATA_DOUBLE v = rrdr2value(r, i, options, &all_values_are_null, NULL);

        if(likely(i != start)) {
            if(r->view.min > v) r->view.min = v;
            if(r->view.max < v) r->view.max = v;
        }
        else {
            r->view.min = v;
            r->view.max = v;
        }

        if(likely(i != start))
            buffer_strcat(wb, separator);

        if(all_values_are_null) {
            if(options & RRDR_OPTION_NULL2ZERO)
                buffer_strcat(wb, "0");
            else
                buffer_strcat(wb, "null");
        }
        else
            buffer_print_netdata_double(wb, v);
    }
    buffer_strcat(wb, suffix);
    //netdata_log_info("RRD2SSV(): %s: END", r->st->id);
}
