// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

#if RRD_STORAGE_TIERS != 5
#error RRD_STORAGE_TIERS is not 5 - you need to update the grouping iterations per tier
#endif

RRDHOST *localhost = NULL;
netdata_rwlock_t rrd_rwlock = NETDATA_RWLOCK_INITIALIZER;

RRDHOST *rrdhost_find_by_node_id(char *node_id) {

    ND_UUID node_uuid;
    if (unlikely(!node_id || uuid_parse(node_id, node_uuid.uuid)))
        return NULL;

    RRDHOST *host, *ret = NULL;
    dfe_start_read(rrdhost_root_index, host) {
        if (UUIDeq(host->node_id, node_uuid)) {
            ret = host;
            break;
        }
    }
    dfe_done(host);

    return ret;
}

RRDHOST *rrdhost_find_by_hostname(const char *hostname) {
    if(strcmp(hostname, "localhost") == 0)
        return localhost;

    STRING *name = string_strdupz(hostname);

    RRDHOST *host, *ret = NULL;
    dfe_start_read(rrdhost_root_index, host) {
        if (host->hostname == name) {
            ret = host;
            break;
        }
    }
    dfe_done(host);

    string_freez(name);

    return ret;
}

// ----------------------------------------------------------------------------
// RRDHOST indexes management

DICTIONARY *rrdhost_root_index = NULL;

void rrdhost_init() {
    if(unlikely(!rrdhost_root_index)) {
        rrdhost_root_index = dictionary_create_advanced(
            DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE,
            &dictionary_stats_category_rrdhost, 0);
    }
}

RRDHOST_ACQUIRED *rrdhost_find_and_acquire(const char *machine_guid) {
    return (RRDHOST_ACQUIRED *)dictionary_get_and_acquire_item(rrdhost_root_index, machine_guid);
}

RRDHOST *rrdhost_acquired_to_rrdhost(RRDHOST_ACQUIRED *rha) {
    if(unlikely(!rha))
        return NULL;

    return (RRDHOST *) dictionary_acquired_item_value((const DICTIONARY_ITEM *)rha);
}

void rrdhost_acquired_release(RRDHOST_ACQUIRED *rha) {
    if(unlikely(!rha))
        return;

    dictionary_acquired_item_release(rrdhost_root_index, (const DICTIONARY_ITEM *)rha);
}

// ----------------------------------------------------------------------------
// RRDHOST index by UUID

inline size_t rrdhost_hosts_available(void) {
    return dictionary_entries(rrdhost_root_index);
}

inline RRDHOST *rrdhost_find_by_guid(const char *guid) {
    return dictionary_get(rrdhost_root_index, guid);
}

static inline RRDHOST *rrdhost_index_add_by_guid(RRDHOST *host) {
    return dictionary_set(rrdhost_root_index, host->machine_guid, host, sizeof(RRDHOST));
}

static void rrdhost_index_del_by_guid(RRDHOST *host) {
    RRDHOST *t = rrdhost_find_by_guid(host->machine_guid);
    if(t == host) {
        if (!dictionary_del(rrdhost_root_index, host->machine_guid))
            nd_log(
                NDLS_DAEMON, NDLP_NOTICE,
                "RRDHOST: failed to delete machine guid '%s' from index",
                host->machine_guid);
    }
    else
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "RRDHOST: failed to delete machine guid '%s' from index, not found",
               host->machine_guid);
}

// ----------------------------------------------------------------------------
// RRDHOST - internal helpers

static inline void rrdhost_init_hostname(RRDHOST *host, const char *hostname) {
    if(unlikely(hostname && !*hostname)) hostname = NULL;

    if(host->hostname && hostname && !strcmp(rrdhost_hostname(host), hostname))
        return;

    STRING *old = host->hostname;
    host->hostname = string_strdupz(hostname?hostname:"localhost");
    string_freez(old);
}

static inline void rrdhost_init_os(RRDHOST *host, const char *os) {
    if(host->os && os && !strcmp(rrdhost_os(host), os))
        return;

    STRING *old = host->os;
    host->os = string_strdupz(os?os:"unknown");
    string_freez(old);
}

