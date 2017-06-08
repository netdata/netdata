#include "common.h"

unsigned long rrdset_info2json_api_old(RRDSET *st, char *options, BUFFER *wb) {
    time_t now = now_realtime_sec();

    rrdset_rdlock(st);

    st->last_accessed_time = now;

    buffer_sprintf(wb,
            "\t\t{\n"
            "\t\t\t\"id\": \"%s\",\n"
            "\t\t\t\"name\": \"%s\",\n"
            "\t\t\t\"type\": \"%s\",\n"
            "\t\t\t\"family\": \"%s\",\n"
            "\t\t\t\"context\": \"%s\",\n"
            "\t\t\t\"title\": \"%s (%s)\",\n"
            "\t\t\t\"priority\": %ld,\n"
            "\t\t\t\"enabled\": %d,\n"
            "\t\t\t\"units\": \"%s\",\n"
            "\t\t\t\"url\": \"/data/%s/%s\",\n"
            "\t\t\t\"chart_type\": \"%s\",\n"
            "\t\t\t\"counter\": %lu,\n"
            "\t\t\t\"entries\": %ld,\n"
            "\t\t\t\"first_entry_t\": %ld,\n"
            "\t\t\t\"last_entry\": %lu,\n"
            "\t\t\t\"last_entry_t\": %ld,\n"
            "\t\t\t\"last_entry_secs_ago\": %ld,\n"
            "\t\t\t\"update_every\": %d,\n"
            "\t\t\t\"isdetail\": %d,\n"
            "\t\t\t\"usec_since_last_update\": %llu,\n"
            "\t\t\t\"collected_total\": " TOTAL_NUMBER_FORMAT ",\n"
            "\t\t\t\"last_collected_total\": " TOTAL_NUMBER_FORMAT ",\n"
            "\t\t\t\"dimensions\": [\n"
            , st->id
            , st->name
            , st->type
            , st->family
            , st->context
            , st->title, st->name
            , st->priority
            , rrdset_flag_check(st, RRDSET_FLAG_ENABLED)?1:0
            , st->units
            , st->name, options?options:""
            , rrdset_type_name(st->chart_type)
            , st->counter
            , st->entries
            , rrdset_first_entry_t(st)
            , rrdset_last_slot(st)
            , rrdset_last_entry_t(st)
            , (now < rrdset_last_entry_t(st)) ? (time_t)0 : now - rrdset_last_entry_t(st)
            , st->update_every
            , rrdset_flag_check(st, RRDSET_FLAG_DETAIL)?1:0
            , st->usec_since_last_update
            , st->collected_total
            , st->last_collected_total
    );

    unsigned long memory = st->memsize;

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {

        memory += rd->memsize;

        buffer_sprintf(wb,
                "\t\t\t\t{\n"
                "\t\t\t\t\t\"id\": \"%s\",\n"
                "\t\t\t\t\t\"name\": \"%s\",\n"
                "\t\t\t\t\t\"entries\": %ld,\n"
                "\t\t\t\t\t\"isHidden\": %d,\n"
                "\t\t\t\t\t\"algorithm\": \"%s\",\n"
                "\t\t\t\t\t\"multiplier\": " COLLECTED_NUMBER_FORMAT ",\n"
                "\t\t\t\t\t\"divisor\": " COLLECTED_NUMBER_FORMAT ",\n"
                "\t\t\t\t\t\"last_entry_t\": %ld,\n"
                "\t\t\t\t\t\"collected_value\": " COLLECTED_NUMBER_FORMAT ",\n"
                "\t\t\t\t\t\"calculated_value\": " CALCULATED_NUMBER_FORMAT ",\n"
                "\t\t\t\t\t\"last_collected_value\": " COLLECTED_NUMBER_FORMAT ",\n"
                "\t\t\t\t\t\"last_calculated_value\": " CALCULATED_NUMBER_FORMAT ",\n"
                "\t\t\t\t\t\"memory\": %lu\n"
                "\t\t\t\t}%s\n"
                , rd->id
                , rd->name
                , rd->entries
                , rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)?1:0
                , rrd_algorithm_name(rd->algorithm)
                , rd->multiplier
                , rd->divisor
                , rd->last_collected_time.tv_sec
                , rd->collected_value
                , rd->calculated_value
                , rd->last_collected_value
                , rd->last_calculated_value
                , rd->memsize
                , rd->next?",":""
        );
    }

    buffer_sprintf(wb,
            "\t\t\t],\n"
                    "\t\t\t\"memory\" : %lu\n"
                    "\t\t}"
                   , memory
    );

    rrdset_unlock(st);
    return memory;
}

