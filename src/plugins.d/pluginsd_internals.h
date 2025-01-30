// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGINSD_INTERNALS_H
#define NETDATA_PLUGINSD_INTERNALS_H

#include "pluginsd_parser.h"
#include "pluginsd_functions.h"
#include "pluginsd_dyncfg.h"
#include "pluginsd_replication.h"

#define SERVING_STREAMING(parser) ((parser)->repertoire == PARSER_INIT_STREAMING)
#define SERVING_PLUGINSD(parser) ((parser)->repertoire == PARSER_INIT_PLUGINSD)

PARSER_RC PLUGINSD_DISABLE_PLUGIN(PARSER *parser, const char *keyword, const char *msg);

ssize_t send_to_plugin(const char *txt, PARSER *parser, STREAM_TRAFFIC_TYPE type);

static ALWAYS_INLINE RRDHOST *pluginsd_require_scope_host(PARSER *parser, const char *cmd) {
    RRDHOST *host = parser->user.host;

    if(unlikely(!host))
        netdata_log_error("PLUGINSD: command %s requires a host, but is not set.", cmd);

    return host;
}

static ALWAYS_INLINE RRDSET *pluginsd_require_scope_chart(PARSER *parser, const char *cmd, const char *parent_cmd) {
    RRDSET *st = parser->user.st;

    if(unlikely(!st))
        netdata_log_error("PLUGINSD: command %s requires a chart defined via command %s, but is not set.", cmd, parent_cmd);

    return st;
}

static inline RRDSET *pluginsd_get_scope_chart(PARSER *parser) {
    return parser->user.st;
}

static inline void rrdset_data_collection_lock_with_trace(PARSER *parser, const char *func) {
    if(parser->user.st && !parser->user.v2.locked_data_collection) {
        spinlock_lock_with_trace(&parser->user.st->data_collection_lock, func);
        parser->user.v2.locked_data_collection = true;
    }
}

static inline bool rrdset_data_collection_unlock_with_trace(PARSER *parser, const char *func) {
    if(parser->user.st && parser->user.v2.locked_data_collection) {
        spinlock_unlock_with_trace(&parser->user.st->data_collection_lock, func);
        parser->user.v2.locked_data_collection = false;
        return true;
    }

    return false;
}

#define rrdset_data_collection_lock(parser) rrdset_data_collection_lock_with_trace(parser, __FUNCTION__)
#define rrdset_data_collection_unlock(parser) rrdset_data_collection_unlock_with_trace(parser, __FUNCTION__)

static ALWAYS_INLINE void rrdset_previous_scope_chart_unlock(PARSER *parser, const char *keyword, bool stale) {
    if(unlikely(rrdset_data_collection_unlock(parser))) {
        if(stale)
            netdata_log_error("PLUGINSD: 'host:%s/chart:%s/' stale data collection lock found during %s; it has been unlocked",
                              rrdhost_hostname(parser->user.st->rrdhost),
                              rrdset_id(parser->user.st),
                              keyword);
    }

    if(unlikely(parser->user.v2.ml_locked)) {
        ml_chart_update_end(parser->user.st);
        parser->user.v2.ml_locked = false;

        if(stale)
            netdata_log_error("PLUGINSD: 'host:%s/chart:%s/' stale ML lock found during %s, it has been unlocked",
                              rrdhost_hostname(parser->user.st->rrdhost),
                              rrdset_id(parser->user.st),
                              keyword);
    }
}

static inline void pluginsd_clear_scope_chart(PARSER *parser, const char *keyword) {
    rrdset_previous_scope_chart_unlock(parser, keyword, true);

    if(parser->user.cleanup_slots && parser->user.st)
        rrdset_pluginsd_receive_unslot(parser->user.st);

    parser->user.st = NULL;
    parser->user.cleanup_slots = false;
}

static ALWAYS_INLINE bool pluginsd_set_scope_chart(PARSER *parser, RRDSET *st, const char *keyword) {
    RRDSET *old_st = parser->user.st;
    pid_t old_collector_tid = (old_st) ? old_st->pluginsd.collector_tid : 0;
    pid_t my_collector_tid = gettid_cached();

    if(unlikely(old_collector_tid)) {
        if(old_collector_tid != my_collector_tid) {
            nd_log_limit_static_global_var(erl, 1, 0);
            nd_log_limit(&erl, NDLS_COLLECTORS, NDLP_WARNING,
                         "PLUGINSD: keyword %s: 'host:%s/chart:%s' is collected twice (my tid %d, other collector tid %d)",
                         keyword ? keyword : "UNKNOWN",
                         rrdhost_hostname(st->rrdhost), rrdset_id(st),
                         my_collector_tid, old_collector_tid);

            return false;
        }

        old_st->pluginsd.collector_tid = 0;
    }

    st->pluginsd.collector_tid = my_collector_tid;

    pluginsd_clear_scope_chart(parser, keyword);

    st->pluginsd.pos = 0;
    parser->user.st = st;
    parser->user.cleanup_slots = false;

    return true;
}

