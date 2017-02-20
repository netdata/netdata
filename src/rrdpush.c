#include "common.h"

#define PIPE_READ 0
#define PIPE_WRITE 1

int rrdpush_pipe[2];

static BUFFER *rrdpush_buffer = NULL;
static pthread_mutex_t rrdpush_mutex = PTHREAD_MUTEX_INITIALIZER;
static RRDHOST *last_host = NULL;

static inline void rrdpush_lock() {
    pthread_mutex_lock(&rrdpush_mutex);
}

static inline void rrdpush_unlock() {
    pthread_mutex_unlock(&rrdpush_mutex);
}

static inline int need_to_send_chart_definitions(RRDSET *st) {
    RRDDIM *rd;
    rrddim_foreach_read(rd, st)
        if(rrddim_flag_check(rd, RRDDIM_FLAG_UPDATED) && !rrddim_flag_check(rd, RRDDIM_FLAG_EXPOSED))
            return 1;

    return 0;
}

static inline void send_chart_definitions(RRDSET *st) {
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

static inline void send_chart_metrics(RRDSET *st) {
    buffer_sprintf(rrdpush_buffer, "BEGIN %s %llu\n", st->id, st->usec_since_last_update);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(rrddim_flag_check(rd, RRDDIM_FLAG_UPDATED))
            buffer_sprintf(rrdpush_buffer, "SET %s = " COLLECTED_NUMBER_FORMAT "\n"
                       , rd->id
                       , rd->collected_value
        );
    }

    buffer_strcat(rrdpush_buffer, "END\n");
}

static void reset_all_charts(void) {
    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host) {
        rrdhost_rdlock(host);

        RRDSET *st;
        rrdset_foreach_read(st, host) {
            rrdset_rdlock(st);

            RRDDIM *rd;
            rrddim_foreach_read(rd, st)
                rrddim_flag_clear(rd, RRDDIM_FLAG_EXPOSED);

            rrdset_unlock(st);
        }
        rrdhost_unlock(host);
    }
    rrd_unlock();

    last_host = NULL;
}

void rrdset_done_push(RRDSET *st) {

    if(unlikely(!rrdset_flag_check(st, RRDSET_FLAG_ENABLED) || !rrdpush_buffer))
        return;

    rrdpush_lock();
    rrdset_rdlock(st);

    if(st->rrdhost != last_host) {
        buffer_sprintf(rrdpush_buffer, "HOST '%s' '%s'\n", st->rrdhost->machine_guid, st->rrdhost->hostname);
        last_host = st->rrdhost;
    }

    if(need_to_send_chart_definitions(st))
        send_chart_definitions(st);

    send_chart_metrics(st);

    // signal the sender there are more data
    if(write(rrdpush_pipe[PIPE_WRITE], " ", 1) == -1)
        error("Cannot write to internal pipe");

    rrdset_unlock(st);
    rrdpush_unlock();
}

