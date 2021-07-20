// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"

extern struct config stream_config;
extern int netdata_use_ssl_on_stream;
extern char *netdata_ssl_ca_path;
extern char *netdata_ssl_ca_file;

// Collector thread starting a transmission
void sender_start(struct sender_state *s) {
    netdata_mutex_lock(&s->mutex);
    buffer_flush(s->build);
}

// Collector thread finishing a transmission
void sender_commit(struct sender_state *s) {
    if(cbuffer_add_unsafe(s->host->sender->buffer, buffer_tostring(s->host->sender->build),
       s->host->sender->build->len))
        s->overflow = 1;
    buffer_flush(s->build);
    netdata_mutex_unlock(&s->mutex);
}

// Collector thread finishing a transmission
// Returns 0 on success, 1 on overflow or if more than 50% of the send buffer is filled
int sender_commit_no_overflow(struct sender_state *s) {
    int ret = 0;

    size_t len = cbuffer_len_unsafe(s->host->sender->buffer);
    // check if 50% of the send buffer is filled
    if (s->host->sender->build->len + len >= s->host->sender->buffer->max_size / 2)
        ret = 1;
    // check if an overflow occured
    if(cbuffer_add_unsafe(s->host->sender->buffer, buffer_tostring(s->host->sender->build),
                      s->host->sender->build->len))
        ret = 1;
    buffer_flush(s->build);
    netdata_mutex_unlock(&s->mutex);

    return ret;
}

static inline void rrdpush_sender_thread_close_socket(RRDHOST *host) {
    host->rrdpush_sender_connected = 0;

    if(host->rrdpush_sender_socket != -1) {
        close(host->rrdpush_sender_socket);
        host->rrdpush_sender_socket = -1;
    }
}

static inline void rrdpush_sender_add_host_variable_to_buffer_nolock(RRDHOST *host, RRDVAR *rv) {
    calculated_number *value = (calculated_number *)rv->value;

    buffer_sprintf(
            host->sender->build
            , "VARIABLE HOST %s = " CALCULATED_NUMBER_FORMAT "\n"
            , rv->name
            , *value
    );

    debug(D_STREAM, "RRDVAR pushed HOST VARIABLE %s = " CALCULATED_NUMBER_FORMAT, rv->name, *value);
}

void rrdpush_sender_send_this_host_variable_now(RRDHOST *host, RRDVAR *rv) {
    if(host->rrdpush_send_enabled && host->rrdpush_sender_spawn && host->rrdpush_sender_connected) {
        sender_start(host->sender);
        rrdpush_sender_add_host_variable_to_buffer_nolock(host, rv);
        sender_commit(host->sender);
    }
}


static int rrdpush_sender_thread_custom_host_variables_callback(void *rrdvar_ptr, void *host_ptr) {
    RRDVAR *rv = (RRDVAR *)rrdvar_ptr;
    RRDHOST *host = (RRDHOST *)host_ptr;

    if(unlikely(rv->options & RRDVAR_OPTION_CUSTOM_HOST_VAR && rv->type == RRDVAR_TYPE_CALCULATED)) {
        rrdpush_sender_add_host_variable_to_buffer_nolock(host, rv);

        // return 1, so that the traversal will return the number of variables sent
        return 1;
    }

    // returning a negative number will break the traversal
    return 0;
}

static void rrdpush_sender_thread_send_custom_host_variables(RRDHOST *host) {
    sender_start(host->sender);
    int ret = rrdvar_callback_for_all_host_variables(host, rrdpush_sender_thread_custom_host_variables_callback, host);
    (void)ret;
    sender_commit(host->sender);

    debug(D_STREAM, "RRDVAR sent %d VARIABLES", ret);
}

// resets all the chart, so that their definitions
// will be resent to the central netdata
static void rrdpush_sender_thread_reset_all_charts(RRDHOST *host) {
    rrdhost_rdlock(host);

    RRDSET *st;
    rrdset_foreach_read(st, host) {
        rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

        st->upstream_resync_time = 0;

        rrdset_rdlock(st);

        RRDDIM *rd;
        rrddim_foreach_read(rd, st)
            rd->exposed = 0;

        rrdset_unlock(st);
    }

    rrdhost_unlock(host);
}

static inline void rrdpush_sender_thread_data_flush(RRDHOST *host) {
    netdata_mutex_lock(&host->sender->mutex);

    size_t len = cbuffer_next_unsafe(host->sender->buffer, NULL);
    if (len)
        error("STREAM %s [send]: discarding %zu bytes of metrics already in the buffer.", host->hostname, len);

    cbuffer_remove_unsafe(host->sender->buffer, len);
    netdata_mutex_unlock(&host->sender->mutex);

    rrdpush_sender_thread_reset_all_charts(host);
    rrdpush_sender_thread_send_custom_host_variables(host);
}

