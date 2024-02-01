// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"
#include "web/server/h2o/http_server.h"

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

    rrdpush_decompressor_destroy(&rpt->decompressor);

    if(rpt->system_info)
         rrdhost_system_info_free(rpt->system_info);

    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_receivers, sizeof(*rpt), __ATOMIC_RELAXED);

    freez(rpt);
}

#include "collectors/plugins.d/pluginsd_parser.h"

// IMPORTANT: to add workers, you have to edit WORKER_PARSER_FIRST_JOB accordingly
#define WORKER_RECEIVER_JOB_BYTES_READ (WORKER_PARSER_FIRST_JOB - 1)
#define WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED (WORKER_PARSER_FIRST_JOB - 2)

// this has to be the same at parser.h
#define WORKER_RECEIVER_JOB_REPLICATION_COMPLETION (WORKER_PARSER_FIRST_JOB - 3)

#if WORKER_PARSER_FIRST_JOB < 1
#error The define WORKER_PARSER_FIRST_JOB needs to be at least 1
#endif

static inline int read_stream(struct receiver_state *r, char* buffer, size_t size) {
    if(unlikely(!size)) {
        internal_error(true, "%s() asked to read zero bytes", __FUNCTION__);
        return 0;
    }

#ifdef ENABLE_H2O
    if (is_h2o_rrdpush(r))
        return (int)h2o_stream_read(r->h2o_ctx, buffer, size);
#endif

    int tries = 100;
    ssize_t bytes_read;

    do {
        errno = 0;

#ifdef ENABLE_HTTPS
        if (SSL_connection(&r->ssl))
            bytes_read = netdata_ssl_read(&r->ssl, buffer, size);
        else
            bytes_read = read(r->fd, buffer, size);
#else
        bytes_read = read(r->fd, buffer, size);
#endif

    } while(bytes_read < 0 && errno == EINTR && tries--);

    if((bytes_read == 0 || bytes_read == -1) && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINPROGRESS)) {
        netdata_log_error("STREAM: %s(): timeout while waiting for data on socket!", __FUNCTION__);
        bytes_read = -3;
    }
    else if (bytes_read == 0) {
        netdata_log_error("STREAM: %s(): EOF while reading data from socket!", __FUNCTION__);
        bytes_read = -1;
    }
    else if (bytes_read < 0) {
        netdata_log_error("STREAM: %s() failed to read from socket!", __FUNCTION__);
        bytes_read = -2;
    }

    return (int)bytes_read;
}

static inline STREAM_HANDSHAKE read_stream_error_to_reason(int code) {
    if(code > 0)
        return 0;

    switch(code) {
        case 0:
            // asked to read zero bytes
            return STREAM_HANDSHAKE_DISCONNECT_NOT_SUFFICIENT_READ_BUFFER;

        case -1:
            // EOF
            return STREAM_HANDSHAKE_DISCONNECT_SOCKET_EOF;

        case -2:
            // failed to read
            return STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_FAILED;

        case -3:
            // timeout
            return STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_TIMEOUT;

        default:
            // anything else
            return STREAM_HANDSHAKE_DISCONNECT_UNKNOWN_SOCKET_READ_ERROR;
    }
}

static inline bool receiver_read_uncompressed(struct receiver_state *r, STREAM_HANDSHAKE *reason) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(r->reader.read_buffer[r->reader.read_len] != '\0')
        fatal("%s(): read_buffer does not start with zero", __FUNCTION__ );
#endif

    int bytes_read = read_stream(r, r->reader.read_buffer + r->reader.read_len, sizeof(r->reader.read_buffer) - r->reader.read_len - 1);
    if(unlikely(bytes_read <= 0)) {
        *reason = read_stream_error_to_reason(bytes_read);
        return false;
    }

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes_read);
    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_read);

    r->reader.read_len += bytes_read;
    r->reader.read_buffer[r->reader.read_len] = '\0';

    return true;
}

