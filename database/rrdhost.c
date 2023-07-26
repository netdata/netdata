// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

static void rrdhost_streaming_sender_structures_init(RRDHOST *host);

#if RRD_STORAGE_TIERS != 5
#error RRD_STORAGE_TIERS is not 5 - you need to update the grouping iterations per tier
#endif

size_t get_tier_grouping(size_t tier) {
    if(unlikely(tier >= rrdb.storage_tiers)) tier = rrdb.storage_tiers - 1;

    size_t grouping = 1;
    // first tier is always 1 iteration of whatever update every the chart has
    for(size_t i = 1; i <= tier ;i++)
        grouping *= rrdb.storage_tiers_grouping_iterations[i];

    return grouping;
}

bool is_storage_engine_shared(STORAGE_INSTANCE *engine __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(!rrdeng_is_legacy(engine))
        return true;
#endif

    return false;
}

RRDHOST *rrdhost_find_by_node_id(const char *node_id) {
    uuid_t node_uuid;
    if (unlikely(!node_id || uuid_parse(node_id, node_uuid)))
        return NULL;

    RRDHOST *host, *ret = NULL;
    dfe_start_read(rrdb.rrdhost_root_index, host) {
        if (host->node_id && !(uuid_memcmp(host->node_id, &node_uuid))) {
            ret = host;
            break;
        }
    }
    dfe_done(host);

    return ret;
}

// ----------------------------------------------------------------------------
// RRDHOST indexes management

RRDHOST_ACQUIRED *rrdhost_find_and_acquire(const char *machine_guid) {
    netdata_log_debug(D_RRD_CALLS, "rrdhost_find_and_acquire() host %s", machine_guid);

    return (RRDHOST_ACQUIRED *)dictionary_get_and_acquire_item(rrdb.rrdhost_root_index, machine_guid);
}

RRDHOST *rrdhost_acquired_to_rrdhost(RRDHOST_ACQUIRED *rha) {
    if(unlikely(!rha))
        return NULL;

    return (RRDHOST *) dictionary_acquired_item_value((const DICTIONARY_ITEM *)rha);
}

void rrdhost_acquired_release(RRDHOST_ACQUIRED *rha) {
    if(unlikely(!rha))
        return;

    dictionary_acquired_item_release(rrdb.rrdhost_root_index, (const DICTIONARY_ITEM *)rha);
}

// ----------------------------------------------------------------------------
// RRDHOST index by UUID

inline size_t rrdhost_hosts_available(void) {
    return dictionary_entries(rrdb.rrdhost_root_index);
}

inline RRDHOST *rrdhost_find_by_guid(const char *guid) {
    return dictionary_get(rrdb.rrdhost_root_index, guid);
}

static inline RRDHOST *rrdhost_index_add_by_guid(RRDHOST *host) {
    RRDHOST *ret_machine_guid = dictionary_set(rrdb.rrdhost_root_index, host->machine_guid, host, sizeof(RRDHOST));
    if(ret_machine_guid == host)
        rrdhost_option_set(host, RRDHOST_OPTION_INDEXED_MACHINE_GUID);
    else {
        rrdhost_option_clear(host, RRDHOST_OPTION_INDEXED_MACHINE_GUID);
        netdata_log_error("RRDHOST: %s() host with machine guid '%s' is already indexed",
                          __FUNCTION__, host->machine_guid);
    }

    return host;
}

static void rrdhost_index_del_by_guid(RRDHOST *host) {
    if(rrdhost_option_check(host, RRDHOST_OPTION_INDEXED_MACHINE_GUID)) {
        if(!dictionary_del(rrdb.rrdhost_root_index, host->machine_guid))
            netdata_log_error("RRDHOST: %s() failed to delete machine guid '%s' from index",
                              __FUNCTION__, host->machine_guid);

        rrdhost_option_clear(host, RRDHOST_OPTION_INDEXED_MACHINE_GUID);
    }
}

// ----------------------------------------------------------------------------
// RRDHOST index by hostname

inline RRDHOST *rrdhost_find_by_hostname(const char *hostname) {
    if(unlikely(!strcmp(hostname, "localhost")))
        return rrdb.localhost;

    return dictionary_get(rrdb.rrdhost_root_index_hostname, hostname);
}

static inline void rrdhost_index_del_hostname(RRDHOST *host) {
    if(unlikely(!host->hostname)) return;

    if(rrdhost_option_check(host, RRDHOST_OPTION_INDEXED_HOSTNAME)) {
        if(!dictionary_del(rrdb.rrdhost_root_index_hostname, rrdhost_hostname(host)))
            netdata_log_error("RRDHOST: %s() failed to delete hostname '%s' from index",
                              __FUNCTION__, rrdhost_hostname(host));

        rrdhost_option_clear(host, RRDHOST_OPTION_INDEXED_HOSTNAME);
    }
}

static inline RRDHOST *rrdhost_index_add_hostname(RRDHOST *host) {
    if(!host->hostname) return host;

    RRDHOST *ret_hostname = dictionary_set(rrdb.rrdhost_root_index_hostname, rrdhost_hostname(host), host, sizeof(RRDHOST));
    if(ret_hostname == host)
        rrdhost_option_set(host, RRDHOST_OPTION_INDEXED_HOSTNAME);
    else {
        //have the same hostname but it's not the same host
        //keep the new one only if the old one is orphan or archived
        if (rrdhost_flag_check(ret_hostname, RRDHOST_FLAG_ORPHAN) || rrdhost_flag_check(ret_hostname, RRDHOST_FLAG_ARCHIVED)) {
            rrdhost_index_del_hostname(ret_hostname);
            rrdhost_index_add_hostname(host);
        }
    }

    return host;
}

// ----------------------------------------------------------------------------
// RRDHOST - internal helpers

static inline void rrdhost_init_tags(RRDHOST *host, const char *tags) {
    if(host->tags && tags && !strcmp(rrdhost_tags(host), tags))
        return;

    STRING *old = host->tags;
    host->tags = string_strdupz((tags && *tags)?tags:NULL);
    string_freez(old);
}

