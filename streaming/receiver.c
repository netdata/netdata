// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"

// IMPORTANT: to add workers, you have to edit WORKER_PARSER_FIRST_JOB accordingly
#define WORKER_RECEIVER_JOB_BYTES_READ (WORKER_PARSER_FIRST_JOB - 1)
#define WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED (WORKER_PARSER_FIRST_JOB - 2)

// this has to be the same at parser.h
#define WORKER_RECEIVER_JOB_REPLICATION_COMPLETION (WORKER_PARSER_FIRST_JOB - 3)

#if WORKER_PARSER_FIRST_JOB < 1
#error The define WORKER_PARSER_FIRST_JOB needs to be at least 1
#endif

extern struct config stream_config;

void receiver_state_free(struct receiver_state *rpt) {

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
    netdata_ssl_close(&rpt->ssl);
#endif

    if(rpt->fd != -1) {
        internal_error(true, "closing socket...");
        close(rpt->fd);
    }

#ifdef ENABLE_COMPRESSION
    if (rpt->decompressor)
        rpt->decompressor->destroy(&rpt->decompressor);
#endif

    if(rpt->system_info)
        rrdhost_system_info_free(rpt->system_info);

    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_receivers, sizeof(*rpt), __ATOMIC_RELAXED);

    freez(rpt);
}

#include "collectors/plugins.d/pluginsd_parser.h"

PARSER_RC streaming_claimed_id(char **words, size_t num_words, void *user)
{
    const char *host_uuid_str = get_word(words, num_words, 1);
    const char *claim_id_str = get_word(words, num_words, 2);

    if (!host_uuid_str || !claim_id_str) {
        error("Command CLAIMED_ID came malformed, uuid = '%s', claim_id = '%s'",
              host_uuid_str ? host_uuid_str : "[unset]",
              claim_id_str ? claim_id_str : "[unset]");
        return PARSER_RC_ERROR;
    }

    uuid_t uuid;
    RRDHOST *host = ((PARSER_USER_OBJECT *)user)->host;

    // We don't need the parsed UUID
    // just do it to check the format
    if(uuid_parse(host_uuid_str, uuid)) {
        error("1st parameter (host GUID) to CLAIMED_ID command is not valid GUID. Received: \"%s\".", host_uuid_str);
        return PARSER_RC_ERROR;
    }
    if(uuid_parse(claim_id_str, uuid) && strcmp(claim_id_str, "NULL")) {
        error("2nd parameter (Claim ID) to CLAIMED_ID command is not valid GUID. Received: \"%s\".", claim_id_str);
        return PARSER_RC_ERROR;
    }

    if(strcmp(host_uuid_str, host->machine_guid)) {
        error("Claim ID is for host \"%s\" but it came over connection for \"%s\"", host_uuid_str, host->machine_guid);
        return PARSER_RC_OK; //the message is OK problem must be somewhere else
    }

    rrdhost_aclk_state_lock(host);
    if (host->aclk_state.claimed_id)
        freez(host->aclk_state.claimed_id);
    host->aclk_state.claimed_id = strcmp(claim_id_str, "NULL") ? strdupz(claim_id_str) : NULL;
    rrdhost_aclk_state_unlock(host);

    rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_CLAIMID |RRDHOST_FLAG_METADATA_UPDATE);

    rrdpush_claimed_id(host);

    return PARSER_RC_OK;
}

static int read_stream(struct receiver_state *r, char* buffer, size_t size) {
    if(unlikely(!size)) {
        internal_error(true, "%s() asked to read zero bytes", __FUNCTION__);
        return 0;
    }

    ssize_t bytes_read;

#ifdef ENABLE_HTTPS
    if (SSL_connection(&r->ssl))
        bytes_read = netdata_ssl_read(&r->ssl, buffer, size);
    else
        bytes_read = read(r->fd, buffer, size);
#else
    bytes_read = read(r->fd, buffer, size);
#endif

    if((bytes_read == 0 || bytes_read == -1) && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINPROGRESS)) {
        error("STREAM: %s(): timeout while waiting for data on socket!", __FUNCTION__);
        bytes_read = -3;
    }
    else if (bytes_read == 0) {
        error("STREAM: %s(): EOF while reading data from socket!", __FUNCTION__);
        bytes_read = -1;
    }
    else if (bytes_read < 0) {
        error("STREAM: %s() failed to read from socket!", __FUNCTION__);
        bytes_read = -2;
    }

    return (int)bytes_read;
}

static bool receiver_read_uncompressed(struct receiver_state *r) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(r->read_buffer[r->read_len] != '\0')
        fatal("%s(): read_buffer does not start with zero", __FUNCTION__ );
