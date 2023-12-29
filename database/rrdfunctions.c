// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS
#define NETDATA_RRDCOLLECTOR_INTERNALS

#include "rrd.h"

#define MAX_FUNCTION_LENGTH (PLUGINSD_LINE_MAX - 512) // we need some space for the rest of the line

static unsigned char functions_allowed_chars[256] = {
        [0] = '\0', [1] = '_', [2] = '_', [3] = '_', [4] = '_', [5] = '_', [6] = '_', [7] = '_', [8] = '_',

        // control
        ['\t'] = ' ', ['\n'] = ' ', ['\v'] = ' ', [12] = ' ', ['\r'] = ' ',

        [14] = '_', [15] = '_', [16] = '_', [17] = '_', [18] = '_', [19] = '_', [20] = '_', [21] = '_',
        [22] = '_', [23] = '_', [24] = '_', [25] = '_', [26] = '_', [27] = '_', [28] = '_', [29] = '_',
        [30] = '_', [31] = '_',

        // symbols
        [' '] = ' ', ['!'] = '!', ['"'] = '"', ['#'] = '#', ['$'] = '$', ['%'] = '%', ['&'] = '&', ['\''] = '\'',
        ['('] = '(', [')'] = ')', ['*'] = '*', ['+'] = '+', [','] = ',', ['-'] = '-', ['.'] = '.', ['/'] = '/',

        // numbers
        ['0'] = '0', ['1'] = '1', ['2'] = '2', ['3'] = '3', ['4'] = '4', ['5'] = '5', ['6'] = '6', ['7'] = '7',
        ['8'] = '8', ['9'] = '9',

        // symbols
        [':'] = ':', [';'] = ';', ['<'] = '<', ['='] = '=', ['>'] = '>', ['?'] = '?', ['@'] = '@',

        // capitals
        ['A'] = 'A', ['B'] = 'B', ['C'] = 'C', ['D'] = 'D', ['E'] = 'E', ['F'] = 'F', ['G'] = 'G', ['H'] = 'H',
        ['I'] = 'I', ['J'] = 'J', ['K'] = 'K', ['L'] = 'L', ['M'] = 'M', ['N'] = 'N', ['O'] = 'O', ['P'] = 'P',
        ['Q'] = 'Q', ['R'] = 'R', ['S'] = 'S', ['T'] = 'T', ['U'] = 'U', ['V'] = 'V', ['W'] = 'W', ['X'] = 'X',
        ['Y'] = 'Y', ['Z'] = 'Z',

        // symbols
        ['['] = '[', ['\\'] = '\\', [']'] = ']', ['^'] = '^', ['_'] = '_', ['`'] = '`',

        // lower
        ['a'] = 'a', ['b'] = 'b', ['c'] = 'c', ['d'] = 'd', ['e'] = 'e', ['f'] = 'f', ['g'] = 'g', ['h'] = 'h',
        ['i'] = 'i', ['j'] = 'j', ['k'] = 'k', ['l'] = 'l', ['m'] = 'm', ['n'] = 'n', ['o'] = 'o', ['p'] = 'p',
        ['q'] = 'q', ['r'] = 'r', ['s'] = 's', ['t'] = 't', ['u'] = 'u', ['v'] = 'v', ['w'] = 'w', ['x'] = 'x',
        ['y'] = 'y', ['z'] = 'z',

        // symbols
        ['{'] = '{', ['|'] = '|', ['}'] = '}', ['~'] = '~',

        // rest
        [127] = '_', [128] = '_', [129] = '_', [130] = '_', [131] = '_', [132] = '_', [133] = '_', [134] = '_',
        [135] = '_', [136] = '_', [137] = '_', [138] = '_', [139] = '_', [140] = '_', [141] = '_', [142] = '_',
        [143] = '_', [144] = '_', [145] = '_', [146] = '_', [147] = '_', [148] = '_', [149] = '_', [150] = '_',
        [151] = '_', [152] = '_', [153] = '_', [154] = '_', [155] = '_', [156] = '_', [157] = '_', [158] = '_',
        [159] = '_', [160] = '_', [161] = '_', [162] = '_', [163] = '_', [164] = '_', [165] = '_', [166] = '_',
        [167] = '_', [168] = '_', [169] = '_', [170] = '_', [171] = '_', [172] = '_', [173] = '_', [174] = '_',
        [175] = '_', [176] = '_', [177] = '_', [178] = '_', [179] = '_', [180] = '_', [181] = '_', [182] = '_',
        [183] = '_', [184] = '_', [185] = '_', [186] = '_', [187] = '_', [188] = '_', [189] = '_', [190] = '_',
        [191] = '_', [192] = '_', [193] = '_', [194] = '_', [195] = '_', [196] = '_', [197] = '_', [198] = '_',
        [199] = '_', [200] = '_', [201] = '_', [202] = '_', [203] = '_', [204] = '_', [205] = '_', [206] = '_',
        [207] = '_', [208] = '_', [209] = '_', [210] = '_', [211] = '_', [212] = '_', [213] = '_', [214] = '_',
        [215] = '_', [216] = '_', [217] = '_', [218] = '_', [219] = '_', [220] = '_', [221] = '_', [222] = '_',
        [223] = '_', [224] = '_', [225] = '_', [226] = '_', [227] = '_', [228] = '_', [229] = '_', [230] = '_',
        [231] = '_', [232] = '_', [233] = '_', [234] = '_', [235] = '_', [236] = '_', [237] = '_', [238] = '_',
        [239] = '_', [240] = '_', [241] = '_', [242] = '_', [243] = '_', [244] = '_', [245] = '_', [246] = '_',
        [247] = '_', [248] = '_', [249] = '_', [250] = '_', [251] = '_', [252] = '_', [253] = '_', [254] = '_',
        [255] = '_'
};