static inline void rrdhost_init_timezone(RRDHOST *host, const char *timezone, const char *abbrev_timezone, int32_t utc_offset) {
    if (host->timezone && timezone && !strcmp(rrdhost_timezone(host), timezone) && host->abbrev_timezone && abbrev_timezone &&
        !strcmp(rrdhost_abbrev_timezone(host), abbrev_timezone) && host->utc_offset == utc_offset)
        return;

    STRING *old = host->timezone;
    host->timezone = string_strdupz((timezone && *timezone)?timezone:"unknown");
    string_freez(old);

    old = (void *)host->abbrev_timezone;
    host->abbrev_timezone = string_strdupz((abbrev_timezone && *abbrev_timezone) ? abbrev_timezone : "UTC");
    string_freez(old);

    host->utc_offset = utc_offset;
}

void set_host_properties(RRDHOST *host, int update_every,
    RRD_DB_MODE memory_mode,
                         const char *registry_hostname, const char *os, const char *tzone,
                         const char *abbrev_tzone, int32_t utc_offset, const char *prog_name,
                         const char *prog_version)
{

    host->rrd_update_every = update_every;
    host->rrd_memory_mode = memory_mode;

    rrdhost_init_os(host, os);
    rrdhost_init_timezone(host, tzone, abbrev_tzone, utc_offset);

    host->program_name = string_strdupz((prog_name && *prog_name) ? prog_name : "unknown");
    host->program_version = string_strdupz((prog_version && *prog_version) ? prog_version : "unknown");
    host->registry_hostname = string_strdupz((registry_hostname && *registry_hostname) ? registry_hostname : rrdhost_hostname(host));
}

// ----------------------------------------------------------------------------
// RRDHOST - add a host

#ifdef ENABLE_DBENGINE
//
//  true on success
//
static bool create_dbengine_directory(RRDHOST *host, const char *dbenginepath)
{
    int ret = mkdir(dbenginepath, 0775);
    if (ret != 0 && errno != EEXIST) {
        nd_log(NDLS_DAEMON, NDLP_CRIT, "Host '%s': cannot create directory '%s'", rrdhost_hostname(host), dbenginepath);
        return false;
    }
    return true;
}

static RRDHOST *prepare_host_for_unittest(RRDHOST *host)
{
    char dbenginepath[FILENAME_MAX + 1];

    if (host->cache_dir)
        freez(host->cache_dir);

    snprintfz(dbenginepath, FILENAME_MAX, "%s/%s", netdata_configured_cache_dir, host->machine_guid);
    host->cache_dir = strdupz(dbenginepath);

    int ret = 0;

    bool initialized;
    if ((initialized = create_dbengine_directory(host, dbenginepath))) {
        snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", host->cache_dir);

        if ((initialized = create_dbengine_directory(host, dbenginepath))) {
            host->db[0].mode = RRD_DB_MODE_DBENGINE;
            host->db[0].eng = storage_engine_get(host->db[0].mode);
            host->db[0].tier_grouping = get_tier_grouping(0);

            ret = rrdeng_init(
                (struct rrdengine_instance **)&host->db[0].si,
                dbenginepath,
                default_rrdeng_disk_quota_mb,
                0,
                0); // may fail here for legacy dbengine initialization

            initialized = (ret == 0);

            if (initialized)
                rrdeng_readiness_wait((struct rrdengine_instance *)host->db[0].si);
        }
    }

    if (!initialized) {
        nd_log(
            NDLS_DAEMON,
            NDLP_CRIT,
            "Host '%s': cannot initialize host with machine guid '%s'. Failed to initialize DB engine at '%s'.",
            rrdhost_hostname(host),
            host->machine_guid,
            host->cache_dir);

        rrd_wrlock();
        rrdhost_free___while_having_rrd_wrlock(host);
        rrd_wrunlock();
        return NULL;
    }
    return host;
}
#endif

static void rrdhost_set_replication_parameters(RRDHOST *host, RRD_DB_MODE memory_mode, time_t period, time_t step) {
    host->stream.replication.period = period;
    host->stream.replication.step = step;
    host->stream.rcv.status.replication.percent = 100.0;

    switch(memory_mode) {
        default:
        case RRD_DB_MODE_ALLOC:
        case RRD_DB_MODE_RAM:
            if(host->stream.replication.period > (time_t) host->rrd_history_entries * (time_t) host->rrd_update_every)
                host->stream.replication.period = (time_t) host->rrd_history_entries * (time_t) host->rrd_update_every;
            break;

        case RRD_DB_MODE_DBENGINE:
            break;
    }
}

