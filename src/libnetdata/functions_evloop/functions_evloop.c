// SPDX-License-Identifier: GPL-3.0-or-later

#include "functions_evloop.h"

static void functions_evloop_config_cb(const char *transaction, char *function, usec_t *stop_monotonic_ut,
                                       bool *cancelled, BUFFER *payload, HTTP_ACCESS access,
                                       const char *source, void *data);

struct functions_evloop_worker_job {
    bool used;
    bool running;
    bool cancelled;
    usec_t stop_monotonic_ut;
    char *cmd;
    const char *transaction;
    time_t timeout;

    BUFFER *payload;
    HTTP_ACCESS access;
    const char *source;

    functions_evloop_worker_execute_t cb;
    void *cb_data;
};

static void worker_job_cleanup(struct functions_evloop_worker_job *j) {
    freez((void *)j->cmd);
    freez((void *)j->transaction);
    freez((void *)j->source);
    buffer_free(j->payload);
}

struct rrd_functions_expectation {
    const char *function;
    size_t function_length;
    functions_evloop_worker_execute_t cb;
    void *cb_data;
    time_t default_timeout;
    struct rrd_functions_expectation *prev, *next;
};

struct functions_evloop_globals {
    const char *tag;

    DICTIONARY *worker_queue;
    pthread_mutex_t worker_mutex;
    pthread_cond_t worker_cond_var;
    size_t workers;

    netdata_mutex_t *stdout_mutex;
    bool *plugin_should_exit;
    bool workers_exit; // all workers are waiting on the same condition - this makes them all exit, when any is cancelled

    ND_THREAD *reader_thread;
    ND_THREAD **worker_threads;

    struct {
        DICTIONARY *nodes;
    } dyncfg;

    struct rrd_functions_expectation *expectations;

    struct buffered_reader reader;
    BUFFER *buffer;
    char *words[MAX_FUNCTION_PARAMETERS];
    struct {
        size_t last_len; // to remember the last pos - do not use a pointer, the buffer may realloc...
        bool enabled;
        char *transaction;
        char *function;
        char *timeout_s;
        char *access;
        char *source;
        char *content_type;
    } deferred;
};

static void rrd_functions_worker_canceller(void *data) {
    struct functions_evloop_globals *wg = data;
    pthread_mutex_lock(&wg->worker_mutex);
    wg->workers_exit = true;
    pthread_cond_signal(&wg->worker_cond_var);
    pthread_mutex_unlock(&wg->worker_mutex);
}

static void rrd_functions_worker_globals_worker_main(void *arg) {
    struct functions_evloop_globals *wg = arg;

    nd_thread_register_canceller(rrd_functions_worker_canceller, wg);

    bool last_acquired = true;
    while (true) {
        pthread_mutex_lock(&wg->worker_mutex);

        if(wg->workers_exit || nd_thread_signaled_to_cancel()) {
            pthread_mutex_unlock(&wg->worker_mutex);
            break;
        }

        if(dictionary_entries(wg->worker_queue) == 0 || !last_acquired)
            pthread_cond_wait(&wg->worker_cond_var, &wg->worker_mutex);

        const DICTIONARY_ITEM *acquired = NULL;
        struct functions_evloop_worker_job *j;
        dfe_start_write(wg->worker_queue, j) {
            if(j->running || j->cancelled)
                continue;

            acquired = dictionary_acquired_item_dup(wg->worker_queue, j_dfe.item);
            j->running = true;
            break;
        }
        dfe_done(j);

        pthread_mutex_unlock(&wg->worker_mutex);

        if(wg->workers_exit || nd_thread_signaled_to_cancel()) {
            if(acquired)
                dictionary_acquired_item_release(wg->worker_queue, acquired);

            break;
        }

        if(acquired) {
            ND_LOG_STACK lgs[] = {
                    ND_LOG_FIELD_TXT(NDF_REQUEST, j->cmd),
                    ND_LOG_FIELD_END(),
            };
            ND_LOG_STACK_PUSH(lgs);

            last_acquired = true;
            j = dictionary_acquired_item_value(acquired);
            j->cb(j->transaction, j->cmd, &j->stop_monotonic_ut, &j->cancelled, j->payload, j->access, j->source, j->cb_data);
            dictionary_del(wg->worker_queue, j->transaction);
            dictionary_acquired_item_release(wg->worker_queue, acquired);
            dictionary_garbage_collect(wg->worker_queue);
        }
        else
            last_acquired = false;
    }
}

