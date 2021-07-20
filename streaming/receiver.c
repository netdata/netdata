// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"

extern struct config stream_config;

static void receiver_tx_enq_cmd(struct receiver_state *rpt, struct replication_req *req)
{
    unsigned queue_size;

    uv_mutex_lock(&rpt->cmd_queue.cmd_mutex);
    /* wait for free space in queue */
    while ((queue_size = rpt->cmd_queue.queue_size) == RECEIVER_CMD_Q_MAX_SIZE) {
        char message[RRD_ID_LENGTH_MAX + 60];
        snprintf(message, RRD_ID_LENGTH_MAX + 60, "REPLICATE %s %ld %ld", req->st_id, req->start, req->end);
        error("Replicate command: \"%s\" blocked due to full recv TX command queue.", message);

        uv_mutex_unlock(&rpt->cmd_queue.cmd_mutex);
        (void)sleep_usec(10000); /* 10 msec */
        uv_mutex_lock(&rpt->cmd_queue.cmd_mutex);
    }
    fatal_assert(queue_size < RECEIVER_CMD_Q_MAX_SIZE);
    /* enqueue command */
    rpt->cmd_queue.cmd_array[rpt->cmd_queue.tail] = *req;
    rpt->cmd_queue.tail = rpt->cmd_queue.tail != RECEIVER_CMD_Q_MAX_SIZE - 1 ?
                          rpt->cmd_queue.tail + 1 : 0;
    rpt->cmd_queue.queue_size = queue_size + 1;

    /* wake up consumer */
    uv_cond_signal(&rpt->cmd_queue.cmd_cond);
    uv_mutex_unlock(&rpt->cmd_queue.cmd_mutex);
}

static struct replication_req receiver_tx_deq_cmd(struct receiver_state *rpt)
{
    struct replication_req ret;
    unsigned queue_size;

    uv_mutex_lock(&rpt->cmd_queue.cmd_mutex);
    while (0 == (queue_size = rpt->cmd_queue.queue_size)) {
        uv_cond_wait(&rpt->cmd_queue.cmd_cond, &rpt->cmd_queue.cmd_mutex);
    }
    /* dequeue command */
    ret = rpt->cmd_queue.cmd_array[rpt->cmd_queue.head];
    if (queue_size == 1) {
        rpt->cmd_queue.head = rpt->cmd_queue.tail = 0;
    } else {
        rpt->cmd_queue.head = rpt->cmd_queue.head != RECEIVER_CMD_Q_MAX_SIZE - 1 ?
                              rpt->cmd_queue.head + 1 : 0;
    }
    rpt->cmd_queue.queue_size = queue_size - 1;
    uv_mutex_unlock(&rpt->cmd_queue.cmd_mutex);

    return ret;
}

static void receiver_tx_thread_stop(struct receiver_state *rpt)
{
    struct replication_req flush_req;

    uv_mutex_lock(&rpt->cmd_queue.cmd_mutex);
    rpt->cmd_queue.stop_thread = 1;
    uv_mutex_unlock(&rpt->cmd_queue.cmd_mutex);

    flush_req.host = NULL; /* mark this a no-op request to wake up the receiver tx thread */
    receiver_tx_enq_cmd(rpt, &flush_req);

    info("STREAM %s [receive from %s:%s]: waiting for the receiver TX thread to stop...", rpt->hostname, rpt->client_ip,
         rpt->client_port);

    netdata_thread_join(rpt->receiver_tx_thread, NULL);
    info("STREAM %s [receive from %s:%s]: the receiver TX thread has exited.", rpt->hostname, rpt->client_ip,
         rpt->client_port);
}

void send_replication_req(RRDHOST *host, char *st_id, time_t start, time_t end);

static void *receiver_tx_thread(void *ptr)
{
    struct receiver_state *rpt = (struct receiver_state *)ptr;
    info("STREAM %s [%s]:%s: receiver TX thread created (task id %d)", rpt->hostname, rpt->client_ip, rpt->client_port,
         gettid());

    struct replication_req req;

    while (!rpt->cmd_queue.stop_thread) {
        req = receiver_tx_deq_cmd(rpt);
        if (!req.host)
            continue; // no-op
        fatal_assert(req.st_id);
        send_replication_req(req.host, req.st_id, req.start, req.end);
    }

    return NULL;
}

