// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"

/*
 * rrdpush
 *
 * 3 threads are involved for all stream operations
 *
 * 1. a random data collection thread, calling rrdset_done_push()
 *    this is called for each chart.
 *
 *    the output of this work is kept in a BUFFER in RRDHOST
 *    the sender thread is signalled via a pipe (also in RRDHOST)
 *
 * 2. a sender thread running at the sending netdata
 *    this is spawned automatically on the first chart to be pushed
 *
 *    It tries to push the metrics to the remote netdata, as fast
 *    as possible (i.e. immediately after they are collected).
 *
 * 3. a receiver thread, running at the receiving netdata
 *    this is spawned automatically when the sender connects to
 *    the receiver.
 *
 */

#define START_STREAMING_PROMPT "Hit me baby, push them over..."

typedef enum {
    RRDPUSH_MULTIPLE_CONNECTIONS_ALLOW,
    RRDPUSH_MULTIPLE_CONNECTIONS_DENY_NEW
} RRDPUSH_MULTIPLE_CONNECTIONS_STRATEGY;

static struct config stream_config = {
        .sections = NULL,
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .index = {
                .avl_tree = {
                        .root = NULL,
                        .compar = appconfig_section_compare
                },
                .rwlock = AVL_LOCK_INITIALIZER
        }
};

unsigned int default_rrdpush_enabled = 0;
char *default_rrdpush_destination = NULL;
char *default_rrdpush_api_key = NULL;
char *default_rrdpush_send_charts_matching = NULL;

static void load_stream_conf() {
    errno = 0;
    char *filename = strdupz_path_subpath(netdata_configured_user_config_dir, "stream.conf");
    if(!appconfig_load(&stream_config, filename, 0)) {
        info("CONFIG: cannot load user config '%s'. Will try stock config.", filename);
        freez(filename);

        filename = strdupz_path_subpath(netdata_configured_stock_config_dir, "stream.conf");
        if(!appconfig_load(&stream_config, filename, 0))
            info("CONFIG: cannot load stock config '%s'. Running with internal defaults.", filename);
    }
    freez(filename);
}

int rrdpush_init() {
    // --------------------------------------------------------------------
    // load stream.conf
    load_stream_conf();

    default_rrdpush_enabled     = (unsigned int)appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "enabled", default_rrdpush_enabled);
    default_rrdpush_destination = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "destination", "");
    default_rrdpush_api_key     = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "api key", "");
    default_rrdpush_send_charts_matching      = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "send charts matching", "*");
    rrdhost_free_orphan_time    = config_get_number(CONFIG_SECTION_GLOBAL, "cleanup orphan hosts after seconds", rrdhost_free_orphan_time);

    if(default_rrdpush_enabled && (!default_rrdpush_destination || !*default_rrdpush_destination || !default_rrdpush_api_key || !*default_rrdpush_api_key)) {
        error("STREAM [send]: cannot enable sending thread - information is missing.");
        default_rrdpush_enabled = 0;
    }

    return default_rrdpush_enabled;
}

#define CONNECTED_TO_SIZE 100

// data collection happens from multiple threads
// each of these threads calls rrdset_done()
// which in turn calls rrdset_done_push()
// which uses this pipe to notify the streaming thread
// that there are more data ready to be sent
#define PIPE_READ 0
#define PIPE_WRITE 1

// to have the remote netdata re-sync the charts
// to its current clock, we send for this many
// iterations a BEGIN line without microseconds
// this is for the first iterations of each chart
unsigned int remote_clock_resync_iterations = 60;

#define rrdpush_buffer_lock(host) netdata_mutex_lock(&((host)->rrdpush_sender_buffer_mutex))
#define rrdpush_buffer_unlock(host) netdata_mutex_unlock(&((host)->rrdpush_sender_buffer_mutex))

static inline int should_send_chart_matching(RRDSET *st) {
    if(unlikely(!rrdset_flag_check(st, RRDSET_FLAG_ENABLED))) {
        rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_SEND);
        rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_IGNORE);
    }
    else if(!rrdset_flag_check(st, RRDSET_FLAG_UPSTREAM_SEND|RRDSET_FLAG_UPSTREAM_IGNORE)) {
        RRDHOST *host = st->rrdhost;

        if(simple_pattern_matches(host->rrdpush_send_charts_matching, st->id) ||
            simple_pattern_matches(host->rrdpush_send_charts_matching, st->name)) {
            rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_IGNORE);
            rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_SEND);
        }
        else {
            rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_SEND);
            rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_IGNORE);
        }
    }

    return(rrdset_flag_check(st, RRDSET_FLAG_UPSTREAM_SEND));
}

// checks if the current chart definition has been sent
static inline int need_to_send_chart_definition(RRDSET *st) {
    rrdset_check_rdlock(st);

    if(unlikely(!(rrdset_flag_check(st, RRDSET_FLAG_UPSTREAM_EXPOSED))))
        return 1;

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(unlikely(!rd->exposed)) {
            #ifdef NETDATA_INTERNAL_CHECKS
            info("host '%s', chart '%s', dimension '%s' flag 'exposed' triggered chart refresh to upstream", st->rrdhost->hostname, st->id, rd->id);
            #endif
            return 1;
        }
    }

    return 0;
}

// sends the current chart definition
static inline void rrdpush_send_chart_definition_nolock(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

    // properly set the name for the remote end to parse it
    char *name = "";
    if(unlikely(strcmp(st->id, st->name))) {
        // they differ
        name = strchr(st->name, '.');
        if(name)
            name++;
        else
            name = "";
    }

    // info("CHART '%s' '%s'", st->id, name);

    // send the chart
    buffer_sprintf(
            host->rrdpush_sender_buffer
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

    // send the dimensions
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        buffer_sprintf(
                host->rrdpush_sender_buffer
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
                    host->rrdpush_sender_buffer
                    , "VARIABLE CHART %s = " CALCULATED_NUMBER_FORMAT "\n"
                    , rs->variable
                    , *value
            );
        }
    }

    st->upstream_resync_time = st->last_collected_time.tv_sec + (remote_clock_resync_iterations * st->update_every);
}