#endif

    int bytes_read = read_stream(r, r->read_buffer + r->read_len, sizeof(r->read_buffer) - r->read_len - 1);
    if(unlikely(bytes_read <= 0))
        return false;

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes_read);
    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_read);

    r->read_len += bytes_read;
    r->read_buffer[r->read_len] = '\0';

    return true;
}

#ifdef ENABLE_COMPRESSION
static bool receiver_read_compressed(struct receiver_state *r) {

#ifdef NETDATA_INTERNAL_CHECKS
    if(r->read_buffer[r->read_len] != '\0')
        fatal("%s: read_buffer does not start with zero #2", __FUNCTION__ );
#endif

    // first use any available uncompressed data
    if (r->decompressor->decompressed_bytes_in_buffer(r->decompressor)) {
        size_t available = sizeof(r->read_buffer) - r->read_len - 1;
        if (available) {
            size_t len = r->decompressor->get(r->decompressor, r->read_buffer + r->read_len, available);
            if (!len) {
                internal_error(true, "decompressor returned zero length #1");
                return false;
            }

            r->read_len += (int)len;
            r->read_buffer[r->read_len] = '\0';
        }
        else
            internal_error(true, "The line to read is too big! Already have %d bytes in read_buffer.", r->read_len);

        return true;
    }

    // no decompressed data available
    // read the compression signature of the next block

    if(unlikely(r->read_len + r->decompressor->signature_size > sizeof(r->read_buffer) - 1)) {
        internal_error(true, "The last incomplete line does not leave enough room for the next compression header! Already have %d bytes in read_buffer.", r->read_len);
        return false;
    }

    // read the compression signature from the stream
    // we have to do a loop here, because read_stream() may return less than the data we need
    int bytes_read = 0;
    do {
        int ret = read_stream(r, r->read_buffer + r->read_len + bytes_read, r->decompressor->signature_size - bytes_read);
        if (unlikely(ret <= 0))
            return false;

        bytes_read += ret;
    } while(unlikely(bytes_read < (int)r->decompressor->signature_size));

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes_read);

    if(unlikely(bytes_read != (int)r->decompressor->signature_size))
        fatal("read %d bytes, but expected compression signature of size %zu", bytes_read, r->decompressor->signature_size);

    size_t compressed_message_size = r->decompressor->start(r->decompressor, r->read_buffer + r->read_len, bytes_read);
    if (unlikely(!compressed_message_size)) {
        internal_error(true, "multiplexed uncompressed data in compressed stream!");
        r->read_len += bytes_read;
        r->read_buffer[r->read_len] = '\0';
        return true;
    }

    if(unlikely(compressed_message_size > COMPRESSION_MAX_MSG_SIZE)) {
        error("received a compressed message of %zu bytes, which is bigger than the max compressed message size supported of %zu. Ignoring message.",
              compressed_message_size, (size_t)COMPRESSION_MAX_MSG_SIZE);
        return false;
    }

    // delete compression header from our read buffer
    r->read_buffer[r->read_len] = '\0';

    // Read the entire compressed block of compressed data
    char compressed[compressed_message_size];
    size_t compressed_bytes_read = 0;
    do {
        size_t start = compressed_bytes_read;
        size_t remaining = compressed_message_size - start;

        int last_read_bytes = read_stream(r, &compressed[start], remaining);
        if (unlikely(last_read_bytes <= 0)) {
            internal_error(true, "read_stream() failed #2, with code %d", last_read_bytes);
            return false;
        }

        compressed_bytes_read += last_read_bytes;

    } while(unlikely(compressed_message_size > compressed_bytes_read));

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)compressed_bytes_read);

    // decompress the compressed block
    size_t bytes_to_parse = r->decompressor->decompress(r->decompressor, compressed, compressed_bytes_read);
    if (!bytes_to_parse) {
        internal_error(true, "no bytes to parse.");
        return false;
    }

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_to_parse);

    // fill read buffer with decompressed data
    size_t len = (int)r->decompressor->get(r->decompressor, r->read_buffer + r->read_len, sizeof(r->read_buffer) - r->read_len - 1);
    if (!len) {
        internal_error(true, "decompressor returned zero length #2");
        return false;
    }
    r->read_len += (int)len;
    r->read_buffer[r->read_len] = '\0';

    return true;
}
#else // !ENABLE_COMPRESSION
static bool receiver_read_compressed(struct receiver_state *r) {
    return receiver_read_uncompressed(r);
}
#endif // ENABLE_COMPRESSION

