// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS

#include "rrd.h"
#include "rrdfunctions-internals.h"

#define MAX_FUNCTION_LENGTH (PLUGINSD_LINE_MAX - 512) // we need some space for the rest of the line

static unsigned char functions_allowed_chars[256] = {
        [0] = '\0', [1] = '_', [2] = '_', [3] = '_', [4] = '_', [5] = '_', [6] = '_', [7] = '_', [8] = '_',

        // control
        ['\t'] = ' ', ['\n'] = ' ', ['\v'] = ' ', [12] = ' ', ['\r'] = ' ',

        [14] = '_', [15] = '_', [16] = '_', [17] = '_', [18] = '_', [19] = '_', [20] = '_', [21] = '_',
        [22] = '_', [23] = '_', [24] = '_', [25] = '_', [26] = '_', [27] = '_', [28] = '_', [29] = '_',
        [30] = '_', [31] = '_',

        // symbols
        [' '] = ' ', ['!'] = '!', ['"'] = '\'', ['#'] = '#', ['$'] = '$', ['%'] = '%', ['&'] = '&', ['\''] = '\'',
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

size_t rrd_functions_sanitize(char *dst, const char *src, size_t dst_len) {
    return text_sanitize((unsigned char *)dst, (const unsigned char *)src, dst_len,
                         functions_allowed_chars, true, "", NULL);
}

// ----------------------------------------------------------------------------

// we keep a dictionary per RRDSET with these functions
// the dictionary is created on demand (only when a function is added to an RRDSET)

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

void rrd_functions_host_init(RRDHOST *host) {
    if(host->functions) return;

    host->functions = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                 &dictionary_stats_category_functions, sizeof(struct rrd_host_function));

    dictionary_register_insert_callback(host->functions, rrd_functions_insert_callback, host);
    dictionary_register_delete_callback(host->functions, rrd_functions_delete_callback, host);
    dictionary_register_conflict_callback(host->functions, rrd_functions_conflict_callback, host);
}

void rrd_functions_host_destroy(RRDHOST *host) {
    dictionary_destroy(host->functions);
}

// ----------------------------------------------------------------------------

static inline bool is_function_dyncfg(const char *name) {
    if(!name || !*name)
        return false;

    if(strncmp(name, PLUGINSD_FUNCTION_CONFIG, sizeof(PLUGINSD_FUNCTION_CONFIG) - 1) != 0)
        return false;

    char c = name[sizeof(PLUGINSD_FUNCTION_CONFIG) - 1];
    if(c == 0 || isspace(c))
        return true;

    return false;
}

void rrd_function_add(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority,
                      const char *help, const char *tags,
                      HTTP_ACCESS access, bool sync,
                      rrd_function_execute_cb_t execute_cb, void *execute_cb_data) {

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

    char key[strlen(name) + 1];
    rrd_functions_sanitize(key, name, sizeof(key));

    struct rrd_host_function tmp = {
        .sync = sync,
        .timeout = timeout,
        .options = st ? RRD_FUNCTION_LOCAL: (is_function_dyncfg(name) ? RRD_FUNCTION_DYNCFG : RRD_FUNCTION_GLOBAL),
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

void rrd_function_del(RRDHOST *host, RRDSET *st, const char *name) {
    char key[strlen(name) + 1];
    rrd_functions_sanitize(key, name, sizeof(key));
    dictionary_del(host->functions, key);

    if(st)
        dictionary_del(st->functions_view, key);
    else
        rrdhost_flag_set(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    dictionary_garbage_collect(host->functions);
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

int rrd_functions_find_by_name(RRDHOST *host, BUFFER *wb, const char *name, size_t key_length, const DICTIONARY_ITEM **item) {
    char buffer[MAX_FUNCTION_LENGTH + 1];
    strncpyz(buffer, name, sizeof(buffer) - 1);
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

bool rrd_function_available(RRDHOST *host, const char *function) {
    if(!host || !host->functions)
        return false;

    bool ret = false;
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->functions, function);
    if(item) {
        struct rrd_host_function *rdcf = dictionary_acquired_item_value(item);
        if(rrd_collector_running(rdcf->collector))
            ret = true;

        dictionary_acquired_item_release(host->functions, item);
    }

    return ret;
}