// sends the current chart dimensions
static inline void rrdpush_send_chart_metrics_nolock(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    buffer_sprintf(host->rrdpush_sender_buffer, "BEGIN \"%s\" %llu\n", st->id, (st->last_collected_time.tv_sec > st->upstream_resync_time)?st->usec_since_last_update:0);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(rd->updated && rd->exposed)
            buffer_sprintf(host->rrdpush_sender_buffer
                           , "SET \"%s\" = " COLLECTED_NUMBER_FORMAT "\n"
                           , rd->id
                           , rd->collected_value
        );
    }

    buffer_strcat(host->rrdpush_sender_buffer, "END\n");
}

static void rrdpush_sender_thread_spawn(RRDHOST *host);

void rrdset_push_chart_definition_now(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    if(unlikely(!host->rrdpush_send_enabled || !should_send_chart_matching(st)))
        return;

    rrdset_rdlock(st);
    rrdpush_buffer_lock(host);
    rrdpush_send_chart_definition_nolock(st);
    rrdpush_buffer_unlock(host);
    rrdset_unlock(st);
}

void rrdset_done_push(RRDSET *st) {
    if(unlikely(!should_send_chart_matching(st)))
        return;

    RRDHOST *host = st->rrdhost;

    rrdpush_buffer_lock(host);

    if(unlikely(host->rrdpush_send_enabled && !host->rrdpush_sender_spawn))
        rrdpush_sender_thread_spawn(host);

    if(unlikely(!host->rrdpush_sender_buffer || !host->rrdpush_sender_connected)) {
        if(unlikely(!host->rrdpush_sender_error_shown))
            error("STREAM %s [send]: not ready - discarding collected metrics.", host->hostname);

        host->rrdpush_sender_error_shown = 1;

        rrdpush_buffer_unlock(host);
        return;
    }
    else if(unlikely(host->rrdpush_sender_error_shown)) {
        info("STREAM %s [send]: sending metrics...", host->hostname);
        host->rrdpush_sender_error_shown = 0;
    }

    if(need_to_send_chart_definition(st))
        rrdpush_send_chart_definition_nolock(st);

    rrdpush_send_chart_metrics_nolock(st);

    // signal the sender there are more data
    if(host->rrdpush_sender_pipe[PIPE_WRITE] != -1 && write(host->rrdpush_sender_pipe[PIPE_WRITE], " ", 1) == -1)
        error("STREAM %s [send]: cannot write to internal pipe", host->hostname);

    rrdpush_buffer_unlock(host);
}

// ----------------------------------------------------------------------------
// rrdpush sender thread

static inline void rrdpush_sender_add_host_variable_to_buffer_nolock(RRDHOST *host, RRDVAR *rv) {
    calculated_number *value = (calculated_number *)rv->value;

    buffer_sprintf(
            host->rrdpush_sender_buffer
            , "VARIABLE HOST %s = " CALCULATED_NUMBER_FORMAT "\n"
            , rv->name
            , *value
    );

    debug(D_STREAM, "RRDVAR pushed HOST VARIABLE %s = " CALCULATED_NUMBER_FORMAT, rv->name, *value);
}

