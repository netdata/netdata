// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"
#include <sched.h>

void __rrdset_check_rdlock(RRDSET *st, const char *file, const char *function, const unsigned long line) {
    debug(D_RRD_CALLS, "Checking read lock on chart '%s'", st->id);

    int ret = netdata_rwlock_trywrlock(&st->rrdset_rwlock);
    if(ret == 0)
        fatal("RRDSET '%s' should be read-locked, but it is not, at function %s() at line %lu of file '%s'", st->id, function, line, file);
}

void __rrdset_check_wrlock(RRDSET *st, const char *file, const char *function, const unsigned long line) {
    debug(D_RRD_CALLS, "Checking write lock on chart '%s'", st->id);

    int ret = netdata_rwlock_tryrdlock(&st->rrdset_rwlock);
    if(ret == 0)
        fatal("RRDSET '%s' should be write-locked, but it is not, at function %s() at line %lu of file '%s'", st->id, function, line, file);
}


// ----------------------------------------------------------------------------
// RRDSET index

int rrdset_compare(void* a, void* b) {
    if(((RRDSET *)a)->hash < ((RRDSET *)b)->hash) return -1;
    else if(((RRDSET *)a)->hash > ((RRDSET *)b)->hash) return 1;
    else return strcmp(((RRDSET *)a)->id, ((RRDSET *)b)->id);
}

static RRDSET *rrdset_index_find(RRDHOST *host, const char *id, uint32_t hash) {
    RRDSET tmp;
    strncpyz(tmp.id, id, RRD_ID_LENGTH_MAX);
    tmp.hash = (hash)?hash:simple_hash(tmp.id);

    return (RRDSET *)avl_search_lock(&(host->rrdset_root_index), (avl_t *) &tmp);
}

// ----------------------------------------------------------------------------
// RRDSET name index

#define rrdset_from_avlname(avlname_ptr) ((RRDSET *)((avlname_ptr) - offsetof(RRDSET, avlname)))

int rrdset_compare_name(void* a, void* b) {
    RRDSET *A = rrdset_from_avlname(a);
    RRDSET *B = rrdset_from_avlname(b);

    // fprintf(stderr, "COMPARING: %s with %s\n", A->name, B->name);

    if(A->hash_name < B->hash_name) return -1;
    else if(A->hash_name > B->hash_name) return 1;
    else return strcmp(A->name, B->name);
}

RRDSET *rrdset_index_add_name(RRDHOST *host, RRDSET *st) {
    void *result;
    // fprintf(stderr, "ADDING: %s (name: %s)\n", st->id, st->name);
    result = avl_insert_lock(&host->rrdset_root_index_name, (avl_t *) (&st->avlname));
    if(result) return rrdset_from_avlname(result);
    return NULL;
}

RRDSET *rrdset_index_del_name(RRDHOST *host, RRDSET *st) {
    void *result;
    // fprintf(stderr, "DELETING: %s (name: %s)\n", st->id, st->name);
    result = (RRDSET *)avl_remove_lock(&((host)->rrdset_root_index_name), (avl_t *)(&st->avlname));
    if(result) return rrdset_from_avlname(result);
    return NULL;
}


// ----------------------------------------------------------------------------
// RRDSET - find charts

static inline RRDSET *rrdset_index_find_name(RRDHOST *host, const char *name, uint32_t hash) {
    void *result = NULL;
    RRDSET tmp;
    tmp.name = name;
    tmp.hash_name = (hash)?hash:simple_hash(tmp.name);

    // fprintf(stderr, "SEARCHING: %s\n", name);
    result = avl_search_lock(&host->rrdset_root_index_name, (avl_t *) (&(tmp.avlname)));
    if(result) {
        RRDSET *st = rrdset_from_avlname(result);
        if(strcmp(st->magic, RRDSET_MAGIC) != 0)
            error("Search for RRDSET %s returned an invalid RRDSET %s (name %s)", name, st->id, st->name);

        // fprintf(stderr, "FOUND: %s\n", name);
        return rrdset_from_avlname(result);
    }
    // fprintf(stderr, "NOT FOUND: %s\n", name);
    return NULL;
}

inline RRDSET *rrdset_find(RRDHOST *host, const char *id) {
    debug(D_RRD_CALLS, "rrdset_find() for chart '%s' in host '%s'", id, host->hostname);
    RRDSET *st = rrdset_index_find(host, id, 0);
    return(st);
}

inline RRDSET *rrdset_find_bytype(RRDHOST *host, const char *type, const char *id) {
    debug(D_RRD_CALLS, "rrdset_find_bytype() for chart '%s.%s' in host '%s'", type, id, host->hostname);

    char buf[RRD_ID_LENGTH_MAX + 1];
    strncpyz(buf, type, RRD_ID_LENGTH_MAX - 1);
    strcat(buf, ".");
    int len = (int) strlen(buf);
    strncpyz(&buf[len], id, (size_t) (RRD_ID_LENGTH_MAX - len));

    return(rrdset_find(host, buf));
}

inline RRDSET *rrdset_find_byname(RRDHOST *host, const char *name) {
    debug(D_RRD_CALLS, "rrdset_find_byname() for chart '%s' in host '%s'", name, host->hostname);
    RRDSET *st = rrdset_index_find_name(host, name, 0);
    return(st);
}

// ----------------------------------------------------------------------------
// RRDSET - rename charts

char *rrdset_strncpyz_name(char *to, const char *from, size_t length) {
    char c, *p = to;

    while (length-- && (c = *from++)) {
        if(c != '.' && !isalnum(c))
            c = '_';

        *p++ = c;
    }

    *p = '\0';

    return to;
}

int rrdset_set_name(RRDSET *st, const char *name) {
    if(unlikely(st->name && !strcmp(st->name, name)))
        return 1;

    RRDHOST *host = st->rrdhost;

    debug(D_RRD_CALLS, "rrdset_set_name() old: '%s', new: '%s'", st->name?st->name:"", name);

    char b[CONFIG_MAX_VALUE + 1];
    char n[RRD_ID_LENGTH_MAX + 1];

    snprintfz(n, RRD_ID_LENGTH_MAX, "%s.%s", st->type, name);
    rrdset_strncpyz_name(b, n, CONFIG_MAX_VALUE);

    if(rrdset_index_find_name(host, b, 0)) {
        info("RRDSET: chart name '%s' on host '%s' already exists.", b, host->hostname);
        return 0;
    }

    if(st->name) {
        rrdset_index_del_name(host, st);
        st->name = config_set_default(st->config_section, "name", b);
        st->hash_name = simple_hash(st->name);
        rrdsetvar_rename_all(st);
    }
    else {
        st->name = config_get(st->config_section, "name", b);
        st->hash_name = simple_hash(st->name);
    }

    rrdset_wrlock(st);
    RRDDIM *rd;
    rrddim_foreach_write(rd, st)
        rrddimvar_rename_all(rd);
    rrdset_unlock(st);

    if(unlikely(rrdset_index_add_name(host, st) != st))
        error("RRDSET: INTERNAL ERROR: attempted to index duplicate chart name '%s'", st->name);

    rrdset_flag_clear(st, RRDSET_FLAG_EXPORTING_SEND);
    rrdset_flag_clear(st, RRDSET_FLAG_EXPORTING_IGNORE);
    rrdset_flag_clear(st, RRDSET_FLAG_BACKEND_SEND);
    rrdset_flag_clear(st, RRDSET_FLAG_BACKEND_IGNORE);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_SEND);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_IGNORE);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

    return 2;
}

inline void rrdset_is_obsolete(RRDSET *st) {
    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED))) {
        info("Cannot obsolete already archived chart %s", st->name);
        return;
    }

    if(unlikely(!(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)))) {
        rrdset_flag_set(st, RRDSET_FLAG_OBSOLETE);
        st->rrdhost->obsolete_charts_count++;

        rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

        // the chart will not get more updates (data collection)
        // so, we have to push its definition now
        rrdset_push_chart_definition_now(st);
    }
}

inline void rrdset_isnot_obsolete(RRDSET *st) {
    if(unlikely((rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)))) {
        rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE);
        st->rrdhost->obsolete_charts_count--;

        rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);

        // the chart will be pushed upstream automatically
        // due to data collection
    }
}

inline void rrdset_update_heterogeneous_flag(RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    (void)host;

    RRDDIM *rd;

    rrdset_flag_clear(st, RRDSET_FLAG_HOMOGENEOUS_CHECK);

    RRD_ALGORITHM algorithm = st->dimensions->algorithm;
    collected_number multiplier = ABS(st->dimensions->multiplier);
    collected_number divisor = ABS(st->dimensions->divisor);

    rrddim_foreach_read(rd, st) {
        if(algorithm != rd->algorithm || multiplier != ABS(rd->multiplier) || divisor != ABS(rd->divisor)) {
            if(!rrdset_flag_check(st, RRDSET_FLAG_HETEROGENEOUS)) {
                #ifdef NETDATA_INTERNAL_CHECKS
                info("Dimension '%s' added on chart '%s' of host '%s' is not homogeneous to other dimensions already present (algorithm is '%s' vs '%s', multiplier is " COLLECTED_NUMBER_FORMAT " vs " COLLECTED_NUMBER_FORMAT ", divisor is " COLLECTED_NUMBER_FORMAT " vs " COLLECTED_NUMBER_FORMAT ").",
                        rd->name,
                        st->name,
                        host->hostname,
                        rrd_algorithm_name(rd->algorithm), rrd_algorithm_name(algorithm),
                        rd->multiplier, multiplier,
                        rd->divisor, divisor
                );
                #endif
                rrdset_flag_set(st, RRDSET_FLAG_HETEROGENEOUS);
            }
            return;
        }
    }

    rrdset_flag_clear(st, RRDSET_FLAG_HETEROGENEOUS);
}