static inline void pluginsd_rrddim_put_to_slot(PARSER *parser, RRDSET *st, RRDDIM *rd, ssize_t slot, bool obsolete)  {
    size_t wanted_size = st->pluginsd.size;

    if(slot >= 1) {
        st->pluginsd.dims_with_slots = true;
        wanted_size = slot;
    }
    else {
        st->pluginsd.dims_with_slots = false;
        wanted_size = dictionary_entries(st->rrddim_root_index);
    }

    if(wanted_size > st->pluginsd.size) {
        st->pluginsd.prd_array = reallocz(st->pluginsd.prd_array, wanted_size * sizeof(struct pluginsd_rrddim));

        // initialize the empty slots
        for(ssize_t i = (ssize_t) wanted_size - 1; i >= (ssize_t) st->pluginsd.size; i--) {
            st->pluginsd.prd_array[i].rda = NULL;
            st->pluginsd.prd_array[i].rd = NULL;
            st->pluginsd.prd_array[i].id = NULL;
        }

        rrd_slot_memory_added((wanted_size - st->pluginsd.size) * sizeof(struct pluginsd_rrddim));
        st->pluginsd.size = wanted_size;
    }

    if(st->pluginsd.dims_with_slots) {
        struct pluginsd_rrddim *prd = &st->pluginsd.prd_array[slot - 1];

        if(prd->rd != rd) {
            prd->rda = rrddim_find_and_acquire(st, string2str(rd->id));
            prd->rd = rrddim_acquired_to_rrddim(prd->rda);
            prd->id = string2str(prd->rd->id);
        }

        if(obsolete)
            parser->user.cleanup_slots = true;
    }
}

static ALWAYS_INLINE RRDDIM *pluginsd_acquire_dimension(RRDHOST *host, RRDSET *st, const char *dimension, ssize_t slot, const char *cmd) {
    if (unlikely(!dimension || !*dimension)) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s' got a %s, without a dimension.",
                          rrdhost_hostname(host), rrdset_id(st), cmd);
        return NULL;
    }

    if (unlikely(!st->pluginsd.size)) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s' got a %s, but the chart has no dimensions.",
                          rrdhost_hostname(host), rrdset_id(st), cmd);
        return NULL;
    }

    struct pluginsd_rrddim *prd;
    RRDDIM *rd;

    if(likely(st->pluginsd.dims_with_slots)) {
        // caching with slots

        if(unlikely(slot < 1 || slot > (ssize_t)st->pluginsd.size)) {
            netdata_log_error("PLUGINSD: 'host:%s/chart:%s' got a %s with slot %zd, but slots in the range [1 - %u] are expected.",
                              rrdhost_hostname(host), rrdset_id(st), cmd, slot, st->pluginsd.size);
            return NULL;
        }

        prd = &st->pluginsd.prd_array[slot - 1];

        rd = prd->rd;
        if(likely(rd)) {
#ifdef NETDATA_INTERNAL_CHECKS
            if(strcmp(prd->id, dimension) != 0) {
                ssize_t t;
                for(t = 0; t < st->pluginsd.size ;t++) {
                    if (strcmp(st->pluginsd.prd_array[t].id, dimension) == 0)
                        break;
                }
                if(t >= st->pluginsd.size)
                    t = -1;

                internal_fatal(true,
                               "PLUGINSD: expected to find dimension '%s' on slot %zd, but found '%s', "
                               "the right slot is %zd",
                               dimension, slot, prd->id, t);
            }
#endif
            return rd;
        }
    }
    else {
        // caching without slots

        if(unlikely(st->pluginsd.pos >= st->pluginsd.size))
            st->pluginsd.pos = 0;

        prd = &st->pluginsd.prd_array[st->pluginsd.pos++];

        rd = prd->rd;
        if(likely(rd)) {
            const char *id = prd->id;

            if(strcmp(id, dimension) == 0) {
                // we found it cached
                return rd;
            }
            else {
                // the cached one is not good for us
                rrddim_acquired_release(prd->rda);
                prd->rda = NULL;
                prd->rd = NULL;
                prd->id = NULL;
            }
        }
    }

    // we need to find the dimension and set it to prd

    RRDDIM_ACQUIRED *rda = rrddim_find_and_acquire(st, dimension);
    if (unlikely(!rda)) {
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s/dim:%s' got a %s but dimension does not exist.",
                          rrdhost_hostname(host), rrdset_id(st), dimension, cmd);

        return NULL;
    }

    prd->rda = rda;
    prd->rd = rd = rrddim_acquired_to_rrddim(rda);
    prd->id = string2str(rd->id);

    return rd;
}

