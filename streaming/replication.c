//Includes
#include "rrdpush.h"
#include "collectors/plugins.d/pluginsd_parser.h"

extern struct config stream_config;
extern int netdata_use_ssl_on_stream;

static void replication_receiver_thread_cleanup_callback(void *host);
static void replication_sender_thread_cleanup_callback(void *ptr);
static void print_replication_state(REPLICATION_STATE *state);

static GAPS* gaps_timeline_create() {
    return NULL;
}

// Thread Initialization
static void replication_state_init(REPLICATION_STATE *state)
{
    info("%s: REP STATE INIT", REPLICATION_MSG);
    memset(state, 0, sizeof(*state));
    state->buffer = cbuffer_new(1024, 1024*1024);
    state->build = buffer_create(1);
    state->socket = -1;
// #ifdef ENABLE_HTTPS
//     memset(&state->ssl, 0, sizeof(state->ssl));
// #endif
//     memset(state->gaps_timeline, 0, sizeof(*state->gaps_timeline));
    netdata_mutex_init(&state->mutex);
}

static void print_replication_state(REPLICATION_STATE *state){
    info("%s: Replication State is ...\n pthread_id: %lu\n, enabled: %u\n, spawned: %u\n, socket: %d\n, connected: %u\n, connected_to: %s\n, reconnects_counter: %lu\n",
    REPLICATION_MSG,
    state->thread,
    state->enabled,
    state->spawned,
    state->socket,
    state->connected,
    state->connected_to,
    state->reconnects_counter        
    );
}

static void replication_state_destroy(REPLICATION_STATE *state)
{
    pthread_mutex_destroy(&state->mutex);
    freez(state->buffer);
    freez(state->read_buffer);
    freez(state->gaps_timeline);
#ifdef ENABLE_HTTPS
    freez(&state->ssl);
#endif    
    freez(state);
}

void replication_sender_init(struct sender_state *sender){
    if(!default_rrdpush_replication_enabled)
        return;
    if(!sender || !sender->host){
        error("%s: Host or host's sender state is not initialized! - Tx thread Initialization failed!", REPLICATION_MSG);
        return;
    }

    sender->replication = (REPLICATION_STATE *)callocz(1, sizeof(REPLICATION_STATE));
    replication_state_init(sender->replication);
    sender->replication->enabled = default_rrdpush_replication_enabled;
    info("%s: Initialize REP Tx state during host creation %s .", REPLICATION_MSG, sender->host->hostname);
    print_replication_state(sender->replication);
}

static unsigned int replication_rd_config(struct receiver_state *rpt, struct config *stream_config)
{
    if(!default_rrdpush_replication_enabled)
        return default_rrdpush_replication_enabled;
    unsigned int rrdpush_replication_enable = default_rrdpush_replication_enabled;
    rrdpush_replication_enable = appconfig_get_boolean(stream_config, rpt->key, "enable replication", rrdpush_replication_enable);
    rrdpush_replication_enable = appconfig_get_boolean(stream_config, rpt->machine_guid, "enable replication", rrdpush_replication_enable);
    // Runtime replication enable status
    rrdpush_replication_enable = (default_rrdpush_replication_enabled && rrdpush_replication_enable && (rpt->stream_version >= VERSION_GAP_FILLING));

    return rrdpush_replication_enable;
}

void replication_receiver_init(struct receiver_state *receiver, struct config *stream_config)
{
    unsigned int rrdpush_replication_enable = replication_rd_config(receiver, stream_config);
    if(!rrdpush_replication_enable)
    {
        info("%s:  Could not initialize Rx replication thread. Replication is disabled or not supported!", REPLICATION_MSG);
        return;
    }
    receiver->replication = (REPLICATION_STATE *)callocz(1, sizeof(REPLICATION_STATE));
    replication_state_init(receiver->replication);
    receiver->replication->enabled = rrdpush_replication_enable;
    info("%s: Initialize Rx for host %s ", REPLICATION_MSG, receiver->host->hostname);
    print_replication_state(receiver->replication);
}

// Connection management & socket handling functions
//Close the socket of the replication sender thread
static void replication_sender_thread_close_socket(RRDHOST *host) {
    host->sender->replication->connected = 0;

    if(host->sender->replication->socket != -1) {
        close(host->sender->replication->socket);
        host->sender->replication->socket = -1;
    }
}