static inline size_t sanitize_function_text(char *dst, const char *src, size_t dst_len) {
    return text_sanitize((unsigned char *)dst, (const unsigned char *)src, dst_len,
                         functions_allowed_chars, true, "", NULL);
}

// we keep a dictionary per RRDSET with these functions
// the dictionary is created on demand (only when a function is added to an RRDSET)

typedef enum __attribute__((packed)) {
    RRD_FUNCTION_LOCAL  = (1 << 0),
    RRD_FUNCTION_GLOBAL = (1 << 1),

    // this is 8-bit
} RRD_FUNCTION_OPTIONS;

// ----------------------------------------------------------------------------

struct rrd_host_function {
    bool sync;                      // when true, the function is called synchronously
    RRD_FUNCTION_OPTIONS options;   // RRD_FUNCTION_OPTIONS
    HTTP_ACCESS access;
    STRING *help;
    STRING *tags;
    int timeout;                    // the default timeout of the function
    int priority;

    rrd_function_execute_cb_t execute_cb;
    void *execute_cb_data;

    struct rrd_collector *collector;
};

struct rrd_function_inflight {
    bool used;

    RRDHOST *host;
    uuid_t transaction_uuid;
    const char *transaction;
    const char *cmd;
    const char *sanitized_cmd;
    size_t sanitized_cmd_length;
    int timeout;
    bool cancelled;
    usec_t stop_monotonic_ut;

    const DICTIONARY_ITEM *host_function_acquired;

    // the collector
    // we acquire this structure at the beginning,
    // and we release it at the end
    struct rrd_host_function *rdcf;

    struct {
        BUFFER *wb;

        // in async mode,
        // the function to call to send the result back
        rrd_function_result_callback_t cb;
        void *data;
    } result;

    struct {
        // to be called in sync mode
        // while the function is running
        // to check if the function has been canceled
        rrd_function_is_cancelled_cb_t cb;
        void *data;
    } is_cancelled;

    struct {
        // to be registered by the function itself
        // used to signal the function to cancel
        rrd_function_cancel_cb_t cb;
        void *data;
    } canceller;

    struct {
        // callback to receive progress reports from function
        rrd_function_progress_cb_t cb;
        void *data;
    } progress;

    struct {
        // to be registered by the function itself
        // used to send progress requests to function
        rrd_function_progresser_cb_t cb;
        void *data;
    } progresser;
};

static DICTIONARY *rrd_functions_inflight_requests = NULL;

static void rrd_function_cancel_inflight(struct rrd_function_inflight *r);

// ----------------------------------------------------------------------------

static void rrd_functions_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func, void *rrdhost) {
    RRDHOST *host = rrdhost; (void)host;
    struct rrd_host_function *rdcf = func;

    rrd_collector_started();
    rdcf->collector = rrd_collector_acquire_current_thread();

    if(!rdcf->priority)
        rdcf->priority = RRDFUNCTIONS_PRIORITY_DEFAULT;

//    internal_error(true, "FUNCTIONS: adding function '%s' on host '%s', collection tid %d, %s",
//                   dictionary_acquired_item_name(item), rrdhost_hostname(host),
//                   rdcf->collector->tid, rdcf->collector->running ? "running" : "NOT running");
}

static void rrd_functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func,
                                          void *rrdhost __maybe_unused) {
    struct rrd_host_function *rdcf = func;
    rrd_collector_release(rdcf->collector);
}

