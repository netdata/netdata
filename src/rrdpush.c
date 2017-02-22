#include "common.h"

int rrdpush_enabled = 0;
int rrdpush_exclusive = 1;

static char *central_netdata = NULL;
static char *api_key = NULL;

#define CONNECTED_TO_SIZE 100

// data collection happens from multiple threads
// each of these threads calls rrdset_done()
// which in turn calls rrdset_done_push()
// which uses this pipe to notify the streaming thread
// that there are more data ready to be sent
#define PIPE_READ 0
#define PIPE_WRITE 1
int rrdpush_pipe[2] = { -1, -1 };

// a buffer used to store data to be sent.
// the format is the same as external plugins.
static BUFFER *rrdpush_buffer = NULL;

// locking to get exclusive access to shared resources
// (rrdpush_pipe[PIPE_WRITE], rrdpush_buffer
static pthread_mutex_t rrdpush_mutex = PTHREAD_MUTEX_INITIALIZER;

// if the streaming thread is connected to a central netdata
// this is set to 1, otherwise 0.
static volatile int rrdpush_connected = 0;

// to have the remote netdata re-sync the charts
// to its current clock, we send for this many
// iterations a BEGIN line without microseconds
// this is for the first iterations of each chart
static unsigned int remote_clock_resync_iterations = 60;

#define rrdpush_lock() pthread_mutex_lock(&rrdpush_mutex)
#define rrdpush_unlock() pthread_mutex_unlock(&rrdpush_mutex)

// checks if the current chart definition has been sent
static inline int need_to_send_chart_definition(RRDSET *st) {
    RRDDIM *rd;
    rrddim_foreach_read(rd, st)
        if(!rrddim_flag_check(rd, RRDDIM_FLAG_EXPOSED))
            return 1;

    return 0;
}

// sends the current chart definition
static inline void send_chart_definition(RRDSET *st) {
    buffer_sprintf(rrdpush_buffer, "CHART '%s' '%s' '%s' '%s' '%s' '%s' '%s' %ld %d\n"
                , st->id
                , st->name
                , st->title
                , st->units
                , st->family
                , st->context
                , rrdset_type_name(st->chart_type)
                , st->priority
                , st->update_every
    );

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        buffer_sprintf(rrdpush_buffer, "DIMENSION '%s' '%s' '%s' " COLLECTED_NUMBER_FORMAT " " COLLECTED_NUMBER_FORMAT " '%s %s'\n"
                       , rd->id
                       , rd->name
                       , rrd_algorithm_name(rd->algorithm)
                       , rd->multiplier
                       , rd->divisor
                       , rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)?"hidden":""
                       , rrddim_flag_check(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)?"noreset":""
        );
        rrddim_flag_set(rd, RRDDIM_FLAG_EXPOSED);
    }
}

// sends the current chart dimensions
static inline void send_chart_metrics(RRDSET *st) {
    buffer_sprintf(rrdpush_buffer, "BEGIN %s %llu\n", st->id, (st->counter_done > remote_clock_resync_iterations)?st->usec_since_last_update:0);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(rrddim_flag_check(rd, RRDDIM_FLAG_UPDATED) && rrddim_flag_check(rd, RRDDIM_FLAG_EXPOSED))
            buffer_sprintf(rrdpush_buffer, "SET %s = " COLLECTED_NUMBER_FORMAT "\n"
                       , rd->id
                       , rd->collected_value
        );
    }

    buffer_strcat(rrdpush_buffer, "END\n");
}

// resets all the chart, so that their definitions
// will be resent to the central netdata
static void reset_all_charts(void) {
    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host) {
        rrdhost_rdlock(host);

        RRDSET *st;
        rrdset_foreach_read(st, host) {

            // make it re-align the current time
            // on the remote host
            st->counter_done = 0;

            rrdset_rdlock(st);

            RRDDIM *rd;
            rrddim_foreach_read(rd, st)
                rrddim_flag_clear(rd, RRDDIM_FLAG_EXPOSED);

            rrdset_unlock(st);
        }
        rrdhost_unlock(host);
    }
    rrd_unlock();
}