/* Produce a full line if one exists, statefully return where we start next time.
 * When we hit the end of the buffer with a partial line move it to the beginning for the next fill.
 */
static char *receiver_next_line(struct receiver_state *r, char *buffer, size_t buffer_length, size_t *pos) {
    size_t start = *pos;

    char *ss = &r->read_buffer[start];
    char *se = &r->read_buffer[r->read_len];
    char *ds = buffer;
    char *de = &buffer[buffer_length - 2];

    if(ss >= se) {
        *ds = '\0';
        *pos = 0;
        r->read_len = 0;
        r->read_buffer[r->read_len] = '\0';
        return NULL;
    }

    // copy all bytes to buffer
    while(ss < se && ds < de && *ss != '\n')
        *ds++ = *ss++;

    // if we have a newline, return the buffer
    if(ss < se && ds < de && *ss == '\n') {
        // newline found in the r->read_buffer

        *ds++ = *ss++; // copy the newline too
        *ds = '\0';

        *pos = ss - r->read_buffer;
        return buffer;
    }

    // if the destination is full, oops!
    if(ds == de) {
        error("STREAM: received line exceeds %d bytes. Truncating it.", PLUGINSD_LINE_MAX);
        *ds = '\0';
        *pos = ss - r->read_buffer;
        return buffer;
    }

    // no newline found in the r->read_buffer
    // move everything to the beginning
    memmove(r->read_buffer, &r->read_buffer[start], r->read_len - start);
    r->read_len -= (int)start;
    r->read_buffer[r->read_len] = '\0';
    *ds = '\0';
    *pos = 0;
    return NULL;
}

bool plugin_is_enabled(struct plugind *cd);

static size_t streaming_parser(struct receiver_state *rpt, struct plugind *cd, int fd, void *ssl) {
    size_t result;

    PARSER_USER_OBJECT user = {
        .enabled = plugin_is_enabled(cd),
        .host = rpt->host,
        .opaque = rpt,
        .cd = cd,
        .trust_durations = 1,
        .capabilities = rpt->capabilities,
    };

    PARSER *parser = parser_init(&user, NULL, NULL, fd,
                                 PARSER_INPUT_SPLIT, ssl);

    pluginsd_keywords_init(parser, PARSER_INIT_STREAMING);

    rrd_collector_started();

    // this keeps the parser with its current value
    // so, parser needs to be allocated before pushing it
    netdata_thread_cleanup_push(pluginsd_process_thread_cleanup, parser);

    parser_add_keyword(parser, "CLAIMED_ID", streaming_claimed_id);

    user.parser = parser;

    bool compressed_connection = false;
#ifdef ENABLE_COMPRESSION
    if(stream_has_capability(rpt, STREAM_CAP_COMPRESSION)) {
        compressed_connection = true;

        if (!rpt->decompressor)
            rpt->decompressor = create_decompressor();
        else
            rpt->decompressor->reset(rpt->decompressor);
    }
#endif

    rpt->read_buffer[0] = '\0';
    rpt->read_len = 0;

    size_t read_buffer_start = 0;
    char buffer[PLUGINSD_LINE_MAX + 2] = "";
    while(service_running(SERVICE_STREAMING)) {
        netdata_thread_testcancel();

        if(!receiver_next_line(rpt, buffer, PLUGINSD_LINE_MAX + 2, &read_buffer_start)) {
            bool have_new_data;
            if(likely(compressed_connection))
                have_new_data = receiver_read_compressed(rpt);
            else
                have_new_data = receiver_read_uncompressed(rpt);

            if(unlikely(!have_new_data)) {
                if(!rpt->exit.reason)
                    rpt->exit.reason = "SOCKET READ ERROR";

                break;
            }

            rpt->last_msg_t = now_realtime_sec();
            continue;
        }

        if(unlikely(!service_running(SERVICE_STREAMING))) {
            if(!rpt->exit.reason)
                rpt->exit.reason = "NETDATA EXIT";
            goto done;
        }
        if(unlikely(rpt->exit.shutdown)) {
            if(!rpt->exit.reason)
                rpt->exit.reason = "SHUTDOWN REQUESTED";

            goto done;
        }

        if (unlikely(parser_action(parser,  buffer))) {
            internal_error(true, "parser_action() failed on keyword '%s'.", buffer);

            if(!rpt->exit.reason)
                rpt->exit.reason = "PARSER FAILED";

            break;
        }
    }

done:
    result = user.data_collections_count;

    // free parser with the pop function
    netdata_thread_cleanup_pop(1);

    return result;
}