static inline void rrdhost_init_hostname(RRDHOST *host, const char *hostname, bool add_to_index) {
    if(unlikely(hostname && !*hostname)) hostname = NULL;

    if(host->hostname && hostname && !strcmp(rrdhost_hostname(host), hostname))
        return;

    rrdhost_index_del_hostname(host);

    STRING *old = host->hostname;
    host->hostname = string_strdupz(hostname?hostname:"localhost");
    string_freez(old);

    if(add_to_index)
        rrdhost_index_add_hostname(host);
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

void set_host_properties(RRDHOST *host, int update_every, STORAGE_ENGINE_ID storage_engine_id,
                         const char *registry_hostname, const char *os, const char *tags,
                         const char *tzone, const char *abbrev_tzone, int32_t utc_offset, const char *program_name,
                         const char *program_version)
{

    host->update_every = update_every;
    host->storage_engine_id = storage_engine_id;

    rrdhost_init_os(host, os);
    rrdhost_init_timezone(host, tzone, abbrev_tzone, utc_offset);
    rrdhost_init_tags(host, tags);

    host->program_name = string_strdupz((program_name && *program_name) ? program_name : "unknown");
    host->program_version = string_strdupz((program_version && *program_version) ? program_version : "unknown");
    host->registry_hostname = string_strdupz((registry_hostname && *registry_hostname) ? registry_hostname : rrdhost_hostname(host));
}

// ----------------------------------------------------------------------------
// RRDHOST - add a host

static void rrdhost_initialize_rrdpush_sender(RRDHOST *host,
                                       unsigned int rrdpush_enabled,
                                       char *rrdpush_destination,
                                       char *rrdpush_api_key,
                                       char *rrdpush_send_charts_matching
) {
    if(rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_INITIALIZED)) return;

    if(rrdpush_enabled && rrdpush_destination && *rrdpush_destination && rrdpush_api_key && *rrdpush_api_key) {
        rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_INITIALIZED);

        rrdhost_streaming_sender_structures_init(host);

#ifdef ENABLE_HTTPS
        host->sender->ssl = NETDATA_SSL_UNSET_CONNECTION;
#endif

        host->rrdpush_send_destination = strdupz(rrdpush_destination);
        rrdpush_destinations_init(host);

        host->rrdpush_send_api_key = strdupz(rrdpush_api_key);
        host->rrdpush_send_charts_matching = simple_pattern_create(rrdpush_send_charts_matching, NULL,
                                                                   SIMPLE_PATTERN_EXACT, true);

        rrdhost_option_set(host, RRDHOST_OPTION_SENDER_ENABLED);
    }
    else
        rrdhost_option_clear(host, RRDHOST_OPTION_SENDER_ENABLED);
}