// ----------------------------------------------------------------------------
// RRDSET - reset a chart

void rrdset_reset(RRDSET *st) {
    debug(D_RRD_CALLS, "rrdset_reset() %s", st->name);

    st->last_collected_time.tv_sec = 0;
    st->last_collected_time.tv_usec = 0;
    st->last_updated.tv_sec = 0;
    st->last_updated.tv_usec = 0;
    st->current_entry = 0;
    st->counter = 0;
    st->counter_done = 0;
    st->rrddim_page_alignment = 0;

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        rd->last_collected_time.tv_sec = 0;
        rd->last_collected_time.tv_usec = 0;
        rd->collections_counter = 0;
        // memset(rd->values, 0, rd->entries * sizeof(storage_number));
#ifdef ENABLE_DBENGINE
        if (RRD_MEMORY_MODE_DBENGINE == st->rrd_memory_mode && !rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
            rrdeng_store_metric_flush_current_page(rd);
        }
#endif
    }
}

// ----------------------------------------------------------------------------
// RRDSET - helpers for rrdset_create()

inline long align_entries_to_pagesize(RRD_MEMORY_MODE mode, long entries) {
    if(unlikely(entries < 5)) entries = 5;
    if(unlikely(entries > RRD_HISTORY_ENTRIES_MAX)) entries = RRD_HISTORY_ENTRIES_MAX;

    if(unlikely(mode == RRD_MEMORY_MODE_NONE || mode == RRD_MEMORY_MODE_ALLOC))
        return entries;

    long page = (size_t)sysconf(_SC_PAGESIZE);
    long size = sizeof(RRDDIM) + entries * sizeof(storage_number);
    if(unlikely(size % page)) {
        size -= (size % page);
        size += page;

        long n = (size - sizeof(RRDDIM)) / sizeof(storage_number);
        return n;
    }

    return entries;
}

static inline void last_collected_time_align(RRDSET *st) {
    st->last_collected_time.tv_sec -= st->last_collected_time.tv_sec % st->update_every;

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST)))
        st->last_collected_time.tv_usec = 0;
    else
        st->last_collected_time.tv_usec = 500000;
}

static inline void last_updated_time_align(RRDSET *st) {
    st->last_updated.tv_sec -= st->last_updated.tv_sec % st->update_every;
    st->last_updated.tv_usec = 0;
}

// ----------------------------------------------------------------------------
// RRDSET - free a chart

void rrdset_free(RRDSET *st) {
    if(unlikely(!st)) return;

    RRDHOST *host = st->rrdhost;

    rrdhost_check_wrlock(host);  // make sure we have a write lock on the host
    rrdset_wrlock(st);                  // lock this RRDSET
    // info("Removing chart '%s' ('%s')", st->id, st->name);

    // ------------------------------------------------------------------------
    // remove it from the indexes

    if(unlikely(rrdset_index_del(host, st) != st))
        error("RRDSET: INTERNAL ERROR: attempt to remove from index chart '%s', removed a different chart.", st->id);

    rrdset_index_del_name(host, st);

    // ------------------------------------------------------------------------
    // free its children structures

    freez(st->exporting_flags);

    while(st->variables)  rrdsetvar_free(st->variables);
//  while(st->alarms)     rrdsetcalc_unlink(st->alarms);
    /* We must free all connected alarms here in case this has been an ephemeral chart whose alarm was
     * created by a template. This leads to an effective memory leak, which cannot be detected since the
     * alarms will still be connected to the host, and freed during shutdown. */
    while(st->alarms)     rrdcalc_unlink_and_free(st->rrdhost, st->alarms);
    while(st->dimensions) rrddim_free(st, st->dimensions);

    rrdfamily_free(host, st->rrdfamily);

    debug(D_RRD_CALLS, "RRDSET: Cleaning up remaining chart variables for host '%s', chart '%s'", host->hostname, st->id);
    rrdvar_free_remaining_variables(host, &st->rrdvar_root_index);

    // ------------------------------------------------------------------------
    // remove it from the configuration

    appconfig_section_destroy_non_loaded(&netdata_config, st->config_section);

    // ------------------------------------------------------------------------
    // unlink it from the host

    if(st == host->rrdset_root) {
        host->rrdset_root = st->next;
    }
    else {
        // find the previous one
        RRDSET *s;
        for(s = host->rrdset_root; s && s->next != st ; s = s->next) ;

        // bypass it
        if(s) s->next = st->next;
        else error("Request to free RRDSET '%s': cannot find it under host '%s'", st->id, host->hostname);
    }

    rrdset_unlock(st);

    // ------------------------------------------------------------------------
    // free it

    netdata_rwlock_destroy(&st->rrdset_rwlock);
    netdata_rwlock_destroy(&st->state->labels.labels_rwlock);

    // free directly allocated members
    freez(st->config_section);
    freez(st->plugin_name);
    freez(st->module_name);
    freez(st->state->old_title);
    freez(st->state->old_context);
    free_label_list(st->state->labels.head);
    freez(st->state);
    freez(st->chart_uuid);

    switch(st->rrd_memory_mode) {
        case RRD_MEMORY_MODE_SAVE:
        case RRD_MEMORY_MODE_MAP:
        case RRD_MEMORY_MODE_RAM:
            debug(D_RRD_CALLS, "Unmapping stats '%s'.", st->name);
            munmap(st, st->memsize);
            break;

        case RRD_MEMORY_MODE_ALLOC:
        case RRD_MEMORY_MODE_NONE:
        case RRD_MEMORY_MODE_DBENGINE:
            freez(st);
            break;
    }

}

void rrdset_save(RRDSET *st) {
    rrdset_check_rdlock(st);

    // info("Saving chart '%s' ('%s')", st->id, st->name);

    if(st->rrd_memory_mode == RRD_MEMORY_MODE_SAVE) {
        debug(D_RRD_STATS, "Saving stats '%s' to '%s'.", st->name, st->cache_filename);
        memory_file_save(st->cache_filename, st, st->memsize);
    }

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(likely(rd->rrd_memory_mode == RRD_MEMORY_MODE_SAVE)) {
            debug(D_RRD_STATS, "Saving dimension '%s' to '%s'.", rd->name, rd->cache_filename);
            memory_file_save(rd->cache_filename, rd, rd->memsize);
        }
    }
}

void rrdset_delete_custom(RRDSET *st, int db_rotated) {
    RRDDIM *rd;
#ifndef ENABLE_ACLK
    UNUSED(db_rotated);
#endif
    rrdset_check_rdlock(st);

    info("Deleting chart '%s' ('%s') from disk...", st->id, st->name);

    if(st->rrd_memory_mode == RRD_MEMORY_MODE_SAVE || st->rrd_memory_mode == RRD_MEMORY_MODE_MAP) {
        info("Deleting chart header file '%s'.", st->cache_filename);
        if(unlikely(unlink(st->cache_filename) == -1))
            error("Cannot delete chart header file '%s'", st->cache_filename);
    }

    rrddim_foreach_read(rd, st) {
        if(likely(rd->rrd_memory_mode == RRD_MEMORY_MODE_SAVE || rd->rrd_memory_mode == RRD_MEMORY_MODE_MAP)) {
            info("Deleting dimension file '%s'.", rd->cache_filename);
            if(unlikely(unlink(rd->cache_filename) == -1))
                error("Cannot delete dimension file '%s'", rd->cache_filename);
        }
    }

    recursively_delete_dir(st->cache_dir, "left-over chart");
#ifdef ENABLE_ACLK
    if ((netdata_cloud_setting) && (db_rotated || RRD_MEMORY_MODE_DBENGINE != st->rrd_memory_mode)) {
        aclk_del_collector(st->rrdhost, st->plugin_name, st->module_name);
        st->rrdhost->deleted_charts_count++;
    }
#endif

}

void rrdset_delete_obsolete_dimensions(RRDSET *st) {
    RRDDIM *rd;

    rrdset_check_rdlock(st);

    info("Deleting dimensions of chart '%s' ('%s') from disk...", st->id, st->name);

    rrddim_foreach_read(rd, st) {
        if(rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
            if(likely(rd->rrd_memory_mode == RRD_MEMORY_MODE_SAVE || rd->rrd_memory_mode == RRD_MEMORY_MODE_MAP)) {
                info("Deleting dimension file '%s'.", rd->cache_filename);
                if(unlikely(unlink(rd->cache_filename) == -1))
                    error("Cannot delete dimension file '%s'", rd->cache_filename);
            }
        }
    }
}

// ----------------------------------------------------------------------------
// RRDSET - create a chart

static inline RRDSET *rrdset_find_on_create(RRDHOST *host, const char *fullid) {
    RRDSET *st = rrdset_find(host, fullid);
    if(unlikely(st)) {
        rrdset_isnot_obsolete(st);
        debug(D_RRD_CALLS, "RRDSET '%s', already exists.", fullid);
        return st;
    }

    return NULL;
}

