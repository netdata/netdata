#define NETDATA_RRD_INTERNALS
#include "rrd.h"

#define MAX_FUNCTION_LENGTH (16384 - 1024)

// ----------------------------------------------------------------------------
// RRDSET - collector info and rrdset functions

// we keep a dictionary per RRDSET with these functions
// the dictionary is created on demand (only when a function is added to an RRDSET)

struct rrd_collector_function {
    bool sync;                      // when true, the function is called synchronously
    STRING *format;                 // the format the function produces
    STRING *help;
    int timeout;                    // the default timeout of the function
    int (*function)(BUFFER *wb, int timeout, const char *function, void *collector_data, void (*callback)(BUFFER *wb, int code, void *callback_data), void *callback_data);
    void *collector_data;
    struct rrd_collector *collector;
};

// Each function points to this collector structure
// so that when the collector exits, all of them will
// be invalidated (running == false)
// The last function that is using this collector
// frees the structure too (or when the collector calls
// rrdset_collector_finished()).

struct rrd_collector {
    void *input;
    void *output;
    int32_t refcount;
    pid_t tid;
    bool running;
};

// Each thread that adds RRDSET functions, has to call
// rrdset_collector_started() and rrdset_collector_finished()
// to create the collector structure.

static __thread struct rrd_collector *thread_rrd_collector = NULL;

static void rrd_collector_free(struct rrd_collector *rdc) {
    int32_t expected = 0;
    if(likely(!__atomic_compare_exchange_n(&rdc->refcount, &expected, -1, false, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST))) {
        // the collector is still referenced by charts.
        // leave it hanging there, the last chart will actually free it.
        return;
    }

    // we can free it now
    freez(rdc);
}

// called once per collector
void rrd_collector_started(void *input, void *output) {
    if(likely(thread_rrd_collector)) return;

    thread_rrd_collector = callocz(1, sizeof(struct rrd_collector));
    thread_rrd_collector->tid = gettid();
    thread_rrd_collector->running = true;
    thread_rrd_collector->input = input;
    thread_rrd_collector->output = output;
}

// called once per collector
void rrd_collector_finished(void) {
    if(!thread_rrd_collector)
        return;

    thread_rrd_collector->running = false;
    rrd_collector_free(thread_rrd_collector);
    thread_rrd_collector = NULL;
}

static struct rrd_collector *rrd_collector_acquire(void) {
    __atomic_add_fetch(&thread_rrd_collector->refcount, 1, __ATOMIC_SEQ_CST);
    return thread_rrd_collector;
}

static void rrd_collector_release(struct rrd_collector *rdc) {
    if(unlikely(!rdc)) return;

    int32_t refcount = __atomic_sub_fetch(&rdc->refcount, 1, __ATOMIC_SEQ_CST);
    if(refcount == 0 && !rdc->running)
        rrd_collector_free(rdc);
}

static void rrd_functions_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func __maybe_unused, void *rrdhost __maybe_unused) {
    struct rrd_collector_function *rdcf = func;

    if(!thread_rrd_collector)
        fatal("RRDSET_COLLECTOR: called %s() for function '%s' without calling rrd_collector_started() first.", __FUNCTION__, dictionary_acquired_item_name(item));

    rdcf->collector = rrd_collector_acquire();
}

static void rrd_functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func __maybe_unused, void *rrdhost __maybe_unused) {
    struct rrd_collector_function *rdcf = func;
    rrd_collector_release(rdcf->collector);
    string_freez(rdcf->format);
}

