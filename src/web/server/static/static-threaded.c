// SPDX-License-Identifier: GPL-3.0-or-later

#define WEB_SERVER_INTERNALS 1
#include "static-threaded.h"

int web_client_timeout = DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS;
int web_client_first_request_timeout = DEFAULT_TIMEOUT_TO_RECEIVE_FIRST_WEB_REQUEST;
long web_client_streaming_rate_t = 0L;

#define WORKER_JOB_ADD_CONNECTION 0
#define WORKER_JOB_DEL_COLLECTION 1
#define WORKER_JOB_ADD_FILE       2
#define WORKER_JOB_DEL_FILE       3
#define WORKER_JOB_READ_FILE      4
#define WORKER_JOB_WRITE_FILE     5
#define WORKER_JOB_RCV_DATA       6
#define WORKER_JOB_SND_DATA       7
#define WORKER_JOB_PROCESS        8

#if (WORKER_UTILIZATION_MAX_JOB_TYPES < 9)
#error Please increase WORKER_UTILIZATION_MAX_JOB_TYPES to at least 8
#endif

/*
 * --------------------------------------------------------------------------------------------------------------------
 * Build web_client state from the pollinfo that describes an accepted connection.
 */
static struct web_client *web_client_create_on_fd(POLLINFO *pi) {
    struct web_client *w;

    pulse_web_client_connected();
    w = web_client_get_from_cache();
    w->fd = pi->fd;

    strncpyz(w->user_auth.client_ip,   pi->client_ip,   sizeof(w->user_auth.client_ip) - 1);
    strncpyz(w->client_port, pi->client_port, sizeof(w->client_port) - 1);
    strncpyz(w->client_host, pi->client_host, sizeof(w->client_host) - 1);

    if(unlikely(!*w->user_auth.client_ip))   strcpy(w->user_auth.client_ip,   "-");
    if(unlikely(!*w->client_port)) strcpy(w->client_port, "-");
	w->port_acl = pi->port_acl;

    int flag = 1;
    if(unlikely(
            web_client_check_conn_tcp(w) && setsockopt(w->fd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0))
        netdata_log_debug(D_WEB_CLIENT, "%llu: failed to enable TCP_NODELAY on socket fd %d.", w->id, w->fd);

    flag = 1;
    if(unlikely(setsockopt(w->fd, SOL_SOCKET, SO_KEEPALIVE, (char *) &flag, sizeof(int)) != 0))
        netdata_log_debug(D_WEB_CLIENT, "%llu: failed to enable SO_KEEPALIVE on socket fd %d.", w->id, w->fd);

    web_client_update_acl_matches(w);
    web_client_enable_wait_receive(w);

    web_server_log_connection(w, "CONNECTED");

    return(w);
}

// --------------------------------------------------------------------------------------
// the main socket listener - STATIC-THREADED

struct web_server_static_threaded_worker {
    ND_THREAD *thread;

    int id;
    bool initializing;
    SPINLOCK spinlock;

    size_t max_sockets;