RRDSET *rrdset_create_custom(
          RRDHOST *host
        , const char *type
        , const char *id
        , const char *name
        , const char *family
        , const char *context
        , const char *title
        , const char *units
        , const char *plugin
        , const char *module
        , long priority
        , int update_every
        , RRDSET_TYPE chart_type
        , RRD_MEMORY_MODE memory_mode
        , long history_entries
) {
    if(!type || !type[0]) {
        fatal("Cannot create rrd stats without a type: id '%s', name '%s', family '%s', context '%s', title '%s', units '%s', plugin '%s', module '%s'."
              , (id && *id)?id:"<unset>"
              , (name && *name)?name:"<unset>"
              , (family && *family)?family:"<unset>"
              , (context && *context)?context:"<unset>"
              , (title && *title)?title:"<unset>"
              , (units && *units)?units:"<unset>"
              , (plugin && *plugin)?plugin:"<unset>"
              , (module && *module)?module:"<unset>"
        );
        return NULL;
    }

    if(!id || !id[0]) {
        fatal("Cannot create rrd stats without an id: type '%s', name '%s', family '%s', context '%s', title '%s', units '%s', plugin '%s', module '%s'."
              , type
              , (name && *name)?name:"<unset>"
              , (family && *family)?family:"<unset>"
              , (context && *context)?context:"<unset>"
              , (title && *title)?title:"<unset>"
              , (units && *units)?units:"<unset>"
              , (plugin && *plugin)?plugin:"<unset>"
              , (module && *module)?module:"<unset>"
        );
        return NULL;
    }

    // ------------------------------------------------------------------------
    // check if it already exists

    char fullid[RRD_ID_LENGTH_MAX + 1];
    snprintfz(fullid, RRD_ID_LENGTH_MAX, "%s.%s", type, id);

    int changed_from_archived_to_active = 0;
    RRDSET *st = rrdset_find_on_create(host, fullid);
    if (st) {
        int mark_rebuild = 0;
        rrdset_flag_set(st, RRDSET_FLAG_SYNC_CLOCK);
        rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);
        if (rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)) {
            rrdset_flag_clear(st, RRDSET_FLAG_ARCHIVED);
            changed_from_archived_to_active = 1;
            mark_rebuild |= META_CHART_ACTIVATED;
        }
        char *old_plugin = NULL, *old_module = NULL, *old_title = NULL, *old_context = NULL,
             *old_title_v = NULL, *old_context_v = NULL;
        int rc;

        if(unlikely(name))
            rc = rrdset_set_name(st, name);
        else
            rc = rrdset_set_name(st, id);

        if (rc == 2)
            mark_rebuild |= META_CHART_UPDATED;

        if (unlikely(st->priority != priority)) {
            st->priority = priority;
            mark_rebuild |= META_CHART_UPDATED;
        }
        if (unlikely(st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && st->update_every != update_every)) {
            st->update_every = update_every;
            mark_rebuild |= META_CHART_UPDATED;
        }

        if (plugin && st->plugin_name) {
            if (unlikely(strcmp(plugin, st->plugin_name))) {
                old_plugin = st->plugin_name;
                st->plugin_name = strdupz(plugin);
                mark_rebuild |= META_PLUGIN_UPDATED;
            }
        } else {
            if (plugin != st->plugin_name) { // one is NULL?
                old_plugin = st->plugin_name;
                st->plugin_name = plugin ? strdupz(plugin) : NULL;
                mark_rebuild |= META_PLUGIN_UPDATED;
            }
        }

        if (module && st->module_name) {
            if (unlikely(strcmp(module, st->module_name))) {
                old_module = st->module_name;
                st->module_name = strdupz(module);
                mark_rebuild |= META_MODULE_UPDATED;
            }
        } else {
            if (module != st->module_name) {
                if (st->module_name && *st->module_name) {
                    old_module = st->module_name;
                    st->module_name = module ? strdupz(module) : NULL;
                    mark_rebuild |= META_MODULE_UPDATED;
                }
            }
        }

        if (unlikely(title && st->state->old_title && strcmp(st->state->old_title, title))) {
            char *new_title = strdupz(title);
            old_title_v = st->state->old_title;
            st->state->old_title = strdupz(title);
            json_fix_string(new_title);
            old_title = st->title;
            st->title = new_title;
            mark_rebuild |= META_CHART_UPDATED;
        }

        RRDSET_TYPE new_chart_type =
            rrdset_type_id(config_get(st->config_section, "chart type", rrdset_type_name(chart_type)));
        if (st->chart_type != new_chart_type) {
            st->chart_type = new_chart_type;
            mark_rebuild |= META_CHART_UPDATED;
        }

        if (unlikely(context && st->state->old_context && strcmp(st->state->old_context, context))) {
            char *new_context = strdupz(context);
            old_context_v = st->state->old_context;
            st->state->old_context = strdupz(context);
            json_fix_string(new_context);
            old_context = st->context;
            st->context = new_context;
            st->hash_context = simple_hash(st->context);
            mark_rebuild |= META_CHART_UPDATED;
        }

        if (mark_rebuild) {
#ifdef ENABLE_ACLK
            if (netdata_cloud_setting) {
                if (mark_rebuild & META_CHART_ACTIVATED) {
                    aclk_add_collector(host, st->plugin_name, st->module_name);
                }
                else {
                    if (mark_rebuild & (META_PLUGIN_UPDATED | META_MODULE_UPDATED)) {
                        aclk_del_collector(
                            host, mark_rebuild & META_PLUGIN_UPDATED ? old_plugin : st->plugin_name,
                            mark_rebuild & META_MODULE_UPDATED ? old_module : st->module_name);
                        aclk_add_collector(host, st->plugin_name, st->module_name);
                    }
                }
                rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
            }
#endif
            freez(old_plugin);
            freez(old_module);
            freez(old_title);
            freez(old_context);
            freez(old_title_v);
            freez(old_context_v);
            if (mark_rebuild != META_CHART_ACTIVATED) {
                info("Collector updated metadata for chart %s", st->id);
                sched_yield();
            }
        }
        if (mark_rebuild & (META_CHART_UPDATED | META_PLUGIN_UPDATED | META_MODULE_UPDATED)) {
            debug(D_METADATALOG, "CHART [%s] metadata updated", st->id);
            int rc = update_chart_metadata(st->chart_uuid, st, id, name);
            if (unlikely(rc))
                error_report("Failed to update chart metadata in the database");
        }
        /* Fall-through during switch from archived to active so that the host lock is taken and health is linked */
        if (!changed_from_archived_to_active)
            return st;
    }

    rrdhost_wrlock(host);

    st = rrdset_find_on_create(host, fullid);
    if(st) {
        if (changed_from_archived_to_active) {
            rrdset_flag_clear(st, RRDSET_FLAG_ARCHIVED);
            rrdsetvar_create(st, "last_collected_t",    RRDVAR_TYPE_TIME_T,     &st->last_collected_time.tv_sec, RRDVAR_OPTION_DEFAULT);
            rrdsetvar_create(st, "collected_total_raw", RRDVAR_TYPE_TOTAL,      &st->last_collected_total,       RRDVAR_OPTION_DEFAULT);
            rrdsetvar_create(st, "green",               RRDVAR_TYPE_CALCULATED, &st->green,                      RRDVAR_OPTION_DEFAULT);
            rrdsetvar_create(st, "red",                 RRDVAR_TYPE_CALCULATED, &st->red,                        RRDVAR_OPTION_DEFAULT);
            rrdsetvar_create(st, "update_every",        RRDVAR_TYPE_INT,        &st->update_every,               RRDVAR_OPTION_DEFAULT);
            rrdsetcalc_link_matching(st);
            rrdcalctemplate_link_matching(st);
        }
        rrdhost_unlock(host);
        rrdset_flag_set(st, RRDSET_FLAG_SYNC_CLOCK);
        rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);
        return st;
    }

    char fullfilename[FILENAME_MAX + 1];

    // ------------------------------------------------------------------------
    // compose the config_section for this chart

    char config_section[RRD_ID_LENGTH_MAX + GUID_LEN + 2];
    if(host == localhost)
        strcpy(config_section, fullid);
    else
        snprintfz(config_section, RRD_ID_LENGTH_MAX + GUID_LEN + 1, "%s/%s", host->machine_guid, fullid);

    // ------------------------------------------------------------------------
    // get the options from the config, we need to create it

    long entries;
    if(memory_mode == RRD_MEMORY_MODE_DBENGINE) {
        // only sets it the first time
        entries = config_get_number(config_section, "history", 5);
    } else {
        long rentries = config_get_number(config_section, "history", history_entries);
        entries = align_entries_to_pagesize(memory_mode, rentries);
        if (entries != rentries) entries = config_set_number(config_section, "history", entries);

        if (memory_mode == RRD_MEMORY_MODE_NONE && entries != rentries)
            entries = config_set_number(config_section, "history", 10);
    }
    int enabled = config_get_boolean(config_section, "enabled", 1);
    if(!enabled) entries = 5;

    unsigned long size = sizeof(RRDSET);
    char *cache_dir = rrdset_cache_dir(host, fullid, config_section);

    time_t now = now_realtime_sec();

    // ------------------------------------------------------------------------
    // load it or allocate it

    debug(D_RRD_CALLS, "Creating RRD_STATS for '%s.%s'.", type, id);

    snprintfz(fullfilename, FILENAME_MAX, "%s/main.db", cache_dir);
    if(memory_mode == RRD_MEMORY_MODE_SAVE || memory_mode == RRD_MEMORY_MODE_MAP ||
       memory_mode == RRD_MEMORY_MODE_RAM) {
        st = (RRDSET *) mymmap(
                  (memory_mode == RRD_MEMORY_MODE_RAM) ? NULL : fullfilename
                , size
                , ((memory_mode == RRD_MEMORY_MODE_MAP) ? MAP_SHARED : MAP_PRIVATE)
                , 0
        );

        if(st) {
            memset(&st->avl, 0, sizeof(avl_t));
            memset(&st->avlname, 0, sizeof(avl_t));
            memset(&st->rrdvar_root_index, 0, sizeof(avl_tree_lock));
            memset(&st->dimensions_index, 0, sizeof(avl_tree_lock));
            memset(&st->rrdset_rwlock, 0, sizeof(netdata_rwlock_t));

            st->name = NULL;
            st->config_section = NULL;
            st->type = NULL;
            st->family = NULL;
            st->title = NULL;
            st->units = NULL;
            st->context = NULL;
            st->cache_dir = NULL;
            st->plugin_name = NULL;
            st->module_name = NULL;
            st->dimensions = NULL;
            st->rrdfamily = NULL;
            st->rrdhost = NULL;
            st->next = NULL;
            st->variables = NULL;
            st->alarms = NULL;
            st->flags = 0x00000000;
            st->exporting_flags = NULL;

            if(memory_mode == RRD_MEMORY_MODE_RAM) {
                memset(st, 0, size);
            }
            else {
                if(strcmp(st->magic, RRDSET_MAGIC) != 0) {
                    info("Initializing file %s.", fullfilename);
                    memset(st, 0, size);
                }
                else if(strcmp(st->id, fullid) != 0) {
                    error("File %s contents are not for chart %s. Clearing it.", fullfilename, fullid);
                    // munmap(st, size);
                    // st = NULL;
                    memset(st, 0, size);
                }
                else if(st->memsize != size || st->entries != entries) {
                    error("File %s does not have the desired size. Clearing it.", fullfilename);
                    memset(st, 0, size);
                }
                else if(st->update_every != update_every) {
                    error("File %s does not have the desired update frequency. Clearing it.", fullfilename);
                    memset(st, 0, size);
                }
                else if((now - st->last_updated.tv_sec) > update_every * entries) {
                    info("File %s is too old. Clearing it.", fullfilename);
                    memset(st, 0, size);
                }
                else if(st->last_updated.tv_sec > now + update_every) {
                    error("File %s refers to the future by %zd secs. Resetting it to now.", fullfilename, (ssize_t)(st->last_updated.tv_sec - now));
                    st->last_updated.tv_sec = now;
                }

                // make sure the database is aligned
                if(st->last_updated.tv_sec) {
                    st->update_every = update_every;
                    last_updated_time_align(st);
                }
            }

            // make sure we have the right memory mode
            // even if we cleared the memory
            st->rrd_memory_mode = memory_mode;
        }
    }

    if(unlikely(!st)) {
        st = callocz(1, size);
        if (memory_mode == RRD_MEMORY_MODE_DBENGINE)
            st->rrd_memory_mode = RRD_MEMORY_MODE_DBENGINE;
        else
            st->rrd_memory_mode = (memory_mode == RRD_MEMORY_MODE_NONE) ? RRD_MEMORY_MODE_NONE : RRD_MEMORY_MODE_ALLOC;
    }

    st->plugin_name = plugin?strdupz(plugin):NULL;
    st->module_name = module?strdupz(module):NULL;

    st->config_section = strdupz(config_section);
    st->rrdhost = host;
    st->memsize = size;
    st->entries = entries;
    st->update_every = update_every;

    if(st->current_entry >= st->entries) st->current_entry = 0;

    strcpy(st->cache_filename, fullfilename);
    strcpy(st->magic, RRDSET_MAGIC);

    strcpy(st->id, fullid);
    st->hash = simple_hash(st->id);

    st->cache_dir = cache_dir;

    st->chart_type = rrdset_type_id(config_get(st->config_section, "chart type", rrdset_type_name(chart_type)));
    st->type       = config_get(st->config_section, "type", type);

    st->state = callocz(1, sizeof(*st->state));
    st->family     = config_get(st->config_section, "family", family?family:st->type);
    json_fix_string(st->family);

    st->units      = config_get(st->config_section, "units", units?units:"");
    json_fix_string(st->units);

    st->context    = config_get(st->config_section, "context", context?context:st->id);
    st->state->old_context = strdupz(st->context);
    json_fix_string(st->context);
    st->hash_context = simple_hash(st->context);

    st->priority = config_get_number(st->config_section, "priority", priority);
    if(enabled)
        rrdset_flag_set(st, RRDSET_FLAG_ENABLED);
    else
        rrdset_flag_clear(st, RRDSET_FLAG_ENABLED);

    rrdset_flag_clear(st, RRDSET_FLAG_DETAIL);
    rrdset_flag_clear(st, RRDSET_FLAG_DEBUG);
    rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE);
    rrdset_flag_clear(st, RRDSET_FLAG_EXPORTING_SEND);
    rrdset_flag_clear(st, RRDSET_FLAG_EXPORTING_IGNORE);
    rrdset_flag_clear(st, RRDSET_FLAG_BACKEND_SEND);
    rrdset_flag_clear(st, RRDSET_FLAG_BACKEND_IGNORE);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_SEND);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_IGNORE);
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_EXPOSED);
    rrdset_flag_set(st, RRDSET_FLAG_SYNC_CLOCK);

    // if(!strcmp(st->id, "disk_util.dm-0")) {
    //     st->debug = 1;
    //     error("enabled debugging for '%s'", st->id);
    // }
    // else error("not enabled debugging for '%s'", st->id);

    st->green = NAN;
    st->red = NAN;

    st->last_collected_time.tv_sec = 0;
    st->last_collected_time.tv_usec = 0;
    st->counter_done = 0;
    st->rrddim_page_alignment = 0;

    st->gap_when_lost_iterations_above = (int) (gap_when_lost_iterations_above + 2);

    st->last_accessed_time = 0;
    st->upstream_resync_time = 0;

    avl_init_lock(&st->dimensions_index, rrddim_compare);
    avl_init_lock(&st->rrdvar_root_index, rrdvar_compare);

    netdata_rwlock_init(&st->rrdset_rwlock);
    netdata_rwlock_init(&st->state->labels.labels_rwlock);

    if(name && *name && rrdset_set_name(st, name))
        // we did set the name
        ;
    else
        // could not use the name, use the id
        rrdset_set_name(st, id);

    st->title = config_get(st->config_section, "title", title);
    st->state->old_title = strdupz(st->title);
    json_fix_string(st->title);

    st->rrdfamily = rrdfamily_create(host, st->family);

    st->next = host->rrdset_root;
    host->rrdset_root = st;

    if(host->health_enabled) {
        rrdsetvar_create(st, "last_collected_t",    RRDVAR_TYPE_TIME_T,     &st->last_collected_time.tv_sec, RRDVAR_OPTION_DEFAULT);
        rrdsetvar_create(st, "collected_total_raw", RRDVAR_TYPE_TOTAL,      &st->last_collected_total,       RRDVAR_OPTION_DEFAULT);
        rrdsetvar_create(st, "green",               RRDVAR_TYPE_CALCULATED, &st->green,                      RRDVAR_OPTION_DEFAULT);
        rrdsetvar_create(st, "red",                 RRDVAR_TYPE_CALCULATED, &st->red,                        RRDVAR_OPTION_DEFAULT);
        rrdsetvar_create(st, "update_every",        RRDVAR_TYPE_INT,        &st->update_every,               RRDVAR_OPTION_DEFAULT);
    }

    if(unlikely(rrdset_index_add(host, st) != st))
        error("RRDSET: INTERNAL ERROR: attempt to index duplicate chart '%s'", st->id);

    rrdsetcalc_link_matching(st);
    rrdcalctemplate_link_matching(st);

    st->chart_uuid = find_chart_uuid(host, type, id, name);
    if (unlikely(!st->chart_uuid))
        st->chart_uuid = create_chart_uuid(st, id, name);
    else
        update_chart_metadata(st->chart_uuid, st, id, name);

    store_active_chart(st->chart_uuid);
    compute_chart_hash(st);

    rrdhost_unlock(host);
