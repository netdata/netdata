// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "csv.h"

void rrdr2csv(RRDR *r, BUFFER *wb, uint32_t format, RRDR_OPTIONS options, const char *startline, const char *separator, const char *endline, const char *betweenlines) {
    //netdata_log_info("RRD2CSV(): %s: BEGIN", r->st->id);
    long c, i;
    const long used = (long)r->d;

    // print the csv header
    for(c = 0, i = 0; c < used ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        if(!i) {
            buffer_strcat(wb, startline);
            if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
            buffer_strcat(wb, "time");
            if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
        }
        buffer_strcat(wb, separator);
        if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
        buffer_strcat(wb, string2str(r->dn[c]));
        if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
        i++;
    }
    buffer_strcat(wb, endline);

    if(format == DATASOURCE_CSV_MARKDOWN) {
        // print the --- line after header
        for(c = 0, i = 0; c < used ;c++) {
            if(!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            if(!i) {
                buffer_strcat(wb, startline);
                if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
                buffer_strcat(wb, ":---:");
                if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
            }
            buffer_strcat(wb, separator);
            if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
            buffer_strcat(wb, ":---:");
            if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
            i++;
        }
        buffer_strcat(wb, endline);
    }

    if(!i) {
        // no dimensions present
        return;
    }

    long start = 0, end = rrdr_rows(r), step = 1;
    if(!(options & RRDR_OPTION_REVERSED)) {
        start = rrdr_rows(r) - 1;
        end = -1;
        step = -1;
    }

    // for each line in the array
    for(i = start; i != end ;i += step) {
        NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
        RRDR_VALUE_FLAGS *co = &r->o[ i * r->d ];

        buffer_strcat(wb, betweenlines);
        buffer_strcat(wb, startline);

        time_t now = r->t[i];

        if((options & RRDR_OPTION_SECONDS) || (options & RRDR_OPTION_MILLISECONDS)) {
            // print the timestamp of the line
            buffer_print_netdata_double(wb, (NETDATA_DOUBLE) now);
            // in ms
            if(options & RRDR_OPTION_MILLISECONDS) buffer_strcat(wb, "000");
        }
        else {
            // generate the local date time
            struct tm tmbuf, *tm = localtime_r(&now, &tmbuf);
            if(!tm) {
                netdata_log_error("localtime() failed."); continue; }
            buffer_date(wb, tm->tm_year + 1900, tm->tm_mon + 1, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec);
        }

        // for each dimension
        for(c = 0; c < used ;c++) {
            if(!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            buffer_strcat(wb, separator);

            NETDATA_DOUBLE n = cn[c];

            if(co[c] & RRDR_VALUE_EMPTY) {
                if(options & RRDR_OPTION_NULL2ZERO)
                    buffer_strcat(wb, "0");
                else
                    buffer_strcat(wb, "null");
            }
            else
                buffer_print_netdata_double(wb, n);
        }

        buffer_strcat(wb, endline);
    }
    //netdata_log_info("RRD2CSV(): %s: END", r->st->id);
}
