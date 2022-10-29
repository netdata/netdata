// SPDX-License-Identifier: GPL-3.0-or-later

#include "replication.h"

#define MAX_DIMENSIONS_PER_CHART 99

static void replicate_do_chart(BUFFER *wb, RRDSET *st, time_t after, time_t before) {
    size_t dimensions = rrdset_number_of_dimensions(st);
    if(dimensions > MAX_DIMENSIONS_PER_CHART)
        dimensions = MAX_DIMENSIONS_PER_CHART;

    struct storage_engine_query_ops *ops = &st->rrdhost->db[0].eng->api.query_ops;

    struct {
        DICTIONARY *dict;
        const DICTIONARY_ITEM *rda;
        RRDDIM *rd;
        struct storage_engine_query_handle handle;
        STORAGE_POINT sp;
    } data[dimensions];

    memset(data, 0, sizeof(data));

    // prepare our array of dimensions
    buffer_sprintf(wb, "REPLAY_RRDSET_HEADER start_time end_time");
    void *rdptr;
    rrddim_foreach_read(rdptr, st) {
        if(rdptr_dfe.counter >= dimensions)
            break;

        RRDDIM *rd = (RRDDIM *)rdptr;
        data[rdptr_dfe.counter].dict = rdptr_dfe.dict;
        data[rdptr_dfe.counter].rda = dictionary_acquired_item_dup(rdptr_dfe.dict, rdptr_dfe.item);
        data[rdptr_dfe.counter].rd = rd;

        ops->init(rd->tiers[0]->db_metric_handle, &data[rdptr_dfe.counter].handle, after, before);
        buffer_sprintf(wb, " \"%s\"", rrddim_id(rd));
    }
    rrddim_foreach_done(rdptr);
    buffer_fast_strcat(wb, "\n", 1);

    // find a point
    time_t now = after;
    while(now < before) {
        time_t min_start_time = 0, min_end_time = 0;
        for (size_t i = 0; i < dimensions && data[i].rd; i++) {
            // fetch the first valid point for the dimension
            int max_skip = 100;
            while(data[i].sp.start_time < now && !ops->is_finished(&data[i].handle) && max_skip-- > 0)
                data[i].sp = ops->next_metric(&data[i].handle);

            if(max_skip <= 0)
                error("REPLAY: host '%s', chart '%s', dimension '%s': db does not advance the query beyond time %ld",
                      rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_id(data[i].rd), now);

            if(data[i].sp.start_time < now)
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

        if(min_start_time < now) {
            error("REPLAY: host '%s', chart '%s': no useful dimensions beyond time %ld",
                  rrdhost_hostname(st->rrdhost), rrdset_id(st), now);
            break;
        }

        if(min_end_time <= min_start_time)
            min_end_time = min_start_time + 1;

        // output the replay values for this time
        buffer_sprintf(wb, "REPLAY_RRDSET_DONE %ld %ld", min_start_time, min_end_time);
        for (size_t i = 0; i < dimensions && data[i].rd; i++) {
            if(data[i].sp.start_time >= min_start_time && data[i].sp.end_time < min_end_time)
                buffer_sprintf(wb, " %lf %u", data[i].sp.sum, (unsigned int)data[i].sp.flags);
            else
                buffer_sprintf(wb, " NAN %u", (unsigned int)SN_EMPTY_SLOT);
        }
        buffer_fast_strcat(wb, "\n", 1);

        now = min_end_time;
    }

    // release all the dictionary items acquired
    // finalize the queries
    for(size_t i = 0; i < dimensions && data[i].rda ;i++) {
        ops->finalize(&data[i].handle);
        dictionary_acquired_item_release(data[i].dict, data[i].rda);
    }
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
        buffer_sprintf(wb, "REPLAY_RRDSET_BEGIN \"%s\"\n", rrdset_id(st));

        // fill the data table
        replicate_do_chart(wb, st, after, before);
        // (void) ChartQuery::get(wb, st, after, before);

        // end with first/last entries we have, and the first start time and
        // last end time of the data we sent
        buffer_sprintf(wb, "REPLAY_RRDSET_END %ld %ld %ld %s %ld %ld\n",
                       (time_t) st->update_every, first_entry_local, last_entry_local,
                       enable_streaming ? "true" : "false", after, before);
    }
    sender_commit(host->sender, wb);

    return enable_streaming;
}

