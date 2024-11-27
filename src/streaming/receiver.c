// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"
#include "receiver-internals.h"
#include "web/server/h2o/http_server.h"

// When a child disconnects this is the maximum we will wait
// before we update the cloud that the child is offline
#define MAX_CHILD_DISC_DELAY (30000)
#define MAX_CHILD_DISC_TOLERANCE (125 / 100)

static uint32_t streaming_connected_receivers = 0;

uint32_t stream_currently_connected_receivers(void) {
    return __atomic_load_n(&streaming_connected_receivers, __ATOMIC_RELAXED);
}

static void streaming_receiver_connected(void) {
    __atomic_add_fetch(&streaming_connected_receivers, 1, __ATOMIC_RELAXED);
}

static void streaming_receiver_disconnected(void) {
    __atomic_sub_fetch(&streaming_connected_receivers, 1, __ATOMIC_RELAXED);
}

void receiver_state_free(struct receiver_state *rpt) {
    nd_sock_close(&rpt->sock);
    rrdpush_decompressor_destroy(&rpt->receiver.compressed.decompressor);

    if(rpt->system_info)
        rrdhost_system_info_free(rpt->system_info);

    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_receivers, sizeof(*rpt), __ATOMIC_RELAXED);

    freez(rpt->key);
    freez(rpt->hostname);
    freez(rpt->registry_hostname);
    freez(rpt->machine_guid);
    freez(rpt->os);
    freez(rpt->timezone);
    freez(rpt->abbrev_timezone);
    freez(rpt->client_ip);
    freez(rpt->client_port);
    freez(rpt->program_name);
    freez(rpt->program_version);

    string_freez(rpt->config.send.api_key);
    string_freez(rpt->config.send.parents);
    string_freez(rpt->config.send.charts_matching);

    freez(rpt);
}

#include "plugins.d/pluginsd_parser.h"

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
    if (is_h2o_rrdpush(r)) {
        if(nd_thread_signaled_to_cancel())
            return -4;

        return (int)h2o_stream_read(r->h2o_ctx, buffer, size);
    }
#endif

    int tries = 100;
    ssize_t bytes_read;

    do {
        errno_clear();
        bytes_read = nd_sock_read(&r->sock, buffer, size);

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
            return STREAM_HANDSHAKE_DISCONNECT_NOT_SUFFICIENT_SENDER_READ_BUFFER;

        case -1:
            // EOF
            return STREAM_HANDSHAKE_DISCONNECT_SOCKET_EOF;

        case -2:
            // failed to read
            return STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_FAILED;

        case -3:
            // timeout
            return STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_TIMEOUT;

        case -4:
            // the thread is cancelled
            return STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN;

        default:
            // anything else
            return STREAM_HANDSHAKE_DISCONNECT_UNKNOWN_SOCKET_READ_ERROR;
    }
}

static inline bool receiver_read_uncompressed(struct receiver_state *r, STREAM_HANDSHAKE *reason) {
    internal_fatal(r->reader.read_buffer[r->reader.read_len] != '\0',
                   "%s: read_buffer does not start with zero #2", __FUNCTION__ );

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

typedef enum {
    DECOMPRESS_NEED_MORE_DATA,
    DECOMPRESS_FAILED,
    DECOMPRESS_OK,
} decompressor_status_t;

static inline void receiver_move_compressed(struct receiver_state *r) {
    size_t remaining = r->receiver.compressed.used - r->receiver.compressed.start;
    if(remaining > 0) {
        memmove(r->receiver.compressed.buf, r->receiver.compressed.buf + r->receiver.compressed.start, remaining);
        r->receiver.compressed.start = 0;
        r->receiver.compressed.used = remaining;
    }
    else {
        r->receiver.compressed.start = 0;
        r->receiver.compressed.used = 0;
    }
}

static inline decompressor_status_t receiver_feed_decompressor(struct receiver_state *r) {
    char *buf = r->receiver.compressed.buf;
    size_t start = r->receiver.compressed.start;
    size_t signature_size = r->receiver.compressed.decompressor.signature_size;
    size_t used = r->receiver.compressed.used;

    if(start + signature_size > used) {
        // incomplete header, we need to wait for more data
        receiver_move_compressed(r);
        return DECOMPRESS_NEED_MORE_DATA;
    }

    size_t compressed_message_size = rrdpush_decompressor_start(&r->receiver.compressed.decompressor, buf + start, signature_size);

    if (unlikely(!compressed_message_size)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "multiplexed uncompressed data in compressed stream!");
        return DECOMPRESS_FAILED;
    }

    if(unlikely(compressed_message_size > COMPRESSION_MAX_MSG_SIZE)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "received a compressed message of %zu bytes, which is bigger than the max compressed message "
               "size supported of %zu. Ignoring message.",
               compressed_message_size, (size_t)COMPRESSION_MAX_MSG_SIZE);
        return DECOMPRESS_FAILED;
    }

    if(start + signature_size + compressed_message_size > used) {
        // incomplete compressed message, we need to wait for more data
        receiver_move_compressed(r);
        return DECOMPRESS_NEED_MORE_DATA;
    }

    size_t bytes_to_parse = rrdpush_decompress(
        &r->receiver.compressed.decompressor, buf + start + signature_size, compressed_message_size);

    if (unlikely(!bytes_to_parse)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "no bytes to parse.");
        return DECOMPRESS_FAILED;
    }

    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_UNCOMPRESSED, (NETDATA_DOUBLE)bytes_to_parse);

    // move the header to the next message
    r->receiver.compressed.start += signature_size + compressed_message_size;

    return DECOMPRESS_OK;
}