static void rrdpush_receiver_replication_reset(RRDHOST *host) {
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        rrdset_flag_clear(st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS);
        rrdset_flag_set(st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);
    }
    rrdset_foreach_done(st);
    rrdhost_receiver_replicating_charts_zero(host);
}

void rrdhost_receiver_to_json(BUFFER *wb, RRDHOST *host, const char *key, time_t now __maybe_unused) {
    size_t receiver_hops = host->system_info ? host->system_info->hops : (host == localhost) ? 0 : 1;

    netdata_mutex_lock(&host->receiver_lock);

    buffer_json_member_add_object(wb, key);
    buffer_json_member_add_uint64(wb, "hops", receiver_hops);

    bool online = host == localhost || !rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN | RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED);
    buffer_json_member_add_boolean(wb, "online", online);

    if(host->child_connect_time || host->child_disconnected_time) {
        time_t since = MAX(host->child_connect_time, host->child_disconnected_time);
        buffer_json_member_add_time_t(wb, "since", since);
        buffer_json_member_add_time_t(wb, "age", now - since);
    }

    if(!online && host->rrdpush_last_receiver_exit_reason)
        buffer_json_member_add_string(wb, "reason", host->rrdpush_last_receiver_exit_reason);

    if(host != localhost && host->receiver) {
        buffer_json_member_add_object(wb, "replication");
        {
            size_t instances = rrdhost_receiver_replicating_charts(host);
            buffer_json_member_add_boolean(wb, "in_progress", instances);
            buffer_json_member_add_double(wb, "completion", host->rrdpush_receiver_replication_percent);
            buffer_json_member_add_uint64(wb, "instances", instances);
        }
        buffer_json_object_close(wb); // replication

        buffer_json_member_add_object(wb, "source");
        {

            char buf[1024 + 1];
            SOCKET_PEERS peers = socket_peers(host->receiver->fd);
            bool ssl = SSL_connection(&host->receiver->ssl);

            snprintfz(buf, 1024, "[%s]:%d%s", peers.local.ip, peers.local.port, ssl ? ":SSL" : "");
            buffer_json_member_add_string(wb, "local", buf);

            snprintfz(buf, 1024, "[%s]:%d%s", peers.peer.ip, peers.peer.port, ssl ? ":SSL" : "");
            buffer_json_member_add_string(wb, "remote", buf);

            stream_capabilities_to_json_array(wb, host->receiver->capabilities, "capabilities");
        }
        buffer_json_object_close(wb); // source
    }
    buffer_json_object_close(wb); // collection

    netdata_mutex_unlock(&host->receiver_lock);
}

static bool rrdhost_set_receiver(RRDHOST *host, struct receiver_state *rpt) {
    bool signal_rrdcontext = false;
    bool set_this = false;

    netdata_mutex_lock(&host->receiver_lock);

    if (!host->receiver || host->receiver == rpt) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);

        host->receiver = rpt;
        rpt->host = host;

        host->child_connect_time = now_realtime_sec();
        host->child_disconnected_time = 0;
        host->child_last_chart_command = 0;
        host->trigger_chart_obsoletion_check = 1;

        if (rpt->config.health_enabled != CONFIG_BOOLEAN_NO) {
            if (rpt->config.alarms_delay > 0) {
                host->health.health_delay_up_to = now_realtime_sec() + rpt->config.alarms_delay;
                log_health(
                        "[%s]: Postponing health checks for %" PRId64 " seconds, because it was just connected.",
                        rrdhost_hostname(host),
                        (int64_t) rpt->config.alarms_delay);
            }
        }

//         this is a test
//        if(rpt->hops <= host->sender->hops)
//            rrdpush_sender_thread_stop(host, "HOPS MISMATCH", false);

        signal_rrdcontext = true;
        rrdpush_receiver_replication_reset(host);

        rrdhost_flag_clear(rpt->host, RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED);
        aclk_queue_node_info(rpt->host, true);

        rrdpush_reset_destinations_postpone_time(host);

        set_this = true;
    }

    netdata_mutex_unlock(&host->receiver_lock);

    if(signal_rrdcontext)
        rrdcontext_host_child_connected(host);

    return set_this;
}