void rrdpush_sender_send_this_host_variable_now(RRDHOST *host, RRDVAR *rv) {
    if(host->rrdpush_send_enabled && host->rrdpush_sender_spawn && host->rrdpush_sender_connected) {
        rrdpush_buffer_lock(host);
        rrdpush_sender_add_host_variable_to_buffer_nolock(host, rv);
        rrdpush_buffer_unlock(host);
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
    int ret = rrdvar_callback_for_all_host_variables(host, rrdpush_sender_thread_custom_host_variables_callback, host);
    (void)ret;

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
    rrdpush_buffer_lock(host);

    if(buffer_strlen(host->rrdpush_sender_buffer))
        error("STREAM %s [send]: discarding %zu bytes of metrics already in the buffer.", host->hostname, buffer_strlen(host->rrdpush_sender_buffer));

    buffer_flush(host->rrdpush_sender_buffer);

    rrdpush_sender_thread_reset_all_charts(host);
    rrdpush_sender_thread_send_custom_host_variables(host);

    rrdpush_buffer_unlock(host);
}

void rrdpush_sender_thread_stop(RRDHOST *host) {
    rrdpush_buffer_lock(host);
    rrdhost_wrlock(host);

    netdata_thread_t thr = 0;

    if(host->rrdpush_sender_spawn) {
        info("STREAM %s [send]: signaling sending thread to stop...", host->hostname);

        // signal the thread that we want to join it
        host->rrdpush_sender_join = 1;

        // copy the thread id, so that we will be waiting for the right one
        // even if a new one has been spawn
        thr = host->rrdpush_sender_thread;

        // signal it to cancel
        netdata_thread_cancel(host->rrdpush_sender_thread);
    }

    rrdhost_unlock(host);
    rrdpush_buffer_unlock(host);

    if(thr != 0) {
        info("STREAM %s [send]: waiting for the sending thread to stop...", host->hostname);
        void *result;
        netdata_thread_join(thr, &result);
        info("STREAM %s [send]: sending thread has exited.", host->hostname);
    }
}

static inline void rrdpush_sender_thread_close_socket(RRDHOST *host) {
    host->rrdpush_sender_connected = 0;

    if(host->rrdpush_sender_socket != -1) {
        close(host->rrdpush_sender_socket);
        host->rrdpush_sender_socket = -1;
    }
}

static int rrdpush_sender_thread_connect_to_master(RRDHOST *host, int default_port, int timeout, size_t *reconnects_counter, char *connected_to, size_t connected_to_size) {
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
            , reconnects_counter
            , connected_to
            , connected_to_size
    );

    if(unlikely(host->rrdpush_sender_socket == -1)) {
        error("STREAM %s [send to %s]: failed to connect", host->hostname, host->rrdpush_send_destination);
        return 0;
    }

    info("STREAM %s [send to %s]: initializing communication...", host->hostname, connected_to);

    #define HTTP_HEADER_SIZE 8192
    char http[HTTP_HEADER_SIZE + 1];
    snprintfz(http, HTTP_HEADER_SIZE,
            "STREAM key=%s&hostname=%s&registry_hostname=%s&machine_guid=%s&update_every=%d&os=%s&timezone=%s&tags=%s HTTP/1.1\r\n"
                    "User-Agent: %s/%s\r\n"
                    "Accept: */*\r\n\r\n"
              , host->rrdpush_send_api_key
              , host->hostname
              , host->registry_hostname
              , host->machine_guid
              , default_rrd_update_every
              , host->os
              , host->timezone
              , (host->tags)?host->tags:""
              , host->program_name
              , host->program_version
    );

    if(send_timeout(host->rrdpush_sender_socket, http, strlen(http), 0, timeout) == -1) {
        error("STREAM %s [send to %s]: failed to send HTTP header to remote netdata.", host->hostname, connected_to);
        rrdpush_sender_thread_close_socket(host);
        return 0;
    }

    info("STREAM %s [send to %s]: waiting response from remote netdata...", host->hostname, connected_to);

    if(recv_timeout(host->rrdpush_sender_socket, http, HTTP_HEADER_SIZE, 0, timeout) == -1) {
        error("STREAM %s [send to %s]: remote netdata does not respond.", host->hostname, connected_to);
        rrdpush_sender_thread_close_socket(host);
        return 0;
    }

    if(strncmp(http, START_STREAMING_PROMPT, strlen(START_STREAMING_PROMPT)) != 0) {
        error("STREAM %s [send to %s]: server is not replying properly (is it a netdata?).", host->hostname, connected_to);
        rrdpush_sender_thread_close_socket(host);
        return 0;
    }

    info("STREAM %s [send to %s]: established communication - ready to send metrics...", host->hostname, connected_to);

    if(sock_setnonblock(host->rrdpush_sender_socket) < 0)
        error("STREAM %s [send to %s]: cannot set non-blocking mode for socket.", host->hostname, connected_to);

    if(sock_enlarge_out(host->rrdpush_sender_socket) < 0)
        error("STREAM %s [send to %s]: cannot enlarge the socket buffer.", host->hostname, connected_to);

    debug(D_STREAM, "STREAM: Connected on fd %d...", host->rrdpush_sender_socket);

    return 1;
}

static void rrdpush_sender_thread_cleanup_callback(void *ptr) {
    RRDHOST *host = (RRDHOST *)ptr;

    rrdpush_buffer_lock(host);
    rrdhost_wrlock(host);

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

    buffer_free(host->rrdpush_sender_buffer);
    host->rrdpush_sender_buffer = NULL;

    if(!host->rrdpush_sender_join) {
        info("STREAM %s [send]: sending thread detaches itself.", host->hostname);
        netdata_thread_detach(netdata_thread_self());
    }

    host->rrdpush_sender_spawn = 0;

    info("STREAM %s [send]: sending thread now exits.", host->hostname);

    rrdhost_unlock(host);
    rrdpush_buffer_unlock(host);
}

