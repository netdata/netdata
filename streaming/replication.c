#include "rrdpush.h"
#include "collectors/plugins.d/pluginsd_parser.h"

// TODO: fix the header file to look nice and clean
extern void rrdpush_encode_variable(stream_encoded_t *se, RRDHOST *host);
extern void rrdpush_clean_encoded(stream_encoded_t *se);
size_t replication_parser(REPLICATION_STATE *rpt, struct plugind *cd, FILE *fp);

static void replication_receiver_thread_cleanup_callback(void *ptr);
static void replication_sender_thread_cleanup_callback(void *ptr);
static inline void replication_send_chart_definition_nolock(RRDSET *st);
GAP* add_gap_data(GAPS *host_queue, GAP *gap);

/********************************
* Thread Initialization functions
*********************************/ 
static void replication_state_init(REPLICATION_STATE *state)
{
    debug(D_REPLICATION, "%s: Replication State Initialization", REPLICATION_MSG);
    memset(state, 0, sizeof(*state));
    state->buffer = cbuffer_new(1024, 1024*1024);
    state->build = buffer_create(1);
    state->socket = -1;
    state->dim_past_data = (RRDDIM_PAST_DATA *)callocz(1, sizeof(RRDDIM_PAST_DATA));
    state->dim_past_data->page = (void *)callocz(MEM_PAGE_BLOCK_SIZE, sizeof(char));
    netdata_mutex_init(&state->mutex);
}

void replication_state_destroy(REPLICATION_STATE **state)
{
    REPLICATION_STATE *r = *state;
    pthread_mutex_destroy(&r->mutex);
    if(r->buffer)
        cbuffer_free(r->buffer);
    buffer_free(r->build);
#ifdef ENABLE_HTTPS
    if(r->ssl.conn){
        SSL_free(r->ssl.conn);
    }
#endif
    if(r->dim_past_data && r->dim_past_data->page)
        freez(r->dim_past_data->page);
    freez(r->dim_past_data);
    freez(*state);
    debug(D_REPLICATION, "%s: Replication state destroyed.", REPLICATION_MSG);
}

void replication_sender_init(RRDHOST *host){
    if(!host || !host->replication){
        error("%s: Host or host's replication state is not initialized! - Replication Tx thread Initialization failed!", REPLICATION_MSG);
        return;
    }
    host->replication->tx_replication = (REPLICATION_STATE *)callocz(1, sizeof(REPLICATION_STATE));
    replication_state_init(host->replication->tx_replication);
    host->replication->tx_replication->host = host;
    host->replication->tx_replication->enabled = default_rrdpush_sender_replication_enabled;
#ifdef ENABLE_HTTPS
    host->replication->tx_replication->ssl.conn = NULL;
    host->replication->tx_replication->ssl.flags = NETDATA_SSL_START;
#endif
    info("%s: Initialize Replication Tx thread state at host creation %s .", REPLICATION_MSG, host->hostname);
}

static unsigned int replication_rd_config(RRDHOST *host, struct config *stream_config, char *key)
{
    REPLICATION_STATE *rep_state = host->replication->rx_replication;
    debug(D_REPLICATION, "%s: Reading configuration for Replication Rx thread for host %s ", REPLICATION_MSG, host->hostname);
    
    //TODO: Add any missing configuration.
    rep_state->timeout = (int)appconfig_get_number(stream_config, CONFIG_SECTION_STREAM, "timeout seconds", 60);
    rep_state->default_port = (int)appconfig_get_number(stream_config, CONFIG_SECTION_STREAM, "default port", 19999);    

    unsigned int rrdpush_receiver_replication_enable = default_rrdpush_receiver_replication_enabled;
    rrdpush_receiver_replication_enable = appconfig_get_boolean(stream_config, key, "enable replication", rrdpush_receiver_replication_enable);
    rrdpush_receiver_replication_enable = appconfig_get_boolean(stream_config, host->machine_guid, "enable replication", rrdpush_receiver_replication_enable);
    
    RRD_MEMORY_MODE mode = host->rrd_memory_mode;
    // mode = rrd_memory_mode_id(appconfig_get(stream_config, key, "default memory mode", rrd_memory_mode_name(mode)));
    // mode = rrd_memory_mode_id(appconfig_get(stream_config, host->machine_guid, "default memory mode", rrd_memory_mode_name(mode)));
    if(mode != RRD_MEMORY_MODE_DBENGINE)
    {
        infoerr("%s: Could not initialize Rx replication thread. Memory mode %s is not supported for replication!", REPLICATION_MSG, rrd_memory_mode_name(mode));
        rrdpush_receiver_replication_enable = 0;
    }

    debug(D_REPLICATION, "%s: Replication receiver is %u ", REPLICATION_MSG, rrdpush_receiver_replication_enable);
    return rrdpush_receiver_replication_enable;
}

void replication_receiver_init(RRDHOST *image_host, struct config *stream_config, char *key)
{
    image_host->replication->rx_replication = (REPLICATION_STATE *)callocz(1, sizeof(REPLICATION_STATE));
    replication_state_init(image_host->replication->rx_replication);
    unsigned int rrdpush_replication_enable = replication_rd_config(image_host, stream_config, key);
    if(!rrdpush_replication_enable)
    {
        infoerr("%s: Could not initialize Rx replication thread. Replication is disabled!", REPLICATION_MSG);
        image_host->replication->rx_replication->enabled = rrdpush_replication_enable; 
        return;
    }
    image_host->replication->rx_replication->enabled = rrdpush_replication_enable; 
    info("%s: Initialize Replication Rx thread state for replicated host %s ", REPLICATION_MSG, image_host->hostname);
}

/**************************************************
* Connection management & socket handling functions
***************************************************/ 
static void replication_thread_close_socket(REPLICATION_STATE *rep_state) {
    rep_state->connected = 0;

    if(rep_state->socket != -1) {
        close(rep_state->socket);
        rep_state->socket = -1;
    }
}

