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
    for(rd = st->dimensions; rd ;rd = rd->next)
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
    for(rd = st->dimensions; rd ;rd = rd->next) {
        buffer_sprintf(rrdpush_buffer, "DIMENSION '%s' '%s' '%s' " COLLECTED_NUMBER_FORMAT " " COLLECTED_NUMBER_FORMAT " '%s %s'\n"
                       , rd->id
                       , rd->name
                       , rrd_algorithm_name(rd->algorithm)
                       , rd->multiplier
                       , rd->divisor
                       , rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN)?"hidden":""
                       , rrddim_flag_check(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)?"noreset":""
        );
    }
}

static inline void send_chart_metrics(RRDSET *st) {
    buffer_sprintf(rrdpush_buffer, "BEGIN %s %llu\n", st->id, st->usec_since_last_update);

    RRDDIM *rd;
    for(rd = st->dimensions; rd ;rd = rd->next) {
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

    RRDHOST *h;
    for(h = localhost; h ;h = h->next) {
        RRDSET *st;
        for(st = h->rrdset_root ; st ; st = st->next) {
            rrdset_rdlock(st);

            RRDDIM *rd;
            for(rd = st->dimensions; rd ;rd = rd->next)
                rrddim_flag_clear(rd, RRDDIM_FLAG_EXPOSED);

            rrdset_unlock(st);
        }
    }

    last_host = NULL;

    rrd_unlock();
}

void rrdset_done_push(RRDSET *st) {

    if(!rrdset_flag_check(st, RRDSET_FLAG_ENABLED))
        return;

    rrdpush_lock();
    rrdset_rdlock(st);

    if(st->rrdhost != last_host)
        buffer_sprintf(rrdpush_buffer, "HOST '%s' '%s'\n", st->rrdhost->hostname, st->rrdhost->machine_guid);

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
    int sock = -1;
    char buffer[1];

    for(;;) {
        if(unlikely(sock == -1)) {
            sock = connect_to_one_of(central_netdata_to_push_data, 19999, &tv, &reconnects_counter);

            if(unlikely(sock != -1)) {
                if(fcntl(sock, F_SETFL, O_NONBLOCK) < 0)
                    error("Cannot set non-blocking mode for socket.");

                buffer_sprintf(rrdpush_buffer, "GET /stream?key=%s\r\n\r\n", config_get("global", "central netdata api key", ""));
                reset_all_charts();
            }
        }

        if(read(rrdpush_pipe[PIPE_READ], buffer, 1) == -1) {
            error("Cannot read from internal pipe.");
            sleep(1);
        }

        if(likely(sock != -1)) {
            rrdpush_lock();
            ssize_t ret = send(sock, &rrdpush_buffer->buffer[begin], rrdpush_buffer->len, MSG_DONTWAIT);
            if(ret == -1) {
                error("Failed to send metrics to central netdata at %s", central_netdata_to_push_data);
                close(sock);
                sock = -1;
            }
            else {
                begin += ret;
                if(begin == rrdpush_buffer->len) {
                    buffer_flush(rrdpush_buffer);
                    begin = 0;
                }
            }
            rrdpush_unlock();
        }

        // protection from overflow
        if(rrdpush_buffer->len > max_size) {
            rrdpush_lock();

            error("Discarding %zu bytes of metrics data, because we cannot connect to central netdata at %s"
                  , buffer_strlen(rrdpush_buffer), central_netdata_to_push_data);

            buffer_flush(rrdpush_buffer);

            if(sock != -1) {
                close(sock);
                sock = -1;
            }

            rrdpush_unlock();
        }
    }

cleanup:
    debug(D_WEB_CLIENT, "Central netdata push thread exits.");
    if(sock != -1)
        close(sock);

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}
