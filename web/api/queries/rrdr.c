// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdr.h"

/*
static void rrdr_dump(RRDR *r)
{
    long c, i;
    RRDDIM *d;

    fprintf(stderr, "\nCHART %s (%s)\n", r->st->id, r->st->name);

    for(c = 0, d = r->st->dimensions; d ;c++, d = d->next) {
        fprintf(stderr, "DIMENSION %s (%s), %s%s%s%s\n"
                , d->id
                , d->name
                , (r->od[c] & RRDR_EMPTY)?"EMPTY ":""
                , (r->od[c] & RRDR_RESET)?"RESET ":""
                , (r->od[c] & RRDR_DIMENSION_HIDDEN)?"HIDDEN ":""
                , (r->od[c] & RRDR_DIMENSION_NONZERO)?"NONZERO ":""
                );
    }

    if(r->rows <= 0) {
        fprintf(stderr, "RRDR does not have any values in it.\n");
        return;
    }

    fprintf(stderr, "RRDR includes %d values in it:\n", r->rows);

    // for each line in the array
    for(i = 0; i < r->rows ;i++) {
        calculated_number *cn = &r->v[ i * r->d ];
        RRDR_DIMENSION_FLAGS *co = &r->o[ i * r->d ];

        // print the id and the timestamp of the line
        fprintf(stderr, "%ld %ld ", i + 1, r->t[i]);

        // for each dimension
        for(c = 0, d = r->st->dimensions; d ;c++, d = d->next) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
            if(unlikely(!(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

            if(co[c] & RRDR_EMPTY)
                fprintf(stderr, "null ");
            else
                fprintf(stderr, CALCULATED_NUMBER_FORMAT " %s%s%s%s "
                    , cn[c]
                    , (co[c] & RRDR_EMPTY)?"E":" "
                    , (co[c] & RRDR_RESET)?"R":" "
                    , (co[c] & RRDR_DIMENSION_HIDDEN)?"H":" "
                    , (co[c] & RRDR_DIMENSION_NONZERO)?"N":" "
                    );
        }

        fprintf(stderr, "\n");
    }
}
*/

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

void rrdr2csv(RRDR *r, BUFFER *wb, RRDR_OPTIONS options, const char *startline, const char *separator, const char *endline, const char *betweenlines) {
    rrdset_check_rdlock(r->st);

    //info("RRD2CSV(): %s: BEGIN", r->st->id);
    long c, i;
    RRDDIM *d;

    // print the csv header
    for(c = 0, i = 0, d = r->st->dimensions; d && c < r->d ;c++, d = d->next) {
        if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
        if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

        if(!i) {
            buffer_strcat(wb, startline);
            if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
            buffer_strcat(wb, "time");
            if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
        }
        buffer_strcat(wb, separator);
        if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
        buffer_strcat(wb, d->name);
        if(options & RRDR_OPTION_LABEL_QUOTES) buffer_strcat(wb, "\"");
        i++;
    }
    buffer_strcat(wb, endline);

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
    calculated_number total = 1;
    for(i = start; i != end ;i += step) {
        calculated_number *cn = &r->v[ i * r->d ];
        RRDR_VALUE_FLAGS *co = &r->o[ i * r->d ];

        buffer_strcat(wb, betweenlines);
        buffer_strcat(wb, startline);

        time_t now = r->t[i];

        if((options & RRDR_OPTION_SECONDS) || (options & RRDR_OPTION_MILLISECONDS)) {
            // print the timestamp of the line
            buffer_rrd_value(wb, (calculated_number)now);
            // in ms
            if(options & RRDR_OPTION_MILLISECONDS) buffer_strcat(wb, "000");
        }
        else {
            // generate the local date time
            struct tm tmbuf, *tm = localtime_r(&now, &tmbuf);
            if(!tm) { error("localtime() failed."); continue; }
            buffer_date(wb, tm->tm_year + 1900, tm->tm_mon + 1, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec);
        }

        int set_min_max = 0;
        if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
            total = 0;
            for(c = 0, d = r->st->dimensions; d && c < r->d ;c++, d = d->next) {
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
        for(c = 0, d = r->st->dimensions; d && c < r->d ;c++, d = d->next) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
            if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

            buffer_strcat(wb, separator);

            calculated_number n = cn[c];

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
        }

        buffer_strcat(wb, endline);
    }
    //info("RRD2CSV(): %s: END", r->st->id);
}