static bool rrd_functions_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func,
                                            void *new_func, void *rrdhost) {
    RRDHOST *host = rrdhost; (void)host;
    struct rrd_host_function *rdcf = func;
    struct rrd_host_function *new_rdcf = new_func;

    rrd_collector_started();

    bool changed = false;

    if(rdcf->collector != thread_rrd_collector) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed collector from %d to %d",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               rrd_collector_tid(rdcf->collector), rrd_collector_tid(thread_rrd_collector));

        struct rrd_collector *old_rdc = rdcf->collector;
        rdcf->collector = rrd_collector_acquire_current_thread();
        rrd_collector_release(old_rdc);
        changed = true;
    }

    if(rdcf->execute_cb != new_rdcf->execute_cb) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed execute callback",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        rdcf->execute_cb = new_rdcf->execute_cb;
        changed = true;
    }

    if(rdcf->help != new_rdcf->help) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed help text",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        STRING *old = rdcf->help;
        rdcf->help = new_rdcf->help;
        string_freez(old);
        changed = true;
    }
    else
        string_freez(new_rdcf->help);

    if(rdcf->tags != new_rdcf->tags) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed tags",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        STRING *old = rdcf->tags;
        rdcf->tags = new_rdcf->tags;
        string_freez(old);
        changed = true;
    }
    else
        string_freez(new_rdcf->tags);

    if(rdcf->timeout != new_rdcf->timeout) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed timeout",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        rdcf->timeout = new_rdcf->timeout;
        changed = true;
    }

    if(rdcf->priority != new_rdcf->priority) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed priority",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        rdcf->priority = new_rdcf->priority;
        changed = true;
    }

    if(rdcf->access != new_rdcf->access) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed access level",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        rdcf->access = new_rdcf->access;
        changed = true;
    }

    if(rdcf->sync != new_rdcf->sync) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed sync/async mode",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        rdcf->sync = new_rdcf->sync;
        changed = true;
    }

    if(rdcf->execute_cb_data != new_rdcf->execute_cb_data) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed execute callback data",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        rdcf->execute_cb_data = new_rdcf->execute_cb_data;
        changed = true;
    }

//    internal_error(true, "FUNCTIONS: adding function '%s' on host '%s', collection tid %d, %s",
//                   dictionary_acquired_item_name(item), rrdhost_hostname(host),
//                   rdcf->collector->tid, rdcf->collector->running ? "running" : "NOT running");

    return changed;
}

void rrdfunctions_host_init(RRDHOST *host) {
    if(host->functions) return;

    host->functions = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                 &dictionary_stats_category_functions, sizeof(struct rrd_host_function));

    dictionary_register_insert_callback(host->functions, rrd_functions_insert_callback, host);
    dictionary_register_delete_callback(host->functions, rrd_functions_delete_callback, host);
    dictionary_register_conflict_callback(host->functions, rrd_functions_conflict_callback, host);
}

void rrdfunctions_host_destroy(RRDHOST *host) {
    dictionary_destroy(host->functions);
}

// ----------------------------------------------------------------------------

