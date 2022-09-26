#define NETDATA_RRD_INTERNALS
#include "rrd.h"

// ----------------------------------------------------------------------------
// RRDSET - collector info and rrdset functions

// we keep a dictionary per RRDSET with these functions
// the dictionary is created on demand (only when a function is added to an RRDSET)

struct rrdset_collector_function {
    bool sync;                      // when true, the function is called synchronously
    STRING *format;                 // the format the function produces
    int timeout;                    // the default timeout of the function
    int (*function)(BUFFER *wb, RRDSET *st, int timeout, const char *name, int argc, char **argv, void *collector_data, void (*callback)(BUFFER *wb, int code, void *callback_data), void *callback_data);
    void *collector_data;
    struct rrdset_collector *collector;
};

// Each function points to this collector structure
// so that when the collector exits, all of them will
// be invalidated (running == false)
// The last function that is using this collector
// frees the structure too (or when the collector calls
// rrdset_collector_finished()).

struct rrdset_collector {
    void *input;
    void *output;
    int32_t refcount;
    pid_t tid;
    bool running;
};

// Each thread that adds RRDSET functions, has to call
// rrdset_collector_started() and rrdset_collector_finished()
// to create the collector structure.

static __thread struct rrdset_collector *thread_collector = NULL;

static void rrdset_collector_free(struct rrdset_collector *rdc) {
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
void rrdset_collector_started(void *input, void *output) {
    if(likely(thread_collector)) return;

    thread_collector = callocz(1, sizeof(struct rrdset_collector));
    thread_collector->tid = gettid();
    thread_collector->running = true;
    thread_collector->input = input;
    thread_collector->output = output;
}

// called once per collector
void rrdset_collector_finished(void) {
    if(!thread_collector)
        return;

    thread_collector->running = false;
    rrdset_collector_free(thread_collector);
    thread_collector = NULL;
}

static struct rrdset_collector *rrdset_collector_acquire(void) {
    __atomic_add_fetch(&thread_collector->refcount, 1, __ATOMIC_SEQ_CST);
    return thread_collector;
}

static void rrdset_collector_release(struct rrdset_collector *rdc) {
    if(unlikely(!rdc)) return;

    int32_t refcount = __atomic_sub_fetch(&rdc->refcount, 1, __ATOMIC_SEQ_CST);
    if(refcount == 0 && !rdc->running)
        rrdset_collector_free(rdc);
}

static void rrdset_functions_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func __maybe_unused, void *rrdset __maybe_unused) {
    struct rrdset_collector_function *rdcf = func;
    RRDSET *st = rrdset;

    if(!thread_collector)
        fatal("RRDSET_COLLECTOR: called %s() without calling rrdset_collector_started() first, for chart '%s', host '%s'.", __FUNCTION__, rrdset_id(st), rrdhost_hostname(st->rrdhost));

    rdcf->collector = rrdset_collector_acquire();
}

static void rrdset_functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func __maybe_unused, void *rrdset __maybe_unused) {
    struct rrdset_collector_function *rdcf = func;
    rrdset_collector_release(rdcf->collector);
    string_freez(rdcf->format);
}