static inline decompressor_status_t receiver_get_decompressed(struct receiver_state *r) {
    if (unlikely(!rrdpush_decompressed_bytes_in_buffer(&r->receiver.compressed.decompressor)))
        return DECOMPRESS_NEED_MORE_DATA;

    size_t available = sizeof(r->reader.read_buffer) - r->reader.read_len - 1;
    if (likely(available)) {
        size_t len = rrdpush_decompressor_get(&r->receiver.compressed.decompressor, r->reader.read_buffer + r->reader.read_len, available);
        if (unlikely(!len)) {
            internal_error(true, "decompressor returned zero length #1");
            return DECOMPRESS_FAILED;
        }

        r->reader.read_len += (int)len;
        r->reader.read_buffer[r->reader.read_len] = '\0';
    }
    else {
        internal_fatal(true, "The line to read is too big! Already have %zd bytes in read_buffer.", r->reader.read_len);
        return DECOMPRESS_FAILED;
    }

    return DECOMPRESS_OK;
}

static inline bool receiver_read_compressed(struct receiver_state *r, STREAM_HANDSHAKE *reason) {

    internal_fatal(r->reader.read_buffer[r->reader.read_len] != '\0',
                   "%s: read_buffer does not start with zero #2", __FUNCTION__ );

    int bytes_read = read_stream(r, r->receiver.compressed.buf + r->receiver.compressed.used,
                                 sizeof(r->receiver.compressed.buf) - r->receiver.compressed.used);

    if (unlikely(bytes_read <= 0)) {
        *reason = read_stream_error_to_reason(bytes_read);
        return false;
    }

    r->receiver.compressed.used += bytes_read;
    worker_set_metric(WORKER_RECEIVER_JOB_BYTES_READ, (NETDATA_DOUBLE)bytes_read);

    return true;
}

bool plugin_is_enabled(struct plugind *cd);
static void rrdhost_clear_receiver(struct receiver_state *rpt);

static void rrdpush_receive_log_status(struct receiver_state *rpt, const char *msg, const char *status, ND_LOG_FIELD_PRIORITY priority) {
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

static void receiver_set_exit_reason(struct receiver_state *rpt, STREAM_HANDSHAKE reason, bool force) {
    if(force || !rpt->exit.reason)
        rpt->exit.reason = reason;
}

static inline bool receiver_should_stop(struct receiver_state *rpt) {
    if(unlikely(__atomic_load_n(&rpt->exit.shutdown, __ATOMIC_RELAXED))) {
        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, false);
        return true;
    }

    return false;
}