static int replication_sender_thread_connect_to_parent(RRDHOST *host, int default_port, int timeout, REPLICATION_STATE *rep_state) {

    struct timeval tv = {
            .tv_sec = timeout,
            .tv_usec = 0
    };

    replication_thread_close_socket(rep_state);

    debug(D_REPLICATION, "%s: Attempting to connect...", REPLICATION_MSG);
    info("%s %s [send to %s]: connecting...", REPLICATION_MSG, host->hostname, host->rrdpush_send_destination);

    rep_state->socket = connect_to_one_of(
            host->rrdpush_send_destination
            , default_port
            , &tv
            , &rep_state->reconnects_counter
            , rep_state->connected_to
            , sizeof(rep_state->connected_to)-1
    );

    if(unlikely(rep_state->socket == -1)) {
        error("%s %s [send to %s]: failed to connect", REPLICATION_MSG, host->hostname, host->rrdpush_send_destination);
        return 0;
    }

    info("%s %s [send to %s]: initializing communication...", REPLICATION_MSG, host->hostname, rep_state->connected_to);

#ifdef ENABLE_HTTPS
    if( netdata_replication_client_ctx ){
        rep_state->ssl.flags = NETDATA_SSL_START;
        if (!rep_state->ssl.conn){
            rep_state->ssl.conn = SSL_new(netdata_replication_client_ctx);
            if(!rep_state->ssl.conn){
                error("Failed to allocate SSL structure.");
                rep_state->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
            }
        }
        else{
            SSL_clear(rep_state->ssl.conn);
        }

        if (rep_state->ssl.conn)
        {
            if (SSL_set_fd(rep_state->ssl.conn, rep_state->socket) != 1) {
                error("Failed to set the socket to the SSL on socket fd %d.", rep_state->socket);
                rep_state->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
            } else{
                rep_state->ssl.flags = NETDATA_SSL_HANDSHAKE_COMPLETE;
            }
        }
    }
    else {
        rep_state->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
    }
#endif

    stream_encoded_t se;
    rrdpush_encode_variable(&se, host);
    char http[HTTP_HEADER_SIZE + 1];
    int eol = snprintfz(http, HTTP_HEADER_SIZE,
            "%s "
            "key=%s"
            "&hostname=%s"
            "&registry_hostname=%s"
            "&machine_guid=%s"
            "&update_every=%d"
            "&timezone=%s"
            "&abbrev_timezone=%s"
            "&utc_offset=%d"
            "&hops=%d"
            "&tags=%s"
            "&ver=%u"
            "&NETDATA_PROTOCOL_VERSION=%s"
            " HTTP/1.1\r\n"
            "User-Agent: %s/%s\r\n"
            "Accept: */*\r\n\r\n"
            , REPLICATE_CMD
            , host->rrdpush_send_api_key
            , host->hostname
            , host->registry_hostname
            , host->machine_guid
            , default_rrd_update_every
            , host->timezone
            , host->abbrev_timezone
            , host->utc_offset
            , host->system_info->hops + 1
            , (host->tags) ? host->tags : ""
            , STREAMING_PROTOCOL_CURRENT_VERSION
            , STREAMING_PROTOCOL_VERSION
            , host->program_name
            , host->program_version);
    http[eol] = 0x00;
    rrdpush_clean_encoded(&se);


#ifdef ENABLE_HTTPS
    if (!rep_state->ssl.flags) {
        ERR_clear_error();
        SSL_set_connect_state(rep_state->ssl.conn);
        int err = SSL_connect(rep_state->ssl.conn);
        if (err != 1){
            err = SSL_get_error(rep_state->ssl.conn, err);
            error("SSL cannot connect with the server:  %s ",ERR_error_string((long)SSL_get_error(rep_state->ssl.conn,err),NULL));
            if (netdata_use_ssl_on_replication == NETDATA_SSL_FORCE) {
                replication_thread_close_socket(rep_state);
                return 0;
            }else {
                rep_state->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
            }
        }
        else {
            if (netdata_use_ssl_on_replication == NETDATA_SSL_FORCE) {
                if (netdata_validate_server == NETDATA_SSL_VALID_CERTIFICATE) {
                    if ( security_test_certificate(rep_state->ssl.conn)) {
                        error("Closing the replication stream connection, because the server SSL certificate is not valid.");
                        replication_thread_close_socket(rep_state);
                        return 0;
                    }
                }
            }
        }
    }
    if(send_timeout(&rep_state->ssl, rep_state->socket, http, strlen(http), 0, timeout) == -1) {
#else
    if(send_timeout(rep_state->socket, http, strlen(http), 0, timeout) == -1) {
#endif
        error("%s %s [send to %s]: failed to send HTTP header to remote netdata.", REPLICATION_MSG, host->hostname, rep_state->connected_to);
        replication_thread_close_socket(rep_state);
        return 0;
    }

    info("%s %s [send to %s]: waiting response from remote netdata...", REPLICATION_MSG, host->hostname, rep_state->connected_to);

    ssize_t received;
#ifdef ENABLE_HTTPS
    received = recv_timeout(&rep_state->ssl, rep_state->socket, http, HTTP_HEADER_SIZE, 0, timeout);
    if(received == -1) {
#else
    received = recv_timeout(rep_state->socket, http, HTTP_HEADER_SIZE, 0, timeout);
    if(received == -1) {
#endif
        error("%s %s [send to %s]: remote netdata does not respond.", REPLICATION_MSG, host->hostname, rep_state->connected_to);
        replication_thread_close_socket(rep_state);
        return 0;
    }

    http[received] = '\0';
    debug(D_REPLICATION, "%s: Response to sender from far end: %s", REPLICATION_MSG, http);

    if(unlikely(memcmp(http, REP_ACK_CMD, (size_t)strlen(REP_ACK_CMD)))) {
        if(unlikely(memcmp(http, REP_OFF_CMD, (size_t)strlen(REP_OFF_CMD)))) {
            error("%s %s [send to %s]: Replication Rx thread is disabled.", REPLICATION_MSG, host->hostname, rep_state->connected_to);
            replication_thread_close_socket(rep_state);
            rep_state->enabled = 0;
            return 0;
        }
        error("%s %s [send to %s]: server is not replying properly (is it a netdata?).", REPLICATION_MSG, host->hostname, rep_state->connected_to);
        replication_thread_close_socket(rep_state);
        return 0;
    }
    rep_state->connected = 1;

    info("%s %s [send to %s]: established replication communication with a parent using protocol version %d - ready to replicate metrics..."
         , REPLICATION_MSG
         , host->hostname
         , rep_state->connected_to
         , host->sender->version);

    if(sock_setnonblock(rep_state->socket) < 0)
        error("%s %s [send to %s]: cannot set non-blocking mode for socket.", REPLICATION_MSG, host->hostname, rep_state->connected_to);

    if(sock_enlarge_out(rep_state->socket) < 0)
        error("%s %s [send to %s]: cannot enlarge the socket buffer.", REPLICATION_MSG, host->hostname, rep_state->connected_to);

    debug(D_REPLICATION, "%s: Connected on fd %d...", REPLICATION_MSG, rep_state->socket);

    return 1;
}

static void replication_sender_thread_data_flush(RRDHOST *host) {
    REPLICATION_STATE *rep_state = host->replication->tx_replication;

    netdata_mutex_lock(&rep_state->mutex);
    size_t len = cbuffer_next_unsafe(rep_state->buffer, NULL);
    if (len)
        error("%s %s [send]: discarding %zu bytes of metrics already in the buffer.", REPLICATION_MSG, host->hostname, len);

    cbuffer_remove_unsafe(rep_state->buffer, len);
    netdata_mutex_unlock(&rep_state->mutex);
}

// Higher level wrap function for connection management and socket metadata updates.
static void replication_attempt_to_connect(RRDHOST *host)
{
    REPLICATION_STATE *rep_state = host->replication->tx_replication;
    rep_state->send_attempts = 0;

    if(replication_sender_thread_connect_to_parent(host, rep_state->default_port, rep_state->timeout, rep_state)) {
        rep_state->last_sent_t = now_realtime_sec();

        // reset the buffer, to properly gaps and replicate commands
        replication_sender_thread_data_flush(host);

        // Clear the read buffer
        memset(rep_state->read_buffer, 0, sizeof(rep_state->read_buffer));
        rep_state->read_len = 0;

        // send from the beginning
        rep_state->begin = 0;

        // make sure the next reconnection will be immediate
        rep_state->not_connected_loops = 0;

        // reset the bytes we have sent for this session
        rep_state->sent_bytes_on_this_connection = 0;

        // Update the connection state flag
        rep_state->connected = 1;

        // remove the non-blocking flag from the socket
        if(sock_delnonblock(rep_state->socket) < 0)
            error("%s %s [receive from [%s]:%s]: cannot remove the non-blocking flag from socket %d", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port, rep_state->socket);

        // Set file pointer
        rep_state->fp = fdopen(rep_state->socket, "w+");
        if(!rep_state->fp) {
            log_replication_connection(rep_state->client_ip, rep_state->client_port, host->rrdpush_send_api_key, host->machine_guid, host->hostname, "SOCKET CONVERSION TO FD FAILED - SOCKET ERROR");
            error("%s %s [receive from [%s]:%s]: failed to get a FILE for FD %d.", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port, rep_state->socket);
            close(rep_state->socket);
        } 
    }
    else {
        // increase the failed connections counter
        rep_state->not_connected_loops++;

        // reset the number of bytes sent
        rep_state->sent_bytes_on_this_connection = 0;

        // slow re-connection on repeating errors
        sleep_usec(USEC_PER_SEC * rep_state->reconnect_delay); // seconds
    }
}

// Replication thread starting a transmission
void replication_start(struct replication_state *replication) {
    netdata_mutex_lock(&replication->mutex);
    buffer_flush(replication->build);
}

// Replication thread finishing a transmission
void replication_commit(struct replication_state *replication) {
    if(cbuffer_add_unsafe(replication->buffer, buffer_tostring(replication->build),
       replication->build->len))
        replication->overflow = 1;
    buffer_flush(replication->build);
    netdata_mutex_unlock(&replication->mutex);
}

void replication_attempt_read(struct replication_state *replication) {
int ret;
#ifdef ENABLE_HTTPS
    if (replication->ssl.conn && !replication->ssl.flags) {
        ERR_clear_error();
        int desired = sizeof(replication->read_buffer) - replication->read_len - 1;
        ret = SSL_read(replication->ssl.conn, replication->read_buffer, desired);
        if (ret > 0 ) {
            replication->read_len += ret;
            return;
        }
        int sslerrno = SSL_get_error(replication->ssl.conn, desired);
        if (sslerrno == SSL_ERROR_WANT_READ || sslerrno == SSL_ERROR_WANT_WRITE)
            return;
        u_long err;
        char buf[256];
        while ((err = ERR_get_error()) != 0) {
            ERR_error_string_n(err, buf, sizeof(buf));
            error("%s: Host %s [send to %s] ssl error: %s", REPLICATION_MSG, replication->host->hostname, replication->connected_to, buf);
        }
        error("%s: Restarting connection", REPLICATION_MSG);
        replication_thread_close_socket(replication);
        return;
    }
#endif
    ret = recv(replication->socket, replication->read_buffer + replication->read_len, sizeof(replication->read_buffer) - replication->read_len - 1,
               MSG_DONTWAIT);
    
    if (ret>0) {
        replication->read_len += ret;
        return;
    }
    debug(D_REPLICATION, "%s: Socket was POLLIN, but req %zu bytes gave %d", REPLICATION_MSG, sizeof(replication->read_buffer) - replication->read_len - 1, ret);
    
    if (ret<0 && (errno == EAGAIN || errno == EWOULDBLOCK || errno == EINTR))
        return;
    if (ret==0)
        error("%s: Host %s [send to %s]: connection closed by far end. Restarting connection", REPLICATION_MSG, replication->host->hostname, replication->connected_to);
    else
        error("%s: Host %s [send to %s]: error during read (%d). Restarting connection", REPLICATION_MSG, replication->host->hostname, replication->connected_to, ret);
    replication_thread_close_socket(replication);
}

void replication_attempt_to_send(struct replication_state *replication) {

#ifdef NETDATA_INTERNAL_CHECKS
    struct circular_buffer *cb = replication->buffer;
#endif

    netdata_thread_disable_cancelability();
    netdata_mutex_lock(&replication->mutex);
    char *chunk = NULL;
    size_t outstanding = cbuffer_next_unsafe(replication->buffer, &chunk);
    debug(D_REPLICATION, "%s: Sending data. Buffer r=%zu w=%zu s=%zu/max=%zu, next chunk=%zu", REPLICATION_MSG, cb->read, cb->write, cb->size, cb->max_size, outstanding);
    ssize_t ret;
#ifdef ENABLE_HTTPS
        SSL *conn = replication->ssl.conn;
        if(conn && !replication->ssl.flags) {
            ret = SSL_write(conn, chunk, outstanding);
        } else {
            ret = send(replication->socket, chunk, outstanding, 0);
        }
#else
        ret = send(replication->socket, chunk, outstanding, 0);
#endif
        if (likely(ret > 0)) {
            cbuffer_remove_unsafe(replication->buffer, ret);
            replication->sent_bytes_on_this_connection += ret;
            replication->sent_bytes += ret;
            debug(
                D_REPLICATION,
                "%s: Host %s [send to %s:%d]: Sent %zd bytes",
                REPLICATION_MSG,
                replication->host->hostname,
                replication->connected_to,
                replication->socket,
                ret);
            replication->last_sent_t = now_realtime_sec();
        } else if (ret == -1 && (errno == EAGAIN || errno == EINTR || errno == EWOULDBLOCK)) {
                error(
                    "%s: Host %s [send to %s]: Send failed with error.",
                    REPLICATION_MSG,
                    replication->host->hostname,
                    replication->connected_to);
        } else if (ret == -1) {
            error(
                "%s: Host %s [send to %s]: failed to send metrics - closing connection - we have sent %zu bytes on this connection.",
                REPLICATION_MSG,
                replication->host->hostname,
                replication->connected_to,
                replication->sent_bytes_on_this_connection);
            replication_thread_close_socket(replication);
        } else {
            debug(D_REPLICATION, "%s: send() returned 0 -> no error but no transmission", REPLICATION_MSG);
        }
    netdata_mutex_unlock(&replication->mutex);
    netdata_thread_enable_cancelability();
}

static int trigger_replication(RRDHOST *host){
    time_t t_now = now_realtime_sec();
    time_t wait_streaming = 0;
    time_t abs_wait_streaming = 0;
    if(host->sender){
        if(t_now > host->sender->t_first_exposed_chart_definition){
            wait_streaming = (t_now - host->sender->t_last_exposed_chart_definition);
            abs_wait_streaming = (host->sender->t_last_exposed_chart_definition - host->sender->t_first_exposed_chart_definition);
        }
        // info("%s: First exposed: %ld, Last exposed: %ld, diff: %ld, Now: %ld, Diff: %ld", REPLICATION_MSG, host->sender->t_first_exposed_chart_definition, host->sender->t_last_exposed_chart_definition, abs_wait_streaming, t_now, wait_streaming);
        if(wait_streaming >=0 && abs_wait_streaming && ((wait_streaming > 10) || (abs_wait_streaming + wait_streaming) > 15))
            return 1;
        return 0;
    }
    infoerr("%s: Streaming sender is NULL. Replication will not run.", REPLICATION_MSG);
    return -1;
}
/**************************************************
* Thread management functions
***************************************************/
void *replication_sender_thread(void *ptr) {
    
    RRDHOST *host = (RRDHOST *)ptr;
    REPLICATION_STATE *rep_state = host->replication->tx_replication;
    unsigned int rrdpush_replication_enabled = rep_state->enabled;
    int trigger_replication_flag = 0;
    // Pause = 1 if it is a child host (!localhost) and there are gaps for replication
    rep_state->pause = (!is_localhost(host) && host->gaps_timeline->gaps->count) ? 1 : 0;
    rep_state->resume = 0;
    rep_state->shutdown = 0;
    info("%s Starting Replication Tx thread.", REPLICATION_MSG);    

#ifdef ENABLE_HTTPS
    if (netdata_use_ssl_on_replication & NETDATA_SSL_FORCE ){
        security_start_ssl(NETDATA_SSL_CONTEXT_REPLICATION);
        security_location_for_context(netdata_replication_client_ctx, netdata_ssl_ca_file, netdata_ssl_ca_path);
    }
#endif

    // Read the config for sending in replication
    rep_state->timeout = (int)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "timeout seconds", 60);
    rep_state->default_port = (int)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "default port", 19999);

    netdata_thread_cleanup_push(replication_sender_thread_cleanup_callback, host);
    for(;rrdpush_replication_enabled && !netdata_exit;)
    {
        // check for outstanding cancellation requests
        netdata_thread_testcancel();

        // Pause Tx REP thread mechanism.
        // Waiting for a proxy to replicate GAPs with lower layer hops before
        // it starts replication for higher level hops
        if(rep_state->pause){
            sleep_usec(USEC_PER_SEC);
            continue;
        }

        if(!rep_state->connected) {
            if(!rep_state->enabled)
                break;
            replication_attempt_to_connect(host);
            rep_state->not_connected_loops++;
            rep_state->pause = 0;            
        }
        else {
            sleep_usec(200 * USEC_PER_MS);
            trigger_replication_flag = trigger_replication(host);
            if(trigger_replication_flag == 1)
            {
                send_message(rep_state, "REP 2\n"); // REP_ON
                replication_parser(rep_state, NULL, rep_state->fp);
                break;
            }
            else if (trigger_replication_flag == -1) // Holding the case of a host->sender NULL
              break;
        }
    }
    info("%s: Terminating Replication Tx thread", REPLICATION_MSG);

    netdata_thread_cleanup_pop(1);
    return NULL;
}

