// SPDX-License-Identifier: GPL-3.0-or-later

#include "replication.h"

static time_t replicate_chart_timeframe(BUFFER *wb, RRDSET *st, time_t after, time_t before, bool enable_streaming) {
    size_t dimensions = rrdset_number_of_dimensions(st);

    struct storage_engine_query_ops *ops = &st->rrdhost->db[0].eng->api.query_ops;

    struct {
        DICTIONARY *dict;
        const DICTIONARY_ITEM *rda;
        RRDDIM *rd;
        struct storage_engine_query_handle handle;
        STORAGE_POINT sp;
    } data[dimensions];

    memset(data, 0, sizeof(data));

    if(enable_streaming && st->last_updated.tv_sec > before) {
        internal_error(true, "REPLAY: '%s' overwriting replication before from %llu to %llu",
                       rrdset_id(st),
                       (unsigned long long)before,
                       (unsigned long long)st->last_updated.tv_sec
        );
        before = st->last_updated.tv_sec;
    }

    // prepare our array of dimensions
    {
        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {
            if (rd_dfe.counter >= dimensions)
                break;

            data[rd_dfe.counter].dict = rd_dfe.dict;
            data[rd_dfe.counter].rda = dictionary_acquired_item_dup(rd_dfe.dict, rd_dfe.item);
            data[rd_dfe.counter].rd = rd;

            ops->init(rd->tiers[0]->db_metric_handle, &data[rd_dfe.counter].handle, after, before);
        }
        rrddim_foreach_done(rd);
    }

    time_t now = after, actual_after = 0, actual_before = 0;
    while(now <= before) {
        time_t min_start_time = 0, min_end_time = 0;
        for (size_t i = 0; i < dimensions && data[i].rd; i++) {
            // fetch the first valid point for the dimension
            int max_skip = 100;
            while(data[i].sp.end_time < now && !ops->is_finished(&data[i].handle) && max_skip-- > 0)
                data[i].sp = ops->next_metric(&data[i].handle);

            if(max_skip <= 0)
                error("REPLAY: host '%s', chart '%s', dimension '%s': db does not advance the query beyond time %ld",
                      rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_id(data[i].rd), now);

            if(data[i].sp.end_time < now)
                continue;

            if(!min_start_time) {
                min_start_time = data[i].sp.start_time;
                min_end_time = data[i].sp.end_time;
            }
            else {
                min_start_time = MIN(min_start_time, data[i].sp.start_time);
                min_end_time = MIN(min_end_time, data[i].sp.end_time);
            }
        }

        if(min_end_time < now) {
            internal_error(true,
                           "REPLAY: host '%s', chart '%s': no data on any dimension beyond time %ld",
                           rrdhost_hostname(st->rrdhost), rrdset_id(st), now);
            break;
        }

        if(min_end_time <= min_start_time)
            min_start_time = min_end_time - st->update_every;

        if(!actual_after) {
            actual_after = min_end_time;
            actual_before = min_end_time;
        }
        else
            actual_before = min_end_time;

        buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_BEGIN " '' %ld %ld\n", min_start_time, min_end_time);

        // output the replay values for this time
        for (size_t i = 0; i < dimensions && data[i].rd; i++) {
            if(data[i].sp.start_time <= min_end_time && data[i].sp.end_time >= min_end_time)
                buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_SET " \"%s\" " NETDATA_DOUBLE_FORMAT_AUTO " \"%s\"\n",
                               rrddim_id(data[i].rd), data[i].sp.sum, data[i].sp.flags & SN_FLAG_RESET ? "R" : "");
            else
                buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_SET " \"%s\" NAN \"E\"\n",
                               rrddim_id(data[i].rd));
        }

        now = min_end_time + 1;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    if(actual_after) {
        char actual_after_buf[LOG_DATE_LENGTH + 1], actual_before_buf[LOG_DATE_LENGTH + 1];
        log_date(actual_after_buf, LOG_DATE_LENGTH, actual_after);
        log_date(actual_before_buf, LOG_DATE_LENGTH, actual_before);
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': sending data %ld [%s] to %ld [%s] (requested %ld [delta %ld] to %ld [delta %ld])",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       actual_after, actual_after_buf, actual_before, actual_before_buf,
                       after, actual_after - after, before, actual_before - before);
    }
    else
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': nothing to send (requested %ld to %ld)",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       after, before);
#endif

    // release all the dictionary items acquired
    // finalize the queries
    for(size_t i = 0; i < dimensions && data[i].rda ;i++) {
        ops->finalize(&data[i].handle);
        dictionary_acquired_item_release(data[i].dict, data[i].rda);
    }

    return before;
}