    volatile size_t connected;
    volatile size_t disconnected;
    volatile size_t receptions;
    volatile size_t sends;
    volatile size_t max_concurrent;
};

static long long static_threaded_workers_count = 1;

static struct web_server_static_threaded_worker *static_workers_private_data = NULL;
static __thread struct web_server_static_threaded_worker *worker_private = NULL;

// ----------------------------------------------------------------------------

static inline int web_server_check_client_status(struct web_client *w) {
    if(unlikely(web_client_check_dead(w) || (!web_client_has_wait_receive(w) && !web_client_has_wait_send(w))))
        return -1;

    return 0;
}

// ----------------------------------------------------------------------------
// web server clients

static void *web_server_add_callback(POLLINFO *pi, nd_poll_event_t *events, void *data __maybe_unused) {
    worker_is_busy(WORKER_JOB_ADD_CONNECTION);
    worker_private->connected++;

    size_t concurrent = worker_private->connected - worker_private->disconnected;
    if(unlikely(concurrent > worker_private->max_concurrent))
        worker_private->max_concurrent = concurrent;

    *events = ND_POLL_READ;

    netdata_log_debug(D_WEB_CLIENT_ACCESS, "LISTENER on %d: new connection.", pi->fd);
    struct web_client *w = web_client_create_on_fd(pi);

    if (!strncmp(pi->client_port, "UNIX", 4)) {
        web_client_set_conn_unix(w);
    } else {
        web_client_set_conn_tcp(w);
    }

    if ((web_client_check_conn_tcp(w)) && (netdata_ssl_web_server_ctx)) {
        sock_setnonblock(w->fd, false);

        //Read the first 7 bytes from the message, but the message
        //is not removed from the queue, because we are using MSG_PEEK
        char test[8];
        if ( recv(w->fd,test, 7, MSG_PEEK) == 7 ) {
            test[7] = '\0';
        }
        else {
            // we couldn't read 7 bytes
            sock_setnonblock(w->fd, true);
            goto cleanup;
        }

        if(test[0] > 0x17) {
            // no SSL
            netdata_ssl_close(&w->ssl); // free any previous SSL data
        }
        else {
            // SSL
            if(!netdata_ssl_open(&w->ssl, netdata_ssl_web_server_ctx, w->fd) || !netdata_ssl_accept(&w->ssl))
                WEB_CLIENT_IS_DEAD(w);
        }

        sock_setnonblock(w->fd, true);
    }

    netdata_log_debug(D_WEB_CLIENT, "%llu: ADDED CLIENT FD %d", w->id, pi->fd);

cleanup:
    worker_is_idle();
    return w;
}

// TCP client disconnected
static void web_server_del_callback(POLLINFO *pi) {
    worker_is_busy(WORKER_JOB_DEL_COLLECTION);

    worker_private->disconnected++;

    struct web_client *w = (struct web_client *)pi->data;

    if(web_client_flag_check(w, WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET))
        pi->flags |= POLLINFO_FLAG_DONT_CLOSE;

    netdata_log_debug(D_WEB_CLIENT, "%llu: CLOSING CLIENT FD %d", w->id, pi->fd);
    web_server_log_connection(w, "DISCONNECTED");
    web_client_request_done(w);
    web_client_release_to_cache(w);
    pulse_web_client_disconnected();

    worker_is_idle();
}

static __thread POLLINFO *current_thread_pollinfo = NULL;

void web_server_remove_current_socket_from_poll(void) {
    if(!current_thread_pollinfo) return;
    poll_process_remove_from_poll(current_thread_pollinfo);
}

static int web_server_rcv_callback(POLLINFO *pi, nd_poll_event_t *events) {
    int ret = -1;
    worker_is_busy(WORKER_JOB_RCV_DATA);

    worker_private->receptions++;

    struct web_client *w = (struct web_client *)pi->data;
    int fd = pi->fd;

    ssize_t bytes;
    bytes = web_client_receive(w);

    if (likely(bytes > 0)) {
        pulse_web_server_received_bytes(bytes);

        netdata_log_debug(D_WEB_CLIENT, "%llu: processing received data on fd %d.", w->id, fd);
        worker_is_idle();
        worker_is_busy(WORKER_JOB_PROCESS);
        current_thread_pollinfo = pi;
        web_client_process_request_from_web_server(w);
        current_thread_pollinfo = NULL;

        if (unlikely(w->mode == HTTP_REQUEST_MODE_STREAM)) {
            ssize_t rc = web_client_send(w);
            if(rc > 0)
                pulse_web_server_sent_bytes(rc);
        }
        else if(unlikely(w->fd == fd && web_client_has_wait_receive(w)))
            *events |= ND_POLL_READ;

        if(unlikely(w->fd == fd && web_client_has_wait_send(w)))
            *events |= ND_POLL_WRITE;

    } else if(unlikely(bytes < 0)) {
        ret = -1;
        goto cleanup;
    } else if (unlikely(bytes == 0)) {
        if(unlikely(w->fd == fd && web_client_has_ssl_wait_receive(w)))
            *events |= ND_POLL_READ;

        if(unlikely(w->fd == fd && web_client_has_ssl_wait_send(w)))
            *events |= ND_POLL_WRITE;
    }

    ret = web_server_check_client_status(w);

cleanup:
    worker_is_idle();
    return ret;
}

static int web_server_snd_callback(POLLINFO *pi, nd_poll_event_t *events) {
    int retval = -1;
    worker_is_busy(WORKER_JOB_SND_DATA);

    worker_private->sends++;

    struct web_client *w = (struct web_client *)pi->data;
    int fd = pi->fd;

    netdata_log_debug(D_WEB_CLIENT, "%llu: sending data on fd %d.", w->id, fd);

    current_thread_pollinfo = pi;
    ssize_t ret = web_client_send(w);
    current_thread_pollinfo = NULL;

    if(unlikely(ret < 0)) {
        retval = -1;
        goto cleanup;
    }

    pulse_web_server_sent_bytes(ret);

    if(unlikely(w->fd == fd && web_client_has_wait_receive(w)))
        *events |= ND_POLL_READ;

    if(unlikely(w->fd == fd && web_client_has_wait_send(w)))
        *events |= ND_POLL_WRITE;

    retval = web_server_check_client_status(w);

cleanup:
    worker_is_idle();
    return retval;
}

// ----------------------------------------------------------------------------
// web server worker thread

static void socket_listen_main_static_threaded_worker_cleanup(void *pptr) {
    worker_private = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!worker_private) return;