void *rrdpush_sender_thread(void *ptr) {
    RRDHOST *host = (RRDHOST *)ptr;

    if(!host->rrdpush_send_enabled || !host->rrdpush_send_destination || !*host->rrdpush_send_destination || !host->rrdpush_send_api_key || !*host->rrdpush_send_api_key) {
        error("STREAM %s [send]: thread created (task id %d), but host has streaming disabled.", host->hostname, gettid());
        return NULL;
    }

    info("STREAM %s [send]: thread created (task id %d)", host->hostname, gettid());

    int timeout = (int)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "timeout seconds", 60);
    int default_port = (int)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "default port", 19999);
    size_t max_size = (size_t)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "buffer size bytes", 1024 * 1024);
    unsigned int reconnect_delay = (unsigned int)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "reconnect delay seconds", 5);
    remote_clock_resync_iterations = (unsigned int)appconfig_get_number(&stream_config, CONFIG_SECTION_STREAM, "initial clock resync iterations", remote_clock_resync_iterations);
    char connected_to[CONNECTED_TO_SIZE + 1] = "";

    // initialize rrdpush globals
    host->rrdpush_sender_buffer = buffer_create(1);
    host->rrdpush_sender_connected = 0;
    if(pipe(host->rrdpush_sender_pipe) == -1) fatal("STREAM %s [send]: cannot create required pipe.", host->hostname);

    // initialize local variables
    size_t begin = 0;
    size_t reconnects_counter = 0;
    size_t sent_bytes = 0;
    size_t sent_bytes_on_this_connection = 0;
    size_t send_attempts = 0;


    time_t last_sent_t = 0;
    struct pollfd fds[2], *ifd, *ofd;
    nfds_t fdmax;

    ifd = &fds[0];
    ofd = &fds[1];

    size_t not_connected_loops = 0;

    netdata_thread_cleanup_push(rrdpush_sender_thread_cleanup_callback, host);

        for(; host->rrdpush_send_enabled && !netdata_exit ;) {
            // check for outstanding cancellation requests
            netdata_thread_testcancel();

            // if we don't have socket open, lets wait a bit
            if(unlikely(host->rrdpush_sender_socket == -1)) {
                send_attempts = 0;

                if(not_connected_loops == 0 && sent_bytes_on_this_connection > 0) {
                    // fast re-connection on first disconnect
                    sleep_usec(USEC_PER_MS * 500); // milliseconds
                }
                else {
                    // slow re-connection on repeating errors
                    sleep_usec(USEC_PER_SEC * reconnect_delay); // seconds
                }

                if(rrdpush_sender_thread_connect_to_master(host, default_port, timeout, &reconnects_counter, connected_to, CONNECTED_TO_SIZE)) {
                    last_sent_t = now_monotonic_sec();

                    // reset the buffer, to properly send charts and metrics
                    rrdpush_sender_thread_data_flush(host);

                    // send from the beginning
                    begin = 0;

                    // make sure the next reconnection will be immediate
                    not_connected_loops = 0;

                    // reset the bytes we have sent for this session
                    sent_bytes_on_this_connection = 0;

                    // let the data collection threads know we are ready
                    host->rrdpush_sender_connected = 1;
                }
                else {
                    // increase the failed connections counter
                    not_connected_loops++;

                    // reset the number of bytes sent
                    sent_bytes_on_this_connection = 0;
                }

                // loop through
                continue;
            }
            else if(unlikely(now_monotonic_sec() - last_sent_t > timeout)) {
                error("STREAM %s [send to %s]: could not send metrics for %d seconds - closing connection - we have sent %zu bytes on this connection via %zu send attempts.", host->hostname, connected_to, timeout, sent_bytes_on_this_connection, send_attempts);
                rrdpush_sender_thread_close_socket(host);
            }

            ifd->fd = host->rrdpush_sender_pipe[PIPE_READ];
            ifd->events = POLLIN;
            ifd->revents = 0;

            ofd->fd = host->rrdpush_sender_socket;
            ofd->revents = 0;
            if(ofd->fd != -1 && begin < buffer_strlen(host->rrdpush_sender_buffer)) {
                debug(D_STREAM, "STREAM: Requesting data output on streaming socket %d...", ofd->fd);
                ofd->events = POLLOUT;
                fdmax = 2;
                send_attempts++;
            }
            else {
                debug(D_STREAM, "STREAM: Not requesting data output on streaming socket %d (nothing to send now)...", ofd->fd);
                ofd->events = 0;
                fdmax = 1;
            }

            debug(D_STREAM, "STREAM: Waiting for poll() events (current buffer length %zu bytes)...", buffer_strlen(host->rrdpush_sender_buffer));
            if(unlikely(netdata_exit)) break;
            int retval = poll(fds, fdmax, 1000);
            if(unlikely(netdata_exit)) break;

            if(unlikely(retval == -1)) {
                debug(D_STREAM, "STREAM: poll() failed (current buffer length %zu bytes)...", buffer_strlen(host->rrdpush_sender_buffer));

                if(errno == EAGAIN || errno == EINTR) {
                    debug(D_STREAM, "STREAM: poll() failed with EAGAIN or EINTR...");
                }
                else {
                    error("STREAM %s [send to %s]: failed to poll(). Closing socket.", host->hostname, connected_to);
                    rrdpush_sender_thread_close_socket(host);
                }

                continue;
            }
            else if(likely(retval)) {
                if (ifd->revents & POLLIN || ifd->revents & POLLPRI) {
                    debug(D_STREAM, "STREAM: Data added to send buffer (current buffer length %zu bytes)...", buffer_strlen(host->rrdpush_sender_buffer));

                    char buffer[1000 + 1];
                    if (read(host->rrdpush_sender_pipe[PIPE_READ], buffer, 1000) == -1)
                        error("STREAM %s [send to %s]: cannot read from internal pipe.", host->hostname, connected_to);
                }

                if (ofd->revents & POLLOUT) {
                    if (begin < buffer_strlen(host->rrdpush_sender_buffer)) {
                        debug(D_STREAM, "STREAM: Sending data (current buffer length %zu bytes, begin = %zu)...", buffer_strlen(host->rrdpush_sender_buffer), begin);

                        // BEGIN RRDPUSH LOCKED SESSION

                        // during this session, data collectors
                        // will not be able to append data to our buffer
                        // but the socket is in non-blocking mode
                        // so, we will not block at send()

                        netdata_thread_disable_cancelability();

                        debug(D_STREAM, "STREAM: Getting exclusive lock on host...");
                        rrdpush_buffer_lock(host);

                        debug(D_STREAM, "STREAM: Sending data, starting from %zu, size %zu...", begin, buffer_strlen(host->rrdpush_sender_buffer));
                        ssize_t ret = send(host->rrdpush_sender_socket, &host->rrdpush_sender_buffer->buffer[begin], buffer_strlen(host->rrdpush_sender_buffer) - begin, MSG_DONTWAIT);
                        if (unlikely(ret == -1)) {
                            if (errno != EAGAIN && errno != EINTR && errno != EWOULDBLOCK) {
                                debug(D_STREAM, "STREAM: Send failed - closing socket...");
                                error("STREAM %s [send to %s]: failed to send metrics - closing connection - we have sent %zu bytes on this connection.", host->hostname, connected_to, sent_bytes_on_this_connection);
                                rrdpush_sender_thread_close_socket(host);
                            }
                            else {
                                debug(D_STREAM, "STREAM: Send failed - will retry...");
                            }
                        }
                        else if (likely(ret > 0)) {
                            // DEBUG - dump the string to see it
                            //char c = host->rrdpush_sender_buffer->buffer[begin + ret];
                            //host->rrdpush_sender_buffer->buffer[begin + ret] = '\0';
                            //debug(D_STREAM, "STREAM: sent from %zu to %zd:\n%s\n", begin, ret, &host->rrdpush_sender_buffer->buffer[begin]);
                            //host->rrdpush_sender_buffer->buffer[begin + ret] = c;

                            sent_bytes_on_this_connection += ret;
                            sent_bytes += ret;
                            begin += ret;

                            if (begin == buffer_strlen(host->rrdpush_sender_buffer)) {
                                // we send it all

                                debug(D_STREAM, "STREAM: Sent %zd bytes (the whole buffer)...", ret);
                                buffer_flush(host->rrdpush_sender_buffer);
                                begin = 0;
                            }
                            else {
                                debug(D_STREAM, "STREAM: Sent %zd bytes (part of the data buffer)...", ret);
                            }

                            last_sent_t = now_monotonic_sec();
                        }
                        else {
                            debug(D_STREAM, "STREAM: send() returned %zd - closing the socket...", ret);
                            error("STREAM %s [send to %s]: failed to send metrics (send() returned %zd) - closing connection - we have sent %zu bytes on this connection.",
                                  host->hostname, connected_to, ret, sent_bytes_on_this_connection);
                            rrdpush_sender_thread_close_socket(host);
                        }

                        debug(D_STREAM, "STREAM: Releasing exclusive lock on host...");
                        rrdpush_buffer_unlock(host);

                        netdata_thread_enable_cancelability();

                        // END RRDPUSH LOCKED SESSION
                    }
                    else {
                        debug(D_STREAM, "STREAM: we have sent the entire buffer, but we received POLLOUT...");
                    }
                }

                if(host->rrdpush_sender_socket != -1) {
                    char *error = NULL;

                    if (unlikely(ofd->revents & POLLERR))
                        error = "socket reports errors (POLLERR)";

                    else if (unlikely(ofd->revents & POLLHUP))
                        error = "connection closed by remote end (POLLHUP)";

                    else if (unlikely(ofd->revents & POLLNVAL))
                        error = "connection is invalid (POLLNVAL)";

                    if(unlikely(error)) {
                        debug(D_STREAM, "STREAM: %s - closing socket...", error);
                        error("STREAM %s [send to %s]: %s - reopening socket - we have sent %zu bytes on this connection.", host->hostname, connected_to, error, sent_bytes_on_this_connection);
                        rrdpush_sender_thread_close_socket(host);
                    }
                }
            }
            else {
                debug(D_STREAM, "STREAM: poll() timed out.");
            }

            // protection from overflow
            if(buffer_strlen(host->rrdpush_sender_buffer) > max_size) {
                debug(D_STREAM, "STREAM: Buffer is too big (%zu bytes), bigger than the max (%zu) - flushing it...", buffer_strlen(host->rrdpush_sender_buffer), max_size);
                errno = 0;
                error("STREAM %s [send to %s]: too many data pending - buffer is %zu bytes long, %zu unsent - we have sent %zu bytes in total, %zu on this connection. Closing connection to flush the data.", host->hostname, connected_to, host->rrdpush_sender_buffer->len, host->rrdpush_sender_buffer->len - begin, sent_bytes, sent_bytes_on_this_connection);
                rrdpush_sender_thread_close_socket(host);
            }
        }

    netdata_thread_cleanup_pop(1);
    return NULL;
}