#ifdef ENABLE_ACLK
    if (netdata_cloud_setting)
        aclk_add_collector(host, plugin, module);
    rrdset_flag_clear(st, RRDSET_FLAG_ACLK);
#endif
    return(st);
}


// ----------------------------------------------------------------------------
// RRDSET - data collection iteration control

inline void rrdset_next_usec_unfiltered(RRDSET *st, usec_t microseconds) {
    if(unlikely(!st->last_collected_time.tv_sec || !microseconds || (rrdset_flag_check_noatomic(st, RRDSET_FLAG_SYNC_CLOCK)))) {
        // call the full next_usec() function
        rrdset_next_usec(st, microseconds);
        return;
    }

    st->usec_since_last_update = microseconds;
}

inline void rrdset_next_usec(RRDSET *st, usec_t microseconds) {
    struct timeval now;
    now_realtime_timeval(&now);

    #ifdef NETDATA_INTERNAL_CHECKS
    char *discard_reason = NULL;
    usec_t discarded = microseconds;
    #endif

    if(unlikely(rrdset_flag_check_noatomic(st, RRDSET_FLAG_SYNC_CLOCK))) {
        // the chart needs to be re-synced to current time
        rrdset_flag_clear(st, RRDSET_FLAG_SYNC_CLOCK);

        // discard the microseconds supplied
        microseconds = 0;

        #ifdef NETDATA_INTERNAL_CHECKS
        if(!discard_reason) discard_reason = "SYNC CLOCK FLAG";
        #endif
    }

    if(unlikely(!st->last_collected_time.tv_sec)) {
        // the first entry
        microseconds = st->update_every * USEC_PER_SEC;
        #ifdef NETDATA_INTERNAL_CHECKS
        if(!discard_reason) discard_reason = "FIRST DATA COLLECTION";
        #endif
    }
    else if(unlikely(!microseconds)) {
        // no dt given by the plugin
        microseconds = dt_usec(&now, &st->last_collected_time);
        #ifdef NETDATA_INTERNAL_CHECKS
        if(!discard_reason) discard_reason = "NO USEC GIVEN BY COLLECTOR";
        #endif
    }
    else {
        // microseconds has the time since the last collection
        susec_t since_last_usec = dt_usec_signed(&now, &st->last_collected_time);

        if(unlikely(since_last_usec < 0)) {
            // oops! the database is in the future
            info("RRD database for chart '%s' on host '%s' is %0.5" LONG_DOUBLE_MODIFIER " secs in the future (counter #%zu, update #%zu). Adjusting it to current time.", st->id, st->rrdhost->hostname, (LONG_DOUBLE)-since_last_usec / USEC_PER_SEC, st->counter, st->counter_done);

            st->last_collected_time.tv_sec  = now.tv_sec - st->update_every;
            st->last_collected_time.tv_usec = now.tv_usec;
            last_collected_time_align(st);

            st->last_updated.tv_sec  = now.tv_sec - st->update_every;
            st->last_updated.tv_usec = now.tv_usec;
            last_updated_time_align(st);

            microseconds    = st->update_every * USEC_PER_SEC;
            #ifdef NETDATA_INTERNAL_CHECKS
            if(!discard_reason) discard_reason = "COLLECTION TIME IN FUTURE";
            #endif
        }
        else if(unlikely((usec_t)since_last_usec > (usec_t)(st->update_every * 5 * USEC_PER_SEC))) {
            // oops! the database is too far behind
            info("RRD database for chart '%s' on host '%s' is %0.5" LONG_DOUBLE_MODIFIER " secs in the past (counter #%zu, update #%zu). Adjusting it to current time.", st->id, st->rrdhost->hostname, (LONG_DOUBLE)since_last_usec / USEC_PER_SEC, st->counter, st->counter_done);

            microseconds = (usec_t)since_last_usec;
            #ifdef NETDATA_INTERNAL_CHECKS
            if(!discard_reason) discard_reason = "COLLECTION TIME TOO FAR IN THE PAST";
            #endif
        }

#ifdef NETDATA_INTERNAL_CHECKS
        if(since_last_usec > 0 && (susec_t)microseconds < since_last_usec) {
            static __thread susec_t min_delta = USEC_PER_SEC * 3600, permanent_min_delta = 0;
            static __thread time_t last_t = 0;

            // the first time initialize it so that it will make the check later
            if(last_t == 0) last_t = now.tv_sec + 60;

            susec_t delta = since_last_usec - (susec_t)microseconds;
            if(delta < min_delta) min_delta = delta;

            if(now.tv_sec >= last_t + 60) {
                last_t = now.tv_sec;

                if(min_delta > permanent_min_delta) {
                    info("MINIMUM MICROSECONDS DELTA of thread %d increased from %lld to %lld (+%lld)", gettid(), permanent_min_delta, min_delta, min_delta - permanent_min_delta);
                    permanent_min_delta = min_delta;
                }

                min_delta = USEC_PER_SEC * 3600;
            }
        }
#endif
    }

    #ifdef NETDATA_INTERNAL_CHECKS
    debug(D_RRD_CALLS, "rrdset_next_usec() for chart %s with microseconds %llu", st->name, microseconds);
    rrdset_debug(st, "NEXT: %llu microseconds", microseconds);

    if(discarded && discarded != microseconds)
        info("host '%s', chart '%s': discarded data collection time of %llu usec, replaced with %llu usec, reason: '%s'", st->rrdhost->hostname, st->id, discarded, microseconds, discard_reason?discard_reason:"UNDEFINED");

    #endif

    st->usec_since_last_update = microseconds;
}