static void rrdhost_clear_receiver(struct receiver_state *rpt) {
    bool signal_rrdcontext = false;

    RRDHOST *host = rpt->host;
    if(host) {
        netdata_mutex_lock(&host->receiver_lock);

        // Make sure that we detach this thread and don't kill a freshly arriving receiver
        if(host->receiver == rpt) {
            host->trigger_chart_obsoletion_check = 0;
            host->child_connect_time = 0;
            host->child_disconnected_time = now_realtime_sec();

            if (rpt->config.health_enabled == CONFIG_BOOLEAN_AUTO)
                host->health.health_enabled = 0;

            rrdpush_sender_thread_stop(host, "RECEIVER LEFT", false);

            signal_rrdcontext = true;
            rrdpush_receiver_replication_reset(host);

            rrdhost_flag_set(host, RRDHOST_FLAG_ORPHAN);
            host->receiver = NULL;
            host->rrdpush_last_receiver_exit_reason = rpt->exit.reason;
        }

        netdata_mutex_unlock(&host->receiver_lock);

        if(signal_rrdcontext)
            rrdcontext_host_child_disconnected(host);

        rrdpush_reset_destinations_postpone_time(host);
    }
}

bool stop_streaming_receiver(RRDHOST *host, const char *reason) {
    bool ret = false;

    netdata_mutex_lock(&host->receiver_lock);

    if(host->receiver) {
        if(!host->receiver->exit.shutdown) {
            host->receiver->exit.shutdown = true;
            host->receiver->exit.reason = reason;
            shutdown(host->receiver->fd, SHUT_RDWR);
        }

        netdata_thread_cancel(host->receiver->thread);
    }

    int count = 2000;
    while (host->receiver && count-- > 0) {
        netdata_mutex_unlock(&host->receiver_lock);

        // let the lock for the receiver thread to exit
        sleep_usec(1 * USEC_PER_MS);

        netdata_mutex_lock(&host->receiver_lock);
    }

    if(host->receiver)
        error("STREAM '%s' [receive from [%s]:%s]: "
              "thread %d takes too long to stop, giving up..."
        , rrdhost_hostname(host)
        , host->receiver->client_ip, host->receiver->client_port
        , host->receiver->tid);
    else
        ret = true;

    netdata_mutex_unlock(&host->receiver_lock);

    return ret;
}

static void rrdpush_send_error_on_taken_over_connection(struct receiver_state *rpt, const char *msg) {
    (void) send_timeout(
#ifdef ENABLE_HTTPS
            &rpt->ssl,
#endif
            rpt->fd,
            (char *)msg,
            strlen(msg),
            0,
            5);
}

void rrdpush_receive_log_status(struct receiver_state *rpt, const char *msg, const char *status) {

    log_stream_connection(rpt->client_ip, rpt->client_port,
                          (rpt->key && *rpt->key)? rpt->key : "-",
                          (rpt->machine_guid && *rpt->machine_guid) ? rpt->machine_guid : "-",
                          (rpt->hostname && *rpt->hostname) ? rpt->hostname : "-",
                          status);

    info("STREAM '%s' [receive from [%s]:%s]: "
          "%s. "
          "STATUS: %s%s%s%s"
          , rpt->hostname
          , rpt->client_ip, rpt->client_port
          , msg
          , status
          , rpt->exit.reason?" (":""
          , rpt->exit.reason?rpt->exit.reason:""
          , rpt->exit.reason?")":""
    );

}

static void rrdhost_reset_destinations(RRDHOST *host) {
    for (struct rrdpush_destinations *d = host->destinations; d; d = d->next)
        d->postpone_reconnection_until = 0;
}