static inline bool receiver_read_compressed(struct receiver_state *r, STREAM_HANDSHAKE *reason) {

    internal_fatal(r->reader.read_buffer[r->reader.read_len] != '\0',
                   "%s: read_buffer does not start with zero #2", __FUNCTION__ );

    // first use any available uncompressed data
    if (likely(rrdpush_decompressed_bytes_in_buffer(&r->decompressor))) {
        size_t available = sizeof(r->reader.read_buffer) - r->reader.read_len - 1;
        if (likely(available)) {
            size_t len = rrdpush_decompressor_get(&r->decompressor, r->reader.read_buffer + r->reader.read_len, available);
            if (unlikely(!len)) {
                internal_error(true, "decompressor returned zero length #1");
                return false;
            }

            r->reader.read_len += (int)len;
            r->reader.read_buffer[r->reader.read_len] = '\0';
        }
        else
            internal_fatal(true, "The line to read is too big! Already have %zd bytes in read_buffer.", r->reader.read_len);

        return true;
    }

    // no decompressed data available
    // read the compression signature of the next block

    if(unlikely(r->reader.read_len + r->decompressor.signature_size > sizeof(r->reader.read_buffer) - 1)) {
        internal_error(true, "The last incomplete line does not leave enough room for the next compression header! "
                             "Already have %zd bytes in read_buffer.", r->reader.read_len);
        return false;
    }

    // read the compression signature from the stream
    // we have to do a loop here, because read_stream() may return less than the data we need
    int bytes_read = 0;
    do {
        int ret = read_stream(r, r->reader.read_buffer + r->reader.read_len + bytes_read, r->decompressor.signature_size - bytes_read);
        if (unlikely(ret <= 0)) {
            *reason = read_stream_error_to_reason(ret);
            return false;
        }

        bytes_read += ret;
    } while(unlikely(bytes_read < (int)r->decompressor.signature_size));

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes_read);

    if(unlikely(bytes_read != (int)r->decompressor.signature_size))
        fatal("read %d bytes, but expected compression signature of size %zu", bytes_read, r->decompressor.signature_size);

    size_t compressed_message_size = rrdpush_decompressor_start(&r->decompressor, r->reader.read_buffer + r->reader.read_len, bytes_read);
    if (unlikely(!compressed_message_size)) {
        internal_error(true, "multiplexed uncompressed data in compressed stream!");
        r->reader.read_len += bytes_read;
        r->reader.read_buffer[r->reader.read_len] = '\0';
        return true;
    }

    if(unlikely(compressed_message_size > COMPRESSION_MAX_MSG_SIZE)) {
        netdata_log_error("received a compressed message of %zu bytes, which is bigger than the max compressed message size supported of %zu. Ignoring message.",
              compressed_message_size, (size_t)COMPRESSION_MAX_MSG_SIZE);
        return false;
    }

    // delete compression header from our read buffer
    r->reader.read_buffer[r->reader.read_len] = '\0';

    // Read the entire compressed block of compressed data
    char compressed[compressed_message_size];
    size_t compressed_bytes_read = 0;
    do {
        size_t start = compressed_bytes_read;
        size_t remaining = compressed_message_size - start;

        int last_read_bytes = read_stream(r, &compressed[start], remaining);
        if (unlikely(last_read_bytes <= 0)) {
            *reason = read_stream_error_to_reason(last_read_bytes);
            return false;
        }

        compressed_bytes_read += last_read_bytes;

    } while(unlikely(compressed_message_size > compressed_bytes_read));

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)compressed_bytes_read);

    // decompress the compressed block
    size_t bytes_to_parse = rrdpush_decompress(&r->decompressor, compressed, compressed_bytes_read);
    if (unlikely(!bytes_to_parse)) {
        internal_error(true, "no bytes to parse.");
        return false;
    }

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_to_parse);

    // fill read buffer with decompressed data
    size_t len = (int) rrdpush_decompressor_get(&r->decompressor, r->reader.read_buffer + r->reader.read_len, sizeof(r->reader.read_buffer) - r->reader.read_len - 1);
    if (unlikely(!len)) {
        internal_error(true, "decompressor returned zero length #2");
        return false;
    }
    r->reader.read_len += (int)len;
    r->reader.read_buffer[r->reader.read_len] = '\0';

    return true;
}

bool plugin_is_enabled(struct plugind *cd);

static void receiver_set_exit_reason(struct receiver_state *rpt, STREAM_HANDSHAKE reason, bool force) {
    if(force || !rpt->exit.reason)
        rpt->exit.reason = reason;
}

static inline bool receiver_should_stop(struct receiver_state *rpt) {
    static __thread size_t counter = 0;

    if(unlikely(rpt->exit.shutdown)) {
        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, false);
        return true;
    }

    if(unlikely(!service_running(SERVICE_STREAMING))) {
        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_NETDATA_EXIT, false);
        return true;
    }

    if(unlikely((counter++ % 1000) == 0)) {
        // check every 1000 lines read
        netdata_thread_testcancel();
        rpt->last_msg_t = now_monotonic_sec();
    }

    return false;
}