extern void rrdpush_encode_variable(stream_encoded_t *se, RRDHOST *host);
extern void rrdpush_clean_encoded(stream_encoded_t *se);

// Connect to parent. The REPLICATE command over HTTP req triggers the receiver thread in parent.
// TODO: revise the logic of the communication
static int replication_sender_thread_connect_to_parent(RRDHOST *host, int default_port, int timeout, struct sender_state *s) {

    struct timeval tv = {
            .tv_sec = timeout,
            .tv_usec = 0
    };

    // make sure the socket is closed
    replication_sender_thread_close_socket(host);

    debug(D_REPLICATION, "%s: Attempting to connect...", REPLICATION_MSG);
    info("%s %s [send to %s]: connecting...", REPLICATION_MSG, host->hostname, host->rrdpush_send_destination);

    host->sender->replication->socket = connect_to_one_of(
            host->rrdpush_send_destination
            , default_port
            , &tv
            , &s->replication->reconnects_counter
            , s->replication->connected_to
            , sizeof(s->replication->connected_to)-1
    );

    if(unlikely(host->sender->replication->socket == -1)) {
        error("%s %s [send to %s]: failed to connect", REPLICATION_MSG, host->hostname, host->rrdpush_send_destination);
        return 0;
    }

    info("%s %s [send to %s]: initializing communication...", REPLICATION_MSG, host->hostname, s->replication->connected_to);

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
            if (SSL_set_fd(host->ssl.conn, host->sender->replication->socket) != 1) {
                error("Failed to set the socket to the SSL on socket fd %d.", host->sender->replication->socket);
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

    stream_encoded_t se;
    rrdpush_encode_variable(&se, host);
    // Add here any extra information to transmit with the.
    char http[HTTP_HEADER_SIZE + 1];
    int eol = snprintfz(http, HTTP_HEADER_SIZE,
            "%s key=%s&hostname=%s&registry_hostname=%s&machine_guid=%s&update_every=%d&timezone=%s&abbrev_timezone=%s&utc_offset=%d&hops=%d&tags=%s&ver=%u"
                 "&NETDATA_PROTOCOL_VERSION=%s"
                 " HTTP/1.1\r\n"
                 "User-Agent: %s\r\n"
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
                 , host->program_version
                 , host->program_name);
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
                replication_sender_thread_close_socket(host);
                return 0;
            }else {
                host->ssl.flags = NETDATA_SSL_NO_HANDSHAKE;
            }
        }
        else {
            if (netdata_use_ssl_on_stream == NETDATA_SSL_FORCE) {
                if (netdata_validate_server == NETDATA_SSL_VALID_CERTIFICATE) {
                    if ( security_test_certificate(host->ssl.conn)) {
                        error("Closing the replication stream connection, because the server SSL certificate is not valid.");
                        replication_sender_thread_close_socket(host);
                        return 0;
                    }
                }
            }
        }
    }
    if(send_timeout(&host->ssl, host->sender->replication->socket, http, strlen(http), 0, timeout) == -1) {
#else
    if(send_timeout(host->sender->replication->socket, http, strlen(http), 0, timeout) == -1) {
#endif
        error("%s %s [send to %s]: failed to send HTTP header to remote netdata.", REPLICATION_MSG, host->hostname, s->replication->connected_to);
        replication_sender_thread_close_socket(host);
        return 0;
    }

    info("%s %s [send to %s]: waiting response from remote netdata...", REPLICATION_MSG, host->hostname, s->replication->connected_to);

    ssize_t received;
#ifdef ENABLE_HTTPS
    received = recv_timeout(&host->ssl,host->sender->replication->socket, http, HTTP_HEADER_SIZE, 0, timeout);
    if(received == -1) {
#else
    received = recv_timeout(host->sender->replication->socket, http, HTTP_HEADER_SIZE, 0, timeout);
    if(received == -1) {
#endif
        error("%s %s [send to %s]: remote netdata does not respond.", REPLICATION_MSG, host->hostname, s->replication->connected_to);
        replication_sender_thread_close_socket(host);
        return 0;
    }

    http[received] = '\0';
    // debug(D_REPLICATION, "Response to sender from far end: %s", http);
    info("%s: Response to sender from far end: %s", REPLICATION_MSG, http);
    print_replication_state(s->replication);
    // REP ack should be in the beggining of the response
    // TODO: Verify the final commands (strings, numbers???) - a simple parser function can be used here.
    if(unlikely(memcmp(http, REP_ACK_CMD, (size_t)strlen(REP_ACK_CMD)))) {
        error("%s %s [send to %s]: server is not replying properly (is it a netdata?).", REPLICATION_MSG, host->hostname, s->connected_to);
        replication_sender_thread_close_socket(host);
        return 0;
    }
    s->replication->connected = 1;
    // END of REP ack checking.

    info("%s %s [send to %s]: established replication communication with a parent using protocol version %d - ready to replicate metrics..."
         , REPLICATION_MSG
         , host->hostname
         , s->replication->connected_to
         , s->version);

    if(sock_setnonblock(host->sender->replication->socket) < 0)
        error("%s %s [send to %s]: cannot set non-blocking mode for socket.", REPLICATION_MSG, host->hostname, s->replication->connected_to);

    // TODO: Check the linux manual for sock tcp enlarge. If I remember well this does nothing. The SE Linux provides a file that needs priviledged access to modify this variable.
    if(sock_enlarge_out(host->sender->replication->socket) < 0)
        error("%s %s [send to %s]: cannot enlarge the socket buffer.", REPLICATION_MSG, host->hostname, s->replication->connected_to);

    debug(D_REPLICATION, "%s: Connected on fd %d...", REPLICATION_MSG, host->sender->replication->socket);

    return 1;
}

static void replication_sender_thread_data_flush(RRDHOST *host) {
    netdata_mutex_lock(&host->sender->replication->mutex);

    size_t len = cbuffer_next_unsafe(host->sender->replication->buffer, NULL);
    if (len)
        error("%s %s [send]: discarding %zu bytes of metrics already in the buffer.", REPLICATION_MSG, host->hostname, len);

    cbuffer_remove_unsafe(host->sender->replication->buffer, len);
    netdata_mutex_unlock(&host->sender->replication->mutex);
}

// Higher level wrap function for connection management and socket metadata updates.
static void replication_attempt_to_connect(struct sender_state *state)
{
    state->replication->send_attempts = 0;

    if(replication_sender_thread_connect_to_parent(state->host, state->default_port, state->timeout, state)) {
        state->replication->last_sent_t = now_monotonic_sec();

        // reset the buffer, to properly gaps and replicate commands
        replication_sender_thread_data_flush(state->host);

        // send from the beginning
        state->replication->begin = 0;

        // make sure the next reconnection will be immediate
        state->replication->not_connected_loops = 0;

        // reset the bytes we have sent for this session
        state->replication->sent_bytes_on_this_connection = 0;

        // Update the connection state flag
        state->replication->connected = 1;
    }
    else {
        // increase the failed connections counter
        state->replication->not_connected_loops++;

        // reset the number of bytes sent
        state->replication->sent_bytes_on_this_connection = 0;

        // slow re-connection on repeating errors
        sleep_usec(USEC_PER_SEC * state->replication->reconnect_delay); // seconds
    }
}

// Thread creation
void *replication_sender_thread(void *ptr) {
    struct sender_state *s = (struct sender_state *) ptr;
    unsigned int rrdpush_replication_enabled = s->replication->enabled;
    info("%s Replication sender thread is starting", REPLICATION_MSG);
    
    // attempt to connect to parent
    // Read the config for sending in replication
    // Add here the sender initialization logic of the thread.
    netdata_thread_cleanup_push(replication_sender_thread_cleanup_callback, s->host);
    // Add here the thread loop
    // break condition on netdata_exit, disabled replication (for runtime configuration/restart)
    // for(; rrdpush_replication_enabled && !netdata_exit ;) {
        // check for outstanding cancellation requests
        // netdata_thread_testcancel();
    //     // try to connect
    //     // replcation parser
    //     // send hi
    //     // retrieve response
    //     // if reponse is REP off - exit
    // }
    //Implementation...
    int send_count = 1;
    for(;rrdpush_replication_enabled && !netdata_exit && send_count < 100;)
    {
        // check for outstanding cancellation requests
        netdata_thread_testcancel();
        
        // try to connect
        // if(!s->replication->connected)
        if((s->replication->not_connected_loops < 3) && !s->replication->connected) {
            replication_attempt_to_connect(s);
            // Tmp solution to test the thread cleanup process
            s->replication->not_connected_loops++;            
        }
        else {
            char http[256];
            sprintf(http, "Erdem was here %d", send_count);
            send_count ++;
            if(send_timeout(&s->host->sender->replication->ssl, s->host->sender->replication->socket, http, strlen(http), 0, s->timeout) == -1) {
                error("%s %s [send to %s]: failed to send HTTP header to remote netdata.", REPLICATION_MSG, s->host->hostname, s->replication->connected_to);
                replication_sender_thread_close_socket(s->host);
                return 0;
            }

                info("%s %s [send to %s]: waiting response from remote netdata...", REPLICATION_MSG,  s->host->hostname, s->replication->connected_to);

                ssize_t received;

                received = recv_timeout(&s->host->sender->replication->ssl, s->host->sender->replication->socket, http, HTTP_HEADER_SIZE, 0, s->timeout);
                if(received == -1) {
                    error("%s %s [send to %s]: remote netdata does not respond.", REPLICATION_MSG,  s->host->hostname, s->replication->connected_to);
                    replication_sender_thread_close_socket(s->host);
                    return 0;
                }

                http[received] = '\0';
                // debug(D_REPLICATION, "Response to sender from far end: %s", http);
                info("%s: Response to sender from far end: %s", REPLICATION_MSG, http);
        }
    }
    // Closing thread - clean up any resources allocated here
    netdata_thread_cleanup_pop(1);
    return NULL;
}

void replication_sender_thread_spawn(RRDHOST *host) {
    netdata_mutex_lock(&host->sender->replication->mutex);

    //TDRemoved Replication
    print_replication_state(host->sender->replication);
    
    if(!host->sender->replication->spawned) {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "REPLICATION_SENDER[%s]", host->hostname);

        if(netdata_thread_create(&host->sender->replication->thread, tag, NETDATA_THREAD_OPTION_JOINABLE, replication_sender_thread, (void *) host->sender))
            error("%s %s [send]: failed to create new thread for client.", REPLICATION_MSG, host->hostname);
        else
            host->sender->replication->spawned = 1;
    }
    netdata_mutex_unlock(&host->sender->replication->mutex);
}