static void replicate_chart_collection_state(BUFFER *wb, RRDSET *st) {
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_RRDDIM_STATE " \"%s\" %llu %lld " NETDATA_DOUBLE_FORMAT_AUTO " " NETDATA_DOUBLE_FORMAT_AUTO "\n",
                       rrddim_id(rd),
                       (usec_t)rd->last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)rd->last_collected_time.tv_usec,
                       rd->last_collected_value,
                       rd->last_calculated_value,
                       rd->last_stored_value
        );
    }
    rrddim_foreach_done(rd);

    buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_RRDSET_STATE " %llu %llu " TOTAL_NUMBER_FORMAT " " TOTAL_NUMBER_FORMAT "\n",
                   (usec_t)st->last_collected_time.tv_sec * USEC_PER_SEC + (usec_t)st->last_collected_time.tv_usec,
                   (usec_t)st->last_updated.tv_sec * USEC_PER_SEC + (usec_t)st->last_updated.tv_usec,
                   st->last_collected_total,
                   st->collected_total
    );
}

bool replicate_chart_response(RRDHOST *host, RRDSET *st,
                              bool start_streaming, time_t after, time_t before)
{
    time_t query_after = after;
    time_t query_before = before;
    time_t now = now_realtime_sec();

    // only happens when the parent received nonsensical timestamps from
    // us, in which case we want to skip replication and start streaming.
    // (or when replication is disabled)
    if (start_streaming && after == 0 && before == 0)
        return true;

    // find the first entry we have
    time_t first_entry_local = rrdset_first_entry_t(st);
    if(first_entry_local > now) {
        internal_error(true,
                       "RRDSET: '%s' first time %ld is in the future (now is %ld)",
                       rrdset_id(st), first_entry_local, now);
        first_entry_local = now;
    }

    if (query_after < first_entry_local)
        query_after = first_entry_local;

    // find the latest entry we have
    time_t last_entry_local = st->last_updated.tv_sec;
    if(last_entry_local > now) {
        internal_error(true,
                       "RRDSET: '%s' last updated time %ld is in the future (now is %ld)",
                       rrdset_id(st), last_entry_local, now);
        last_entry_local = now;
    }

    if (query_before > last_entry_local)
        query_before = last_entry_local;

    // if the parent asked us to start streaming, then fill the rest of the
    // data that we have
    if (start_streaming)
        query_before = last_entry_local;

    // should never happen, but nevertheless enable streaming
    if (query_after > query_before)
        return true;

    bool enable_streaming = (start_streaming || query_before == last_entry_local) ? true : false;

    // we might want to optimize this by filling a temporary buffer
    // and copying the result to the host's buffer in order to avoid
    // holding the host's buffer lock for too long
    BUFFER *wb = sender_start(host->sender);
    {
        // pass the original after/before so that the parent knows about
        // which time range we responded
        buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_BEGIN " \"%s\"\n", rrdset_id(st));

        // fill the data table
        before = replicate_chart_timeframe(wb, st, after, before, enable_streaming);

        if(enable_streaming)
            replicate_chart_collection_state(wb, st);

        // end with first/last entries we have, and the first start time and
        // last end time of the data we sent
        buffer_sprintf(wb, PLUGINSD_KEYWORD_REPLAY_END " %ld %ld %ld %s %ld %ld\n",
                       (time_t) st->update_every, first_entry_local, last_entry_local,
                       enable_streaming ? "true" : "false", after, before);
    }
    sender_commit(host->sender, wb);

    return enable_streaming;
}

static bool send_replay_chart_cmd(FILE *outfp, RRDSET *st, bool start_streaming, time_t after, time_t before) {

#ifdef NETDATA_INTERNAL_CHECKS
    if(after && before) {
        char after_buf[LOG_DATE_LENGTH + 1], before_buf[LOG_DATE_LENGTH + 1];
        log_date(after_buf, LOG_DATE_LENGTH, after);
        log_date(before_buf, LOG_DATE_LENGTH, before);
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': sending replication request %ld [%s] to %ld [%s], start streaming: %s",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       after, after_buf, before, before_buf,
                       start_streaming?"true":"false");
    }
    else {
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': sending empty replication request, start streaming: %s",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st),
                       start_streaming?"true":"false");
    }