RRDHOST *rrdhost_create(
        const char *hostname,
        const char *registry_hostname,
        const char *guid,
        const char *os,
        const char *timezone,
        const char *abbrev_timezone,
        int32_t utc_offset,
        const char *tags,
        const char *program_name,
        const char *program_version,
        int update_every,
        long entries,
        STORAGE_ENGINE_ID storage_engine_id,
        unsigned int health_enabled,
        unsigned int rrdpush_enabled,
        char *rrdpush_destination,
        char *rrdpush_api_key,
        char *rrdpush_send_charts_matching,
        bool rrdpush_enable_replication,
        time_t rrdpush_seconds_to_replicate,
        time_t rrdpush_replication_step,
        struct rrdhost_system_info *system_info,
        int is_localhost,
        bool archived
) {
    netdata_log_debug(D_RRDHOST, "Host '%s': adding with guid '%s'", hostname, guid);

    if(storage_engine_id == STORAGE_ENGINE_DBENGINE && !rrdb.dbengine_enabled) {
        netdata_log_error("memory mode 'dbengine' is not enabled, but host '%s' is configured for it. Falling back to 'alloc'", hostname);
        storage_engine_id = STORAGE_ENGINE_ALLOC;
    }

#ifdef ENABLE_DBENGINE
    int is_legacy = (storage_engine_id == STORAGE_ENGINE_DBENGINE) && is_legacy_child(guid);
#else
int is_legacy = 1;
#endif

    int is_in_multihost = (storage_engine_id == STORAGE_ENGINE_DBENGINE && !is_legacy);
    RRDHOST *host = callocz(1, sizeof(RRDHOST));
    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(RRDHOST), __ATOMIC_RELAXED);

    strncpyz(host->machine_guid, guid, GUID_LEN + 1);

    set_host_properties(host, (update_every > 0)?update_every:1, storage_engine_id, registry_hostname, os,
                        tags, timezone, abbrev_timezone, utc_offset, program_name, program_version);

    rrdhost_init_hostname(host, hostname, false);

    host->rrd_history_entries        = align_entries_to_pagesize(storage_engine_id, entries);
    host->health.health_enabled      = ((storage_engine_id == STORAGE_ENGINE_NONE)) ? 0 : health_enabled;

    if (likely(!archived)) {
        rrdfunctions_init(host);
        host->rrdlabels = rrdlabels_create();
        rrdhost_initialize_rrdpush_sender(
            host, rrdpush_enabled, rrdpush_destination, rrdpush_api_key, rrdpush_send_charts_matching);
    }

    if(rrdpush_enable_replication)
        rrdhost_option_set(host, RRDHOST_OPTION_REPLICATION);
    else
        rrdhost_option_clear(host, RRDHOST_OPTION_REPLICATION);

    host->rrdpush_seconds_to_replicate = rrdpush_seconds_to_replicate;
    host->rrdpush_replication_step = rrdpush_replication_step;
    host->rrdpush_receiver_replication_percent = 100.0;

    switch(storage_engine_id) {
        case STORAGE_ENGINE_DBENGINE:
            break;
        case STORAGE_ENGINE_ALLOC:
        case STORAGE_ENGINE_MAP:
        case STORAGE_ENGINE_SAVE:
        case STORAGE_ENGINE_RAM:
        default:
            if(host->rrdpush_seconds_to_replicate > (time_t) host->rrd_history_entries * (time_t) host->update_every)
                host->rrdpush_seconds_to_replicate = (time_t) host->rrd_history_entries * (time_t) host->update_every;
            break;
    }

    netdata_mutex_init(&host->aclk_state_lock);
    netdata_mutex_init(&host->receiver_lock);

    host->system_info = system_info;

    rrdset_index_init(host);

    if(config_get_boolean(CONFIG_SECTION_DB, "delete obsolete charts files", 1))
        rrdhost_option_set(host, RRDHOST_OPTION_DELETE_OBSOLETE_CHARTS);

    if(config_get_boolean(CONFIG_SECTION_DB, "delete orphan hosts files", 1) && !is_localhost)
        rrdhost_option_set(host, RRDHOST_OPTION_DELETE_ORPHAN_HOST);

    char filename[FILENAME_MAX + 1];
    if(is_localhost)
        host->cache_dir  = strdupz(netdata_configured_cache_dir);
    else {
        // this is not localhost - append our GUID to localhost path
        if (is_in_multihost) { // don't append to cache dir in multihost
            host->cache_dir  = strdupz(netdata_configured_cache_dir);
        }
        else {
            snprintfz(filename, FILENAME_MAX, "%s/%s", netdata_configured_cache_dir, host->machine_guid);
            host->cache_dir = strdupz(filename);
        }

        if((host->storage_engine_id == STORAGE_ENGINE_MAP || host->storage_engine_id == STORAGE_ENGINE_SAVE ||
             (host->storage_engine_id == STORAGE_ENGINE_DBENGINE && is_legacy))) {
            int r = mkdir(host->cache_dir, 0775);
            if(r != 0 && errno != EEXIST)
                netdata_log_error("Host '%s': cannot create directory '%s'", rrdhost_hostname(host), host->cache_dir);
        }
    }

    // this is also needed for custom host variables - not only health
    if(!host->rrdvars)
        host->rrdvars = rrdvariables_create();

    if (likely(!uuid_parse(host->machine_guid, host->host_uuid)))
        sql_load_node_id(host);
    else
        error_report("Host machine GUID %s is not valid", host->machine_guid);

    rrdfamily_index_init(host);
    rrdcalctemplate_index_init(host);
    rrdcalc_rrdhost_index_init(host);

    if (host->storage_engine_id == STORAGE_ENGINE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        char dbenginepath[FILENAME_MAX + 1];
        int ret;

        snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", host->cache_dir);
        ret = mkdir(dbenginepath, 0775);

        if (ret != 0 && errno != EEXIST)
            netdata_log_error("Host '%s': cannot create directory '%s'", rrdhost_hostname(host), dbenginepath);
        else
            ret = 0; // succeed

        if (is_legacy) {
            // initialize legacy dbengine instance as needed

            host->db[0].id = STORAGE_ENGINE_DBENGINE;
            host->db[0].tier_grouping = get_tier_grouping(0);

             // may fail here for legacy dbengine initialization
            ret = rrdeng_init(&host->db[0].instance, dbenginepath, rrdb.default_rrdeng_disk_quota_mb, 0);

            if(ret == 0) {
                rrdeng_readiness_wait(host->db[0].instance);

                // assign the rest of the shared storage instances to it
                // to allow them collect its metrics too

                for(size_t tier = 1; tier < rrdb.storage_tiers ; tier++) {
                    host->db[tier].id= STORAGE_ENGINE_DBENGINE;
                    host->db[tier].instance = rrdb.multidb_ctx[tier];
                    host->db[tier].tier_grouping = get_tier_grouping(tier);
                }
            }
        }
        else {
            for(size_t tier = 0; tier < rrdb.storage_tiers ; tier++) {
                host->db[tier].id = STORAGE_ENGINE_DBENGINE;
                host->db[tier].instance = rrdb.multidb_ctx[tier];
                host->db[tier].tier_grouping = get_tier_grouping(tier);
            }
        }

        if (ret) { // check legacy or multihost initialization success
            netdata_log_error("Host '%s': cannot initialize host with machine guid '%s'. Failed to initialize DB engine at '%s'.",
                              rrdhost_hostname(host), host->machine_guid, host->cache_dir);

            rrd_wrlock();
            rrdhost_free___while_having_rrd_wrlock(host, true);
            rrd_unlock();

            return NULL;
        }

#else
        fatal("STORAGE_ENGINE_DBENGINE is not supported in this platform.");
#endif
    }
    else {
        host->db[0].id = host->storage_engine_id;
        host->db[0].instance = NULL;
        host->db[0].tier_grouping = get_tier_grouping(0);

#ifdef ENABLE_DBENGINE
        // the first tier is reserved for the non-dbengine modes
        for(size_t tier = 1; tier < rrdb.storage_tiers ; tier++) {
            host->db[tier].id = STORAGE_ENGINE_DBENGINE;
            host->db[tier].instance = rrdb.multidb_ctx[tier];
            host->db[tier].tier_grouping = get_tier_grouping(tier);
        }
#endif
    }

    // ------------------------------------------------------------------------
    // init new ML host and update system_info to let upstreams know
    // about ML functionality
    //

    if (is_localhost && host->system_info) {
        host->system_info->ml_capable = ml_capable();
        host->system_info->ml_enabled = ml_enabled(host);
        host->system_info->mc_version = enable_metric_correlations ? metric_correlations_version : 0;
    }

    // ------------------------------------------------------------------------
    // link it and add it to the index

    rrd_wrlock();

    RRDHOST *t = rrdhost_index_add_by_guid(host);
    if(t != host) {
        netdata_log_error("Host '%s': cannot add host with machine guid '%s' to index. It already exists as host '%s' with machine guid '%s'.",
                          rrdhost_hostname(host), host->machine_guid, rrdhost_hostname(t), t->machine_guid);
        rrdhost_free___while_having_rrd_wrlock(host, true);
        rrd_unlock();
        return NULL;
    }

    rrdhost_index_add_hostname(host);

    if(is_localhost)
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(rrdb.localhost, host, prev, next);
    else
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(rrdb.localhost, host, prev, next);

    rrd_unlock();

    // ------------------------------------------------------------------------

    netdata_log_info("Host '%s' (at registry as '%s') with guid '%s' initialized"
                 ", os '%s'"
                 ", timezone '%s'"
                 ", tags '%s'"
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
         , rrdhost_tags(host)
         , rrdhost_program_name(host)
         , rrdhost_program_version(host)
         , host->update_every
         , storage_engine_name(host->storage_engine_id)
         , host->rrd_history_entries
         , rrdhost_has_rrdpush_sender_enabled(host)?"enabled":"disabled"
         , host->rrdpush_send_destination?host->rrdpush_send_destination:""
         , host->rrdpush_send_api_key?host->rrdpush_send_api_key:""
         , host->health.health_enabled?"enabled":"disabled"
         , host->cache_dir
         , string2str(host->health.health_default_exec)
         , string2str(host->health.health_default_recipient)
    );

    if(!archived) {
        metaqueue_host_update_info(host);
        rrdhost_load_rrdcontext_data(host);
//        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_UPDATE);
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
                    , const char *tags
                    , const char *program_name
                    , const char *program_version
                    , int update_every
                    , long history
                    , STORAGE_ENGINE_ID storage_engine_id
                    , unsigned int health_enabled
                    , unsigned int rrdpush_enabled
                    , char *rrdpush_destination
                    , char *rrdpush_api_key
                    , char *rrdpush_send_charts_matching
                    , bool rrdpush_enable_replication
                    , time_t rrdpush_seconds_to_replicate
                    , time_t rrdpush_replication_step
                    , struct rrdhost_system_info *system_info
)
{
    UNUSED(guid);

    spinlock_lock(&host->rrdhost_update_lock);

    host->health.health_enabled = (storage_engine_id == STORAGE_ENGINE_NONE) ? 0 : health_enabled;

    {
        struct rrdhost_system_info *old = host->system_info;
        host->system_info = system_info;
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_CLAIMID | RRDHOST_FLAG_METADATA_UPDATE);
        rrdhost_system_info_free(old);
    }

    rrdhost_init_os(host, os);
    rrdhost_init_timezone(host, timezone, abbrev_timezone, utc_offset);

    string_freez(host->registry_hostname);
    host->registry_hostname = string_strdupz((registry_hostname && *registry_hostname)?registry_hostname:hostname);

    if(strcmp(rrdhost_hostname(host), hostname) != 0) {
        netdata_log_info("Host '%s' has been renamed to '%s'. If this is not intentional it may mean multiple hosts are using the same machine_guid.", rrdhost_hostname(host), hostname);
        rrdhost_init_hostname(host, hostname, true);
    } else {
        rrdhost_index_add_hostname(host);
    }

    if(strcmp(rrdhost_program_name(host), program_name) != 0) {
        netdata_log_info("Host '%s' switched program name from '%s' to '%s'", rrdhost_hostname(host), rrdhost_program_name(host), program_name);
        STRING *t = host->program_name;
        host->program_name = string_strdupz(program_name);
        string_freez(t);
    }

    if(strcmp(rrdhost_program_version(host), program_version) != 0) {
        netdata_log_info("Host '%s' switched program version from '%s' to '%s'", rrdhost_hostname(host), rrdhost_program_version(host), program_version);
        STRING *t = host->program_version;
        host->program_version = string_strdupz(program_version);
        string_freez(t);
    }

    if(host->update_every != update_every)
        netdata_log_error("Host '%s' has an update frequency of %d seconds, but the wanted one is %d seconds. "
                          "Restart netdata here to apply the new settings.",
                          rrdhost_hostname(host), host->update_every, update_every);

    if(host->storage_engine_id != storage_engine_id) {
        netdata_log_error("Host '%s' has memory mode '%s', but the wanted one is '%s'. "
                          "Restart netdata here to apply the new settings.",
                          rrdhost_hostname(host), storage_engine_name(host->storage_engine_id), storage_engine_name(storage_engine_id));
    } else if(host->storage_engine_id != STORAGE_ENGINE_DBENGINE && host->rrd_history_entries < history) {
        netdata_log_error("Host '%s' has history of %d entries, but the wanted one is %ld entries. "
                          "Restart netdata here to apply the new settings.",
                          rrdhost_hostname(host), host->rrd_history_entries, history);
    }

    // update host tags
    rrdhost_init_tags(host, tags);

    if(!host->rrdvars)
        host->rrdvars = rrdvariables_create();

    if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_ARCHIVED);

        rrdfunctions_init(host);

        if(!host->rrdlabels)
            host->rrdlabels = rrdlabels_create();

        if (!host->rrdset_root_index)
            rrdset_index_init(host);

        rrdhost_initialize_rrdpush_sender(host,
                                   rrdpush_enabled,
                                   rrdpush_destination,
                                   rrdpush_api_key,
                                   rrdpush_send_charts_matching);

        rrdfamily_index_init(host);
        rrdcalctemplate_index_init(host);
        rrdcalc_rrdhost_index_init(host);

        if(rrdpush_enable_replication)
            rrdhost_option_set(host, RRDHOST_OPTION_REPLICATION);
        else
            rrdhost_option_clear(host, RRDHOST_OPTION_REPLICATION);

        host->rrdpush_seconds_to_replicate = rrdpush_seconds_to_replicate;
        host->rrdpush_replication_step = rrdpush_replication_step;

        ml_host_new(host);
        
        rrdhost_load_rrdcontext_data(host);
        netdata_log_info("Host %s is not in archived mode anymore", rrdhost_hostname(host));
    }

    spinlock_unlock(&host->rrdhost_update_lock);
}