// ----------------------------------------------------------------------------
// rrdpush receiver thread

static void log_stream_connection(const char *client_ip, const char *client_port, const char *api_key, const char *machine_guid, const char *host, const char *msg) {
    log_access("STREAM: %d '[%s]:%s' '%s' host '%s' api key '%s' machine guid '%s'", gettid(), client_ip, client_port, msg, host, api_key, machine_guid);
}

static RRDPUSH_MULTIPLE_CONNECTIONS_STRATEGY get_multiple_connections_strategy(struct config *c, const char *section, const char *name, RRDPUSH_MULTIPLE_CONNECTIONS_STRATEGY def) {
    char *value;
    switch(def) {
        default:
        case RRDPUSH_MULTIPLE_CONNECTIONS_ALLOW:
            value = "allow";
            break;

        case RRDPUSH_MULTIPLE_CONNECTIONS_DENY_NEW:
            value = "deny";
            break;
    }

    value = appconfig_get(c, section, name, value);

    RRDPUSH_MULTIPLE_CONNECTIONS_STRATEGY ret = def;

    if(strcasecmp(value, "allow") == 0 || strcasecmp(value, "permit") == 0 || strcasecmp(value, "accept") == 0)
        ret = RRDPUSH_MULTIPLE_CONNECTIONS_ALLOW;

    else if(strcasecmp(value, "deny") == 0 || strcasecmp(value, "reject") == 0 || strcasecmp(value, "block") == 0)
        ret = RRDPUSH_MULTIPLE_CONNECTIONS_DENY_NEW;

    else
        error("Invalid stream config value at section [%s], setting '%s', value '%s'", section, name, value);

    return ret;
}