inline calculated_number rrdr2value(RRDR *r, long i, RRDR_OPTIONS options, int *all_values_are_null) {
    rrdset_check_rdlock(r->st);

    long c;
    RRDDIM *d;

    calculated_number *cn = &r->v[ i * r->d ];
    RRDR_VALUE_FLAGS *co = &r->o[ i * r->d ];

    calculated_number sum = 0, min = 0, max = 0, v;
    int all_null = 1, init = 1;

    calculated_number total = 1;
    int set_min_max = 0;
    if(unlikely(options & RRDR_OPTION_PERCENTAGE)) {
        total = 0;
        for(c = 0, d = r->st->dimensions; d && c < r->d ;c++, d = d->next) {
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
    for(c = 0, d = r->st->dimensions; d && c < r->d ;c++, d = d->next) {
        if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
        if(unlikely((options & RRDR_OPTION_NONZERO) && !(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

        calculated_number n = cn[c];

        if(likely((options & RRDR_OPTION_ABSOLUTE) && n < 0))
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

        if(unlikely(init)) {
            if(n > 0) {
                min = 0;
                max = n;
            }
            else {
                min = n;
                max = 0;
            }
            init = 0;
        }

        if(likely(!(co[c] & RRDR_VALUE_EMPTY))) {
            all_null = 0;
            sum += n;
        }

        if(n < min) min = n;
        if(n > max) max = n;
    }

    if(unlikely(all_null)) {
        if(likely(all_values_are_null))
            *all_values_are_null = 1;
        return 0;
    }
    else {
        if(likely(all_values_are_null))
            *all_values_are_null = 0;
    }

    if(options & RRDR_OPTION_MIN2MAX)
        v = max - min;
    else
        v = sum;

    return v;
}

void rrdr2ssv(RRDR *r, BUFFER *wb, RRDR_OPTIONS options, const char *prefix, const char *separator, const char *suffix) {
    //info("RRD2SSV(): %s: BEGIN", r->st->id);
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
        calculated_number v = rrdr2value(r, i, options, &all_values_are_null);

        if(likely(i != start)) {
            if(r->min > v) r->min = v;
            if(r->max < v) r->max = v;
        }
        else {
            r->min = v;
            r->max = v;
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
            buffer_rrd_value(wb, v);
    }
    buffer_strcat(wb, suffix);
    //info("RRD2SSV(): %s: END", r->st->id);
}

inline static void rrdr_lock_rrdset(RRDR *r) {
    if(unlikely(!r)) {
        error("NULL value given!");
        return;
    }

    rrdset_rdlock(r->st);
    r->has_st_lock = 1;
}

inline static void rrdr_unlock_rrdset(RRDR *r) {
    if(unlikely(!r)) {
        error("NULL value given!");
        return;
    }

    if(likely(r->has_st_lock)) {
        rrdset_unlock(r->st);
        r->has_st_lock = 0;
    }
}

inline void rrdr_free(RRDR *r)
{
    if(unlikely(!r)) {
        error("NULL value given!");
        return;
    }

    rrdr_unlock_rrdset(r);
    freez(r->t);
    freez(r->v);
    freez(r->o);
    freez(r->od);
    freez(r);
}

RRDR *rrdr_create(RRDSET *st, long n)
{
    if(unlikely(!st)) {
        error("NULL value given!");
        return NULL;
    }

    RRDR *r = callocz(1, sizeof(RRDR));
    r->st = st;

    rrdr_lock_rrdset(r);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) r->d++;

    r->n = n;

    r->t = callocz((size_t)n, sizeof(time_t));
    r->v = mallocz(n * r->d * sizeof(calculated_number));
    r->o = mallocz(n * r->d * sizeof(RRDR_VALUE_FLAGS));
    r->od = mallocz(r->d * sizeof(RRDR_DIMENSION_FLAGS));

    // set the hidden flag on hidden dimensions
    int c;
    for(c = 0, rd = st->dimensions ; rd ; c++, rd = rd->next) {
        if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)))
            r->od[c] = RRDR_DIMENSION_HIDDEN;
        else
            r->od[c] = 0;
    }

    r->group = 1;
    r->update_every = 1;

    return r;
}
