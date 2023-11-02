// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS
#include "rrd.h"

#define MAX_FUNCTION_LENGTH (PLUGINSD_LINE_MAX - 512) // we need some space for the rest of the line

static unsigned char functions_allowed_chars[256] = {
    [0] = '\0', //
    [1] = '_', //
    [2] = '_', //
    [3] = '_', //
    [4] = '_', //
    [5] = '_', //
    [6] = '_', //
    [7] = '_', //
    [8] = '_', //
    [9] = ' ', // Horizontal Tab
    [10] = ' ', // Line Feed
    [11] = ' ', // Vertical Tab
    [12] = ' ', // Form Feed
    [13] = ' ', // Carriage Return
    [14] = '_', //
    [15] = '_', //
    [16] = '_', //
    [17] = '_', //
    [18] = '_', //
    [19] = '_', //
    [20] = '_', //
    [21] = '_', //
    [22] = '_', //
    [23] = '_', //
    [24] = '_', //
    [25] = '_', //
    [26] = '_', //
    [27] = '_', //
    [28] = '_', //
    [29] = '_', //
    [30] = '_', //
    [31] = '_', //
    [32] = ' ', // SPACE keep
    [33] = '!', // ! keep
    [34] = '"', // " keep
    [35] = '#', // # keep
    [36] = '$', // $ keep
    [37] = '%', // % keep
    [38] = '&', // & keep
    [39] = '\'', // ' keep
    [40] = '(', // ( keep
    [41] = ')', // ) keep
    [42] = '*', // * keep
    [43] = '+', // + keep
    [44] = ',', // , keep
    [45] = '-', // - keep
    [46] = '.', // . keep
    [47] = '/', // / keep
    [48] = '0', // 0 keep
    [49] = '1', // 1 keep
    [50] = '2', // 2 keep
    [51] = '3', // 3 keep
    [52] = '4', // 4 keep
    [53] = '5', // 5 keep
    [54] = '6', // 6 keep
    [55] = '7', // 7 keep
    [56] = '8', // 8 keep
    [57] = '9', // 9 keep
    [58] = ':', // : keep
    [59] = ';', // ; keep
    [60] = '<', // < keep
    [61] = '=', // = keep
    [62] = '>', // > keep
    [63] = '?', // ? keep
    [64] = '@', // @ keep
    [65] = 'A', // A keep
    [66] = 'B', // B keep
    [67] = 'C', // C keep
    [68] = 'D', // D keep
    [69] = 'E', // E keep
    [70] = 'F', // F keep
    [71] = 'G', // G keep
    [72] = 'H', // H keep
    [73] = 'I', // I keep
    [74] = 'J', // J keep
    [75] = 'K', // K keep
    [76] = 'L', // L keep
    [77] = 'M', // M keep
    [78] = 'N', // N keep
    [79] = 'O', // O keep
    [80] = 'P', // P keep
    [81] = 'Q', // Q keep
    [82] = 'R', // R keep
    [83] = 'S', // S keep
    [84] = 'T', // T keep
    [85] = 'U', // U keep
    [86] = 'V', // V keep
    [87] = 'W', // W keep
    [88] = 'X', // X keep
    [89] = 'Y', // Y keep
    [90] = 'Z', // Z keep
    [91] = '[', // [ keep
    [92] = '\\', // backslash keep
    [93] = ']', // ] keep
    [94] = '^', // ^ keep
    [95] = '_', // _ keep
    [96] = '`', // ` keep
    [97] = 'a', // a keep
    [98] = 'b', // b keep
    [99] = 'c', // c keep
    [100] = 'd', // d keep
    [101] = 'e', // e keep
    [102] = 'f', // f keep
    [103] = 'g', // g keep
    [104] = 'h', // h keep
    [105] = 'i', // i keep
    [106] = 'j', // j keep
    [107] = 'k', // k keep
    [108] = 'l', // l keep
    [109] = 'm', // m keep
    [110] = 'n', // n keep
    [111] = 'o', // o keep
    [112] = 'p', // p keep
    [113] = 'q', // q keep
    [114] = 'r', // r keep
    [115] = 's', // s keep
    [116] = 't', // t keep
    [117] = 'u', // u keep
    [118] = 'v', // v keep
    [119] = 'w', // w keep
    [120] = 'x', // x keep
    [121] = 'y', // y keep
    [122] = 'z', // z keep
    [123] = '{', // { keep
    [124] = '|', // | keep
    [125] = '}', // } keep
    [126] = '~', // ~ keep
    [127] = '_', //
    [128] = '_', //
    [129] = '_', //
    [130] = '_', //
    [131] = '_', //
    [132] = '_', //
    [133] = '_', //
    [134] = '_', //
    [135] = '_', //
    [136] = '_', //
    [137] = '_', //
    [138] = '_', //
    [139] = '_', //
    [140] = '_', //
    [141] = '_', //
    [142] = '_', //
    [143] = '_', //
    [144] = '_', //
    [145] = '_', //
    [146] = '_', //
    [147] = '_', //
    [148] = '_', //
    [149] = '_', //
    [150] = '_', //
    [151] = '_', //
    [152] = '_', //
    [153] = '_', //
    [154] = '_', //
    [155] = '_', //
    [156] = '_', //
    [157] = '_', //
    [158] = '_', //
    [159] = '_', //
    [160] = '_', //
    [161] = '_', //
    [162] = '_', //
    [163] = '_', //
    [164] = '_', //
    [165] = '_', //
    [166] = '_', //
    [167] = '_', //
    [168] = '_', //
    [169] = '_', //
    [170] = '_', //
    [171] = '_', //
    [172] = '_', //
    [173] = '_', //
    [174] = '_', //
    [175] = '_', //
    [176] = '_', //
    [177] = '_', //
    [178] = '_', //
    [179] = '_', //
    [180] = '_', //
    [181] = '_', //
    [182] = '_', //
    [183] = '_', //
    [184] = '_', //
    [185] = '_', //
    [186] = '_', //
    [187] = '_', //
    [188] = '_', //
    [189] = '_', //
    [190] = '_', //
    [191] = '_', //
    [192] = '_', //
    [193] = '_', //
    [194] = '_', //
    [195] = '_', //
    [196] = '_', //
    [197] = '_', //
    [198] = '_', //
    [199] = '_', //
    [200] = '_', //
    [201] = '_', //
    [202] = '_', //
    [203] = '_', //
    [204] = '_', //
    [205] = '_', //
    [206] = '_', //
    [207] = '_', //
    [208] = '_', //
    [209] = '_', //
    [210] = '_', //
    [211] = '_', //
    [212] = '_', //
    [213] = '_', //
    [214] = '_', //
    [215] = '_', //
    [216] = '_', //
    [217] = '_', //
    [218] = '_', //
    [219] = '_', //
    [220] = '_', //
    [221] = '_', //
    [222] = '_', //
    [223] = '_', //
    [224] = '_', //
    [225] = '_', //
    [226] = '_', //
    [227] = '_', //
    [228] = '_', //
    [229] = '_', //
    [230] = '_', //
    [231] = '_', //
    [232] = '_', //
    [233] = '_', //
    [234] = '_', //
    [235] = '_', //
    [236] = '_', //
    [237] = '_', //
    [238] = '_', //
    [239] = '_', //
    [240] = '_', //
    [241] = '_', //
    [242] = '_', //
    [243] = '_', //
    [244] = '_', //
    [245] = '_', //
    [246] = '_', //
    [247] = '_', //
    [248] = '_', //
    [249] = '_', //
    [250] = '_', //
    [251] = '_', //
    [252] = '_', //
    [253] = '_', //
    [254] = '_', //
    [255] = '_'  //
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

struct rrd_host_function {
    bool sync;                      // when true, the function is called synchronously
    RRD_FUNCTION_OPTIONS options;   // RRD_FUNCTION_OPTIONS
    STRING *help;
    int timeout;                    // the default timeout of the function

    rrd_function_execute_cb_t execute_cb;

    void *execute_cb_data;
    struct rrd_collector *collector;
};

// Each function points to this collector structure
// so that when the collector exits, all of them will
// be invalidated (running == false)
// The last function that is using this collector
// frees the structure too (or when the collector calls
// rrdset_collector_finished()).

struct rrd_collector {
    int32_t refcount;
    int32_t refcount_canceller;
    pid_t tid;
    bool running;
};

// Each thread that adds RRDSET functions, has to call
// rrdset_collector_started() and rrdset_collector_finished()
// to create the collector structure.

static __thread struct rrd_collector *thread_rrd_collector = NULL;

static void rrd_collector_free(struct rrd_collector *rdc) {
    if(rdc->running)
        return;

    int32_t expected = 0;
    if(!__atomic_compare_exchange_n(&rdc->refcount, &expected, -1, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED)) {
        // the collector is still referenced by charts.
        // leave it hanging there, the last chart will actually free it.
        return;
    }

    // we can free it now
    freez(rdc);
}

// called once per collector
void rrd_collector_started(void) {
    if(!thread_rrd_collector)
        thread_rrd_collector = callocz(1, sizeof(struct rrd_collector));

    thread_rrd_collector->tid = gettid();
    __atomic_store_n(&thread_rrd_collector->running, true, __ATOMIC_RELAXED);
}

// called once per collector
void rrd_collector_finished(void) {
    if(!thread_rrd_collector)
        return;

    __atomic_store_n(&thread_rrd_collector->running, false, __ATOMIC_RELAXED);

    int32_t expected = 0;
    while(!__atomic_compare_exchange_n(&thread_rrd_collector->refcount_canceller, &expected, -1, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED)) {
        expected = 0;
        sleep_usec(1 * USEC_PER_MS);
    }

    rrd_collector_free(thread_rrd_collector);
    thread_rrd_collector = NULL;
}

#define rrd_collector_running(c) __atomic_load_n(&(c)->running, __ATOMIC_RELAXED)

static struct rrd_collector *rrd_collector_acquire(void) {
    rrd_collector_started();

    int32_t expected = __atomic_load_n(&thread_rrd_collector->refcount, __ATOMIC_RELAXED), wanted = 0;
    do {
        if(expected < 0 || !rrd_collector_running(thread_rrd_collector)) {
            internal_fatal(true, "FUNCTIONS: Trying to acquire a collector that is exiting.");
            return thread_rrd_collector;
        }

        wanted = expected + 1;

    } while(!__atomic_compare_exchange_n(&thread_rrd_collector->refcount, &expected, wanted, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

    return thread_rrd_collector;
}

static void rrd_collector_release(struct rrd_collector *rdc) {
    if(unlikely(!rdc)) return;

    int32_t expected = __atomic_load_n(&rdc->refcount, __ATOMIC_RELAXED), wanted = 0;
    do {
        if(expected < 0) {
            internal_fatal(true, "FUNCTIONS: Trying to release a collector that is exiting.");
            return;
        }

        if(expected == 0) {
            internal_fatal(true, "FUNCTIONS: Trying to release a collector that is not acquired.");
            return;
        }

        wanted = expected - 1;

    } while(!__atomic_compare_exchange_n(&rdc->refcount, &expected, wanted, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED));

    if(wanted == 0)
        rrd_collector_free(rdc);
}

static void rrd_functions_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func, void *rrdhost) {
    RRDHOST *host = rrdhost; (void)host;
    struct rrd_host_function *rdcf = func;

    rrd_collector_started();
    rdcf->collector = rrd_collector_acquire();

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
        netdata_log_info("FUNCTIONS: function '%s' of host '%s' changed collector from %d to %d",
                         dictionary_acquired_item_name(item), rrdhost_hostname(host), rdcf->collector->tid, thread_rrd_collector->tid);

        struct rrd_collector *old_rdc = rdcf->collector;
        rdcf->collector = rrd_collector_acquire();
        rrd_collector_release(old_rdc);
        changed = true;
    }

    if(rdcf->execute_cb != new_rdcf->execute_cb) {
        netdata_log_info("FUNCTIONS: function '%s' of host '%s' changed execute callback",
                         dictionary_acquired_item_name(item), rrdhost_hostname(host));

        rdcf->execute_cb = new_rdcf->execute_cb;
        changed = true;
    }

    if(rdcf->help != new_rdcf->help) {
        netdata_log_info("FUNCTIONS: function '%s' of host '%s' changed help text",
                         dictionary_acquired_item_name(item), rrdhost_hostname(host));

        STRING *old = rdcf->help;
        rdcf->help = new_rdcf->help;
        string_freez(old);
        changed = true;
    }
    else
        string_freez(new_rdcf->help);

    if(rdcf->timeout != new_rdcf->timeout) {
        netdata_log_info("FUNCTIONS: function '%s' of host '%s' changed timeout",
                         dictionary_acquired_item_name(item), rrdhost_hostname(host));

        rdcf->timeout = new_rdcf->timeout;
        changed = true;
    }

    if(rdcf->sync != new_rdcf->sync) {
        netdata_log_info("FUNCTIONS: function '%s' of host '%s' changed sync/async mode",
                         dictionary_acquired_item_name(item), rrdhost_hostname(host));

        rdcf->sync = new_rdcf->sync;
        changed = true;
    }

    if(rdcf->execute_cb_data != new_rdcf->execute_cb_data) {
        netdata_log_info("FUNCTIONS: function '%s' of host '%s' changed execute callback data",
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

void rrd_function_add(RRDHOST *host, RRDSET *st, const char *name, int timeout, const char *help,
                      bool sync, rrd_function_execute_cb_t execute_cb, void *execute_cb_data) {

    // RRDSET *st may be NULL in this function
    // to create a GLOBAL function

    if(st && !st->functions_view)
        st->functions_view = dictionary_create_view(host->functions);

    char key[PLUGINSD_LINE_MAX + 1];
    sanitize_function_text(key, name, PLUGINSD_LINE_MAX);

    struct rrd_host_function tmp = {
        .sync = sync,
        .timeout = timeout,
        .options = (st)?RRD_FUNCTION_LOCAL:RRD_FUNCTION_GLOBAL,
        .execute_cb = execute_cb,
        .execute_cb_data = execute_cb_data,
        .help = string_strdupz(help),
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
                       , PLUGINSD_KEYWORD_FUNCTION " \"%s\" %d \"%s\"\n"
                       , tmp_dfe.name
                       , tmp->timeout
                       , string2str(tmp->help)
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
                , PLUGINSD_KEYWORD_FUNCTION " GLOBAL \"%s\" %d \"%s\"\n"
                , tmp_dfe.name
                , tmp->timeout
                , string2str(tmp->help)
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

            // if s == NULL, set it to the end of the buffer
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

struct rrd_function_inflight {
    bool used;

    RRDHOST *host;
    const char *transaction;
    const char *cmd;
    const char *sanitized_cmd;
    size_t sanitized_cmd_length;
    int timeout;
    bool cancelled;

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
        // to check if the function has been cancelled
        rrd_function_is_cancelled_cb_t cb;
        void *data;
    } is_cancelled;

    struct {
        // to be registered by the function itself
        // used to signal the function to cancel
        rrd_function_canceller_cb_t cb;
        void *data;
    } canceller;
};

static DICTIONARY *rrd_functions_inflight_requests = NULL;

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

static void rrd_inflight_async_function_register_canceller_cb(void *register_canceller_cb_data, rrd_function_canceller_cb_t canceller_cb, void *canceller_cb_data) {
    struct rrd_function_inflight *r = register_canceller_cb_data;
    r->canceller.cb = canceller_cb;
    r->canceller.data = canceller_cb_data;
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
                                          const DICTIONARY_ITEM *host_function_acquired __maybe_unused,
                                          const char *transaction) {
    dictionary_del(rrd_functions_inflight_requests, transaction);
    dictionary_garbage_collect(rrd_functions_inflight_requests);
}

static void rrd_function_call_wait_free(struct rrd_function_call_wait *tmp) {
    rrd_inflight_function_cleanup(tmp->host, tmp->host_function_acquired, tmp->transaction);
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

    rrd_inflight_function_cleanup(r->host, r->host_function_acquired, r->transaction);
}

static bool rrd_inflight_async_function_is_cancelled(void *data) {
    struct rrd_function_inflight *r = data;
    return __atomic_load_n(&r->cancelled, __ATOMIC_RELAXED);
}

static inline int rrd_call_function_async_and_dont_wait(struct rrd_function_inflight *r) {
    int code = r->rdcf->execute_cb(r->result.wb, r->timeout, r->sanitized_cmd, r->rdcf->execute_cb_data,
                                   rrd_inflight_async_function_nowait_finished, r,
                                   rrd_inflight_async_function_is_cancelled, r,
                                   rrd_inflight_async_function_register_canceller_cb, r);

    if(code != HTTP_RESP_OK) {
        if (!buffer_strlen(r->result.wb))
            rrd_call_function_error(r->result.wb, "Failed to send request to the collector.", code);

        rrd_inflight_function_cleanup(r->host, r->host_function_acquired, r->transaction);
    }

    return code;
}

static int rrd_call_function_async_and_wait(struct rrd_function_inflight *r) {
    struct timespec tp;
    clock_gettime(CLOCK_REALTIME, &tp);
    usec_t now_ut = tp.tv_sec * USEC_PER_SEC + tp.tv_nsec / NSEC_PER_USEC;
    usec_t end_ut = now_ut + r->timeout * USEC_PER_SEC + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT;

    struct rrd_function_call_wait *tmp = mallocz(sizeof(struct rrd_function_call_wait));
    tmp->free_with_signal = false;
    tmp->data_are_ready = false;
    tmp->host = r->host;
    tmp->host_function_acquired = r->host_function_acquired;
    tmp->transaction = strdupz(r->transaction);
    netdata_mutex_init(&tmp->mutex);
    pthread_cond_init(&tmp->cond, NULL);

    // we need a temporary BUFFER, because we may time out and the caller supplied one may vanish
    // so, we create a new one we guarantee will survive until the collector finishes...

    bool we_should_free = true;
    BUFFER *temp_wb  = buffer_create(PLUGINSD_LINE_MAX + 1, &netdata_buffers_statistics.buffers_functions); // we need it because we may give up on it
    temp_wb->content_type = r->result.wb->content_type;

    int code = r->rdcf->execute_cb(temp_wb, r->timeout, r->sanitized_cmd, r->rdcf->execute_cb_data,
            // we overwrite the result callbacks,
            // so that we can clean up the allocations made
                                   rrd_async_function_signal_when_ready, tmp,
                                   rrd_inflight_async_function_is_cancelled, r,
                                   rrd_inflight_async_function_register_canceller_cb, r);

    if (code == HTTP_RESP_OK) {
        netdata_mutex_lock(&tmp->mutex);

        bool cancelled = false;
        int rc = 0;
        while (rc == 0 && !cancelled && !tmp->data_are_ready) {
            clock_gettime(CLOCK_REALTIME, &tp);
            now_ut = tp.tv_sec * USEC_PER_SEC + tp.tv_nsec / NSEC_PER_USEC;

            if(now_ut >= end_ut) {
                rc = ETIMEDOUT;
                break;
            }

            tp.tv_nsec += 10 * NSEC_PER_MSEC;
            if(tp.tv_nsec > (long)(1 * NSEC_PER_SEC)) {
                tp.tv_sec++;
                tp.tv_nsec -= 1 * NSEC_PER_SEC;
            }

            // the mutex is unlocked within pthread_cond_timedwait()
            rc = pthread_cond_timedwait(&tmp->cond, &tmp->mutex, &tp);
            // the mutex is again ours

            if(rc == ETIMEDOUT) {
                rc = 0;
                if (!tmp->data_are_ready && r->is_cancelled.cb &&
                    r->is_cancelled.cb(r->is_cancelled.data)) {
//                    internal_error(true, "FUNCTIONS: transaction '%s' is cancelled while waiting for response",
//                                   r->transaction);
                    rc = 0;
                    cancelled = true;
                    rrd_function_cancel(r->transaction);
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

int rrd_function_run(RRDHOST *host, BUFFER *result_wb, int timeout, const char *cmd,
                     bool wait, const char *transaction,
                     rrd_function_result_callback_t result_cb, void *result_cb_data,
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

    if(timeout <= 0)
        timeout = rdcf->timeout;


    // ------------------------------------------------------------------------
    // the function can only be executed in sync mode

    if(rdcf->sync) {
        // the caller has to wait

        code = rdcf->execute_cb(result_wb, timeout, sanitized_cmd, rdcf->execute_cb_data,
                                result_cb, result_cb_data,
                                is_cancelled_cb, is_cancelled_cb_data,  // it is ok to pass these, we block the caller
                                NULL, NULL);                            // no need to pass, we will wait

        if (code != HTTP_RESP_OK && !buffer_strlen(result_wb))
            rrd_call_function_error(result_wb, "Collector reported error.", code);

        dictionary_acquired_item_release(host->functions, host_function_acquired);
        return code;
    }


    // ------------------------------------------------------------------------
    // the function can only be executed in async mode
    // put the function into the inflight requests

    char uuid_str[UUID_STR_LEN];
    if(!transaction) {
        uuid_t uuid;
        uuid_generate_random(uuid);
        uuid_unparse_lower(uuid, uuid_str);
        transaction = uuid_str;
    }

    // put the request into the inflight requests
    struct rrd_function_inflight t = {
            .used = false,
            .host = host,
            .cmd = strdupz(cmd),
            .sanitized_cmd = strdupz(sanitized_cmd),
            .sanitized_cmd_length = sanitized_cmd_length,
            .transaction = strdupz(transaction),
            .timeout = timeout,
            .cancelled = false,
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
            }
    };
    struct rrd_function_inflight *r = dictionary_set(rrd_functions_inflight_requests, transaction, &t, sizeof(t));
    if(r->used) {
        netdata_log_info("FUNCTIONS: duplicate transaction '%s', function: '%s'", t.transaction, t.cmd);
        code = rrd_call_function_error(result_wb, "duplicate transaction", HTTP_RESP_BAD_REQUEST);
        freez((void *)t.transaction);
        freez((void *)t.cmd);
        freez((void *)t.sanitized_cmd);
        dictionary_acquired_item_release(r->host->functions, t.host_function_acquired);
        return code;
    }
    r->used = true;
    // internal_error(true, "FUNCTIONS: transaction '%s' started", r->transaction);

    return rrd_call_function_async(r, wait);
}

void rrd_function_cancel(const char *transaction) {
    // internal_error(true, "FUNCTIONS: request to cancel transaction '%s'", transaction);

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(rrd_functions_inflight_requests, transaction);
    if(!item) {
        netdata_log_info("FUNCTIONS: received a cancel request for transaction '%s', but the transaction is not running.",
                         transaction);
        return;
    }

    struct rrd_function_inflight *r = dictionary_acquired_item_value(item);

    bool cancelled = __atomic_load_n(&r->cancelled, __ATOMIC_RELAXED);
    if(cancelled) {
        netdata_log_info("FUNCTIONS: received a cancel request for transaction '%s', but it is already cancelled.",
                         transaction);
        goto cleanup;
    }

    __atomic_store_n(&r->cancelled, true, __ATOMIC_RELAXED);

    int32_t expected = __atomic_load_n(&r->rdcf->collector->refcount_canceller, __ATOMIC_RELAXED);
    int32_t wanted;
    do {
        if(expected < 0) {
            netdata_log_info("FUNCTIONS: received a cancel request for transaction '%s', but the collector is not running.",
                             transaction);
            goto cleanup;
        }

        wanted = expected + 1;
    } while(!__atomic_compare_exchange_n(&r->rdcf->collector->refcount_canceller, &expected, wanted, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));

    if(r->canceller.cb)
        r->canceller.cb(r->canceller.data);

    __atomic_sub_fetch(&r->rdcf->collector->refcount_canceller, 1, __ATOMIC_RELAXED);

cleanup:
    dictionary_acquired_item_release(rrd_functions_inflight_requests, item);
}

// ----------------------------------------------------------------------------

static void functions2json(DICTIONARY *functions, BUFFER *wb, const char *ident, const char *kq, const char *sq) {
    struct rrd_host_function *t;
    dfe_start_read(functions, t) {
        if(!rrd_collector_running(t->collector)) continue;

        if(t_dfe.counter)
            buffer_strcat(wb, ",\n");

        buffer_sprintf(wb, "%s%s%s%s: {", ident, kq, t_dfe.name, kq);
        buffer_sprintf(wb, "\n\t%s%shelp%s: %s%s%s", ident, kq, kq, sq, string2str(t->help), sq);
        buffer_sprintf(wb, ",\n\t%s%stimeout%s: %d", ident, kq, kq, t->timeout);
        buffer_sprintf(wb, ",\n\t%s%soptions%s: \"%s%s\"", ident, kq, kq
                       , (t->options & RRD_FUNCTION_LOCAL)?"LOCAL ":""
                       , (t->options & RRD_FUNCTION_GLOBAL)?"GLOBAL ":""
                       );
        buffer_sprintf(wb, "\n%s}", ident);
    }
    dfe_done(t);
    buffer_strcat(wb, "\n");
}

void chart_functions2json(RRDSET *st, BUFFER *wb, int tabs, const char *kq, const char *sq) {
    if(!st || !st->functions_view) return;

    char ident[tabs + 1];
    ident[tabs] = '\0';
    while(tabs) ident[--tabs] = '\t';

    functions2json(st->functions_view, wb, ident, kq, sq);
}

void host_functions2json(RRDHOST *host, BUFFER *wb) {
    if(!host || !host->functions) return;

    buffer_json_member_add_object(wb, "functions");

    struct rrd_host_function *t;
    dfe_start_read(host->functions, t) {
        if(!rrd_collector_running(t->collector)) continue;

        buffer_json_member_add_object(wb, t_dfe.name);
        buffer_json_member_add_string(wb, "help", string2str(t->help));
        buffer_json_member_add_int64(wb, "timeout", t->timeout);
        buffer_json_member_add_array(wb, "options");
        if(t->options & RRD_FUNCTION_GLOBAL)
            buffer_json_add_array_item_string(wb, "GLOBAL");
        if(t->options & RRD_FUNCTION_LOCAL)
            buffer_json_add_array_item_string(wb, "LOCAL");
        buffer_json_array_close(wb);
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

void host_functions_to_dict(RRDHOST *host, DICTIONARY *dst, void *value, size_t value_size, STRING **help) {
    if(!host || !host->functions || !dictionary_entries(host->functions) || !dst) return;

    struct rrd_host_function *t;
    dfe_start_read(host->functions, t) {
        if(!rrd_collector_running(t->collector)) continue;

        if(help)
            *help = t->help;

        dictionary_set(dst, t_dfe.name, value, value_size);
    }
    dfe_done(t);
}

// ----------------------------------------------------------------------------

int rrdhost_function_streaming(BUFFER *wb, int timeout __maybe_unused, const char *function __maybe_unused,
                               void *collector_data __maybe_unused,
                               rrd_function_result_callback_t result_cb, void *result_cb_data,
                               rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
                               rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
                               void *register_canceller_cb_data __maybe_unused) {

    time_t now = now_realtime_sec();

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_STREAMING_HELP);
    buffer_json_member_add_array(wb, "data");

    size_t max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_MAX];
    size_t max_db_metrics = 0, max_db_instances = 0, max_db_contexts = 0;
    size_t max_collection_replication_instances = 0, max_streaming_replication_instances = 0;
    size_t max_ml_anomalous = 0, max_ml_normal = 0, max_ml_trained = 0, max_ml_pending = 0, max_ml_silenced = 0;
    {
        RRDHOST *host;
        dfe_start_read(rrdhost_root_index, host) {
                    RRDHOST_STATUS s;
                    rrdhost_status(host, now, &s);
                    buffer_json_add_array_item_array(wb);

                    if(s.db.metrics > max_db_metrics)
                        max_db_metrics = s.db.metrics;

                    if(s.db.instances > max_db_instances)
                        max_db_instances = s.db.instances;

                    if(s.db.contexts > max_db_contexts)
                        max_db_contexts = s.db.contexts;

                    if(s.ingest.replication.instances > max_collection_replication_instances)
                        max_collection_replication_instances = s.ingest.replication.instances;

                    if(s.stream.replication.instances > max_streaming_replication_instances)
                        max_streaming_replication_instances = s.stream.replication.instances;

                    for(int i = 0; i < STREAM_TRAFFIC_TYPE_MAX ;i++) {
                        if (s.stream.sent_bytes_on_this_connection_per_type[i] >
                            max_sent_bytes_on_this_connection_per_type[i])
                            max_sent_bytes_on_this_connection_per_type[i] =
                                    s.stream.sent_bytes_on_this_connection_per_type[i];
                    }

                    // retention
                    buffer_json_add_array_item_string(wb, rrdhost_hostname(s.host)); // Node
                    buffer_json_add_array_item_uint64(wb, s.db.first_time_s * MSEC_PER_SEC); // dbFrom
                    buffer_json_add_array_item_uint64(wb, s.db.last_time_s * MSEC_PER_SEC); // dbTo

                    if(s.db.first_time_s && s.db.last_time_s && s.db.last_time_s > s.db.first_time_s)
                        buffer_json_add_array_item_uint64(wb, s.db.last_time_s - s.db.first_time_s); // dbDuration
                    else
                        buffer_json_add_array_item_string(wb, NULL); // dbDuration

                    buffer_json_add_array_item_uint64(wb, s.db.metrics); // dbMetrics
                    buffer_json_add_array_item_uint64(wb, s.db.instances); // dbInstances
                    buffer_json_add_array_item_uint64(wb, s.db.contexts); // dbContexts

                    // statuses
                    buffer_json_add_array_item_string(wb, rrdhost_ingest_status_to_string(s.ingest.status)); // InStatus
                    buffer_json_add_array_item_string(wb, rrdhost_streaming_status_to_string(s.stream.status)); // OutStatus
                    buffer_json_add_array_item_string(wb, rrdhost_ml_status_to_string(s.ml.status)); // MLStatus

                    // collection
                    if(s.ingest.since) {
                        buffer_json_add_array_item_uint64(wb, s.ingest.since * MSEC_PER_SEC); // InSince
                        buffer_json_add_array_item_time_t(wb, s.now - s.ingest.since); // InAge
                    }
                    else {
                        buffer_json_add_array_item_string(wb, NULL); // InSince
                        buffer_json_add_array_item_string(wb, NULL); // InAge
                    }
                    buffer_json_add_array_item_string(wb, stream_handshake_error_to_string(s.ingest.reason)); // InReason
                    buffer_json_add_array_item_uint64(wb, s.ingest.hops); // InHops
                    buffer_json_add_array_item_double(wb, s.ingest.replication.completion); // InReplCompletion
                    buffer_json_add_array_item_uint64(wb, s.ingest.replication.instances); // InReplInstances
                    buffer_json_add_array_item_string(wb, s.ingest.peers.local.ip); // InLocalIP
                    buffer_json_add_array_item_uint64(wb, s.ingest.peers.local.port); // InLocalPort
                    buffer_json_add_array_item_string(wb, s.ingest.peers.peer.ip); // InRemoteIP
                    buffer_json_add_array_item_uint64(wb, s.ingest.peers.peer.port); // InRemotePort
                    buffer_json_add_array_item_string(wb, s.ingest.ssl ? "SSL" : "PLAIN"); // InSSL
                    stream_capabilities_to_json_array(wb, s.ingest.capabilities, NULL); // InCapabilities

                    // streaming
                    if(s.stream.since) {
                        buffer_json_add_array_item_uint64(wb, s.stream.since * MSEC_PER_SEC); // OutSince
                        buffer_json_add_array_item_time_t(wb, s.now - s.stream.since); // OutAge
                    }
                    else {
                        buffer_json_add_array_item_string(wb, NULL); // OutSince
                        buffer_json_add_array_item_string(wb, NULL); // OutAge
                    }
                    buffer_json_add_array_item_string(wb, stream_handshake_error_to_string(s.stream.reason)); // OutReason
                    buffer_json_add_array_item_uint64(wb, s.stream.hops); // OutHops
                    buffer_json_add_array_item_double(wb, s.stream.replication.completion); // OutReplCompletion
                    buffer_json_add_array_item_uint64(wb, s.stream.replication.instances); // OutReplInstances
                    buffer_json_add_array_item_string(wb, s.stream.peers.local.ip); // OutLocalIP
                    buffer_json_add_array_item_uint64(wb, s.stream.peers.local.port); // OutLocalPort
                    buffer_json_add_array_item_string(wb, s.stream.peers.peer.ip); // OutRemoteIP
                    buffer_json_add_array_item_uint64(wb, s.stream.peers.peer.port); // OutRemotePort
                    buffer_json_add_array_item_string(wb, s.stream.ssl ? "SSL" : "PLAIN"); // OutSSL
                    buffer_json_add_array_item_string(wb, s.stream.compression ? "COMPRESSED" : "UNCOMPRESSED"); // OutCompression
                    stream_capabilities_to_json_array(wb, s.stream.capabilities, NULL); // OutCapabilities
                    buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_DATA]);
                    buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_METADATA]);
                    buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_REPLICATION]);
                    buffer_json_add_array_item_uint64(wb, s.stream.sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_FUNCTIONS]);

                    buffer_json_add_array_item_array(wb); // OutAttemptHandshake
                    time_t last_attempt = 0;
                    for(struct rrdpush_destinations *d = host->destinations; d ; d = d->next) {
                        if(d->since > last_attempt)
                            last_attempt = d->since;

                        buffer_json_add_array_item_string(wb, stream_handshake_error_to_string(d->reason));
                    }
                    buffer_json_array_close(wb); // // OutAttemptHandshake

                    if(!last_attempt) {
                        buffer_json_add_array_item_string(wb, NULL); // OutAttemptSince
                        buffer_json_add_array_item_string(wb, NULL); // OutAttemptAge
                    }
                    else {
                        buffer_json_add_array_item_uint64(wb, last_attempt * 1000); // OutAttemptSince
                        buffer_json_add_array_item_time_t(wb, s.now - last_attempt); // OutAttemptAge
                    }

                    // ML
                    if(s.ml.status == RRDHOST_ML_STATUS_RUNNING) {
                        buffer_json_add_array_item_uint64(wb, s.ml.metrics.anomalous); // MlAnomalous
                        buffer_json_add_array_item_uint64(wb, s.ml.metrics.normal); // MlNormal
                        buffer_json_add_array_item_uint64(wb, s.ml.metrics.trained); // MlTrained
                        buffer_json_add_array_item_uint64(wb, s.ml.metrics.pending); // MlPending
                        buffer_json_add_array_item_uint64(wb, s.ml.metrics.silenced); // MlSilenced

                        if(s.ml.metrics.anomalous > max_ml_anomalous)
                            max_ml_anomalous = s.ml.metrics.anomalous;

                        if(s.ml.metrics.normal > max_ml_normal)
                            max_ml_normal = s.ml.metrics.normal;

                        if(s.ml.metrics.trained > max_ml_trained)
                            max_ml_trained = s.ml.metrics.trained;

                        if(s.ml.metrics.pending > max_ml_pending)
                            max_ml_pending = s.ml.metrics.pending;

                        if(s.ml.metrics.silenced > max_ml_silenced)
                            max_ml_silenced = s.ml.metrics.silenced;

                    }
                    else {
                        buffer_json_add_array_item_string(wb, NULL); // MlAnomalous
                        buffer_json_add_array_item_string(wb, NULL); // MlNormal
                        buffer_json_add_array_item_string(wb, NULL); // MlTrained
                        buffer_json_add_array_item_string(wb, NULL); // MlPending
                        buffer_json_add_array_item_string(wb, NULL); // MlSilenced
                    }

                    // close
                    buffer_json_array_close(wb);
        }
        dfe_done(host);
    }
    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        // Node
        buffer_rrdf_table_add_field(wb, field_id++, "Node", "Node's Hostname",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY,
                                    NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbFrom", "DB Data Retention From",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbTo", "DB Data Retention To",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbDuration", "DB Data Retention Duration",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbMetrics", "Time-series Metrics in the DB",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, max_db_metrics, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbInstances", "Instances in the DB",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, max_db_instances, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "dbContexts", "Contexts in the DB",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, max_db_contexts, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- statuses ---

        buffer_rrdf_table_add_field(wb, field_id++, "InStatus", "Data Collection Online Status",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);


        buffer_rrdf_table_add_field(wb, field_id++, "OutStatus", "Streaming Online Status",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlStatus", "ML Status",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- collection ---

        buffer_rrdf_table_add_field(wb, field_id++, "InSince", "Last Data Collection Status Change",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InAge", "Last Data Collection Online Status Change Age",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InReason", "Data Collection Online Status Reason",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InHops", "Data Collection Distance Hops from Origin Node",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InReplCompletion", "Inbound Replication Completion",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    1, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InReplInstances", "Inbound Replicating Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", max_collection_replication_instances, RRDF_FIELD_SORT_DESCENDING,
                                    NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InLocalIP", "Inbound Local IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InLocalPort", "Inbound Local Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InRemoteIP", "Inbound Remote IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InRemotePort", "Inbound Remote Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InSSL", "Inbound SSL Connection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "InCapabilities", "Inbound Connection Capabilities",
                                    RRDF_FIELD_TYPE_ARRAY, RRDF_FIELD_VISUAL_PILL, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        // --- streaming ---

        buffer_rrdf_table_add_field(wb, field_id++, "OutSince", "Last Streaming Status Change",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAge", "Last Streaming Status Change Age",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutReason", "Streaming Status Reason",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutHops", "Streaming Distance Hops from Origin Node",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutReplCompletion", "Outbound Replication Completion",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                                    1, "%", 100.0, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutReplInstances", "Outbound Replicating Instances",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "instances", max_streaming_replication_instances, RRDF_FIELD_SORT_DESCENDING,
                                    NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutLocalIP", "Outbound Local IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutLocalPort", "Outbound Local Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutRemoteIP", "Outbound Remote IP",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutRemotePort", "Outbound Remote Port",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutSSL", "Outbound SSL Connection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutCompression", "Outbound Compressed Connection",
                                    RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutCapabilities", "Outbound Connection Capabilities",
                                    RRDF_FIELD_TYPE_ARRAY, RRDF_FIELD_VISUAL_PILL, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficData", "Outbound Metric Data Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes", max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_DATA],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficMetadata", "Outbound Metric Metadata Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes",
                                    max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_METADATA],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficReplication", "Outbound Metric Replication Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes",
                                    max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_REPLICATION],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutTrafficFunctions", "Outbound Metric Functions Traffic",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "bytes",
                                    max_sent_bytes_on_this_connection_per_type[STREAM_TRAFFIC_TYPE_FUNCTIONS],
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAttemptHandshake",
                                    "Outbound Connection Attempt Handshake Status",
                                    RRDF_FIELD_TYPE_ARRAY, RRDF_FIELD_VISUAL_PILL, RRDF_FIELD_TRANSFORM_NONE,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAttemptSince",
                                    "Last Outbound Connection Attempt Status Change Time",
                                    RRDF_FIELD_TYPE_TIMESTAMP, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DATETIME_MS,
                                    0, NULL, NAN, RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MAX, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "OutAttemptAge",
                                    "Last Outbound Connection Attempt Status Change Age",
                                    RRDF_FIELD_TYPE_DURATION, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_DURATION_S,
                                    0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_MIN, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_VISIBLE, NULL);

        // --- ML ---

        buffer_rrdf_table_add_field(wb, field_id++, "MlAnomalous", "Number of Anomalous Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    max_ml_anomalous,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlNormal", "Number of Not Anomalous Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    max_ml_normal,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlTrained", "Number of Trained Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    max_ml_trained,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlPending", "Number of Pending Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    max_ml_pending,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);

        buffer_rrdf_table_add_field(wb, field_id++, "MlSilenced", "Number of Silenced Metrics",
                                    RRDF_FIELD_TYPE_INTEGER, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NUMBER,
                                    0, "metrics",
                                    max_ml_silenced,
                                    RRDF_FIELD_SORT_DESCENDING, NULL,
                                    RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_RANGE,
                                    RRDF_FIELD_OPTS_NONE, NULL);
    }
    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "Node");
    buffer_json_member_add_object(wb, "charts");
    {
        // Data Collection Age chart
        buffer_json_member_add_object(wb, "InAge");
        {
            buffer_json_member_add_string(wb, "name", "Data Collection Age");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "InAge");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // Streaming Age chart
        buffer_json_member_add_object(wb, "OutAge");
        {
            buffer_json_member_add_string(wb, "name", "Streaming Age");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "OutAge");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        // DB Duration
        buffer_json_member_add_object(wb, "dbDuration");
        {
            buffer_json_member_add_string(wb, "name", "Retention Duration");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "dbDuration");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "InAge");
        buffer_json_add_array_item_string(wb, "Node");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "OutAge");
        buffer_json_add_array_item_string(wb, "Node");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        buffer_json_member_add_object(wb, "Node");
        {
            buffer_json_member_add_string(wb, "name", "Node");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Node");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "InStatus");
        {
            buffer_json_member_add_string(wb, "name", "Nodes by Collection Status");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "InStatus");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "OutStatus");
        {
            buffer_json_member_add_string(wb, "name", "Nodes by Streaming Status");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "OutStatus");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "MlStatus");
        {
            buffer_json_member_add_string(wb, "name", "Nodes by ML Status");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "MlStatus");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "InRemoteIP");
        {
            buffer_json_member_add_string(wb, "name", "Nodes by Inbound IP");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "InRemoteIP");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "OutRemoteIP");
        {
            buffer_json_member_add_string(wb, "name", "Nodes by Outbound IP");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "OutRemoteIP");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    int response = HTTP_RESP_OK;
    if(is_cancelled_cb && is_cancelled_cb(is_cancelled_cb_data)) {
        buffer_flush(wb);
        response = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(result_cb)
        result_cb(wb, response, result_cb_data);

    return response;
}