    netdata_log_info("stopped after %zu connects, %zu disconnects (max concurrent %zu), %zu receptions and %zu sends",
            worker_private->connected,
            worker_private->disconnected,
            worker_private->max_concurrent,
            worker_private->receptions,
            worker_private->sends
    );

    worker_unregister();
}

static bool web_server_should_stop(void) {
    return !service_running(SERVICE_WEB_SERVER);
}

void socket_listen_main_static_threaded_worker(void *ptr) {
    worker_private = ptr;
    spinlock_lock(&worker_private->spinlock);
    worker_private->initializing = false;
    spinlock_unlock(&worker_private->spinlock);
    worker_register("WEB");
    worker_register_job_name(WORKER_JOB_ADD_CONNECTION, "connect");
    worker_register_job_name(WORKER_JOB_DEL_COLLECTION, "disconnect");
    worker_register_job_name(WORKER_JOB_ADD_FILE, "file start");
    worker_register_job_name(WORKER_JOB_DEL_FILE, "file end");
    worker_register_job_name(WORKER_JOB_READ_FILE, "file read");
    worker_register_job_name(WORKER_JOB_WRITE_FILE, "file write");
    worker_register_job_name(WORKER_JOB_RCV_DATA, "receive");
    worker_register_job_name(WORKER_JOB_SND_DATA, "send");
    worker_register_job_name(WORKER_JOB_PROCESS, "process");

    CLEANUP_FUNCTION_REGISTER(socket_listen_main_static_threaded_worker_cleanup) cleanup_ptr = worker_private;
    poll_events(&api_sockets
                , web_server_add_callback
                , web_server_del_callback
                , web_server_rcv_callback
                , web_server_snd_callback
                , NULL
                , web_server_should_stop
                , web_allow_connections_from
                , web_allow_connections_dns
                , NULL
                , web_client_first_request_timeout
                , web_client_timeout
                , nd_profile.update_every * 1000 // timer_milliseconds
                , ptr // timer_data
                , worker_private->max_sockets
    );
}


// ----------------------------------------------------------------------------
// web server main thread - also becomes a worker

static void socket_listen_main_static_threaded_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    netdata_log_info("closing all web server sockets...");
    listen_sockets_close(&api_sockets);

    netdata_log_info("all static web threads stopped.");

    // Lets join all threads
    for (int i = 1; i < static_threaded_workers_count; i++) {
        bool initializing;
        do {
            spinlock_lock(&static_workers_private_data[i].spinlock);
            initializing = static_workers_private_data[i].initializing;
            spinlock_unlock(&static_workers_private_data[i].spinlock);
            if (unlikely(initializing))
                sleep_usec(1000);
        } while(initializing);
        (void) nd_thread_join(static_workers_private_data[i].thread);
    }

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void socket_listen_main_static_threaded(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(socket_listen_main_static_threaded_cleanup) cleanup_ptr = ptr;
    web_server_mode = WEB_SERVER_MODE_STATIC_THREADED;

    if(!api_sockets.opened)
        fatal("LISTENER: no listen sockets available.");

    netdata_ssl_validate_certificate = !inicfg_get_boolean(&netdata_config, CONFIG_SECTION_WEB, "ssl skip certificate verification", !netdata_ssl_validate_certificate);

    if(!netdata_ssl_validate_certificate_sender)
        netdata_log_info("SSL: web server will skip SSL certificates verification.");

    netdata_ssl_initialize_ctx(NETDATA_SSL_WEB_SERVER_CTX);

    static_threaded_workers_count = netdata_conf_web_query_threads();

    size_t max_sockets = (size_t)inicfg_get_number(&netdata_config, CONFIG_SECTION_WEB, "web server max sockets",
                                                   (long long int)(rlimit_nofile.rlim_cur / 4));

    static_workers_private_data = callocz((size_t)static_threaded_workers_count,
                                          sizeof(struct web_server_static_threaded_worker));

    int i;
    spinlock_init(&static_workers_private_data[0].spinlock);
    static_workers_private_data[0].initializing = true;
    for (i = 1; i < static_threaded_workers_count; i++) {
        static_workers_private_data[i].id = i;
        static_workers_private_data[i].max_sockets = max_sockets / static_threaded_workers_count;

        char tag[50 + 1];
        snprintfz(tag, sizeof(tag) - 1, "WEB[%d]", i+1);

        spinlock_init(&static_workers_private_data[i].spinlock);
        static_workers_private_data[i].initializing = true;
        static_workers_private_data[i].thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT,
                                                                 socket_listen_main_static_threaded_worker,
                                                                 (void *)&static_workers_private_data[i]);
    }

    // and the main one
    static_workers_private_data[0].max_sockets = max_sockets / static_threaded_workers_count;
    socket_listen_main_static_threaded_worker((void *)&static_workers_private_data[0]);
}