static size_t streaming_parser(struct receiver_state *rpt, struct plugind *cd, int fd, void *ssl) {
    size_t result = 0;

    PARSER *parser = NULL;
    {
        PARSER_USER_OBJECT user = {
                .enabled = plugin_is_enabled(cd),
                .host = rpt->host,
                .opaque = rpt,
                .cd = cd,
                .trust_durations = 1,
                .capabilities = rpt->capabilities,
        };

        parser = parser_init(&user, NULL, NULL, fd, PARSER_INPUT_SPLIT, ssl);
    }

#ifdef ENABLE_H2O
    parser->h2o_ctx = rpt->h2o_ctx;
#endif

    pluginsd_keywords_init(parser, PARSER_INIT_STREAMING);

    rrd_collector_started();

    // this keeps the parser with its current value
    // so, parser needs to be allocated before pushing it
    netdata_thread_cleanup_push(pluginsd_process_thread_cleanup, parser) {
        bool compressed_connection = rrdpush_decompression_initialize(rpt);
        buffered_reader_init(&rpt->reader);

#ifdef NETDATA_LOG_STREAM_RECEIVE
        {
            char filename[FILENAME_MAX + 1];
            snprintfz(filename, FILENAME_MAX, "/tmp/stream-receiver-%s.txt", rpt->host ? rrdhost_hostname(
                            rpt->host) : "unknown"
                     );
            parser->user.stream_log_fp = fopen(filename, "w");
            parser->user.stream_log_repertoire = PARSER_REP_METADATA;
        }
#endif

        CLEAN_BUFFER *buffer = buffer_create(sizeof(rpt->reader.read_buffer), NULL);

        ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_CB(NDF_REQUEST, line_splitter_reconstruct_line, &parser->line),
                ND_LOG_FIELD_CB(NDF_NIDL_NODE, parser_reconstruct_node, parser),
                ND_LOG_FIELD_CB(NDF_NIDL_INSTANCE, parser_reconstruct_instance, parser),
                ND_LOG_FIELD_CB(NDF_NIDL_CONTEXT, parser_reconstruct_context, parser),
                ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        while(!receiver_should_stop(rpt)) {

            if(!buffered_reader_next_line(&rpt->reader, buffer)) {
                STREAM_HANDSHAKE reason = STREAM_HANDSHAKE_DISCONNECT_UNKNOWN_SOCKET_READ_ERROR;

                bool have_new_data = compressed_connection ? receiver_read_compressed(rpt, &reason)
                                                           : receiver_read_uncompressed(rpt, &reason);

                if(unlikely(!have_new_data)) {
                    receiver_set_exit_reason(rpt, reason, false);
                    break;
                }

                continue;
            }

            if(unlikely(parser_action(parser, buffer->buffer))) {
                receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                break;
            }

            buffer->len = 0;
            buffer->buffer[0] = '\0';
        }
        result = parser->user.data_collections_count;
    }
    netdata_thread_cleanup_pop(1); // free parser with the pop function

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

static bool rrdhost_set_receiver(RRDHOST *host, struct receiver_state *rpt) {
    bool signal_rrdcontext = false;
    bool set_this = false;

    netdata_mutex_lock(&host->receiver_lock);

    if (!host->receiver) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);

        host->rrdpush_receiver_connection_counter++;
        __atomic_add_fetch(&localhost->connected_children_count, 1, __ATOMIC_RELAXED);

        host->receiver = rpt;
        rpt->host = host;

        host->child_connect_time = now_realtime_sec();
        host->child_disconnected_time = 0;
        host->child_last_chart_command = 0;
        host->trigger_chart_obsoletion_check = 1;

        if (rpt->config.health_enabled != CONFIG_BOOLEAN_NO) {
            if (rpt->config.alarms_delay > 0) {
                host->health.health_delay_up_to = now_realtime_sec() + rpt->config.alarms_delay;
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "[%s]: Postponing health checks for %" PRId64 " seconds, because it was just connected.",
                       rrdhost_hostname(host),
                       (int64_t) rpt->config.alarms_delay);
            }
        }

        host->health_log.health_log_history = rpt->config.alarms_history;

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
            __atomic_sub_fetch(&localhost->connected_children_count, 1, __ATOMIC_RELAXED);
            rrdhost_flag_set(rpt->host, RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED);

            host->trigger_chart_obsoletion_check = 0;
            host->child_connect_time = 0;
            host->child_disconnected_time = now_realtime_sec();

            if (rpt->config.health_enabled == CONFIG_BOOLEAN_AUTO)
                host->health.health_enabled = 0;

            rrdpush_sender_thread_stop(host, STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT, false);

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