static inline RRDSET *pluginsd_find_chart(RRDHOST *host, const char *chart, const char *cmd) {
    if (unlikely(!chart || !*chart)) {
        netdata_log_error("PLUGINSD: 'host:%s' got a %s without a chart id.",
                          rrdhost_hostname(host), cmd);
        return NULL;
    }

    RRDSET *st = rrdset_find(host, chart);
    if (unlikely(!st))
        netdata_log_error("PLUGINSD: 'host:%s/chart:%s' got a %s but chart does not exist.",
                          rrdhost_hostname(host), chart, cmd);

    return st;
}

static ALWAYS_INLINE ssize_t pluginsd_parse_rrd_slot(char **words, size_t num_words) {
    ssize_t slot = -1;
    char *id = get_word(words, num_words, 1);
    if(id && id[0] == PLUGINSD_KEYWORD_SLOT[0] && id[1] == PLUGINSD_KEYWORD_SLOT[1] &&
       id[2] == PLUGINSD_KEYWORD_SLOT[2] && id[3] == PLUGINSD_KEYWORD_SLOT[3] && id[4] == ':') {
        slot = (ssize_t) str2ull_encoded(&id[5]);
        if(slot < 0) slot = 0; // to make the caller increment its idx of the words
    }

    return slot;
}

static inline void pluginsd_rrdset_cache_put_to_slot(PARSER *parser, RRDSET *st, ssize_t slot, bool obsolete) {
    // clean possible old cached data
    rrdset_pluginsd_receive_unslot(st);

    if(unlikely(slot < 1 || slot >= INT32_MAX))
        return;

    RRDHOST *host = st->rrdhost;

    if(unlikely((size_t)slot > host->stream.rcv.pluginsd_chart_slots.size)) {
        spinlock_lock(&host->stream.rcv.pluginsd_chart_slots.spinlock);
        size_t old_slots = host->stream.rcv.pluginsd_chart_slots.size;
        size_t new_slots = (old_slots < PLUGINSD_MIN_RRDSET_POINTERS_CACHE) ? PLUGINSD_MIN_RRDSET_POINTERS_CACHE : old_slots * 2;

        if(new_slots < (size_t)slot)
            new_slots = slot;

        host->stream.rcv.pluginsd_chart_slots.array =
                reallocz(host->stream.rcv.pluginsd_chart_slots.array, new_slots * sizeof(RRDSET *));

        for(size_t i = old_slots; i < new_slots ;i++)
            host->stream.rcv.pluginsd_chart_slots.array[i] = NULL;

        host->stream.rcv.pluginsd_chart_slots.size = new_slots;
        spinlock_unlock(&host->stream.rcv.pluginsd_chart_slots.spinlock);

        rrd_slot_memory_added((new_slots - old_slots) * sizeof(uint32_t));
    }

    host->stream.rcv.pluginsd_chart_slots.array[slot - 1] = st;
    st->pluginsd.last_slot = (int32_t)slot - 1;
    parser->user.cleanup_slots = obsolete;
}

static ALWAYS_INLINE RRDSET *pluginsd_rrdset_cache_get_from_slot(PARSER *parser, RRDHOST *host, const char *id, ssize_t slot, const char *keyword) {
    if(unlikely(slot < 1 || (size_t)slot > host->stream.rcv.pluginsd_chart_slots.size))
        return pluginsd_find_chart(host, id, keyword);

    RRDSET *st = host->stream.rcv.pluginsd_chart_slots.array[slot - 1];

    if(!st) {
        st = pluginsd_find_chart(host, id, keyword);
        if(st)
            pluginsd_rrdset_cache_put_to_slot(parser, st, slot, rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE));
    }
    else {
        internal_fatal(string_strcmp(st->id, id) != 0,
                       "PLUGINSD: wrong chart in slot %zd, expected '%s', found '%s'",
                       slot - 1, id, string2str(st->id));
    }

    return st;
}

static inline SN_FLAGS pluginsd_parse_storage_number_flags(const char *flags_str) {
    SN_FLAGS flags = SN_FLAG_NONE;

    char c;
    while ((c = *flags_str++)) {
        switch (c) {
            case 'A':
                flags |= SN_FLAG_NOT_ANOMALOUS;
                break;

            case 'R':
                flags |= SN_FLAG_RESET;
                break;

            case 'E':
                flags = SN_EMPTY_SLOT;
                return flags;

            default:
                internal_error(true, "Unknown SN_FLAGS flag '%c'", c);
                break;
        }
    }

    return flags;
}

#endif //NETDATA_PLUGINSD_INTERNALS_H