static int rrdpush_receive(int fd
                           , const char *key
                           , const char *hostname
                           , const char *registry_hostname
                           , const char *machine_guid
                           , const char *os
                           , const char *timezone
                           , const char *tags
                           , const char *program_name
                           , const char *program_version
                           , int update_every
                           , char *client_ip
                           , char *client_port
) {
    RRDHOST *host;
    int history = default_rrd_history_entries;
    RRD_MEMORY_MODE mode = default_rrd_memory_mode;
    int health_enabled = default_health_enabled;
    int rrdpush_enabled = default_rrdpush_enabled;
    char *rrdpush_destination = default_rrdpush_destination;
    char *rrdpush_api_key = default_rrdpush_api_key;
    char *rrdpush_send_charts_matching = default_rrdpush_send_charts_matching;
    time_t alarms_delay = 60;
    RRDPUSH_MULTIPLE_CONNECTIONS_STRATEGY rrdpush_multiple_connections_strategy = RRDPUSH_MULTIPLE_CONNECTIONS_ALLOW;

    update_every = (int)appconfig_get_number(&stream_config, machine_guid, "update every", update_every);
    if(update_every < 0) update_every = 1;

    history = (int)appconfig_get_number(&stream_config, key, "default history", history);
    history = (int)appconfig_get_number(&stream_config, machine_guid, "history", history);
    if(history < 5) history = 5;

    mode = rrd_memory_mode_id(appconfig_get(&stream_config, key, "default memory mode", rrd_memory_mode_name(mode)));
    mode = rrd_memory_mode_id(appconfig_get(&stream_config, machine_guid, "memory mode", rrd_memory_mode_name(mode)));

    health_enabled = appconfig_get_boolean_ondemand(&stream_config, key, "health enabled by default", health_enabled);
    health_enabled = appconfig_get_boolean_ondemand(&stream_config, machine_guid, "health enabled", health_enabled);

    alarms_delay = appconfig_get_number(&stream_config, key, "default postpone alarms on connect seconds", alarms_delay);
    alarms_delay = appconfig_get_number(&stream_config, machine_guid, "postpone alarms on connect seconds", alarms_delay);

    rrdpush_enabled = appconfig_get_boolean(&stream_config, key, "default proxy enabled", rrdpush_enabled);
    rrdpush_enabled = appconfig_get_boolean(&stream_config, machine_guid, "proxy enabled", rrdpush_enabled);

    rrdpush_destination = appconfig_get(&stream_config, key, "default proxy destination", rrdpush_destination);
    rrdpush_destination = appconfig_get(&stream_config, machine_guid, "proxy destination", rrdpush_destination);

    rrdpush_api_key = appconfig_get(&stream_config, key, "default proxy api key", rrdpush_api_key);
    rrdpush_api_key = appconfig_get(&stream_config, machine_guid, "proxy api key", rrdpush_api_key);

    rrdpush_multiple_connections_strategy = get_multiple_connections_strategy(&stream_config, key, "multiple connections", rrdpush_multiple_connections_strategy);
    rrdpush_multiple_connections_strategy = get_multiple_connections_strategy(&stream_config, machine_guid, "multiple connections", rrdpush_multiple_connections_strategy);

    rrdpush_send_charts_matching = appconfig_get(&stream_config, key, "default proxy send charts matching", rrdpush_send_charts_matching);
    rrdpush_send_charts_matching = appconfig_get(&stream_config, machine_guid, "proxy send charts matching", rrdpush_send_charts_matching);

    tags = appconfig_set_default(&stream_config, machine_guid, "host tags", (tags)?tags:"");
    if(tags && !*tags) tags = NULL;

    if(!strcmp(machine_guid, "localhost"))
        host = localhost;
    else
        host = rrdhost_find_or_create(
                hostname
                , registry_hostname
                , machine_guid
                , os
                , timezone
                , tags
                , program_name
                , program_version
                , update_every
                , history
                , mode
                , (unsigned int)(health_enabled != CONFIG_BOOLEAN_NO)
                , (unsigned int)(rrdpush_enabled && rrdpush_destination && *rrdpush_destination && rrdpush_api_key && *rrdpush_api_key)
                , rrdpush_destination
                , rrdpush_api_key
                , rrdpush_send_charts_matching
        );

    if(!host) {
        close(fd);
        log_stream_connection(client_ip, client_port, key, machine_guid, hostname, "FAILED - CANNOT ACQUIRE HOST");
        error("STREAM %s [receive from [%s]:%s]: failed to find/create host structure.", hostname, client_ip, client_port);
        return 1;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    info("STREAM %s [receive from [%s]:%s]: client willing to stream metrics for host '%s' with machine_guid '%s': update every = %d, history = %ld, memory mode = %s, health %s, tags '%s'"
         , hostname
         , client_ip
         , client_port
         , host->hostname
         , host->machine_guid
         , host->rrd_update_every
         , host->rrd_history_entries
         , rrd_memory_mode_name(host->rrd_memory_mode)
         , (health_enabled == CONFIG_BOOLEAN_NO)?"disabled":((health_enabled == CONFIG_BOOLEAN_YES)?"enabled":"auto")
         , host->tags?host->tags:""
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
    };

    // put the client IP and port into the buffers used by plugins.d
    snprintfz(cd.id,           CONFIG_MAX_NAME,  "%s:%s", client_ip, client_port);
    snprintfz(cd.filename,     FILENAME_MAX,     "%s:%s", client_ip, client_port);
    snprintfz(cd.fullfilename, FILENAME_MAX,     "%s:%s", client_ip, client_port);
    snprintfz(cd.cmd,          PLUGINSD_CMD_MAX, "%s:%s", client_ip, client_port);

    info("STREAM %s [receive from [%s]:%s]: initializing communication...", host->hostname, client_ip, client_port);
    if(send_timeout(fd, START_STREAMING_PROMPT, strlen(START_STREAMING_PROMPT), 0, 60) != strlen(START_STREAMING_PROMPT)) {
        log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "FAILED - CANNOT REPLY");
        error("STREAM %s [receive from [%s]:%s]: cannot send ready command.", host->hostname, client_ip, client_port);
        close(fd);
        return 0;
    }

    // remove the non-blocking flag from the socket
    if(sock_delnonblock(fd) < 0)
        error("STREAM %s [receive from [%s]:%s]: cannot remove the non-blocking flag from socket %d", host->hostname, client_ip, client_port, fd);

    // convert the socket to a FILE *
    FILE *fp = fdopen(fd, "r");
    if(!fp) {
        log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "FAILED - SOCKET ERROR");
        error("STREAM %s [receive from [%s]:%s]: failed to get a FILE for FD %d.", host->hostname, client_ip, client_port, fd);
        close(fd);
        return 0;
    }

    rrdhost_wrlock(host);
    if(host->connected_senders > 0) {
        switch(rrdpush_multiple_connections_strategy) {
            case RRDPUSH_MULTIPLE_CONNECTIONS_ALLOW:
                info("STREAM %s [receive from [%s]:%s]: multiple streaming connections for the same host detected. If multiple netdata are pushing metrics for the same charts, at the same time, the result is unexpected.", host->hostname, client_ip, client_port);
                break;

            case RRDPUSH_MULTIPLE_CONNECTIONS_DENY_NEW:
                rrdhost_unlock(host);
                log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "REJECTED - ALREADY CONNECTED");
                info("STREAM %s [receive from [%s]:%s]: multiple streaming connections for the same host detected. Rejecting new connection.", host->hostname, client_ip, client_port);
                fclose(fp);
                return 0;
        }
    }

    rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);
    host->connected_senders++;
    host->senders_disconnected_time = 0;
    if(health_enabled != CONFIG_BOOLEAN_NO) {
        if(alarms_delay > 0) {
            host->health_delay_up_to = now_realtime_sec() + alarms_delay;
            info("Postponing health checks for %ld seconds, on host '%s', because it was just connected."
            , alarms_delay
            , host->hostname
            );
        }
    }
    rrdhost_unlock(host);

    // call the plugins.d processor to receive the metrics
    info("STREAM %s [receive from [%s]:%s]: receiving metrics...", host->hostname, client_ip, client_port);
    log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "CONNECTED");

    size_t count = pluginsd_process(host, &cd, fp, 1);

    log_stream_connection(client_ip, client_port, key, host->machine_guid, host->hostname, "DISCONNECTED");
    error("STREAM %s [receive from [%s]:%s]: disconnected (completed %zu updates).", host->hostname, client_ip, client_port, count);

    rrdhost_wrlock(host);
    host->senders_disconnected_time = now_realtime_sec();
    host->connected_senders--;
    if(!host->connected_senders) {
        rrdhost_flag_set(host, RRDHOST_FLAG_ORPHAN);
        if(health_enabled == CONFIG_BOOLEAN_AUTO)
            host->health_enabled = 0;
    }
    rrdhost_unlock(host);

    if(host->connected_senders == 0)
        rrdpush_sender_thread_stop(host);

    // cleanup
    fclose(fp);

    return (int)count;
}

