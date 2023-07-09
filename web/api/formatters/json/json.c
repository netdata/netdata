// SPDX-License-Identifier: GPL-3.0-or-later

#include "json.h"

#define JSON_DATES_JS 1
#define JSON_DATES_TIMESTAMP 2

void rrdr2json(RRDR *r, BUFFER *wb, RRDR_OPTIONS options, int datatable) {
    //netdata_log_info("RRD2JSON(): %s: BEGIN", r->st->id);
    int row_annotations = 0, dates, dates_with_new = 0;
    char kq[2] = "",                        // key quote
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
            finish[101] = "",               // at the end of everything
            object_rows_time[101] = "";

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
        strcpy(finish,             "\n        ]\n    }");

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
            strcpy(pre_date, "            {");
        else
            strcpy(pre_date, "            [");
        strcpy(pre_label,  ",\"");
        strcpy(post_label, "\"");
        strcpy(pre_value,  ",");
        if( options & RRDR_OPTION_OBJECTSROWS )
            strcpy(post_line, "}");
        else
            strcpy(post_line, "]");
        snprintfz(data_begin, 100, "],\n        %sdata%s:[\n", kq, kq);
        strcpy(finish,             "\n        ]\n    }");

        buffer_sprintf(wb, "{\n        %slabels%s:[", kq, kq);
        buffer_sprintf(wb, "%stime%s", sq, sq);

        if( options & RRDR_OPTION_OBJECTSROWS )
            snprintfz(object_rows_time, 100, "%stime%s: ", kq, kq);

    }

    size_t pre_value_len = strlen(pre_value);
    size_t post_value_len = strlen(post_value);
    size_t pre_label_len = strlen(pre_label);
    size_t post_label_len = strlen(post_label);
    size_t pre_date_len = strlen(pre_date);
    size_t post_date_len = strlen(post_date);
    size_t post_line_len = strlen(post_line);
    size_t normal_annotation_len = strlen(normal_annotation);
    size_t overflow_annotation_len = strlen(overflow_annotation);
    size_t object_rows_time_len = strlen(object_rows_time);

    // -------------------------------------------------------------------------
    // print the JSON header

    long c, i;
    const long used = (long)r->d;

    // print the header lines
    for(c = 0, i = 0; c < used ; c++) {
        if(!rrdr_dimension_should_be_exposed(r->od[c], options))
            continue;

        buffer_fast_strcat(wb, pre_label, pre_label_len);
        buffer_strcat(wb, string2str(r->dn[c]));
        buffer_fast_strcat(wb, post_label, post_label_len);
        i++;
    }

    if(!i) {
        buffer_fast_strcat(wb, pre_label, pre_label_len);
        buffer_fast_strcat(wb, "no data", 7);
        buffer_fast_strcat(wb, post_label, post_label_len);
    }
    size_t total_number_of_dimensions = i;

    // print the beginning of row data
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

    // pre-allocate a large enough buffer for us
    // this does not need to be accurate - it is just a hint to avoid multiple realloc().
    buffer_need_bytes(wb,
                      ( 20 * rrdr_rows(r)) // timestamp + json overhead
                    + ( (pre_value_len + post_value_len + 4) * total_number_of_dimensions * rrdr_rows(r) ) // number
                      );

    // for each line in the array
    for(i = start; i != end ;i += step) {
        NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
        RRDR_VALUE_FLAGS *co = &r->o[ i * r->d ];
        NETDATA_DOUBLE *ar = &r->ar[ i * r->d ];

        time_t now = r->t[i];

        if(dates == JSON_DATES_JS) {
            // generate the local date time
            struct tm tmbuf, *tm = localtime_r(&now, &tmbuf);
            if(!tm) {
                netdata_log_error("localtime_r() failed."); continue; }

            if(likely(i != start)) buffer_fast_strcat(wb, ",\n", 2);
            buffer_fast_strcat(wb, pre_date, pre_date_len);

            if( options & RRDR_OPTION_OBJECTSROWS )
                buffer_fast_strcat(wb, object_rows_time, object_rows_time_len);

            if(unlikely(dates_with_new))
                buffer_fast_strcat(wb, "new ", 4);

            buffer_jsdate(wb, tm->tm_year + 1900, tm->tm_mon, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec);

            buffer_fast_strcat(wb, post_date, post_date_len);

            if(unlikely(row_annotations)) {
                // google supports one annotation per row
                int annotation_found = 0;
                for(c = 0; c < used ; c++) {
                    if(unlikely(!(r->od[c] & RRDR_DIMENSION_QUERIED))) continue;

                    if(unlikely(co[c] & RRDR_VALUE_RESET)) {
                        buffer_fast_strcat(wb, overflow_annotation, overflow_annotation_len);
                        annotation_found = 1;
                        break;
                    }
                }
                if(likely(!annotation_found))
                    buffer_fast_strcat(wb, normal_annotation, normal_annotation_len);
            }
        }
        else {
            // print the timestamp of the line
            if(likely(i != start))
                buffer_fast_strcat(wb, ",\n", 2);

            buffer_fast_strcat(wb, pre_date, pre_date_len);

            if(unlikely( options & RRDR_OPTION_OBJECTSROWS ))
                buffer_fast_strcat(wb, object_rows_time, object_rows_time_len);

            buffer_print_netdata_double(wb, (NETDATA_DOUBLE) r->t[i]);

            // in ms
            if(unlikely(options & RRDR_OPTION_MILLISECONDS))
                buffer_fast_strcat(wb, "000", 3);

            buffer_fast_strcat(wb, post_date, post_date_len);
        }

        // for each dimension
        for(c = 0; c < used ;c++) {
            if(!rrdr_dimension_should_be_exposed(r->od[c], options))
                continue;

            NETDATA_DOUBLE n;
            if(unlikely(options & RRDR_OPTION_INTERNAL_AR))
                n = ar[c];
            else
                n = cn[c];

            buffer_fast_strcat(wb, pre_value, pre_value_len);

            if(unlikely( options & RRDR_OPTION_OBJECTSROWS ))
                buffer_sprintf(wb, "%s%s%s: ", kq, string2str(r->dn[c]), kq);

            if(co[c] & RRDR_VALUE_EMPTY && !(options & (RRDR_OPTION_INTERNAL_AR))) {
                if(unlikely(options & RRDR_OPTION_NULL2ZERO))
                    buffer_fast_strcat(wb, "0", 1);
                else
                    buffer_fast_strcat(wb, "null", 4);
            }
            else
                buffer_print_netdata_double(wb, n);

            buffer_fast_strcat(wb, post_value, post_value_len);
        }

        buffer_fast_strcat(wb, post_line, post_line_len);
    }

    buffer_strcat(wb, finish);
    //netdata_log_info("RRD2JSON(): %s: END", r->st->id);
}