#define RRD_GRAPH_JSON_HEADER "{\n\t\"charts\": [\n"
#define RRD_GRAPH_JSON_FOOTER "\n\t]\n}\n"

void rrd_graph2json_api_old(RRDSET *st, char *options, BUFFER *wb)
{
    buffer_strcat(wb, RRD_GRAPH_JSON_HEADER);
    rrdset_info2json_api_old(st, options, wb);
    buffer_strcat(wb, RRD_GRAPH_JSON_FOOTER);
}

void rrd_all2json_api_old(RRDHOST *host, BUFFER *wb)
{
    unsigned long memory = 0;
    long c = 0;
    RRDSET *st;

    time_t now = now_realtime_sec();

    buffer_strcat(wb, RRD_GRAPH_JSON_HEADER);

    rrdhost_rdlock(host);
    rrdset_foreach_read(st, host) {
        if(rrdset_is_available_for_viewers(st)) {
            if(c) buffer_strcat(wb, ",\n");
            memory += rrdset_info2json_api_old(st, NULL, wb);

            c++;
            st->last_accessed_time = now;
        }
    }
    rrdhost_unlock(host);

    buffer_sprintf(wb, "\n\t],\n"
                    "\t\"hostname\": \"%s\",\n"
                    "\t\"update_every\": %d,\n"
                    "\t\"history\": %ld,\n"
                    "\t\"memory\": %lu\n"
                    "}\n"
                   , host->hostname
                   , host->rrd_update_every
                   , host->rrd_history_entries
                   , memory
    );
}