void *replication_receiver_thread(void *ptr){
    netdata_thread_cleanup_push(replication_receiver_thread_cleanup_callback, ptr);
    struct receiver_state *rpt = (struct receiver_state *)ptr;
    unsigned int rrdpush_replication_enabled =  rpt->replication->enabled;
    // GAPS *gaps_timeline = rpt->replication->gaps_timeline;
    //read configuration
    //create pluginds cd object
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
    snprintfz(cd.id,           CONFIG_MAX_NAME,  "%s:%s", rpt->replication->client_ip, rpt->replication->client_port);
    snprintfz(cd.filename,     FILENAME_MAX,     "%s:%s", rpt->replication->client_ip, rpt->replication->client_port);
    snprintfz(cd.fullfilename, FILENAME_MAX,     "%s:%s", rpt->replication->client_ip, rpt->replication->client_port);
    snprintfz(cd.cmd,          PLUGINSD_CMD_MAX, "%s:%s", rpt->replication->client_ip, rpt->replication->client_port);    
    // Respond with the REP ack command
    info("%s %s [receive from [%s]:%s]: initializing replication communication...", REPLICATION_MSG, rpt->host->hostname, rpt->replication->client_ip, rpt->replication->client_port);
    char initial_response[HTTP_HEADER_SIZE];
    if (rpt->stream_version >= VERSION_GAP_FILLING) {
        info("%s %s [receive from [%s]:%s]: Netdata acknowledged replication over stream version %u.", REPLICATION_MSG, rpt->host->hostname, rpt->replication->client_ip, rpt->replication->client_port, rpt->stream_version);
        sprintf(initial_response, "%s", REP_ACK_CMD);
    } 
    else {
        info("%s [receive from [%s]:%s]: Netdata stream protocol does not support replication.", rpt->host->hostname, rpt->replication->client_ip, rpt->replication->client_port);
        sprintf(initial_response, "%s", "REP off");
    }
    // debug(D_REPLICATION, "Initial REPLICATION response to %s: %s", rpt->client_ip, initial_response);
    info("%s: Initial REPLICATION response to [%s:%s]: %s", REPLICATION_MSG, rpt->replication->client_ip, rpt->replication->client_port, initial_response);
#ifdef ENABLE_HTTPS
    rpt->host->stream_ssl.conn = rpt->replication->ssl.conn;
    rpt->host->stream_ssl.flags = rpt->replication->ssl.flags;
    if(send_timeout(&rpt->replication->ssl, rpt->replication->socket, initial_response, strlen(initial_response), 0, 60) != (ssize_t)strlen(initial_response)) {
#else
    if(send_timeout(rpt->replication->socket, initial_response, strlen(initial_response), 0, 60) != strlen(initial_response)) {
#endif
        log_stream_connection(rpt->replication->client_ip, rpt->replication->client_port, rpt->key, rpt->host->machine_guid, rpt->host->hostname, "REPLICATION CONNECTION FAILED - THIS HOST FAILED TO REPLY");
        error("%s %s [receive from [%s]:%s]: failed to send replication acknowledgement command.", REPLICATION_MSG, rpt->host->hostname, rpt->replication->client_ip, rpt->replication->client_port);
        close(rpt->replication->socket);
        return 0;
    }
    // Here is the first proof of connection with the sender thread.

    // remove the non-blocking flag from the socket
    if(sock_delnonblock(rpt->replication->socket) < 0)
        error("%s %s [receive from [%s]:%s]: cannot remove the non-blocking flag from socket %d", REPLICATION_MSG, rpt->host->hostname, rpt->replication->client_ip, rpt->replication->client_port, rpt->replication->socket);

    // convert the socket to a FILE *
    FILE *fp = fdopen(rpt->fd, "r");
    if(!fp) {
        log_stream_connection(rpt->replication->client_ip, rpt->replication->client_port, rpt->key, rpt->host->machine_guid, rpt->host->hostname, "SOCKET CONVERSION TO FD FAILED - SOCKET ERROR");
        error("%s %s [receive from [%s]:%s]: failed to get a FILE for FD %d.", REPLICATION_MSG, rpt->host->hostname, rpt->replication->client_ip, rpt->replication->client_port, rpt->replication->socket);
        close(rpt->replication->socket);
        return 0;
    }
    
    // call the plugins.d processor to receive the metrics
    info("%s %s [receive from [%s]:%s]: filling replication gaps...", REPLICATION_MSG, rpt->host->hostname, rpt->replication->client_ip, rpt->replication->client_port);
    log_stream_connection(rpt->replication->client_ip, rpt->replication->client_port, rpt->key, rpt->host->machine_guid, rpt->host->hostname, "CONNECTED");

    cd.version = rpt->stream_version;
    // Add here the receiver thread logic
    // Need a host
    // Need a PARSER_USER_OBJECT
    // Need a socket
    // need flags to deactivate the no necessary keywords
    // Add here the thread loop
    // for(;rrdpush_replication_enabled && !netdata_exit;)
    // {
    //     // check for outstanding cancellation requests
    //     netdata_thread_testcancel();      

    //     if(gaps_timeline->queue_size == 0){
    //         // Send REP off CMD to the child agent.
    //         break;
    //     }

    //     // send GAP uid ts tf te to the child agent        
    //     // recv RDATA command from child agent
    // }    
    // Closing thread - clean any resources allocated in this thread function

    int send_count = 100;
    for(;rrdpush_replication_enabled && !netdata_exit && send_count > 0;)
    {
        // check for outstanding cancellation requests
        netdata_thread_testcancel();
        char http[256];
        sprintf(http, "Erdem was here %d", send_count);
        send_count --;
        if(send_timeout(&rpt->replication->ssl, rpt->replication->socket, http, strlen(http), 0, rpt->timeout) == -1) {
            error("%s %s [send to %s]: failed to send HTTP header to remote netdata.", REPLICATION_MSG, rpt->host->hostname, rpt->replication->connected_to);
            replication_sender_thread_close_socket(rpt->host);
            return 0;
        }

        info("%s %s [send to %s]: waiting response from remote netdata...", REPLICATION_MSG,  rpt->host->hostname, rpt->replication->connected_to);

        ssize_t received;

        received = recv_timeout(&rpt->host->sender->replication->ssl, rpt->host->sender->replication->socket, http, HTTP_HEADER_SIZE, 0, rpt->timeout);
        if(received == -1) {
            error("%s %s [send to %s]: remote netdata does not respond.", REPLICATION_MSG,  rpt->host->hostname, rpt->replication->connected_to);
            replication_sender_thread_close_socket(rpt->host);
            return 0;
        }

        http[received] = '\0';
        // debug(D_REPLICATION, "Response to sender from far end: %s", http);
        info("%s: Response to sender from far end: %s", REPLICATION_MSG, http);
    }
    // Closing thread - clean up any resources allocated here
    netdata_thread_cleanup_pop(1);
    return NULL;   
}

extern int rrdpush_receiver_too_busy_now(struct web_client *w);

extern int rrdpush_receiver_permission_denied(struct web_client *w);

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
        else if(!strcmp(name, "ver"))
            stream_version = MIN((uint32_t) strtoul(value, NULL, 0), STREAMING_PROTOCOL_CURRENT_VERSION);
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
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO KEY");
        error("%s [receive from [%s]:%s]: Replicate request without an API key. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!hostname || !*hostname) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO HOSTNAME");
        error("%s [receive from [%s]:%s]: Replicate request without a hostname. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!machine_guid || !*machine_guid) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO MACHINE GUID");
        error("%s [receive from [%s]:%s]: Replicate request without a machine GUID. Forbidding access.",REPLICATION_MSG, w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(regenerate_guid(key, buf) == -1) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - INVALID KEY");
        error("%s [receive from [%s]:%s]: API key '%s' is not valid GUID (use the command uuidgen to generate one). Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, key);
        return rrdpush_receiver_permission_denied(w);
    }

    if(regenerate_guid(machine_guid, buf) == -1) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - INVALID MACHINE GUID");
        error("%s [receive from [%s]:%s]: machine GUID '%s' is not GUID. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, machine_guid);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!appconfig_get_boolean(&stream_config, key, "enabled", 0)) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - KEY NOT ENABLED");
        error("%s [receive from [%s]:%s]: API key '%s' is not allowed. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, key);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *key_allow_from = simple_pattern_create(appconfig_get(&stream_config, key, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT);
        if(key_allow_from) {
            if(!simple_pattern_matches(key_allow_from, w->client_ip)) {
                simple_pattern_free(key_allow_from);
                rrdhost_system_info_free(system_info);
                log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname) ? hostname : "-", "ACCESS DENIED - KEY NOT ALLOWED FROM THIS IP");
                error("%s [receive from [%s]:%s]: API key '%s' is not permitted from this IP. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, key);
                return rrdpush_receiver_permission_denied(w);
            }
            simple_pattern_free(key_allow_from);
        }
    }

    if(!appconfig_get_boolean(&stream_config, machine_guid, "enabled", 1)) {
        rrdhost_system_info_free(system_info);
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - MACHINE GUID NOT ENABLED");
        error("%s [receive from [%s]:%s]: machine GUID '%s' is not allowed. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, machine_guid);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *machine_allow_from = simple_pattern_create(appconfig_get(&stream_config, machine_guid, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT);
        if(machine_allow_from) {
            if(!simple_pattern_matches(machine_allow_from, w->client_ip)) {
                simple_pattern_free(machine_allow_from);
                rrdhost_system_info_free(system_info);
                log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname) ? hostname : "-", "ACCESS DENIED - MACHINE GUID NOT ALLOWED FROM THIS IP");
                error("%s [receive from [%s]:%s]: Machine GUID '%s' is not permitted from this IP. Forbidding access.", REPLICATION_MSG, w->client_ip, w->client_port, machine_guid);
                return rrdpush_receiver_permission_denied(w);
            }
            simple_pattern_free(machine_allow_from);
        }
    }

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

    // What it does: if host doesn't exist prepare the receiver state struct
    // and start the streaming receiver to create it.
    // What I want:  If the host doesn't exist I should depend on streaming to create it. At this point, streaming should have already call the receiver thread to create the host. So if the host exists we continue with the call to the replication rx thread. If the host doesn't exist and host->receiver is NULL means that there was a problem with host creation during streaming or the REPLICATE command arrived earlier than the respective STREAM command. So do not start the Rx replication thread.The replication Tx thread in child should try to reconnect.

    rrd_rdlock();
    RRDHOST *host = rrdhost_find_by_guid(machine_guid, 0);
    if (unlikely(host && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) /* Ignore archived hosts. */
        host = NULL;
    if(!host) {
            rrd_unlock();
            log_stream_connection(w->client_ip, w->client_port, key, machine_guid, hostname, "ABORT REPLICATION - HOST DOES NOT EXIST");
            infoerr("%s - [received from [%s]:%s]: Host(%s) with machine GUID %s does not exist - Abort replication.", REPLICATION_MSG, w->client_ip, w->client_port, hostname, machine_guid);
            return 409;
    }
    // Chase race condition in case of two REPLICATE requests hit the web server. One should start the receiver replication thread
    // and the other should be rejected.
    // Verify this code: Host exists and replication is active.
    rrdhost_wrlock(host);
    if (host->receiver->replication != NULL) {
        time_t age = now_realtime_sec() - host->receiver->replication->last_msg_t;
        rrdhost_unlock(host);
        rrd_unlock();
        log_stream_connection(w->client_ip, w->client_port, key, host->machine_guid, host->hostname, "REJECTED - ALREADY CONNECTED");
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
    replication_receiver_init(host->receiver, &stream_config);
    host->receiver->replication->last_msg_t = now_realtime_sec();
    host->receiver->replication->socket = w->ifd;
    host->receiver->replication->client_ip = strdupz(w->client_ip);
    host->receiver->replication->client_port = strdupz(w->client_port);
#ifdef ENABLE_HTTPS
    host->receiver->replication->ssl.conn = w->ssl.conn;
    host->receiver->replication->ssl.flags = w->ssl.flags;
    w->ssl.conn = NULL;
    w->ssl.flags = NETDATA_SSL_START;
#endif

    if(w->user_agent && w->user_agent[0]) {
        char *t = strchr(w->user_agent, '/');
        if(t && *t) {
            *t = '\0';
            t++;
        }

        host->receiver->replication->program_name = strdupz(w->user_agent);
        if(t && *t) host->receiver->replication->program_version = strdupz(t);
    }

    debug(D_SYSTEM, "starting REPLICATE receive thread.");

    char tag[FILENAME_MAX + 1];
    snprintfz(tag, FILENAME_MAX, "REPLICATION_RECEIVER[%s,[%s]:%s]", host->hostname, w->client_ip, w->client_port);

    if(netdata_thread_create(&host->receiver->replication->thread, tag, NETDATA_THREAD_OPTION_DEFAULT, replication_receiver_thread, (void *)(host->receiver)))
        error("Failed to create new REPLICATE receive thread for client.");

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

    netdata_mutex_lock(&host->sender->replication->mutex);

    info("%s %s [send]: sending thread cleans up...", REPLICATION_MSG, host->hostname);

    //close replication sender thread socket or/and pipe
    replication_sender_thread_close_socket(host);
    // clean the structures and resources in the thread function
    // follow the shutdown sequence with the sender thread from the rrdhost.c file

    // TBD - Check if joining the streaming threads is good for shutting down the replication threads.
    if(!host->rrdpush_sender_join) {
        info("%s %s [send]: sending thread detaches itself.", REPLICATION_MSG, host->hostname);
        netdata_thread_detach(netdata_thread_self());
    }

    host->sender->replication->spawned = 0;

    info("%s %s [send]: sending thread now exits.", REPLICATION_MSG, host->hostname);

    netdata_mutex_unlock(&host->sender->replication->mutex);
}

void replication_receiver_thread_cleanup_callback(void *host)
{
    // follow the receiver clean-up
    // destroy the replication rx structs
    info("%s: Hey you, add something here...I need to cleanup the receiver thread!!! :P", REPLICATION_MSG);
}

// Any join, start, stop, wait, etc thread function goes here.
// This function should be called when Rx thread is terminating. The Rx thread can start termination after a parser error and/or netdata_exit signal. On shutdown this function will be called to remove any sending replication thread.
void replication_sender_thread_stop(RRDHOST *host) {

    netdata_mutex_lock(&host->sender->replication->mutex);
    netdata_thread_t thr = 0;

    if(host->sender->replication->spawned) {
        info("%s %s [send]: signaling sending thread to stop...", REPLICATION_MSG, host->hostname);

        // Check if this is necessary for replication thread?
        //signal the thread that we want to join it
        //host->rrdpush_sender_join = 1;

        // copy the thread id, so that we will be waiting for the right one
        // even if a new one has been spawn
        thr = host->sender->replication->thread;

        // signal it to cancel
        netdata_thread_cancel(host->sender->replication->thread);
    }

    netdata_mutex_unlock(&host->sender->replication->mutex);

    if(thr != 0) {
        info("%s %s [send]: waiting for the sending thread to stop...", REPLICATION_MSG, host->hostname);
        void *result;
        netdata_thread_join(thr, &result);
        info("%s %s [send]: sending thread has exited.", REPLICATION_MSG, host->hostname);
    }
}

// static inline int parse_replication_ack(char *http)
// {
//     if(unlikely(memcmp(http, REP_ACK_CMD, (size_t)strlen(REP_ACK_CMD))))
//         return 1;
//     return 0;
// }

// Memory Mode access
void collect_replication_gap_data(){
    // collection of gap data in cache/temporary structure
}

void update_memory_index(){
    //dbengine
    //other memory modes?
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

// Replication parser & commands
size_t replication_parser(struct receiver_state *rpt, struct plugind *cd, FILE *fp) {
    size_t result;
    PARSER_USER_OBJECT *user = callocz(1, sizeof(*user));
    user->enabled = cd->enabled;
    user->host = rpt->host;
    user->opaque = rpt;
    user->cd = cd;
    user->trust_durations = 0;

    // flags & PARSER_NO_PARSE_INIT to avoid default keyword
    PARSER *parser = parser_init(rpt->host, user, fp, PARSER_INPUT_SPLIT);

    if (unlikely(!parser)) {
        error("Failed to initialize parser");
        cd->serial_failures++;
        freez(user);
        return 0;
    }
    
    // Add keywords related with REPlication
    // REP on/off/pause/ack
    // GAP - Gap metdata. Information to describe the gap (window_start/end, uuid, chart/dim_id)
    // RDATA - gap data transmission
    // Do I need these two commands in replication?
    // parser_add_keyword(parser, "TIMESTAMP", pluginsd_suspend_this_action);
    // parser_add_keyword(parser, "CLAIMED_ID", pluginsd_suspend_this_action);

    // These are not necessary for the replication parser. Normally I would suggest to assign an inactive action so the replication won't be able to use other functions that can trigger function execution not related with its tasks.
    parser->plugins_action->begin_action     = &pluginsd_suspend_this_action;
    // discuss it with the team
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
    // Add the actions related with replication here.
    // parser->plugins_action->gap_action    = &pluginsd_gap_action;
    // parser->plugins_action->rep_action    = &pluginsd_rep_action;
    // parser->plugins_action->rdata_action    = &pluginsd_rdata_action;

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

// GAP creation and processing
GAP *init_gap(){
    GAP *agap = malloc(sizeof(GAP));
    //memset(&agap, 0, sizeof(GAP));
    return agap;
}

GAP *generate_new_gap(struct receiver_state *stream_recv){
    GAP *newgap = init_gap();
    // newgap->uid = uuidgen(); // find a way to create unique identifiers for gaps
    newgap->uuid = stream_recv->machine_guid;
    newgap->t_window.t_first = now_realtime_sec();
    newgap->status = "oncreate";
    return newgap;
}

int complete_new_gap(GAP *potential_gap){
    if(!strcmp("oncreate", potential_gap->status)) {
        error("%s: This GAP cannot be completed. Need to create it first.", REPLICATION_MSG);
        return 0;
    }
    potential_gap->t_window.t_end = now_realtime_sec();
    potential_gap->status = "oncompletion";
    return 1;
}

int verify_new_gap(GAP *new_gap){
    // Access memory to first time_t for all charts?
    // Access memory to verify last time_t for all charts?
    // update the gap time_first
    // Update the gap timewindow
    // Respect any retention period
    // push the gap in the queue
    return 0;
}

// Push a new gap in the queue
int push_gap(){
    return 0;
}
// Pop a new gap from the queue
int pop_gap(){
    return 0;
}
// delete a gap from the queue
int delete_gap(){
    return 0;
}
// transmit the gap information to the child nodes - send the GAP command
int transmit_gap(){
    return 0;
}

// FSMs for replication protocol implementation
// REP on
// REP off
// REP pause/continue
// REP ack

// RDATA

// Replication FSM logic functions