bool stop_streaming_receiver(RRDHOST *host, STREAM_HANDSHAKE reason) {
    bool ret = false;

    netdata_mutex_lock(&host->receiver_lock);

    if(host->receiver) {
        if(!host->receiver->exit.shutdown) {
            host->receiver->exit.shutdown = true;
            receiver_set_exit_reason(host->receiver, reason, true);
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
        netdata_log_error("STREAM '%s' [receive from [%s]:%s]: "
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

void rrdpush_receive_log_status(struct receiver_state *rpt, const char *msg, const char *status, ND_LOG_FIELD_PRIORITY priority) {
    // this function may be called BEFORE we spawn the receiver thread
    // so, we need to add the fields again (it does not harm)
    ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->client_ip),
            ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->client_port),
            ND_LOG_FIELD_TXT(NDF_NIDL_NODE, (rpt->hostname && *rpt->hostname) ? rpt->hostname : ""),
            ND_LOG_FIELD_TXT(NDF_RESPONSE_CODE, status),
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_from_child_msgid),
            ND_LOG_FIELD_END(),
    };
    ND_LOG_STACK_PUSH(lgs);

    nd_log(NDLS_ACCESS, priority, "api_key:'%s' machine_guid:'%s' msg:'%s'"
                       , (rpt->key && *rpt->key)? rpt->key : ""
                       , (rpt->machine_guid && *rpt->machine_guid) ? rpt->machine_guid : ""
                       , msg);

    nd_log(NDLS_DAEMON, priority, "STREAM_RECEIVER for '%s': %s %s%s%s"
                     , (rpt->hostname && *rpt->hostname) ? rpt->hostname : ""
                     , msg
                     , rpt->exit.reason != STREAM_HANDSHAKE_NEVER?" (":""
                     , stream_handshake_error_to_string(rpt->exit.reason)
                     , rpt->exit.reason != STREAM_HANDSHAKE_NEVER?")":""
                     );
}