void rrd_function_add(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, const char *help, const char *tags,
                      HTTP_ACCESS access, bool sync, rrd_function_execute_cb_t execute_cb,
                      void *execute_cb_data) {

    // RRDSET *st may be NULL in this function
    // to create a GLOBAL function

    if(!tags || !*tags) {
        if(strcmp(name, "systemd-journal") == 0)
            tags = "logs";
        else
            tags = "top";
    }

    if(st && !st->functions_view)
        st->functions_view = dictionary_create_view(host->functions);

    char key[PLUGINSD_LINE_MAX + 1];
    sanitize_function_text(key, name, PLUGINSD_LINE_MAX);

    struct rrd_host_function tmp = {
        .sync = sync,
        .timeout = timeout,
        .options = (st)?RRD_FUNCTION_LOCAL:RRD_FUNCTION_GLOBAL,
        .access = access,
        .execute_cb = execute_cb,
        .execute_cb_data = execute_cb_data,
        .help = string_strdupz(help),
        .tags = string_strdupz(tags),
        .priority = priority,
    };
    const DICTIONARY_ITEM *item = dictionary_set_and_acquire_item(host->functions, key, &tmp, sizeof(tmp));

    if(st)
        dictionary_view_set(st->functions_view, key, item);
    else
        rrdhost_flag_set(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    dictionary_acquired_item_release(host->functions, item);
}

void rrd_functions_expose_rrdpush(RRDSET *st, BUFFER *wb) {
    if(!st->functions_view)
        return;

    struct rrd_host_function *tmp;
    dfe_start_read(st->functions_view, tmp) {
        buffer_sprintf(wb
                       , PLUGINSD_KEYWORD_FUNCTION " \"%s\" %d \"%s\" \"%s\" \"%s\" %d\n"
                       , tmp_dfe.name
                       , tmp->timeout
                       , string2str(tmp->help)
                       , string2str(tmp->tags)
                       , http_id2access(tmp->access)
                       , tmp->priority
                       );
    }
    dfe_done(tmp);
}

void rrd_functions_expose_global_rrdpush(RRDHOST *host, BUFFER *wb) {
    rrdhost_flag_clear(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    struct rrd_host_function *tmp;
    dfe_start_read(host->functions, tmp) {
        if(!(tmp->options & RRD_FUNCTION_GLOBAL))
            continue;

        buffer_sprintf(wb
                , PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\" \"%s\" \"%s\" %d\n"
                , tmp_dfe.name
                , tmp->timeout
                , string2str(tmp->help)
                , string2str(tmp->tags)
                , http_id2access(tmp->access)
               , tmp->priority
        );
    }
    dfe_done(tmp);
}

struct {
    const char *format;
    HTTP_CONTENT_TYPE content_type;
} function_formats[] = {
    { .format = "application/json", CT_APPLICATION_JSON },
    { .format = "text/plain",       CT_TEXT_PLAIN },
    { .format = "application/xml",  CT_APPLICATION_XML },
    { .format = "prometheus",       CT_PROMETHEUS },
    { .format = "text",             CT_TEXT_PLAIN },
    { .format = "txt",              CT_TEXT_PLAIN },
    { .format = "json",             CT_APPLICATION_JSON },
    { .format = "html",             CT_TEXT_HTML },
    { .format = "text/html",        CT_TEXT_HTML },
    { .format = "xml",              CT_APPLICATION_XML },

    // terminator
    { .format = NULL,               CT_TEXT_PLAIN },
};

uint8_t functions_format_to_content_type(const char *format) {
    if(format && *format) {
        for (int i = 0; function_formats[i].format; i++)
            if (strcmp(function_formats[i].format, format) == 0)
                return function_formats[i].content_type;
    }

    return CT_TEXT_PLAIN;
}

const char *functions_content_type_to_format(HTTP_CONTENT_TYPE content_type) {
    for (int i = 0; function_formats[i].format; i++)
        if (function_formats[i].content_type == content_type)
            return function_formats[i].format;

    return "text/plain";
}

int rrd_call_function_error(BUFFER *wb, const char *msg, int code) {
    char buffer[PLUGINSD_LINE_MAX];
    json_escape_string(buffer, msg, PLUGINSD_LINE_MAX);

    buffer_flush(wb);
    buffer_sprintf(wb, "{\"status\":%d,\"error_message\":\"%s\"}", code, buffer);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_no_cacheable(wb);
    return code;
}

static int rrd_call_function_find(RRDHOST *host, BUFFER *wb, const char *name, size_t key_length, const DICTIONARY_ITEM **item) {
    char buffer[MAX_FUNCTION_LENGTH + 1];

    strncpyz(buffer, name, MAX_FUNCTION_LENGTH);
    char *s = NULL;

    bool found = false;
    *item = NULL;
    if(host->functions) {
        while (buffer[0]) {
            if((*item = dictionary_get_and_acquire_item(host->functions, buffer))) {
                found = true;

                struct rrd_host_function *rdcf = dictionary_acquired_item_value(*item);
                if(rrd_collector_running(rdcf->collector)) {
                    break;
                }
                else {
                    dictionary_acquired_item_release(host->functions, *item);
                    *item = NULL;
                }
            }

            // if s == NULL, set it to the end of the buffer;
            // this should happen only the first time
            if (unlikely(!s))
                s = &buffer[key_length - 1];

            // skip a word from the end
            while (s >= buffer && !isspace(*s)) *s-- = '\0';

            // skip all spaces
            while (s >= buffer && isspace(*s)) *s-- = '\0';
        }
    }

    buffer_flush(wb);

    if(!(*item)) {
        if(found)
            return rrd_call_function_error(wb,
                                           "The collector that registered this function, is not currently running.",
                                           HTTP_RESP_SERVICE_UNAVAILABLE);
        else
            return rrd_call_function_error(wb,
                                           "No collector is supplying this function on this host at this time.",
                                           HTTP_RESP_NOT_FOUND);
    }

    return HTTP_RESP_OK;
}

// ----------------------------------------------------------------------------

static void rrd_functions_inflight_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct rrd_function_inflight *r = value;

    // internal_error(true, "FUNCTIONS: transaction '%s' finished", r->transaction);

    freez((void *)r->transaction);
    freez((void *)r->cmd);
    freez((void *)r->sanitized_cmd);
    dictionary_acquired_item_release(r->host->functions, r->host_function_acquired);
}

void rrd_functions_inflight_init(void) {
    if(rrd_functions_inflight_requests)
        return;

    rrd_functions_inflight_requests = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct rrd_function_inflight));

    dictionary_register_delete_callback(rrd_functions_inflight_requests, rrd_functions_inflight_delete_cb, NULL);
}

void rrd_functions_inflight_destroy(void) {
    if(!rrd_functions_inflight_requests)
        return;

    dictionary_destroy(rrd_functions_inflight_requests);
    rrd_functions_inflight_requests = NULL;
}

static void rrd_inflight_async_function_register_canceller_cb(void *register_canceller_cb_data, rrd_function_cancel_cb_t canceller_cb, void *canceller_cb_data) {
    struct rrd_function_inflight *r = register_canceller_cb_data;
    r->canceller.cb = canceller_cb;
    r->canceller.data = canceller_cb_data;
}