static void receiver_tx_thread_spawn(struct receiver_state *rpt)
{
    char tag[NETDATA_THREAD_TAG_MAX + 1];
    snprintfz(tag, NETDATA_THREAD_TAG_MAX, "STREAM_RECV_TX[%s,[%s]:%s]", rpt->hostname, rpt->client_ip,
              rpt->client_port);

    if(netdata_thread_create(&rpt->receiver_tx_thread, tag, NETDATA_THREAD_OPTION_JOINABLE, receiver_tx_thread,
                              (void *)rpt))
        error("Failed to create new STREAM receive TX thread for client.");
    else
        rpt->receiver_tx_spawn = 1;
}

void destroy_receiver_state(struct receiver_state *rpt) {
    freez(rpt->key);
    freez(rpt->hostname);
    freez(rpt->registry_hostname);
    freez(rpt->machine_guid);
    freez(rpt->os);
    freez(rpt->timezone);
    freez(rpt->abbrev_timezone);
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

        if (rpt->receiver_tx_spawn)
            receiver_tx_thread_stop(rpt);

        if (rpt->host) {
            netdata_mutex_lock(&rpt->host->receiver_lock);
            // If the shutdown sequence has started, and this receiver is still attached to the host then we cannot touch
            // the host pointer as it is unpredicable when the RRDHOST is deleted. Do the cleanup from rrdhost_free().
            if (netdata_exit) {
                rpt->exited = 1;
                netdata_mutex_unlock(&rpt->host->receiver_lock);
                return;
            }

            // Make sure that we detach this thread and don't kill a freshly arriving receiver
            if (!netdata_exit) {
                if (rpt->host->receiver == rpt)
                    rpt->host->receiver = NULL;
            }
            netdata_mutex_unlock(&rpt->host->receiver_lock);
        }

        info("STREAM %s [receive from [%s]:%s]: receive thread ended (task id %d)", rpt->hostname, rpt->client_ip, rpt->client_port, gettid());
        destroy_receiver_state(rpt);
    }
}

#include "collectors/plugins.d/pluginsd_parser.h"

static void enqueue_replication_req(struct receiver_state *rpt, RRDHOST *host, char *st_id, time_t start, time_t end)
{
    struct replication_req req;

    req.host = host;
    req.st_id = st_id;
    req.start = start;
    req.end = end;
    receiver_tx_enq_cmd(rpt, &req);
}


void send_replication_req(RRDHOST *host, char *st_id, time_t start, time_t end) {
    char message[RRD_ID_LENGTH_MAX+60];
    snprintfz(message, RRD_ID_LENGTH_MAX + 60, "REPLICATE %s %ld %ld\n", st_id, start, end);
    int ret;
    debug(D_STREAM, "Replicate command: %s",message);
#ifdef ENABLE_HTTPS
    SSL *conn = host->stream_ssl.conn ;
    if(conn && !host->stream_ssl.flags) {
        ret = SSL_write(conn, message, strlen(message));
    } else {
        ret = send(host->receiver->fd, message, strlen(message), 0);
    }
#else
    ret = send(host->receiver->fd, message, strlen(message), 0);
#endif
    if (ret != (int)strlen(message))
        error("Failed to send replication request for %s!", st_id);
}

static void skip_gap(RRDSET *st, time_t first_t, time_t last_t) {
    // dbengine should handle time jump
    if (st->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
        rrdset_rdlock(st);
        RRDDIM *rd;
        rrddim_foreach_read(rd, st) {
            long current_entry = st->current_entry;
            debug(D_REPLICATION, "Legacy-mode on %s.%s filling empty slots %ld-%ld.",
                  st->name, rd->name, (long)first_t, (long)last_t);
            for (time_t gap_t = first_t; gap_t <= last_t; gap_t += st->update_every) {
                rd->values[current_entry] = SN_EMPTY_SLOT;
                current_entry = ((current_entry + 1) >= st->entries) ? 0 : current_entry + 1;
            }
        }
        rrdset_unlock(st);
    }
    else
        debug(D_REPLICATION, "dbengine on %s skipping gap %ld-%ld.", st->name, (long)first_t, (long)last_t);
    st->last_updated.tv_sec = last_t;
}