void rrdset_done_push(RRDSET *st) {
    static int error_shown = 0;

    if(unlikely(!rrdset_flag_check(st, RRDSET_FLAG_ENABLED)))
        return;

    rrdpush_lock();

    if(unlikely(!rrdpush_buffer || !rrdpush_connected)) {
        if(!error_shown)
            error("STREAM: not ready - discarding collected metrics.");

        error_shown = 1;

        rrdpush_unlock();
        return;
    }
    error_shown = 0;

    rrdset_rdlock(st);
    if(need_to_send_chart_definition(st))
        send_chart_definition(st);

    send_chart_metrics(st);
    rrdset_unlock(st);

    // signal the sender there are more data
    if(write(rrdpush_pipe[PIPE_WRITE], " ", 1) == -1)
        error("STREAM: cannot write to internal pipe");

    rrdpush_unlock();
}

static inline void rrdpush_flush(void) {
    rrdpush_lock();
    if(buffer_strlen(rrdpush_buffer))
        error("STREAM: discarding %zu bytes of metrics data already in the buffer.", buffer_strlen(rrdpush_buffer));

    buffer_flush(rrdpush_buffer);
    reset_all_charts();
    rrdpush_unlock();
}

int rrdpush_init() {
    rrdpush_enabled = config_get_boolean("stream", "enabled", rrdpush_enabled);
    rrdpush_exclusive = config_get_boolean("stream", "exclusive", rrdpush_exclusive);
    central_netdata = config_get("stream", "stream metrics to", "");
    api_key = config_get("stream", "api key", "");

    if(!rrdpush_enabled || !central_netdata || !*central_netdata || !api_key || !*api_key) {
        rrdpush_enabled = 0;
        rrdpush_exclusive = 0;
    }

    return rrdpush_enabled;
}