static void worker_add_job(struct functions_evloop_globals *wg, const char *keyword, char *transaction, char *function, char *timeout_s, BUFFER *payload, const char *access, const char *source) {
    if(!transaction || !*transaction || !timeout_s || !*timeout_s || !function || !*function) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Received incomplete %s (transaction = '%s', timeout = '%s', function = '%s'). Ignoring it.",
               keyword,
               transaction?transaction:"(unset)",
               timeout_s?timeout_s:"(unset)",
               function?function:"(unset)");
    }
    else {
//        nd_log(NDLS_COLLECTORS, NDLP_INFO, "WORKER JOB: keyword '%s', transaction '%s', function '%s', timeout '%s', access '%s', source '%s', payload '%s'",
//               keyword, transaction, function, timeout_s, access, source, payload ? buffer_tostring(payload) : "NONE");

        int timeout = str2i(timeout_s);

        const char *msg = "No function with this name found";
        bool found = false;
        struct rrd_functions_expectation *we;
        for(we = wg->expectations; we ;we = we->next) {
            if(strncmp(function, we->function, we->function_length) == 0) {
                if(timeout <= 0)
                    timeout = (int)we->default_timeout;

                struct functions_evloop_worker_job t = {
                    .cmd = strdupz(function),
                    .transaction = strdupz(transaction),
                    .running = false,
                    .cancelled = false,
                    .timeout = timeout,
                    .stop_monotonic_ut = now_monotonic_usec() + (timeout * USEC_PER_SEC),
                    .used = false,
                    .payload = buffer_dup(payload),
                    .access = http_access_from_hex(access),
                    .source = source ? strdupz(source) : NULL,
                    .cb = we->cb,
                    .cb_data = we->cb_data,
                };
                struct functions_evloop_worker_job *j = dictionary_set(wg->worker_queue, transaction, &t, sizeof(t));
                if(j->used) {
                    nd_log(NDLS_COLLECTORS, NDLP_WARNING, "Received duplicate function transaction '%s'. Ignoring it.", transaction);
                    worker_job_cleanup(&t);
                    msg = "Duplicate function transaction. Ignoring it.";
                }
                else {
                    found = true;
                    j->used = true;
                    pthread_mutex_lock(&wg->worker_mutex);
                    pthread_cond_signal(&wg->worker_cond_var);
                    pthread_mutex_unlock(&wg->worker_mutex);
                }
            }
        }

        if(!found) {
            netdata_mutex_lock(wg->stdout_mutex);
            pluginsd_function_json_error_to_stdout(transaction, HTTP_RESP_NOT_FOUND, msg);
            netdata_mutex_unlock(wg->stdout_mutex);
        }
    }
}

