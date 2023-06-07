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

    w = web_client_get_from_cache();
    w->ifd = w->ofd = pi->fd;

    strncpyz(w->client_ip,   pi->client_ip,   sizeof(w->client_ip) - 1);
    strncpyz(w->client_port, pi->client_port, sizeof(w->client_port) - 1);
    strncpyz(w->client_host, pi->client_host, sizeof(w->client_host) - 1);

    if(unlikely(!*w->client_ip))   strcpy(w->client_ip,   "-");
    if(unlikely(!*w->client_port)) strcpy(w->client_port, "-");
	w->port_acl = pi->port_acl;

    int flag = 1;
    if(unlikely(web_client_check_tcp(w) && setsockopt(w->ifd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0))
        debug(D_WEB_CLIENT, "%llu: failed to enable TCP_NODELAY on socket fd %d.", w->id, w->ifd);

    flag = 1;
    if(unlikely(setsockopt(w->ifd, SOL_SOCKET, SO_KEEPALIVE, (char *) &flag, sizeof(int)) != 0))
        debug(D_WEB_CLIENT, "%llu: failed to enable SO_KEEPALIVE on socket fd %d.", w->id, w->ifd);

    web_client_update_acl_matches(w);
    web_client_enable_wait_receive(w);

    web_server_log_connection(w, "CONNECTED");

    w->pollinfo_slot = pi->slot;
    return(w);
}

// --------------------------------------------------------------------------------------
// the main socket listener - STATIC-THREADED

struct web_server_static_threaded_worker {
    netdata_thread_t thread;

    int id;
    int running;

    size_t max_sockets;

    volatile size_t connected;
    volatile size_t disconnected;
    volatile size_t receptions;
    volatile size_t sends;
    volatile size_t max_concurrent;