RRDHOST *rrdhost_create(
        const char *hostname,
        const char *registry_hostname,
        const char *guid,
        const char *os,
        const char *timezone,
        const char *abbrev_timezone,
        int32_t utc_offset,
        const char *prog_name,
        const char *prog_version,
        int update_every,
        long entries,
        RRD_DB_MODE memory_mode,
        bool health,
        bool stream,
        STRING *parents,
        STRING *api_key,
        STRING *send_charts_matching,
        bool replication,
        time_t replication_period,
        time_t replication_step,
        struct rrdhost_system_info *system_info,
        int is_localhost,
        bool archived
) {
    if(memory_mode == RRD_DB_MODE_DBENGINE && !dbengine_enabled) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "memory mode 'dbengine' is not enabled, but host '%s' is configured for it. Falling back to 'alloc'",
               hostname);

        memory_mode = RRD_DB_MODE_ALLOC;
    }

    RRDHOST *host = callocz(1, sizeof(RRDHOST));
    host->state_id = OBJECT_STATE_INIT_DEACTIVATED;

    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(RRDHOST), __ATOMIC_RELAXED);

    strncpyz(host->machine_guid, guid, GUID_LEN + 1);
    rrdhost_stream_path_init(host);
    rrdhost_stream_parents_init(host);

    set_host_properties(
        host,
        (update_every > 0) ? update_every : 1,
        memory_mode,
        registry_hostname,
        os,
        timezone,
        abbrev_timezone,
        utc_offset,
        prog_name,
        prog_version);

    rrdhost_init_hostname(host, hostname);

    host->rrd_history_entries = align_entries_to_pagesize(memory_mode, entries);
    host->health.enabled = ((memory_mode == RRD_DB_MODE_NONE)) ? false : health;

    spinlock_init(&host->receiver_lock);

    if (likely(!archived)) {
        rrd_functions_host_init(host);
        host->stream.snd.status.last_connected = now_realtime_sec();
        host->rrdlabels = rrdlabels_create();
        stream_sender_structures_init(host, stream, parents, api_key, send_charts_matching);
    }

    if(replication)
        rrdhost_option_set(host, RRDHOST_OPTION_REPLICATION);
    else
        rrdhost_option_clear(host, RRDHOST_OPTION_REPLICATION);

    rrdhost_set_replication_parameters(host, memory_mode, replication_period, replication_step);

    host->system_info = rrdhost_system_info_create();
    rrdhost_system_info_swap(host->system_info, system_info);

    rrdset_index_init(host);

    if(is_localhost)
        host->cache_dir  = strdupz(netdata_configured_cache_dir);

    // this is also needed for custom host variables - not only health
    host->rrdvars = rrdvariables_create();

    if (likely(!uuid_parse(host->machine_guid, host->host_id.uuid)))
        sql_load_node_id(host);
    else
        error_report("Host machine GUID %s is not valid", host->machine_guid);

    rrdcalc_rrdhost_index_init(host);

    if (host->rrd_memory_mode == RRD_DB_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        if (unittest_running) {
            host = prepare_host_for_unittest(host);
            if (!host)
                return NULL;
        }
        else {
            for(size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {
                host->db[tier].mode = RRD_DB_MODE_DBENGINE;
                host->db[tier].eng = storage_engine_get(host->db[tier].mode);
                host->db[tier].si = (STORAGE_INSTANCE *)multidb_ctx[tier];
                host->db[tier].tier_grouping = get_tier_grouping(tier);
            }
        }
#else
        fatal("RRD_DB_MODE_DBENGINE is not supported in this platform.");
#endif
    }
    else {
        host->db[0].mode = host->rrd_memory_mode;
        host->db[0].eng = storage_engine_get(host->db[0].mode);
        host->db[0].si = NULL;
        host->db[0].tier_grouping = get_tier_grouping(0);

#ifdef ENABLE_DBENGINE
        // the first tier is reserved for the non-dbengine modes
        for(size_t tier = 1; tier < nd_profile.storage_tiers; tier++) {
            host->db[tier].mode = RRD_DB_MODE_DBENGINE;
            host->db[tier].eng = storage_engine_get(host->db[tier].mode);
            host->db[tier].si = (STORAGE_INSTANCE *) multidb_ctx[tier];
            host->db[tier].tier_grouping = get_tier_grouping(tier);
        }
#endif
    }

    // ------------------------------------------------------------------------
    // init new ML host and update system_info to let upstreams know
    // about ML functionality
    //

    if (is_localhost && host->system_info) {
        rrdhost_system_info_ml_capable_set(host->system_info, ml_capable());
        rrdhost_system_info_ml_enabled_set(host->system_info, ml_enabled(host));
        rrdhost_system_info_mc_version_set(host->system_info, metric_correlations_version);
    }

    // ------------------------------------------------------------------------
    // link it and add it to the index

    rrd_wrlock();

    RRDHOST *t = rrdhost_index_add_by_guid(host);
    if(t != host) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "Host '%s': cannot add host with machine guid '%s' to index. It already exists as host '%s' with machine guid '%s'.",
               rrdhost_hostname(host), host->machine_guid, rrdhost_hostname(t), t->machine_guid);

        if (!is_localhost)
            rrdhost_free___while_having_rrd_wrlock(host);

        rrd_wrunlock();
        return NULL;
    }

    if(is_localhost)
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(localhost, host, prev, next);
    else
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(localhost, host, prev, next);

    rrd_wrunlock();

    // ------------------------------------------------------------------------

    nd_log(NDLS_DAEMON, NDLP_INFO,
           "Host '%s' (at registry as '%s') with guid '%s' initialized"
           ", os '%s'"
           ", timezone '%s'"
           ", program_name '%s'"
           ", program_version '%s'"
           ", update every %d"
           ", memory mode %s"
           ", history entries %d"
           ", streaming %s"
           " (to '%s' with api key '%s')"
           ", health %s"
           ", cache_dir '%s'"
           ", alarms default handler '%s'"
           ", alarms default recipient '%s'"
         , rrdhost_hostname(host)
         , rrdhost_registry_hostname(host)
         , host->machine_guid
         , rrdhost_os(host)
         , rrdhost_timezone(host)
         , rrdhost_program_name(host)
         , rrdhost_program_version(host)
         , host->rrd_update_every
         , rrd_memory_mode_name(host->rrd_memory_mode)
         , host->rrd_history_entries
         ,
        rrdhost_has_stream_sender_enabled(host)?"enabled":"disabled"
         , string2str(host->stream.snd.destination)
         , string2str(host->stream.snd.api_key)
         , host->health.enabled ?"enabled":"disabled"
         , host->cache_dir
         , string2str(host->health.default_exec)
         , string2str(host->health.default_recipient)
    );

    if(!archived) {
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_UPDATE);
        if (is_localhost) {
            BUFFER *buf = buffer_create(0, NULL);
            size_t query_counter = 0;
            store_host_info_and_metadata(host, buf, &query_counter);
            buffer_free(buf);
        }
        rrdhost_load_rrdcontext_data(host);
        ml_host_new(host);
    } else
        rrdhost_flag_set(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD | RRDHOST_FLAG_ARCHIVED | RRDHOST_FLAG_ORPHAN);

    return host;
}