static void streaming_parser_init(struct receiver_state *rpt) {
    rpt->receiver.cd = (struct plugind){
        .update_every = default_rrd_update_every,
        .unsafe = {
            .spinlock = NETDATA_SPINLOCK_INITIALIZER,
            .running = true,
            .enabled = true,
        },
        .started_t = now_realtime_sec(),
    };

    // put the client IP and port into the buffers used by plugins.d
    snprintfz(rpt->receiver.cd.id,           CONFIG_MAX_NAME,  "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(rpt->receiver.cd.filename,     FILENAME_MAX,     "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(rpt->receiver.cd.fullfilename, FILENAME_MAX,     "%s:%s", rpt->client_ip, rpt->client_port);
    snprintfz(rpt->receiver.cd.cmd,          PLUGINSD_CMD_MAX, "%s:%s", rpt->client_ip, rpt->client_port);

    PARSER *parser = NULL;
    {
        PARSER_USER_OBJECT user = {
            .enabled = plugin_is_enabled(&rpt->receiver.cd),
            .host = rpt->host,
            .opaque = rpt,
            .cd = &rpt->receiver.cd,
            .trust_durations = 1,
            .capabilities = rpt->capabilities,
        };

        parser = parser_init(&user, -1, -1, PARSER_INPUT_SPLIT, &rpt->sock);
    }

#ifdef ENABLE_H2O
    parser->h2o_ctx = rpt->h2o_ctx;
#endif

    pluginsd_keywords_init(parser, PARSER_INIT_STREAMING);

    rrd_collector_started();

    rpt->receiver.compressed.enabled = rrdpush_decompression_initialize(rpt);
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

    __atomic_store_n(&rpt->receiver.parser, parser, __ATOMIC_RELAXED);
    rrdpush_receiver_send_node_and_claim_id_to_child(rpt->host);

    rpt->receiver.buffer = buffer_create(sizeof(rpt->reader.read_buffer), NULL);
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

    buffer_strcat(wb, nd_sock_is_ssl(&rpt->sock) ? "https" : "http");
    return true;
}

#define MAX_RECEIVERS 2048

struct receiver {
    size_t id;
    pid_t tid;

    ND_THREAD *thread;
    size_t nodes;

    struct {
        SPINLOCK spinlock;
        struct receiver_state *ll;
    } queue;

    struct {
        size_t size;
        size_t used;
        struct pollfd *pollfds;
        struct receiver_state **nodes;
    } run;
};

struct {
    size_t cores;
    struct receiver receivers[MAX_RECEIVERS];
} receiver_globals = { 0 };

void stream_receiver_cancel_threads(void) {
    for(size_t i = 0; i < MAX_RECEIVERS ;i++)
        nd_thread_signal_cancel(receiver_globals.receivers[i].thread);
}

static void stream_receiver_realloc_arrays_unsafe(struct receiver *rr, size_t slot) {
    internal_fatal(rr->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    if(slot >= rr->run.size) {
        size_t new_size = rr->run.size > 0 ? rr->run.size * 2 : 8;
        rr->run.pollfds = reallocz(rr->run.pollfds, new_size * sizeof(*rr->run.pollfds));
        rr->run.nodes = reallocz(rr->run.nodes, new_size * sizeof(*rr->run.nodes));
        rr->run.size = new_size;
        rr->run.used = slot + 1;
    }
    else if(slot >= rr->run.used)
        rr->run.used = slot + 1;
}

static void stream_receiver_move_queue_to_running(struct receiver *rr) {
    internal_fatal(rr->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    size_t first_slot = 0;

    // process the queue
    spinlock_lock(&rr->queue.spinlock);
    while(rr->queue.ll) {
        // worker_is_busy(WORKER_SENDER_DISPATCHER_JOB_DEQUEUE);

        struct receiver_state *rpt = rr->queue.ll;
        ND_LOG_STACK lgs[] = {
            ND_LOG_FIELD_STR(NDF_NIDL_NODE, rpt->host->hostname),
            ND_LOG_FIELD_UUID(NDF_MESSAGE_ID, &streaming_to_parent_msgid),
            ND_LOG_FIELD_END(),
        };
        ND_LOG_STACK_PUSH(lgs);

        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(rr->queue.ll, rpt, prev, next);

        // slot 0 is our pipe
        size_t slot = rr->run.used;

        // find an empty slot
        for(size_t i = first_slot; i < slot && i < rr->run.used ;i++) {
            if(!rr->run.nodes[i]) {
                slot = i;
                break;
            }
        }

        stream_receiver_realloc_arrays_unsafe(rr, slot);
        rpt->receiver.compressed.start = 0;
        rpt->receiver.compressed.used = 0;

        streaming_parser_init(rpt);

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM [receive%zu] [%s]: moving host from receiver queue to receiver running slot %zu...",
               rr->id, rrdhost_hostname(rpt->host), slot);

        rr->run.nodes[slot] = rpt;
        rr->run.pollfds[slot] = (struct pollfd){
            .fd = rpt->sock.fd,
            .events = POLLIN,
            .revents = 0,
        };

        first_slot = slot + 1;
    }
    spinlock_unlock(&rr->queue.spinlock);
}

static void stream_receiver_on_disconnect(struct receiver *rr __maybe_unused, struct receiver_state *rpt) {
    internal_fatal(rr->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );
    if(!rpt) return;

    buffer_free(rpt->receiver.buffer);
    rpt->receiver.buffer = NULL;

    // cleanup the sender buffer, because we may end-up reusing an incomplete buffer
    sender_thread_buffer_free();

    size_t count = 0;
    PARSER *parser = __atomic_load_n(&rpt->receiver.parser, __ATOMIC_RELAXED);
    if(parser) {
        parser->user.v2.stream_buffer.wb = NULL;

        // make sure send_to_plugin() will not write any data to the socket
        spinlock_lock(&parser->writer.spinlock);
        parser->fd_input = -1;
        parser->fd_output = -1;
        parser->sock = NULL;
        spinlock_unlock(&parser->writer.spinlock);

        count = parser->user.data_collections_count;
    }

    // the parser stopped
    receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_EXIT, false);

    {
        char msg[100 + 1];
        snprintfz(msg, sizeof(msg) - 1, "disconnected (completed %zu updates)", count);
        rrdpush_receive_log_status(rpt, msg, RRDPUSH_STATUS_DISCONNECTED, NDLP_WARNING);
    }

    // in case we have cloud connection we inform cloud
    // a child disconnected
    uint64_t total_reboot = rrdhost_stream_path_total_reboot_time_ms(rpt->host);
    schedule_node_state_update(rpt->host, MIN((total_reboot * MAX_CHILD_DISC_TOLERANCE), MAX_CHILD_DISC_DELAY));

    rrdhost_clear_receiver(rpt);
    rrdhost_set_is_parent_label();
    receiver_state_free(rpt);
}

static void stream_receiver_remove(struct receiver *rr, struct receiver_state *rpt, size_t slot, const char *why) {
    internal_fatal(rr->tid != gettid_cached(), "Function %s() should only be used by the dispatcher thread", __FUNCTION__ );

    nd_log(NDLS_DAEMON, NDLP_ERR,
           "STREAM '%s' [receive from [%s]:%s]: "
           "receiver disconnected: %s"
           , rpt->hostname ? rpt->hostname : "-"
           , rpt->client_ip ? rpt->client_ip : "-"
           , rpt->client_port ? rpt->client_port : "-"
           , why ? why : "");

    stream_receiver_on_disconnect(rr, rpt);
    rr->run.nodes[slot] = NULL;
    rr->run.pollfds[slot] = (struct pollfd){
        .fd = -1,
        .events = 0,
        .revents = 0,
    };
}

static void *stream_receive_thread(void *ptr) {
    struct receiver *rr = ptr;
    rr->tid = gettid_cached();

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

    while(!nd_thread_signaled_to_cancel() && service_running(SERVICE_STREAMING)) {
        stream_receiver_move_queue_to_running(rr);

        if(!rr->run.used) {
            sleep_usec(50 * USEC_PER_MS);
            continue;
        }

        int poll_rc = poll(rr->run.pollfds, rr->run.used, 100);

        if (poll_rc == 0 || ((poll_rc == -1) && (errno == EAGAIN || errno == EINTR)))
            // timed out - just loop again
            continue;

        if(unlikely(poll_rc == -1)) {
            // poll() returned an error
            nd_log_limit_static_thread_var(erl, 1, 1 * USEC_PER_MS);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_ERR, "STREAM [receiver%zu] poll() returned error", rr->id);
            continue;
        }

        time_t now_s = now_monotonic_sec();

        for(size_t slot = 0; slot < rr->run.used ;slot++) {
            if(!rr->run.pollfds[slot].revents || !rr->run.nodes[slot]) continue;

            if(nd_thread_signaled_to_cancel() || !service_running(SERVICE_STREAMING))
                break;

            struct receiver_state *rpt = rr->run.nodes[slot];

            PARSER *parser = __atomic_load_n(&rpt->receiver.parser, __ATOMIC_RELAXED);
            ND_LOG_STACK lgs[] = {
                ND_LOG_FIELD_TXT(NDF_SRC_IP, rpt->client_ip),
                ND_LOG_FIELD_TXT(NDF_SRC_PORT, rpt->client_port),
                ND_LOG_FIELD_TXT(NDF_NIDL_NODE, rpt->hostname),
                ND_LOG_FIELD_CB(NDF_SRC_TRANSPORT, stream_receiver_log_transport, rpt),
                ND_LOG_FIELD_CB(NDF_SRC_CAPABILITIES, stream_receiver_log_capabilities, rpt),
                ND_LOG_FIELD_CB(NDF_REQUEST, line_splitter_reconstruct_line, &parser->line),
                ND_LOG_FIELD_CB(NDF_NIDL_NODE, parser_reconstruct_node, parser),
                ND_LOG_FIELD_CB(NDF_NIDL_INSTANCE, parser_reconstruct_instance, parser),
                ND_LOG_FIELD_CB(NDF_NIDL_CONTEXT, parser_reconstruct_context, parser),
                ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            if(receiver_should_stop(rpt)) {
                receiver_set_exit_reason(rpt, rpt->exit.reason, false);
                stream_receiver_remove(rr, rpt, slot, "received stop signal");
                continue;
            }

            rpt->last_msg_t = now_s;

            if(rpt->receiver.compressed.enabled) {
                STREAM_HANDSHAKE reason = STREAM_HANDSHAKE_DISCONNECT_UNKNOWN_SOCKET_READ_ERROR;
                if(unlikely(!receiver_read_compressed(rpt, &reason))) {
                    receiver_set_exit_reason(rpt, reason, false);
                    stream_receiver_remove(rr, rpt, slot, "socket read error");
                    continue;
                }

                bool node_broken = false;
                while(!node_broken && !nd_thread_signaled_to_cancel() && service_running(SERVICE_STREAMING) && !receiver_should_stop(rpt)) {
                    decompressor_status_t feed = receiver_feed_decompressor(rpt);
                    if(likely(feed == DECOMPRESS_OK)) {
                        while (!node_broken) {
                            decompressor_status_t rc = receiver_get_decompressed(rpt);
                            if (likely(rc == DECOMPRESS_OK)) {
                                while (buffered_reader_next_line(&rpt->reader, rpt->receiver.buffer)) {
                                    if (unlikely(parser_action(parser, rpt->receiver.buffer->buffer))) {

                                        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                                        stream_receiver_remove(rr, rpt, slot, "parser failed");
                                        node_broken = true;
                                        break;
                                    }

                                    rpt->receiver.buffer->len = 0;
                                    rpt->receiver.buffer->buffer[0] = '\0';
                                }
                            }
                            else if (rc == DECOMPRESS_NEED_MORE_DATA)
                                break;

                            else {
                                receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                                stream_receiver_remove(rr, rpt, slot, "decompressor failed");
                                node_broken = true;
                                break;
                            }
                        }
                    }
                    else if (feed == DECOMPRESS_NEED_MORE_DATA)
                        break;
                    else {
                        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                        stream_receiver_remove(rr, rpt, slot, "compressed data invalid");
                        node_broken = true;
                        break;
                    }
                }

                if(receiver_should_stop(rpt)) {
                    receiver_set_exit_reason(rpt, rpt->exit.reason, false);
                    stream_receiver_remove(rr, rpt, slot, "received stop signal");
                    continue;
                }
            }
            else {
                STREAM_HANDSHAKE reason = STREAM_HANDSHAKE_DISCONNECT_UNKNOWN_SOCKET_READ_ERROR;
                if(unlikely(!receiver_read_uncompressed(rpt, &reason))) {
                    receiver_set_exit_reason(rpt, reason, false);
                    stream_receiver_remove(rr, rpt, slot, "socker read error");
                    continue;
                }

                while(buffered_reader_next_line(&rpt->reader, rpt->receiver.buffer)) {
                    if(unlikely(parser_action(parser, rpt->receiver.buffer->buffer))) {
                        receiver_set_exit_reason(rpt, STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, false);
                        stream_receiver_remove(rr, rpt, slot, "parser failed");
                        break;
                    }

                    rpt->receiver.buffer->len = 0;
                    rpt->receiver.buffer->buffer[0] = '\0';
                }
            }

            rr->run.pollfds[slot].revents = 0;
        }
    }

    for(size_t i = 0; i < rr->run.used ;i++) {
        if (rr->run.nodes[i])
            stream_receiver_remove(rr, rr->run.nodes[i], i, "shutdown");
    }

    worker_unregister();

    rr->thread = NULL;
    return NULL;
}

static void stream_receiver_add(struct receiver_state *rpt) {
    SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;

    spinlock_lock(&spinlock);
    if(!receiver_globals.cores) {
        receiver_globals.cores = get_netdata_cpus() - 1;
        if(receiver_globals.cores < 4)
            receiver_globals.cores = 4;
        else if(receiver_globals.cores > MAX_RECEIVERS)
            receiver_globals.cores = MAX_RECEIVERS;
    }

    size_t min_slot = 0;
    size_t min_nodes = receiver_globals.receivers[0].nodes;
    for(size_t i = 1; i < receiver_globals.cores ; i++) {
        if(receiver_globals.receivers[i].nodes < min_nodes) {
            min_slot = i;
            min_nodes = receiver_globals.receivers[i].nodes;
        }
    }
    rpt->receiver.id = min_slot;

    struct receiver *rr = &receiver_globals.receivers[rpt->receiver.id];

    spinlock_lock(&rr->queue.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(rr->queue.ll, rpt, prev, next);
    rr->nodes++;
    spinlock_unlock(&rr->queue.spinlock);

    if(!rr->thread) {
        rr->id = rr - receiver_globals.receivers;

        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_RECEIVER "[%zu]", rr->id);
        tag[NETDATA_THREAD_TAG_MAX] = '\0';

        rr->thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, stream_receive_thread, rr);
        if(!rr->thread) {
            rrdpush_receive_log_status(
                rpt, "can't create receiver thread",
                RRDPUSH_STATUS_INTERNAL_SERVER_ERROR, NDLP_ERR);
        }
    }

    spinlock_unlock(&spinlock);
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

    rrdhost_receiver_lock(host);

    if (!host->receiver) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);

        host->stream.rcv.status.connections++;
        streaming_receiver_connected();

        host->receiver = rpt;
        rpt->host = host;

        __atomic_store_n(&rpt->exit.shutdown, false, __ATOMIC_RELAXED);
        host->stream.rcv.status.last_connected = now_realtime_sec();
        host->stream.rcv.status.last_disconnected = 0;
        host->stream.rcv.status.last_chart = 0;
        host->stream.rcv.status.check_obsolete = true;

        if (rpt->config.health.enabled != CONFIG_BOOLEAN_NO) {
            if (rpt->config.health.delay > 0) {
                host->health.delay_up_to = now_realtime_sec() + rpt->config.health.delay;
                nd_log(NDLS_DAEMON, NDLP_DEBUG,
                       "[%s]: Postponing health checks for %" PRId64 " seconds, because it was just connected.",
                       rrdhost_hostname(host),
                       (int64_t) rpt->config.health.delay);
            }
        }

        host->health_log.health_log_retention_s = rpt->config.health.history;

