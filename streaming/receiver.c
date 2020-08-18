// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"

extern struct config stream_config;

void destroy_receiver_state(struct receiver_state *rpt) {
    freez(rpt->key);
    freez(rpt->hostname);
    freez(rpt->registry_hostname);
    freez(rpt->machine_guid);
    freez(rpt->os);
    freez(rpt->timezone);
    freez(rpt->tags);
    freez(rpt->client_ip);
    freez(rpt->client_port);
    freez(rpt->program_name);
    freez(rpt->program_version);
#ifdef ENABLE_HTTPS
    if(rpt->ssl.conn){
        SSL_free(rpt->ssl.conn);
    }
#endif
    freez(rpt);
}

static void rrdpush_receiver_thread_cleanup(void *ptr) {
    static __thread int executed = 0;
    if(!executed) {
        executed = 1;
        struct receiver_state *rpt = (struct receiver_state *) ptr;
        // If the shutdown sequence has started, and this receiver is still attached to the host then we cannot touch
        // the host pointer as it is unpredicable when the RRDHOST is deleted. Do the cleanup from rrdhost_free().
        if (netdata_exit && rpt->host) {
            rpt->exited = 1;
            return;
        }

        // Make sure that we detach this thread and don't kill a freshly arriving receiver
        if (!netdata_exit && rpt->host) {
            netdata_mutex_lock(&rpt->host->receiver_lock);
            if (rpt->host->receiver == rpt)
                rpt->host->receiver = NULL;
            netdata_mutex_unlock(&rpt->host->receiver_lock);
        }

        info("STREAM %s [receive from [%s]:%s]: receive thread ended (task id %d)", rpt->hostname, rpt->client_ip, rpt->client_port, gettid());
        destroy_receiver_state(rpt);
    }
}

#include "../collectors/plugins.d/pluginsd_parser.h"

// Added for gap-filling, if this proves to be a bottleneck in large-scale systems then we will need to cache
// the last entry times as the metric updates, but let's see if it is a problem first.
/*
    FIRST APPROACH - NOT USED BUT LEFT FOR REFERENCE

time_t rrdhost_last_entry_t(RRDHOST *h) {
    rrdhost_rdlock(h);
    RRDSET *st;
    time_t now = now_realtime_sec();
    time_t result = now;
    rrdset_foreach_read(st, h) {
        RRDDIM *rd;
        netdata_rwlock_rdlock(&st->rrdset_rwlock);
        rrddim_foreach_read(rd, st) {
            time_t last_update_t, last_collect_t;
            if (st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
                last_update_t = rd->state->query_ops.latest_time(rd);
            else
                last_update_t = st->last_updated.tv_sec;
            last_collect_t = rd->last_collected_time.tv_sec;
            if (last_collect_t > 0) {
                if (last_collect_t < result)
                    result = last_collect_t;
            }
        }
        netdata_rwlock_unlock(&st->rrdset_rwlock);
        //if (st_last > now)
        //    info("Skipping %s -> future update! %ld is %ld ahead", st->name, st_last-now);
        //else {
        //    info("Chart %s last %ld global %ld gap=%ld", st->name, st_last, result, now-st_last);
        //    if (st_last < result)
        //        result = st_last;
    }
    rrdhost_unlock(h);
    return result;
}
*/

// TODO-GAPS: we can't drop the message here, with the chart in replication mode it will kill the
//            link. Probably ened to put a proper poll() / buffer in the parser main loop....
void send_replication_req(RRDHOST *host, char *st_id, time_t start, time_t end) {
    char message[RRD_ID_LENGTH_MAX+60];
    sprintf(message,"REPLICATE %s %ld %ld\n", st_id, start, end);
    int ret;
    debug(D_STREAM, "Replicate command: %s",message);
#ifdef ENABLE_HTTPS
    SSL *conn = host->stream_ssl.conn ;
    if(conn && !host->stream_ssl.flags) {
        ret = SSL_write(conn, message, strlen(message));
    } else {
        ret = send(host->receiver->fd, message, strlen(message), MSG_DONTWAIT);
    }
#else
    ret = send(host->receiver->fd, message, strlen(message), MSG_DONTWAIT);
#endif
    if (ret != (int)strlen(message))
        error("Failed to send replication request for %s!", st_id);
}