// ----------------------------------------------------------------------------
// RRDSET - process the collected values for all dimensions of a chart

static inline usec_t rrdset_init_last_collected_time(RRDSET *st) {
    now_realtime_timeval(&st->last_collected_time);
    last_collected_time_align(st);

    usec_t last_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;

    #ifdef NETDATA_INTERNAL_CHECKS
    rrdset_debug(st, "initialized last collected time to %0.3" LONG_DOUBLE_MODIFIER, (LONG_DOUBLE)last_collect_ut / USEC_PER_SEC);
    #endif

    return last_collect_ut;
}

static inline usec_t rrdset_update_last_collected_time(RRDSET *st) {
    usec_t last_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;
    usec_t ut = last_collect_ut + st->usec_since_last_update;
    st->last_collected_time.tv_sec = (time_t) (ut / USEC_PER_SEC);
    st->last_collected_time.tv_usec = (suseconds_t) (ut % USEC_PER_SEC);

    #ifdef NETDATA_INTERNAL_CHECKS
    rrdset_debug(st, "updated last collected time to %0.3" LONG_DOUBLE_MODIFIER, (LONG_DOUBLE)last_collect_ut / USEC_PER_SEC);
    #endif

    return last_collect_ut;
}

static inline usec_t rrdset_init_last_updated_time(RRDSET *st) {
    // copy the last collected time to last updated time
    st->last_updated.tv_sec  = st->last_collected_time.tv_sec;
    st->last_updated.tv_usec = st->last_collected_time.tv_usec;

    if(rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST))
        st->last_updated.tv_sec -= st->update_every;

    last_updated_time_align(st);

    usec_t last_updated_ut = st->last_updated.tv_sec * USEC_PER_SEC + st->last_updated.tv_usec;

    #ifdef NETDATA_INTERNAL_CHECKS
    rrdset_debug(st, "initialized last updated time to %0.3" LONG_DOUBLE_MODIFIER, (LONG_DOUBLE)last_updated_ut / USEC_PER_SEC);
    #endif

    return last_updated_ut;
}

static inline size_t rrdset_done_interpolate(
        RRDSET *st
        , usec_t update_every_ut
        , usec_t last_stored_ut
        , usec_t next_store_ut
        , usec_t last_collect_ut
        , usec_t now_collect_ut
        , char store_this_entry
        , uint32_t storage_flags
) {
    RRDDIM *rd;

    size_t stored_entries = 0;     // the number of entries we have stored in the db, during this call to rrdset_done()

    usec_t first_ut = last_stored_ut, last_ut = 0;
    (void)first_ut;

    ssize_t iterations = (ssize_t)((now_collect_ut - last_stored_ut) / (update_every_ut));
    if((now_collect_ut % (update_every_ut)) == 0) iterations++;

    size_t counter = st->counter;
    long current_entry = st->current_entry;

    for( ; next_store_ut <= now_collect_ut ; last_collect_ut = next_store_ut, next_store_ut += update_every_ut, iterations-- ) {

        #ifdef NETDATA_INTERNAL_CHECKS
        if(iterations < 0) { error("INTERNAL CHECK: %s: iterations calculation wrapped! first_ut = %llu, last_stored_ut = %llu, next_store_ut = %llu, now_collect_ut = %llu", st->name, first_ut, last_stored_ut, next_store_ut, now_collect_ut); }
        rrdset_debug(st, "last_stored_ut = %0.3" LONG_DOUBLE_MODIFIER " (last updated time)", (LONG_DOUBLE)last_stored_ut/USEC_PER_SEC);
        rrdset_debug(st, "next_store_ut  = %0.3" LONG_DOUBLE_MODIFIER " (next interpolation point)", (LONG_DOUBLE)next_store_ut/USEC_PER_SEC);
        #endif

        last_ut = next_store_ut;

        rrddim_foreach_read(rd, st) {
            if (rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED))
                continue;

            calculated_number new_value;

            switch(rd->algorithm) {
                case RRD_ALGORITHM_INCREMENTAL:
                    new_value = (calculated_number)
                            (      rd->calculated_value
                                   * (calculated_number)(next_store_ut - last_collect_ut)
                                   / (calculated_number)(now_collect_ut - last_collect_ut)
                            );

                    #ifdef NETDATA_INTERNAL_CHECKS
                    rrdset_debug(st, "%s: CALC2 INC "
                                CALCULATED_NUMBER_FORMAT " = "
                                CALCULATED_NUMBER_FORMAT
                                " * (%llu - %llu)"
                                " / (%llu - %llu)"
                              , rd->name
                              , new_value
                              , rd->calculated_value
                              , next_store_ut, last_collect_ut
                              , now_collect_ut, last_collect_ut
                    );
                    #endif

                    rd->calculated_value -= new_value;
                    new_value += rd->last_calculated_value;
                    rd->last_calculated_value = 0;
                    new_value /= (calculated_number)st->update_every;

                    if(unlikely(next_store_ut - last_stored_ut < update_every_ut)) {

                        #ifdef NETDATA_INTERNAL_CHECKS
                        rrdset_debug(st, "%s: COLLECTION POINT IS SHORT " CALCULATED_NUMBER_FORMAT " - EXTRAPOLATING",
                                    rd->name
                                  , (calculated_number)(next_store_ut - last_stored_ut)
                        );
                        #endif

                        new_value = new_value * (calculated_number)(st->update_every * USEC_PER_SEC) / (calculated_number)(next_store_ut - last_stored_ut);
                    }
                    break;

                case RRD_ALGORITHM_ABSOLUTE:
                case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
                case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
                default:
                    if(iterations == 1) {
                        // this is the last iteration
                        // do not interpolate
                        // just show the calculated value

                        new_value = rd->calculated_value;
                    }
                    else {
                        // we have missed an update
                        // interpolate in the middle values

                        new_value = (calculated_number)
                                (   (     (rd->calculated_value - rd->last_calculated_value)
                                          * (calculated_number)(next_store_ut - last_collect_ut)
                                          / (calculated_number)(now_collect_ut - last_collect_ut)
                                    )
                                    +  rd->last_calculated_value
                                );

                        #ifdef NETDATA_INTERNAL_CHECKS
                        rrdset_debug(st, "%s: CALC2 DEF "
                                    CALCULATED_NUMBER_FORMAT " = ((("
                                            "(" CALCULATED_NUMBER_FORMAT " - " CALCULATED_NUMBER_FORMAT ")"
                                            " * %llu"
                                            " / %llu) + " CALCULATED_NUMBER_FORMAT
                                  , rd->name
                                  , new_value
                                  , rd->calculated_value, rd->last_calculated_value
                                  , (next_store_ut - first_ut)
                                  , (now_collect_ut - first_ut), rd->last_calculated_value
                        );
                        #endif
                    }
                    break;
            }

            if(unlikely(!store_this_entry)) {
                rd->state->collect_ops.store_metric(rd, next_store_ut, SN_EMPTY_SLOT); //pack_storage_number(0, SN_NOT_EXISTS)
//                rd->values[current_entry] = SN_EMPTY_SLOT; //pack_storage_number(0, SN_NOT_EXISTS);
                continue;
            }

            if(likely(rd->updated && rd->collections_counter > 1 && iterations < st->gap_when_lost_iterations_above)) {
                rd->state->collect_ops.store_metric(rd, next_store_ut, pack_storage_number(new_value, storage_flags));
//                rd->values[current_entry] = pack_storage_number(new_value, storage_flags );
                rd->last_stored_value = new_value;

                #ifdef NETDATA_INTERNAL_CHECKS
                rrdset_debug(st, "%s: STORE[%ld] "
                            CALCULATED_NUMBER_FORMAT " = " CALCULATED_NUMBER_FORMAT
                          , rd->name
                          , current_entry
                          , unpack_storage_number(rd->values[current_entry]), new_value
                );
                #endif

            }
            else {

                #ifdef NETDATA_INTERNAL_CHECKS
                rrdset_debug(st, "%s: STORE[%ld] = NON EXISTING "
                          , rd->name
                          , current_entry
                );
                #endif

//                rd->values[current_entry] = SN_EMPTY_SLOT; // pack_storage_number(0, SN_NOT_EXISTS);
                rd->state->collect_ops.store_metric(rd, next_store_ut, SN_EMPTY_SLOT); //pack_storage_number(0, SN_NOT_EXISTS)
                rd->last_stored_value = NAN;
            }

            stored_entries++;

            #ifdef NETDATA_INTERNAL_CHECKS
            if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_DEBUG))) {
                calculated_number t1 = new_value * (calculated_number)rd->multiplier / (calculated_number)rd->divisor;
                calculated_number t2 = unpack_storage_number(rd->values[current_entry]);

                calculated_number accuracy = accuracy_loss(t1, t2);
                debug(D_RRD_STATS, "%s/%s: UNPACK[%ld] = " CALCULATED_NUMBER_FORMAT " FLAGS=0x%08x (original = " CALCULATED_NUMBER_FORMAT ", accuracy loss = " CALCULATED_NUMBER_FORMAT "%%%s)"
                      , st->id, rd->name
                      , current_entry
                      , t2
                      , get_storage_number_flags(rd->values[current_entry])
                      , t1
                      , accuracy
                      , (accuracy > ACCURACY_LOSS_ACCEPTED_PERCENT) ? " **TOO BIG** " : ""
                );

                rd->collected_volume += t1;
                rd->stored_volume += t2;

                accuracy = accuracy_loss(rd->collected_volume, rd->stored_volume);
                debug(D_RRD_STATS, "%s/%s: VOLUME[%ld] = " CALCULATED_NUMBER_FORMAT ", calculated  = " CALCULATED_NUMBER_FORMAT ", accuracy loss = " CALCULATED_NUMBER_FORMAT "%%%s"
                      , st->id, rd->name
                      , current_entry
                      , rd->stored_volume
                      , rd->collected_volume
                      , accuracy
                      , (accuracy > ACCURACY_LOSS_ACCEPTED_PERCENT) ? " **TOO BIG** " : ""
                );
            }
            #endif
        }
        // reset the storage flags for the next point, if any;
        storage_flags = SN_EXISTS;

        st->counter = ++counter;
        st->current_entry = current_entry = ((current_entry + 1) >= st->entries) ? 0 : current_entry + 1;

        st->last_updated.tv_sec = (time_t) (last_ut / USEC_PER_SEC);
        st->last_updated.tv_usec = 0;

        last_stored_ut = next_store_ut;
    }

