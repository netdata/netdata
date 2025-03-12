// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"
#include "rrdfunctions-internals.h"

#define MAX_FUNCTION_LENGTH (PLUGINSD_LINE_MAX - 512) // we need some space for the rest of the line

// ----------------------------------------------------------------------------

// we keep a dictionary per RRDSET with these functions
// the dictionary is created on demand (only when a function is added to an RRDSET)

// ----------------------------------------------------------------------------

static void rrd_functions_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func, void *rrdhost) {
    RRDHOST *host = rrdhost;
    struct rrd_host_function *rdcf = func;

    rrd_collector_started();
    rdcf->collector = rrd_collector_acquire_current_thread();
    rdcf->rrdhost_state_id = object_state_id(&host->state_id);

    if(!rdcf->priority)
        rdcf->priority = RRDFUNCTIONS_PRIORITY_DEFAULT;

//    internal_error(true, "FUNCTIONS: adding function '%s' on host '%s', collection tid %d, %s",
//                   dictionary_acquired_item_name(item), rrdhost_hostname(host),
//                   rdcf->collector->tid, rdcf->collector->running ? "running" : "NOT running");
}

static void rrd_functions_cleanup(struct rrd_host_function *rdcf) {
    rrd_collector_release(rdcf->collector);
    string_freez(rdcf->help);
    string_freez(rdcf->tags);
}

static void rrd_functions_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *func,
                                          void *rrdhost __maybe_unused) {
    struct rrd_host_function *rdcf = func;
    rrd_functions_cleanup(rdcf);
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

        new_rdcf->collector = rdcf->collector;
        rdcf->collector = rrd_collector_acquire_current_thread();
        changed = true;
    }

    if(rdcf->rrdhost_state_id != object_state_id(&host->state_id)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed state id from %u to %u",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               rdcf->rrdhost_state_id,
               object_state_id(&host->state_id));

        rdcf->rrdhost_state_id = object_state_id(&host->state_id);
        changed = true;
    }

    if(rdcf->execute_cb != new_rdcf->execute_cb) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed execute callback",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->execute_cb, new_rdcf->execute_cb);
        changed = true;
    }

    if(rdcf->help != new_rdcf->help) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed help text",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->help, new_rdcf->help);
        changed = true;
    }

    if(rdcf->tags != new_rdcf->tags) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed tags",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->tags, new_rdcf->tags);
        changed = true;
    }

    if(rdcf->timeout != new_rdcf->timeout) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed timeout (from %d to %d)",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               rdcf->timeout, new_rdcf->timeout);

        SWAP(rdcf->timeout, new_rdcf->timeout);
        changed = true;
    }

    if(rdcf->version != new_rdcf->version) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed version (from %"PRIu32", to %"PRIu32")",
               dictionary_acquired_item_name(item), rrdhost_hostname(host),
               rdcf->version, new_rdcf->version);

        SWAP(rdcf->version, new_rdcf->version);
        changed = true;
    }

    if(rdcf->priority != new_rdcf->priority) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed priority",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->priority, new_rdcf->priority);
        changed = true;
    }

    if(rdcf->access != new_rdcf->access) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed access level",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->access, new_rdcf->access);
        changed = true;
    }

    if(rdcf->sync != new_rdcf->sync) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed sync/async mode",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->sync, new_rdcf->sync);
        changed = true;
    }

    if(rdcf->execute_cb_data != new_rdcf->execute_cb_data) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "FUNCTIONS: function '%s' of host '%s' changed execute callback data",
               dictionary_acquired_item_name(item), rrdhost_hostname(host));

        SWAP(rdcf->execute_cb_data, new_rdcf->execute_cb_data);
        changed = true;
    }

//    internal_error(true, "FUNCTIONS: adding function '%s' on host '%s', collection tid %d, %s",
//                   dictionary_acquired_item_name(item), rrdhost_hostname(host),
//                   rdcf->collector->tid, rdcf->collector->running ? "running" : "NOT running");

    rrd_functions_cleanup(new_rdcf);

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
    host->functions = NULL;
}

// ----------------------------------------------------------------------------

static inline bool is_function_restricted(const char *name, const char *tags) {
    return (name && name[0] == '_' && name[1] == '_') || (tags && strstr(tags, RRDFUNCTIONS_TAG_HIDDEN) != NULL);
}

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

static inline RRD_FUNCTION_OPTIONS get_function_options(RRDSET *st, const char *name, const char *tags) {
    if(is_function_dyncfg(name))
        return RRD_FUNCTION_DYNCFG;

    RRD_FUNCTION_OPTIONS options = st ? RRD_FUNCTION_LOCAL : RRD_FUNCTION_GLOBAL;

    return options | (is_function_restricted(name, tags) ? RRD_FUNCTION_RESTRICTED : 0);
}

void rrd_function_add(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, uint32_t version,
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
        .collector = NULL,
        .sync = sync,
        .timeout = timeout,
        .version = version,
        .priority = priority,
        .options = get_function_options(st, name, tags),
        .access = access,
        .execute_cb = execute_cb,
        .execute_cb_data = execute_cb_data,
        .help = string_strdupz(help),
        .tags = string_strdupz(tags),
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

int rrd_functions_find_by_name(RRDHOST *host, BUFFER *wb, const char *name, size_t key_length, const DICTIONARY_ITEM **item) {
    char buffer[MAX_FUNCTION_LENGTH + 1];
    strncpyz(buffer, name, sizeof(buffer) - 1);
    char *s = NULL;

    OBJECT_STATE_ID state_id = object_state_id(&host->state_id);

    bool found = false;
    *item = NULL;
    if(host->functions) {
        while (buffer[0]) {
            if((*item = dictionary_get_and_acquire_item(host->functions, buffer))) {
                found = true;

                struct rrd_host_function *rdcf = dictionary_acquired_item_value(*item);
                if(rrd_collector_running(rdcf->collector) && rdcf->rrdhost_state_id == state_id) {
                    break;
                }
                else {

                    nd_log(NDLS_DAEMON, NDLP_DEBUG,
                           "Function '%s' is not available. "
                           "host '%s', collector = { tid: %d, running: %s }, host tid { rcv: %d, snd: %d }, host state { id: %u, expected %u }, hops: %d",
                           name,
                           rrdhost_hostname(host),
                           rrd_collector_tid(rdcf->collector),
                           rrd_collector_running(rdcf->collector) ? "yes" : "no",
                           host->stream.rcv.status.tid, host->stream.snd.status.tid,
                           state_id, rdcf->rrdhost_state_id,
                           rrdhost_ingestion_hops(host)
                           );

                    dictionary_acquired_item_release(host->functions, *item);
                    *item = NULL;
                }
            }

            // if s == NULL, set it to the end of the buffer;
            // this should happen only the first time
            if (unlikely(!s))
                s = &buffer[key_length - 1];

            // skip a word from the end
            while (s >= buffer && !isspace((uint8_t)*s)) *s-- = '\0';

            // skip all spaces
            while (s >= buffer && isspace((uint8_t)*s)) *s-- = '\0';
        }
    }

    buffer_flush(wb);

    if(!(*item)) {
        if(found)
            return rrd_call_function_error(wb,
                                           "The plugin that registered this feature, is not currently running.",
                                           HTTP_RESP_SERVICE_UNAVAILABLE);
        else
            return rrd_call_function_error(wb,
                                           "This feature is not available on this host at this time.",
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
        if(rrd_collector_running(rdcf->collector) && rdcf->rrdhost_state_id == object_state_id(&host->state_id))
            ret = true;

        dictionary_acquired_item_release(host->functions, item);
    }

    return ret;
}