static void rrdpush_receive(struct receiver_state *rpt)
{
    rpt->config.mode = default_rrd_memory_mode;
    rpt->config.history = default_rrd_history_entries;

    rpt->config.health_enabled = (int)default_health_enabled;
    rpt->config.alarms_delay = 60;

    rpt->config.rrdpush_enabled = (int)default_rrdpush_enabled;
    rpt->config.rrdpush_destination = default_rrdpush_destination;
    rpt->config.rrdpush_api_key = default_rrdpush_api_key;
    rpt->config.rrdpush_send_charts_matching = default_rrdpush_send_charts_matching;

    rpt->config.rrdpush_enable_replication = default_rrdpush_enable_replication;
    rpt->config.rrdpush_seconds_to_replicate = default_rrdpush_seconds_to_replicate;
    rpt->config.rrdpush_replication_step = default_rrdpush_replication_step;

    rpt->config.update_every = (int)appconfig_get_number(&stream_config, rpt->machine_guid, "update every", rpt->config.update_every);
    if(rpt->config.update_every < 0) rpt->config.update_every = 1;

    rpt->config.history = (int)appconfig_get_number(&stream_config, rpt->key, "default history", rpt->config.history);
    rpt->config.history = (int)appconfig_get_number(&stream_config, rpt->machine_guid, "history", rpt->config.history);
    if(rpt->config.history < 5) rpt->config.history = 5;

    rpt->config.mode = rrd_memory_mode_id(appconfig_get(&stream_config, rpt->key, "default memory mode", rrd_memory_mode_name(rpt->config.mode)));
    rpt->config.mode = rrd_memory_mode_id(appconfig_get(&stream_config, rpt->machine_guid, "memory mode", rrd_memory_mode_name(rpt->config.mode)));

    if (unlikely(rpt->config.mode == RRD_MEMORY_MODE_DBENGINE && !dbengine_enabled)) {
        error("STREAM '%s' [receive from %s:%s]: "
              "dbengine is not enabled, falling back to default."
              , rpt->hostname
              , rpt->client_ip, rpt->client_port
              );

        rpt->config.mode = default_rrd_memory_mode;
    }

    rpt->config.health_enabled = appconfig_get_boolean_ondemand(&stream_config, rpt->key, "health enabled by default", rpt->config.health_enabled);
    rpt->config.health_enabled = appconfig_get_boolean_ondemand(&stream_config, rpt->machine_guid, "health enabled", rpt->config.health_enabled);

    rpt->config.alarms_delay = appconfig_get_number(&stream_config, rpt->key, "default postpone alarms on connect seconds", rpt->config.alarms_delay);
    rpt->config.alarms_delay = appconfig_get_number(&stream_config, rpt->machine_guid, "postpone alarms on connect seconds", rpt->config.alarms_delay);

    rpt->config.rrdpush_enabled = appconfig_get_boolean(&stream_config, rpt->key, "default proxy enabled", rpt->config.rrdpush_enabled);
    rpt->config.rrdpush_enabled = appconfig_get_boolean(&stream_config, rpt->machine_guid, "proxy enabled", rpt->config.rrdpush_enabled);

    rpt->config.rrdpush_destination = appconfig_get(&stream_config, rpt->key, "default proxy destination", rpt->config.rrdpush_destination);
    rpt->config.rrdpush_destination = appconfig_get(&stream_config, rpt->machine_guid, "proxy destination", rpt->config.rrdpush_destination);

    rpt->config.rrdpush_api_key = appconfig_get(&stream_config, rpt->key, "default proxy api key", rpt->config.rrdpush_api_key);
    rpt->config.rrdpush_api_key = appconfig_get(&stream_config, rpt->machine_guid, "proxy api key", rpt->config.rrdpush_api_key);

    rpt->config.rrdpush_send_charts_matching = appconfig_get(&stream_config, rpt->key, "default proxy send charts matching", rpt->config.rrdpush_send_charts_matching);
    rpt->config.rrdpush_send_charts_matching = appconfig_get(&stream_config, rpt->machine_guid, "proxy send charts matching", rpt->config.rrdpush_send_charts_matching);

    rpt->config.rrdpush_enable_replication = appconfig_get_boolean(&stream_config, rpt->key, "enable replication", rpt->config.rrdpush_enable_replication);
    rpt->config.rrdpush_enable_replication = appconfig_get_boolean(&stream_config, rpt->machine_guid, "enable replication", rpt->config.rrdpush_enable_replication);

    rpt->config.rrdpush_seconds_to_replicate = appconfig_get_number(&stream_config, rpt->key, "seconds to replicate", rpt->config.rrdpush_seconds_to_replicate);
    rpt->config.rrdpush_seconds_to_replicate = appconfig_get_number(&stream_config, rpt->machine_guid, "seconds to replicate", rpt->config.rrdpush_seconds_to_replicate);

    rpt->config.rrdpush_replication_step = appconfig_get_number(&stream_config, rpt->key, "seconds per replication step", rpt->config.rrdpush_replication_step);
    rpt->config.rrdpush_replication_step = appconfig_get_number(&stream_config, rpt->machine_guid, "seconds per replication step", rpt->config.rrdpush_replication_step);

#ifdef  ENABLE_COMPRESSION
    rpt->config.rrdpush_compression = default_compression_enabled;
    rpt->config.rrdpush_compression = appconfig_get_boolean(&stream_config, rpt->key, "enable compression", rpt->config.rrdpush_compression);
    rpt->config.rrdpush_compression = appconfig_get_boolean(&stream_config, rpt->machine_guid, "enable compression", rpt->config.rrdpush_compression);
    rpt->rrdpush_compression = (rpt->config.rrdpush_compression && default_compression_enabled);
#endif  //ENABLE_COMPRESSION

    (void)appconfig_set_default(&stream_config, rpt->machine_guid, "host tags", (rpt->tags)?rpt->tags:"");

    // find the host for this receiver
    {
        // this will also update the host with our system_info
        RRDHOST *host = rrdhost_find_or_create(
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
                , rpt->config.update_every
                , rpt->config.history
                , rpt->config.mode
                , (unsigned int)(rpt->config.health_enabled != CONFIG_BOOLEAN_NO)
                , (unsigned int)(rpt->config.rrdpush_enabled && rpt->config.rrdpush_destination && *rpt->config.rrdpush_destination && rpt->config.rrdpush_api_key && *rpt->config.rrdpush_api_key)
                , rpt->config.rrdpush_destination
                , rpt->config.rrdpush_api_key
                , rpt->config.rrdpush_send_charts_matching
                , rpt->config.rrdpush_enable_replication
                , rpt->config.rrdpush_seconds_to_replicate
                , rpt->config.rrdpush_replication_step
                , rpt->system_info
                , 0
        );

        if(!host) {
            rrdpush_receive_log_status(rpt, "failed to find/create host structure", "INTERNAL ERROR DROPPING CONNECTION");
            rrdpush_send_error_on_taken_over_connection(rpt, START_STREAMING_ERROR_INTERNAL_ERROR);
            goto cleanup;
        }

        if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))) {
            rrdpush_receive_log_status(rpt, "host is initializing", "INITIALIZATION IN PROGRESS RETRY LATER");
            rrdpush_send_error_on_taken_over_connection(rpt, START_STREAMING_ERROR_INITIALIZATION);
            goto cleanup;
        }

        // system_info has been consumed by the host structure
        rpt->system_info = NULL;

        if(!rrdhost_set_receiver(host, rpt)) {
            rrdpush_receive_log_status(rpt, "host is already served by another receiver", "DUPLICATE RECEIVER DROPPING CONNECTION");
            rrdpush_send_error_on_taken_over_connection(rpt, START_STREAMING_ERROR_ALREADY_STREAMING);
            goto cleanup;
        }
    }