RRDHOST *rrdhost_get_or_create(
          const char *hostname
        , const char *registry_hostname
        , const char *guid
        , const char *os
        , const char *timezone
        , const char *abbrev_timezone
        , int32_t utc_offset
        , const char *tags
        , const char *program_name
        , const char *program_version
        , int update_every
        , long history
        , STORAGE_ENGINE_ID storage_engine_id
        , unsigned int health_enabled
        , unsigned int rrdpush_enabled
        , char *rrdpush_destination
        , char *rrdpush_api_key
        , char *rrdpush_send_charts_matching
        , bool rrdpush_enable_replication
        , time_t rrdpush_seconds_to_replicate
        , time_t rrdpush_replication_step
        , struct rrdhost_system_info *system_info
        , bool archived
) {
    netdata_log_debug(D_RRDHOST, "Searching for host '%s' with guid '%s'", hostname, guid);

    RRDHOST *host = rrdhost_find_by_guid(guid);
    if (unlikely(host && host->storage_engine_id != storage_engine_id && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) {

        if (likely(!archived && rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD)))
            return host;

        /* If a legacy storage engine instantiates all dbengine state must be discarded to avoid inconsistencies */
        netdata_log_error("Archived host '%s' has memory mode '%s', but the wanted one is '%s'. Discarding archived state.",
                          rrdhost_hostname(host),
                          storage_engine_name(host->storage_engine_id),
                          storage_engine_name(storage_engine_id));

        rrd_wrlock();
        rrdhost_free___while_having_rrd_wrlock(host, true);
        host = NULL;
        rrd_unlock();
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
                , tags
                , program_name
                , program_version
                , update_every
                , history
                , storage_engine_id
                , health_enabled
                , rrdpush_enabled
                , rrdpush_destination
                , rrdpush_api_key
                , rrdpush_send_charts_matching
                , rrdpush_enable_replication
                , rrdpush_seconds_to_replicate
                , rrdpush_replication_step
                , system_info
                , 0
                , archived
        );
    }
    else {
        if (likely(!rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD)))
            rrdhost_update(host
               , hostname
               , registry_hostname
               , guid
               , os
               , timezone
               , abbrev_timezone
               , utc_offset
               , tags
               , program_name
               , program_version
               , update_every
               , history
               , storage_engine_id
               , health_enabled
               , rrdpush_enabled
               , rrdpush_destination
               , rrdpush_api_key
               , rrdpush_send_charts_matching
               , rrdpush_enable_replication
               , rrdpush_seconds_to_replicate
               , rrdpush_replication_step
               , system_info);
    }

    return host;
}

inline int rrdhost_should_be_removed(RRDHOST *host, RRDHOST *protected_host, time_t now_s) {
    if(host != protected_host
       && host != rrdb.localhost
       && rrdhost_receiver_replicating_charts(host) == 0
       && rrdhost_sender_replicating_charts(host) == 0
       && rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN)
       && !rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)
       && !host->receiver
       && host->child_disconnected_time
       && host->child_disconnected_time + rrdb.rrdhost_free_orphan_time_s < now_s)
        return 1;

    return 0;
}