/* Streaming has its own parsing of the BEGIN to allow detection of gaps in the data and trigger replication
   of the missing data from the child node. There are two relevant configuration options in stream.conf that
   can be set at the API_KEY or MGUID level:
       max gap replication -> limits the replication to the last n seconds
       history gap replication -> limits the replication on initial connection to the last n seconds
   For data at expected times drop into the pluginsd parser to process.
*/
PARSER_RC streaming_begin_action(void *user_v, RRDSET *st, usec_t microseconds, usec_t remote_clock) {
    PARSER_USER_OBJECT *user = user_v;
    struct receiver_state *rpt = user->opaque;
    time_t remote_t = (time_t)remote_clock;
    netdata_mutex_lock(&st->shared_flags_lock);
    time_t now = now_realtime_sec();
    // TODO: we should supress this on the other side and use this as an indicator that the two sides are out of
    //       sync so that we can trigger recovery.
    if (st->sflag_replicating_down) {
        debug(D_STREAM, "Ignoring data stream for %s @ %llu during replication", st->name, remote_clock);
        netdata_mutex_unlock(&st->shared_flags_lock);
        return PARSER_RC_OK;
    }
    if (st->last_updated.tv_sec == 0 && rpt->gap_history==0) {
        netdata_mutex_unlock(&st->shared_flags_lock);
        debug(D_REPLICATION, "First data value for %s, no historical gaps. remote=%llu offset=%llu last_collected=%ld",
                             user->st->name, remote_clock, microseconds, (long)st->last_collected_time.tv_sec);
        return pluginsd_begin_action(user_v, st, microseconds, remote_clock);
    }
    if (st->last_updated.tv_sec == 0) {
        st->sflag_replicating_down = 1;
        netdata_mutex_unlock(&st->shared_flags_lock);
        send_replication_req(user->host, st->id, now - rpt->gap_history + 1, now);
        return PARSER_RC_OK;
    }
    // The collected value is sent before interpolation so the last_updated time is not for the new value, it is
    // the most recent timestamp before the new point. We do not know in advance how many stored points this collected
    // value will produce on interpolation.
    time_t expected_t = st->last_updated.tv_sec;
    debug(D_REPLICATION, "BEGIN on %s, last_updated=%ld remote=%llu offset=%llu", st->name,
                         (long)st->last_updated.tv_sec, remote_clock, microseconds);
    if (remote_t < expected_t) {
        debug(D_REPLICATION, "Stale value received %s @%ld - last replication window covered?", st->id, remote_t);
        rpt->skip_one_value = 1;    // Drop this update
        netdata_mutex_unlock(&st->shared_flags_lock);
        return PARSER_RC_OK;
    }
    // There are two cases to drop into replication mode during an ongoing stream:
    //   - the data is old, this is probably a stale buffer that arrived as part of a reconnection
    //   - the data contains a gap, either the connection dropped or this node restarted
    if ( (remote_t - expected_t > st->update_every * 2) || (now - remote_t > st->update_every*2))
    {
        debug(D_REPLICATION, "Gap detected in chart data %s: remote=%ld expected=%ld local=%ld", st->name, remote_t,
                             expected_t, now);
        time_t gap_length = now - expected_t;
        if (gap_length > rpt->max_gap) {
            gap_length = rpt->max_gap;
            expected_t = now - gap_length;
        }
        if (gap_length > 0) {
            st->sflag_replicating_down = 1;
            netdata_mutex_unlock(&st->shared_flags_lock);
            send_replication_req(user->host, st->id, expected_t, now);
            return PARSER_RC_OK;
        }
        // else fall-through to ignoring the gap
    }
    netdata_mutex_unlock(&st->shared_flags_lock);
    return pluginsd_begin_action(user_v, st, microseconds, remote_clock);
}

PARSER_RC streaming_set_action(void *user_v, RRDSET *st, RRDDIM *rd, long long int value) {
    PARSER_USER_OBJECT *user = user_v;
    struct receiver_state *rpt = user->opaque;
    netdata_mutex_lock(&st->shared_flags_lock);
    int replicating = st->sflag_replicating_down;
    netdata_mutex_unlock(&st->shared_flags_lock);
    if (replicating || rpt->skip_one_value)
        return PARSER_RC_OK;
    return pluginsd_set_action(user_v, st, rd, value);
}

PARSER_RC streaming_end_action(void *user_v, RRDSET *st) {
    PARSER_USER_OBJECT *user = user_v;
    struct receiver_state *rpt = user->opaque;
    netdata_mutex_lock(&st->shared_flags_lock);
    int replicating = st->sflag_replicating_down;
    netdata_mutex_unlock(&st->shared_flags_lock);
    if (replicating)
        return PARSER_RC_OK;
    if (rpt->skip_one_value) {
        rpt->skip_one_value = 0;
        return PARSER_RC_OK;
    }
    return pluginsd_end_action(user_v, st);
}