static void rrdhost_update(RRDHOST *host
                           , const char *hostname
                           , const char *registry_hostname
                           , const char *guid
                           , const char *os
                           , const char *timezone
                           , const char *abbrev_timezone
                           , int32_t utc_offset
                           , const char *prog_name
                           , const char *prog_version
                           , int update_every
                           , long history
                           , RRD_DB_MODE mode
                           , bool health
                           , bool stream
                           , STRING *parents
                           , STRING *api_key
                           , STRING *send_charts_matching
                           , bool replication
                           , time_t replication_period
                           , time_t replication_step
                           , struct rrdhost_system_info *system_info
)
{
    UNUSED(guid);

    spinlock_lock(&host->rrdhost_update_lock);

    host->health.enabled = (mode == RRD_DB_MODE_NONE) ? 0 : health;

    rrdhost_system_info_swap(host->system_info, system_info);
    rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_CLAIMID | RRDHOST_FLAG_METADATA_UPDATE);

    rrdhost_init_os(host, os);
    rrdhost_init_timezone(host, timezone, abbrev_timezone, utc_offset);

    string_freez(host->registry_hostname);
    host->registry_hostname = string_strdupz((registry_hostname && *registry_hostname)?registry_hostname:hostname);

    if(strcmp(rrdhost_hostname(host), hostname) != 0) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Host '%s' has been renamed to '%s'. If this is not intentional it may mean multiple hosts are using the same machine_guid.",
               rrdhost_hostname(host), hostname);

        rrdhost_init_hostname(host, hostname);
    }

    if(strcmp(rrdhost_program_name(host), prog_name) != 0) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "Host '%s' switched program name from '%s' to '%s'",
               rrdhost_hostname(host), rrdhost_program_name(host),
            prog_name);

        STRING *t = host->program_name;
        host->program_name = string_strdupz(prog_name);
        string_freez(t);
    }

    if(strcmp(rrdhost_program_version(host), prog_version) != 0) {
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "Host '%s' switched program version from '%s' to '%s'",
               rrdhost_hostname(host), rrdhost_program_version(host),
            prog_version);

        STRING *t = host->program_version;
        host->program_version = string_strdupz(prog_version);
        string_freez(t);
    }

    if(host->rrd_update_every != update_every)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Host '%s' has an update frequency of %d seconds, but the wanted one is %d seconds. "
               "Restart netdata here to apply the new settings.",
               rrdhost_hostname(host), host->rrd_update_every, update_every);

    if(host->rrd_memory_mode != mode)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Host '%s' has memory mode '%s', but the wanted one is '%s'. "
               "Restart netdata here to apply the new settings.",
               rrdhost_hostname(host),
               rrd_memory_mode_name(host->rrd_memory_mode),
               rrd_memory_mode_name(mode));

    else if(host->rrd_memory_mode != RRD_DB_MODE_DBENGINE && host->rrd_history_entries < history)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Host '%s' has history of %d entries, but the wanted one is %ld entries. "
               "Restart netdata here to apply the new settings.",
               rrdhost_hostname(host),
               host->rrd_history_entries,
               history);

    if(!host->rrdvars)
        host->rrdvars = rrdvariables_create();

    host->stream.snd.status.last_connected = now_realtime_sec();

    if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_ARCHIVED);

        rrd_functions_host_init(host);

        if(!host->rrdlabels)
            host->rrdlabels = rrdlabels_create();

        if (!host->rrdset_root_index)
            rrdset_index_init(host);

        stream_sender_structures_init(host, stream, parents, api_key, send_charts_matching);

        rrdcalc_rrdhost_index_init(host);

        if(replication)
            rrdhost_option_set(host, RRDHOST_OPTION_REPLICATION);
        else
            rrdhost_option_clear(host, RRDHOST_OPTION_REPLICATION);

        rrdhost_set_replication_parameters(host, host->rrd_memory_mode, replication_period, replication_step);

        ml_host_new(host);

        rrdhost_load_rrdcontext_data(host);
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "Host %s is not in archived mode anymore",
               rrdhost_hostname(host));
    }

    spinlock_unlock(&host->rrdhost_update_lock);
}