// ----------------------------------------------------------------------------
// RRDHOST - free

void rrdhost_system_info_free(struct rrdhost_system_info *system_info) {
    if(likely(system_info)) {
        __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);

        freez(system_info->cloud_provider_type);
        freez(system_info->cloud_instance_type);
        freez(system_info->cloud_instance_region);
        freez(system_info->host_os_name);
        freez(system_info->host_os_id);
        freez(system_info->host_os_id_like);
        freez(system_info->host_os_version);
        freez(system_info->host_os_version_id);
        freez(system_info->host_os_detection);
        freez(system_info->host_cores);
        freez(system_info->host_cpu_freq);
        freez(system_info->host_ram_total);
        freez(system_info->host_disk_space);
        freez(system_info->container_os_name);
        freez(system_info->container_os_id);
        freez(system_info->container_os_id_like);
        freez(system_info->container_os_version);
        freez(system_info->container_os_version_id);
        freez(system_info->container_os_detection);
        freez(system_info->kernel_name);
        freez(system_info->kernel_version);
        freez(system_info->architecture);
        freez(system_info->virtualization);
        freez(system_info->virt_detection);
        freez(system_info->container);
        freez(system_info->container_detection);
        freez(system_info->is_k8s_node);
        freez(system_info->install_type);
        freez(system_info->prebuilt_arch);
        freez(system_info->prebuilt_dist);
        freez(system_info);
    }
}

static void rrdhost_streaming_sender_structures_init(RRDHOST *host)
{
    if (host->sender)
        return;

    host->sender = callocz(1, sizeof(*host->sender));
    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(*host->sender), __ATOMIC_RELAXED);

    host->sender->host = host;
    host->sender->buffer = cbuffer_new(CBUFFER_INITIAL_SIZE, 1024 * 1024, &netdata_buffers_statistics.cbuffers_streaming);
    host->sender->capabilities = stream_our_capabilities(host, true);

    host->sender->rrdpush_sender_pipe[PIPE_READ] = -1;
    host->sender->rrdpush_sender_pipe[PIPE_WRITE] = -1;
    host->sender->rrdpush_sender_socket  = -1;

#ifdef ENABLE_RRDPUSH_COMPRESSION
    if(default_rrdpush_compression_enabled)
        host->sender->flags |= SENDER_FLAG_COMPRESSION;
    else
        host->sender->flags &= ~SENDER_FLAG_COMPRESSION;
#endif

    spinlock_init(&host->sender->spinlock);
    replication_init_sender(host->sender);
}

static void rrdhost_streaming_sender_structures_free(RRDHOST *host)
{
    rrdhost_option_clear(host, RRDHOST_OPTION_SENDER_ENABLED);

    if (unlikely(!host->sender))
        return;

    rrdpush_sender_thread_stop(host, STREAM_HANDSHAKE_DISCONNECT_HOST_CLEANUP, true); // stop a possibly running thread
    cbuffer_free(host->sender->buffer);
#ifdef ENABLE_RRDPUSH_COMPRESSION
    rrdpush_compressor_destroy(&host->sender->compressor);
#endif
    replication_cleanup_sender(host->sender);

    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(*host->sender), __ATOMIC_RELAXED);

    freez(host->sender);
    host->sender = NULL;
    rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_INITIALIZED);
}

void rrdhost_free___while_having_rrd_wrlock(RRDHOST *host, bool force) {
    if(!host) return;

    if (netdata_exit || force) {
        netdata_log_info("RRD: 'host:%s' freeing memory...", rrdhost_hostname(host));

        // ------------------------------------------------------------------------
        // first remove it from the indexes, so that it will not be discoverable

        rrdhost_index_del_hostname(host);
        rrdhost_index_del_by_guid(host);

        if (host->prev)
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(rrdb.localhost, host, prev, next);
    }

    // ------------------------------------------------------------------------
    // clean up streaming

    rrdhost_streaming_sender_structures_free(host);

    if (netdata_exit || force)
        stop_streaming_receiver(host, STREAM_HANDSHAKE_DISCONNECT_HOST_CLEANUP);


    // ------------------------------------------------------------------------
    // clean up alarms

    rrdcalc_delete_all(host);

    // ------------------------------------------------------------------------
    // release its children resources

#ifdef ENABLE_DBENGINE
    for(size_t tier = 0; tier < rrdb.storage_tiers ;tier++) {
        if(host->db[tier].id == STORAGE_ENGINE_DBENGINE
            && host->db[tier].instance
            && !is_storage_engine_shared(host->db[tier].instance))
            rrdeng_prepare_exit(host->db[tier].instance);
    }
#endif

    // delete all the RRDSETs of the host
    rrdset_index_destroy(host);
    rrdcalc_rrdhost_index_destroy(host);
    rrdcalctemplate_index_destroy(host);

    // cleanup ML resources
    ml_host_delete(host);

    freez(host->exporting_flags);

    health_alarm_log_free(host);

#ifdef ENABLE_DBENGINE
    for(size_t tier = 0; tier < rrdb.storage_tiers ;tier++) {
        if(host->db[tier].id == STORAGE_ENGINE_DBENGINE
            && host->db[tier].instance
            && !is_storage_engine_shared(host->db[tier].instance))
            rrdeng_exit(host->db[tier].instance);
    }
#endif

    if (!netdata_exit && !force) {
        netdata_log_info("RRD: 'host:%s' is now in archive mode...", rrdhost_hostname(host));
        rrdhost_flag_set(host, RRDHOST_FLAG_ARCHIVED | RRDHOST_FLAG_ORPHAN);
        return;
    }

    // ------------------------------------------------------------------------
    // free it

    pthread_mutex_destroy(&host->aclk_state_lock);
    freez(host->aclk_state.claimed_id);
    freez(host->aclk_state.prev_claimed_id);
    string_freez(host->tags);
    rrdlabels_destroy(host->rrdlabels);
    string_freez(host->os);
    string_freez(host->timezone);
    string_freez(host->abbrev_timezone);
    string_freez(host->program_name);
    string_freez(host->program_version);
    rrdhost_system_info_free(host->system_info);
    freez(host->cache_dir);
    freez(host->rrdpush_send_api_key);
    freez(host->rrdpush_send_destination);
    rrdpush_destinations_free(host);
    string_freez(host->health.health_default_exec);
    string_freez(host->health.health_default_recipient);
    string_freez(host->registry_hostname);
    simple_pattern_free(host->rrdpush_send_charts_matching);
    freez(host->node_id);

    rrdfamily_index_destroy(host);
    rrdfunctions_destroy(host);
    rrdvariables_destroy(host->rrdvars);
    if (host == rrdb.localhost)
        rrdvariables_destroy(health_rrdvars);

    rrdhost_destroy_rrdcontexts(host);

    string_freez(host->hostname);
    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(RRDHOST), __ATOMIC_RELAXED);
    freez(host);
}