    volatile size_t files_read;
    volatile size_t file_reads;
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
// web server files

static void *web_server_file_add_callback(POLLINFO *pi, short int *events, void *data) {
    struct web_client *w = (struct web_client *)data;

    worker_is_busy(WORKER_JOB_ADD_FILE);

    worker_private->files_read++;

    debug(D_WEB_CLIENT, "%llu: ADDED FILE READ ON FD %d", w->id, pi->fd);
    *events = POLLIN;
    pi->data = w;

    worker_is_idle();
    return w;
}

static void web_server_file_del_callback(POLLINFO *pi) {
    struct web_client *w = (struct web_client *)pi->data;
    debug(D_WEB_CLIENT, "%llu: RELEASE FILE READ ON FD %d", w->id, pi->fd);

    worker_is_busy(WORKER_JOB_DEL_FILE);

    w->pollinfo_filecopy_slot = 0;

    if(unlikely(!w->pollinfo_slot)) {
        debug(D_WEB_CLIENT, "%llu: CROSS WEB CLIENT CLEANUP (iFD %d, oFD %d)", w->id, pi->fd, w->ofd);
        web_server_log_connection(w, "DISCONNECTED");
        web_client_request_done(w);
        web_client_release_to_cache(w);
        global_statistics_web_client_disconnected();
    }

    worker_is_idle();
}

static int web_server_file_read_callback(POLLINFO *pi, short int *events) {
    int retval = -1;
    struct web_client *w = (struct web_client *)pi->data;

    worker_is_busy(WORKER_JOB_READ_FILE);

    // if there is no POLLINFO linked to this, it means the client disconnected
    // stop the file reading too
    if(unlikely(!w->pollinfo_slot)) {
        debug(D_WEB_CLIENT, "%llu: PREVENTED ATTEMPT TO READ FILE ON FD %d, ON CLOSED WEB CLIENT", w->id, pi->fd);
        retval = -1;
        goto cleanup;
    }

    if(unlikely(w->mode != WEB_CLIENT_MODE_FILECOPY || w->ifd == w->ofd)) {
        debug(D_WEB_CLIENT, "%llu: PREVENTED ATTEMPT TO READ FILE ON FD %d, ON NON-FILECOPY WEB CLIENT", w->id, pi->fd);
        retval = -1;
        goto cleanup;
    }

    debug(D_WEB_CLIENT, "%llu: READING FILE ON FD %d", w->id, pi->fd);

    worker_private->file_reads++;
    ssize_t ret = unlikely(web_client_read_file(w));

    if(likely(web_client_has_wait_send(w))) {
        POLLJOB *p = pi->p;                                        // our POLLJOB
        POLLINFO *wpi = pollinfo_from_slot(p, w->pollinfo_slot);  // POLLINFO of the client socket

        debug(D_WEB_CLIENT, "%llu: SIGNALING W TO SEND (iFD %d, oFD %d)", w->id, pi->fd, wpi->fd);
        p->fds[wpi->slot].events |= POLLOUT;
    }

    if(unlikely(ret <= 0 || w->ifd == w->ofd)) {
        debug(D_WEB_CLIENT, "%llu: DONE READING FILE ON FD %d", w->id, pi->fd);
        retval = -1;
        goto cleanup;
    }

    *events = POLLIN;
    retval = 0;

cleanup:
    worker_is_idle();
    return retval;
}

static int web_server_file_write_callback(POLLINFO *pi, short int *events) {
    (void)pi;
    (void)events;

    worker_is_busy(WORKER_JOB_WRITE_FILE);
    error("Writing to web files is not supported!");
    worker_is_idle();

    return -1;
}

// ----------------------------------------------------------------------------
// web server clients

static void *web_server_add_callback(POLLINFO *pi, short int *events, void *data) {
    (void)data;         // Suppress warning on unused argument

    worker_is_busy(WORKER_JOB_ADD_CONNECTION);
    worker_private->connected++;

    size_t concurrent = worker_private->connected - worker_private->disconnected;
    if(unlikely(concurrent > worker_private->max_concurrent))
        worker_private->max_concurrent = concurrent;

    *events = POLLIN;

    debug(D_WEB_CLIENT_ACCESS, "LISTENER on %d: new connection.", pi->fd);
    struct web_client *w = web_client_create_on_fd(pi);

    if (!strncmp(pi->client_port, "UNIX", 4)) {
        web_client_set_unix(w);
    } else {
        web_client_set_tcp(w);
    }

#ifdef ENABLE_HTTPS
    if ((!web_client_check_unix(w)) && (netdata_ssl_web_server_ctx)) {
        sock_delnonblock(w->ifd);

        //Read the first 7 bytes from the message, but the message
        //is not removed from the queue, because we are using MSG_PEEK
        char test[8];
        if ( recv(w->ifd,test, 7, MSG_PEEK) == 7 ) {
            test[7] = '\0';
        }
        else {
            // we couldn't read 7 bytes
            sock_setnonblock(w->ifd);
            goto cleanup;
        }

        if(test[0] > 0x17) {
            // no SSL
            netdata_ssl_close(&w->ssl); // free any previous SSL data
        }
        else {
            // SSL
            if(!netdata_ssl_open(&w->ssl, netdata_ssl_web_server_ctx, w->ifd) || !netdata_ssl_accept(&w->ssl))
                WEB_CLIENT_IS_DEAD(w);
        }

        sock_setnonblock(w->ifd);
    }
#endif

    debug(D_WEB_CLIENT, "%llu: ADDED CLIENT FD %d", w->id, pi->fd);

cleanup:
    worker_is_idle();
    return w;
}

// TCP client disconnected
static void web_server_del_callback(POLLINFO *pi) {
    worker_is_busy(WORKER_JOB_DEL_COLLECTION);

    worker_private->disconnected++;

    struct web_client *w = (struct web_client *)pi->data;

    w->pollinfo_slot = 0;
    if(unlikely(w->pollinfo_filecopy_slot)) {
        POLLINFO *fpi = pollinfo_from_slot(pi->p, w->pollinfo_filecopy_slot);  // POLLINFO of the client socket
        (void)fpi;

        debug(D_WEB_CLIENT, "%llu: THE CLIENT WILL BE FRED BY READING FILE JOB ON FD %d", w->id, fpi->fd);
    }
    else {
        if(web_client_flag_check(w, WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET))
            pi->flags |= POLLINFO_FLAG_DONT_CLOSE;

        debug(D_WEB_CLIENT, "%llu: CLOSING CLIENT FD %d", w->id, pi->fd);
        web_server_log_connection(w, "DISCONNECTED");
        web_client_request_done(w);
        web_client_release_to_cache(w);
        global_statistics_web_client_disconnected();
    }

    worker_is_idle();
}

static int web_server_rcv_callback(POLLINFO *pi, short int *events) {
    int ret = -1;
    worker_is_busy(WORKER_JOB_RCV_DATA);

    worker_private->receptions++;

    struct web_client *w = (struct web_client *)pi->data;
    int fd = pi->fd;

    ssize_t bytes;
    bytes = web_client_receive(w);

    if (likely(bytes > 0)) {
        debug(D_WEB_CLIENT, "%llu: processing received data on fd %d.", w->id, fd);
        worker_is_idle();
        worker_is_busy(WORKER_JOB_PROCESS);
        web_client_process_request(w);

        if (unlikely(w->mode == WEB_CLIENT_MODE_STREAM)) {
            web_client_send(w);
        }

        else if(unlikely(w->mode == WEB_CLIENT_MODE_FILECOPY)) {
            if(w->pollinfo_filecopy_slot == 0) {
                debug(D_WEB_CLIENT, "%llu: FILECOPY DETECTED ON FD %d", w->id, pi->fd);

                if (unlikely(w->ifd != -1 && w->ifd != w->ofd && w->ifd != fd)) {
                    // add a new socket to poll_events, with the same
                    debug(D_WEB_CLIENT, "%llu: CREATING FILECOPY SLOT ON FD %d", w->id, pi->fd);

                    POLLINFO *fpi = poll_add_fd(
                                                pi->p
                                                , w->ifd
                                                , pi->port_acl
                                                , 0
                                                , POLLINFO_FLAG_CLIENT_SOCKET
                                                , "FILENAME"
                                                , ""
                                                , ""
                                                , web_server_file_add_callback
                                                , web_server_file_del_callback
                                                , web_server_file_read_callback
                                                , web_server_file_write_callback
                                                , (void *) w
                                                );

                    if(fpi)
                        w->pollinfo_filecopy_slot = fpi->slot;
                    else {
                        error("Failed to add filecopy fd. Closing client.");
                        ret = -1;
                        goto cleanup;
                    }
                }
            }
        }
        else {
            if(unlikely(w->ifd == fd && web_client_has_wait_receive(w)))
                *events |= POLLIN;
        }

        if(unlikely(w->ofd == fd && web_client_has_wait_send(w)))
            *events |= POLLOUT;
    } else if(unlikely(bytes < 0)) {
        ret = -1;
        goto cleanup;
    } else if (unlikely(bytes == 0)) {
        if(unlikely(w->ifd == fd && web_client_has_ssl_wait_receive(w)))
            *events |= POLLIN;

        if(unlikely(w->ofd == fd && web_client_has_ssl_wait_send(w)))
            *events |= POLLOUT;
    }

    ret = web_server_check_client_status(w);

cleanup:
    worker_is_idle();
    return ret;
}

static int web_server_snd_callback(POLLINFO *pi, short int *events) {
    int retval = -1;
    worker_is_busy(WORKER_JOB_SND_DATA);

    worker_private->sends++;

    struct web_client *w = (struct web_client *)pi->data;
    int fd = pi->fd;

    debug(D_WEB_CLIENT, "%llu: sending data on fd %d.", w->id, fd);

    int ret = web_client_send(w);

    if(unlikely(ret < 0)) {
        retval = -1;
        goto cleanup;
    }

    if(unlikely(w->ifd == fd && web_client_has_wait_receive(w)))
        *events |= POLLIN;

    if(unlikely(w->ofd == fd && web_client_has_wait_send(w)))
        *events |= POLLOUT;

    retval = web_server_check_client_status(w);

cleanup:
    worker_is_idle();
    return retval;
}

// ----------------------------------------------------------------------------
// web server worker thread

static void socket_listen_main_static_threaded_worker_cleanup(void *ptr) {
    worker_private = (struct web_server_static_threaded_worker *)ptr;

    info("stopped after %zu connects, %zu disconnects (max concurrent %zu), %zu receptions and %zu sends",
            worker_private->connected,
            worker_private->disconnected,
            worker_private->max_concurrent,
            worker_private->receptions,
            worker_private->sends
    );

    worker_private->running = 0;
    worker_unregister();
}

static bool web_server_should_stop(void) {
    return !service_running(SERVICE_WEB_SERVER);
}

void *socket_listen_main_static_threaded_worker(void *ptr) {
    worker_private = (struct web_server_static_threaded_worker *)ptr;
    worker_private->running = 1;
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

    netdata_thread_cleanup_push(socket_listen_main_static_threaded_worker_cleanup, ptr);

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
                        , default_rrd_update_every * 1000 // timer_milliseconds
                        , ptr // timer_data
                        , worker_private->max_sockets
            );