/*
    st->counter = counter;
    st->current_entry = current_entry;

    if(likely(last_ut)) {
        st->last_updated.tv_sec = (time_t) (last_ut / USEC_PER_SEC);
        st->last_updated.tv_usec = 0;
    }
*/

    return stored_entries;
}

static inline void rrdset_done_fill_the_gap(RRDSET *st) {
    usec_t update_every_ut = st->update_every * USEC_PER_SEC;
    usec_t now_collect_ut  = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;

    long c = 0, entries = st->entries;
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        usec_t next_store_ut = (st->last_updated.tv_sec + st->update_every) * USEC_PER_SEC;
        long current_entry = st->current_entry;

        for(c = 0; c < entries && next_store_ut <= now_collect_ut ; next_store_ut += update_every_ut, c++) {
            rd->values[current_entry] = SN_EMPTY_SLOT;
            current_entry = ((current_entry + 1) >= entries) ? 0 : current_entry + 1;

            #ifdef NETDATA_INTERNAL_CHECKS
            rrdset_debug(st, "%s: STORE[%ld] = NON EXISTING (FILLED THE GAP)", rd->name, current_entry);
            #endif
        }
    }

    if(c > 0) {
        c--;
        st->last_updated.tv_sec += c * st->update_every;

        st->current_entry += c;
        if(st->current_entry >= st->entries)
            st->current_entry -= st->entries;
    }
}

void rrdset_done(RRDSET *st) {
    if(unlikely(netdata_exit)) return;

    debug(D_RRD_CALLS, "rrdset_done() for chart %s", st->name);

    RRDDIM *rd;

    char
            store_this_entry = 1,   // boolean: 1 = store this entry, 0 = don't store this entry
            first_entry = 0;        // boolean: 1 = this is the first entry seen for this chart, 0 = all other entries

    usec_t
            last_collect_ut = 0,    // the timestamp in microseconds, of the last collected value
            now_collect_ut = 0,     // the timestamp in microseconds, of this collected value (this is NOW)
            last_stored_ut = 0,     // the timestamp in microseconds, of the last stored entry in the db
            next_store_ut = 0,      // the timestamp in microseconds, of the next entry to store in the db
            update_every_ut = st->update_every * USEC_PER_SEC; // st->update_every in microseconds

    netdata_thread_disable_cancelability();

    // a read lock is OK here
    rrdset_rdlock(st);

#ifdef ENABLE_ACLK
    #ifdef ENABLE_NEW_CLOUD_PROTOCOL
    if (unlikely(!rrdset_flag_check(st, RRDSET_FLAG_ACLK))) {
        if (st->counter_done >= RRDSET_MINIMUM_LIVE_COUNT) {
            if (likely(!sql_queue_chart_to_aclk(st)))
                rrdset_flag_set(st, RRDSET_FLAG_ACLK);
        }
    }
    #else
    if (unlikely(!rrdset_flag_check(st, RRDSET_FLAG_ACLK))) {
        rrdset_flag_set(st, RRDSET_FLAG_ACLK);
        aclk_update_chart(st->rrdhost, st->id, 1);
    }
    #endif
#endif

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE))) {
        error("Chart '%s' has the OBSOLETE flag set, but it is collected.", st->id);
        rrdset_isnot_obsolete(st);
    }

    // check if the chart has a long time to be updated
    if(unlikely(st->usec_since_last_update > st->entries * update_every_ut &&
                st->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE && st->rrd_memory_mode != RRD_MEMORY_MODE_NONE)) {
        info("host '%s', chart %s: took too long to be updated (counter #%zu, update #%zu, %0.3" LONG_DOUBLE_MODIFIER " secs). Resetting it.", st->rrdhost->hostname, st->name, st->counter, st->counter_done, (LONG_DOUBLE)st->usec_since_last_update / USEC_PER_SEC);
        rrdset_reset(st);
        st->usec_since_last_update = update_every_ut;
        store_this_entry = 0;
        first_entry = 1;
    }

    #ifdef NETDATA_INTERNAL_CHECKS
    rrdset_debug(st, "microseconds since last update: %llu", st->usec_since_last_update);
    #endif

    // set last_collected_time
    if(unlikely(!st->last_collected_time.tv_sec)) {
        // it is the first entry
        // set the last_collected_time to now
        last_collect_ut = rrdset_init_last_collected_time(st) - update_every_ut;

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }
    else {
        // it is not the first entry
        // calculate the proper last_collected_time, using usec_since_last_update
        last_collect_ut = rrdset_update_last_collected_time(st);
    }
    if (unlikely(st->rrd_memory_mode == RRD_MEMORY_MODE_NONE)) {
        goto after_first_database_work;
    }

    // if this set has not been updated in the past
    // we fake the last_update time to be = now - usec_since_last_update
    if(unlikely(!st->last_updated.tv_sec)) {
        // it has never been updated before
        // set a fake last_updated, in the past using usec_since_last_update
        rrdset_init_last_updated_time(st);

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }

    // check if we will re-write the entire data set
    if(unlikely(dt_usec(&st->last_collected_time, &st->last_updated) > st->entries * update_every_ut &&
                st->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE)) {
        info("%s: too old data (last updated at %ld.%ld, last collected at %ld.%ld). Resetting it. Will not store the next entry.", st->name, st->last_updated.tv_sec, st->last_updated.tv_usec, st->last_collected_time.tv_sec, st->last_collected_time.tv_usec);
        rrdset_reset(st);
        rrdset_init_last_updated_time(st);

        st->usec_since_last_update = update_every_ut;

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }

