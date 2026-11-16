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
        if(format == DATASOURCE_CSV_JSON_ARRAY)
            // the array format is JSON: a quote or backslash in a
            // dimension name (or a label value under group_by=label)
            // must be escaped or the payload breaks
            buffer_json_strcat(wb, string2str(r->dn[c]));
        else if(format == DATASOURCE_HTML)
            // the html format is served as text/html: a dimension name
            // must not be able to add markup to the page
            buffer_strcat_htmlescape(wb, string2str(r->dn[c]));
        else
            buffer_strcat(wb, string2str(r->dn[c]));
        if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
        i++;
    }

    if(!i) {
        // no dimensions present
        return;
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

// --------------------------------------------------------------------------------------------------------------------
// unittest

static int rrdr2csv_unittest_case(
    const char *description,
    uint32_t format,
    const char *startline,
    const char *separator,
    const char *endline,
    const char *dimension_name,
    const char *expected) {
    // a header-only result: one queried dimension, no rows
    STRING *dn = string_strdupz(dimension_name);
    RRDR_DIMENSION_FLAGS od = RRDR_DIMENSION_QUERIED;
    RRDR r = {
        .d = 1,
        .rows = 0,
        .od = &od,
        .dn = &dn,
    };

    int errors = 0;
    BUFFER *wb = buffer_create(0, NULL);
    rrdr2csv(&r, wb, format, 0, startline, separator, endline, "");

    if(strcmp(buffer_tostring(wb), expected) != 0) {
        fprintf(stderr, "rrdr2csv %s header mismatch\nexpected: %s\nactual:   %s\n",
                description, expected, buffer_tostring(wb));
        errors++;
    }

    buffer_free(wb);
    string_freez(dn);
    return errors;
}

int rrdr2csv_unittest(void) {
    int errors = 0;
    const char *hostile = "<img src=x onerror=\"alert('&')\">";

    // html is served as text/html: the name must not add markup
    errors += rrdr2csv_unittest_case(
        "html", DATASOURCE_HTML, "<tr><td>", "</td><td>", "</td></tr>\n", hostile,
        "<tr><td>time</td><td>&lt;img src=x onerror=&quot;alert(&#x27;&amp;&#x27;)&quot;&gt;</td></tr>\n");

    // plain csv is not markup: the name is written as it is
    errors += rrdr2csv_unittest_case(
        "csv", DATASOURCE_CSV, "", ",", "\r\n", hostile,
        "time,<img src=x onerror=\"alert('&')\">\r\n");

    return errors;
}