#endif

    debug(D_REPLICATION, PLUGINSD_KEYWORD_REPLAY_CHART " \"%s\" \"%s\" %ld %ld\n",
          rrdset_id(st), start_streaming ? "true" : "false", after, before);

    int ret = fprintf(outfp, PLUGINSD_KEYWORD_REPLAY_CHART " \"%s\" \"%s\" %ld %ld\n",
                      rrdset_id(st), start_streaming ? "true" : "false",
                      after, before);
    if (ret < 0) {
        error("failed to send replay request to child (ret=%d)", ret);
        return false;
    }

    fflush(outfp);
    return true;
}

bool replicate_chart_request(FILE *outfp, RRDHOST *host, RRDSET *st,
                             time_t first_entry_child, time_t last_entry_child,
                             time_t prev_first_entry_wanted, time_t prev_last_entry_wanted)
{
    time_t now = now_realtime_sec();

    // if replication is disabled, send an empty replication request
    // asking no data
    if (!host->rrdpush_enable_replication) {
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': sending empty replication request because replication is disabled",
                       rrdhost_hostname(host), rrdset_id(st));

        return send_replay_chart_cmd(outfp, st, true, 0, 0);
    }

    // Child has no stored data
    if (!last_entry_child) {
        error("REPLAY: host '%s', chart '%s': sending empty replication request because child has no stored data",
              rrdhost_hostname(host), rrdset_id(st));

        return send_replay_chart_cmd(outfp, st, true, 0, 0);
    }

    // Nothing to get if the chart has not dimensions
    if (!rrdset_number_of_dimensions(st)) {
        error("REPLAY: host '%s', chart '%s': sending empty replication request because chart has no dimensions",
              rrdhost_hostname(host), rrdset_id(st));

        return send_replay_chart_cmd(outfp, st, true, 0, 0);
    }

    // if the child's first/last entries are nonsensical, resume streaming
    // without asking for any data
    if (first_entry_child <= 0) {
        error("REPLAY: host '%s', chart '%s': sending empty replication because first entry of the child is invalid (%ld)",
              rrdhost_hostname(host), rrdset_id(st), first_entry_child);

        return send_replay_chart_cmd(outfp, st, true, 0, 0);
    }

    if (first_entry_child > last_entry_child) {
        error("REPLAY: host '%s', chart '%s': sending empty replication because child timings are invalid (first entry %ld > last entry %ld)",
              rrdhost_hostname(host), rrdset_id(st), first_entry_child, last_entry_child);

        return send_replay_chart_cmd(outfp, st, true, 0, 0);
    }

    time_t last_entry_local = rrdset_last_entry_t(st);
    if(last_entry_local > now) {
        internal_error(true,
                       "REPLAY: host '%s', chart '%s': local last entry time %ld is in the future (now is %ld). Adjusting it.",
                       rrdhost_hostname(host), rrdset_id(st), last_entry_local, now);
        last_entry_local = now;
    }

    // should never happen but it if does, start streaming without asking
    // for any data
    if (last_entry_local > last_entry_child) {
        error("REPLAY: host '%s', chart '%s': sending empty replication request because our last entry (%ld) in later than the child one (%ld)",
              rrdhost_hostname(host), rrdset_id(st), last_entry_local, last_entry_child);

        return send_replay_chart_cmd(outfp, st, true, 0, 0);
    }

    time_t first_entry_wanted;
    if (prev_first_entry_wanted && prev_last_entry_wanted) {
        first_entry_wanted = prev_last_entry_wanted;
        if ((now - first_entry_wanted) > host->rrdpush_seconds_to_replicate)
            first_entry_wanted = now - host->rrdpush_seconds_to_replicate;
    }
    else
        first_entry_wanted = MAX(last_entry_local, first_entry_child);

    time_t last_entry_wanted = first_entry_wanted + host->rrdpush_replication_step;
    last_entry_wanted = MIN(last_entry_wanted, last_entry_child);

    bool start_streaming = (last_entry_wanted == last_entry_child);

    return send_replay_chart_cmd(outfp, st,
                                 start_streaming, first_entry_wanted, last_entry_wanted);
}