static bool rrd_function_worker_global_process_input(struct functions_evloop_globals *wg) {
    if(wg->deferred.enabled) {
        char *s = (char *)buffer_tostring(wg->buffer);

        if(strstr(&s[wg->deferred.last_len], PLUGINSD_CALL_FUNCTION_PAYLOAD_END "\n") != NULL) {
            // nd_log(NDLS_COLLECTORS, NDLP_INFO, "FUNCTION PAYLOAD END");

            if(wg->deferred.last_len > 0)
                // remove the trailing newline from the buffer
                wg->deferred.last_len--;

            s[wg->deferred.last_len] = '\0';
            wg->buffer->len = wg->deferred.last_len;
            wg->buffer->content_type = content_type_string2id(wg->deferred.content_type);
            worker_add_job(wg, PLUGINSD_CALL_FUNCTION_PAYLOAD_BEGIN,
                           wg->deferred.transaction, wg->deferred.function,
                           wg->deferred.timeout_s, wg->buffer, wg->deferred.access, wg->deferred.source);
            buffer_flush(wg->buffer);

            freez(wg->deferred.transaction);
            freez(wg->deferred.function);
            freez(wg->deferred.timeout_s);
            freez(wg->deferred.access);
            freez(wg->deferred.source);
            freez(wg->deferred.content_type);
            memset(&wg->deferred, 0, sizeof(wg->deferred));
        }
        else
            wg->deferred.last_len = wg->buffer->len;

        return false;
    }

    size_t num_words = quoted_strings_splitter_whitespace((char *)buffer_tostring(wg->buffer), wg->words, _countof(wg->words));
    const char *keyword = get_word(wg->words, num_words, 0);

    char **words = wg->words;
    if(keyword && (strcmp(keyword, PLUGINSD_CALL_FUNCTION) == 0)) {
        char *transaction = get_word(words, num_words, 1);
        char *timeout_s = get_word(words, num_words, 2);
        char *function = get_word(words, num_words, 3);
        char *access = get_word(words, num_words, 4);
        char *source = get_word(words, num_words, 5);
        worker_add_job(wg, keyword, transaction, function, timeout_s, NULL, access, source);
    }
    else if(keyword && (strcmp(keyword, PLUGINSD_CALL_FUNCTION_PAYLOAD_BEGIN) == 0)) {
        char *transaction = get_word(words, num_words, 1);
        char *timeout_s = get_word(words, num_words, 2);
        char *function = get_word(words, num_words, 3);
        char *access = get_word(words, num_words, 4);
        char *source = get_word(words, num_words, 5);
        char *content_type = get_word(words, num_words, 6);

        wg->deferred.transaction = strdupz(transaction ? transaction : "");
        wg->deferred.timeout_s = strdupz(timeout_s ? timeout_s : "");
        wg->deferred.function = strdupz(function ? function : "");
        wg->deferred.access = strdupz(access ? access : "");
        wg->deferred.source = strdupz(source ? source : "");
        wg->deferred.content_type = strdupz(content_type ? content_type : "");
        wg->deferred.last_len = 0;
        wg->deferred.enabled = true;
    }
    else if(keyword && strcmp(keyword, PLUGINSD_CALL_FUNCTION_CANCEL) == 0) {
        char *transaction = get_word(words, num_words, 1);
        const DICTIONARY_ITEM *acquired = dictionary_get_and_acquire_item(wg->worker_queue, transaction);
        if(acquired) {
            struct functions_evloop_worker_job *j = dictionary_acquired_item_value(acquired);
            __atomic_store_n(&j->cancelled, true, __ATOMIC_RELAXED);
            dictionary_acquired_item_release(wg->worker_queue, acquired);
            dictionary_del(wg->worker_queue, transaction);
            dictionary_garbage_collect(wg->worker_queue);
        }
        else
            nd_log(NDLS_COLLECTORS, NDLP_NOTICE, "Received CANCEL for transaction '%s', but it not available here", transaction);
    }
    else if(keyword && strcmp(keyword, PLUGINSD_CALL_FUNCTION_PROGRESS) == 0) {
        char *transaction = get_word(words, num_words, 1);
        const DICTIONARY_ITEM *acquired = dictionary_get_and_acquire_item(wg->worker_queue, transaction);
        if(acquired) {
            struct functions_evloop_worker_job *j = dictionary_acquired_item_value(acquired);

            functions_stop_monotonic_update_on_progress(&j->stop_monotonic_ut);

            dictionary_acquired_item_release(wg->worker_queue, acquired);
        }
        else
            nd_log(NDLS_COLLECTORS, NDLP_NOTICE, "Received PROGRESS for transaction '%s', but it not available here", transaction);
    }
    else if(keyword && strcmp(keyword, PLUGINSD_CALL_QUIT) == 0) {
        *wg->plugin_should_exit = true;
        return true;
    }
    else
        nd_log(NDLS_COLLECTORS, NDLP_NOTICE, "Received unknown command: %s", keyword ? keyword : "(unset)");

    buffer_flush(wg->buffer);

    return false;
}