#ifdef ENABLE_DBENGINE
    // check if we will re-write the entire page
    if(unlikely(st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE &&
                dt_usec(&st->last_collected_time, &st->last_updated) > (RRDENG_BLOCK_SIZE / sizeof(storage_number)) * update_every_ut)) {
        info("%s: too old data (last updated at %ld.%ld, last collected at %ld.%ld). Resetting it. Will not store the next entry.", st->name, st->last_updated.tv_sec, st->last_updated.tv_usec, st->last_collected_time.tv_sec, st->last_collected_time.tv_usec);
        rrdset_reset(st);
        rrdset_init_last_updated_time(st);

        st->usec_since_last_update = update_every_ut;

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }
#endif

    // these are the 3 variables that will help us in interpolation
    // last_stored_ut = the last time we added a value to the storage
    // now_collect_ut = the time the current value has been collected
    // next_store_ut  = the time of the next interpolation point
    now_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;
    last_stored_ut = st->last_updated.tv_sec * USEC_PER_SEC + st->last_updated.tv_usec;
    next_store_ut  = (st->last_updated.tv_sec + st->update_every) * USEC_PER_SEC;

    if(unlikely(!st->counter_done)) {
        // if we have not collected metrics this session (st->counter_done == 0)
        // and we have collected metrics for this chart in the past (st->counter != 0)
        // fill the gap (the chart has been just loaded from disk)
        if(unlikely(st->counter) && st->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE) {
            rrdset_done_fill_the_gap(st);
            last_stored_ut = st->last_updated.tv_sec * USEC_PER_SEC + st->last_updated.tv_usec;
            next_store_ut  = (st->last_updated.tv_sec + st->update_every) * USEC_PER_SEC;
        }
        if (st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
            // set a fake last_updated to jump to current time
            rrdset_init_last_updated_time(st);
            last_stored_ut = st->last_updated.tv_sec * USEC_PER_SEC + st->last_updated.tv_usec;
            next_store_ut  = (st->last_updated.tv_sec + st->update_every) * USEC_PER_SEC;
        }

        if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST))) {
            store_this_entry = 1;
            last_collect_ut = next_store_ut - update_every_ut;

            #ifdef NETDATA_INTERNAL_CHECKS
            rrdset_debug(st, "Fixed first entry.");
            #endif
        }
        else {
            store_this_entry = 0;

            #ifdef NETDATA_INTERNAL_CHECKS
            rrdset_debug(st, "Will not store the next entry.");
            #endif
        }
    }
after_first_database_work:
    st->counter_done++;

    if(unlikely(st->rrdhost->rrdpush_send_enabled))
        rrdset_done_push(st);
    if (unlikely(st->rrd_memory_mode == RRD_MEMORY_MODE_NONE)) {
        goto after_second_database_work;
    }

    #ifdef NETDATA_INTERNAL_CHECKS
    rrdset_debug(st, "last_collect_ut = %0.3" LONG_DOUBLE_MODIFIER " (last collection time)", (LONG_DOUBLE)last_collect_ut/USEC_PER_SEC);
    rrdset_debug(st, "now_collect_ut  = %0.3" LONG_DOUBLE_MODIFIER " (current collection time)", (LONG_DOUBLE)now_collect_ut/USEC_PER_SEC);
    rrdset_debug(st, "last_stored_ut  = %0.3" LONG_DOUBLE_MODIFIER " (last updated time)", (LONG_DOUBLE)last_stored_ut/USEC_PER_SEC);
    rrdset_debug(st, "next_store_ut   = %0.3" LONG_DOUBLE_MODIFIER " (next interpolation point)", (LONG_DOUBLE)next_store_ut/USEC_PER_SEC);
    #endif

    // calculate totals and count the dimensions
    int dimensions = 0;
    st->collected_total = 0;
    rrddim_foreach_read(rd, st) {
        if (rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED))
            continue;
        dimensions++;
        if(likely(rd->updated))
            st->collected_total += rd->collected_value;
    }

    uint32_t storage_flags = SN_EXISTS;

    // process all dimensions to calculate their values
    // based on the collected figures only
    // at this stage we do not interpolate anything
    rrddim_foreach_read(rd, st) {
        if (rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED))
            continue;

        if(unlikely(!rd->updated)) {
            rd->calculated_value = 0;
            continue;
        }

        if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE))) {
            error("Dimension %s in chart '%s' has the OBSOLETE flag set, but it is collected.", rd->name, st->id);
            rrddim_isnot_obsolete(st, rd);
        }

        #ifdef NETDATA_INTERNAL_CHECKS
        rrdset_debug(st, "%s: START "
                " last_collected_value = " COLLECTED_NUMBER_FORMAT
                " collected_value = " COLLECTED_NUMBER_FORMAT
                " last_calculated_value = " CALCULATED_NUMBER_FORMAT
                " calculated_value = " CALCULATED_NUMBER_FORMAT
                                      , rd->name
                                      , rd->last_collected_value
                                      , rd->collected_value
                                      , rd->last_calculated_value
                                      , rd->calculated_value
        );
        #endif

        switch(rd->algorithm) {
            case RRD_ALGORITHM_ABSOLUTE:
                rd->calculated_value = (calculated_number)rd->collected_value
                                       * (calculated_number)rd->multiplier
                                       / (calculated_number)rd->divisor;

                #ifdef NETDATA_INTERNAL_CHECKS
                rrdset_debug(st, "%s: CALC ABS/ABS-NO-IN "
                            CALCULATED_NUMBER_FORMAT " = "
                            COLLECTED_NUMBER_FORMAT
                            " * " CALCULATED_NUMBER_FORMAT
                            " / " CALCULATED_NUMBER_FORMAT
                          , rd->name
                          , rd->calculated_value
                          , rd->collected_value
                          , (calculated_number)rd->multiplier
                          , (calculated_number)rd->divisor
                );
                #endif

                break;

            case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
                if(unlikely(!st->collected_total))
                    rd->calculated_value = 0;
                else
                    // the percentage of the current value
                    // over the total of all dimensions
                    rd->calculated_value =
                            (calculated_number)100
                            * (calculated_number)rd->collected_value
                            / (calculated_number)st->collected_total;

                #ifdef NETDATA_INTERNAL_CHECKS
                rrdset_debug(st, "%s: CALC PCENT-ROW "
                            CALCULATED_NUMBER_FORMAT " = 100"
                            " * " COLLECTED_NUMBER_FORMAT
                            " / " COLLECTED_NUMBER_FORMAT
                          , rd->name
                          , rd->calculated_value
                          , rd->collected_value
                          , st->collected_total
                );
                #endif

                break;

            case RRD_ALGORITHM_INCREMENTAL:
                if(unlikely(rd->collections_counter <= 1)) {
                    rd->calculated_value = 0;
                    continue;
                }

                // If the new is smaller than the old (an overflow, or reset), set the old equal to the new
                // to reset the calculation (it will give zero as the calculation for this second).
                // It is imperative to set the comparison to uint64_t since type collected_number is signed and
                // produces wrong results as far as incremental counters are concerned.
                if(unlikely((uint64_t)rd->last_collected_value > (uint64_t)rd->collected_value)) {
                    debug(D_RRD_STATS, "%s.%s: RESET or OVERFLOW. Last collected value = " COLLECTED_NUMBER_FORMAT ", current = " COLLECTED_NUMBER_FORMAT
                          , st->name, rd->name
                          , rd->last_collected_value
                          , rd->collected_value);

                    if(!(rrddim_flag_check(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)))
                        storage_flags = SN_EXISTS_RESET;

                    uint64_t last = (uint64_t)rd->last_collected_value;
                    uint64_t new = (uint64_t)rd->collected_value;
                    uint64_t max = (uint64_t)rd->collected_value_max;
                    uint64_t cap = 0;

                    // Signed values are handled by exploiting two's complement which will produce positive deltas
                    if (max > 0x00000000FFFFFFFFULL)
                        cap = 0xFFFFFFFFFFFFFFFFULL; // handles signed and unsigned 64-bit counters
                    else
                        cap = 0x00000000FFFFFFFFULL; // handles signed and unsigned 32-bit counters

                    uint64_t delta = cap - last + new;
                    uint64_t max_acceptable_rate = (cap / 100) * MAX_INCREMENTAL_PERCENT_RATE;

                    // If the delta is less than the maximum acceptable rate and the previous value was near the cap
                    // then this is an overflow. There can be false positives such that a reset is detected as an
                    // overflow.
                    // TODO: remember recent history of rates and compare with current rate to reduce this chance.
                    if (delta < max_acceptable_rate) {
                        rd->calculated_value +=
                                (calculated_number) delta
                                * (calculated_number) rd->multiplier
                                / (calculated_number) rd->divisor;
                    } else {
                        // This is a reset. Any overflow with a rate greater than MAX_INCREMENTAL_PERCENT_RATE will also
                        // be detected as a reset instead.
                        rd->calculated_value += (calculated_number)0;
                    }
                }
                else {
                    rd->calculated_value +=
                            (calculated_number) (rd->collected_value - rd->last_collected_value)
                            * (calculated_number) rd->multiplier
                            / (calculated_number) rd->divisor;
                }

                #ifdef NETDATA_INTERNAL_CHECKS
                rrdset_debug(st, "%s: CALC INC PRE "
                            CALCULATED_NUMBER_FORMAT " = ("
                            COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT
                            ")"
                                    " * " CALCULATED_NUMBER_FORMAT
                            " / " CALCULATED_NUMBER_FORMAT
                          , rd->name
                          , rd->calculated_value
                          , rd->collected_value, rd->last_collected_value
                          , (calculated_number)rd->multiplier
                          , (calculated_number)rd->divisor
                );
                #endif

                break;

            case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
                if(unlikely(rd->collections_counter <= 1)) {
                    rd->calculated_value = 0;
                    continue;
                }

                // if the new is smaller than the old (an overflow, or reset), set the old equal to the new
                // to reset the calculation (it will give zero as the calculation for this second)
                if(unlikely(rd->last_collected_value > rd->collected_value)) {
                    debug(D_RRD_STATS, "%s.%s: RESET or OVERFLOW. Last collected value = " COLLECTED_NUMBER_FORMAT ", current = " COLLECTED_NUMBER_FORMAT
                          , st->name, rd->name
                          , rd->last_collected_value
                          , rd->collected_value
                    );

                    if(!(rrddim_flag_check(rd, RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)))
                        storage_flags = SN_EXISTS_RESET;

                    rd->last_collected_value = rd->collected_value;
                }

                // the percentage of the current increment
                // over the increment of all dimensions together
                if(unlikely(st->collected_total == st->last_collected_total))
                    rd->calculated_value = 0;
                else
                    rd->calculated_value =
                            (calculated_number)100
                            * (calculated_number)(rd->collected_value - rd->last_collected_value)
                            / (calculated_number)(st->collected_total - st->last_collected_total);

                #ifdef NETDATA_INTERNAL_CHECKS
                rrdset_debug(st, "%s: CALC PCENT-DIFF "
                            CALCULATED_NUMBER_FORMAT " = 100"
                            " * (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
                            " / (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
                          , rd->name
                          , rd->calculated_value
                          , rd->collected_value, rd->last_collected_value
                          , st->collected_total, st->last_collected_total
                );
                #endif

                break;

            default:
                // make the default zero, to make sure
                // it gets noticed when we add new types
                rd->calculated_value = 0;

                #ifdef NETDATA_INTERNAL_CHECKS
                rrdset_debug(st, "%s: CALC "
                            CALCULATED_NUMBER_FORMAT " = 0"
                          , rd->name
                          , rd->calculated_value
                );
                #endif

                break;
        }

        #ifdef NETDATA_INTERNAL_CHECKS
        rrdset_debug(st, "%s: PHASE2 "
                    " last_collected_value = " COLLECTED_NUMBER_FORMAT
                    " collected_value = " COLLECTED_NUMBER_FORMAT
                    " last_calculated_value = " CALCULATED_NUMBER_FORMAT
                    " calculated_value = " CALCULATED_NUMBER_FORMAT
                                      , rd->name
                                      , rd->last_collected_value
                                      , rd->collected_value
                                      , rd->last_calculated_value
                                      , rd->calculated_value
        );
        #endif

    }

    // at this point we have all the calculated values ready
    // it is now time to interpolate values on a second boundary