//         this is a test
//        if(rpt->hops <= host->sender->hops)
//            rrdpush_sender_thread_stop(host, "HOPS MISMATCH", false);

        signal_rrdcontext = true;
        rrdpush_receiver_replication_reset(host);

        rrdhost_flag_clear(rpt->host, RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED);
        aclk_queue_node_info(rpt->host, true);

        rrdhost_stream_parents_reset(host, STREAM_HANDSHAKE_PREPARING);

        set_this = true;
    }

    rrdhost_receiver_unlock(host);

    if(signal_rrdcontext)
        rrdcontext_host_child_connected(host);

    return set_this;
}

static void rrdhost_clear_receiver(struct receiver_state *rpt) {
    RRDHOST *host = rpt->host;
    if(!host) return;

    rrdhost_receiver_lock(host);
    {
        // Make sure that we detach this thread and don't kill a freshly arriving receiver

        if (host->receiver == rpt) {
            rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED);
            rrdhost_receiver_unlock(host);
            {
                // run all these without having the receiver lock

                stream_path_child_disconnected(host);
                rrdhost_sender_signal_to_stop_and_wait(host, STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT, false);
                rrdpush_receiver_replication_reset(host);
                rrdcontext_host_child_disconnected(host);

                if (rpt->config.health.enabled)
                    rrdcalc_child_disconnected(host);

                rrdhost_stream_parents_reset(host, STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT);
            }
            rrdhost_receiver_lock(host);

            // now we have the lock again

            streaming_receiver_disconnected();

            __atomic_store_n(&host->receiver->exit.shutdown, false, __ATOMIC_RELAXED);
            host->stream.rcv.status.check_obsolete = false;
            host->stream.rcv.status.last_connected = 0;
            host->stream.rcv.status.last_disconnected = now_realtime_sec();
            host->health.enabled = false;

            host->stream.rcv.status.exit_reason = rpt->exit.reason;
            rrdhost_flag_set(host, RRDHOST_FLAG_ORPHAN);
            host->receiver = NULL;
        }
    }

    // this must be cleared with the receiver lock
    pluginsd_process_cleanup(rpt->receiver.parser);
    __atomic_store_n(&rpt->receiver.parser, NULL, __ATOMIC_RELAXED);

    rrdhost_receiver_unlock(host);
}