static void rrd_inflight_async_function_register_progresser_cb(void *register_progresser_cb_data, rrd_function_progresser_cb_t progresser_cb, void *progresser_cb_data) {
    struct rrd_function_inflight *r = register_progresser_cb_data;
    r->progresser.cb = progresser_cb;
    r->progresser.data = progresser_cb_data;
}

// ----------------------------------------------------------------------------
// waiting for async function completion

struct rrd_function_call_wait {
    RRDHOST *host;
    const DICTIONARY_ITEM *host_function_acquired;
    char *transaction;

    bool free_with_signal;
    bool data_are_ready;
    netdata_mutex_t mutex;
    pthread_cond_t cond;
    int code;
};

static void rrd_inflight_function_cleanup(RRDHOST *host __maybe_unused,
                                          const char *transaction) {
    dictionary_del(rrd_functions_inflight_requests, transaction);
    dictionary_garbage_collect(rrd_functions_inflight_requests);
}

static void rrd_function_call_wait_free(struct rrd_function_call_wait *tmp) {
    rrd_inflight_function_cleanup(tmp->host, tmp->transaction);
    freez(tmp->transaction);

    pthread_cond_destroy(&tmp->cond);
    netdata_mutex_destroy(&tmp->mutex);
    freez(tmp);
}

static void rrd_async_function_signal_when_ready(BUFFER *temp_wb __maybe_unused, int code, void *callback_data) {
    struct rrd_function_call_wait *tmp = callback_data;
    bool we_should_free = false;

    netdata_mutex_lock(&tmp->mutex);

    // since we got the mutex,
    // the waiting thread is either in pthread_cond_timedwait()
    // or gave up and left.

    tmp->code = code;
    tmp->data_are_ready = true;

    if(tmp->free_with_signal)
        we_should_free = true;

    pthread_cond_signal(&tmp->cond);

    netdata_mutex_unlock(&tmp->mutex);

    if(we_should_free) {
        buffer_free(temp_wb);
        rrd_function_call_wait_free(tmp);
    }
}

static void rrd_inflight_async_function_nowait_finished(BUFFER *wb, int code, void *data) {
    struct rrd_function_inflight *r = data;

    if(r->result.cb)
        r->result.cb(wb, code, r->result.data);

    rrd_inflight_function_cleanup(r->host, r->transaction);
}

static bool rrd_inflight_async_function_is_cancelled(void *data) {
    struct rrd_function_inflight *r = data;
    return __atomic_load_n(&r->cancelled, __ATOMIC_RELAXED);
}

static inline int rrd_call_function_async_and_dont_wait(struct rrd_function_inflight *r) {
    int code = r->rdcf->execute_cb(&r->transaction_uuid, r->result.wb,
                                   &r->stop_monotonic_ut, r->sanitized_cmd, r->rdcf->execute_cb_data,
                                   rrd_inflight_async_function_nowait_finished, r,
                                   r->progress.cb, r->progress.data,
                                   rrd_inflight_async_function_is_cancelled, r,
                                   rrd_inflight_async_function_register_canceller_cb, r,
                                   rrd_inflight_async_function_register_progresser_cb, r);

    if(code != HTTP_RESP_OK) {
        if (!buffer_strlen(r->result.wb))
            rrd_call_function_error(r->result.wb, "Failed to send request to the collector.", code);

        rrd_inflight_function_cleanup(r->host, r->transaction);
    }

    return code;
}