static bool rrdset_functions_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func __maybe_unused, void *new_func __maybe_unused, void *rrdset __maybe_unused) {
    struct rrdset_collector_function *rdcf = func;
    struct rrdset_collector_function *new_rdcf = new_func;
    RRDSET *st = rrdset;

    if(!thread_collector)
        fatal("RRDSET_COLLECTOR: called %s() without calling rrdset_collector_started() first, for chart '%s', host '%s'.", __FUNCTION__, rrdset_id(st), rrdhost_hostname(st->rrdhost));

    bool changed = false;

    if(rdcf->collector != thread_collector) {
        struct rrdset_collector *old_rdc = rdcf->collector;
        rdcf->collector = rrdset_collector_acquire();
        rrdset_collector_release(old_rdc);
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

void rrdset_collector_add_function(RRDSET *st, const char *name, const char *format, int timeout, bool sync, int (*function)(BUFFER *wb, RRDSET *st, int timeout, const char *name, int argc, char **argv, void *collector_data, void (*callback)(BUFFER *wb, int code, void *callback_data), void *callback_data), void *collector_data) {
    if(!st->functions) {
        st->functions = dictionary_create(DICT_OPTION_NONE);
        dictionary_register_insert_callback(st->functions, rrdset_functions_insert_callback, st);
        dictionary_register_delete_callback(st->functions, rrdset_functions_delete_callback, st);
        dictionary_register_conflict_callback(st->functions, rrdset_functions_conflict_callback, st);
    }

    struct rrdset_collector_function tmp = {
        .sync = sync,
        .timeout = timeout,
        .format = string_strdupz(format),
        .function = function,
        .collector_data = collector_data,
    };
    dictionary_set(st->functions, name, &tmp, sizeof(tmp));
}

struct rrdset_function_call_wait {
    BUFFER *wb;
    bool free_with_signal;
    bool data_are_ready;
    netdata_mutex_t mutex;
    pthread_cond_t cond;
    int code;
};

static void rrdset_function_call_wait_free(struct rrdset_function_call_wait *tmp) {
    buffer_free(tmp->wb);
    pthread_cond_destroy(&tmp->cond);
    netdata_mutex_destroy(&tmp->mutex);
    freez(tmp);
}

static int rrdset_call_function_prepare(RRDHOST *host, BUFFER *wb, const char *chart, const char *name, RRDSET **st, struct rrdset_collector_function **rdcf) {
    *st = rrdset_find(host, chart);
    if(!(*st))
        *st = rrdset_find_byname(host, chart);

    if(!(*st)) {
        buffer_flush(wb);
        buffer_strcat(wb, "Chart not found");
        return HTTP_RESP_NOT_FOUND;
    }

    if(!(*st)->functions) {
        buffer_flush(wb);
        buffer_strcat(wb, "Chart does not have any functions");
        return HTTP_RESP_NOT_FOUND;
    }

    *rdcf = dictionary_get((*st)->functions, name);
    if(!(*rdcf)) {
        buffer_flush(wb);
        buffer_strcat(wb, "Chart has functions, but the requested function is not found");
        return HTTP_RESP_NOT_FOUND;
    }

    if(!(*rdcf)->collector->running) {
        buffer_flush(wb);
        buffer_strcat(wb, "Collector is not currently running");
        return HTTP_RESP_BACKEND_FETCH_FAILED;
    }

    return HTTP_RESP_OK;
}

static void rrdset_call_function_signal_when_ready(BUFFER *wb __maybe_unused, int code, void *callback_data) {
    struct rrdset_function_call_wait *tmp = callback_data;
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
        rrdset_function_call_wait_free(tmp);
}

int rrdset_call_function_and_wait(RRDHOST *host, BUFFER *wb, int timeout, const char *chart, const char *name, int argc, char **argv) {
    int code;

    struct timespec tp;
    clock_gettime(CLOCK_REALTIME, &tp);
    tp.tv_sec += (time_t)timeout;

    RRDSET *st = NULL;
    struct rrdset_collector_function *rdcf = NULL;
    code = rrdset_call_function_prepare(host, wb, chart, name, &st, &rdcf);
    if(code != HTTP_RESP_OK)
        return code;

    if(rdcf->sync) {
        code = rdcf->function(wb, st, timeout, name, argc, argv, rdcf->collector_data, rrdset_call_function_signal_when_ready, NULL);
    }
    else {
        struct rrdset_function_call_wait *tmp = mallocz(sizeof(struct rrdset_function_call_wait));
        tmp->wb = buffer_create(100);
        tmp->free_with_signal = false;
        tmp->data_are_ready = false;
        netdata_mutex_init(&tmp->mutex);
        pthread_cond_init(&tmp->cond, NULL);

        netdata_mutex_lock(&tmp->mutex);
        int rc = 0;
        bool we_should_free = true;

        code = rdcf->function(tmp->wb, st, timeout, name, argc, argv, rdcf->collector_data, rrdset_call_function_signal_when_ready, &tmp);
        if (code == 200) {
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
        }
        else {
            error("RRDSET FUNCTIONS: failed to send request to the collector");
            buffer_flush(wb);
            buffer_strcat(wb, "Failed to send request to the collector");
            code = HTTP_RESP_INTERNAL_SERVER_ERROR;
        }
        netdata_mutex_unlock(&tmp->mutex);

        if (we_should_free)
            rrdset_function_call_wait_free(tmp);
    }

    return code;
}

int rrdset_call_function_async(RRDHOST *host, BUFFER *wb, int timeout, const char *chart, const char *name, int argc, char **argv, void (*callback)(BUFFER *wb, int code, void *callback_data), void *callback_data) {
    int code;

    RRDSET *st = NULL;
    struct rrdset_collector_function *rdcf = NULL;
    code = rrdset_call_function_prepare(host, wb, chart, name, &st, &rdcf);
    if(code != HTTP_RESP_OK)
        return code;

    if(rdcf->function(wb, st, timeout, name, argc, argv, rdcf->collector_data, callback, callback_data) == 0) {
        code = HTTP_RESP_OK;
    }
    else {
        error("RRDSET FUNCTIONS: failed to send request to the collector");
        buffer_flush(wb);
        buffer_strcat(wb, "Failed to send request to the collector");
        code = HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    return code;
}

