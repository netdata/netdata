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
        // If the shutdown sequence has started, and this receiver is still attached to the host then we cannot touch
        // the host pointer as it is unpredictable when the RRDHOST is deleted. Do the cleanup from rrdhost_free().
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

#include "collectors/plugins.d/pluginsd_parser.h"

PARSER_RC streaming_timestamp(char **words, void *user, PLUGINSD_ACTION *plugins_action)
{
    UNUSED(plugins_action);
    char *remote_time_txt = words[1];
    time_t remote_time = 0;
    RRDHOST *host = ((PARSER_USER_OBJECT *)user)->host;
    struct plugind *cd = ((PARSER_USER_OBJECT *)user)->cd;
    if (cd->version < VERSION_GAP_FILLING ) {
        error("STREAM %s from %s: Child negotiated version %u but sent TIMESTAMP!", host->hostname, cd->cmd,
               cd->version);
        return PARSER_RC_OK;    // Ignore error and continue stream
    }
    if (remote_time_txt && *remote_time_txt) {
        remote_time = str2ull(remote_time_txt);
        time_t now = now_realtime_sec(), prev = rrdhost_last_entry_t(host);
        time_t gap = 0;
        if (prev == 0)
            info("STREAM %s from %s: Initial connection (no gap to check), remote=%ld local=%ld slew=%ld",
                 host->hostname, cd->cmd, remote_time, now, now-remote_time);
        else {
            gap = now - prev;
            info("STREAM %s from %s: Checking for gaps... remote=%ld local=%ld..%ld slew=%ld  %ld-sec gap",
                 host->hostname, cd->cmd, remote_time, prev, now, remote_time - now, gap);
        }
        char message[128];
        sprintf(message,"REPLICATE %ld %ld\n", remote_time - gap, remote_time);
        int ret;
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
            error("Failed to send initial timestamp - gaps may appear in charts");
        return PARSER_RC_OK;
    }
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
    parser_add_keyword(parser, "TIMESTAMP", streaming_timestamp);
    parser_add_keyword(parser, "CLAIMED_ID", streaming_claimed_id);

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
    parser->plugins_action->clabel_commit_action  = &pluginsd_clabel_commit_action;
    parser->plugins_action->clabel_action    = &pluginsd_clabel_action;

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