static void rrd_functions_worker_globals_reader_main(void *arg) {
    struct functions_evloop_globals *wg = arg;

    buffered_reader_init(&wg->reader);
    wg->buffer = buffer_create(sizeof(wg->reader.read_buffer) + 2, NULL);

    while(!(*wg->plugin_should_exit)) {
        if(unlikely(!buffered_reader_next_line(&wg->reader, wg->buffer))) {
            buffered_reader_ret_t ret = buffered_reader_read_timeout(
                &wg->reader,
                fileno((FILE *)stdin),
                2 * 60 * MSEC_PER_SEC,
                false
            );

            if(unlikely(ret != BUFFERED_READER_READ_OK && ret != BUFFERED_READER_READ_POLL_TIMEOUT))
                break;

            continue;
        }

        if(rrd_function_worker_global_process_input(wg))
            break;
    }

    int status = 0;
    if(!(*wg->plugin_should_exit)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Read error on stdin");
        status = 1;
    }

    *wg->plugin_should_exit = true;
    buffer_free(wg->buffer);
    exit(status);
}

void worker_queue_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct functions_evloop_worker_job *j = value;
    worker_job_cleanup(j);
}

struct functions_evloop_globals *functions_evloop_init(size_t worker_threads, const char *tag, netdata_mutex_t *stdout_mutex, bool *plugin_should_exit) {
    struct functions_evloop_globals *wg = callocz(1, sizeof(struct functions_evloop_globals));

    wg->worker_queue = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_delete_callback(wg->worker_queue, worker_queue_delete_cb, NULL);

    wg->dyncfg.nodes = dyncfg_nodes_dictionary_create();

    pthread_mutex_init(&wg->worker_mutex, NULL);
    pthread_cond_init(&wg->worker_cond_var, NULL);

    wg->plugin_should_exit = plugin_should_exit;
    wg->stdout_mutex = stdout_mutex;
    wg->workers = worker_threads;
    wg->worker_threads = callocz(wg->workers, sizeof(ND_THREAD *));
    wg->tag = tag;

    char tag_buffer[NETDATA_THREAD_TAG_MAX + 1];
    snprintfz(tag_buffer, NETDATA_THREAD_TAG_MAX, "%s_READER", wg->tag);
    wg->reader_thread = nd_thread_create(tag_buffer, NETDATA_THREAD_OPTION_DONT_LOG,
                                         rrd_functions_worker_globals_reader_main, wg);

    for(size_t i = 0; i < wg->workers ; i++) {
        snprintfz(tag_buffer, NETDATA_THREAD_TAG_MAX, "%s_WORK[%zu]", wg->tag, i+1);
        wg->worker_threads[i] = nd_thread_create(tag_buffer, NETDATA_THREAD_OPTION_DONT_LOG,
                                                 rrd_functions_worker_globals_worker_main, wg);
    }

    functions_evloop_add_function(wg, "config", functions_evloop_config_cb, 120, wg);

    return wg;
}

void functions_evloop_add_function(struct functions_evloop_globals *wg, const char *function, functions_evloop_worker_execute_t cb, time_t default_timeout, void *data) {
    struct rrd_functions_expectation *we = callocz(1, sizeof(*we));
    we->function = function;
    we->function_length = strlen(we->function);
    we->cb = cb;
    we->cb_data = data;
    we->default_timeout = default_timeout;
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(wg->expectations, we, prev, next);
}

void functions_evloop_cancel_threads(struct functions_evloop_globals *wg) {
    nd_thread_signal_cancel(wg->reader_thread);

    for(size_t i = 0; i < wg->workers ; i++)
        nd_thread_signal_cancel(wg->worker_threads[i]);
}