void rrdr2json_v2(RRDR *r, BUFFER *wb) {
    QUERY_TARGET *qt = r->internal.qt;
    RRDR_OPTIONS options = qt->window.options;

    bool send_count = query_target_aggregatable(qt);
    bool send_hidden = send_count && r->vh && query_has_group_by_aggregation_percentage(qt);

    buffer_json_member_add_object(wb, "result");

    buffer_json_member_add_array(wb, "labels");
    buffer_json_add_array_item_string(wb, "time");
    long d, i;
    const long used = (long)r->d;
    for(d = 0, i = 0; d < used ; d++) {
        if(!rrdr_dimension_should_be_exposed(r->od[d], options))
            continue;

        buffer_json_add_array_item_string(wb, string2str(r->di[d]));
        i++;
    }
    buffer_json_array_close(wb); // labels

    buffer_json_member_add_object(wb, "point");
    {
        size_t point_count = 0;
        buffer_json_member_add_uint64(wb, "value", point_count++);
        buffer_json_member_add_uint64(wb, "arp", point_count++);
        buffer_json_member_add_uint64(wb, "pa", point_count++);
        if (send_count)
            buffer_json_member_add_uint64(wb, "count", point_count++);
        if (send_hidden)
            buffer_json_member_add_uint64(wb, "hidden", point_count++);
    }
    buffer_json_object_close(wb); // point

    buffer_json_member_add_array(wb, "data");
    if(i) {
        long start = 0, end = rrdr_rows(r), step = 1;
        if (!(options & RRDR_OPTION_REVERSED)) {
            start = rrdr_rows(r) - 1;
            end = -1;
            step = -1;
        }

        // for each line in the array
        for (i = start; i != end; i += step) {
            NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
            NETDATA_DOUBLE *ch = send_hidden ? &r->vh[i * r->d ] : NULL;
            RRDR_VALUE_FLAGS *co = &r->o[ i * r->d ];
            NETDATA_DOUBLE *ar = &r->ar[ i * r->d ];
            uint32_t *gbc = &r->gbc [ i * r->d ];
            time_t now = r->t[i];

            buffer_json_add_array_item_array(wb); // row

            if (options & RRDR_OPTION_MILLISECONDS)
                buffer_json_add_array_item_time_ms(wb, now); // the time
            else
                buffer_json_add_array_item_time_t(wb, now); // the time

            for (d = 0; d < used; d++) {
                if (!rrdr_dimension_should_be_exposed(r->od[d], options))
                    continue;

                RRDR_VALUE_FLAGS o = co[d];

                buffer_json_add_array_item_array(wb); // point

                // add the value
                NETDATA_DOUBLE n = cn[d];

                if(o & RRDR_VALUE_EMPTY) {
                    if (unlikely(options & RRDR_OPTION_NULL2ZERO))
                        buffer_json_add_array_item_double(wb, 0);
                    else
                        buffer_json_add_array_item_double(wb, NAN);
                }
                else
                    buffer_json_add_array_item_double(wb, n);

                // add the anomaly
                buffer_json_add_array_item_double(wb, ar[d]);

                // add the point annotations
                buffer_json_add_array_item_uint64(wb, o);

                // add the count
                if(send_count)
                    buffer_json_add_array_item_uint64(wb, gbc[d]);
                if(send_hidden)
                    buffer_json_add_array_item_double(wb, ch[d]);

                buffer_json_array_close(wb); // point
            }

            buffer_json_array_close(wb); // row
        }
    }

    buffer_json_array_close(wb); // data

    buffer_json_object_close(wb); // annotations
}