void rrdhost_free_all(void) {
    rrd_wrlock();

    /* Make sure child-hosts are released before the localhost. */
    while (rrdb.localhost && rrdb.localhost->next)
        rrdhost_free___while_having_rrd_wrlock(rrdb.localhost->next, true);

    if (rrdb.localhost)
        rrdhost_free___while_having_rrd_wrlock(rrdb.localhost, true);

    rrd_unlock();
}

void rrd_finalize_collection_for_all_hosts(void) {
    RRDHOST *host;
    dfe_start_reentrant(rrdb.rrdhost_root_index, host) {
        rrdhost_finalize_collection(host);
    }
    dfe_done(host);
}

// ----------------------------------------------------------------------------
// RRDHOST - save host files

void rrdhost_save_charts(RRDHOST *host) {
    if(!host) return;

    netdata_log_info("RRD: 'host:%s' saving / closing database...", rrdhost_hostname(host));

    RRDSET *st;

    // we get a write lock
    // to ensure only one thread is saving the database
    rrdset_foreach_write(st, host) {
        rrdset_save(st);
    }
    rrdset_foreach_done(st);
}

void rrdhost_set_is_parent_label(void) {
    int count = __atomic_load_n(&rrdb.localhost->connected_children_count, __ATOMIC_RELAXED);

    if (count == 0 || count == 1) {
        DICTIONARY *labels = rrdb.localhost->rrdlabels;
        rrdlabels_add(labels, "_is_parent", (count) ? "true" : "false", RRDLABEL_SRC_AUTO);

        //queue a node info
#ifdef ENABLE_ACLK
        if (netdata_cloud_enabled) {
            aclk_queue_node_info(rrdb.localhost, false);
        }
#endif
    }
}

void rrdhost_finalize_collection(RRDHOST *host) {
    netdata_log_info("RRD: 'host:%s' stopping data collection...", rrdhost_hostname(host));

    RRDSET *st;
    rrdset_foreach_read(st, host)
        rrdset_finalize_collection(st, true);
    rrdset_foreach_done(st);
}

// ----------------------------------------------------------------------------
// RRDHOST - delete host files

void rrdhost_delete_charts(RRDHOST *host) {
    if(!host) return;

    netdata_log_info("RRD: 'host:%s' deleting disk files...", rrdhost_hostname(host));

    RRDSET *st;

    if(host->storage_engine_id == STORAGE_ENGINE_SAVE || host->storage_engine_id == STORAGE_ENGINE_MAP) {
        // we get a write lock
        // to ensure only one thread is saving the database
        rrdset_foreach_write(st, host){
                    rrdset_delete_files(st);
                }
        rrdset_foreach_done(st);
    }

    recursively_delete_dir(host->cache_dir, "left over host");
}

// ----------------------------------------------------------------------------
// RRDHOST - cleanup host files

static void delete_obsolete_dimensions(RRDSET *st) {
    RRDDIM *rd;

    netdata_log_info("Deleting dimensions of chart '%s' ('%s') from disk...", rrdset_id(st), rrdset_name(st));

    rrddim_foreach_read(rd, st) {
        if(rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
            const char *cache_filename = rrddim_cache_filename(rd);
            if(!cache_filename) continue;
            netdata_log_info("Deleting dimension file '%s'.", cache_filename);
            if(unlikely(unlink(cache_filename) == -1))
                netdata_log_error("Cannot delete dimension file '%s'", cache_filename);
        }
    }
    rrddim_foreach_done(rd);
}

void rrdhost_cleanup_charts(RRDHOST *host) {
    if(!host) return;

    netdata_log_info("RRD: 'host:%s' cleaning up disk files...", rrdhost_hostname(host));

    RRDSET *st;
    uint32_t rrdhost_delete_obsolete_charts = rrdhost_option_check(host, RRDHOST_OPTION_DELETE_OBSOLETE_CHARTS);

    // we get a write lock
    // to ensure only one thread is saving the database
    rrdset_foreach_write(st, host) {

        if(rrdhost_delete_obsolete_charts && rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE))
            rrdset_delete_files(st);

        else if(rrdhost_delete_obsolete_charts && rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS))
            delete_obsolete_dimensions(st);

        else
            rrdset_save(st);

    }
    rrdset_foreach_done(st);
}


// ----------------------------------------------------------------------------
// RRDHOST - save all hosts to disk

void rrdhost_save_all(void) {
    netdata_log_info("RRD: saving databases [%zu hosts(s)]...", rrdhost_hosts_available());

    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host)
        rrdhost_save_charts(host);

    rrd_unlock();
}

// ----------------------------------------------------------------------------
// RRDHOST - save or delete all hosts from disk

void rrdhost_cleanup_all(void) {
    netdata_log_info("RRD: cleaning up database [%zu hosts(s)]...", rrdhost_hosts_available());

    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host) {
        if (host != rrdb.localhost && rrdhost_option_check(host, RRDHOST_OPTION_DELETE_ORPHAN_HOST) && !host->receiver
            /* don't delete multi-host DB host files */
            && !(host->storage_engine_id == STORAGE_ENGINE_DBENGINE && is_storage_engine_shared(host->db[0].instance))
        )
            rrdhost_delete_charts(host);
        else
            rrdhost_cleanup_charts(host);
    }

    rrd_unlock();
}