bool stop_streaming_receiver(RRDHOST *host, STREAM_HANDSHAKE reason) {
    bool ret = false;

    rrdhost_receiver_lock(host);

    if(host->receiver) {
        if(!__atomic_load_n(&host->receiver->exit.shutdown, __ATOMIC_RELAXED)) {
            __atomic_store_n(&host->receiver->exit.shutdown, true, __ATOMIC_RELAXED);
            receiver_set_exit_reason(host->receiver, reason, true);
            shutdown(host->receiver->sock.fd, SHUT_RDWR);
        }
    }

    int count = 2000;
    while (host->receiver && count-- > 0) {
        rrdhost_receiver_unlock(host);

        // let the lock for the receiver thread to exit
        sleep_usec(1 * USEC_PER_MS);

        rrdhost_receiver_lock(host);
    }

    if(host->receiver)
        netdata_log_error("STREAM '%s' [receive from [%s]:%s]: "
              "thread %zu takes too long to stop, giving up..."
        , rrdhost_hostname(host)
        , host->receiver->client_ip, host->receiver->client_port
        , host->receiver->receiver.id);
    else
        ret = true;

    rrdhost_receiver_unlock(host);

    return ret;
}

static void rrdpush_send_error_on_taken_over_connection(struct receiver_state *rpt, const char *msg) {
    nd_sock_send_timeout(&rpt->sock, (char *)msg, strlen(msg), 0, 5);
}