static bool send_replay_chart_cmd(FILE *outfp, const char *chart,
                                  bool start_streaming, time_t after, time_t before)
{
    debug(D_REPLICATION, "REPLAY_CHART \"%s\" \"%s\" %ld %ld\n",
          chart, start_streaming ? "true" : "false", after, before);

    int ret = fprintf(outfp, "REPLAY_CHART \"%s\" \"%s\" %ld %ld\n",
                      chart, start_streaming ? "true" : "false",
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
        error("REPLAY: host '%s', chart '%s': skipping replication request because replication is disabled",
              rrdhost_hostname(host), rrdset_id(st));

        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }

    // Child has no stored data
    if (!last_entry_child) {
        error("REPLAY: host '%s', chart '%s': skipping replication request because child has no stored data",
              rrdhost_hostname(host), rrdset_id(st));

        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }

    // Nothing to get if the chart has not dimensions
    if (!rrdset_number_of_dimensions(st)) {
        error("REPLAY: host '%s', chart '%s': skipping replication request because chart has no dimensions",
              rrdhost_hostname(host), rrdset_id(st));

        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }

    // if the child's first/last entries are nonsensical, resume streaming
    // without asking for any data
    if (first_entry_child <= 0) {
        error("REPLAY: host '%s', chart '%s': skipping replication because first entry of the child is invalid (%ld)",
              rrdhost_hostname(host), rrdset_id(st), first_entry_child);

        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }

    if (first_entry_child > last_entry_child) {
        error("REPLAY: host '%s', chart '%s': skipping replication because child timings are invalid (first entry %ld > last entry %ld)",
              rrdhost_hostname(host), rrdset_id(st), first_entry_child, last_entry_child);

        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
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
        error("REPLAY: host '%s', chart '%s': skipping replication request because our last entry (%ld) in later than the child one (%ld)",
              rrdhost_hostname(host), rrdset_id(st), last_entry_local, last_entry_child);

        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
        return send_replay_chart_cmd(outfp, rrdset_id(st), true, 0, 0);
    }

    // ask for the next timestamp
    time_t first_entry_wanted = last_entry_local + 1;

    // make sure the 1st entry is GE than the child's
    if (first_entry_wanted < first_entry_child)
        first_entry_wanted = first_entry_child;

    // don't ask for more than `rrdpush_seconds_to_replicate`
    if ((now - first_entry_wanted) > host->rrdpush_seconds_to_replicate)
        first_entry_wanted = now - host->rrdpush_seconds_to_replicate;

    // ask the next X points
    time_t last_entry_wanted = first_entry_wanted + host->rrdpush_replication_step;

    // make sure we don't ask more than the child has
    if (last_entry_wanted > last_entry_child)
        last_entry_wanted = last_entry_child;

    // sanity check to make sure our time range is well formed
    if (first_entry_wanted > last_entry_wanted)
        first_entry_wanted = last_entry_wanted;

    // on subsequent calls we can use just the end time of the last entry
    // received
    if (prev_first_entry_wanted && prev_last_entry_wanted) {
        first_entry_wanted = prev_last_entry_wanted + st->update_every;
        last_entry_wanted = MIN(first_entry_wanted + host->rrdpush_replication_step, last_entry_child);
    }

    bool start_streaming = (last_entry_wanted == last_entry_child);

    return send_replay_chart_cmd(outfp, rrdset_id(st), start_streaming,
                                                       first_entry_wanted,
                                                       last_entry_wanted);
}