    netdata_thread_cleanup_pop(1);
    return NULL;
}


// ----------------------------------------------------------------------------
// web server main thread - also becomes a worker

static void socket_listen_main_static_threaded_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

//    int i, found = 0;
//    usec_t max = 2 * USEC_PER_SEC, step = 50000;
//
//    // we start from 1, - 0 is self
//    for(i = 1; i < static_threaded_workers_count; i++) {
//        if(static_workers_private_data[i].running) {
//            found++;
//            info("stopping worker %d", i + 1);
//            netdata_thread_cancel(static_workers_private_data[i].thread);
//        }
//        else
//            info("found stopped worker %d", i + 1);
//    }
//
//    while(found && max > 0) {
//        max -= step;
//        info("Waiting %d static web threads to finish...", found);
//        sleep_usec(step);
//        found = 0;
//
//        // we start from 1, - 0 is self
//        for(i = 1; i < static_threaded_workers_count; i++) {
//            if (static_workers_private_data[i].running)
//                found++;
//        }
//    }
//
//    if(found)
//        error("%d static web threads are taking too long to finish. Giving up.", found);

    info("closing all web server sockets...");
    listen_sockets_close(&api_sockets);

    info("all static web threads stopped.");
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *socket_listen_main_static_threaded(void *ptr) {
    netdata_thread_cleanup_push(socket_listen_main_static_threaded_cleanup, ptr);
    web_server_mode = WEB_SERVER_MODE_STATIC_THREADED;

    if(!api_sockets.opened)
        fatal("LISTENER: no listen sockets available.");

    netdata_ssl_validate_certificate = !config_get_boolean(CONFIG_SECTION_WEB, "ssl skip certificate verification", !netdata_ssl_validate_certificate);

    if(!netdata_ssl_validate_certificate_sender)
        info("SSL: web server will skip SSL certificates verification.");

#ifdef ENABLE_HTTPS
    netdata_ssl_initialize_ctx(NETDATA_SSL_WEB_SERVER_CTX);
#endif

    // 6 threads is the optimal value
    // since 6 are the parallel connections browsers will do
    // so, if the machine has more CPUs, avoid using resources unnecessarily
    int def_thread_count = MIN(get_netdata_cpus(), 6);

    if (!strcmp(config_get(CONFIG_SECTION_WEB, "mode", ""),"single-threaded")) {
                info("Running web server with one thread, because mode is single-threaded");
                config_set(CONFIG_SECTION_WEB, "mode", "static-threaded");
                def_thread_count = 1;
    }
    static_threaded_workers_count = config_get_number(CONFIG_SECTION_WEB, "web server threads", def_thread_count);

    if (static_threaded_workers_count < 1) static_threaded_workers_count = 1;

#ifdef ENABLE_HTTPS
    // See https://github.com/netdata/netdata/issues/11081#issuecomment-831998240 for more details
    if (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110) {
        static_threaded_workers_count = 1;
        info("You are running an OpenSSL older than 1.1.0, web server will not enable multithreading.");
    }
#endif

    size_t max_sockets = (size_t)config_get_number(CONFIG_SECTION_WEB, "web server max sockets",
                                                   (long long int)(rlimit_nofile.rlim_cur / 4));

    static_workers_private_data = callocz((size_t)static_threaded_workers_count,
                                          sizeof(struct web_server_static_threaded_worker));

    web_server_is_multithreaded = (static_threaded_workers_count > 1);

    int i;
    for (i = 1; i < static_threaded_workers_count; i++) {
        static_workers_private_data[i].id = i;
        static_workers_private_data[i].max_sockets = max_sockets / static_threaded_workers_count;

        char tag[50 + 1];
        snprintfz(tag, 50, "WEB[%d]", i+1);

        info("starting worker %d", i+1);
        netdata_thread_create(&static_workers_private_data[i].thread, tag, NETDATA_THREAD_OPTION_DEFAULT,
                              socket_listen_main_static_threaded_worker, (void *)&static_workers_private_data[i]);
    }

    // and the main one
    static_workers_private_data[0].max_sockets = max_sockets / static_threaded_workers_count;
    socket_listen_main_static_threaded_worker((void *)&static_workers_private_data[0]);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