static inline void rrdpush_set_flags_to_newest_stream(RRDHOST *host) {
    host->labels.labels_flag |= LABEL_FLAG_UPDATE_STREAM;
    host->labels.labels_flag &= ~LABEL_FLAG_STOP_STREAM;
}

void rrdpush_encode_variable(stream_encoded_t *se, RRDHOST *host)
{
    se->os_name = (host->system_info->host_os_name)?url_encode(host->system_info->host_os_name):"";
    se->os_id = (host->system_info->host_os_id)?url_encode(host->system_info->host_os_id):"";
    se->os_version = (host->system_info->host_os_version)?url_encode(host->system_info->host_os_version):"";
    se->kernel_name = (host->system_info->kernel_name)?url_encode(host->system_info->kernel_name):"";
    se->kernel_version = (host->system_info->kernel_version)?url_encode(host->system_info->kernel_version):"";
}

void rrdpush_clean_encoded(stream_encoded_t *se)
{
    if (se->os_name)
        freez(se->os_name);

    if (se->os_id)
        freez(se->os_id);

    if (se->os_version)
        freez(se->os_version);

    if (se->kernel_name)
        freez(se->kernel_name);

    if (se->kernel_version)
        freez(se->kernel_version);
}

static int rrdpush_sender_thread_connect_to_parent(RRDHOST *host, int default_port, int timeout,
    struct sender_state *s) {

    struct timeval tv = {
            .tv_sec = timeout,
            .tv_usec = 0
    };

    // make sure the socket is closed
    rrdpush_sender_thread_close_socket(host);

    debug(D_STREAM, "STREAM: Attempting to connect...");
    info("STREAM %s [send to %s]: connecting...", host->hostname, host->rrdpush_send_destination);

    host->rrdpush_sender_socket = connect_to_one_of(
            host->rrdpush_send_destination
            , default_port
            , &tv
            , &s->reconnects_counter
            , s->connected_to
            , sizeof(s->connected_to)-1
    );

    if(unlikely(host->rrdpush_sender_socket == -1)) {
        error("STREAM %s [send to %s]: failed to connect", host->hostname, host->rrdpush_send_destination);
        return 0;
    }

    info("STREAM %s [send to %s]: initializing communication...", host->hostname, s->connected_to);

#ifdef ENABLE_HTTPS
    if( netdata_client_ctx ){
        host->ssl.flags = NETDATA_SSL_START;
        if (!host->ssl.conn){
            host->ssl.conn = SSL_new(netdata_client_ctx);
            if(!host->ssl.conn){
                error("Failed to allocate SSL structure.");
                host->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
            }
        }
        else{
            SSL_clear(host->ssl.conn);
        }

        if (host->ssl.conn)
        {
            if (SSL_set_fd(host->ssl.conn, host->rrdpush_sender_socket) != 1) {
                error("Failed to set the socket to the SSL on socket fd %d.", host->rrdpush_sender_socket);
                host->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
            } else{
                host->ssl.flags = NETDATA_SSL_HANDSHAKE_COMPLETE;
            }
        }
    }
    else {
        host->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
    }
#endif

    /* TODO: During the implementation of #7265 switch the set of variables to HOST_* and CONTAINER_* if the
             version negotiation resulted in a high enough version.
    */
    stream_encoded_t se;
    rrdpush_encode_variable(&se, host);

    char http[HTTP_HEADER_SIZE + 1];
    int eol = snprintfz(http, HTTP_HEADER_SIZE,
            "STREAM key=%s&hostname=%s&registry_hostname=%s&machine_guid=%s&update_every=%d&os=%s&timezone=%s&abbrev_timezone=%s&utc_offset=%d&hops=%d&tags=%s&ver=%u"
                 "&NETDATA_SYSTEM_OS_NAME=%s"
                 "&NETDATA_SYSTEM_OS_ID=%s"
                 "&NETDATA_SYSTEM_OS_ID_LIKE=%s"
                 "&NETDATA_SYSTEM_OS_VERSION=%s"
                 "&NETDATA_SYSTEM_OS_VERSION_ID=%s"
                 "&NETDATA_SYSTEM_OS_DETECTION=%s"
                 "&NETDATA_HOST_IS_K8S_NODE=%s"
                 "&NETDATA_SYSTEM_KERNEL_NAME=%s"
                 "&NETDATA_SYSTEM_KERNEL_VERSION=%s"
                 "&NETDATA_SYSTEM_ARCHITECTURE=%s"
                 "&NETDATA_SYSTEM_VIRTUALIZATION=%s"
                 "&NETDATA_SYSTEM_VIRT_DETECTION=%s"
                 "&NETDATA_SYSTEM_CONTAINER=%s"
                 "&NETDATA_SYSTEM_CONTAINER_DETECTION=%s"
                 "&NETDATA_CONTAINER_OS_NAME=%s"
                 "&NETDATA_CONTAINER_OS_ID=%s"
                 "&NETDATA_CONTAINER_OS_ID_LIKE=%s"
                 "&NETDATA_CONTAINER_OS_VERSION=%s"
                 "&NETDATA_CONTAINER_OS_VERSION_ID=%s"
                 "&NETDATA_CONTAINER_OS_DETECTION=%s"
                 "&NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT=%s"
                 "&NETDATA_SYSTEM_CPU_FREQ=%s"
                 "&NETDATA_SYSTEM_TOTAL_RAM=%s"
                 "&NETDATA_SYSTEM_TOTAL_DISK_SIZE=%s"
                 "&NETDATA_PROTOCOL_VERSION=%s"
                 " HTTP/1.1\r\n"
                 "User-Agent: %s/%s\r\n"
                 "Accept: */*\r\n\r\n"
                 , host->rrdpush_send_api_key
                 , host->hostname
                 , host->registry_hostname
                 , host->machine_guid
                 , default_rrd_update_every
                 , host->os
                 , host->timezone
                 , host->abbrev_timezone
                 , host->utc_offset
                 , host->system_info->hops + 1
                 , (host->tags) ? host->tags : ""
                 , STREAMING_PROTOCOL_CURRENT_VERSION
                 , se.os_name
                 , se.os_id
                 , (host->system_info->host_os_id_like) ? host->system_info->host_os_id_like : ""
                 , se.os_version
                 , (host->system_info->host_os_version_id) ? host->system_info->host_os_version_id : ""
                 , (host->system_info->host_os_detection) ? host->system_info->host_os_detection : ""
                 , (host->system_info->is_k8s_node) ? host->system_info->is_k8s_node : ""
                 , se.kernel_name
                 , se.kernel_version
                 , (host->system_info->architecture) ? host->system_info->architecture : ""
                 , (host->system_info->virtualization) ? host->system_info->virtualization : ""
                 , (host->system_info->virt_detection) ? host->system_info->virt_detection : ""
                 , (host->system_info->container) ? host->system_info->container : ""
                 , (host->system_info->container_detection) ? host->system_info->container_detection : ""
                 , (host->system_info->container_os_name) ? host->system_info->container_os_name : ""
                 , (host->system_info->container_os_id) ? host->system_info->container_os_id : ""
                 , (host->system_info->container_os_id_like) ? host->system_info->container_os_id_like : ""
                 , (host->system_info->container_os_version) ? host->system_info->container_os_version : ""
                 , (host->system_info->container_os_version_id) ? host->system_info->container_os_version_id : ""
                 , (host->system_info->container_os_detection) ? host->system_info->container_os_detection : ""
                 , (host->system_info->host_cores) ? host->system_info->host_cores : ""
                 , (host->system_info->host_cpu_freq) ? host->system_info->host_cpu_freq : ""
                 , (host->system_info->host_ram_total) ? host->system_info->host_ram_total : ""
                 , (host->system_info->host_disk_space) ? host->system_info->host_disk_space : ""
                 , STREAMING_PROTOCOL_VERSION
                 , host->program_name
                 , host->program_version
                 );
    http[eol] = 0x00;
    rrdpush_clean_encoded(&se);

#ifdef ENABLE_HTTPS
    if (!host->ssl.flags) {
        ERR_clear_error();
        SSL_set_connect_state(host->ssl.conn);
        int err = SSL_connect(host->ssl.conn);
        if (err != 1){
            err = SSL_get_error(host->ssl.conn, err);
            error("SSL cannot connect with the server:  %s ",ERR_error_string((long)SSL_get_error(host->ssl.conn,err),NULL));
            if (netdata_use_ssl_on_stream == NETDATA_SSL_FORCE) {
                rrdpush_sender_thread_close_socket(host);
                return 0;
            }else {
                host->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
            }
        }
        else {
            if (netdata_use_ssl_on_stream == NETDATA_SSL_FORCE) {
                if (netdata_validate_server == NETDATA_SSL_VALID_CERTIFICATE) {
                    if ( security_test_certificate(host->ssl.conn)) {
                        error("Closing the stream connection, because the server SSL certificate is not valid.");
                        rrdpush_sender_thread_close_socket(host);
                        return 0;
                    }
                }
            }
        }
    }
    if(send_timeout(&host->ssl,host->rrdpush_sender_socket, http, strlen(http), 0, timeout) == -1) {
#else
    if(send_timeout(host->rrdpush_sender_socket, http, strlen(http), 0, timeout) == -1) {
#endif
        error("STREAM %s [send to %s]: failed to send HTTP header to remote netdata.", host->hostname, s->connected_to);
        rrdpush_sender_thread_close_socket(host);
        return 0;
    }

    info("STREAM %s [send to %s]: waiting response from remote netdata...", host->hostname, s->connected_to);

    ssize_t received;
#ifdef ENABLE_HTTPS
    received = recv_timeout(&host->ssl,host->rrdpush_sender_socket, http, HTTP_HEADER_SIZE, 0, timeout);
    if(received == -1) {
#else
    received = recv_timeout(host->rrdpush_sender_socket, http, HTTP_HEADER_SIZE, 0, timeout);
    if(received == -1) {
#endif
        error("STREAM %s [send to %s]: remote netdata does not respond.", host->hostname, s->connected_to);
        rrdpush_sender_thread_close_socket(host);
        return 0;
    }

    http[received] = '\0';
    debug(D_STREAM, "Response to sender from far end: %s", http);
    int answer = -1;
    char *version_start = strchr(http, '=');
    int32_t version = -1;
    if(version_start) {
        version_start++;
        version = (int32_t)strtol(version_start, NULL, 10);
        answer = memcmp(http, START_STREAMING_PROMPT_VN, (size_t)(version_start - http));
        if(!answer) {
            rrdpush_set_flags_to_newest_stream(host);
        }
    } else {
        answer = memcmp(http, START_STREAMING_PROMPT_V2, strlen(START_STREAMING_PROMPT_V2));
        if(!answer) {
            version = 1;
            rrdpush_set_flags_to_newest_stream(host);
        }
        else {
            answer = memcmp(http, START_STREAMING_PROMPT, strlen(START_STREAMING_PROMPT));
            if(!answer) {
                version = 0;
                host->labels.labels_flag |= LABEL_FLAG_STOP_STREAM;
                host->labels.labels_flag &= ~LABEL_FLAG_UPDATE_STREAM;
            }
        }
    }

    if(version == -1) {
        error("STREAM %s [send to %s]: server is not replying properly (is it a netdata?).", host->hostname, s->connected_to);
        rrdpush_sender_thread_close_socket(host);
        return 0;
    }
    if (version == VERSION_GAP_FILLING && host->rrd_memory_mode == RRD_MEMORY_MODE_NONE)
    {
        version = VERSION_GAP_FILLING - 1;
        info("STREAM %s [send to %s]: dropping back to streaming-mode, memory mode none does not support replication",
             host->hostname, s->connected_to);
    }
    s->version = version;

    info("STREAM %s [send to %s]: established communication with a parent using protocol version %d - ready to send metrics..."
         , host->hostname
         , s->connected_to
         , version);

    if(sock_setnonblock(host->rrdpush_sender_socket) < 0)
        error("STREAM %s [send to %s]: cannot set non-blocking mode for socket.", host->hostname, s->connected_to);

    if(sock_enlarge_out(host->rrdpush_sender_socket) < 0)
        error("STREAM %s [send to %s]: cannot enlarge the socket buffer.", host->hostname, s->connected_to);

    debug(D_STREAM, "STREAM: Connected on fd %d...", host->rrdpush_sender_socket);

    return 1;
}

static void attempt_to_connect(struct sender_state *state)
{
    state->send_attempts = 0;

    if(rrdpush_sender_thread_connect_to_parent(state->host, state->default_port, state->timeout, state)) {
        state->last_sent_t = now_monotonic_sec();

        // reset the buffer, to properly send charts and metrics
        rrdpush_sender_thread_data_flush(state->host);

        // send from the beginning
        state->begin = 0;

        // make sure the next reconnection will be immediate
        state->not_connected_loops = 0;

        // reset the bytes we have sent for this session
        state->sent_bytes_on_this_connection = 0;

        // let the data collection threads know we are ready
        state->host->rrdpush_sender_connected = 1;
    }
    else {
        // increase the failed connections counter
        state->not_connected_loops++;

        // reset the number of bytes sent
        state->sent_bytes_on_this_connection = 0;

        // slow re-connection on repeating errors
        sleep_usec(USEC_PER_SEC * state->reconnect_delay); // seconds
    }
}

// TCP window is open and we have data to transmit.
void attempt_to_send(struct sender_state *s) {

    rrdpush_send_labels(s->host);

    struct circular_buffer *cb = s->buffer;

    netdata_thread_disable_cancelability();
    netdata_mutex_lock(&s->mutex);
    char *chunk;
    size_t outstanding = cbuffer_next_unsafe(s->buffer, &chunk);
    debug(D_STREAM, "STREAM: Sending data. Buffer r=%zu w=%zu s=%zu, next chunk=%zu", cb->read, cb->write, cb->size, outstanding);
    ssize_t ret;
#ifdef ENABLE_HTTPS
    SSL *conn = s->host->ssl.conn ;
    if(conn && !s->host->ssl.flags) {
        ret = SSL_write(conn, chunk, outstanding);
    } else {
        ret = send(s->host->rrdpush_sender_socket, chunk, outstanding, MSG_DONTWAIT);
    }
#else
    ret = send(s->host->rrdpush_sender_socket, chunk, outstanding, MSG_DONTWAIT);
#endif
    if (likely(ret > 0)) {
        cbuffer_remove_unsafe(s->buffer, ret);
        s->sent_bytes_on_this_connection += ret;
        s->sent_bytes += ret;
        debug(D_STREAM, "STREAM %s [send to %s]: Sent %zd bytes", s->host->hostname, s->connected_to, ret);
        s->last_sent_t = now_monotonic_sec();
    }
    else if (ret == -1 && (errno == EAGAIN || errno == EINTR || errno == EWOULDBLOCK))
        debug(D_STREAM, "STREAM %s [send to %s]: unavailable after polling POLLOUT", s->host->hostname,
              s->connected_to);
    else if (ret == -1) {
        debug(D_STREAM, "STREAM: Send failed - closing socket...");
        error("STREAM %s [send to %s]: failed to send metrics - closing connection - we have sent %zu bytes on this connection.",  s->host->hostname, s->connected_to, s->sent_bytes_on_this_connection);
        rrdpush_sender_thread_close_socket(s->host);
    }
    else {
        debug(D_STREAM, "STREAM: send() returned 0 -> no error but no transmission");
    }

    netdata_mutex_unlock(&s->mutex);
    netdata_thread_enable_cancelability();
}

static void sender_attempt_read(struct sender_state *s) {
    int ret, desired = sizeof(s->read_buffer) - s->read_len - 1;

    if (!desired) {
        debug(D_STREAM, "Cannot read any more bytes, read buffer is full, need to execute commands first.");
        return;
    }
#ifdef ENABLE_HTTPS
    if (s->host->ssl.conn && !s->host->stream_ssl.flags) {
        ERR_clear_error();
        ret = SSL_read(s->host->ssl.conn, s->read_buffer, desired);
        if (ret > 0 ) {
            s->read_len += ret;
            return;
        }
        int sslerrno = SSL_get_error(s->host->ssl.conn, desired);
        if (sslerrno == SSL_ERROR_WANT_READ || sslerrno == SSL_ERROR_WANT_WRITE)
            return;
        u_long err;
        char buf[256];
        while ((err = ERR_get_error()) != 0) {
            ERR_error_string_n(err, buf, sizeof(buf));
            error("STREAM %s [send to %s] ssl error: %s", s->host->hostname, s->connected_to, buf);
        }
        error("Restarting connection");
        rrdpush_sender_thread_close_socket(s->host);
        return;
    }
#endif
    ret = recv(s->host->rrdpush_sender_socket, s->read_buffer + s->read_len, desired, MSG_DONTWAIT);
    if (ret>0) {
        s->read_len += ret;
        return;
    }
    debug(D_STREAM, "Socket was POLLIN, but req %zu bytes gave %d", sizeof(s->read_buffer) - s->read_len - 1, ret);
    if (ret<0 && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR))
        return;
    if (ret==0)
        error("STREAM %s [send to %s]: connection closed by far end. Restarting connection", s->host->hostname, s->connected_to);
    else
        error("STREAM %s [send to %s]: error during read (%d). Restarting connection", s->host->hostname, s->connected_to,
              ret);
    rrdpush_sender_thread_close_socket(s->host);
}

static int sender_execute_replicate(struct sender_state *s, char *st_id, long start_t, long end_t);
static void sender_execute_commands(struct sender_state *s) {
    char *start = s->read_buffer, *end = &s->read_buffer[s->read_len], *newline;
    int overflow;
    *end = 0;
    //debug(D_STREAM, "%s [send to %s] received command over connection (%d-bytes): %s", s->host->hostname,
    //      s->connected_to, s->read_len, start);
    while( start<end && (newline=strchr(start, '\n')) ) {
        *newline = 0;
        if (!strncmp(start, "REPLICATE ", 10)) {
            char *next = strchr(start+10, ' ');
            if (next) {
                *next = 0;
                long start_t = strtol(next+1, &next, 10);
                if (*next == ' ') {
                    char *after;
                    long end_t = strtol(next+1, &after, 10);
                    if (after == newline) {
                        overflow = sender_execute_replicate(s, start+10, start_t, end_t);
                        start = after+1;
                        if (overflow) {
                            info("Stopped executing explicit replication commands because the send buffer is filling up.");
                            break;
                        }
                        continue;
                    }
                }
            }
            errno = 0;
            error("Malformed command on streaming link: %s", start);
            start = newline+1;
            continue;
        }
        start = newline+1;
        errno = 0;
        error("Unrecognised command received, skipping to position %ld", start - s->read_buffer);
    }
    if (start<end) {
        memmove(s->read_buffer, start, end-start);
        s->read_len = end-start;
    }
    else
        s->read_len = 0;
}


static void rrdpush_sender_thread_cleanup_callback(void *ptr) {
    RRDHOST *host = (RRDHOST *)ptr;

    netdata_mutex_lock(&host->sender->mutex);

    info("STREAM %s [send]: sending thread cleans up...", host->hostname);

    rrdpush_sender_thread_close_socket(host);

    // close the pipe
    if(host->rrdpush_sender_pipe[PIPE_READ] != -1) {
        close(host->rrdpush_sender_pipe[PIPE_READ]);
        host->rrdpush_sender_pipe[PIPE_READ] = -1;
    }

    if(host->rrdpush_sender_pipe[PIPE_WRITE] != -1) {
        close(host->rrdpush_sender_pipe[PIPE_WRITE]);
        host->rrdpush_sender_pipe[PIPE_WRITE] = -1;
    }

    if(!host->rrdpush_sender_join) {
        info("STREAM %s [send]: sending thread detaches itself.", host->hostname);
        netdata_thread_detach(netdata_thread_self());
    }

    host->rrdpush_sender_spawn = 0;

    info("STREAM %s [send]: sending thread now exits.", host->hostname);

    netdata_mutex_unlock(&host->sender->mutex);
}

void sender_init(struct sender_state *s, RRDHOST *parent) {
    memset(s, 0, sizeof(*s));
    s->host = parent;
    s->buffer = cbuffer_new(1024, 1024);
    s->build = buffer_create(1);
    netdata_mutex_init(&s->mutex);
}

void *rrdpush_sender_thread(void *ptr) {
    struct sender_state *s = ptr;
    s->task_id = gettid();

    if(!s->host->rrdpush_send_enabled || !s->host->rrdpush_send_destination ||
       !*s->host->rrdpush_send_destination || !s->host->rrdpush_send_api_key ||
       !*s->host->rrdpush_send_api_key) {
        error("STREAM %s [send]: thread created (task id %d), but host has streaming disabled.",
              s->host->hostname, s->task_id);
        return NULL;
    }

#ifdef ENABLE_HTTPS
    if (netdata_use_ssl_on_stream & NETDATA_SSL_FORCE ){
        security_start_ssl(NETDATA_SSL_CONTEXT_STREAMING);
        security_location_for_context(netdata_client_ctx, netdata_ssl_ca_file, netdata_ssl_ca_path);
    }
#endif

    info("STREAM %s [send]: thread created (task id %d)", s->host->hostname, s->task_id);

    s->timeout = (int)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "timeout seconds", 60);
    s->default_port = (int)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "default port", 19999);
    s->buffer->max_size =
        (size_t)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "buffer size bytes", 1024 * 1024);
    s->reconnect_delay =
        (unsigned int)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "reconnect delay seconds", 5);
    remote_clock_resync_iterations = (unsigned int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM,
        "initial clock resync iterations",
        remote_clock_resync_iterations); // TODO: REMOVE FOR SLEW / GAPFILLING

    // initialize rrdpush globals
    s->host->rrdpush_sender_connected = 0;
    if(pipe(s->host->rrdpush_sender_pipe) == -1) {
        error("STREAM %s [send]: cannot create required pipe. DISABLING STREAMING THREAD", s->host->hostname);
        return NULL;
    }

    enum {
        Collector,
        Socket
    };
    struct pollfd fds[2];
    fds[Collector].fd = s->host->rrdpush_sender_pipe[PIPE_READ];
    fds[Collector].events = POLLIN;

    netdata_thread_cleanup_push(rrdpush_sender_thread_cleanup_callback, s->host);
    for(; s->host->rrdpush_send_enabled && !netdata_exit ;) {
        // check for outstanding cancellation requests
        netdata_thread_testcancel();

        // The connection attempt blocks (after which we use the socket in nonblocking)
        if(unlikely(s->host->rrdpush_sender_socket == -1)) {
            s->overflow = 0;
            s->read_len = 0;
            s->buffer->read = 0;
            s->buffer->write = 0;
            attempt_to_connect(s);
            continue;
        }

        // If the TCP window never opened then something is wrong, restart connection
        if(unlikely(now_monotonic_sec() - s->last_sent_t > s->timeout)) {
            error("STREAM %s [send to %s]: could not send metrics for %d seconds - closing connection - we have sent %zu bytes on this connection via %zu send attempts.", s->host->hostname, s->connected_to, s->timeout, s->sent_bytes_on_this_connection, s->send_attempts);
            rrdpush_sender_thread_close_socket(s->host);
            continue;
        }

        // Wait until buffer opens in the socket or a rrdset_done_push wakes us
        fds[Collector].revents = 0;
        fds[Socket].revents = 0;
        fds[Socket].fd = s->host->rrdpush_sender_socket;

        netdata_mutex_lock(&s->mutex);
        char *chunk;
        size_t outstanding = cbuffer_next_unsafe(s->host->sender->buffer, &chunk);
        chunk = NULL;   // Do not cache pointer outside of region - could be invalidated
        netdata_mutex_unlock(&s->mutex);
        if(outstanding) {
            s->send_attempts++;
            fds[Socket].events = POLLIN | POLLOUT;
        }
        else {
            fds[Socket].events = POLLIN;
        }

        int retval = poll(fds, 2, 1000);
        debug(D_STREAM, "STREAM: poll() finished collector=%d socket=%d (current chunk %zu bytes)...",
              fds[Collector].revents, fds[Socket].revents, outstanding);
        if(unlikely(netdata_exit)) break;

        // Spurious wake-ups without error - loop again
        if (retval == 0 || ((retval == -1) && (errno == EAGAIN || errno == EINTR)))
        {
            debug(D_STREAM, "Spurious wakeup");
            continue;
        }
        // Only errors from poll() are internal, but try restarting the connection
        if(unlikely(retval == -1)) {
            error("STREAM %s [send to %s]: failed to poll(). Closing socket.", s->host->hostname, s->connected_to);
            rrdpush_sender_thread_close_socket(s->host);
            continue;
        }

        // If the collector woke us up then empty the pipe to remove the signal
        if (fds[Collector].revents & POLLIN || fds[Collector].revents & POLLPRI) {
            debug(D_STREAM, "STREAM: Data added to send buffer (current buffer chunk %zu bytes)...", outstanding);

            char buffer[1000 + 1];
            if (read(s->host->rrdpush_sender_pipe[PIPE_READ], buffer, 1000) == -1)
                error("STREAM %s [send to %s]: cannot read from internal pipe.", s->host->hostname, s->connected_to);
        }

        // Read as much as possible to fill the buffer, split into full lines for execution.
        if (fds[Socket].revents & POLLIN)
            sender_attempt_read(s);
        sender_execute_commands(s);

        // If we have data and have seen the TCP window open then try to close it by a transmission.
        if (outstanding && fds[Socket].revents & POLLOUT)
            attempt_to_send(s);

        // TODO-GAPS - why do we only check this on the socket, not the pipe?
        if (outstanding) {
            char *error = NULL;
            if (unlikely(fds[Socket].revents & POLLERR))
                error = "socket reports errors (POLLERR)";
            else if (unlikely(fds[Socket].revents & POLLHUP))
                error = "connection closed by remote end (POLLHUP)";
            else if (unlikely(fds[Socket].revents & POLLNVAL))
                error = "connection is invalid (POLLNVAL)";
            if(unlikely(error)) {
                error("STREAM %s [send to %s]: restart stream because %s - %zu bytes transmitted.", s->host->hostname,
                      s->connected_to, error, s->sent_bytes_on_this_connection);
                rrdpush_sender_thread_close_socket(s->host);
            }
        }

        // protection from overflow
        if (s->overflow) {
            errno = 0;
            error("STREAM %s [send to %s]: buffer full (%zu-bytes) after %zu bytes. Restarting connection",
                  s->host->hostname, s->connected_to, s->buffer->size, s->sent_bytes_on_this_connection);
            rrdpush_sender_thread_close_socket(s->host);
        }
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

extern time_t default_rrdpush_gap_block_size;
/* start_time is set during an explicit replication request from the far end, or zero if we are pushing the latest
   data from the collector.
*/
static void sender_fill_gap_nolock(struct sender_state *s, RRDSET *st, time_t start_time)
{
    UNUSED(s);
    RRDDIM *rd;
    struct rrddim_query_handle handle;

    // When the sync is unknown against the far end (last_sent=0), sending the latest sample will trigger a
    // replication from the far end if necessary.
    time_t first_t = rrdset_first_entry_t(st);
    time_t st_newest = st->last_updated.tv_sec;
    time_t window_start;

    if (start_time == 0) {
        if (st->state->last_sent.tv_sec)
            window_start = MAX((time_t)st->state->last_sent.tv_sec + st->update_every, first_t);
        else // It is the first time this agent streams this chart during its uptime
            window_start = st_newest;
    }
    else
        window_start = MAX(start_time, first_t);

    // Decide how much data to replicate from the beginning of the window
    time_t unsent_points = (st_newest - window_start) / st->update_every + 1;
    if (unsent_points > default_rrdpush_gap_block_size)
        unsent_points = default_rrdpush_gap_block_size;
    time_t window_end = window_start + unsent_points * st->update_every;    // window_end <= st_newest + update_every

    // If we are responding to an explicit request then we may send empty windows, these communicate to the far end
    // that there is no data in the interval. There are times in the REPBEGIN:
    //   * The beginning of the window in time (e.g. where we are reporting from including gaps)
    //   * The beginning of the data in time   (e.g. dropping leading gaps from the window)
    //   * The end of the window.
    // The start times are inclusive and the end time is exclusive, so if the requested window is 10..20 and the
    // chart contains data at times 15,16,17,18 then the response will be REPBEGIN 10 15 19
    if (start_time == 0)
        buffer_sprintf(s->build, "REPBEGIN \"%s\" %ld %ld %ld\n", st->id, window_start, window_start, window_end);
    else
        buffer_sprintf(s->build, "REPBEGIN \"%s\" %ld %ld %ld\n", st->id, start_time, window_start, window_end);

    rrdset_dump_debug_state(st);

    size_t num_points = 0;
    rrddim_foreach_read(rd, st) {
        // Send the intersection of this dimension and the time-window on the chart
        if (!rd->exposed)
            continue;
        time_t rd_start = rrddim_first_entry_t(rd);
        time_t rd_end   = rrddim_last_entry_t(rd) + st->update_every;
        if (rd_start < window_end && rd_end >= window_start) {

            time_t rd_oldest = MAX(rd_start, window_start);
            rd_end = MIN(rd_end,   window_end);

            rd->state->query_ops.init(rd, &handle, rd_oldest, rd_end);
            debug(D_REPLICATION, "Fill replication with %s.%s window=%ld-%ld data=%ld-%ld query=%ld-%ld",
                  st->id, rd->id, window_start, window_end, rd_oldest, rd_end, handle.start_time, handle.end_time);

            for (time_t metric_t = rd_oldest; metric_t < rd_end; ) {

                if (rd->state->query_ops.is_finished(&handle)) {
                    debug(D_REPLICATION, "%s.%s query handle finished early @%ld", st->id, rd->id, metric_t);
                    break;
                }

                storage_number n = rd->state->query_ops.next_metric(&handle, &metric_t);
                if (n == SN_EMPTY_SLOT)
                    debug(D_REPLICATION, "%s.%s db empty in valid dimension range @ %ld", st->id, rd->id, metric_t);
                else
                    buffer_sprintf(s->build, "REPDIM \"%s\" %ld " STORAGE_NUMBER_FORMAT "\n", rd->id, metric_t, n);
                debug(D_REPLICATION, "%s.%s REPDIM %ld " STORAGE_NUMBER_FORMAT "\n", st->id, rd->id, metric_t, n);
                num_points++;
            }
            rd->state->query_ops.finalize(&handle);
        }
        else
            debug(D_REPLICATION, "%s.%s has no data in the replication window (@%ld-%ld) last_collected=%ld.%ld",
                                 st->id, rd->id, (long)window_start, window_end, (long)rd->last_collected_time.tv_sec,
                                 rd->last_collected_time.tv_usec);

    }
    buffer_sprintf(s->build, "REPEND %zu %lld %lld\n", num_points, st->collected_total, st->last_collected_total);
    st->state->last_sent.tv_sec = window_end - st->update_every;
}

// Call-site for the collector thread (from inside rrdset_done)
void sender_replicate(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    if (!host->rrdpush_sender_connected || host->sender->version < VERSION_GAP_FILLING)
        return;
    if(unlikely(!should_send_chart_matching(st)))
        return;

    sender_start(host->sender);         // Locks the sender buffer
    if(need_to_send_chart_definition(st))
        rrdpush_send_chart_definition_nolock(st);
    sender_fill_gap_nolock(host->sender, st, 0);
    sender_commit(host->sender);        // Releases the sender buffer

    // signal the sender there are more data
    if(host->rrdpush_sender_pipe[PIPE_WRITE] != -1 && write(host->rrdpush_sender_pipe[PIPE_WRITE], " ", 1) == -1)
        error("STREAM %s [send]: cannot write to internal pipe", host->hostname);
}

// Call-site for the sender thread (for explict requests)
// Returns 1 if the send buffer would fill up (>= 50%) or overflow, 0 otherwise
// Allow 50% of the send buffer to be available for commands that are not allowed to fail
static int sender_execute_replicate(struct sender_state *s, char *st_id, long start_t, long end_t) {
    int overflow = 0;
    time_t now = now_realtime_sec();
    debug(D_REPLICATION, "Replication request started: %s %ld - %ld @ %ld", st_id, (long)start_t, (long)end_t, (long)now);
    RRDSET *st = rrdset_find(s->host, st_id);
    if (!st) {
        errno = 0;
        error("Cannot replicate chart %s @ %ld - not found! (req. window %ld-%ld)", st_id, now, start_t, end_t);
    }
    else {
        rrdset_rdlock(st);
        sender_start(s);                    // Locks the sender buffer
        sender_fill_gap_nolock(s, st, start_t);
        overflow = sender_commit_no_overflow(s); // Releases the sender buffer
        rrdset_unlock(st);
    }
    return overflow;
}