// ----------------------------------------------------------------------------
// RRDHOST - set system info from environment variables
// system_info fields must be heap allocated or NULL
int rrdhost_set_system_info_variable(struct rrdhost_system_info *system_info, char *name, char *value) {
    int res = 0;

    if (!strcmp(name, "NETDATA_PROTOCOL_VERSION"))
        return res;
    else if(!strcmp(name, "NETDATA_INSTANCE_CLOUD_TYPE")){
        freez(system_info->cloud_provider_type);
        system_info->cloud_provider_type = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_INSTANCE_CLOUD_INSTANCE_TYPE")){
        freez(system_info->cloud_instance_type);
        system_info->cloud_instance_type = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_INSTANCE_CLOUD_INSTANCE_REGION")){
        freez(system_info->cloud_instance_region);
        system_info->cloud_instance_region = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_NAME")){
        freez(system_info->container_os_name);
        system_info->container_os_name = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_ID")){
        freez(system_info->container_os_id);
        system_info->container_os_id = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_ID_LIKE")){
        freez(system_info->container_os_id_like);
        system_info->container_os_id_like = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_VERSION")){
        freez(system_info->container_os_version);
        system_info->container_os_version = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_VERSION_ID")){
        freez(system_info->container_os_version_id);
        system_info->container_os_version_id = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_CONTAINER_OS_DETECTION")){
        freez(system_info->container_os_detection);
        system_info->container_os_detection = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_NAME")){
        freez(system_info->host_os_name);
        system_info->host_os_name = strdupz(value);
        json_fix_string(system_info->host_os_name);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_ID")){
        freez(system_info->host_os_id);
        system_info->host_os_id = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_ID_LIKE")){
        freez(system_info->host_os_id_like);
        system_info->host_os_id_like = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_VERSION")){
        freez(system_info->host_os_version);
        system_info->host_os_version = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_VERSION_ID")){
        freez(system_info->host_os_version_id);
        system_info->host_os_version_id = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_OS_DETECTION")){
        freez(system_info->host_os_detection);
        system_info->host_os_detection = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_KERNEL_NAME")){
        freez(system_info->kernel_name);
        system_info->kernel_name = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_CPU_LOGICAL_CPU_COUNT")){
        freez(system_info->host_cores);
        system_info->host_cores = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_CPU_FREQ")){
        freez(system_info->host_cpu_freq);
        system_info->host_cpu_freq = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_TOTAL_RAM")){
        freez(system_info->host_ram_total);
        system_info->host_ram_total = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_TOTAL_DISK_SIZE")){
        freez(system_info->host_disk_space);
        system_info->host_disk_space = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_KERNEL_VERSION")){
        freez(system_info->kernel_version);
        system_info->kernel_version = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_ARCHITECTURE")){
        freez(system_info->architecture);
        system_info->architecture = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_VIRTUALIZATION")){
        freez(system_info->virtualization);
        system_info->virtualization = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_VIRT_DETECTION")){
        freez(system_info->virt_detection);
        system_info->virt_detection = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_CONTAINER")){
        freez(system_info->container);
        system_info->container = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_SYSTEM_CONTAINER_DETECTION")){
        freez(system_info->container_detection);
        system_info->container_detection = strdupz(value);
    }
    else if(!strcmp(name, "NETDATA_HOST_IS_K8S_NODE")){
        freez(system_info->is_k8s_node);
        system_info->is_k8s_node = strdupz(value);
    }
    else if (!strcmp(name, "NETDATA_SYSTEM_CPU_VENDOR"))
        return res;
    else if (!strcmp(name, "NETDATA_SYSTEM_CPU_MODEL"))
        return res;
    else if (!strcmp(name, "NETDATA_SYSTEM_CPU_DETECTION"))
        return res;
    else if (!strcmp(name, "NETDATA_SYSTEM_RAM_DETECTION"))
        return res;
    else if (!strcmp(name, "NETDATA_SYSTEM_DISK_DETECTION"))
        return res;
    else if (!strcmp(name, "NETDATA_CONTAINER_IS_OFFICIAL_IMAGE"))
        return res;
    else {
        res = 1;
    }

    return res;
}

static NETDATA_DOUBLE rrdhost_sender_replication_completion_unsafe(RRDHOST *host, time_t now, size_t *instances) {
    size_t charts = rrdhost_sender_replicating_charts(host);
    NETDATA_DOUBLE completion;
    if(!charts || !host->sender || !host->sender->replication.oldest_request_after_t)
        completion = 100.0;
    else if(!host->sender->replication.latest_completed_before_t || host->sender->replication.latest_completed_before_t < host->sender->replication.oldest_request_after_t)
        completion = 0.0;
    else {
        time_t total = now - host->sender->replication.oldest_request_after_t;
        time_t current = host->sender->replication.latest_completed_before_t - host->sender->replication.oldest_request_after_t;
        completion = (NETDATA_DOUBLE) current * 100.0 / (NETDATA_DOUBLE) total;
    }

    *instances = charts;

    return completion;
}

static void rrdhost_retention(RRDHOST *host, time_t now, bool online, time_t *from, time_t *to)
{
    spinlock_lock(&host->retention.spinlock);
    time_t first_time_s = host->retention.first_time_s;
    time_t last_time_s = host->retention.last_time_s;
    spinlock_unlock(&host->retention.spinlock);

    if (from)
        *from = first_time_s;

    if (to)
        *to = online ? now : last_time_s;
}

bool rrdhost_matches_window(RRDHOST *host, time_t after, time_t before, time_t now) {
    time_t first_time_s, last_time_s;
    rrdhost_retention(host, now, rrdhost_is_online(host), &first_time_s, &last_time_s);
    return query_matches_retention(after, before, first_time_s, last_time_s, 0);
}

bool rrdhost_state_cloud_emulation(RRDHOST *host) {
    return rrdhost_is_online(host);
}