static void rrdpush_receive(struct receiver_state *rpt)
{
    rpt->config.mode = default_rrd_memory_mode;
    rpt->config.history = default_rrd_history_entries;

    rpt->config.health_enabled = health_plugin_enabled();
    rpt->config.alarms_delay = 60;
    rpt->config.alarms_history = HEALTH_LOG_DEFAULT_HISTORY;

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
        netdata_log_error("STREAM '%s' [receive from %s:%s]: "
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

    rpt->config.alarms_history = appconfig_get_number(&stream_config, rpt->key, "default health log history", rpt->config.alarms_history);
    rpt->config.alarms_history = appconfig_get_number(&stream_config, rpt->machine_guid, "health log history", rpt->config.alarms_history);

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

    rpt->config.rrdpush_compression = default_rrdpush_compression_enabled;
    rpt->config.rrdpush_compression = appconfig_get_boolean(&stream_config, rpt->key, "enable compression", rpt->config.rrdpush_compression);
    rpt->config.rrdpush_compression = appconfig_get_boolean(&stream_config, rpt->machine_guid, "enable compression", rpt->config.rrdpush_compression);

    bool is_ephemeral = false;
    is_ephemeral = appconfig_get_boolean(&stream_config, rpt->key, "is ephemeral node", CONFIG_BOOLEAN_NO);
    is_ephemeral = appconfig_get_boolean(&stream_config, rpt->machine_guid, "is ephemeral node", is_ephemeral);

    if(rpt->config.rrdpush_compression) {
        char *order = appconfig_get(&stream_config, rpt->key, "compression algorithms order", RRDPUSH_COMPRESSION_ALGORITHMS_ORDER);
        order = appconfig_get(&stream_config, rpt->machine_guid, "compression algorithms order", order);
        rrdpush_parse_compression_order(rpt, order);
    }

    (void)appconfig_set_default(&stream_config, rpt->machine_guid, "host tags", (rpt->tags)?rpt->tags:"");

    // find the host for this receiver
    {
        // this will also update the host with our system_info
        RRDHOST *host = rrdhost_find_or_create(
            rpt->hostname,
            rpt->registry_hostname,
            rpt->machine_guid,
            rpt->os,
            rpt->timezone,
            rpt->abbrev_timezone,
            rpt->utc_offset,
            rpt->tags,
            rpt->program_name,
            rpt->program_version,
            rpt->config.update_every,
            rpt->config.history,
            rpt->config.mode,
            (unsigned int)(rpt->config.health_enabled != CONFIG_BOOLEAN_NO),
            (unsigned int)(rpt->config.rrdpush_enabled && rpt->config.rrdpush_destination &&
                           *rpt->config.rrdpush_destination && rpt->config.rrdpush_api_key &&
                           *rpt->config.rrdpush_api_key),
            rpt->config.rrdpush_destination,
            rpt->config.rrdpush_api_key,
            rpt->config.rrdpush_send_charts_matching,
            rpt->config.rrdpush_enable_replication,
            rpt->config.rrdpush_seconds_to_replicate,
            rpt->config.rrdpush_replication_step,
            rpt->system_info,
            0);

        if(!host) {
            rrdpush_receive_log_status(
                    rpt,"failed to find/create host structure, rejecting connection",
                    RRDPUSH_STATUS_INTERNAL_SERVER_ERROR, NDLP_ERR);

            rrdpush_send_error_on_taken_over_connection(rpt, START_STREAMING_ERROR_INTERNAL_ERROR);
            goto cleanup;
        }

        if (unlikely(rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))) {
            rrdpush_receive_log_status(
                    rpt, "host is initializing, retry later",
                    RRDPUSH_STATUS_INITIALIZATION_IN_PROGRESS, NDLP_NOTICE);

            rrdpush_send_error_on_taken_over_connection(rpt, START_STREAMING_ERROR_INITIALIZATION);
            goto cleanup;
        }

        // system_info has been consumed by the host structure
        rpt->system_info = NULL;

        if(!rrdhost_set_receiver(host, rpt)) {
            rrdpush_receive_log_status(
                    rpt, "host is already served by another receiver",
                    RRDPUSH_STATUS_DUPLICATE_RECEIVER, NDLP_INFO);

            rrdpush_send_error_on_taken_over_connection(rpt, START_STREAMING_ERROR_ALREADY_STREAMING);
            goto cleanup;
        }
    }

#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_info("STREAM '%s' [receive from [%s]:%s]: "
         "client willing to stream metrics for host '%s' with machine_guid '%s': "
         "update every = %d, history = %d, memory mode = %s, health %s,%s tags '%s'"
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

    rrdpush_select_receiver_compression_algorithm(rpt);

    {
        // netdata_log_info("STREAM %s [receive from [%s]:%s]: initializing communication...", rrdhost_hostname(rpt->host), rpt->client_ip, rpt->client_port);
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

        netdata_log_debug(D_STREAM, "Initial response to %s: %s", rpt->client_ip, initial_response);
#ifdef ENABLE_H2O
        if (is_h2o_rrdpush(rpt)) {
            h2o_stream_write(rpt->h2o_ctx, initial_response, strlen(initial_response));
        } else {
#endif
            ssize_t bytes_sent = send_timeout(
#ifdef ENABLE_HTTPS
                    &rpt->ssl,
#endif
                    rpt->fd, initial_response, strlen(initial_response), 0, 60);

            if(bytes_sent != (ssize_t)strlen(initial_response)) {
                internal_error(true, "Cannot send response, got %zd bytes, expecting %zu bytes", bytes_sent, strlen(initial_response));
                rrdpush_receive_log_status(
                        rpt, "cannot reply back, dropping connection",
                        RRDPUSH_STATUS_CANT_REPLY, NDLP_ERR);
                goto cleanup;
            }
#ifdef ENABLE_H2O
        }
#endif
    }

#ifdef ENABLE_H2O
    unless_h2o_rrdpush(rpt)
#endif
    {
        // remove the non-blocking flag from the socket
        if(sock_delnonblock(rpt->fd) < 0)
            netdata_log_error("STREAM '%s' [receive from [%s]:%s]: "
                  "cannot remove the non-blocking flag from socket %d"
                  , rrdhost_hostname(rpt->host)
                  , rpt->client_ip, rpt->client_port
                  , rpt->fd);

        struct timeval timeout;
        timeout.tv_sec = 600;
        timeout.tv_usec = 0;
        if (unlikely(setsockopt(rpt->fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof timeout) != 0))
            netdata_log_error("STREAM '%s' [receive from [%s]:%s]: "
                  "cannot set timeout for socket %d"
                  , rrdhost_hostname(rpt->host)
                  , rpt->client_ip, rpt->client_port
                  , rpt->fd);
    }

    rrdpush_receive_log_status(
            rpt, "connected and ready to receive data",
            RRDPUSH_STATUS_CONNECTED, NDLP_INFO);

#ifdef ENABLE_ACLK
    // in case we have cloud connection we inform cloud
    // new child connected
    if (netdata_cloud_enabled)
        aclk_host_state_update(rpt->host, 1, 1);
#endif

    rrdhost_set_is_parent_label();

    if (is_ephemeral)
        rrdhost_option_set(rpt->host, RRDHOST_OPTION_EPHEMERAL_HOST);

    // let it reconnect to parent immediately
    rrdpush_reset_destinations_postpone_time(rpt->host);

    size_t count = streaming_parser(rpt, &cd, rpt->fd,
#ifdef ENABLE_HTTPS
                                    (rpt->ssl.conn) ? &rpt->ssl : NULL
#else
                                    NULL
#endif
                                    );

    receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_EXIT, false);

    {
        char msg[100 + 1];
        snprintfz(msg, sizeof(msg) - 1, "disconnected (completed %zu updates)", count);
        rrdpush_receive_log_status(
                rpt, msg,
                RRDPUSH_STATUS_DISCONNECTED, NDLP_WARNING);
    }