#ifdef NETDATA_INTERNAL_CHECKS
    info("STREAM '%s' [receive from [%s]:%s]: "
         "client willing to stream metrics for host '%s' with machine_guid '%s': "
         "update every = %d, history = %ld, memory mode = %s, health %s,%s tags '%s'"
         , rpt->hostname
         , rpt->client_ip
         , rpt->client_port
         , rrdhost_hostname(rpt->host)
         , rpt->host->machine_guid
         , rpt->host->rrd_update_every
         , rpt->host->rrd_history_entries
         , rrd_memory_mode_name(rpt->host->rrd_memory_mode)
         , (rpt->config.health_enabled == CONFIG_BOOLEAN_NO)?"disabled":((rpt->config.health_enabled == CONFIG_BOOLEAN_YES)?"enabled":"auto")
#ifdef ENABLE_HTTPS
         , (rpt->ssl.conn != NULL) ? " SSL," : ""
#else
         , ""
#endif
         , rrdhost_tags(rpt->host)
    );
#endif // NETDATA_INTERNAL_CHECKS


    struct plugind cd = {
            .update_every = default_rrd_update_every,
            .unsafe = {
                    .spinlock = NETDATA_SPINLOCK_INITIALIZER,
                    .running = true,
                    .enabled = true,
            },
            .started_t = now_realtime_sec(),
    };

    // put the client IP and port into the buffers used by plugins.d
    snprintfz(cd.id,           CONFIG_MAX_NAME,  "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(cd.filename,     FILENAME_MAX,     "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(cd.fullfilename, FILENAME_MAX,     "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(cd.cmd,          PLUGINSD_CMD_MAX, "%s:%s", rpt->client_ip, rpt->client_port);

#ifdef ENABLE_COMPRESSION
    if (stream_has_capability(rpt, STREAM_CAP_COMPRESSION)) {
        if (!rpt->rrdpush_compression)
            rpt->capabilities &= ~STREAM_CAP_COMPRESSION;
    }
#endif

    {
        // info("STREAM %s [receive from [%s]:%s]: initializing communication...", rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port);
        char initial_response[HTTP_HEADER_SIZE];
        if (stream_has_capability(rpt, STREAM_CAP_VCAPS)) {
            log_receiver_capabilities(rpt);
            sprintf(initial_response, "%s%u", START_STREAMING_PROMPT_VN, rpt->capabilities);
        }
        else if (stream_has_capability(rpt, STREAM_CAP_VN)) {
            log_receiver_capabilities(rpt);
            sprintf(initial_response, "%s%d", START_STREAMING_PROMPT_VN, stream_capabilities_to_vn(rpt->capabilities));
        }
        else if (stream_has_capability(rpt, STREAM_CAP_V2)) {
            log_receiver_capabilities(rpt);
            sprintf(initial_response, "%s", START_STREAMING_PROMPT_V2);
        }
        else { // stream_has_capability(rpt, STREAM_CAP_V1)
            log_receiver_capabilities(rpt);
            sprintf(initial_response, "%s", START_STREAMING_PROMPT_V1);
        }

        debug(D_STREAM, "Initial response to %s: %s", rpt->client_ip, initial_response);
        ssize_t bytes_sent = send_timeout(
#ifdef ENABLE_HTTPS
                &rpt->ssl,
#endif
                rpt->fd, initial_response, strlen(initial_response), 0, 60);

        if(bytes_sent != (ssize_t)strlen(initial_response)) {
            internal_error(true, "Cannot send response, got %zd bytes, expecting %zu bytes", bytes_sent, strlen(initial_response));
            rrdpush_receive_log_status(rpt, "cannot reply back", "CANT REPLY DROPPING CONNECTION");
            goto cleanup;
        }
    }

    {
        // remove the non-blocking flag from the socket
        if(sock_delnonblock(rpt->fd) < 0)
            error("STREAM '%s' [receive from [%s]:%s]: "
                  "cannot remove the non-blocking flag from socket %d"
                  , rrdhost_hostname(rpt->host)
                  , rpt->client_ip, rpt->client_port
                  , rpt->fd);

        struct timeval timeout;
        timeout.tv_sec = 600;
        timeout.tv_usec = 0;
        if (unlikely(setsockopt(rpt->fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof timeout) != 0))
            error("STREAM '%s' [receive from [%s]:%s]: "
                  "cannot set timeout for socket %d"
                  , rrdhost_hostname(rpt->host)
                  , rpt->client_ip, rpt->client_port
                  , rpt->fd);
    }

    rrdpush_receive_log_status(rpt, "ready to receive data", "CONNECTED");

#ifdef ENABLE_ACLK
    // in case we have cloud connection we inform cloud
    // new child connected
    if (netdata_cloud_setting)
        aclk_host_state_update(rpt->host, 1);
#endif

    rrdhost_set_is_parent_label(++localhost->connected_children_count);

    // let it reconnect to parent immediately
    rrdhost_reset_destinations(rpt->host);

    size_t count = streaming_parser(rpt, &cd, rpt->fd,
#ifdef ENABLE_HTTPS
                                    (rpt->ssl.conn) ? &rpt->ssl : NULL
#else
                                    NULL
#endif
                                    );

    rrdhost_flag_set(rpt->host, RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED);

    if(!rpt->exit.reason)
        rpt->exit.reason = "PARSER EXIT";

    {
        char msg[100 + 1];
        snprintfz(msg, 100, "disconnected (completed %zu updates)", count);
        rrdpush_receive_log_status(rpt, msg, "DISCONNECTED");
    }

#ifdef ENABLE_ACLK
    // in case we have cloud connection we inform cloud
    // a child disconnected
    if (netdata_cloud_setting)
        aclk_host_state_update(rpt->host, 0);
#endif

    rrdhost_set_is_parent_label(--localhost->connected_children_count);

cleanup:
    ;
}

static void rrdpush_receiver_thread_cleanup(void *ptr) {
    struct receiver_state *rpt = (struct receiver_state *) ptr;
    worker_unregister();

    rrdhost_clear_receiver(rpt);

    info("STREAM '%s' [receive from [%s]:%s]: "
         "receive thread ended (task id %d)"
    , rpt->hostname ? rpt->hostname : "-"
    , rpt->client_ip ? rpt->client_ip : "-", rpt->client_port ? rpt->client_port : "-"
    , gettid());

    receiver_state_free(rpt);
}

void *rrdpush_receiver_thread(void *ptr) {
    netdata_thread_cleanup_push(rrdpush_receiver_thread_cleanup, ptr);

    worker_register("STREAMRCV");
    worker_register_job_custom_metric(WORKER_RECEIVER_JOB_BYTES_READ, "received bytes", "bytes/s", WORKER_METRIC_INCREMENT);
    worker_register_job_custom_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, "uncompressed bytes", "bytes/s", WORKER_METRIC_INCREMENT);
    worker_register_job_custom_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION, "replication completion", "%", WORKER_METRIC_ABSOLUTE);

    struct receiver_state *rpt = (struct receiver_state *)ptr;
    rpt->tid = gettid();
    info("STREAM %s [%s]:%s: receive thread created (task id %d)", rpt->hostname, rpt->client_ip, rpt->client_port, rpt->tid);

    rrdpush_receive(rpt);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