void rrdhost_status(RRDHOST *host, time_t now, RRDHOST_STATUS *s) {
    memset(s, 0, sizeof(*s));

    s->host = host;
    s->now = now;

    RRDHOST_FLAGS flags = __atomic_load_n(&host->flags, __ATOMIC_RELAXED);

    // --- db ---

    bool online = rrdhost_is_online(host);

    rrdhost_retention(host, now, online, &s->db.first_time_s, &s->db.last_time_s);
    s->db.metrics = host->rrdctx.metrics;
    s->db.instances = host->rrdctx.instances;
    s->db.contexts = dictionary_entries(host->rrdctx.contexts);
    if(!s->db.first_time_s || !s->db.last_time_s || !s->db.metrics || !s->db.instances || !s->db.contexts ||
            (flags & (RRDHOST_FLAG_PENDING_CONTEXT_LOAD|RRDHOST_FLAG_CONTEXT_LOAD_IN_PROGRESS)))
        s->db.status = RRDHOST_DB_STATUS_INITIALIZING;
    else
        s->db.status = RRDHOST_DB_STATUS_QUERYABLE;

    s->db.storage_engine_id = host->storage_engine_id;

    // --- ingest ---

    s->ingest.since = MAX(host->child_connect_time, host->child_disconnected_time);
    s->ingest.reason = (online) ? STREAM_HANDSHAKE_NEVER : host->rrdpush_last_receiver_exit_reason;

    netdata_mutex_lock(&host->receiver_lock);
    s->ingest.hops = (host->system_info ? host->system_info->hops : (host == rrdb.localhost) ? 0 : 1);
    bool has_receiver = false;
    if (host->receiver) {
        has_receiver = true;
        s->ingest.replication.instances = rrdhost_receiver_replicating_charts(host);
        s->ingest.replication.completion = host->rrdpush_receiver_replication_percent;
        s->ingest.replication.in_progress = s->ingest.replication.instances > 0;

        s->ingest.capabilities = host->receiver->capabilities;
        s->ingest.peers = socket_peers(host->receiver->fd);
#ifdef ENABLE_HTTPS
        s->ingest.ssl = SSL_connection(&host->receiver->ssl);
#endif
    }
    netdata_mutex_unlock(&host->receiver_lock);

    if (online) {
        if(s->db.status == RRDHOST_DB_STATUS_INITIALIZING)
            s->ingest.status = RRDHOST_INGEST_STATUS_INITIALIZING;

        else if (host == rrdb.localhost || rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST)) {
            s->ingest.status = RRDHOST_INGEST_STATUS_ONLINE;
            s->ingest.since = netdata_start_time;
        }

        else if (s->ingest.replication.in_progress)
            s->ingest.status = RRDHOST_INGEST_STATUS_REPLICATING;

        else
            s->ingest.status = RRDHOST_INGEST_STATUS_ONLINE;
    }
    else {
        if (!s->ingest.since) {
            s->ingest.status = RRDHOST_INGEST_STATUS_ARCHIVED;
            s->ingest.since = s->db.last_time_s;
        }

        else
            s->ingest.status = RRDHOST_INGEST_STATUS_OFFLINE;
    }

    if(host == rrdb.localhost)
        s->ingest.type = RRDHOST_INGEST_TYPE_LOCALHOST;
    else if(has_receiver || rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED))
        s->ingest.type = RRDHOST_INGEST_TYPE_CHILD;
    else if(rrdhost_option_check(host, RRDHOST_OPTION_VIRTUAL_HOST))
        s->ingest.type = RRDHOST_INGEST_TYPE_VIRTUAL;
    else
        s->ingest.type = RRDHOST_INGEST_TYPE_ARCHIVED;

    s->ingest.id = host->rrdpush_receiver_connection_counter;

    if(!s->ingest.since)
        s->ingest.since = netdata_start_time;

    if(s->ingest.status == RRDHOST_INGEST_STATUS_ONLINE)
        s->db.liveness = RRDHOST_DB_LIVENESS_LIVE;
    else
        s->db.liveness = RRDHOST_DB_LIVENESS_STALE;

    // --- stream ---

    if (!host->sender) {
        s->stream.status = RRDHOST_STREAM_STATUS_DISABLED;
        s->stream.hops = s->ingest.hops + 1;
    }
    else {
        sender_lock(host->sender);

        s->stream.since = host->sender->last_state_since_t;
        s->stream.peers = socket_peers(host->sender->rrdpush_sender_socket);
        s->stream.ssl = SSL_connection(&host->sender->ssl);

        memcpy(s->stream.sent_bytes_on_this_connection_per_type,
               host->sender->sent_bytes_on_this_connection_per_type,
            MIN(sizeof(s->stream.sent_bytes_on_this_connection_per_type),
                sizeof(host->sender->sent_bytes_on_this_connection_per_type)));

        if (rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_CONNECTED)) {
            s->stream.hops = host->sender->hops;
            s->stream.reason = STREAM_HANDSHAKE_NEVER;
            s->stream.capabilities = host->sender->capabilities;

            s->stream.replication.completion = rrdhost_sender_replication_completion_unsafe(host, now, &s->stream.replication.instances);
            s->stream.replication.in_progress = s->stream.replication.instances > 0;

            if(s->stream.replication.in_progress)
                s->stream.status = RRDHOST_STREAM_STATUS_REPLICATING;
            else
                s->stream.status = RRDHOST_STREAM_STATUS_ONLINE;

#ifdef ENABLE_RRDPUSH_COMPRESSION
            s->stream.compression = (stream_has_capability(host->sender, STREAM_CAP_COMPRESSION) && host->sender->compressor.initialized);
#endif
        }
        else {
            s->stream.status = RRDHOST_STREAM_STATUS_OFFLINE;
            s->stream.hops = s->ingest.hops + 1;
            s->stream.reason = host->sender->exit.reason;
        }

        sender_unlock(host->sender);
    }

    s->stream.id = host->rrdpush_sender_connection_counter;

    if(!s->stream.since)
        s->stream.since = netdata_start_time;

    // --- ml ---

    if(ml_host_get_host_status(host, &s->ml.metrics)) {
        s->ml.type = RRDHOST_ML_TYPE_SELF;

        if(s->ingest.status == RRDHOST_INGEST_STATUS_OFFLINE || s->ingest.status == RRDHOST_INGEST_STATUS_ARCHIVED)
            s->ml.status = RRDHOST_ML_STATUS_OFFLINE;
        else
            s->ml.status = RRDHOST_ML_STATUS_RUNNING;
    }
    else if(stream_has_capability(&s->ingest, STREAM_CAP_DATA_WITH_ML)) {
        s->ml.type = RRDHOST_ML_TYPE_RECEIVED;
        s->ml.status = RRDHOST_ML_STATUS_RUNNING;
    }
    else {
        // does not receive ML, does not run ML
        s->ml.type = RRDHOST_ML_TYPE_DISABLED;
        s->ml.status = RRDHOST_ML_STATUS_DISABLED;
    }

    // --- health ---

    if(host->health.health_enabled) {
        if(flags & RRDHOST_FLAG_PENDING_HEALTH_INITIALIZATION)
            s->health.status = RRDHOST_HEALTH_STATUS_INITIALIZING;
        else {
            s->health.status = RRDHOST_HEALTH_STATUS_RUNNING;

            RRDCALC *rc;
            foreach_rrdcalc_in_rrdhost_read(host, rc) {
                if (unlikely(!rc->rrdset || !rc->rrdset->last_collected_time.tv_sec))
                    continue;

                switch (rc->status) {
                    default:
                    case RRDCALC_STATUS_REMOVED:
                        break;

                    case RRDCALC_STATUS_CLEAR:
                        s->health.alerts.clear++;
                        break;

                    case RRDCALC_STATUS_WARNING:
                        s->health.alerts.warning++;
                        break;

                    case RRDCALC_STATUS_CRITICAL:
                        s->health.alerts.critical++;
                        break;

                    case RRDCALC_STATUS_UNDEFINED:
                        s->health.alerts.undefined++;
                        break;

                    case RRDCALC_STATUS_UNINITIALIZED:
                        s->health.alerts.uninitialized++;
                        break;
                }
            }
            foreach_rrdcalc_in_rrdhost_done(rc);
        }
    }
    else
        s->health.status = RRDHOST_HEALTH_STATUS_DISABLED;
}