static int rrd_call_function_async_and_wait(struct rrd_function_inflight *r) {
    struct rrd_function_call_wait *tmp = mallocz(sizeof(struct rrd_function_call_wait));
    tmp->free_with_signal = false;
    tmp->data_are_ready = false;
    tmp->host = r->host;
    tmp->host_function_acquired = r->host_function_acquired;
    tmp->transaction = strdupz(r->transaction);
    netdata_mutex_init(&tmp->mutex);
    pthread_cond_init(&tmp->cond, NULL);

    // we need a temporary BUFFER, because we may time out and the caller supplied one may vanish,
    // so we create a new one we guarantee will survive until the collector finishes...

    bool we_should_free = true;
    BUFFER *temp_wb  = buffer_create(PLUGINSD_LINE_MAX + 1, &netdata_buffers_statistics.buffers_functions); // we need it because we may give up on it
    temp_wb->content_type = r->result.wb->content_type;

    int code = r->rdcf->execute_cb(&r->transaction_uuid, temp_wb, &r->stop_monotonic_ut,
                                   r->sanitized_cmd, r->rdcf->execute_cb_data,
                                   // we overwrite the result callbacks,
                                   // so that we can clean up the allocations made
                                   rrd_async_function_signal_when_ready, tmp,
                                   r->progress.cb, r->progress.data,
                                   rrd_inflight_async_function_is_cancelled, r,
                                   rrd_inflight_async_function_register_canceller_cb, r,
                                   rrd_inflight_async_function_register_progresser_cb, r);

    if (code == HTTP_RESP_OK) {
        netdata_mutex_lock(&tmp->mutex);

        bool cancelled = false;
        int rc = 0;
        while (rc == 0 && !cancelled && !tmp->data_are_ready) {
            usec_t now_mono_ut = now_monotonic_usec();
            usec_t stop_mono_ut = __atomic_load_n(&r->stop_monotonic_ut, __ATOMIC_RELAXED) + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT;
            if(now_mono_ut > stop_mono_ut) {
                rc = ETIMEDOUT;
                break;
            }

            // wait for 10ms, and loop again...
            struct timespec tp;
            clock_gettime(CLOCK_REALTIME, &tp);
            tp.tv_nsec += 10 * NSEC_PER_MSEC;
            if(tp.tv_nsec > (long)(1 * NSEC_PER_SEC)) {
                tp.tv_sec++;
                tp.tv_nsec -= 1 * NSEC_PER_SEC;
            }

            // the mutex is unlocked within pthread_cond_timedwait()
            rc = pthread_cond_timedwait(&tmp->cond, &tmp->mutex, &tp);
            // the mutex is again ours

            if(rc == ETIMEDOUT) {
                // 10ms have passed

                rc = 0;
                if (!tmp->data_are_ready && r->is_cancelled.cb &&
                    r->is_cancelled.cb(r->is_cancelled.data)) {
//                    internal_error(true, "FUNCTIONS: transaction '%s' is cancelled while waiting for response",
//                                   r->transaction);
                    cancelled = true;
                    rrd_function_cancel_inflight(r);
                    break;
                }
            }
        }

        if (tmp->data_are_ready) {
            // we have a response
            buffer_fast_strcat(r->result.wb, buffer_tostring(temp_wb), buffer_strlen(temp_wb));
            r->result.wb->content_type = temp_wb->content_type;
            r->result.wb->expires = temp_wb->expires;

            if(r->result.wb->expires)
                buffer_cacheable(r->result.wb);
            else
                buffer_no_cacheable(r->result.wb);

            code = tmp->code;
        }
        else if (rc == ETIMEDOUT || cancelled) {
            // timeout
            // we will go away and let the callback free the structure
            tmp->free_with_signal = true;
            we_should_free = false;

            if(cancelled)
                code = rrd_call_function_error(r->result.wb,
                                               "Request cancelled",
                                               HTTP_RESP_CLIENT_CLOSED_REQUEST);
            else
                code = rrd_call_function_error(r->result.wb,
                                               "Timeout while waiting for a response from the collector.",
                                               HTTP_RESP_GATEWAY_TIMEOUT);
        }
        else
            code = rrd_call_function_error(r->result.wb,
                                           "Internal error while communicating with the collector",
                                           HTTP_RESP_INTERNAL_SERVER_ERROR);

        netdata_mutex_unlock(&tmp->mutex);
    }
    else {
        if(!buffer_strlen(r->result.wb))
            rrd_call_function_error(r->result.wb, "The collector returned an error.", code);
    }

    if (we_should_free) {
        rrd_function_call_wait_free(tmp);
        buffer_free(temp_wb);
    }

    return code;
}

static inline int rrd_call_function_async(struct rrd_function_inflight *r, bool wait) {
    if(wait)
        return rrd_call_function_async_and_wait(r);
    else
        return rrd_call_function_async_and_dont_wait(r);
}


void call_virtual_function_async(BUFFER *wb, RRDHOST *host, const char *name, const char *payload, rrd_function_result_callback_t callback, void *callback_data);
// ----------------------------------------------------------------------------