static bool rrdpush_receive(struct receiver_state *rpt) {
    stream_conf_receiver_config(rpt, &rpt->config, rpt->key, rpt->machine_guid);

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
            rpt->program_name,
            rpt->program_version,
            rpt->config.update_every,
            rpt->config.history,
            rpt->config.mode,
            rpt->config.health.enabled != CONFIG_BOOLEAN_NO,
            rpt->config.send.enabled && rpt->config.send.parents && rpt->config.send.api_key,
            rpt->config.send.parents,
            rpt->config.send.api_key,
            rpt->config.send.charts_matching,
            rpt->config.replication.enabled,
            rpt->config.replication.period,
            rpt->config.replication.step,
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
         "update every = %d, history = %d, memory mode = %s, health %s,%s"
         , rpt->hostname
         , rpt->client_ip
         , rpt->client_port
         , rrdhost_hostname(rpt->host)
         , rpt->host->machine_guid
         , rpt->host->rrd_update_every
         , rpt->host->rrd_history_entries
         , rrd_memory_mode_name(rpt->host->rrd_memory_mode)
         , (rpt->config.health.enabled == CONFIG_BOOLEAN_NO)?"disabled":((rpt->config.health.enabled == CONFIG_BOOLEAN_YES)?"enabled":"auto")
         , (rpt->sock.ssl.conn != NULL) ? " SSL," : ""
    );
#endif // NETDATA_INTERNAL_CHECKS


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
            ssize_t bytes_sent = nd_sock_send_timeout(&rpt->sock, initial_response, strlen(initial_response), 0, 60);

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
        if(sock_delnonblock(rpt->sock.fd) < 0)
            netdata_log_error("STREAM '%s' [receive from [%s]:%s]: "
                  "cannot remove the non-blocking flag from socket %d"
                  , rrdhost_hostname(rpt->host)
                  , rpt->client_ip, rpt->client_port
                  , rpt->sock.fd);

        struct timeval timeout;
        timeout.tv_sec = 600;
        timeout.tv_usec = 0;
        if (unlikely(setsockopt(rpt->sock.fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof timeout) != 0))
            netdata_log_error("STREAM '%s' [receive from [%s]:%s]: "
                  "cannot set timeout for socket %d"
                  , rrdhost_hostname(rpt->host)
                  , rpt->client_ip, rpt->client_port
                  , rpt->sock.fd);
    }

    rrdpush_receive_log_status(
            rpt, "connected and ready to receive data",
            RRDPUSH_STATUS_CONNECTED, NDLP_INFO);

    // in case we have cloud connection we inform cloud
    // new child connected
    schedule_node_state_update(rpt->host, 300);
    rrdhost_set_is_parent_label();

    if (rpt->config.ephemeral)
        rrdhost_option_set(rpt->host, RRDHOST_OPTION_EPHEMERAL_HOST);

    // let it reconnect to parent asap
    rrdhost_stream_parents_reset(rpt->host, STREAM_HANDSHAKE_PREPARING);

    // receive data
    stream_receiver_add(rpt);
    // streaming_parser(rpt);
    // stream_receiver_on_disconnect(rpt);
    return true;

cleanup:
    return false;
}

int rrdpush_receiver_permission_denied(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, START_STREAMING_ERROR_NOT_PERMITTED);
    return HTTP_RESP_UNAUTHORIZED;
}

int rrdpush_receiver_too_busy_now(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, START_STREAMING_ERROR_BUSY_TRY_LATER);
    return HTTP_RESP_SERVICE_UNAVAILABLE;
}

static void rrdpush_receiver_takeover_web_connection(struct web_client *w, struct receiver_state *rpt) {
    rpt->sock.fd = w->ifd;
    rpt->sock.ssl = w->ssl;

    w->ssl = NETDATA_SSL_UNSET_CONNECTION;

    WEB_CLIENT_IS_DEAD(w);

    if(web_server_mode == WEB_SERVER_MODE_STATIC_THREADED) {
        web_client_flag_set(w, WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET);
    }
    else {
        if(w->ifd == w->ofd)
            w->ifd = w->ofd = -1;
        else
            w->ifd = -1;
    }

    buffer_flush(w->response.data);
}