void *central_netdata_push_thread(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("Central netdata push thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");


    rrdpush_buffer = buffer_create(1);

    if(pipe(rrdpush_pipe) == -1)
        fatal("Cannot create required pipe.");

    struct timeval tv = {
            .tv_sec = 60,
            .tv_usec = 0
    };

    size_t begin = 0;
    size_t max_size = 1024 * 1024;
    size_t reconnects_counter = 0;
    size_t sent_bytes = 0;
    size_t sent_connection = 0;
    int sock = -1;
    char buffer[1000 + 1];

    struct pollfd fds[2], *ifd, *ofd;

    ifd = &fds[0];
    ofd = &fds[1];

    ifd->fd = rrdpush_pipe[PIPE_READ];
    ifd->events = POLLIN;
    ofd->events = POLLOUT;

    nfds_t fdmax = 2;

    for(;;) {
        if(netdata_exit) break;

        if(unlikely(sock == -1)) {
            info("PUSH: connecting to central netdata at: %s", central_netdata_to_push_data);
            sock = connect_to_one_of(central_netdata_to_push_data, 19999, &tv, &reconnects_counter);

            if(unlikely(sock == -1)) {
                error("PUSH: failed to connect to central netdata at: %s", central_netdata_to_push_data);
                sleep(5);
                continue;
            }

            info("PUSH: connected to central netdata at: %s", central_netdata_to_push_data);

            char http[1000 + 1];
            snprintfz(http, 1000, "GET /stream?key=%s HTTP/1.1\r\nUser-Agent: netdata-push-service/%s\r\nAccept: */*\r\n\r\n", config_get("global", "central netdata api key", ""), program_version);
            if(send_timeout(sock, http, strlen(http), 0, 60) == -1) {
                close(sock);
                sock = -1;
                error("PUSH: failed to send http header to netdata at: %s", central_netdata_to_push_data);
                sleep(5);
                continue;
            }

            if(recv_timeout(sock, http, 1000, 0, 60) == -1) {
                close(sock);
                sock = -1;
                error("PUSH: failed to receive OK from netdata at: %s", central_netdata_to_push_data);
                sleep(5);
                continue;
            }

            if(strncmp(http, "STREAM", 6)) {
                close(sock);
                sock = -1;
                error("PUSH: netdata servers at  %s, did not send STREAM", central_netdata_to_push_data);
                sleep(5);
                continue;
            }

            if(fcntl(sock, F_SETFL, O_NONBLOCK) < 0)
                error("PUSH: cannot set non-blocking mode for socket.");

            rrdpush_lock();
            if(buffer_strlen(rrdpush_buffer))
                error("PUSH: discarding %zu bytes of metrics data already in the buffer.", buffer_strlen(rrdpush_buffer));

            buffer_flush(rrdpush_buffer);
            reset_all_charts();
            last_host = NULL;
            rrdpush_unlock();
            sent_connection = 0;
        }

        ifd->revents = 0;
        ofd->revents = 0;
        ofd->fd = sock;

        if(begin < buffer_strlen(rrdpush_buffer))
            ofd->events = POLLOUT;
        else
            ofd->events = 0;

        if(netdata_exit) break;
        int retval = poll(fds, fdmax, 60 * 1000);
        if(netdata_exit) break;

        if(unlikely(retval == -1)) {
            if(errno == EAGAIN || errno == EINTR)
                continue;

            error("PUSH: Failed to poll().");
            close(sock);
            sock = -1;
            break;
        }
        else if(unlikely(!retval)) {
            // timeout
            continue;
        }

        if(ifd->revents & POLLIN) {
            if(read(rrdpush_pipe[PIPE_READ], buffer, 1000) == -1)
                error("PUSH: Cannot read from internal pipe.");
        }

        if(ofd->revents & POLLOUT && begin < buffer_strlen(rrdpush_buffer)) {
            // fprintf(stderr, "PUSH BEGIN\n");
            // fwrite(&rrdpush_buffer->buffer[begin], 1, buffer_strlen(rrdpush_buffer) - begin, stderr);
            // fprintf(stderr, "\nPUSH END\n");

            rrdpush_lock();
            ssize_t ret = send(sock, &rrdpush_buffer->buffer[begin], buffer_strlen(rrdpush_buffer) - begin, MSG_DONTWAIT);
            if(ret == -1) {
                if(errno != EAGAIN && errno != EINTR) {
                    error("PUSH: failed to send metrics to central netdata at %s. We have sent %zu bytes on this connection.", central_netdata_to_push_data, sent_connection);
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
            error("PUSH: too many data pending. Buffer is %zu bytes long, %zu unsent. We have sent %zu bytes in total, %zu on this connection. Closing connection to flush the data.", rrdpush_buffer->len, rrdpush_buffer->len - begin, sent_bytes, sent_connection);
            if(sock != -1) {
                close(sock);
                sock = -1;
            }
        }
    }

    debug(D_WEB_CLIENT, "Central netdata push thread exits.");
    if(sock != -1)
        close(sock);

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}