// ----------------------------------------------------------------------------

static void functions_evloop_config_cb(const char *transaction, char *function, usec_t *stop_monotonic_ut, bool *cancelled,
                                       BUFFER *payload, HTTP_ACCESS access, const char *source, void *data) {
    struct functions_evloop_globals *wg = data;

    CLEAN_BUFFER *result = buffer_create(1024, NULL);
    int code = dyncfg_node_find_and_call(wg->dyncfg.nodes, transaction, function, stop_monotonic_ut,
                                         cancelled, payload, access, source, result);

    netdata_mutex_lock(wg->stdout_mutex);
    pluginsd_function_result_begin_to_stdout(transaction, code, content_type_id2string(result->content_type), result->expires);
    printf("%s", buffer_tostring(result));
    pluginsd_function_result_end_to_stdout();
    fflush(stdout);
    netdata_mutex_unlock(wg->stdout_mutex);
}

void functions_evloop_dyncfg_add(struct functions_evloop_globals *wg, const char *id, const char *path,
                                 DYNCFG_STATUS status, DYNCFG_TYPE type, DYNCFG_SOURCE_TYPE source_type,
                                 const char *source, DYNCFG_CMDS cmds,
                                 HTTP_ACCESS view_access, HTTP_ACCESS edit_access,
                                 dyncfg_cb_t cb, void *data) {

    if(!dyncfg_is_valid_id(id)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "DYNCFG: id '%s' is invalid. Ignoring dynamic configuration for it.", id);
        return;
    }

    struct dyncfg_node tmp = {
        .cmds = cmds,
        .type = type,
        .cb = cb,
        .data = data,
    };
    dictionary_set(wg->dyncfg.nodes, id, &tmp, sizeof(tmp));

    CLEAN_BUFFER *c = buffer_create(100, NULL);
    dyncfg_cmds2buffer(cmds, c);

    netdata_mutex_lock(wg->stdout_mutex);

    fprintf(stdout,
            PLUGINSD_KEYWORD_CONFIG " '%s' " PLUGINSD_KEYWORD_CONFIG_ACTION_CREATE " '%s' '%s' '%s' '%s' '%s' '%s' "HTTP_ACCESS_FORMAT" "HTTP_ACCESS_FORMAT"\n",
            id,
            dyncfg_id2status(status),
            dyncfg_id2type(type), path,
            dyncfg_id2source_type(source_type),
            source,
            buffer_tostring(c),
            (HTTP_ACCESS_FORMAT_CAST)view_access,
            (HTTP_ACCESS_FORMAT_CAST)edit_access
    );
    fflush(stdout);

    netdata_mutex_unlock(wg->stdout_mutex);
}

void functions_evloop_dyncfg_del(struct functions_evloop_globals *wg, const char *id) {
    if(!dyncfg_is_valid_id(id)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "DYNCFG: id '%s' is invalid. Ignoring dynamic configuration for it.", id);
        return;
    }

    dictionary_del(wg->dyncfg.nodes, id);

    netdata_mutex_lock(wg->stdout_mutex);

    fprintf(stdout,
            PLUGINSD_KEYWORD_CONFIG " %s " PLUGINSD_KEYWORD_CONFIG_ACTION_DELETE "\n",
            id);
    fflush(stdout);

    netdata_mutex_unlock(wg->stdout_mutex);
}

void functions_evloop_dyncfg_status(struct functions_evloop_globals *wg, const char *id, DYNCFG_STATUS status) {
    if(!dyncfg_is_valid_id(id)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "DYNCFG: id '%s' is invalid. Ignoring dynamic configuration for it.", id);
        return;
    }

    netdata_mutex_lock(wg->stdout_mutex);

    fprintf(stdout,
            PLUGINSD_KEYWORD_CONFIG " %s " PLUGINSD_KEYWORD_CONFIG_ACTION_STATUS " %s\n",
            id, dyncfg_id2status(status));

    fflush(stdout);

    netdata_mutex_unlock(wg->stdout_mutex);
}