int rrdpush_receiver_thread_spawn(struct web_client *w, char *decoded_query_string, void *h2o_ctx __maybe_unused) {

    if(!service_running(ABILITY_STREAMING_CONNECTIONS))
        return rrdpush_receiver_too_busy_now(w);

    struct receiver_state *rpt = callocz(1, sizeof(*rpt));
    rpt->connected_since_s = now_realtime_sec();
    rpt->last_msg_t = now_monotonic_sec();
    rpt->hops = 1;

    rpt->capabilities = STREAM_CAP_INVALID;

#ifdef ENABLE_H2O
    rpt->h2o_ctx = h2o_ctx;
#endif

    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_receivers, sizeof(*rpt), __ATOMIC_RELAXED);
    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);

    rpt->system_info = callocz(1, sizeof(struct rrdhost_system_info));
    rpt->system_info->hops = rpt->hops;

    nd_sock_init(&rpt->sock, netdata_ssl_web_server_ctx, false);
    rpt->client_ip         = strdupz(w->client_ip);
    rpt->client_port       = strdupz(w->client_port);

    rpt->config.update_every = default_rrd_update_every;

    // parse the parameters and fill rpt and rpt->system_info

    while(decoded_query_string) {
        char *value = strsep_skip_consecutive_separators(&decoded_query_string, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "key") && !rpt->key)
            rpt->key = strdupz(value);

        else if(!strcmp(name, "hostname") && !rpt->hostname)
            rpt->hostname = strdupz(value);

        else if(!strcmp(name, "registry_hostname") && !rpt->registry_hostname)
            rpt->registry_hostname = strdupz(value);

        else if(!strcmp(name, "machine_guid") && !rpt->machine_guid)
            rpt->machine_guid = strdupz(value);

        else if(!strcmp(name, "update_every"))
            rpt->config.update_every = (int)strtoul(value, NULL, 0);

        else if(!strcmp(name, "os") && !rpt->os)
            rpt->os = strdupz(value);

        else if(!strcmp(name, "timezone") && !rpt->timezone)
            rpt->timezone = strdupz(value);

        else if(!strcmp(name, "abbrev_timezone") && !rpt->abbrev_timezone)
            rpt->abbrev_timezone = strdupz(value);

        else if(!strcmp(name, "utc_offset"))
            rpt->utc_offset = (int32_t)strtol(value, NULL, 0);

        else if(!strcmp(name, "hops"))
            rpt->hops = rpt->system_info->hops = (int16_t)strtol(value, NULL, 0);

        else if(!strcmp(name, "ml_capable"))
            rpt->system_info->ml_capable = strtoul(value, NULL, 0);

        else if(!strcmp(name, "ml_enabled"))
            rpt->system_info->ml_enabled = strtoul(value, NULL, 0);

        else if(!strcmp(name, "mc_version"))
            rpt->system_info->mc_version = strtoul(value, NULL, 0);

        else if(!strcmp(name, "ver") && (rpt->capabilities & STREAM_CAP_INVALID))
            rpt->capabilities = convert_stream_version_to_capabilities(strtoul(value, NULL, 0), NULL, false);

        else {
            // An old Netdata child does not have a compatible streaming protocol, map to something sane.
            if (!strcmp(name, "NETDATA_SYSTEM_OS_NAME"))
                name = "NETDATA_HOST_OS_NAME";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_ID"))
                name = "NETDATA_HOST_OS_ID";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_ID_LIKE"))
                name = "NETDATA_HOST_OS_ID_LIKE";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_VERSION"))
                name = "NETDATA_HOST_OS_VERSION";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_VERSION_ID"))
                name = "NETDATA_HOST_OS_VERSION_ID";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_DETECTION"))
                name = "NETDATA_HOST_OS_DETECTION";

            else if(!strcmp(name, "NETDATA_PROTOCOL_VERSION") && (rpt->capabilities & STREAM_CAP_INVALID))
                rpt->capabilities = convert_stream_version_to_capabilities(1, NULL, false);

            if (unlikely(rrdhost_set_system_info_variable(rpt->system_info, name, value))) {
                nd_log_daemon(NDLP_NOTICE, "STREAM '%s' [receive from [%s]:%s]: "
                                           "request has parameter '%s' = '%s', which is not used."
                              , (rpt->hostname && *rpt->hostname) ? rpt->hostname : "-"
                              , rpt->client_ip, rpt->client_port
                              , name, value);
            }
        }
    }

    if (rpt->capabilities & STREAM_CAP_INVALID)
        // no version is supplied, assume version 0;
        rpt->capabilities = convert_stream_version_to_capabilities(0, NULL, false);

    // find the program name and version
    if(w->user_agent && w->user_agent[0]) {
        char *t = strchr(w->user_agent, '/');
        if(t && *t) {
            *t = '\0';
            t++;
        }

        rpt->program_name = strdupz(w->user_agent);
        if(t && *t) rpt->program_version = strdupz(t);
    }

    // check if we should accept this connection

    if(!rpt->key || !*rpt->key) {
        rrdpush_receive_log_status(
            rpt, "request without an API key, rejecting connection",
            RRDPUSH_STATUS_NO_API_KEY, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!rpt->hostname || !*rpt->hostname) {
        rrdpush_receive_log_status(
            rpt, "request without a hostname, rejecting connection",
            RRDPUSH_STATUS_NO_HOSTNAME, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!rpt->registry_hostname)
        rpt->registry_hostname = strdupz(rpt->hostname);

    if(!rpt->machine_guid || !*rpt->machine_guid) {
        rrdpush_receive_log_status(
            rpt, "request without a machine GUID, rejecting connection",
            RRDPUSH_STATUS_NO_MACHINE_GUID, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        char buf[GUID_LEN + 1];

        if (regenerate_guid(rpt->key, buf) == -1) {
            rrdpush_receive_log_status(
                rpt, "API key is not a valid UUID (use the command uuidgen to generate one)",
                RRDPUSH_STATUS_INVALID_API_KEY, NDLP_WARNING);

            receiver_state_free(rpt);
            return rrdpush_receiver_permission_denied(w);
        }

        if (regenerate_guid(rpt->machine_guid, buf) == -1) {
            rrdpush_receive_log_status(
                rpt, "machine GUID is not a valid UUID",
                RRDPUSH_STATUS_INVALID_MACHINE_GUID, NDLP_WARNING);

            receiver_state_free(rpt);
            return rrdpush_receiver_permission_denied(w);
        }
    }

    if(!stream_conf_is_key_type(rpt->key, "api")) {
        rrdpush_receive_log_status(
            rpt, "API key is a machine GUID",
            RRDPUSH_STATUS_INVALID_API_KEY, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    // the default for api keys is false, so that users
    // have to enable them manually
    if(!stream_conf_api_key_is_enabled(rpt->key, false)) {
        rrdpush_receive_log_status(
            rpt, "API key is not enabled",
            RRDPUSH_STATUS_API_KEY_DISABLED, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!stream_conf_api_key_allows_client(rpt->key, w->client_ip)) {
        rrdpush_receive_log_status(
            rpt, "API key is not allowed from this IP",
            RRDPUSH_STATUS_NOT_ALLOWED_IP, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if (!stream_conf_is_key_type(rpt->machine_guid, "machine")) {
        rrdpush_receive_log_status(
            rpt, "machine GUID is an API key",
            RRDPUSH_STATUS_INVALID_MACHINE_GUID, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    // the default for machine guids is true, so that users do not
    // have to enable them manually
    if(!stream_conf_api_key_is_enabled(rpt->machine_guid, true)) {
        rrdpush_receive_log_status(
            rpt, "machine GUID is not enabled",
            RRDPUSH_STATUS_MACHINE_GUID_DISABLED, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!stream_conf_api_key_allows_client(rpt->machine_guid, w->client_ip)) {
        rrdpush_receive_log_status(
            rpt, "machine GUID is not allowed from this IP",
            RRDPUSH_STATUS_NOT_ALLOWED_IP, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if (strcmp(rpt->machine_guid, localhost->machine_guid) == 0) {
        rrdpush_receiver_takeover_web_connection(w, rpt);

        rrdpush_receive_log_status(
            rpt, "machine GUID is my own",
            RRDPUSH_STATUS_LOCALHOST, NDLP_DEBUG);

        char initial_response[HTTP_HEADER_SIZE + 1];
        snprintfz(initial_response, HTTP_HEADER_SIZE, "%s", START_STREAMING_ERROR_SAME_LOCALHOST);

        if(nd_sock_send_timeout(&rpt->sock, initial_response, strlen(initial_response), 0, 60) !=
            (ssize_t)strlen(initial_response)) {

            nd_log_daemon(NDLP_ERR, "STREAM '%s' [receive from [%s]:%s]: "
                                    "failed to reply."
                          , rpt->hostname
                          , rpt->client_ip, rpt->client_port
            );
        }

        receiver_state_free(rpt);
        return HTTP_RESP_OK;
    }

    if(unlikely(web_client_streaming_rate_t > 0)) {
        static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
        static time_t last_stream_accepted_t = 0;

        time_t now = now_realtime_sec();
        spinlock_lock(&spinlock);

        if(unlikely(last_stream_accepted_t == 0))
            last_stream_accepted_t = now;

        if(now - last_stream_accepted_t < web_client_streaming_rate_t) {
            spinlock_unlock(&spinlock);

            char msg[100 + 1];
            snprintfz(msg, sizeof(msg) - 1,
                      "rate limit, will accept new connection in %ld secs",
                      (long)(web_client_streaming_rate_t - (now - last_stream_accepted_t)));

            rrdpush_receive_log_status(
                rpt, msg,
                RRDPUSH_STATUS_RATE_LIMIT, NDLP_NOTICE);

            receiver_state_free(rpt);
            return rrdpush_receiver_too_busy_now(w);
        }

        last_stream_accepted_t = now;
        spinlock_unlock(&spinlock);
    }

    /*
     * Quick path for rejecting multiple connections. The lock taken is fine-grained - it only protects the receiver
     * pointer within the host (if a host exists). This protects against multiple concurrent web requests hitting
     * separate threads within the web-server and landing here. The lock guards the thread-shutdown sequence that
     * detaches the receiver from the host. If the host is being created (first time-access) then we also use the
     * lock to prevent race-hazard (two threads try to create the host concurrently, one wins and the other does a
     * lookup to the now-attached structure).
     */

    {
        time_t age = 0;
        bool receiver_stale = false;
        bool receiver_working = false;

        rrd_rdlock();
        RRDHOST *host = rrdhost_find_by_guid(rpt->machine_guid);
        if (unlikely(host && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) /* Ignore archived hosts. */
            host = NULL;

        if (host) {
            rrdhost_receiver_lock(host);
            if (host->receiver) {
                age = now_monotonic_sec() - host->receiver->last_msg_t;

                if (age < 30)
                    receiver_working = true;
                else
                    receiver_stale = true;
            }
            rrdhost_receiver_unlock(host);
        }
        rrd_rdunlock();

        if (receiver_stale && stop_streaming_receiver(host, STREAM_HANDSHAKE_DISCONNECT_STALE_RECEIVER)) {
            // we stopped the receiver
            // we can proceed with this connection
            receiver_stale = false;

            nd_log_daemon(NDLP_NOTICE, "STREAM '%s' [receive from [%s]:%s]: "
                                       "stopped previous stale receiver to accept this one."
                          , rpt->hostname
                          , rpt->client_ip, rpt->client_port
            );
        }

        if (receiver_working || receiver_stale) {
            // another receiver is already connected
            // try again later

            char msg[200 + 1];
            snprintfz(msg, sizeof(msg) - 1,
                      "multiple connections for same host, "
                      "old connection was last used %ld secs ago%s",
                      age, receiver_stale ? " (signaled old receiver to stop)" : " (new connection not accepted)");

            rrdpush_receive_log_status(
                rpt, msg,
                RRDPUSH_STATUS_ALREADY_CONNECTED, NDLP_DEBUG);

            // Have not set WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET - caller should clean up
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, START_STREAMING_ERROR_ALREADY_STREAMING);
            receiver_state_free(rpt);
            return HTTP_RESP_CONFLICT;
        }
    }

    rrdpush_receiver_takeover_web_connection(w, rpt);

    char tag[NETDATA_THREAD_TAG_MAX + 1];
    snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_RECEIVER "[%s]", rpt->hostname);
    tag[NETDATA_THREAD_TAG_MAX] = '\0';

    rrdpush_receive(rpt);

    // prevent the caller from closing the streaming socket
    return HTTP_RESP_OK;
}