struct rrdpush_thread {
    int fd;
    char *key;
    char *hostname;
    char *registry_hostname;
    char *machine_guid;
    char *os;
    char *timezone;
    char *tags;
    char *client_ip;
    char *client_port;
    char *program_name;
    char *program_version;
    int update_every;
};

static void rrdpush_receiver_thread_cleanup(void *ptr) {
    static __thread int executed = 0;
    if(!executed) {
        executed = 1;
        struct rrdpush_thread *rpt = (struct rrdpush_thread *) ptr;

        info("STREAM %s [receive from [%s]:%s]: receive thread ended (task id %d)", rpt->hostname, rpt->client_ip, rpt->client_port, gettid());

        freez(rpt->key);
        freez(rpt->hostname);
        freez(rpt->registry_hostname);
        freez(rpt->machine_guid);
        freez(rpt->os);
        freez(rpt->timezone);
        freez(rpt->tags);
        freez(rpt->client_ip);
        freez(rpt->client_port);
        freez(rpt->program_name);
        freez(rpt->program_version);
        freez(rpt);
    }
}

static void *rrdpush_receiver_thread(void *ptr) {
    netdata_thread_cleanup_push(rrdpush_receiver_thread_cleanup, ptr);

        struct rrdpush_thread *rpt = (struct rrdpush_thread *)ptr;
        info("STREAM %s [%s]:%s: receive thread created (task id %d)", rpt->hostname, rpt->client_ip, rpt->client_port, gettid());

        rrdpush_receive(
                rpt->fd
                , rpt->key
                , rpt->hostname
                , rpt->registry_hostname
                , rpt->machine_guid
                , rpt->os
                , rpt->timezone
                , rpt->tags
                , rpt->program_name
                , rpt->program_version
                , rpt->update_every
                , rpt->client_ip
                , rpt->client_port
        );

    netdata_thread_cleanup_pop(1);
    return NULL;
}

static void rrdpush_sender_thread_spawn(RRDHOST *host) {
    rrdhost_wrlock(host);

    if(!host->rrdpush_sender_spawn) {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "STREAM_SENDER[%s]", host->hostname);

        if(netdata_thread_create(&host->rrdpush_sender_thread, tag, NETDATA_THREAD_OPTION_JOINABLE, rrdpush_sender_thread, (void *) host))
            error("STREAM %s [send]: failed to create new thread for client.", host->hostname);
        else
            host->rrdpush_sender_spawn = 1;
    }

    rrdhost_unlock(host);
}

int rrdpush_receiver_permission_denied(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_sprintf(w->response.data, "You are not permitted to access this. Check the logs for more info.");
    return 401;
}

int rrdpush_receiver_too_busy_now(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_sprintf(w->response.data, "The server is too busy now to accept this request. Try later.");
    return 503;
}