time_t rrdset2json_api_old(
        int type
        , RRDSET *st
        , BUFFER *wb
        , long points
        , long group
        , int group_method
        , time_t after
        , time_t before
        , int only_non_zero
) {
    int c;
    rrdset_rdlock(st);

    st->last_accessed_time = now_realtime_sec();

    // -------------------------------------------------------------------------
    // switch from JSON to google JSON

    char kq[2] = "\"";
    char sq[2] = "\"";
    switch(type) {
        case DATASOURCE_DATATABLE_JSON:
        case DATASOURCE_DATATABLE_JSONP:
            kq[0] = '\0';
            sq[0] = '\'';
            break;

        case DATASOURCE_JSON:
        default:
            break;
    }


    // -------------------------------------------------------------------------
    // validate the parameters

    if(points < 1) points = 1;
    if(group < 1) group = 1;

    if(before == 0 || before > rrdset_last_entry_t(st)) before = rrdset_last_entry_t(st);
    if(after  == 0 || after < rrdset_first_entry_t(st)) after = rrdset_first_entry_t(st);

    // ---

    // our return value (the last timestamp printed)
    // this is required to detect re-transmit in google JSONP
    time_t last_timestamp = 0;


    // -------------------------------------------------------------------------
    // find how many dimensions we have

    int dimensions = 0;
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) dimensions++;
    if(!dimensions) {
        rrdset_unlock(st);
        buffer_strcat(wb, "No dimensions yet.");
        return 0;
    }


    // -------------------------------------------------------------------------
    // prepare various strings, to speed up the loop

    char overflow_annotation[201]; snprintfz(overflow_annotation, 200, ",{%sv%s:%sRESET OR OVERFLOW%s},{%sv%s:%sThe counters have been wrapped.%s}", kq, kq, sq, sq, kq, kq, sq, sq);
    char normal_annotation[201];   snprintfz(normal_annotation,   200, ",{%sv%s:null},{%sv%s:null}", kq, kq, kq, kq);
    char pre_date[51];             snprintfz(pre_date,             50, "        {%sc%s:[{%sv%s:%s", kq, kq, kq, kq, sq);
    char post_date[21];            snprintfz(post_date,            20, "%s}", sq);
    char pre_value[21];            snprintfz(pre_value,            20, ",{%sv%s:", kq, kq);
    char post_value[21];           strcpy(post_value,                  "}");


    // -------------------------------------------------------------------------
    // checks for debugging

    if(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)) {
        debug(D_RRD_STATS, "%s first_entry_t = %ld, last_entry_t = %ld, duration = %ld, after = %ld, before = %ld, duration = %ld, entries_to_show = %ld, group = %ld"
              , st->id
              , rrdset_first_entry_t(st)
              , rrdset_last_entry_t(st)
              , rrdset_last_entry_t(st) - rrdset_first_entry_t(st)
              , after
              , before
              , before - after
              , points
              , group
        );

        if(before < after)
            debug(D_RRD_STATS, "WARNING: %s The newest value in the database (%ld) is earlier than the oldest (%ld)", st->name, before, after);

        if((before - after) > st->entries * st->update_every)
            debug(D_RRD_STATS, "WARNING: %s The time difference between the oldest and the newest entries (%ld) is higher than the capacity of the database (%ld)", st->name, before - after, st->entries * st->update_every);
    }


    // -------------------------------------------------------------------------
    // temp arrays for keeping values per dimension

    calculated_number group_values[dimensions]; // keep sums when grouping
    int               print_hidden[dimensions]; // keep hidden flags
    int               found_non_zero[dimensions];
    int               found_non_existing[dimensions];

    // initialize them
    for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
        group_values[c] = 0;
        print_hidden[c] = rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)?1:0;
        found_non_zero[c] = 0;
        found_non_existing[c] = 0;
    }


    // error("OLD: points=%d after=%d before=%d group=%d, duration=%d", entries_to_show, before - (st->update_every * group * entries_to_show), before, group, before - after + 1);
    // rrd2array(st, entries_to_show, before - (st->update_every * group * entries_to_show), before, group_method, only_non_zero);
    // rrd2rrdr(st, entries_to_show, before - (st->update_every * group * entries_to_show), before, group_method);

    // -------------------------------------------------------------------------
    // remove dimensions that contain only zeros

    int max_loop = 1;
    if(only_non_zero) max_loop = 2;

    for(; max_loop ; max_loop--) {

        // -------------------------------------------------------------------------
        // print the JSON header

        buffer_sprintf(wb, "{\n %scols%s:\n [\n", kq, kq);
        buffer_sprintf(wb, "        {%sid%s:%s%s,%slabel%s:%stime%s,%spattern%s:%s%s,%stype%s:%sdatetime%s},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq);
        buffer_sprintf(wb, "        {%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotation%s}},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);
        buffer_sprintf(wb, "        {%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotationText%s}}", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);

        // print the header for each dimension
        // and update the print_hidden array for the dimensions that should be hidden
        int pc = 0;
        for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
            if(!print_hidden[c]) {
                pc++;
                buffer_sprintf(wb, ",\n     {%sid%s:%s%s,%slabel%s:%s%s%s,%spattern%s:%s%s,%stype%s:%snumber%s}", kq, kq, sq, sq, kq, kq, sq, rd->name, sq, kq, kq, sq, sq, kq, kq, sq, sq);
            }
        }
        if(!pc) {
            buffer_sprintf(wb, ",\n     {%sid%s:%s%s,%slabel%s:%s%s%s,%spattern%s:%s%s,%stype%s:%snumber%s}", kq, kq, sq, sq, kq, kq, sq, "no data", sq, kq, kq, sq, sq, kq, kq, sq, sq);
        }

        // print the begin of row data
        buffer_sprintf(wb, "\n  ],\n    %srows%s:\n [\n", kq, kq);


        // -------------------------------------------------------------------------
        // the main loop

        int annotate_reset = 0;
        int annotation_count = 0;

        long    t = rrdset_time2slot(st, before),
                stop_at_t = rrdset_time2slot(st, after),
                stop_now = 0;

        t -= t % group;

        time_t  now = rrdset_slot2time(st, t),
                dt = st->update_every;

        long count = 0, printed = 0, group_count = 0;
        last_timestamp = 0;

        if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
            debug(D_RRD_STATS, "%s: REQUEST after:%u before:%u, points:%ld, group:%ld, CHART cur:%ld first: %u last:%u, CALC start_t:%ld, stop_t:%ld"
                  , st->id
                  , (uint32_t)after
                  , (uint32_t)before
                  , points
                  , group
                  , st->current_entry
                  , (uint32_t)rrdset_first_entry_t(st)
                  , (uint32_t)rrdset_last_entry_t(st)
                  , t
                  , stop_at_t
            );

        long counter = 0;
        for(; !stop_now ; now -= dt, t--, counter++) {
            if(t < 0) t = st->entries - 1;
            if(t == stop_at_t) stop_now = counter;

            int print_this = 0;

            if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG)))
                debug(D_RRD_STATS, "%s t = %ld, count = %ld, group_count = %ld, printed = %ld, now = %ld, %s %s"
                      , st->id
                      , t
                      , count + 1
                      , group_count + 1
                      , printed
                      , now
                      , (group_count + 1 == group)?"PRINT":"  -  "
                      , (now >= after && now <= before)?"RANGE":"  -  "
                );


            // make sure we return data in the proper time range
            if(now > before) continue;
            if(now < after) break;

            //if(rrdset_slot2time(st, t) != now)
            //  error("%s: slot=%ld, now=%ld, slot2time=%ld, diff=%ld, last_entry_t=%ld, rrdset_last_slot=%ld", st->id, t, now, rrdset_slot2time(st,t), now - rrdset_slot2time(st,t), rrdset_last_entry_t(st), rrdset_last_slot(st));

            count++;
            group_count++;

            // check if we have to print this now
            if(group_count == group) {
                if(printed >= points) {
                    // debug(D_RRD_STATS, "Already printed all rows. Stopping.");
                    break;
                }

                // generate the local date time
                struct tm tmbuf, *tm = localtime_r(&now, &tmbuf);
                if(!tm) { error("localtime() failed."); continue; }
                if(now > last_timestamp) last_timestamp = now;

                if(printed) buffer_strcat(wb, "]},\n");
                buffer_strcat(wb, pre_date);
                buffer_jsdate(wb, tm->tm_year + 1900, tm->tm_mon, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec);
                buffer_strcat(wb, post_date);

                print_this = 1;
            }

            // do the calculations
            for(rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
                storage_number n = rd->values[t];
                calculated_number value = unpack_storage_number(n);

                if(!does_storage_number_exist(n)) {
                    value = 0.0;
                    found_non_existing[c]++;
                }
                if(did_storage_number_reset(n)) annotate_reset = 1;

                switch(group_method) {
                    case GROUP_MAX:
                        if(abs(value) > abs(group_values[c])) group_values[c] = value;
                        break;

                    case GROUP_SUM:
                        group_values[c] += value;
                        break;

                    default:
                    case GROUP_AVERAGE:
                        group_values[c] += value;
                        if(print_this) group_values[c] /= ( group_count - found_non_existing[c] );
                        break;
                }
            }

            if(print_this) {
                if(annotate_reset) {
                    annotation_count++;
                    buffer_strcat(wb, overflow_annotation);
                    annotate_reset = 0;
                }
                else
                    buffer_strcat(wb, normal_annotation);

                pc = 0;
                for(c = 0 ; c < dimensions ; c++) {
                    if(found_non_existing[c] == group_count) {
                        // all entries are non-existing
                        pc++;
                        buffer_strcat(wb, pre_value);
                        buffer_strcat(wb, "null");
                        buffer_strcat(wb, post_value);
                    }
                    else if(!print_hidden[c]) {
                        pc++;
                        buffer_strcat(wb, pre_value);
                        buffer_rrd_value(wb, group_values[c]);
                        buffer_strcat(wb, post_value);

                        if(group_values[c]) found_non_zero[c]++;
                    }

                    // reset them for the next loop
                    group_values[c] = 0;
                    found_non_existing[c] = 0;
                }

                // if all dimensions are hidden, print a null
                if(!pc) {
                    buffer_strcat(wb, pre_value);
                    buffer_strcat(wb, "null");
                    buffer_strcat(wb, post_value);
                }

                printed++;
                group_count = 0;
            }
        }

        if(printed) buffer_strcat(wb, "]}");
        buffer_strcat(wb, "\n   ]\n}\n");

        if(only_non_zero && max_loop > 1) {
            int changed = 0;
            for(rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
                group_values[c] = 0;
                found_non_existing[c] = 0;

                if(!print_hidden[c] && !found_non_zero[c]) {
                    changed = 1;
                    print_hidden[c] = 1;
                }
            }

            if(changed) buffer_flush(wb);
            else break;
        }
        else break;

    } // max_loop

    debug(D_RRD_STATS, "RRD_STATS_JSON: %s total %zu bytes", st->name, wb->len);

    rrdset_unlock(st);
    return last_timestamp;
}