RRDHOST *rrdhost_find_or_create(
      const char *hostname
    , const char *registry_hostname
    , const char *guid
    , const char *os
    , const char *timezone
    , const char *abbrev_timezone
    , int32_t utc_offset
    , const char *prog_name
    , const char *prog_version
    , int update_every
    , long history
    ,
    RRD_DB_MODE mode
    , bool health
    , bool stream
    , STRING *parents
    , STRING *api_key
    , STRING *send_charts_matching
    , bool replication
    , time_t replication_period
    , time_t replication_step
    , struct rrdhost_system_info *system_info
    , bool archived
) {
    RRDHOST *host = rrdhost_find_by_guid(guid);
    if (unlikely(host && host->rrd_memory_mode != mode && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) {

        if (likely(!archived && rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD)))
            return host;

        /* If a legacy memory mode instantiates all dbengine state must be discarded to avoid inconsistencies */
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "Archived host '%s' has memory mode '%s', but the wanted one is '%s'. Discarding archived state.",
               rrdhost_hostname(host),
               rrd_memory_mode_name(host->rrd_memory_mode),
               rrd_memory_mode_name(mode));

        rrd_wrlock();
        rrdhost_free___while_having_rrd_wrlock(host);
        host = NULL;
        rrd_wrunlock();
    }

    if(!host) {
        host = rrdhost_create(
                hostname
                , registry_hostname
                , guid
                , os
                , timezone
                , abbrev_timezone
                , utc_offset
                , prog_name
                , prog_version
                , update_every
                , history
                , mode
                , health
                , stream
                , parents
                , api_key
                , send_charts_matching
                , replication
                , replication_period
                , replication_step
                , system_info
                , 0
                , archived
         );
    }
    else {
        if (likely(!rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD)))
            rrdhost_update(
                host
                , hostname
                , registry_hostname
                , guid
                , os
                , timezone
                , abbrev_timezone
                , utc_offset
                , prog_name
                , prog_version
                , update_every
                , history
                , mode
                , health
                , stream
                , parents
                , api_key
                , send_charts_matching
                , replication
                , replication_period
                , replication_step
                , system_info);
    }

    return host;
}