void replication_sender_thread_spawn(RRDHOST *host) {
    netdata_mutex_lock(&host->replication->tx_replication->mutex);
    
    if(!host->replication->tx_replication->spawned) {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "REPLICATION_SENDER[%s]", host->hostname);

        if(netdata_thread_create(&host->replication->tx_replication->thread, tag, NETDATA_THREAD_OPTION_JOINABLE, replication_sender_thread, (void *) host))
            error("%s %s [send]: failed to create new thread for client.", REPLICATION_MSG, host->hostname);
        else
            host->replication->tx_replication->spawned = 1;
    }
    netdata_mutex_unlock(&host->replication->tx_replication->mutex);
}

void send_message(REPLICATION_STATE *replication, char* message){
    debug(D_REPLICATION, "%s: Sending... [%s]", REPLICATION_MSG, message);
    replication_start(replication);
    buffer_sprintf(replication->build, "%s", message);
    replication_commit(replication);
    replication_attempt_to_send(replication);
}

void *replication_receiver_thread(void *ptr){
    RRDHOST *host = (RRDHOST *)ptr;
    REPLICATION_STATE *rep_state = host->replication->rx_replication;
    unsigned int rrdpush_replication_enabled =  rep_state->enabled;
    rep_state->exited = 0; //latch down flag on Rx thread start up.

    int try_count = 0;
    while(rep_state->host->receiver->first_msg_t == 0) {
        sleep(REPLICATION_STREAMING_WAIT_STEP); // Wait a while until stream starts receiving metrics
        try_count++;
        if(try_count > REPLICATION_STREAMING_WAIT_STEP_COUNT) {
            error("%s Failed to get first metric timestamp from streaming state.", REPLICATION_MSG);
            close(rep_state->socket);
            return 0;
        }
    }

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
    
    netdata_thread_cleanup_push(replication_receiver_thread_cleanup_callback, host);

    snprintfz(cd.id,           CONFIG_MAX_NAME,  "%s:%s", rep_state->client_ip, rep_state->client_port);
    snprintfz(cd.filename,     FILENAME_MAX,     "%s:%s", rep_state->client_ip, rep_state->client_port);
    snprintfz(cd.fullfilename, FILENAME_MAX,     "%s:%s", rep_state->client_ip, rep_state->client_port);
    snprintfz(cd.cmd,          PLUGINSD_CMD_MAX, "%s:%s", rep_state->client_ip, rep_state->client_port);    
    // Respond with the REP ack text command
    info("%s %s [receive from [%s]:%s]: initializing replication communication...", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port);
    char initial_response[HTTP_HEADER_SIZE];
    if (rep_state->stream_version >= STREAM_VERSION_GAP_FILLING && rrdpush_replication_enabled) {
        info("%s %s [receive from [%s]:%s]: Netdata acknowledged replication over stream version %u.", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port, rep_state->stream_version);
        sprintf(initial_response, "%s", REP_ACK_CMD);
    } 
    else {
        info("%s [receive from [%s]:%s]: Netdata stream protocol does not support replication.", host->hostname, rep_state->client_ip, rep_state->client_port);
        sprintf(initial_response, "%s", "REP OFF");
    }
    debug(D_REPLICATION,"%s: Initial REPLICATION response to [%s:%s]: %s", REPLICATION_MSG, rep_state->client_ip, rep_state->client_port, initial_response);
#ifdef ENABLE_HTTPS
    if(send_timeout(&rep_state->ssl, rep_state->socket, initial_response, strlen(initial_response), 0, rep_state->timeout) != (ssize_t)strlen(initial_response)) {
#else
    if(send_timeout(rep_state->socket, initial_response, strlen(initial_response), 0, 60) != strlen(initial_response)) {
#endif
        log_replication_connection(rep_state->client_ip, rep_state->client_port, rep_state->key, host->machine_guid, host->hostname, "REPLICATION CONNECTION FAILED - THIS HOST FAILED TO REPLY");
        error("%s %s [receive from [%s]:%s]: failed to send replication acknowledgement command.", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port);
        close(rep_state->socket);
        return 0;
    }
    if(!rrdpush_replication_enabled){
        error("%s %s [receive from [%s]:%s]: Replication receiving thread is not enabled. Closing...", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port);
        return 0;
    }
    // Here is the first proof of connection with the sender thread.
    rep_state->connected = 1;

    // remove the non-blocking flag from the socket
    if(sock_delnonblock(rep_state->socket) < 0)
        error("%s %s [receive from [%s]:%s]: cannot remove the non-blocking flag from socket %d", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port, rep_state->socket);

    // convert the socket to a FILE *
    FILE *fp = fdopen(rep_state->socket, "w+");
    if(!fp) {
        log_replication_connection(rep_state->client_ip, rep_state->client_port, rep_state->key, host->machine_guid, host->hostname, "SOCKET CONVERSION TO FD FAILED - SOCKET ERROR");
        error("%s %s [receive from [%s]:%s]: failed to get a FILE for FD %d.", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port, rep_state->socket);
        close(rep_state->socket);
        return 0;
    }
    
    // call the plugins.d processor to receive the metrics
    info("%s %s [receive from [%s]:%s]: filling replication gaps...", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port);
    log_replication_connection(rep_state->client_ip, rep_state->client_port, rep_state->key, host->machine_guid, host->hostname, "CONNECTED");

    cd.version = rep_state->stream_version;

    // Wait for the sender thread to send REP ON
    size_t count = replication_parser(rep_state, &cd, fp);

    // Replication completed or exitted
    replication_thread_close_socket(rep_state);
    
    log_replication_connection(rep_state->client_ip, rep_state->client_port, rep_state->key, host->machine_guid, host->hostname, "DISCONNECTED");
    debug(D_REPLICATION, "%s: %s [receive from [%s]:%s]: disconnected (completed %zu updates).", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port, count);

    info("%s: Replication Parser Finished (completed %zu updates)!", REPLICATION_MSG, count);
    fclose(fp);
    netdata_thread_cleanup_pop(1);
    return NULL;   
}

int finish_gap_replication(RRDHOST *host, REPLICATION_STATE *rep_state) {
    int num_of_queued_gaps = host->gaps_timeline->gaps->count;
    if(!num_of_queued_gaps) {
        info("%s: No more GAPs to replicate for host %s. Switch off the Replication Rx thread", REPLICATION_MSG, host->hostname);
        // Send REP OFF to terminate replication at the Tx side.
        send_message(rep_state, "REP 1\n");
        rep_state->shutdown = 1;
        // Start replication sender thread (Tx) for child image hosts. After the GAP replication. Bottom-Up approach.
        if (host->replication->tx_replication->enabled && host->replication->tx_replication->spawned) {
            info("%s: No more GAPs to replicate for host %s. Switch ON the Replication Tx thread to send the metrics to higher level hops.",
                REPLICATION_MSG,
                host->hostname);
            host->replication->tx_replication->pause = 0;
            host->replication->tx_replication->resume = 1;
        }
        return 1;
    }
    return 0;
}

void send_gap_for_replication(RRDHOST *host, REPLICATION_STATE *rep_state)
{
    GAP *the_gap = (GAP *)host->gaps_timeline->gaps->front->item;
    //  Assign the timestamp of first metric comes from streaming to avoid
    // missing metrics between the disconnection and start of the streaming
    // the_gap->t_window.t_end = rep_state->host->receiver->first_msg_t - 1;
    the_gap->t_window.t_end = rep_state->host->receiver->first_msg_t;
    char *rep_msg_cmd;
    size_t len;
    replication_gap_to_str(the_gap, &rep_msg_cmd, &len);
    send_message(rep_state, rep_msg_cmd);
    the_gap->status = "ontransmit";
}

void cleanup_after_gap_replication(GAPS *gaps_timeline)
{
    GAP *the_gap = (GAP *)queue_pop(gaps_timeline->gaps);
    if (the_gap) {
        debug(D_REPLICATION, "%s: Finishing...the replication of the GAP", REPLICATION_MSG);
        remove_gap(the_gap); // Remove it from the SQLite if it exists
        reset_gap(the_gap);  // Clean the gaps table runtime host GAPS memory
    }
}

int replication_receiver_thread_spawn(struct web_client *w, char *url) {
    info("clients wants to REPLICATE metrics.");

    char *key = NULL, *hostname = NULL, *registry_hostname = NULL, *machine_guid = NULL, *os = "unknown", *timezone = "unknown", *abbrev_timezone = "UTC", *tags = NULL;
    int32_t utc_offset = 0;
    int update_every = default_rrd_update_every;
    uint32_t stream_version = UINT_MAX;
    char buf[GUID_LEN + 1];

    //parse url or REPLICATE command arguments
    struct rrdhost_system_info *system_info = callocz(1, sizeof(struct rrdhost_system_info));
    system_info->hops = 1;
    while(url) {
        char *value = mystrsep(&url, "&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "key"))
            key = value;
        else if(!strcmp(name, "hostname"))
            hostname = value;
        else if(!strcmp(name, "registry_hostname"))
            registry_hostname = value;
        else if(!strcmp(name, "machine_guid"))
            machine_guid = value;
        else if(!strcmp(name, "update_every"))
            update_every = (int)strtoul(value, NULL, 0);
        else if(!strcmp(name, "timezone"))
            timezone = value;
        else if(!strcmp(name, "abbrev_timezone"))
            abbrev_timezone = value;
        else if(!strcmp(name, "utc_offset"))
            utc_offset = (int32_t)strtol(value, NULL, 0);
        else if(!strcmp(name, "hops"))
            system_info->hops = (uint16_t) strtoul(value, NULL, 0);
        else if(!strcmp(name, "tags"))
            tags = value;
        else if(!strcmp(name, "ver")) {
            stream_version = negotiating_stream_version(STREAMING_PROTOCOL_CURRENT_VERSION, (uint32_t) strtoul(value, NULL, 0));
            info("REPLICATION [decided version is %u]", stream_version);
        }
        else {
            // An old Netdata child does not have a compatible streaming protocol, map to something sane.
            if(!strcmp(name, "NETDATA_PROTOCOL_VERSION") && stream_version == UINT_MAX) {
                stream_version = 1;
            }

            if (unlikely(rrdhost_set_system_info_variable(system_info, name, value))) {
                infoerr("%s [receive from [%s]:%s]: request has parameter '%s' = '%s', which is not used.", REPLICATION_MSG, w->client_ip, w->client_port, name, value);
            }
        }
    }

    //  Verify URL parameters - Replication arg before the creation of the receiving thread
    if (stream_version == UINT_MAX)
        stream_version = 0;

    if(!key || !*key) {
        rrdhost_system_info_free(system_info);
        log_replication_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO KEY");
        error("%s [receive from [%s]:%s]: Replicate request without an API key. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!hostname || !*hostname) {
        rrdhost_system_info_free(system_info);
        log_replication_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO HOSTNAME");
        error("%s [receive from [%s]:%s]: Replicate request without a hostname. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!machine_guid || !*machine_guid) {
        rrdhost_system_info_free(system_info);
        log_replication_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO MACHINE GUID");
        error("%s [receive from [%s]:%s]: Replicate request without a machine GUID. Forbidding access.",REPLICATION_MSG, w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(regenerate_guid(key, buf) == -1) {
        rrdhost_system_info_free(system_info);
        log_replication_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - INVALID KEY");
        error("%s [receive from [%s]:%s]: API key '%s' is not valid GUID (use the command uuidgen to generate one). Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, key);
        return rrdpush_receiver_permission_denied(w);
    }

    if(regenerate_guid(machine_guid, buf) == -1) {
        rrdhost_system_info_free(system_info);
        log_replication_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - INVALID MACHINE GUID");
        error("%s [receive from [%s]:%s]: machine GUID '%s' is not GUID. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, machine_guid);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!appconfig_get_boolean(&stream_config, key, "enabled", 0)) {
        rrdhost_system_info_free(system_info);
        log_replication_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - KEY NOT ENABLED");
        error("%s [receive from [%s]:%s]: API key '%s' is not allowed. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, key);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *key_allow_from = simple_pattern_create(appconfig_get(&stream_config, key, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT);
        if(key_allow_from) {
            if(!simple_pattern_matches(key_allow_from, w->client_ip)) {
                simple_pattern_free(key_allow_from);
                rrdhost_system_info_free(system_info);
                log_replication_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname) ? hostname : "-", "ACCESS DENIED - KEY NOT ALLOWED FROM THIS IP");
                error("%s [receive from [%s]:%s]: API key '%s' is not permitted from this IP. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, key);
                return rrdpush_receiver_permission_denied(w);
            }
            simple_pattern_free(key_allow_from);
        }
    }

    if(!appconfig_get_boolean(&stream_config, machine_guid, "enabled", 1)) {
        rrdhost_system_info_free(system_info);
        log_replication_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - MACHINE GUID NOT ENABLED");
        error("%s [receive from [%s]:%s]: machine GUID '%s' is not allowed. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, machine_guid);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *machine_allow_from = simple_pattern_create(appconfig_get(&stream_config, machine_guid, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT);
        if(machine_allow_from) {
            if(!simple_pattern_matches(machine_allow_from, w->client_ip)) {
                simple_pattern_free(machine_allow_from);
                rrdhost_system_info_free(system_info);
                log_replication_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname) ? hostname : "-", "ACCESS DENIED - MACHINE GUID NOT ALLOWED FROM THIS IP");
                error("%s [receive from [%s]:%s]: Machine GUID '%s' is not permitted from this IP. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, machine_guid);
                return rrdpush_receiver_permission_denied(w);
            }
            simple_pattern_free(machine_allow_from);
        }
    }
    //TODO: Remove the redundant parameters
    UNUSED(update_every);
    UNUSED(abbrev_timezone);
    UNUSED(tags);
    UNUSED(timezone);
    UNUSED(os);                
    UNUSED(utc_offset);
    UNUSED(registry_hostname);                 

    // Replication request rate limit control
    if(unlikely(web_client_replication_rate_t > 0)) {
        static netdata_mutex_t replication_rate_mutex = NETDATA_MUTEX_INITIALIZER;
        static volatile time_t last_replication_accepted_t = 0;

        netdata_mutex_lock(&replication_rate_mutex);
        time_t now = now_realtime_sec();

        if(unlikely(last_replication_accepted_t == 0))
            last_replication_accepted_t = now;

        if(now - last_replication_accepted_t < web_client_replication_rate_t) {
            netdata_mutex_unlock(&replication_rate_mutex);
            rrdhost_system_info_free(system_info);
            error("%s [receive from [%s]:%s]: too busy to accept new streaming request. Will be allowed in %ld secs.", REPLICATION_MSG, w->client_ip, w->client_port, (long)(web_client_replication_rate_t - (now - last_replication_accepted_t)));
            return rrdpush_receiver_too_busy_now(w);
        }

        last_replication_accepted_t = now;
        netdata_mutex_unlock(&replication_rate_mutex);
    }

    // TBR - Comment
    // What it does: if host doesn't exist prepare the receiver state struct
    // and start the streaming receiver to create it.
    // What I want:  If the host doesn't exist I should depend on streaming to create it.
    // At this point, streaming should have already call the receiver thread to create the host.
    // So if the host exists we continue with the call to the replication rx thread.
    // If the host doesn't exist and host->receiver is NULL means that there was a problem
    // with host creation during streaming or the REPLICATE command arrived earlier than the 
    // respective STREAM command. So do not start the Rx replication thread.
    // The replication Tx thread in child should try to reconnect.

    rrd_rdlock();
    RRDHOST *host = rrdhost_find_by_guid(machine_guid, 0);
    if (unlikely(host && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) /* Ignore archived hosts. */
        host = NULL;
    if(!host) {
            rrd_unlock();
            log_replication_connection(w->client_ip, w->client_port, key, machine_guid, hostname, "ABORT REPLICATION - HOST DOES NOT EXIST");
            infoerr("%s - [received from [%s]:%s]: Host(%s) with machine GUID %s does not exist - Abort replication.", REPLICATION_MSG, w->client_ip, w->client_port, hostname, machine_guid);
            return 409;
    }
    // Chase race condition in case of two REPLICATE requests hit the web server. One should start the receiver replication thread
    // and the other should be rejected.
    rrdhost_wrlock(host);
    if (host->replication->rx_replication != NULL && host->replication->rx_replication->spawned) {
        time_t age = now_realtime_sec() - host->replication->rx_replication->last_msg_t;
        rrdhost_unlock(host);
        rrd_unlock();
        log_replication_connection(w->client_ip, w->client_port, key, host->machine_guid, host->hostname, "REJECTED - ALREADY CONNECTED");
        info("%s %s [receive from [%s]:%s]: multiple connections for same host detected - existing connection is active (within last %ld sec), rejecting new connection.", REPLICATION_MSG, host->hostname, w->client_ip, w->client_port, age);
        // Have not set WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET - caller should clean up
        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "This GUID is already replicating to this server");
        return 409;
    }
    rrdhost_unlock(host);
    rrd_unlock();

    // Host exists and replication is not active
    // Initialize replication receiver structure.
    replication_receiver_init(host, &stream_config, key);
    host->replication->rx_replication->host = host;
    host->replication->rx_replication->last_msg_t = now_realtime_sec();
    host->replication->rx_replication->socket = w->ifd;
    host->replication->rx_replication->client_ip = strdupz(w->client_ip);
    host->replication->rx_replication->client_port = strdupz(w->client_port);
    host->replication->rx_replication->key = strdupz(key);
    host->replication->rx_replication->stream_version = stream_version;
    host->replication->rx_replication->connected = 1;
#ifdef ENABLE_HTTPS
    host->replication->rx_replication->ssl.conn = w->ssl.conn;
    host->replication->rx_replication->ssl.flags = w->ssl.flags;
    w->ssl.conn = NULL;
    w->ssl.flags = NETDATA_SSL_START;
#endif

    if(w->user_agent && w->user_agent[0]) {
        char *t = strchr(w->user_agent, '/');
        if(t && *t) {
            *t = '\0';
            t++;
        }

        host->replication->rx_replication->program_name = strdupz(w->user_agent);
        if(t && *t) host->replication->rx_replication->program_version = strdupz(t);
    }

    info("%s: Starting Rx Replication thread.", REPLICATION_MSG);

    char tag[FILENAME_MAX + 1];
    snprintfz(tag, FILENAME_MAX, "REPLICATION_RECEIVER[%s,[%s]:%s]", host->hostname, w->client_ip, w->client_port);

    if(netdata_thread_create(&host->replication->rx_replication->thread, tag, NETDATA_THREAD_OPTION_DEFAULT, replication_receiver_thread, (void *)host))
        error("Failed to create new REPLICATE receive thread for client.");
    else
        host->replication->rx_replication->spawned = 1;

    // prevent the caller from closing the streaming socket
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
    return 200;
}

// Thread clean-up & destroy
static void replication_sender_thread_cleanup_callback(void *ptr) {
    RRDHOST *host = (RRDHOST *)ptr;
    REPLICATION_STATE *rep_state = host->replication->tx_replication;

    netdata_mutex_lock(&rep_state->mutex);
    info("%s: %s [send]: sending thread cleans up...", REPLICATION_MSG, host->hostname);

    replication_thread_close_socket(rep_state);

    if(!rep_state->sender_thread_join) {
        info("%s: %s [send]: sending thread detaches itself.", REPLICATION_MSG, host->hostname);
        netdata_thread_detach(netdata_thread_self());
    }
    rep_state->spawned = 0;
    info("%s: %s [send]: sending thread now exits.", REPLICATION_MSG, host->hostname);
    netdata_mutex_unlock(&host->replication->tx_replication->mutex);
}

void replication_receiver_thread_cleanup_callback(void *ptr)
{
    info("%s: Cleaning up the Replication Rx thread...", REPLICATION_MSG);

    static __thread int executed = 0;
    if(!executed) {
        executed = 1;
        RRDHOST *host = (RRDHOST *)ptr;
        REPLICATION_STATE *rep_state = host->replication->rx_replication;

        if (netdata_exit && rep_state) {
            info("%s: Replication Rx thread cleanup: Shutdown sequence has started...", REPLICATION_MSG);
            rep_state->exited = 1;
            return;
        }

        if (!netdata_exit && rep_state) {
            info("%s %s [receive from [%s]:%s]: Replication Rx thread ended (task id %d)", REPLICATION_MSG, host->hostname, rep_state->client_ip, rep_state->client_port, gettid());
            replication_state_destroy(&rep_state);
            host->replication->rx_replication = NULL;
        }
    }
}

void replication_sender_thread_stop(RRDHOST *host) {
    REPLICATION_STATE *rep_state = host->replication->tx_replication;

    netdata_mutex_lock(&rep_state->mutex);
    netdata_thread_t thr = 0;

    if(rep_state->spawned) {
        info("%s %s [send]: signaling Replication Tx thread to stop...", REPLICATION_MSG, host->hostname);

        rep_state->sender_thread_join = 1;
        thr = rep_state->thread;
        netdata_thread_cancel(rep_state->thread);
    }

    netdata_mutex_unlock(&rep_state->mutex);

    if(thr != 0) {
        info("%s %s [send]: waiting for the Replication Tx thread to stop...", REPLICATION_MSG, host->hostname);
        void *result;
        netdata_thread_join(thr, &result);
        info("%s %s [send]: Replication Tx thread has exited.", REPLICATION_MSG, host->hostname);
    }
    replication_state_destroy(&rep_state);
}

/******************************
* DBENGINE RRDDIM_PAST_DATA ops
********************************/
void replication_collect_past_metric_init(REPLICATION_STATE *rep_state, char *rrdset_id, char *rrddim_id) {

    RRDDIM_PAST_DATA *dim_past_data = rep_state->dim_past_data;
    if(dim_past_data->page){
        memset(dim_past_data->page, 0, MEM_PAGE_BLOCK_SIZE);
        dim_past_data->page_length = 0;
        dim_past_data->start_time = 0;
        dim_past_data->end_time = 0;
        dim_past_data->rd = NULL;
    }
    dim_past_data->rrdset_id = strdupz(rrdset_id);
    dim_past_data->rrddim_id = strdupz(rrddim_id);

    RRDSET *st = rrdset_find(rep_state->host, dim_past_data->rrdset_id);
    if(unlikely(!st)) {
        error("Cannot find chart with name_id '%s' on host '%s'.", dim_past_data->rrdset_id, rep_state->host->hostname);
        return;
    }
    dim_past_data->rd = rrddim_find(st, dim_past_data->rrddim_id);
    if(unlikely(!dim_past_data->rd)) {
        error("Cannot find dimension with id '%s' in chart '%s' on host '%s'.", dim_past_data->rrddim_id, dim_past_data->rrdset_id, rep_state->host->hostname);
        return;
    }    
    debug(D_REPLICATION, "%s: Initializaton for collecting past data of dimension \"%s\".\"%s\"\n", REPLICATION_MSG, rrdset_id, rrddim_id);
}

void replication_collect_past_metric(REPLICATION_STATE *rep_state, time_t timestamp, storage_number number) {
    storage_number *page = rep_state->dim_past_data->page;
    uint32_t page_length = rep_state->dim_past_data->page_length;
    if(!rep_state->dim_past_data->rd){
        infoerr("%s: Collect past metric: Dimension not found in the host", REPLICATION_MSG);
        return;
    }
    time_t update_every = rep_state->dim_past_data->rd->update_every;
    if(!page_length)
        rep_state->dim_past_data->start_time = timestamp * USEC_PER_SEC;
    if((page_length + sizeof(number)) < MEM_PAGE_BLOCK_SIZE){
        if(page_length && rep_state->dim_past_data->rd){
            time_t current_end_time = rep_state->dim_past_data->end_time / USEC_PER_SEC;
            time_t t_sample_diff  = (timestamp -  current_end_time);
            if(t_sample_diff > update_every){
                page_length += ((t_sample_diff - update_every)*sizeof(number));
#ifdef NETDATA_INTERNAL_CHECKS
                error("%s: Hard gap [%ld, %ld] = %ld was detected. Need to fill it with zeros up to page index %u", REPLICATION_MSG, current_end_time, timestamp, t_sample_diff, page_length);
#endif
                if(page_length > MEM_PAGE_BLOCK_SIZE){
                    infoerr("%s: Page size is not enough to fill the hard gap.", REPLICATION_MSG);
                    return;
                }
            }
        }
        page[page_length / sizeof(number)] = number;
        page_length += sizeof(number);
        rep_state->dim_past_data->page_length = page_length;
        rep_state->dim_past_data->end_time = timestamp * USEC_PER_SEC;
    }
    debug(D_REPLICATION, "%s: Collect past metric sample#%u@%ld: "CALCULATED_NUMBER_FORMAT" \n", REPLICATION_MSG, page_length, timestamp, unpack_storage_number(number));
}

void replication_collect_past_metric_done(REPLICATION_STATE *rep_state) {

    RRDDIM_PAST_DATA *dim_past_data = rep_state->dim_past_data;
    if(!rep_state->dim_past_data->rd){
        infoerr("%s: Collect past metric: Dimension not found in the host", REPLICATION_MSG);
        return;
    }    
    flush_collected_metric_past_data(dim_past_data, rep_state);
}

void flush_collected_metric_past_data(RRDDIM_PAST_DATA *dim_past_data, REPLICATION_STATE *rep_state){
#ifdef ENABLE_DBENGINE
    if(rrdeng_store_past_metrics_realtime(dim_past_data->rd, dim_past_data)){    
        if(rrdeng_store_past_metrics_page_init(dim_past_data, rep_state)){
            infoerr("%s: Cannot initialize db engine page: Flushing collected past data skipped!", REPLICATION_MSG);
            return;
        }
        rrdeng_store_past_metrics_page(dim_past_data, rep_state);
        rrdeng_flush_past_metrics_page(dim_past_data, rep_state);
        rrdeng_store_past_metrics_page_finalize(dim_past_data, rep_state);
        debug(D_REPLICATION, "%s: Flushed Collected Past Metric %s.%s", REPLICATION_MSG, dim_past_data->rd->rrdset->id, dim_past_data->rd->id);
    }
#else
    UNUSED(dim_past_data);
    infoerr("%s: Flushed Collected Past Metric is not supported for host %s. Replication Rx thread needs `dbengine` memory mode.", REPLICATION_MSG, rep_state->host->hostname);
#endif
}

/****************************************
 * Store GAP ops in agent metdata DB(SQLite)
 *****************************************/
int save_gap(GAP *a_gap)
{
    int rc;

    if (unlikely(!db_meta) && default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
        return 0;
    if (!a_gap || !a_gap->status)
        return 0;
    if (!strcmp(a_gap->status, "oninit"))
        return 0;

    rc = sql_store_gap(
        &a_gap->gap_uuid,
        a_gap->host_mguid,
        a_gap->t_window.t_start,
        a_gap->t_window.t_first,
        a_gap->t_window.t_end,
        a_gap->status);

    return rc;
}

void copy_gap(GAP *dst, GAP *src) {
    if(!dst || !src) {
        error("%s: No copy - Args contain NULL pointers: dst*: %p src*: %p", REPLICATION_MSG, dst, src);
        return;
    }
    uuid_copy(dst->gap_uuid, src->gap_uuid);
    dst->host_mguid = strdupz(src->host_mguid);
    dst->t_window.t_start = src->t_window.t_start;
    dst->t_window.t_first = src->t_window.t_first;
    dst->t_window.t_end = src->t_window.t_end;
    dst->status = strdupz(src->status);    
}

void reset_gap(GAP *a_gap) {
    memset(&a_gap->t_window, 0, sizeof(TIME_WINDOW));
    memset(a_gap, 0, sizeof(GAP));
    a_gap->status = "oninit";
}

GAP* add_gap_data(GAPS *host_queue, GAP *gap) {
    int q_count = host_queue->gaps->count;
    int q_max = host_queue->gaps->max;
    // This wrapping index functionality needs more attention
    unsigned int index = (q_count ? (q_count + 1) : q_count) % q_max;
    // unsigned int index = q_count % q_max;
    GAP *gap_in_mem = &host_queue->gap_data[index];
    copy_gap(gap_in_mem, gap);
    debug(D_REPLICATION, "%s: Add GAP data at index %u: \n", REPLICATION_MSG, index);
    // print_replication_gap(gap_in_mem);
    return gap_in_mem;
}

int save_all_host_gaps(GAPS *gap_timeline){
    int rc = 0;
    info("%s: Save all GAPs in the metadata DB table.", REPLICATION_MSG);

    // Traverse over the linked-list of the GAPs in the queue
    GAP *q_gap;
    node elem = gap_timeline->gaps->front; // start
    for(; ((elem != NULL) && (elem->item != NULL)) ; elem = elem->next) {
        q_gap = (GAP *)elem->item;
        if(!strcmp(q_gap->status, "oncompletion"))
            rc += save_gap(q_gap);
    };

    return rc;
}

int load_gap(RRDHOST *host)
{
    int rc;   
    if (unlikely(!db_meta) && default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
        return SQLITE_ERROR;
    rc = sql_load_host_gap(host);

    return rc;
}

int remove_all_gaps(void)
{
    int rc;

    if (unlikely(!db_meta) && default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
        return 0;
    rc = sql_delete_all_gaps();

    return rc;
}

int remove_all_host_gaps(RRDHOST* host)
{
    int rc;
    info("%s: Clean up GAPs table in metadata DB for host %s...", REPLICATION_MSG, host->hostname);

    if (unlikely(!db_meta) && default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
        return 0;
    rc = sql_delete_all_host_gaps(host);

    return rc;
}

int remove_gap(GAP *a_gap)
{
    int rc;

    if (unlikely(!db_meta) && default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)
        return 0;
    rc = sql_delete_gap(&a_gap->gap_uuid);

    return rc;
}

/***********************************
* Replication Parser and commands
************************************/
// Produce a full line if one exists, statefully return where we start next time.
// When we hit the end of the buffer with a partial line move it to the beginning for the next fill.
static char *receiver_next_line(struct replication_state *r, int *pos) {
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

// The receiver socket is blocking, perform a single read into a buffer so that we can reassemble lines for parsing.
static int receiver_read(struct replication_state *r, FILE *fp) {
#ifdef ENABLE_HTTPS
    if (r->ssl.conn && !r->ssl.flags) {
        ERR_clear_error();
        int desired = sizeof(r->read_buffer) - r->read_len - 1;
        int ret = SSL_read(r->ssl.conn, r->read_buffer + r->read_len, desired);
        if (ret > 0 ) {
            r->read_len += ret;
            debug(D_REPLICATION, "%s: RxREAD SSLread %d bytes. Buffer[%s]@%d\n", REPLICATION_MSG, ret, r->read_buffer, r->read_len);
            return 0;
        }

        // Don't treat SSL_ERROR_WANT_READ or SSL_ERROR_WANT_WRITE differently on blocking socket
        u_long err;
        char buf[256];
        while ((err = ERR_get_error()) != 0) {
            ERR_error_string_n(err, buf, sizeof(buf));
            error("%s: %s [receive from %s] ssl error: %s", REPLICATION_MSG, r->host->hostname, r->client_ip, buf);
        }
        return 1;
    }
#endif
    if (!fgets(r->read_buffer, sizeof(r->read_buffer), fp)){
        debug(D_REPLICATION, "%s: RxREAD FGETS Buffer[%s]\n", REPLICATION_MSG, r->read_buffer);
        return 1;
    }
    r->read_len = strlen(r->read_buffer);
    return 0;
}

// Replication parser & commands
size_t replication_parser(REPLICATION_STATE *rpt, struct plugind *cd, FILE *fp) {
    size_t result;
    PARSER_USER_OBJECT *user = callocz(1, sizeof(*user));
    if(cd){
        user->enabled = cd->enabled;
    }
    user->host = rpt->host;
    user->opaque = rpt;
    user->cd = cd;
    user->trust_durations = 0;

    // flags & PARSER_NO_PARSE_INIT to avoid default keyword
    PARSER *parser = parser_init(rpt->host, user, fp, PARSER_INPUT_SPLIT);

    if (unlikely(!parser)) {
        error("Failed to initialize parser");
        if(cd){
            cd->serial_failures++;
        }

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
    parser->plugins_action->gap_action    = &pluginsd_gap_action;
    parser->plugins_action->rep_action    = &pluginsd_rep_action;
    parser->plugins_action->rdata_action    = &pluginsd_rdata_action;
    parser->plugins_action->fill_action    = &pluginsd_fill_action;
    parser->plugins_action->fill_end_action    = &pluginsd_fill_end_action;

    user->parser = parser;
    do {
        if (receiver_read(rpt, fp)) {
            infoerr("%s: Nothing to read in the parser. Deadlocked?", REPLICATION_MSG);
            // continue;
            break;
        }
        int pos = 0;
        char *line;
        while ((line = receiver_next_line(rpt, &pos))) {
            debug(D_REPLICATION, "%s: Rx REP Parser received: %s \n", REPLICATION_MSG, line);
            if (unlikely(netdata_exit /*|| rpt->shutdown*/ || parser_action(parser,  line)))
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

/*************************
 * GAPS structs and ops
 **************************/
void gap_destroy(GAP *a_gap) {
    uuid_clear(a_gap->gap_uuid);
    freez(a_gap->host_mguid);
    freez(a_gap);
}

void gaps_init(RRDHOST **a_host)
{
    (*a_host)->gaps_timeline = (GAPS *)callocz(1, sizeof(GAPS));
    if (!(*a_host)->gaps_timeline) {
        error(
            "%s: Failed to create GAP timeline - GAP Awarness is not supported for host %s",
            REPLICATION_MSG,
            (*a_host)->hostname);
            return;
    }
    RRDHOST *host = *a_host;
    host->gaps_timeline->gaps = queue_new(REPLICATION_RX_CMD_Q_MAX_SIZE, false);
    if (!host->gaps_timeline->gaps) {
        error("%s Gaps timeline queue could not be created", REPLICATION_MSG);
        //Handle this case. Probably shutdown deactivate replication.
        return;
    }
    host->gaps_timeline->gap_buffer = (GAP *)callocz(1, sizeof(GAP));
    host->gaps_timeline->beginoftime = rrdhost_first_entry_t(host);
    if (unlikely(!load_gap(host))) {
        infoerr("%s: No past GAPs to add in the queue.", REPLICATION_MSG);
        return;
    }
    info("%s: GAPs Initialization/Loading for host %s", REPLICATION_MSG, host->hostname);

    return;
}

void gaps_destroy(RRDHOST **a_host) {
    RRDHOST *host = *a_host;

    info("%s: Destroying GAPs structs...", REPLICATION_MSG);

    if(remove_all_host_gaps(host))
        error("%s: Cannot delete all GAPs in metadata DB.", REPLICATION_MSG);
    if(save_all_host_gaps(host->gaps_timeline))
        error("%s: Cannot save Queue GAP struct in metadata DB.", REPLICATION_MSG);
    if(save_gap(host->gaps_timeline->gap_buffer))
        error("%s: Cannot save GAP BUFFER struct in metadata DB.", REPLICATION_MSG);
    info("%s: Save the buffer GAP in metadata DB table.", REPLICATION_MSG);        
    queue_free(host->gaps_timeline->gaps);
    gap_destroy(host->gaps_timeline->gap_buffer);
    freez(host->gaps_timeline);
}

void generate_new_gap(struct receiver_state *stream_recv) {
    GAP *newgap = stream_recv->host->gaps_timeline->gap_buffer;
    uuid_generate(newgap->gap_uuid);
    newgap->host_mguid = strdupz(stream_recv->machine_guid);
    newgap->t_window.t_start = MIN((now_realtime_sec() - REPLICATION_GAP_TIME_MARGIN), stream_recv->last_msg_t);
    newgap->t_window.t_first = stream_recv->last_msg_t;
    newgap->t_window.t_end = 0;
    newgap->status = "oncreate";
    return;
}

int complete_new_gap(GAP *potential_gap){
    if(!potential_gap->status || strcmp("oncreate", potential_gap->status)) {
        error("%s: This GAP cannot be completed. Need to create it first or its an empty GAP if status is null.", REPLICATION_MSG);
        return 1;
    }
    potential_gap->t_window.t_end = now_realtime_sec() + REPLICATION_GAP_TIME_MARGIN;
    potential_gap->status = "oncompletion";
    return 0;
}

// GAP completion at staging buffer
// GAP addition at the end of the queue
void evaluate_gap_onconnection(struct receiver_state *stream_recv)
{
    debug(D_REPLICATION, "%s: Evaluate GAPs on connection", REPLICATION_MSG);
    info("%s: Evaluate GAPs on connection", REPLICATION_MSG);
    if (!stream_recv->host->gaps_timeline) {
        infoerr("%s: GAP Awareness mechanism is not ready - Continue...", REPLICATION_MSG);
        return;
    }
    GAP *seed_gap = (GAP *)stream_recv->host->gaps_timeline->gap_buffer;
    // Re-connection
    if (complete_new_gap(seed_gap)) {
        error("%s: Broken GAP sequence. GAP status is %s", REPLICATION_MSG, seed_gap->status);
        return;
    }
    // TODO: Handle the retention check here
    debug(D_REPLICATION, "%s: A new complete GAP was detected", REPLICATION_MSG);
    GAP * gap_to_push = add_gap_data(stream_recv->host->gaps_timeline, seed_gap);
    if (!queue_push(stream_recv->host->gaps_timeline->gaps, (void *)gap_to_push)) {
        infoerr("%s: Couldn't add the GAP in the queue.", REPLICATION_MSG);
        return;
    }
    // Clean the gap buffer
    reset_gap(seed_gap);
    info("%s: New GAP added in the queue.", REPLICATION_MSG);
}

// GAP generation at disconnection
// staging GAP
void evaluate_gap_ondisconnection(struct receiver_state *stream_recv) {

    debug(D_REPLICATION, "%s: Evaluate GAPs on dis-connection", REPLICATION_MSG);
    info("%s: Evaluate GAPs on dis-connection", REPLICATION_MSG);
    if (!stream_recv->host->gaps_timeline) {
        infoerr("%s: GAP Awareness mechanism is not ready - Continue...", REPLICATION_MSG);
        return;
    }
    generate_new_gap(stream_recv);
    info("%s: New GAP seed was collected in the GAPs buffer!", REPLICATION_MSG);
}

/*************************************************
* FSM functionalities for replication protocol
**************************************************/ 
// chart labels for REP
void replication_send_clabels(REPLICATION_STATE *rep_state, RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    struct label_index *labels_c = &st->state->labels;
    if (labels_c) {
        netdata_rwlock_rdlock(&host->labels.labels_rwlock);
        struct label *lbl = labels_c->head;
        while(lbl) {
            buffer_sprintf(rep_state->build,
                           "CLABEL \"%s\" \"%s\" %d\n", lbl->key, lbl->value, (int)lbl->label_source);

            lbl = lbl->next;
        }
        if (labels_c->head)
            buffer_sprintf(rep_state->build,"CLABEL_COMMIT\n");
        netdata_rwlock_unlock(&host->labels.labels_rwlock);
    }
}

// Send the current chart definition.
// Assumes that collector thread has already called sender_start for mutex / buffer state.
static inline void replication_send_chart_definition_nolock(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    REPLICATION_STATE *rep_state = host->replication->tx_replication;

    rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

    // properly set the name for the remote end to parse it
    char *name = "";
    if(likely(st->name)) {
        if(unlikely(strcmp(st->id, st->name))) {
            // they differ
            name = strchr(st->name, '.');
            if(name)
                name++;
            else
                name = "";
        }
    }

    // send the chart
    buffer_sprintf(
            rep_state->build
            , "CHART \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" %ld %d \"%s %s %s %s\" \"%s\" \"%s\"\n"
            , st->id
            , name
            , st->title
            , st->units
            , st->family
            , st->context
            , rrdset_type_name(st->chart_type)
            , st->priority
            , st->update_every
            , rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)?"obsolete":""
            , rrdset_flag_check(st, RRDSET_FLAG_DETAIL)?"detail":""
            , rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST)?"store_first":""
            , rrdset_flag_check(st, RRDSET_FLAG_HIDDEN)?"hidden":""
            , (st->plugin_name)?st->plugin_name:""
            , (st->module_name)?st->module_name:""
    );

    // send the chart labels
    if (rep_state->stream_version >= STREAM_VERSION_CLABELS)
        replication_send_clabels(rep_state, st);

    // send the dimensions
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        buffer_sprintf(
                rep_state->build
                , "DIMENSION \"%s\" \"%s\" \"%s\" " COLLECTED_NUMBER_FORMAT " " COLLECTED_NUMBER_FORMAT " \"%s %s %s\"\n"
                , rd->id
                , rd->name
                , rrd_algorithm_name(rd->algorithm)
                , rd->multiplier
                , rd->divisor
                , rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)?"obsolete":""
                , rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)?"hidden":""
                , rrddim_flag_check(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)?"noreset":""
        );
        rd->exposed = 1;
    }

    // send the chart local custom variables
    RRDSETVAR *rs;
    for(rs = st->variables; rs ;rs = rs->next) {
        if(unlikely(rs->type == RRDVAR_TYPE_CALCULATED && rs->options & RRDVAR_OPTION_CUSTOM_CHART_VAR)) {
            calculated_number *value = (calculated_number *) rs->value;

            buffer_sprintf(
                    rep_state->build
                    , "VARIABLE CHART %s = " CALCULATED_NUMBER_FORMAT "\n"
                    , rs->variable
                    , *value
            );
        }
    }

    st->upstream_resync_time = st->last_collected_time.tv_sec + (remote_clock_resync_iterations * st->update_every);
}
// send all metrics from all the charts for this gap
void sender_fill_gap_nolock(REPLICATION_STATE *rep_state, RRDSET *st, GAP a_gap)
{
    RRDDIM *rd;
    struct rrddim_query_handle handle;
    // TODO: Add it in the stream.conf file
    unsigned int block_size_in_bytes = MEM_PAGE_BLOCK_SIZE;
    unsigned int sample_in_bytes = (unsigned int) sizeof(storage_number);
    unsigned int default_replication_gap_block_size = block_size_in_bytes/sample_in_bytes; // in samples

    unsigned int block_id = 0;
    int update_every = st->update_every;

    time_t t_delta_start = a_gap.t_window.t_start;
    time_t t_delta_first = a_gap.t_window.t_first;
    time_t t_delta_end = a_gap.t_window.t_end;
    time_t window_start, window_end;
    time_t newest_connection = st->rrdhost->sender->t_newest_connection;
    UNUSED(t_delta_first);
   
    // Chop the GAP time interval to fit a MEM_PAGE_BLOCK_SIZE(4096)
    // window_end is more important than window start
    // window_end = MAX((t_delta_end + (t_delta_end % update_every)), newest_connection);
    window_end = MAX((t_delta_end + (update_every - (t_delta_end % update_every))), newest_connection);
    window_start = t_delta_start - (t_delta_start % update_every);
    size_t replication_points = (t_delta_end - t_delta_start) / update_every + 1;
    if (replication_points > default_replication_gap_block_size){
        replication_points = default_replication_gap_block_size;
        window_start = window_end - replication_points * update_every;
    }

    debug(
        D_REPLICATION,
        "%s: REP GAP timestamps request from MEMORY\nt_d_start: %ld, t_d_end:  %ld \nw_start:  %ld w_end:  %ld \nreplication points: %lu\nnewest_connection: %ld\nupdate_every: %d",
        REPLICATION_MSG,
        t_delta_start,
        t_delta_end,
        window_start,
        window_end,
        replication_points,
        newest_connection,
        update_every);

    char gap_uuid_str[UUID_STR_LEN];
    uuid_unparse(a_gap.gap_uuid, gap_uuid_str);

#ifdef  NETDATA_INTERNAL_CHECKS
    rrdset_dump_debug_rep_state(st);
#endif  //NETDATA_INTERNAL_CHECKS

    size_t num_points = 0;
    rrddim_foreach_read(rd, st) {
        if (!rd->exposed)
            continue;        

        // Send the intersection of this dimension and the time-window on the chart
        time_t rd_start = rrddim_first_entry_t(rd);
        time_t rd_end   = rrddim_last_entry_t(rd) + update_every;
        if (rd_start <= window_end && rd_end >= window_start) {

            time_t rd_oldest = MAX(rd_start, window_start);
            rd_end = MIN(rd_end,   window_end);
            size_t dim_collected_samples = rd->collections_counter;
            size_t gap_samples = (window_end - window_start)/update_every + 1;
            time_t last_t_collected_sample = rd->last_collected_time.tv_sec;
            time_t first_t_collected_sample = last_t_collected_sample - (dim_collected_samples * update_every);
            size_t gap_left_samples = (rd_oldest - first_t_collected_sample)/update_every;
            time_t left_t_gap_sample = (first_t_collected_sample - first_t_collected_sample%update_every) + gap_left_samples*update_every;
            // time_t rigth_t_gap_sample = (left_t_gap_sample + (gap_samples + 1)*update_every);
            time_t rigth_t_gap_sample = (left_t_gap_sample + gap_samples*update_every);


            rd->state->query_ops.init(rd, &handle, left_t_gap_sample, rigth_t_gap_sample);
#ifdef NETDATA_INTERNAL_CHECKS
            time_t first_sample_of_new_conn =
                (time_t)(rd->last_collected_time.tv_sec - ((rd->rrdset->current_entry + ABS((int)(rd->rrdset->counter_done - rd->rrdset->counter)) + 1) * rd->update_every));
            debug(
                D_REPLICATION,
                "Fill replication with %s.%s last_coll=%ld entries=%ld col_total:%lu tmpstamp:%ld window=%ld-%ld data=%ld-%ld query=%ld-%ld",
                st->id,
                rd->id,
                rd->last_collected_time.tv_sec,
                rd->rrdset->current_entry,
                rd->collections_counter,
                first_sample_of_new_conn,
                window_start,
                window_end,
                rd_start,
                rrddim_last_entry_t(rd),
                handle.start_time,
                handle.end_time);
#endif
            buffer_sprintf(rep_state->build, "RDATA %s \"%s\" \"%s\" %ld %ld %u\n", gap_uuid_str, st->id, rd->id, window_start, window_end, block_id);
            
            num_points = 0;
            for (time_t metric_t = left_t_gap_sample; metric_t <= rigth_t_gap_sample;) {

                if (rd->state->query_ops.is_finished(&handle)) {
                    debug(D_REPLICATION, "%s.%s query handle finished early @%ld", st->id, rd->id, metric_t);
                    break;
                }

                storage_number n = rd->state->query_ops.next_metric(&handle, &metric_t);
                if(default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE){
                    metric_t += rd->update_every;
                    rd->state->query_ops.init(rd, &handle, metric_t, rigth_t_gap_sample);
                }
                
                if (n == SN_EMPTY_SLOT)
                    debug(D_REPLICATION, "%s.%s db empty in valid dimension range @ %ld", st->id, rd->id, metric_t);
                else {
                buffer_sprintf(rep_state->build, "FILL \"%s\" \"%s\" %ld " STORAGE_NUMBER_FORMAT "\n", st->id, rd->id, metric_t, n);
                debug(D_REPLICATION, "%s.%s FILL %ld " STORAGE_NUMBER_FORMAT "\n", st->id, rd->id, metric_t, n);
                    num_points++;
                    
                }
            }
            buffer_sprintf(rep_state->build, "FILLEND %zu %u\n", num_points, block_id);
            rd->state->query_ops.finalize(&handle);
        }
        else
            debug(D_REPLICATION, "%s.%s has no data in the replication window (@%ld-%ld) last_collected=%ld.%ld",
                                 st->id, rd->id, (long)window_start, window_end, (long)rd->last_collected_time.tv_sec,
                                 rd->last_collected_time.tv_usec);

    }
    //To be removed
    debug(D_REPLICATION, "Send BUFFER(%s): [%s]",rep_state->host->hostname, buffer_tostring(rep_state->build));
}

void sender_gap_filling(REPLICATION_STATE *rep_state, GAP a_gap)
{
    if(!rep_state){
        error("%s: Cannot Send GAP replication state is NULL!", REPLICATION_MSG);
        return;
    }
    RRDSET *st;
    RRDHOST *host = rep_state->host;
    rrdhost_rdlock(host);
    rrdset_foreach_read(st, host)
    {
        sender_chart_gap_filling(st, a_gap); //Send gap data per chart
    }
    rrdhost_unlock(host);
}

// For each chart send the dimension past data in RDATA format
void sender_chart_gap_filling(RRDSET *st, GAP a_gap) {
    REPLICATION_STATE *rep_state = st->rrdhost->replication->tx_replication;
    rrdset_rdlock(st);    
    if(unlikely(!should_send_chart_matching(st))){
        rrdset_unlock(st);    
        return;
    }

    replication_start(rep_state);         // Locks the sender buffer
    if(need_to_send_chart_definition(st))
        replication_send_chart_definition_nolock(st); // Does Replication need to resend the defintions?
    sender_fill_gap_nolock(rep_state, st, a_gap);
    replication_commit(rep_state);        // Releases the sender buffer
    replication_attempt_to_send(rep_state);
    rrdset_unlock(st);
}

void replication_gap_to_str(GAP *a_gap, char **gap_str, size_t *len)
{
    char gap_uuid_str[UUID_STR_LEN];
    uuid_unparse(a_gap->gap_uuid, gap_uuid_str);
    *len = (size_t) (sizeof("GAP ") + UUID_STR_LEN + 3*(10) + 2 + 1);
    *gap_str = (char *)callocz(*len, sizeof(char));
    snprintf(*gap_str, *len, "GAP %s %ld %ld %ld\n",
        gap_uuid_str,
        a_gap->t_window.t_start,
        a_gap->t_window.t_first,
        a_gap->t_window.t_end);
}

void replication_rdata_to_str(GAP *a_gap, char **rdata_str, size_t *len, int block_id)
{
    char gap_uuid_str[UUID_STR_LEN];
    uuid_unparse(a_gap->gap_uuid, gap_uuid_str);
    *len = (size_t) (sizeof("RDATA ") + UUID_STR_LEN + 2*(10) + 3 + 1);
    *rdata_str = (char *)callocz(*len, sizeof(char));
    snprintf(*rdata_str, *len, "RDATA %s %ld %ld %d\n",
        gap_uuid_str,
        a_gap->t_window.t_start,
        a_gap->t_window.t_end,
        block_id);
}

/***************************
* Helper and Debug functions
****************************/
void print_replication_state(REPLICATION_STATE *state)
{
    info(
        "%s: Replication State is ...\n pthread_id: %lu\n, enabled: %u\n, spawned: %u\n, socket: %d\n, connected: %u\n, connected_to: %s\n, reconnects_counter: %lu\n, resume: %u\n, pause: %u\n, shutdown: %u\n",
        REPLICATION_MSG,
        state->thread,
        state->enabled,
        state->spawned,
        state->socket,
        state->connected,
        state->connected_to,
        state->reconnects_counter,
        state->resume,
        state->pause,
        state->shutdown);
}

void print_replication_gap(GAP *a_gap)
{
    if(!a_gap || !a_gap->status){
        info("%s: Empty or there is no GAP", REPLICATION_MSG);
        return ;
    }

    char gap_uuid_str[UUID_STR_LEN];
    uuid_unparse(a_gap->gap_uuid, gap_uuid_str);
    info("%s: GAP details are: \nstatus: %s\n, t_s: %ld t_f: %ld t_e: %ld\n, host_mguid: %s\n, gap_uuid_str: %s\n",
        REPLICATION_MSG,
        a_gap->status,
        a_gap->t_window.t_start,
        a_gap->t_window.t_first,
        a_gap->t_window.t_end,
        a_gap->host_mguid,
        gap_uuid_str);
}

void print_replication_queue_gap(GAPS *gaps_timeline)
{
    int count = gaps_timeline->gaps->count;
    info("%s: GAPS in the queue (%d)", REPLICATION_MSG, count);
    
    // Traverse over the linked-list of the GAPs in the queue
    node elem = gaps_timeline->gaps->front; // start
    for(; ((elem != NULL) && (elem->item != NULL)) ; elem = elem->next) {
        print_replication_gap((GAP *)elem->item);
    };

    info("%s: GAP in Buffer: \n", REPLICATION_MSG);
    print_replication_gap(gaps_timeline->gap_buffer);
}

void rrdset_dump_debug_rep_state(RRDSET *st) {
#ifdef NETDATA_INTERNAL_CHECKS
    if (debug_flags & D_REPLICATION) {
        RRDDIM *rd;
        debug(D_REPLICATION, "Chart state %s: counter=%zu counter_done=%zu current_entry=%ld usec_since_last=%llu last_updated=%ld.%ld"
                             " last_collected=%ld.%ld collected_total=%lld last_collected_total=%lld",
                             st->id, st->counter, st->counter_done, st->current_entry, st->usec_since_last_update,
                             st->last_updated.tv_sec, st->last_updated.tv_usec,
                             st->last_collected_time.tv_sec, st->last_collected_time.tv_usec,
                             st->collected_total, st->last_collected_total);
        netdata_rwlock_rdlock(&st->rrdset_rwlock);
        rrddim_foreach_read(rd, st) {
            debug(D_REPLICATION, "Dimension state %s.%s: calculated_value=" CALCULATED_NUMBER_FORMAT
                                 " last_calculated_value=" CALCULATED_NUMBER_FORMAT
                                 " last_stored_value=" CALCULATED_NUMBER_FORMAT
                                 " collected_value=" COLLECTED_NUMBER_FORMAT
                                 " last_collected_value=" COLLECTED_NUMBER_FORMAT
                                 " col_counter=%zu"
                                 " col_volume=" CALCULATED_NUMBER_FORMAT
                                 " store_volume=" CALCULATED_NUMBER_FORMAT
                                 " last_coll_time=%ld.%ld",
                                 st->id, rd->id, rd->calculated_value, rd->last_calculated_value,
                                 rd->last_stored_value, rd->collected_value, rd->last_collected_value,
                                 rd->collections_counter, rd->collected_volume,
                                 rd->stored_volume, rd->last_collected_time.tv_sec, rd->last_collected_time.tv_usec);
            #ifdef ENABLE_DBENGINE
            if (st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && !rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED) &&
                ((struct rrdeng_collect_handle *)rd->state->handle)->descr) {
                // This is safe but do not do this from production code. (The debugging points that call this are
                // on the collector thread and this is the hot-page so it cannot be flushed during execution).
                uv_rwlock_rdlock(&((struct rrdeng_collect_handle *)rd->state->handle)->ctx->pg_cache.pg_cache_rwlock);

                struct rrdeng_page_descr *descr = ((struct rrdeng_collect_handle *)rd->state->handle)->descr;
                struct page_cache_descr *pc_descr = descr->pg_cache_descr;
                if (pc_descr) {
                    storage_number x;
                    uint32_t entries = descr->page_length / sizeof(x);
                    uint32_t start = 0;
                    if (entries > 3)
                        start = entries-3;
                    char buffer[80] = {0};
                    for(uint32_t i=start; i<entries; i++) {
                        x = ((storage_number*)pc_descr->page)[i];
                        sprintf(buffer + strlen(buffer), STORAGE_NUMBER_FORMAT " ", x);
                    }
                    debug(D_REPLICATION, "%s.%s page_descr %llu - %llu with %u, last points %s", st->id, rd->id,
                                         descr->start_time,
                                         descr->end_time,
                                         entries,
                                         buffer);
                }
                uv_rwlock_rdunlock(&((struct rrdeng_collect_handle *)rd->state->handle)->ctx->pg_cache.pg_cache_rwlock);
            }
            #endif
        }
        netdata_rwlock_unlock(&st->rrdset_rwlock);
    }
#else
    UNUSED(st);
#endif
}

void print_collected_metric_past_data(RRDDIM_PAST_DATA *past_data, REPLICATION_STATE *rep_state){

    RRDSET *st = rrdset_find_byname(rep_state->host, past_data->rrdset_id);
    if(unlikely(!st)) {
        error("Cannot find chart with name_id '%s' on host '%s'.", past_data->rrdset_id, rep_state->host->hostname);
        return;
    }
    past_data->rd = rrddim_find(st, past_data->rrddim_id);
    if(unlikely(!past_data->rd)) {
        error("Cannot find dimension with id '%s' in chart '%s' on host '%s'.", past_data->rrddim_id, past_data->rrdset_id, rep_state->host->hostname);
        return;
    }

    RRDDIM *rd = past_data->rd;
    time_t ts = past_data->start_time  / USEC_PER_SEC;
    time_t te = past_data->end_time  / USEC_PER_SEC;
    storage_number *page = (storage_number *)past_data->page;
    uint32_t len = past_data->page_length / sizeof(storage_number);
    
    info("%s: Past Samples(%u) [%ld, %ld] for dimension %s\n", REPLICATION_MSG, len, ts, te, rd->id);
    time_t t = ts;
    for(uint32_t i=0; i < len ; i++){
        info("T: %ld, V: "STORAGE_NUMBER_FORMAT" \n", t, page[i]);
        t += rd->update_every;
    }
}

unsigned int is_localhost(RRDHOST* host){
    return !strcmp(host->machine_guid, localhost->machine_guid);
}