void *central_netdata_push_thread(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("STREAM: central netdata push thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("STREAM: cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("STREAM: cannot set pthread cancel state to ENABLE.");

    int timeout = (int)config_get_number("stream", "timeout seconds", 60);
    int default_port = (int)config_get_number("stream", "default port", 19999);
    size_t max_size = (size_t)config_get_number("stream", "buffer size bytes", 1024 * 1024);
    unsigned int reconnect_delay = (unsigned int)config_get_number("stream", "reconnect delay seconds", 5);
    remote_clock_resync_iterations = (unsigned int)config_get_number("stream", "initial clock resync iterations", remote_clock_resync_iterations);
    int sock = -1;

    if(!rrdpush_enabled || !central_netdata || !*central_netdata || !api_key || !*api_key)
        goto cleanup;

    // initialize rrdpush globals
    rrdpush_buffer = buffer_create(1);
    rrdpush_connected = 0;
    if(pipe(rrdpush_pipe) == -1) fatal("STREAM: cannot create required pipe.");

    // initialize local variables
    size_t begin = 0;
    size_t reconnects_counter = 0;
    size_t sent_bytes = 0;
    size_t sent_connection = 0;

    struct timeval tv = {
            .tv_sec = timeout,
            .tv_usec = 0
    };

    struct pollfd fds[2], *ifd, *ofd;
    nfds_t fdmax;

    ifd = &fds[0];
    ofd = &fds[1];

    char connected_to[CONNECTED_TO_SIZE + 1];

    for(;;) {
        if(netdata_exit) break;

        if(unlikely(sock == -1)) {
            // stop appending data into rrdpush_buffer
            // they will be lost, so there is no point to do it
            rrdpush_connected = 0;

            info("STREAM: connecting to central netdata at: %s", central_netdata);
            sock = connect_to_one_of(central_netdata, default_port, &tv, &reconnects_counter, connected_to, CONNECTED_TO_SIZE);

            if(unlikely(sock == -1)) {
                error("STREAM: failed to connect to central netdata at: %s", central_netdata);
                sleep(reconnect_delay);
                continue;
            }

            info("STREAM: initializing communication to central netdata at: %s", connected_to);

            char http[1000 + 1];
            snprintfz(http, 1000,
                    "STREAM key=%s&hostname=%s&machine_guid=%s&os=%s&update_every=%d HTTP/1.1\r\n"
                    "User-Agent: netdata-push-service/%s\r\n"
                    "Accept: */*\r\n\r\n"
                      , api_key
                      , localhost->hostname
                      , localhost->machine_guid
                      , localhost->os
                      , default_rrd_update_every
                      , program_version
            );

            if(send_timeout(sock, http, strlen(http), 0, timeout) == -1) {
                close(sock);
                sock = -1;
                error("STREAM: failed to send http header to netdata at: %s", connected_to);
                sleep(reconnect_delay);
                continue;
            }

            info("STREAM: Waiting for STREAM from central netdata at: %s", connected_to);

            if(recv_timeout(sock, http, 1000, 0, timeout) == -1) {
                close(sock);
                sock = -1;
                error("STREAM: failed to receive STREAM from netdata at: %s", connected_to);
                sleep(reconnect_delay);
                continue;
            }

            if(strncmp(http, "STREAM", 6)) {
                close(sock);
                sock = -1;
                error("STREAM: server at %s, did not send STREAM", connected_to);
                sleep(reconnect_delay);
                continue;
            }

            info("STREAM: Established communication with central netdata at: %s - sending metrics...", connected_to);

            if(fcntl(sock, F_SETFL, O_NONBLOCK) < 0)
                error("STREAM: cannot set non-blocking mode for socket.");

            rrdpush_flush();
            sent_connection = 0;

            // allow appending data into rrdpush_buffer
            rrdpush_connected = 1;
        }

        ifd->fd = rrdpush_pipe[PIPE_READ];
        ifd->events = POLLIN;
        ifd->revents = 0;

        ofd->fd = sock;
        ofd->revents = 0;
        if(begin < buffer_strlen(rrdpush_buffer)) {
            ofd->events = POLLOUT;
            fdmax = 2;
        }
        else {
            ofd->events = 0;
            fdmax = 1;
        }

        if(netdata_exit) break;
        int retval = poll(fds, fdmax, timeout * 1000);
        if(netdata_exit) break;

        if(unlikely(retval == -1)) {
            if(errno == EAGAIN || errno == EINTR)
                continue;

            error("STREAM: Failed to poll().");
            close(sock);
            sock = -1;
            break;
        }
        else if(unlikely(!retval)) {
            // timeout
            continue;
        }

        if(ifd->revents & POLLIN) {
            char buffer[1000 + 1];
            if(read(rrdpush_pipe[PIPE_READ], buffer, 1000) == -1)
                error("STREAM: Cannot read from internal pipe.");
        }

        if(ofd->revents & POLLOUT && begin < buffer_strlen(rrdpush_buffer)) {
            // info("STREAM: send buffer is ready, sending %zu bytes starting at %zu", buffer_strlen(rrdpush_buffer) - begin, begin);

            // fprintf(stderr, "PUSH BEGIN\n");
            // fwrite(&rrdpush_buffer->buffer[begin], 1, buffer_strlen(rrdpush_buffer) - begin, stderr);
            // fprintf(stderr, "\nPUSH END\n");

            rrdpush_lock();
            ssize_t ret = send(sock, &rrdpush_buffer->buffer[begin], buffer_strlen(rrdpush_buffer) - begin, MSG_DONTWAIT);
            if(ret == -1) {
                if(errno != EAGAIN && errno != EINTR) {
                    error("STREAM: failed to send metrics to central netdata at %s. We have sent %zu bytes on this connection.", connected_to, sent_connection);
                    close(sock);
                    sock = -1;
                }
            }
            else {
                sent_connection += ret;
                sent_bytes += ret;
                begin += ret;
                if(begin == buffer_strlen(rrdpush_buffer)) {
                    buffer_flush(rrdpush_buffer);
                    begin = 0;
                }
            }
            rrdpush_unlock();
        }

        // protection from overflow
        if(rrdpush_buffer->len > max_size) {
            errno = 0;
            error("STREAM: too many data pending. Buffer is %zu bytes long, %zu unsent. We have sent %zu bytes in total, %zu on this connection. Closing connection to flush the data.", rrdpush_buffer->len, rrdpush_buffer->len - begin, sent_bytes, sent_connection);
            if(sock != -1) {
                close(sock);
                sock = -1;
            }
        }
    }

cleanup:
    debug(D_WEB_CLIENT, "STREAM: central netdata push thread exits.");

    // make sure the data collection threads do not write data
    rrdpush_connected = 0;

    // close the pipe
    if(rrdpush_pipe[PIPE_READ] != -1)  close(rrdpush_pipe[PIPE_READ]);
    if(rrdpush_pipe[PIPE_WRITE] != -1) close(rrdpush_pipe[PIPE_WRITE]);

    // close the socket
    if(sock != -1) close(sock);

    rrdpush_lock();
    buffer_free(rrdpush_buffer);
    rrdpush_buffer = NULL;
    rrdpush_unlock();

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}