int rrdpush_receiver_thread_spawn(RRDHOST *host, struct web_client *w, char *url) {
    (void)host;

    info("clients wants to STREAM metrics.");

    char *key = NULL, *hostname = NULL, *registry_hostname = NULL, *machine_guid = NULL, *os = "unknown", *timezone = "unknown", *tags = NULL;
    int update_every = default_rrd_update_every;
    char buf[GUID_LEN + 1];

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
        else if(!strcmp(name, "os"))
            os = value;
        else if(!strcmp(name, "timezone"))
            timezone = value;
        else if(!strcmp(name, "tags"))
            tags = value;
        else
            info("STREAM [receive from [%s]:%s]: request has parameter '%s' = '%s', which is not used.", w->client_ip, w->client_port, key, value);
    }

    if(!key || !*key) {
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO KEY");
        error("STREAM [receive from [%s]:%s]: request without an API key. Forbidding access.", w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!hostname || !*hostname) {
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO HOSTNAME");
        error("STREAM [receive from [%s]:%s]: request without a hostname. Forbidding access.", w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!machine_guid || !*machine_guid) {
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - NO MACHINE GUID");
        error("STREAM [receive from [%s]:%s]: request without a machine GUID. Forbidding access.", w->client_ip, w->client_port);
        return rrdpush_receiver_permission_denied(w);
    }

    if(regenerate_guid(key, buf) == -1) {
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - INVALID KEY");
        error("STREAM [receive from [%s]:%s]: API key '%s' is not valid GUID (use the command uuidgen to generate one). Forbidding access.", w->client_ip, w->client_port, key);
        return rrdpush_receiver_permission_denied(w);
    }

    if(regenerate_guid(machine_guid, buf) == -1) {
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - INVALID MACHINE GUID");
        error("STREAM [receive from [%s]:%s]: machine GUID '%s' is not GUID. Forbidding access.", w->client_ip, w->client_port, machine_guid);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!appconfig_get_boolean(&stream_config, key, "enabled", 0)) {
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - KEY NOT ENABLED");
        error("STREAM [receive from [%s]:%s]: API key '%s' is not allowed. Forbidding access.", w->client_ip, w->client_port, key);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *key_allow_from = simple_pattern_create(appconfig_get(&stream_config, key, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT);
        if(key_allow_from) {
            if(!simple_pattern_matches(key_allow_from, w->client_ip)) {
                simple_pattern_free(key_allow_from);
                log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname) ? hostname : "-", "ACCESS DENIED - KEY NOT ALLOWED FROM THIS IP");
                error("STREAM [receive from [%s]:%s]: API key '%s' is not permitted from this IP. Forbidding access.", w->client_ip, w->client_port, key);
                return rrdpush_receiver_permission_denied(w);
            }
            simple_pattern_free(key_allow_from);
        }
    }

    if(!appconfig_get_boolean(&stream_config, machine_guid, "enabled", 1)) {
        log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname)?hostname:"-", "ACCESS DENIED - MACHINE GUID NOT ENABLED");
        error("STREAM [receive from [%s]:%s]: machine GUID '%s' is not allowed. Forbidding access.", w->client_ip, w->client_port, machine_guid);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *machine_allow_from = simple_pattern_create(appconfig_get(&stream_config, machine_guid, "allow from", "*"), NULL, SIMPLE_PATTERN_EXACT);
        if(machine_allow_from) {
            if(!simple_pattern_matches(machine_allow_from, w->client_ip)) {
                simple_pattern_free(machine_allow_from);
                log_stream_connection(w->client_ip, w->client_port, (key && *key)?key:"-", (machine_guid && *machine_guid)?machine_guid:"-", (hostname && *hostname) ? hostname : "-", "ACCESS DENIED - MACHINE GUID NOT ALLOWED FROM THIS IP");
                error("STREAM [receive from [%s]:%s]: Machine GUID '%s' is not permitted from this IP. Forbidding access.", w->client_ip, w->client_port, machine_guid);
                return rrdpush_receiver_permission_denied(w);
            }
            simple_pattern_free(machine_allow_from);
        }
    }

    if(unlikely(web_client_streaming_rate_t > 0)) {
        static netdata_mutex_t stream_rate_mutex = NETDATA_MUTEX_INITIALIZER;
        static volatile time_t last_stream_accepted_t = 0;

        netdata_mutex_lock(&stream_rate_mutex);
        time_t now = now_realtime_sec();

        if(unlikely(last_stream_accepted_t == 0))
            last_stream_accepted_t = now;

        if(now - last_stream_accepted_t < web_client_streaming_rate_t) {
            netdata_mutex_unlock(&stream_rate_mutex);
            error("STREAM [receive from [%s]:%s]: too busy to accept new streaming request. Will be allowed in %ld secs.", w->client_ip, w->client_port, (long)(web_client_streaming_rate_t - (now - last_stream_accepted_t)));
            return rrdpush_receiver_too_busy_now(w);
        }

        last_stream_accepted_t = now;
        netdata_mutex_unlock(&stream_rate_mutex);
    }

    struct rrdpush_thread *rpt = callocz(1, sizeof(struct rrdpush_thread));
    rpt->fd                = w->ifd;
    rpt->key               = strdupz(key);
    rpt->hostname          = strdupz(hostname);
    rpt->registry_hostname = strdupz((registry_hostname && *registry_hostname)?registry_hostname:hostname);
    rpt->machine_guid      = strdupz(machine_guid);
    rpt->os                = strdupz(os);
    rpt->timezone          = strdupz(timezone);
    rpt->tags              = (tags)?strdupz(tags):NULL;
    rpt->client_ip         = strdupz(w->client_ip);
    rpt->client_port       = strdupz(w->client_port);
    rpt->update_every      = update_every;

    if(w->user_agent && w->user_agent[0]) {
        char *t = strchr(w->user_agent, '/');
        if(t && *t) {
            *t = '\0';
            t++;
        }

        rpt->program_name = strdupz(w->user_agent);
        if(t && *t) rpt->program_version = strdupz(t);
    }

    netdata_thread_t thread;

    debug(D_SYSTEM, "starting STREAM receive thread.");

    char tag[FILENAME_MAX + 1];
    snprintfz(tag, FILENAME_MAX, "STREAM_RECEIVER[%s,[%s]:%s]", rpt->hostname, w->client_ip, w->client_port);

    if(netdata_thread_create(&thread, tag, NETDATA_THREAD_OPTION_DEFAULT, rrdpush_receiver_thread, (void *)rpt))
        error("Failed to create new STREAM receive thread for client.");

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