int rrd_function_run(RRDHOST *host, BUFFER *result_wb, int timeout_s, HTTP_ACCESS access, const char *cmd,
                     bool wait, const char *transaction,
                     rrd_function_result_callback_t result_cb, void *result_cb_data,
                     rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                     rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data, const char *payload) {

    int code;
    char sanitized_cmd[PLUGINSD_LINE_MAX + 1];
    const DICTIONARY_ITEM *host_function_acquired = NULL;

    // ------------------------------------------------------------------------
    // find the function

    size_t sanitized_cmd_length = sanitize_function_text(sanitized_cmd, cmd, PLUGINSD_LINE_MAX);

    if (is_dyncfg_function(sanitized_cmd, DYNCFG_FUNCTION_TYPE_ALL)) {
        call_virtual_function_async(result_wb, host, sanitized_cmd, payload, result_cb, result_cb_data);
        return HTTP_RESP_OK;
    }

    code = rrd_call_function_find(host, result_wb, sanitized_cmd, sanitized_cmd_length, &host_function_acquired);
    if(code != HTTP_RESP_OK)
        return code;

    struct rrd_host_function *rdcf = dictionary_acquired_item_value(host_function_acquired);

    if(access != HTTP_ACCESS_ADMINS && rdcf->access != HTTP_ACCESS_ANY && access > rdcf->access) {

        if(!aclk_connected)
            rrd_call_function_error(result_wb, "This Netdata must be connected to Netdata Cloud to access this function.", HTTP_RESP_PRECOND_FAIL);
        else if(access >= HTTP_ACCESS_ANY)
            rrd_call_function_error(result_wb, "You need to login to the Netdata Cloud space this agent is claimed to, to access this function.", HTTP_RESP_PRECOND_FAIL);
        else /* if(access < HTTP_ACCESS_ANY && rdcf->access < access) */
            rrd_call_function_error(result_wb, "To access this function you need to be an admin in this Netdata Cloud space.", HTTP_RESP_PRECOND_FAIL);

        dictionary_acquired_item_release(host->functions, host_function_acquired);
        return HTTP_RESP_PRECOND_FAIL;
    }

    if(timeout_s <= 0)
        timeout_s = rdcf->timeout;

    // ------------------------------------------------------------------------
    // validate and parse the transaction, or generate a new transaction id

    char uuid_str[UUID_COMPACT_STR_LEN];
    uuid_t uuid;

    if(!transaction || !*transaction || uuid_parse_flexi(transaction, uuid) != 0)
        uuid_generate_random(uuid);

    uuid_unparse_lower_compact(uuid, uuid_str);
    transaction = uuid_str;

    // ------------------------------------------------------------------------
    // the function can only be executed in async mode
    // put the function into the inflight requests

    struct rrd_function_inflight t = {
            .used = false,
            .host = host,
            .cmd = strdupz(cmd),
            .sanitized_cmd = strdupz(sanitized_cmd),
            .sanitized_cmd_length = sanitized_cmd_length,
            .transaction = strdupz(transaction),
            .timeout = timeout_s,
            .cancelled = false,
            .stop_monotonic_ut = now_monotonic_usec() + timeout_s * USEC_PER_SEC,
            .host_function_acquired = host_function_acquired,
            .rdcf = rdcf,
            .result = {
                    .wb = result_wb,
                    .cb = result_cb,
                    .data = result_cb_data,
            },
            .is_cancelled = {
                    .cb = is_cancelled_cb,
                    .data = is_cancelled_cb_data,
            },
            .progress = {
                    .cb = progress_cb,
                    .data = progress_cb_data,
            },
    };
    uuid_copy(t.transaction_uuid, uuid);

    struct rrd_function_inflight *r = dictionary_set(rrd_functions_inflight_requests, transaction, &t, sizeof(t));
    if(r->used) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "FUNCTIONS: duplicate transaction '%s', function: '%s'",
               t.transaction, t.cmd);

        code = rrd_call_function_error(result_wb, "duplicate transaction", HTTP_RESP_BAD_REQUEST);
        freez((void *)t.transaction);
        freez((void *)t.cmd);
        freez((void *)t.sanitized_cmd);
        dictionary_acquired_item_release(r->host->functions, t.host_function_acquired);
        return code;
    }
    r->used = true;
    // internal_error(true, "FUNCTIONS: transaction '%s' started", r->transaction);

    if(r->rdcf->sync) {
        // the caller has to wait
        code = r->rdcf->execute_cb(&r->transaction_uuid, r->result.wb,
                                &r->stop_monotonic_ut, r->sanitized_cmd, r->rdcf->execute_cb_data,
                                r->result.cb, r->result.data,
                                r->progress.cb, r->progress.data,
                                r->is_cancelled.cb, r->is_cancelled.data,  // it is ok to pass these, we block the caller
                                NULL, NULL,   // no need to register canceller, we will wait
                                NULL, NULL    // ?? do we need a progresser in this case?
        );

        if(code != HTTP_RESP_OK && !buffer_strlen(result_wb))
            rrd_call_function_error(result_wb, "Collector reported error.", code);

        rrd_inflight_function_cleanup(host, r->transaction);
        return code;
    }

    return rrd_call_function_async(r, wait);
}

static void rrd_function_cancel_inflight(struct rrd_function_inflight *r) {
    if(!r)
        return;

    bool cancelled = __atomic_load_n(&r->cancelled, __ATOMIC_RELAXED);
    if(cancelled) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: received a CANCEL request for transaction '%s', but it is already cancelled.",
               r->transaction);
        return;
    }

    __atomic_store_n(&r->cancelled, true, __ATOMIC_RELAXED);

    if(!rrd_collector_dispatcher_acquire(r->rdcf->collector)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: received a CANCEL request for transaction '%s', but the collector is not running.",
               r->transaction);
        return;
    }

    if(r->canceller.cb)
        r->canceller.cb(r->canceller.data);

    rrd_collector_dispatcher_release(r->rdcf->collector);
}