bool rrdhost_should_be_cleaned_up(RRDHOST *host, RRDHOST *protected_host, time_t now_s) {
    if(host != protected_host
        && host != localhost
        && rrdhost_receiver_replicating_charts(host) == 0
        && rrdhost_sender_replicating_charts(host) == 0
        && rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN)
        && !rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD | RRDHOST_FLAG_COLLECTOR_ONLINE)
        && health_evloop_current_iteration() - rrdhost_health_evloop_last_iteration(host) > 10
        && host->stream.rcv.status.last_disconnected
        && host->stream.rcv.status.last_disconnected + rrdhost_cleanup_orphan_to_archive_time_s < now_s)
        return true;

    return false;
}

bool rrdhost_should_run_health(RRDHOST *host) {
    if (!host->health.enabled || !rrdhost_flag_check(host, RRDHOST_FLAG_COLLECTOR_ONLINE) ||
        rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN) || rrdhost_ingestion_status(host) != RRDHOST_INGEST_STATUS_ONLINE)
        return false;

    return true;
}

// ----------------------------------------------------------------------------
// RRDHOST - free

void rrdhost_cleanup_data_collection_and_health(RRDHOST *host) {
    stream_receiver_signal_to_stop_and_wait(host, STREAM_HANDSHAKE_SND_DISCONNECT_HOST_CLEANUP);

    rrdhost_pluginsd_send_chart_slots_free(host);
    rrdhost_pluginsd_receive_chart_slots_free(host);

    rrdcalc_delete_all(host);
    rrdset_index_destroy(host);
    rrdcalc_rrdhost_index_destroy(host);
    health_alarm_log_free(host);

    ml_host_delete(host);

    freez(host->exporting_flags);
    host->exporting_flags = NULL;

    rrd_functions_host_destroy(host);
    rrdvariables_destroy(host->rrdvars);
    host->rrdvars = NULL;

    rrdhost_stream_path_clear(host, true);
    stream_sender_structures_free(host);

    rrdhost_flag_set(host, RRDHOST_FLAG_ARCHIVED | RRDHOST_FLAG_ORPHAN);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "RRD: 'host:%s' is now in archive mode...",
           rrdhost_hostname(host));
}

void rrdhost_free___while_having_rrd_wrlock(RRDHOST *host) {
    if(!host) return;

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "RRD: 'host:%s' freeing memory...",
           rrdhost_hostname(host));

    // ------------------------------------------------------------------------
    // first remove it from the indexes, so that it will not be discoverable

    rrdhost_index_del_by_guid(host);

    if (host->prev)
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(localhost, host, prev, next);

    // ------------------------------------------------------------------------

    rrdhost_cleanup_data_collection_and_health(host);

    // ------------------------------------------------------------------------
    // free it

    pulse_host_status(host, PULSE_HOST_STATUS_DELETED, 0);
    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(RRDHOST), __ATOMIC_RELAXED);

    if (host == localhost)
        health_plugin_destroy();

    freez(host->cache_dir);
    rrdhost_stream_parents_free(host, false);
    simple_pattern_free(host->stream.snd.charts_matching);
    rrdhost_system_info_free(host->system_info);

    rrdhost_destroy_rrdcontexts(host);
    rrdlabels_destroy(host->rrdlabels);
    destroy_aclk_config(host);

    string_freez(host->hostname);
    string_freez(host->os);
    string_freez(host->timezone);
    string_freez(host->abbrev_timezone);
    string_freez(host->program_name);
    string_freez(host->program_version);
    string_freez(host->health.default_exec);
    string_freez(host->health.default_recipient);
    string_freez(host->registry_hostname);
    string_freez(host->stream.snd.api_key);
    string_freez(host->stream.snd.destination);
    freez(host);
}

void rrdhost_free_all(void) {
    rrd_wrlock();

    /* Make sure child-hosts are released before the localhost. */
    while(localhost && localhost->next)
        rrdhost_free___while_having_rrd_wrlock(localhost->next);

    if(localhost)
        rrdhost_free___while_having_rrd_wrlock(localhost);

    localhost = NULL;

    RRDHOST *host;
    dfe_start_write(rrdhost_root_index, host) {
        fprintf(stderr, "RRDHOST: MACHINE_GUID '%s' is still in the dictionary!\n",
                host_dfe.name);
    }
    dfe_done(host);

    dictionary_garbage_collect(rrdhost_root_index);
    dictionary_destroy(rrdhost_root_index);
    rrdhost_root_index = NULL;

    rrd_wrunlock();
}