static bool rrd_functions_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func __maybe_unused, void *new_func __maybe_unused, void *rrdhost __maybe_unused) {
    struct rrd_collector_function *rdcf = func;
    struct rrd_collector_function *new_rdcf = new_func;

    if(!thread_rrd_collector)
        fatal("RRDSET_COLLECTOR: called %s() for function '%s' without calling rrd_collector_started() first.", __FUNCTION__, dictionary_acquired_item_name(item));

    bool changed = false;

    if(rdcf->collector != thread_rrd_collector) {
        struct rrd_collector *old_rdc = rdcf->collector;
        rdcf->collector = rrd_collector_acquire();
        rrd_collector_release(old_rdc);
        changed = true;
    }

    if(rdcf->function != new_rdcf->function) {
        rdcf->function = new_rdcf->function;
        changed = true;
    }

    if(rdcf->format != new_rdcf->format) {
        STRING *old = rdcf->format;
        rdcf->format = new_rdcf->format;
        string_freez(old);
        changed = true;
    }
    else
        string_freez(new_rdcf->format);

    if(rdcf->help != new_rdcf->help) {
        STRING *old = rdcf->help;
        rdcf->help = new_rdcf->help;
        string_freez(old);
        changed = true;
    }
    else
        string_freez(new_rdcf->help);

    if(rdcf->timeout != new_rdcf->timeout) {
        rdcf->timeout = new_rdcf->timeout;
        changed = true;
    }

    if(rdcf->sync != new_rdcf->sync) {
        rdcf->sync = new_rdcf->sync;
        changed = true;
    }

    if(rdcf->collector_data != new_rdcf->collector_data) {
        rdcf->collector_data = new_rdcf->collector_data;
        changed = true;
    }

    return changed;
}


void rrdfunctions_init(RRDHOST *host) {
    if(host->functions) return;

    host->functions = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(host->functions, rrd_functions_insert_callback, host);
    dictionary_register_delete_callback(host->functions, rrd_functions_delete_callback, host);
    dictionary_register_conflict_callback(host->functions, rrd_functions_conflict_callback, host);
}

void rrdfunctions_destroy(RRDHOST *host) {
    dictionary_destroy(host->functions);
}

void rrd_collector_add_function(RRDSET *st, const char *name, const char *help, const char *format, int timeout, bool sync, int (*function)(BUFFER *wb, int timeout, const char *name, void *collector_data, void (*callback)(BUFFER *wb, int code, void *callback_data), void *callback_data), void *collector_data) {
    if(!st->functions_view)
        st->functions_view = dictionary_create_view(st->rrdhost->functions);

    struct rrd_collector_function tmp = {
        .sync = sync,
        .timeout = timeout,
        .format = string_strdupz(format),
        .function = function,
        .collector_data = collector_data,
        .help = string_strdupz(help),
    };
    const DICTIONARY_ITEM *item = dictionary_set_and_acquire_item(st->rrdhost->functions, name, &tmp, sizeof(tmp));
    dictionary_view_set(st->functions_view, name, item);
    dictionary_acquired_item_release(st->rrdhost->functions, item);
}

struct rrd_function_call_wait {
    BUFFER *wb;
    bool free_with_signal;
    bool data_are_ready;
    netdata_mutex_t mutex;
    pthread_cond_t cond;
    int code;
};

static void rrd_function_call_wait_free(struct rrd_function_call_wait *tmp) {
    buffer_free(tmp->wb);
    pthread_cond_destroy(&tmp->cond);
    netdata_mutex_destroy(&tmp->mutex);
    freez(tmp);
}

static int rrd_call_function_prepare(RRDHOST *host, BUFFER *wb, const char *name, struct rrd_collector_function **rdcf) {
    char buffer[MAX_FUNCTION_LENGTH + 1];
    char *d = buffer, *e = &buffer[MAX_FUNCTION_LENGTH - 1];
    const char *s = name;

    // copy the first word - up to the 1st space
    while(*s && !isspace(*s) && d < e)
        *d++ = *s++;

    *rdcf = NULL;
    while(!(*rdcf) && d < e) {
        *rdcf = dictionary_get(host->functions, buffer);
        if(*rdcf || !*s)
            break;

        // copy all the spaces
        while(*s && isspace(*s) && d < e)
            *d++ = *s++;

        // copy another word
        while(*s && !isspace(*s) && d < e)
            *d++ = *s++;
    }

    if(!(*rdcf)) {
        buffer_flush(wb);
        buffer_strcat(wb, "Function is not found.");
        return HTTP_RESP_NOT_FOUND;
    }

    if(!(*rdcf)->collector->running) {
        buffer_flush(wb);
        buffer_strcat(wb, "Collector is not currently running");
        return HTTP_RESP_BACKEND_FETCH_FAILED;
    }

    return HTTP_RESP_OK;
}