PARSER_RC streaming_rep_begin(char **words, void *user_v, PLUGINSD_ACTION *plugins_action) {
    PARSER_USER_OBJECT *user = user_v;
    UNUSED(plugins_action);
    char *id = words[1];
    char *start_txt = words[2];
    char *end_txt = words[3];
    time_t end_t;
    if (!id || !start_txt || !end_txt)
        goto disable;

    RRDSET *st = NULL;
    st = rrdset_find(user->host, id);
    if (unlikely(!st))
        goto disable;
    user->st = st;
    netdata_mutex_lock(&st->shared_flags_lock);
    int replicating = st->sflag_replicating_down;
    netdata_mutex_unlock(&st->shared_flags_lock);
    if (!replicating) {
        errno = 0;
        error("Received REPBEGIN on %s - but chart is not replicating!", st->name);
        return PARSER_RC_ERROR;
    }

    user->st->gap_start = str2ull(start_txt);
    end_t = str2ull(end_txt);

/*    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
#ifdef ENABLE_DBENGINE
        if (RRD_MEMORY_MODE_DBENGINE == st->rrd_memory_mode && !rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
            rrdeng_store_metric_flush_current_page(rd);
        }
#endif
    }*/
    time_t now = now_realtime_sec();
    debug(D_REPLICATION, "Replication started on %s for interval %zu..%ld against current gap %ld..%ld", st->name, 
          user->st->gap_start, end_t, st->last_updated.tv_sec, now);
    return PARSER_RC_OK;
disable:
    errno = 0;
    error("Gap replication failed - Invalid REPBEGIN %s %s %s on host %s. Disabling it.", words[1], words[2], words[3], 
          user->host->hostname);
    user->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC streaming_rep_meta(char **words, void *user_v, PLUGINSD_ACTION *plugins_action) {
    PARSER_USER_OBJECT *user = user_v;
    UNUSED(plugins_action);
    char *id = words[1];
    char *last_collected_str = words[2];
    char *collected_str = words[3];
    char *collected_max_str = words[4];
    char *last_stored_str = words[5];
    char *calculated_str = words[6];
    char *last_calculated_str = words[7];

    if ( !id || !last_collected_str || !collected_str || !collected_max_str || !last_stored_str || !calculated_str ||
         !last_calculated_str )
        goto disable;

    if (user->st == NULL) {
        errno = 0;
        error("Received RRDMETA out of sequence");
        goto disable;
    }
    RRDDIM *rd = rrddim_find(user->st, id);
    if (rd == NULL) {
        errno = 0;
        error("Unknown dimension \"%s\" on %s during replication - ignoring", id, user->st->name);
        return PARSER_RC_OK;
    }
    rd->last_collected_value = strtoll(last_collected_str, NULL, 0);
    rd->collected_value = strtoll(collected_str, NULL, 0);
    rd->collected_value_max = strtoll(collected_max_str, NULL, 0);
    rd->last_stored_value = strtod(last_stored_str, NULL);
    rd->calculated_value = strtod(calculated_str, NULL);
    rd->last_calculated_value = strtod(last_calculated_str, NULL);
    debug(D_REPLICATION, "Replication of %s.%s: last_col_val=" COLLECTED_NUMBER_FORMAT " col_val=" COLLECTED_NUMBER_FORMAT " col_val_max=" COLLECTED_NUMBER_FORMAT " last_store=" CALCULATED_NUMBER_FORMAT " calc_val=" CALCULATED_NUMBER_FORMAT " last_calc_val=" CALCULATED_NUMBER_FORMAT,
          user->st->id, id, rd->last_collected_value, rd->collected_value, rd->collected_value_max,
          rd->last_stored_value, rd->calculated_value, rd->last_calculated_value);
    return PARSER_RC_OK;
disable:
    errno = 0;
    error("Gap replication failed - Invalid REPMETA %s %s %s %s %s %s %s on host %s. Disabling it.", words[1], words[2], 
          words[3], words[4], words[5], words[6], words[7], user->host->hostname);
    user->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC streaming_rep_end(char **words, void *user_v, PLUGINSD_ACTION *plugins_action) {
    PARSER_USER_OBJECT *user = user_v;
    UNUSED(plugins_action);
    RRDDIM *rd;
    char *num_points_txt = words[1];
    char *col_total_txt  = words[2];
    char *last_total_txt = words[3];
    char *col_time_txt   = words[4];
    time_t advance_in_secs;
    long advance_in_points;
    if (!num_points_txt)
        goto disable;
    size_t num_points = str2ull(num_points_txt);
    total_number col_total = str2ll(col_total_txt, NULL);
    total_number last_total = str2ll(last_total_txt, NULL);
    usec_t last_collect_ut = str2ull(col_time_txt);  // The chart-level last_collected_time in usec

    /* If data was transferred in the replication window then we do not know how the number of points relates to the
       length of the window or the distribution amongst the dimension, so update the chart-level timestamps from
       the dimensions. */
    if (num_points > 0) {
        time_t latest_collect_t = 0;
        netdata_rwlock_rdlock(&user->st->rrdset_rwlock);
        rrddim_foreach_read(rd, user->st) {
            if (rd->last_collected_time.tv_sec > latest_collect_t)
                latest_collect_t = rd->last_collected_time.tv_sec;
        }
        netdata_rwlock_unlock(&user->st->rrdset_rwlock);
        if (user->st->last_updated.tv_sec == 0)
            advance_in_secs = latest_collect_t - user->st->gap_start + user->st->update_every;
        else
            advance_in_secs = latest_collect_t - user->st->last_updated.tv_sec + user->st->update_every;
        advance_in_points = advance_in_secs / user->st->update_every;
        if ((advance_in_secs % user->st->update_every) != 0)
            debug(D_REPLICATION, "Timestamps are mis-aligned during replication!");

        // Timestamps stored during replication are storage times (db timestamps) not collection times.
        user->st->last_updated.tv_sec        = latest_collect_t;
        // This needs usec precision to replicate the interpolators on both machines (it drives the deltas)
        user->st->last_collected_time.tv_sec = last_collect_ut / USEC_PER_SEC;
        user->st->last_collected_time.tv_usec = last_collect_ut % USEC_PER_SEC;

        user->st->counter       += advance_in_points;
        user->st->counter_done  += advance_in_points;
        user->st->current_entry += advance_in_points;
        user->st->usec_since_last_update = USEC_PER_SEC;
        while (user->st->current_entry >= user->st->entries)        // Once except for an exceptional corner-case
            user->st->current_entry -= user->st->entries;
        user->st->collected_total = col_total;
        user->st->last_collected_total = last_total;

        debug(D_REPLICATION, "Finished replication on %s: %zu points transferred, advance=(%ld-secs %ld-points)"
                             " last_col_time->%ld.%ld last_up_time->%ld col_total=%lld last_col_total=%lld",
                             user->st->name, num_points, advance_in_secs, advance_in_points,
                             user->st->last_collected_time.tv_sec, user->st->last_collected_time.tv_usec,
                             user->st->last_updated.tv_sec, user->st->collected_total, user->st->last_collected_total);
    }
    else
        debug(D_REPLICATION, "Finished replication on %s: window was empty", user->st->name);

    netdata_mutex_lock(&user->st->shared_flags_lock);
    user->st->sflag_replicating_down = 0;
    netdata_mutex_unlock(&user->st->shared_flags_lock);

    debug_dump_rrdset_state(user->st);
    // Prevent 1-sec gap after replication, this is independent of the same flag on the sender. If the sender is
    // not storing first then another replication will be triggered after the empty window.
    rrdset_flag_set(user->st, RRDSET_FLAG_STORE_FIRST);
    user->st = NULL;
    return PARSER_RC_OK;
disable:
    errno = 0;
    error("Gap replication failed - Invalid REPEND %s on host %s. Disabling it.", words[1], user->host->hostname);
    user->enabled = 0;
    return PARSER_RC_ERROR;
}


PARSER_RC streaming_rep_dim(char **words, void *user_v, PLUGINSD_ACTION *plugins_action) {
    PARSER_USER_OBJECT *user = user_v;
    UNUSED(plugins_action);
    char *id = words[1];
    char *idx_txt = words[2];
    char *time_txt = words[3];
    char *value_txt = words[4];

    if (user->st == NULL) {
        errno = 0;
        error("Received RRDDIM out of sequence");
        goto disable;
    }

    if (!id || !idx_txt || !time_txt || !value_txt)
        goto disable;

    // Remote clock or local clock with slew estimate?
    time_t timestamp = (time_t)str2ul(time_txt);
    storage_number value = str2ull(value_txt);
    size_t idx = str2ul(idx_txt);

    RRDDIM *rd = rrddim_find(user->st, id);
    //time_t st_last = rrdset_last_entry_t(user->st);  UNUSED?
    if (rd == NULL) {
        errno = 0;
        error("Unknown dimension \"%s\" on %s during replication - ignoring", id, user->st->name);
        return PARSER_RC_OK;
    }

    if (user->st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
    {
        rd->state->collect_ops.store_metric(rd, timestamp * USEC_PER_SEC, value);
        debug(D_REPLICATION, "store " STORAGE_NUMBER_FORMAT "@%ld for %s.%s (idx %zu)", value, timestamp, user->st->id,
                             id, idx);
        rd->last_stored_value = value;
        if (rd->state->handle.rrdeng.descr)
            debug(D_REPLICATION, "page_descr %llu - %llu with %u", rd->state->handle.rrdeng.descr->start_time,
                  rd->state->handle.rrdeng.descr->end_time, rd->state->handle.rrdeng.descr->page_length);
    }
    else
        rd->values[(rd->rrdset->current_entry + idx) % rd->entries] = value;
    rd->last_collected_time.tv_sec = timestamp; // Technically these are storage times but maintain legacy modes
    rd->last_collected_time.tv_usec = 0;        // Chart usec is controlled by rrdset_next_usec_slew
    rd->collections_counter++;

    return PARSER_RC_OK;
disable:
    error("Gap replication failed - Invalid REPDIM %s %s %s on host %s. Disabling it.", words[1], words[2], words[3],
          user->host->hostname);
    user->enabled = 0;
    return PARSER_RC_ERROR;
}

/* The receiver socket is blocking, perform a single read into a buffer so that we can reassemble lines for parsing.
 */
static int receiver_read(struct receiver_state *r, FILE *fp) {
#ifdef ENABLE_HTTPS
    if (r->ssl.conn && !r->ssl.flags) {
        ERR_clear_error();
        int desired = sizeof(r->read_buffer) - r->read_len - 1;
        int ret = SSL_read(r->ssl.conn, r->read_buffer + r->read_len, desired);
        if (ret > 0 ) {
            r->read_len += ret;
            return 0;
        }
        // Don't treat SSL_ERROR_WANT_READ or SSL_ERROR_WANT_WRITE differently on blocking socket
        u_long err;
        char buf[256];
        while ((err = ERR_get_error()) != 0) {
            ERR_error_string_n(err, buf, sizeof(buf));
            error("STREAM %s [receive from %s] ssl error: %s", r->hostname, r->client_ip, buf);
        }
        return 1;
    }
#endif
    if (!fgets(r->read_buffer, sizeof(r->read_buffer), fp))
        return 1;
    r->read_len = strlen(r->read_buffer);
    return 0;
}

/* Produce a full line if one exists, statefully return where we start next time.
 * When we hit the end of the buffer with a partial line move it to the beginning for the next fill.
 */
static char *receiver_next_line(struct receiver_state *r, int *pos) {
    int start = *pos, scan = *pos;
    if (scan >= r->read_len) {
        r->read_len = 0;
        return NULL;
    }
    while (scan < r->read_len && r->read_buffer[scan] != '\n')
        scan++;
    if (scan < r->read_len && r->read_buffer[scan] == '\n') {
        *pos = scan+1;
        r->read_buffer[scan] = 0;
        return &r->read_buffer[start];
    }
    memcpy(r->read_buffer, &r->read_buffer[start], r->read_len - start);
    r->read_len -= start;
    return NULL;
}


size_t streaming_parser(struct receiver_state *rpt, struct plugind *cd, FILE *fp) {
    size_t result;
    PARSER_USER_OBJECT *user = callocz(1, sizeof(*user));
    user->enabled = cd->enabled;
    user->host = rpt->host;
    user->opaque = rpt;
    user->cd = cd;
    if (cd->version >= VERSION_GAP_FILLING)
        user->usec_semantics = PLUGINSD_USEC_SLEW;
    else
        user->usec_semantics = PLUGINSD_USEC_TRUST;

    PARSER *parser = parser_init(rpt->host, user, fp, PARSER_INPUT_SPLIT);
    parser_add_keyword(parser, "REPBEGIN", streaming_rep_begin);
    parser_add_keyword(parser, "REPDIM", streaming_rep_dim);
    parser_add_keyword(parser, "REPEND", streaming_rep_end);
    parser_add_keyword(parser, "REPMETA", streaming_rep_meta);

    if (unlikely(!parser)) {
        error("Failed to initialize parser");
        cd->serial_failures++;
        freez(user);
        return 0;
    }

    parser->plugins_action->begin_action     = &streaming_begin_action;
    parser->plugins_action->set_action       = &streaming_set_action;
    parser->plugins_action->end_action       = &streaming_end_action;
    parser->plugins_action->flush_action     = &pluginsd_flush_action;
    parser->plugins_action->disable_action   = &pluginsd_disable_action;
    parser->plugins_action->variable_action  = &pluginsd_variable_action;
    parser->plugins_action->dimension_action = &pluginsd_dimension_action;
    parser->plugins_action->label_action     = &pluginsd_label_action;
    parser->plugins_action->overwrite_action = &pluginsd_overwrite_action;
    parser->plugins_action->chart_action     = &pluginsd_chart_action;

    user->parser = parser;

    do {
        if (receiver_read(rpt, fp))
            break;
        int pos = 0;
        char *line;
        while ((line = receiver_next_line(rpt, &pos))) {
            if (unlikely(netdata_exit || rpt->shutdown || parser_action(parser,  line)))
                goto done;
        }
        rpt->last_msg_t = now_realtime_sec();
    }
    while(!netdata_exit);
done:
    result= user->count;
    freez(user);
    parser_destroy(parser);
    return result;
}


static int rrdpush_receive(struct receiver_state *rpt)
{
    int history = default_rrd_history_entries;
    RRD_MEMORY_MODE mode = default_rrd_memory_mode;
    int health_enabled = default_health_enabled;
    int rrdpush_enabled = default_rrdpush_enabled;
    char *rrdpush_destination = default_rrdpush_destination;
    char *rrdpush_api_key = default_rrdpush_api_key;
    char *rrdpush_send_charts_matching = default_rrdpush_send_charts_matching;
    time_t alarms_delay = 60;

    rpt->update_every = (int)appconfig_get_number(&stream_config, rpt->machine_guid, "update every", rpt->update_every);
    if(rpt->update_every < 0) rpt->update_every = 1;

    history = (int)appconfig_get_number(&stream_config, rpt->key, "default history", history);
    history = (int)appconfig_get_number(&stream_config, rpt->machine_guid, "history", history);
    if(history < 5) history = 5;

    mode = rrd_memory_mode_id(appconfig_get(&stream_config, rpt->key, "default memory mode", rrd_memory_mode_name(mode)));
    mode = rrd_memory_mode_id(appconfig_get(&stream_config, rpt->machine_guid, "memory mode", rrd_memory_mode_name(mode)));

    health_enabled = appconfig_get_boolean_ondemand(&stream_config, rpt->key, "health enabled by default", health_enabled);
    health_enabled = appconfig_get_boolean_ondemand(&stream_config, rpt->machine_guid, "health enabled", health_enabled);

    alarms_delay = appconfig_get_number(&stream_config, rpt->key, "default postpone alarms on connect seconds", alarms_delay);
    alarms_delay = appconfig_get_number(&stream_config, rpt->machine_guid, "postpone alarms on connect seconds", alarms_delay);

    rrdpush_enabled = appconfig_get_boolean(&stream_config, rpt->key, "default proxy enabled", rrdpush_enabled);
    rrdpush_enabled = appconfig_get_boolean(&stream_config, rpt->machine_guid, "proxy enabled", rrdpush_enabled);

    rrdpush_destination = appconfig_get(&stream_config, rpt->key, "default proxy destination", rrdpush_destination);
    rrdpush_destination = appconfig_get(&stream_config, rpt->machine_guid, "proxy destination", rrdpush_destination);

    rrdpush_api_key = appconfig_get(&stream_config, rpt->key, "default proxy api key", rrdpush_api_key);
    rrdpush_api_key = appconfig_get(&stream_config, rpt->machine_guid, "proxy api key", rrdpush_api_key);

    rrdpush_send_charts_matching = appconfig_get(&stream_config, rpt->key, "default proxy send charts matching", rrdpush_send_charts_matching);
    rrdpush_send_charts_matching = appconfig_get(&stream_config, rpt->machine_guid, "proxy send charts matching", rrdpush_send_charts_matching);

    rpt->gap_history = appconfig_get_number(&stream_config, rpt->key, "history gap replication", rpt->gap_history);
    rpt->gap_history = appconfig_get_number(&stream_config, rpt->machine_guid, "history gap replication", rpt->gap_history);

    rpt->max_gap = appconfig_get_number(&stream_config, rpt->key, "max gap replication", rpt->max_gap);
    rpt->max_gap = appconfig_get_number(&stream_config, rpt->machine_guid, "max gap replication", rpt->max_gap);

    rpt->tags = (char*)appconfig_set_default(&stream_config, rpt->machine_guid, "host tags", (rpt->tags)?rpt->tags:"");
    if(rpt->tags && !*rpt->tags) rpt->tags = NULL;

    if (strcmp(rpt->machine_guid, localhost->machine_guid) == 0) {
        log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->machine_guid, rpt->hostname, "DENIED - ATTEMPT TO RECEIVE METRICS FROM MACHINE_GUID IDENTICAL TO PARENT");
        error("STREAM %s [receive from %s:%s]: denied to receive metrics, machine GUID [%s] is my own. Did you copy the parent/proxy machine GUID to a child?", rpt->hostname, rpt->client_ip, rpt->client_port, rpt->machine_guid);
        close(rpt->fd);
        return 1;
    }

    if (rpt->host==NULL) {

        rpt->host = rrdhost_find_or_create(
                rpt->hostname
                , rpt->registry_hostname
                , rpt->machine_guid
                , rpt->os
                , rpt->timezone
                , rpt->tags
                , rpt->program_name
                , rpt->program_version
                , rpt->update_every
                , history
                , mode
                , (unsigned int)(health_enabled != CONFIG_BOOLEAN_NO)
                , (unsigned int)(rrdpush_enabled && rrdpush_destination && *rrdpush_destination && rrdpush_api_key && *rrdpush_api_key)
                , rrdpush_destination
                , rrdpush_api_key
                , rrdpush_send_charts_matching
                , rpt->system_info
        );

        if(!rpt->host) {
            close(rpt->fd);
            log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->machine_guid, rpt->hostname, "FAILED - CANNOT ACQUIRE HOST");
            error("STREAM %s [receive from [%s]:%s]: failed to find/create host structure.", rpt->hostname, rpt->client_ip, rpt->client_port);
            return 1;
        }

        netdata_mutex_lock(&rpt->host->receiver_lock);
        if (rpt->host->receiver == NULL)
            rpt->host->receiver = rpt;
        else {
            error("Multiple receivers connected for %s concurrently, cancelling this one...", rpt->machine_guid);
            netdata_mutex_unlock(&rpt->host->receiver_lock);
            close(rpt->fd);
            log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->machine_guid, rpt->hostname, "FAILED - BEATEN TO HOST CREATION");
            return 1;
        }
        netdata_mutex_unlock(&rpt->host->receiver_lock);
    }

    int ssl = 0;
#ifdef ENABLE_HTTPS
    if (rpt->ssl.conn != NULL)
        ssl = 1;
#endif

#ifdef NETDATA_INTERNAL_CHECKS
    info("STREAM %s [receive from [%s]:%s]: client willing to stream metrics for host '%s' with machine_guid '%s': update every = %d, history = %ld, memory mode = %s, health %s,%s tags '%s'"
         , rpt->hostname
         , rpt->client_ip
         , rpt->client_port
         , rpt->host->hostname
         , rpt->host->machine_guid
         , rpt->host->rrd_update_every
         , rpt->host->rrd_history_entries
         , rrd_memory_mode_name(rpt->host->rrd_memory_mode)
         , (health_enabled == CONFIG_BOOLEAN_NO)?"disabled":((health_enabled == CONFIG_BOOLEAN_YES)?"enabled":"auto")
         , ssl ? " SSL," : ""
         , rpt->host->tags?rpt->host->tags:""
    );
#endif // NETDATA_INTERNAL_CHECKS


    struct plugind cd = {
            .enabled = 1,
            .update_every = default_rrd_update_every,
            .pid = 0,
            .serial_failures = 0,
            .successful_collections = 0,
            .obsolete = 0,
            .started_t = now_realtime_sec(),
            .next = NULL,
            .version = 0,
    };

    // put the client IP and port into the buffers used by plugins.d
    snprintfz(cd.id,           CONFIG_MAX_NAME,  "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(cd.filename,     FILENAME_MAX,     "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(cd.fullfilename, FILENAME_MAX,     "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(cd.cmd,          PLUGINSD_CMD_MAX, "%s:%s", rpt->client_ip, rpt->client_port);

    info("STREAM %s [receive from [%s]:%s]: initializing communication...", rpt->host->hostname, rpt->client_ip, rpt->client_port);
    char initial_response[HTTP_HEADER_SIZE];
    if (rpt->stream_version > 1) {
        info("STREAM %s [receive from [%s]:%s]: Netdata is using the stream version %u.", rpt->host->hostname, rpt->client_ip, rpt->client_port, rpt->stream_version);
        sprintf(initial_response, "%s%u", START_STREAMING_PROMPT_VN, rpt->stream_version);
    } else if (rpt->stream_version == 1) {
        info("STREAM %s [receive from [%s]:%s]: Netdata is using the stream version %u.", rpt->host->hostname, rpt->client_ip, rpt->client_port, rpt->stream_version);
        sprintf(initial_response, "%s", START_STREAMING_PROMPT_V2);
    } else {
        info("STREAM %s [receive from [%s]:%s]: Netdata is using first stream protocol.", rpt->host->hostname, rpt->client_ip, rpt->client_port);
        sprintf(initial_response, "%s", START_STREAMING_PROMPT);
    }
    debug(D_STREAM, "Initial response to %s: %s", rpt->client_ip, initial_response);
    #ifdef ENABLE_HTTPS
    rpt->host->stream_ssl.conn = rpt->ssl.conn;
    rpt->host->stream_ssl.flags = rpt->ssl.flags;
    if(send_timeout(&rpt->ssl, rpt->fd, initial_response, strlen(initial_response), 0, 60) != (ssize_t)strlen(initial_response)) {
#else
    if(send_timeout(rpt->fd, initial_response, strlen(initial_response), 0, 60) != strlen(initial_response)) {
#endif
        log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->host->machine_guid, rpt->host->hostname, "FAILED - CANNOT REPLY");
        error("STREAM %s [receive from [%s]:%s]: cannot send ready command.", rpt->host->hostname, rpt->client_ip, rpt->client_port);
        close(rpt->fd);
        return 0;
    }

    // remove the non-blocking flag from the socket
    if(sock_delnonblock(rpt->fd) < 0)
        error("STREAM %s [receive from [%s]:%s]: cannot remove the non-blocking flag from socket %d", rpt->host->hostname, rpt->client_ip, rpt->client_port, rpt->fd);

    // convert the socket to a FILE *
    FILE *fp = fdopen(rpt->fd, "r");
    if(!fp) {
        log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->host->machine_guid, rpt->host->hostname, "FAILED - SOCKET ERROR");
        error("STREAM %s [receive from [%s]:%s]: failed to get a FILE for FD %d.", rpt->host->hostname, rpt->client_ip, rpt->client_port, rpt->fd);
        close(rpt->fd);
        return 0;
    }

    rrdhost_wrlock(rpt->host);
/* if(rpt->host->connected_senders > 0) {
        rrdhost_unlock(rpt->host);
        log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->host->machine_guid, rpt->host->hostname, "REJECTED - ALREADY CONNECTED");
        info("STREAM %s [receive from [%s]:%s]: multiple streaming connections for the same host detected. Rejecting new connection.", rpt->host->hostname, rpt->client_ip, rpt->client_port);
        fclose(fp);
        return 0;
    }
*/

    // MAYBE HERE?
    rrdhost_flag_clear(rpt->host, RRDHOST_FLAG_ORPHAN);
//    rpt->host->connected_senders++;
    rpt->host->senders_disconnected_time = 0;
    rpt->host->labels_flag = (rpt->stream_version > 0)?LABEL_FLAG_UPDATE_STREAM:LABEL_FLAG_STOP_STREAM;

    if(health_enabled != CONFIG_BOOLEAN_NO) {
        if(alarms_delay > 0) {
            rpt->host->health_delay_up_to = now_realtime_sec() + alarms_delay;
            info("Postponing health checks for %ld seconds, on host '%s', because it was just connected."
            , alarms_delay
            , rpt->host->hostname
            );
        }
    }
    rrdhost_unlock(rpt->host);

    // call the plugins.d processor to receive the metrics
    info("STREAM %s [receive from [%s]:%s]: receiving metrics...", rpt->host->hostname, rpt->client_ip, rpt->client_port);
    log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->host->machine_guid, rpt->host->hostname, "CONNECTED");

    cd.version = rpt->stream_version;


    size_t count = streaming_parser(rpt, &cd, fp);

    log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->host->machine_guid, rpt->hostname,
                          "DISCONNECTED");
    error("STREAM %s [receive from [%s]:%s]: disconnected (completed %zu updates).", rpt->hostname, rpt->client_ip,
          rpt->client_port, count);

    // During a shutdown there is cleanup code in rrdhost that will cancel the sender thread
    if (!netdata_exit && rpt->host) {
        netdata_mutex_lock(&rpt->host->receiver_lock);
        if (rpt->host->receiver == rpt) {
            rrdhost_wrlock(rpt->host);
            rpt->host->senders_disconnected_time = now_realtime_sec();
            rrdhost_flag_set(rpt->host, RRDHOST_FLAG_ORPHAN);
            if(health_enabled == CONFIG_BOOLEAN_AUTO)
                rpt->host->health_enabled = 0;
            rrdhost_unlock(rpt->host);
            rrdpush_sender_thread_stop(rpt->host);
        }
        netdata_mutex_unlock(&rpt->host->receiver_lock);
    }

    // cleanup
    fclose(fp);
    return (int)count;
}

void *rrdpush_receiver_thread(void *ptr) {
    netdata_thread_cleanup_push(rrdpush_receiver_thread_cleanup, ptr);

    struct receiver_state *rpt = (struct receiver_state *)ptr;
    info("STREAM %s [%s]:%s: receive thread created (task id %d)", rpt->hostname, rpt->client_ip, rpt->client_port, gettid());

    rrdpush_receive(rpt);

    netdata_thread_cleanup_pop(1);
    return NULL;
}