void rrd_function_cancel(const char *transaction) {
    // internal_error(true, "FUNCTIONS: request to cancel transaction '%s'", transaction);

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(rrd_functions_inflight_requests, transaction);
    if(!item) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: received a CANCEL request for transaction '%s', but the transaction is not running.",
               transaction);
        return;
    }

    struct rrd_function_inflight *r = dictionary_acquired_item_value(item);
    rrd_function_cancel_inflight(r);
    dictionary_acquired_item_release(rrd_functions_inflight_requests, item);
}

void rrd_function_progress(const char *transaction) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(rrd_functions_inflight_requests, transaction);
    if(!item) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: received a PROGRESS request for transaction '%s', but the transaction is not running.",
               transaction);
        return;
    }

    struct rrd_function_inflight *r = dictionary_acquired_item_value(item);

    if(!rrd_collector_dispatcher_acquire(r->rdcf->collector)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: received a PROGRESS request for transaction '%s', but the collector is not running.",
               transaction);
        goto cleanup;
    }

    functions_stop_monotonic_update_on_progress(&r->stop_monotonic_ut);

    if(r->progresser.cb)
        r->progresser.cb(r->progresser.data);

    rrd_collector_dispatcher_release(r->rdcf->collector);

cleanup:
    dictionary_acquired_item_release(rrd_functions_inflight_requests, item);
}

void rrd_function_call_progresser(uuid_t *transaction) {
    char str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(*transaction, str);
    rrd_function_progress(str);
}

// ----------------------------------------------------------------------------

static void functions2json(DICTIONARY *functions, BUFFER *wb)
{
    struct rrd_host_function *t;
    dfe_start_read(functions, t)
    {
        if (!rrd_collector_running(t->collector))
            continue;

        buffer_json_member_add_object(wb, t_dfe.name);
        {
            buffer_json_member_add_string_or_empty(wb, "help", string2str(t->help));
            buffer_json_member_add_int64(wb, "timeout", (int64_t) t->timeout);

            char options[65];
            snprintfz(
                    options, 64
                    , "%s%s"
                    , (t->options & RRD_FUNCTION_LOCAL) ? "LOCAL " : ""
                    , (t->options & RRD_FUNCTION_GLOBAL) ? "GLOBAL" : ""
                    );

            buffer_json_member_add_string_or_empty(wb, "options", options);
            buffer_json_member_add_string_or_empty(wb, "tags", string2str(t->tags));
            buffer_json_member_add_string(wb, "access", http_id2access(t->access));
            buffer_json_member_add_uint64(wb, "priority", t->priority);
        }
        buffer_json_object_close(wb);
    }
    dfe_done(t);
}

void chart_functions2json(RRDSET *st, BUFFER *wb) {
    if(!st || !st->functions_view) return;

    functions2json(st->functions_view, wb);
}

void host_functions2json(RRDHOST *host, BUFFER *wb) {
    if(!host || !host->functions) return;

    buffer_json_member_add_object(wb, "functions");

    struct rrd_host_function *t;
    dfe_start_read(host->functions, t) {
        if(!rrd_collector_running(t->collector)) continue;

        buffer_json_member_add_object(wb, t_dfe.name);
        {
            buffer_json_member_add_string(wb, "help", string2str(t->help));
            buffer_json_member_add_int64(wb, "timeout", t->timeout);
            buffer_json_member_add_array(wb, "options");
            {
                if (t->options & RRD_FUNCTION_GLOBAL)
                    buffer_json_add_array_item_string(wb, "GLOBAL");
                if (t->options & RRD_FUNCTION_LOCAL)
                    buffer_json_add_array_item_string(wb, "LOCAL");
            }
            buffer_json_array_close(wb);
            buffer_json_member_add_string(wb, "tags", string2str(t->tags));
            buffer_json_member_add_string(wb, "access", http_id2access(t->access));
            buffer_json_member_add_uint64(wb, "priority", t->priority);
        }
        buffer_json_object_close(wb);
    }
    dfe_done(t);

    buffer_json_object_close(wb);
}

void chart_functions_to_dict(DICTIONARY *rrdset_functions_view, DICTIONARY *dst, void *value, size_t value_size) {
    if(!rrdset_functions_view || !dst) return;

    struct rrd_host_function *t;
    dfe_start_read(rrdset_functions_view, t) {
        if(!rrd_collector_running(t->collector)) continue;

        dictionary_set(dst, t_dfe.name, value, value_size);
    }
    dfe_done(t);
}

void host_functions_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size, STRING **help, STRING **tags, HTTP_ACCESS *access, int *priority) {
    if(!host || !host->functions || !dictionary_entries(host->functions) || !dst) return;

    struct rrd_host_function *t;
    dfe_start_read(host->functions, t) {
        if(!rrd_collector_running(t->collector)) continue;

        if(help)
            *help = t->help;

        if(tags)
            *tags = t->tags;

        if(access)
            *access = t->access;

        if(priority)
            *priority = t->priority;

        dictionary_set(dst, t_dfe.name, value, value_size);
    }
    dfe_done(t);
}
