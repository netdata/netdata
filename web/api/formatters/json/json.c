// SPDX-License-Identifier: GPL-3.0-or-later

#include "json.h"

#define JSON_DATES_JS 1
#define JSON_DATES_TIMESTAMP 2

void rrdr2json(RRDR *r, BUFFER *wb, RRDR_OPTIONS options, int datatable) {
    rrdset_check_rdlock(r->st);

    //info("RRD2JSON(): %s: BEGIN", r->st->id);
    int row_annotations = 0, dates, dates_with_new = 0;
    char kq[2] = "",                    // key quote
            sq[2] = "",                     // string quote
            pre_label[101] = "",            // before each label
            post_label[101] = "",           // after each label
            pre_date[101] = "",             // the beginning of line, to the date
            post_date[101] = "",            // closing the date
            pre_value[101] = "",            // before each value
            post_value[101] = "",           // after each value
            post_line[101] = "",            // at the end of each row
            normal_annotation[201] = "",    // default row annotation
            overflow_annotation[201] = "",  // overflow row annotation
            data_begin[101] = "",           // between labels and values
            finish[101] = "";               // at the end of everything

    if(datatable) {
        dates = JSON_DATES_JS;
        if( options & RRDR_OPTION_GOOGLE_JSON ) {
            kq[0] = '\0';
            sq[0] = '\'';
        }
        else {
            kq[0] = '"';
            sq[0] = '"';
        }
        row_annotations = 1;
        snprintfz(pre_date,   100, "        {%sc%s:[{%sv%s:%s", kq, kq, kq, kq, sq);
        snprintfz(post_date,  100, "%s}", sq);
        snprintfz(pre_label,  100, ",\n     {%sid%s:%s%s,%slabel%s:%s", kq, kq, sq, sq, kq, kq, sq);
        snprintfz(post_label, 100, "%s,%spattern%s:%s%s,%stype%s:%snumber%s}", sq, kq, kq, sq, sq, kq, kq, sq, sq);
        snprintfz(pre_value,  100, ",{%sv%s:", kq, kq);
        strcpy(post_value,         "}");
        strcpy(post_line,          "]}");
        snprintfz(data_begin, 100, "\n  ],\n    %srows%s:\n [\n", kq, kq);
        strcpy(finish,             "\n  ]\n}");

        snprintfz(overflow_annotation, 200, ",{%sv%s:%sRESET OR OVERFLOW%s},{%sv%s:%sThe counters have been wrapped.%s}", kq, kq, sq, sq, kq, kq, sq, sq);
        snprintfz(normal_annotation,   200, ",{%sv%s:null},{%sv%s:null}", kq, kq, kq, kq);

        buffer_sprintf(wb, "{\n %scols%s:\n [\n", kq, kq);
        buffer_sprintf(wb, "        {%sid%s:%s%s,%slabel%s:%stime%s,%spattern%s:%s%s,%stype%s:%sdatetime%s},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq);
        buffer_sprintf(wb, "        {%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotation%s}},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);
        buffer_sprintf(wb, "        {%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotationText%s}}", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);

        // remove the valueobjects flag
        // google wants its own keys
        if(options & RRDR_OPTION_OBJECTSROWS)
            options &= ~RRDR_OPTION_OBJECTSROWS;
    }
    else {
        kq[0] = '"';
        sq[0] = '"';
        if(options & RRDR_OPTION_GOOGLE_JSON) {
            dates = JSON_DATES_JS;
            dates_with_new = 1;
        }
        else {
            dates = JSON_DATES_TIMESTAMP;
            dates_with_new = 0;
        }
        if( options & RRDR_OPTION_OBJECTSROWS )
            strcpy(pre_date, "      { ");
        else
            strcpy(pre_date, "      [ ");
        strcpy(pre_label,  ", \"");
        strcpy(post_label, "\"");
        strcpy(pre_value,  ", ");
        if( options & RRDR_OPTION_OBJECTSROWS )
            strcpy(post_line, "}");
        else
            strcpy(post_line, "]");
        snprintfz(data_begin, 100, "],\n    %sdata%s:\n [\n", kq, kq);
        strcpy(finish,             "\n  ]\n}");

        buffer_sprintf(wb, "{\n %slabels%s: [", kq, kq);
        buffer_sprintf(wb, "%stime%s", sq, sq);
    }

    // -------------------------------------------------------------------------
    // print the JSON header

    long c, i;
    RRDDIM *rd;

    // print the header lines
    for(c = 0, i = 0, rd = r->st->dimensions; rd && c < r->d ;c++, rd = rd->next) {
        if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
        if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

        buffer_strcat(wb, pre_label);
        buffer_strcat(wb, rd->name);
        buffer_strcat(wb, post_label);
        i++;
    }
    if(!i) {
        buffer_strcat(wb, pre_label);
        buffer_strcat(wb, "no data");
        buffer_strcat(wb, post_label);
    }

    // print the begin of row data
    buffer_strcat(wb, data_begin);

    // if all dimensions are hidden, print a null
    if(!i) {
        buffer_strcat(wb, finish);
        return;
    }

    long start = 0, end = rrdr_rows(r), step = 1;
    if(!(options & RRDR_OPTION_REVERSED)) {
        start = rrdr_rows(r) - 1;
        end = -1;
        step = -1;
    }

    // for each line in the array
    calculated_number total = 1;
    for(i = start; i != end ;i += step) {
        calculated_number *cn = &r->v[ i * r->d ];
        RRDR_VALUE_FLAGS *co = &r->o[ i * r->d ];

        time_t now = r->t[i];

        if(dates == JSON_DATES_JS) {
            // generate the local date time
            struct tm tmbuf, *tm = localtime_r(&now, &tmbuf);
            if(!tm) { error("localtime_r() failed."); continue; }

            if(likely(i != start)) buffer_strcat(wb, ",\n");
            buffer_strcat(wb, pre_date);

            if( options & RRDR_OPTION_OBJECTSROWS )
                buffer_sprintf(wb, "%stime%s: ", kq, kq);

            if(dates_with_new)
                buffer_strcat(wb, "new ");

            buffer_jsdate(wb, tm->tm_year + 1900, tm->tm_mon, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec);

            buffer_strcat(wb, post_date);

            if(row_annotations) {
                // google supports one annotation per row
                int annotation_found = 0;
                for(c = 0, rd = r->st->dimensions; rd ;c++, rd = rd->next) {
                    if(co[c] & RRDR_VALUE_RESET) {
                        buffer_strcat(wb, overflow_annotation);
                        annotation_found = 1;
                        break;
                    }
                }
                if(!annotation_found)
                    buffer_strcat(wb, normal_annotation);
            }
        }
        else {
            // print the timestamp of the line
            if(likely(i != start)) buffer_strcat(wb, ",\n");
            buffer_strcat(wb, pre_date);

            if( options & RRDR_OPTION_OBJECTSROWS )
                buffer_sprintf(wb, "%stime%s: ", kq, kq);

            buffer_rrd_value(wb, (calculated_number)r->t[i]);
            // in ms
            if(options & RRDR_OPTION_MILLISECONDS) buffer_strcat(wb, "000");

            buffer_strcat(wb, post_date);
        }

        int set_min_max = 0;
        if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
            total = 0;
            for(c = 0, rd = r->st->dimensions; rd && c < r->d ;c++, rd = rd->next) {
                calculated_number n = cn[c];

                if(likely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                    n = -n;

                total += n;
            }
            // prevent a division by zero
            if(total == 0) total = 1;
            set_min_max = 1;
        }

        // for each dimension
        for(c = 0, rd = r->st->dimensions; rd && c < r->d ;c++, rd = rd->next) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
            if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

            calculated_number n = cn[c];

            buffer_strcat(wb, pre_value);

            if( options & RRDR_OPTION_OBJECTSROWS )
                buffer_sprintf(wb, "%s%s%s: ", kq, rd->name, kq);

            if(co[c] & RRDR_VALUE_EMPTY) {
                if(options & RRDR_OPTION_NULL2ZERO)
                    buffer_strcat(wb, "0");
                else
                    buffer_strcat(wb, "null");
            }
            else {
                if(unlikely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
                    n = -n;

                if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
                    n = n * 100 / total;

                    if(unlikely(set_min_max)) {
                        r->min = r->max = n;
                        set_min_max = 0;
                    }

                    if(n < r->min) r->min = n;
                    if(n > r->max) r->max = n;
                }

                buffer_rrd_value(wb, n);
            }

            buffer_strcat(wb, post_value);
        }

        buffer_strcat(wb, post_line);
    }

    buffer_strcat(wb, finish);
    //info("RRD2JSON(): %s: END", r->st->id);
}