#ifdef NETDATA_INTERNAL_CHECKS
    if(unlikely(now_collect_ut < next_store_ut)) {
        // this is collected in the same interpolation point
        rrdset_debug(st, "THIS IS IN THE SAME INTERPOLATION POINT");
        info("INTERNAL CHECK: host '%s', chart '%s' is collected in the same interpolation point: short by %llu microseconds", st->rrdhost->hostname, st->name, next_store_ut - now_collect_ut);
    }
#endif

    rrdset_done_interpolate(st
            , update_every_ut
            , last_stored_ut
            , next_store_ut
            , last_collect_ut
            , now_collect_ut
            , store_this_entry
            , storage_flags
    );

after_second_database_work:
    st->last_collected_total  = st->collected_total;

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
    time_t mark = now_realtime_sec();
#endif
    rrddim_foreach_read(rd, st) {
        if (rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED))
            continue;

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
        int live = ((mark - rd->last_collected_time.tv_sec) < (RRDSET_MINIMUM_LIVE_COUNT * rd->update_every));
        if (unlikely(live != rd->state->aclk_live_status)) {
            if (likely(rrdset_flag_check(st, RRDSET_FLAG_ACLK))) {
                if (likely(!sql_queue_dimension_to_aclk(rd))) {
                    rd->state->aclk_live_status = live;
                }
            }
        }
#endif
        if(unlikely(!rd->updated))
            continue;

        #ifdef NETDATA_INTERNAL_CHECKS
        rrdset_debug(st, "%s: setting last_collected_value (old: " COLLECTED_NUMBER_FORMAT ") to last_collected_value (new: " COLLECTED_NUMBER_FORMAT ")", rd->name, rd->last_collected_value, rd->collected_value);
        #endif

        rd->last_collected_value = rd->collected_value;

        switch(rd->algorithm) {
            case RRD_ALGORITHM_INCREMENTAL:
                if(unlikely(!first_entry)) {
                    #ifdef NETDATA_INTERNAL_CHECKS
                    rrdset_debug(st, "%s: setting last_calculated_value (old: " CALCULATED_NUMBER_FORMAT ") to last_calculated_value (new: " CALCULATED_NUMBER_FORMAT ")", rd->name, rd->last_calculated_value + rd->calculated_value, rd->calculated_value);
                    #endif

                    rd->last_calculated_value += rd->calculated_value;
                }
                else {
                    #ifdef NETDATA_INTERNAL_CHECKS
                    rrdset_debug(st, "THIS IS THE FIRST POINT");
                    #endif
                }
                break;

            case RRD_ALGORITHM_ABSOLUTE:
            case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
            case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
                #ifdef NETDATA_INTERNAL_CHECKS
                rrdset_debug(st, "%s: setting last_calculated_value (old: " CALCULATED_NUMBER_FORMAT ") to last_calculated_value (new: " CALCULATED_NUMBER_FORMAT ")", rd->name, rd->last_calculated_value, rd->calculated_value);
                #endif

                rd->last_calculated_value = rd->calculated_value;
                break;
        }

        rd->calculated_value = 0;
        rd->collected_value = 0;
        rd->updated = 0;

        #ifdef NETDATA_INTERNAL_CHECKS
        rrdset_debug(st, "%s: END "
                    " last_collected_value = " COLLECTED_NUMBER_FORMAT
                    " collected_value = " COLLECTED_NUMBER_FORMAT
                    " last_calculated_value = " CALCULATED_NUMBER_FORMAT
                    " calculated_value = " CALCULATED_NUMBER_FORMAT
                                      , rd->name
                                      , rd->last_collected_value
                                      , rd->collected_value
                                      , rd->last_calculated_value
                                      , rd->calculated_value
        );
        #endif

    }

    // ALL DONE ABOUT THE DATA UPDATE
    // --------------------------------------------------------------------

    // find if there are any obsolete dimensions
    time_t now = now_realtime_sec();

    if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS))) {
        rrddim_foreach_read(rd, st)
            if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)))
                break;

        if(unlikely(rd)) {
            RRDDIM *last;
            // there is a dimension to free
            // upgrade our read lock to a write lock
            rrdset_unlock(st);
            rrdset_wrlock(st);

            for( rd = st->dimensions, last = NULL ; likely(rd) ; ) {
                if(unlikely(rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE) &&  !rrddim_flag_check(rd, RRDDIM_FLAG_ACLK)
                             && (rd->last_collected_time.tv_sec + rrdset_free_obsolete_time < now))) {
                    info("Removing obsolete dimension '%s' (%s) of '%s' (%s).", rd->name, rd->id, st->name, st->id);

                    if(likely(rd->rrd_memory_mode == RRD_MEMORY_MODE_SAVE || rd->rrd_memory_mode == RRD_MEMORY_MODE_MAP)) {
                        info("Deleting dimension file '%s'.", rd->cache_filename);
                        if(unlikely(unlink(rd->cache_filename) == -1))
                            error("Cannot delete dimension file '%s'", rd->cache_filename);
                    }

#ifdef ENABLE_DBENGINE
                    if (rd->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
                        rrddim_flag_set(rd, RRDDIM_FLAG_ARCHIVED);
                        while(rd->variables)
                            rrddimvar_free(rd->variables);

                        rrddim_flag_clear(rd, RRDDIM_FLAG_OBSOLETE);
                        /* only a collector can mark a chart as obsolete, so we must remove the reference */
                        uint8_t can_delete_metric = rd->state->collect_ops.finalize(rd);
                        if (can_delete_metric) {
                            /* This metric has no data and no references */
                            delete_dimension_uuid(&rd->state->metric_uuid);
                        } else {
                            /* Do not delete this dimension */
                            last = rd;
                            rd = rd->next;
                            continue;
                        }
                    }
#endif
                    if(unlikely(!last)) {
                        rrddim_free(st, rd);
                        rd = st->dimensions;
                        continue;
                    }
                    else {
                        rrddim_free(st, rd);
                        rd = last->next;
                        continue;
                    }
                }

                last = rd;
                rd = rd->next;
            }
        }
        else {
            rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS);
        }
    }

    rrdset_unlock(st);

    netdata_thread_enable_cancelability();
}

void rrdset_add_label_to_new_list(RRDSET *st, char *key, char *value, LABEL_SOURCE source)
{
    st->state->new_labels = add_label_to_list(st->state->new_labels, key, value, source);
}

void rrdset_finalize_labels(RRDSET *st)
{
    struct label *new_labels = st->state->new_labels;
    struct label_index *labels = &st->state->labels;

    if (!labels->head) {
        labels->head = new_labels;
    } else {
        replace_label_list(labels, new_labels);
    }

    netdata_rwlock_wrlock(&labels->labels_rwlock);
    struct label *lbl = labels->head;
    while (lbl) {
        sql_store_chart_label(st->chart_uuid, (int)lbl->label_source, lbl->key, lbl->value);
        lbl = lbl->next;
    }
    netdata_rwlock_unlock(&labels->labels_rwlock);

    st->state->new_labels = NULL;
}

void rrdset_update_labels(RRDSET *st, struct label *labels)
{
    if (!labels)
        return;

    update_label_list(&st->state->new_labels, labels);
    rrdset_finalize_labels(st);
}

int rrdset_contains_label_keylist(RRDSET *st, char *keylist)
{
    struct label_index *labels = &st->state->labels;
    int ret;

    if (!labels->head)
        return 0;

    netdata_rwlock_rdlock(&labels->labels_rwlock);
    ret = label_list_contains_keylist(labels->head, keylist);
    netdata_rwlock_unlock(&labels->labels_rwlock);

    return ret;
}

struct label *rrdset_lookup_label_key(RRDSET *st, char *key, uint32_t key_hash)
{
    struct label_index *labels = &st->state->labels;
    struct label *ret = NULL;

    if (labels->head) {
        netdata_rwlock_rdlock(&labels->labels_rwlock);
        ret = label_list_lookup_key(labels->head, key, key_hash);
        netdata_rwlock_unlock(&labels->labels_rwlock);
    }
    return ret;
}