static void rrd_call_function_signal_when_ready(BUFFER *wb __maybe_unused, int code, void *callback_data) {
    struct rrd_function_call_wait *tmp = callback_data;
    bool we_should_free = false;

    netdata_mutex_lock(&tmp->mutex);

    // since we got the mutex,
    // the waiting thread is either in pthread_cond_timedwait()
    // or gave up and left.

    tmp->code = code;
    tmp->data_are_ready = true;

    pthread_cond_signal(&tmp->cond);

    if(tmp->free_with_signal)
        we_should_free = true;

    netdata_mutex_unlock(&tmp->mutex);

    if(we_should_free)
        rrd_function_call_wait_free(tmp);
}

int rrd_call_function_and_wait(RRDHOST *host, BUFFER *wb, int timeout, const char *name) {
    int code;

    struct timespec tp;
    clock_gettime(CLOCK_REALTIME, &tp);
    tp.tv_sec += (time_t)timeout;

    struct rrd_collector_function *rdcf = NULL;
    code = rrd_call_function_prepare(host, wb, name, &rdcf);
    if(code != HTTP_RESP_OK)
        return code;

    if(timeout <= 0)
        timeout = rdcf->timeout;

    if(rdcf->sync) {
        code = rdcf->function(wb, timeout, name, rdcf->collector_data, NULL, NULL);
    }
    else {
        struct rrd_function_call_wait *tmp = mallocz(sizeof(struct rrd_function_call_wait));
        tmp->wb = buffer_create(100);
        tmp->free_with_signal = false;
        tmp->data_are_ready = false;
        netdata_mutex_init(&tmp->mutex);
        pthread_cond_init(&tmp->cond, NULL);

        bool we_should_free = true;
        code = rdcf->function(tmp->wb, timeout, name, rdcf->collector_data, rrd_call_function_signal_when_ready, &tmp);
        if (code == HTTP_RESP_OK) {
            netdata_mutex_lock(&tmp->mutex);

            int rc = 0;
            while (rc == 0 && !tmp->data_are_ready) {
                // the mutex is unlocked within pthread_cond_timedwait()
                rc = pthread_cond_timedwait(&tmp->cond, &tmp->mutex, &tp);
                // the mutex is again ours
            }

            if (tmp->data_are_ready) {
                // we have a response
                buffer_fast_strcat(wb, buffer_tostring(tmp->wb), buffer_strlen(tmp->wb));
                code = tmp->code;
            }
            else if (rc == ETIMEDOUT) {
                // timeout
                // we will go away and let the callback free the structure
                buffer_flush(wb);
                buffer_strcat(wb, "Timeout");
                tmp->free_with_signal = true;
                we_should_free = false;
                code = HTTP_RESP_GATEWAY_TIMEOUT;
            }
            else {
                error("RRDSET FUNCTIONS: failed to wait for a response from the collector");
                buffer_flush(wb);
                buffer_strcat(wb, "Failed to wait for a response from the collector");
                code = HTTP_RESP_INTERNAL_SERVER_ERROR;
            }

            netdata_mutex_unlock(&tmp->mutex);
        }
        else {
            error("RRDSET FUNCTIONS: failed to send request to the collector");
            buffer_flush(wb);
            buffer_strcat(wb, "Failed to send request to the collector");
        }

        if (we_should_free)
            rrd_function_call_wait_free(tmp);
    }

    return code;
}

int rrd_call_function_async(RRDHOST *host, BUFFER *wb, int timeout, const char *name, void (*callback)(BUFFER *wb, int code, void *callback_data), void *callback_data) {
    int code;

    struct rrd_collector_function *rdcf = NULL;
    code = rrd_call_function_prepare(host, wb, name, &rdcf);
    if(code != HTTP_RESP_OK)
        return code;

    if(timeout <= 0)
        timeout = rdcf->timeout;

    code = rdcf->function(wb, timeout, name, rdcf->collector_data, callback, callback_data);

    if(code != HTTP_RESP_OK) {
        error("RRDSET FUNCTIONS: failed to send request to the collector with code %d", code);
        buffer_flush(wb);
        buffer_strcat(wb, "Failed to send request to the collector");
    }

    return code;
}