PARSER_RC streaming_rep_begin(char **words, void *user_v, PLUGINSD_ACTION *plugins_action) {
    PARSER_USER_OBJECT *user = user_v;
    UNUSED(plugins_action);
    char *id = words[1];
    char *start_txt = words[2];
    char *first_txt = words[3];
    char *end_txt = words[4];
    if (!id || !start_txt || !end_txt)
        goto disable;

    RRDSET *st = rrdset_find(user->host, id);
    if (unlikely(!st))
        goto disable;
    user->st = st;
    st->state->window_start = str2ll(start_txt, NULL);
    st->state->window_first = str2ll(first_txt, NULL);
    st->state->window_end   = str2ll(end_txt, NULL);

    struct receiver_state *rpt = user->opaque;
    time_t now = now_realtime_sec();
    if (st->last_updated.tv_sec == 0) {
        if (rpt->gap_history==0) {
            st->last_updated.tv_sec = st->state->window_start;
            debug(D_REPLICATION, "Initial data for %s, no historical gaps. window=%ld..%ld",
                  st->name, (long)st->state->window_start, (long)st->state->window_end);
            st->state->ignore_block = 0;
            return PARSER_RC_OK;
        } else {
            debug(D_REPLICATION, "Initial data for %s, asking for history window=%ld..%ld",
                  st->name, (long)now - rpt->gap_history+1, now);
            enqueue_replication_req(rpt, user->host, st->id, now - rpt->gap_history + 1, now);
            st->state->ignore_block = 1;
            st->last_updated.tv_sec = now - rpt->gap_history;
            return PARSER_RC_OK;
        }
    }
    time_t expected_t = st->last_updated.tv_sec + st->update_every;
    if (st->state->window_start < expected_t) {
        debug(D_REPLICATION, "Ignoring stale replication on %s block %ld-%ld, last_updated=%ld",
              st->name, (long)st->state->window_start, (long)st->state->window_end,
              st->last_updated.tv_sec);
        st->state->ignore_block = 1;
        return PARSER_RC_OK;
    }

    if (st->state->window_start > expected_t) {
        time_t gap_end   = st->state->window_end;
        time_t gap_points = (gap_end - expected_t) / st->update_every;
        if (gap_points > rpt->max_gap)
            gap_points = rpt->max_gap;
        time_t gap_first = gap_end - gap_points * st->update_every;
        if (gap_first > st->state->window_start) {
            // TODO - put a test scenario in to capture this, triggers a problem in the dbengine
            debug(D_REPLICATION, "Gap detected on %s: expected %ld, block %ld-%ld. Exceeds max_gap %u, ignoring...",
                  st->name, (long)expected_t, (long)st->state->window_start, (long)st->state->window_end, rpt->max_gap);
            skip_gap(st, expected_t, gap_first - st->update_every);
        } else {
            debug(D_REPLICATION, "Gap detected on %s: expected %ld, block %ld-%ld. Requesting %ld-%ld",
                  st->name, (long)expected_t, (long)st->state->window_start, (long)st->state->window_end,
                  (long)gap_first, (long)gap_end);
            if (gap_first > expected_t)
                skip_gap(st, expected_t, gap_first - st->update_every);
            enqueue_replication_req(rpt, user->host, st->id, gap_first, gap_end);
            st->state->ignore_block = 1;
            return PARSER_RC_OK;
        }
    }
    else
        if (st->state->window_first > st->state->window_start)
            skip_gap(st, st->state->window_start, st->state->window_first - st->update_every);

    user->st->state->ignore_block = 0;
    debug(D_REPLICATION, "Replication on %s @ %ld, block %ld/%ld-%ld last_update=%ld", st->name, now,
                         st->state->window_start, st->state->window_first, st->state->window_end,
                         st->last_updated.tv_sec);

    return PARSER_RC_OK;
disable:
    errno = 0;
    error("Replication failed - Invalid REPBEGIN %s %s %s on host %s. Disabling it.", words[1], words[2], words[3],
          user->host->hostname);
    user->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC streaming_rep_dim(char **words, void *user_v, PLUGINSD_ACTION *plugins_action) {
    PARSER_USER_OBJECT *user = user_v;
    UNUSED(plugins_action);
    char *id        = words[1];
    char *time_txt  = words[2];
    char *value_txt = words[3];

    if (user->st == NULL) {
        errno = 0;
        error("Received RRDDIM for %s out of sequence", id);
        goto disable;
    }


    if (!id || !time_txt || !value_txt)
        goto disable;

    time_t timestamp = (time_t)str2ul(time_txt);
    storage_number value = str2ull(value_txt);

    RRDDIM *rd = rrddim_find(user->st, id);
    if (rd == NULL) {
        errno = 0;
        error("Unknown dimension \"%s\" on %s during replication - ignoring", id, user->st->name);
        return PARSER_RC_OK;
    }

    // The sending side sends chart and dimension metadata before replication blocks so it should not be possible
    // to receive a block on an archived dimension. But if the dimension is archived then the dbengine ctx will be
    // uninitialized.
    if (user->st->state->ignore_block || rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED))
        return PARSER_RC_OK;

    if (user->st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
    {
        if (value != SN_EMPTY_SLOT && timestamp > rrddim_last_entry_t(rd)) {
            rd->state->collect_ops.store_metric(rd, timestamp * USEC_PER_SEC, value);
        }
        debug(D_REPLICATION, "store " STORAGE_NUMBER_FORMAT "@%ld for %s.%s (last_val=%p)", value, timestamp, 
              user->st->id, id, &rd->last_stored_value);
    }
    else
    {
        // We are assuming here that data received for each dimension in the window is dense. This is because any
        // gaps from dropped samples should be filled with either SN_EMPTY_SLOT or interpolated values at storage time
        // on the collecting node.
        size_t offset = (size_t)(timestamp - user->st->last_updated.tv_sec) / user->st->update_every;
        rd->values[(rd->rrdset->current_entry + offset) % rd->entries] = value;
        debug(D_REPLICATION, "store " STORAGE_NUMBER_FORMAT "@%ld = %ld + %zu for %s.%s (last_val=%p)", value, timestamp,
              rd->rrdset->current_entry, offset, user->st->id, id, &rd->last_stored_value);
    }
    rd->last_stored_value = unpack_storage_number(value);
    rd->collections_counter++;
    rd->collected_value = rd->last_collected_value = unpack_storage_number(value) / (calculated_number)rd->multiplier *
                                                     (calculated_number)rd->divisor;
    rd->last_collected_time.tv_sec = timestamp;
    rd->last_collected_time.tv_usec = 0;

    return PARSER_RC_OK;
disable:
    error("Gap replication failed - Invalid REPDIM %s %s %s on host %s. Disabling it.", words[1], words[2], words[3],
          user->host->hostname);
    user->enabled = 0;
    return PARSER_RC_ERROR;
}

PARSER_RC streaming_rep_end(char **words, void *user_v, PLUGINSD_ACTION *plugins_action) {
    PARSER_USER_OBJECT *user = user_v;
    UNUSED(plugins_action);
    char *num_points_txt = words[1];
    char *col_total_txt  = words[2];
    char *last_total_txt = words[3];
    if (!num_points_txt || !col_total_txt || !last_total_txt)
        goto disable;

    if (user->st == NULL) {
        error("Received RRDEND out of sequence");
        goto disable;
    }

    if (user->st->state->ignore_block) {
        user->st->state->ignore_block = 0;
        return PARSER_RC_OK;
    }

    size_t num_points = str2ull(num_points_txt);
    total_number col_total = str2ll(col_total_txt, NULL);
    total_number last_total = str2ll(last_total_txt, NULL);

    struct rrdset_volatile *state = user->st->state;
    user->st->last_updated.tv_sec = state->window_end - user->st->update_every;
    user->st->last_collected_time.tv_sec = user->st->last_updated.tv_sec;
    user->st->last_collected_time.tv_usec = USEC_PER_SEC/2;
    if (num_points > 0) {
        long advance = (state->window_end - state->window_first) / user->st->update_every;
        user->st->counter       += advance;
        user->st->counter_done  += advance;
        user->st->current_entry += advance;
        while (user->st->current_entry >= user->st->entries)        // Once except for an exceptional corner-case
            user->st->current_entry -= user->st->entries;
        user->st->collected_total = col_total;
        user->st->last_collected_total = last_total;
        debug(D_REPLICATION, "Finished replication %s: window %ld/%ld..%ld with %zu-pts transferred, advance=%ld-pts",
                             user->st->name, state->window_start, state->window_first, state->window_end, num_points,
                             advance);
    } else {
        debug(D_REPLICATION, "Finished replication on %s: window %ld/%ld-%ld empty, last_updated=%ld", user->st->name,
                             state->window_start, state->window_first, state->window_end,
                             user->st->last_updated.tv_sec);
    }
    state->window_start = 0;
    state->window_first = 0;
    state->window_end = 0;
    rrdset_dump_debug_state(user->st);
    user->st = NULL;
    return PARSER_RC_OK;
disable:
    errno = 0;
    error("Gap replication failed - Invalid REPEND %s on host %s. Disabling it.", words[1], user->host->hostname);
    user->enabled = 0;
    return PARSER_RC_ERROR;
}

#define CLAIMED_ID_MIN_WORDS 3
PARSER_RC streaming_claimed_id(char **words, void *user, PLUGINSD_ACTION *plugins_action)
{
    UNUSED(plugins_action);

    int i;
    uuid_t uuid;
    RRDHOST *host = ((PARSER_USER_OBJECT *)user)->host;

    for (i = 0; words[i]; i++) ;
    if (i != CLAIMED_ID_MIN_WORDS) {
        error("Command CLAIMED_ID came malformed %d parameters are expected but %d received", CLAIMED_ID_MIN_WORDS - 1, i - 1);
        return PARSER_RC_ERROR;
    }

    // We don't need the parsed UUID
    // just do it to check the format
    if(uuid_parse(words[1], uuid)) {
        error("1st parameter (host GUID) to CLAIMED_ID command is not valid GUID. Received: \"%s\".", words[1]);
        return PARSER_RC_ERROR;
    }
    if(uuid_parse(words[2], uuid) && strcmp(words[2], "NULL")) {
        error("2nd parameter (Claim ID) to CLAIMED_ID command is not valid GUID. Received: \"%s\".", words[2]);
        return PARSER_RC_ERROR;
    }

    if(strcmp(words[1], host->machine_guid)) {
        error("Claim ID is for host \"%s\" but it came over connection for \"%s\"", words[1], host->machine_guid);
        return PARSER_RC_OK; //the message is OK problem must be somewhere else
    }

    rrdhost_aclk_state_lock(host);
    if (host->aclk_state.claimed_id)
        freez(host->aclk_state.claimed_id);
    host->aclk_state.claimed_id = strcmp(words[2], "NULL") ? strdupz(words[2]) : NULL;

    store_claim_id(&host->host_uuid, host->aclk_state.claimed_id ? &uuid : NULL);

    rrdhost_aclk_state_unlock(host);

    rrdpush_claimed_id(host);

    return PARSER_RC_OK;
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
    memmove(r->read_buffer, &r->read_buffer[start], r->read_len - start);
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
    user->trust_durations = 0;

    PARSER *parser = parser_init(rpt->host, user, fp, PARSER_INPUT_SPLIT);
    parser_add_keyword(parser, "CLAIMED_ID", streaming_claimed_id);
    parser_add_keyword(parser, "REPBEGIN", streaming_rep_begin);
    parser_add_keyword(parser, "REPDIM", streaming_rep_dim);
    parser_add_keyword(parser, "REPEND", streaming_rep_end);

    if (unlikely(!parser)) {
        error("Failed to initialize parser");
        cd->serial_failures++;
        freez(user);
        return 0;
    }

    parser->plugins_action->begin_action     = &pluginsd_begin_action;
    parser->plugins_action->flush_action     = &pluginsd_flush_action;
    parser->plugins_action->end_action       = &pluginsd_end_action;
    parser->plugins_action->disable_action   = &pluginsd_disable_action;
    parser->plugins_action->variable_action  = &pluginsd_variable_action;
    parser->plugins_action->dimension_action = &pluginsd_dimension_action;
    parser->plugins_action->label_action     = &pluginsd_label_action;
    parser->plugins_action->overwrite_action = &pluginsd_overwrite_action;
    parser->plugins_action->chart_action     = &pluginsd_chart_action;
    parser->plugins_action->set_action       = &pluginsd_set_action;

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

#ifndef ENABLE_DBENGINE
    if (unlikely(mode == RRD_MEMORY_MODE_DBENGINE)) {
        close(rpt->fd);
        log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->machine_guid, rpt->hostname, "REJECTED -- DBENGINE MEMORY MODE NOT SUPPORTED");
        return 1;
    }
#endif

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

    rpt->use_replication = appconfig_get_number(&stream_config, rpt->key, "enable replication", rpt->use_replication);
    rpt->use_replication = appconfig_get_number(&stream_config, rpt->machine_guid, "enable replication", rpt->use_replication);

    if (rpt->stream_version == VERSION_GAP_FILLING && !rpt->use_replication)
        rpt->stream_version = VERSION_GAP_FILLING - 1;
    (void)appconfig_set_default(&stream_config, rpt->machine_guid, "host tags", (rpt->tags)?rpt->tags:"");

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
                , rpt->abbrev_timezone
                , rpt->utc_offset
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

//    rpt->host->connected_senders++;
    rpt->host->labels.labels_flag = (rpt->stream_version > 0)?LABEL_FLAG_UPDATE_STREAM:LABEL_FLAG_STOP_STREAM;

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

#if defined(ENABLE_ACLK)
    // in case we have cloud connection we inform cloud
    // new slave connected
    if (netdata_cloud_setting)
        aclk_host_state_update(rpt->host, 1);
#endif

    if (rpt->stream_version == VERSION_GAP_FILLING)
        receiver_tx_thread_spawn(rpt);
    size_t count = streaming_parser(rpt, &cd, fp);

    log_stream_connection(rpt->client_ip, rpt->client_port, rpt->key, rpt->host->machine_guid, rpt->hostname,
                          "DISCONNECTED");
    error("STREAM %s [receive from [%s]:%s]: disconnected (completed %zu updates).", rpt->hostname, rpt->client_ip,
          rpt->client_port, count);

#if defined(ENABLE_ACLK)
    // in case we have cloud connection we inform cloud
    // new slave connected
    if (netdata_cloud_setting)
        aclk_host_state_update(rpt->host, 0);
#endif

    // During a shutdown there is cleanup code in rrdhost that will cancel the sender thread
    if (!netdata_exit && rpt->host) {
        rrd_rdlock();
        rrdhost_wrlock(rpt->host);
        netdata_mutex_lock(&rpt->host->receiver_lock);
        if (rpt->host->receiver == rpt) {
            rpt->host->senders_disconnected_time = now_realtime_sec();
            rrdhost_flag_set(rpt->host, RRDHOST_FLAG_ORPHAN);
            if(health_enabled == CONFIG_BOOLEAN_AUTO)
                rpt->host->health_enabled = 0;
        }
        rrdhost_unlock(rpt->host);
        if (rpt->host->receiver == rpt) {
            rrdpush_sender_thread_stop(rpt->host);
        }
        netdata_mutex_unlock(&rpt->host->receiver_lock);
        rrd_unlock();
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