#ifdef ENABLE_ACLK
    // in case we have cloud connection we inform cloud
    // a child disconnected
    if (netdata_cloud_enabled)
        aclk_host_state_update(rpt->host, 0, 1);
#endif

cleanup:
    ;
}

static void rrdpush_receiver_thread_cleanup(void *ptr) {
    struct receiver_state *rpt = (struct receiver_state *) ptr;
    worker_unregister();

    rrdhost_clear_receiver(rpt);

    netdata_log_info("STREAM '%s' [receive from [%s]:%s]: "
         "receive thread ended (task id %d)"
    , rpt->hostname ? rpt->hostname : "-"
    , rpt->client_ip ? rpt->client_ip : "-", rpt->client_port ? rpt->client_port : "-"
    , gettid());

    receiver_state_free(rpt);

    rrdhost_set_is_parent_label();
}

static bool stream_receiver_log_capabilities(BUFFER *wb, void *ptr) {
    struct receiver_state *rpt = ptr;
    if(!rpt)
        return false;

    stream_capabilities_to_string(wb, rpt->capabilities);
    return true;
}

static bool stream_receiver_log_transport(BUFFER *wb, void *ptr) {
    struct receiver_state *rpt = ptr;
    if(!rpt)
        return false;

#ifdef ENABLE_HTTPS
    buffer_strcat(wb, SSL_connection(&rpt->ssl) ? "https" : "http");
#else
    buffer_strcat(wb, "http");
#endif
    return true;
}

void *rrdpush_receiver_thread(void *ptr) {
    netdata_thread_cleanup_push(rrdpush_receiver_thread_cleanup, ptr);

            {
                worker_register("STREAMRCV");

                worker_register_job_custom_metric(WORKER_RECEIVER_JOB_BYTES_READ,
                                                  "received bytes", "bytes/s",
                                                  WORKER_METRIC_INCREMENT);

                worker_register_job_custom_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED,
                                                  "uncompressed bytes", "bytes/s",
                                                  WORKER_METRIC_INCREMENT);

                worker_register_job_custom_metric(WORKER_RECEIVER_JOB_REPLICATION_COMPLETION,
                                                  "replication completion", "%",
                                                  WORKER_METRIC_ABSOLUTE);

                struct receiver_state *rpt = (struct receiver_state *) ptr;
                rpt->tid = gettid();

                ND_LOG_STACK lgs[] = {
                        ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->client_ip),
                        ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->client_port),
                        ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rpt->hostname),
                        ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
                        ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
                        ND_LOG_FIELD_END(),
                };
                ND_LOG_STACK_PUSH(lgs);

                netdata_log_info("STREAM %s [%s]:%s: receive thread started", rpt->hostname, rpt->client_ip
                                 , rpt->client_port);

                rrdpush_receive(rpt);
            }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
