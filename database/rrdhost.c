// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"

static void rrdhost_streaming_sender_structures_init(RRDHOST *host);

bool dbengine_enabled = false; // will become true if and when dbengine is initialized
size_t storage_tiers = 3;
bool use_direct_io = true;
size_t storage_tiers_grouping_iterations[RRD_STORAGE_TIERS] = { 1, 60, 60, 60, 60 };
RRD_BACKFILL storage_tiers_backfill[RRD_STORAGE_TIERS] = { RRD_BACKFILL_NEW, RRD_BACKFILL_NEW, RRD_BACKFILL_NEW, RRD_BACKFILL_NEW, RRD_BACKFILL_NEW };

#if RRD_STORAGE_TIERS != 5
#error RRD_STORAGE_TIERS is not 5 - you need to update the grouping iterations per tier
#endif

size_t get_tier_grouping(size_t tier) {
    if(unlikely(tier >= storage_tiers)) tier = storage_tiers - 1;

    size_t grouping = 1;
    // first tier is always 1 iteration of whatever update every the chart has
    for(size_t i = 1; i <= tier ;i++)
        grouping *= storage_tiers_grouping_iterations[i];

    return grouping;
}

RRDHOST *localhost = NULL;
netdata_rwlock_t rrd_rwlock = NETDATA_RWLOCK_INITIALIZER;

time_t rrdset_free_obsolete_time_s = 3600;
time_t rrdhost_free_orphan_time_s = 3600;

bool is_storage_engine_shared(STORAGE_INSTANCE *engine __maybe_unused) {
#ifdef ENABLE_DBENGINE
    if(!rrdeng_is_legacy(engine))
        return true;
#endif

    return false;
}

RRDHOST *find_host_by_node_id(char *node_id) {
    uuid_t node_uuid;
    if (unlikely(!node_id || uuid_parse(node_id, node_uuid)))
        return NULL;

    RRDHOST *host, *ret = NULL;
    dfe_start_read(rrdhost_root_index, host) {
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

DICTIONARY *rrdhost_root_index = NULL;
static DICTIONARY *rrdhost_root_index_hostname = NULL;

static inline void rrdhost_init() {
    if(unlikely(!rrdhost_root_index)) {
        rrdhost_root_index = dictionary_create_advanced(
            DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE,
            &dictionary_stats_category_rrdhost, 0);
    }

    if(unlikely(!rrdhost_root_index_hostname)) {
        rrdhost_root_index_hostname = dictionary_create_advanced(
            DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE,
            &dictionary_stats_category_rrdhost, 0);
    }
}

RRDHOST_ACQUIRED *rrdhost_find_and_acquire(const char *machine_guid) {
    debug(D_RRD_CALLS, "rrdhost_find_and_acquire() host %s", machine_guid);

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
    RRDHOST *ret_machine_guid = dictionary_set(rrdhost_root_index, host->machine_guid, host, sizeof(RRDHOST));
    if(ret_machine_guid == host)
        rrdhost_option_set(host, RRDHOST_OPTION_INDEXED_MACHINE_GUID);
    else {
        rrdhost_option_clear(host, RRDHOST_OPTION_INDEXED_MACHINE_GUID);
        error("RRDHOST: %s() host with machine guid '%s' is already indexed", __FUNCTION__, host->machine_guid);
    }

    return host;
}

static void rrdhost_index_del_by_guid(RRDHOST *host) {
    if(rrdhost_option_check(host, RRDHOST_OPTION_INDEXED_MACHINE_GUID)) {
        if(!dictionary_del(rrdhost_root_index, host->machine_guid))
            error("RRDHOST: %s() failed to delete machine guid '%s' from index", __FUNCTION__, host->machine_guid);

        rrdhost_option_clear(host, RRDHOST_OPTION_INDEXED_MACHINE_GUID);
    }
}

// ----------------------------------------------------------------------------
// RRDHOST index by hostname

inline RRDHOST *rrdhost_find_by_hostname(const char *hostname) {
    if(unlikely(!strcmp(hostname, "localhost")))
        return localhost;

    return dictionary_get(rrdhost_root_index_hostname, hostname);
}

static inline void rrdhost_index_del_hostname(RRDHOST *host) {
    if(unlikely(!host->hostname)) return;

    if(rrdhost_option_check(host, RRDHOST_OPTION_INDEXED_HOSTNAME)) {
        if(!dictionary_del(rrdhost_root_index_hostname, rrdhost_hostname(host)))
            error("RRDHOST: %s() failed to delete hostname '%s' from index", __FUNCTION__, rrdhost_hostname(host));

        rrdhost_option_clear(host, RRDHOST_OPTION_INDEXED_HOSTNAME);
    }
}

static inline RRDHOST *rrdhost_index_add_hostname(RRDHOST *host) {
    if(!host->hostname) return host;

    RRDHOST *ret_hostname = dictionary_set(rrdhost_root_index_hostname, rrdhost_hostname(host), host, sizeof(RRDHOST));
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

void set_host_properties(RRDHOST *host, int update_every, RRD_MEMORY_MODE memory_mode,
                         const char *registry_hostname, const char *os, const char *tags,
                         const char *tzone, const char *abbrev_tzone, int32_t utc_offset, const char *program_name,
                         const char *program_version)
{

    host->rrd_update_every = update_every;
    host->rrd_memory_mode = memory_mode;

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

static RRDHOST *rrdhost_create(
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
        RRD_MEMORY_MODE memory_mode,
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
    debug(D_RRDHOST, "Host '%s': adding with guid '%s'", hostname, guid);

    if(memory_mode == RRD_MEMORY_MODE_DBENGINE && !dbengine_enabled) {
        error("memory mode 'dbengine' is not enabled, but host '%s' is configured for it. Falling back to 'alloc'", hostname);
        memory_mode = RRD_MEMORY_MODE_ALLOC;
    }

#ifdef ENABLE_DBENGINE
    int is_legacy = (memory_mode == RRD_MEMORY_MODE_DBENGINE) && is_legacy_child(guid);
#else
int is_legacy = 1;
#endif

    int is_in_multihost = (memory_mode == RRD_MEMORY_MODE_DBENGINE && !is_legacy);
    RRDHOST *host = callocz(1, sizeof(RRDHOST));
    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(RRDHOST), __ATOMIC_RELAXED);

    strncpyz(host->machine_guid, guid, GUID_LEN + 1);

    set_host_properties(host, (update_every > 0)?update_every:1, memory_mode, registry_hostname, os,
                        tags, timezone, abbrev_timezone, utc_offset, program_name, program_version);

    rrdhost_init_hostname(host, hostname, false);

    host->rrd_history_entries        = align_entries_to_pagesize(memory_mode, entries);
    host->health.health_enabled      = ((memory_mode == RRD_MEMORY_MODE_NONE)) ? 0 : health_enabled;

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

    switch(memory_mode) {
        default:
        case RRD_MEMORY_MODE_ALLOC:
        case RRD_MEMORY_MODE_MAP:
        case RRD_MEMORY_MODE_SAVE:
        case RRD_MEMORY_MODE_RAM:
            if(host->rrdpush_seconds_to_replicate > host->rrd_history_entries * host->rrd_update_every)
                host->rrdpush_seconds_to_replicate = host->rrd_history_entries * host->rrd_update_every;
            break;

        case RRD_MEMORY_MODE_DBENGINE:
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

        if((host->rrd_memory_mode == RRD_MEMORY_MODE_MAP || host->rrd_memory_mode == RRD_MEMORY_MODE_SAVE ||
             (host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && is_legacy))) {
            int r = mkdir(host->cache_dir, 0775);
            if(r != 0 && errno != EEXIST)
                error("Host '%s': cannot create directory '%s'", rrdhost_hostname(host), host->cache_dir);
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

    if (host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        char dbenginepath[FILENAME_MAX + 1];
        int ret;

        snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", host->cache_dir);
        ret = mkdir(dbenginepath, 0775);

        if (ret != 0 && errno != EEXIST)
            error("Host '%s': cannot create directory '%s'", rrdhost_hostname(host), dbenginepath);
        else
            ret = 0; // succeed

        if (is_legacy) {
            // initialize legacy dbengine instance as needed

            host->db[0].mode = RRD_MEMORY_MODE_DBENGINE;
            host->db[0].eng = storage_engine_get(host->db[0].mode);
            host->db[0].tier_grouping = get_tier_grouping(0);

            ret = rrdeng_init(
                (struct rrdengine_instance **)&host->db[0].instance,
                dbenginepath,
                default_rrdeng_disk_quota_mb,
                0); // may fail here for legacy dbengine initialization

            if(ret == 0) {
                rrdeng_readiness_wait((struct rrdengine_instance *)host->db[0].instance);

                // assign the rest of the shared storage instances to it
                // to allow them collect its metrics too

                for(size_t tier = 1; tier < storage_tiers ; tier++) {
                    host->db[tier].mode = RRD_MEMORY_MODE_DBENGINE;
                    host->db[tier].eng = storage_engine_get(host->db[tier].mode);
                    host->db[tier].instance = (STORAGE_INSTANCE *) multidb_ctx[tier];
                    host->db[tier].tier_grouping = get_tier_grouping(tier);
                }
            }
        }
        else {
            for(size_t tier = 0; tier < storage_tiers ; tier++) {
                host->db[tier].mode = RRD_MEMORY_MODE_DBENGINE;
                host->db[tier].eng = storage_engine_get(host->db[tier].mode);
                host->db[tier].instance = (STORAGE_INSTANCE *)multidb_ctx[tier];
                host->db[tier].tier_grouping = get_tier_grouping(tier);
            }
        }

        if (ret) { // check legacy or multihost initialization success
            error(
                "Host '%s': cannot initialize host with machine guid '%s'. Failed to initialize DB engine at '%s'.",
                rrdhost_hostname(host), host->machine_guid, host->cache_dir);

            rrd_wrlock();
            rrdhost_free___while_having_rrd_wrlock(host, true);
            rrd_unlock();

            return NULL;
        }

#else
        fatal("RRD_MEMORY_MODE_DBENGINE is not supported in this platform.");
#endif
    }
    else {
        host->db[0].mode = host->rrd_memory_mode;
        host->db[0].eng = storage_engine_get(host->db[0].mode);
        host->db[0].instance = NULL;
        host->db[0].tier_grouping = get_tier_grouping(0);

#ifdef ENABLE_DBENGINE
        // the first tier is reserved for the non-dbengine modes
        for(size_t tier = 1; tier < storage_tiers ; tier++) {
            host->db[tier].mode = RRD_MEMORY_MODE_DBENGINE;
            host->db[tier].eng = storage_engine_get(host->db[tier].mode);
            host->db[tier].instance = (STORAGE_INSTANCE *) multidb_ctx[tier];
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
        error("Host '%s': cannot add host with machine guid '%s' to index. It already exists as host '%s' with machine guid '%s'.", rrdhost_hostname(host), host->machine_guid, rrdhost_hostname(t), t->machine_guid);
        rrdhost_free___while_having_rrd_wrlock(host, true);
        rrd_unlock();
        return NULL;
    }

    rrdhost_index_add_hostname(host);

    if(is_localhost)
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(localhost, host, prev, next);
    else
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(localhost, host, prev, next);

    rrd_unlock();

    // ------------------------------------------------------------------------

    info("Host '%s' (at registry as '%s') with guid '%s' initialized"
                 ", os '%s'"
                 ", timezone '%s'"
                 ", tags '%s'"
                 ", program_name '%s'"
                 ", program_version '%s'"
                 ", update every %d"
                 ", memory mode %s"
                 ", history entries %ld"
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
         , host->rrd_update_every
         , rrd_memory_mode_name(host->rrd_memory_mode)
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
                    , RRD_MEMORY_MODE mode
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

    netdata_spinlock_lock(&host->rrdhost_update_lock);

    host->health.health_enabled = (mode == RRD_MEMORY_MODE_NONE) ? 0 : health_enabled;

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
        info("Host '%s' has been renamed to '%s'. If this is not intentional it may mean multiple hosts are using the same machine_guid.", rrdhost_hostname(host), hostname);
        rrdhost_init_hostname(host, hostname, true);
    } else {
        rrdhost_index_add_hostname(host);
    }

    if(strcmp(rrdhost_program_name(host), program_name) != 0) {
        info("Host '%s' switched program name from '%s' to '%s'", rrdhost_hostname(host), rrdhost_program_name(host), program_name);
        STRING *t = host->program_name;
        host->program_name = string_strdupz(program_name);
        string_freez(t);
    }

    if(strcmp(rrdhost_program_version(host), program_version) != 0) {
        info("Host '%s' switched program version from '%s' to '%s'", rrdhost_hostname(host), rrdhost_program_version(host), program_version);
        STRING *t = host->program_version;
        host->program_version = string_strdupz(program_version);
        string_freez(t);
    }

    if(host->rrd_update_every != update_every)
        error("Host '%s' has an update frequency of %d seconds, but the wanted one is %d seconds. Restart netdata here to apply the new settings.", rrdhost_hostname(host), host->rrd_update_every, update_every);

    if(host->rrd_memory_mode != mode)
        error("Host '%s' has memory mode '%s', but the wanted one is '%s'. Restart netdata here to apply the new settings.", rrdhost_hostname(host), rrd_memory_mode_name(host->rrd_memory_mode), rrd_memory_mode_name(mode));

    else if(host->rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE && host->rrd_history_entries < history)
        error("Host '%s' has history of %ld entries, but the wanted one is %ld entries. Restart netdata here to apply the new settings.", rrdhost_hostname(host), host->rrd_history_entries, history);

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
        info("Host %s is not in archived mode anymore", rrdhost_hostname(host));
    }

    netdata_spinlock_unlock(&host->rrdhost_update_lock);
}

RRDHOST *rrdhost_find_or_create(
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
        , RRD_MEMORY_MODE mode
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
    debug(D_RRDHOST, "Searching for host '%s' with guid '%s'", hostname, guid);

    RRDHOST *host = rrdhost_find_by_guid(guid);
    if (unlikely(host && host->rrd_memory_mode != mode && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) {

        if (likely(!archived && rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD)))
            return host;

        /* If a legacy memory mode instantiates all dbengine state must be discarded to avoid inconsistencies */
        error("Archived host '%s' has memory mode '%s', but the wanted one is '%s'. Discarding archived state.",
              rrdhost_hostname(host), rrd_memory_mode_name(host->rrd_memory_mode), rrd_memory_mode_name(mode));

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
                , mode
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
               , mode
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
       && host != localhost
       && rrdhost_receiver_replicating_charts(host) == 0
       && rrdhost_sender_replicating_charts(host) == 0
       && rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN)
       && !rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)
       && !host->receiver
       && host->child_disconnected_time
       && host->child_disconnected_time + rrdhost_free_orphan_time_s < now_s)
        return 1;

    return 0;
}

// ----------------------------------------------------------------------------
// RRDHOST global / startup initialization

#ifdef ENABLE_DBENGINE
struct dbengine_initialization {
    netdata_thread_t thread;
    char path[FILENAME_MAX + 1];
    int disk_space_mb;
    size_t tier;
    int ret;
};

void *dbengine_tier_init(void *ptr) {
    struct dbengine_initialization *dbi = ptr;
    dbi->ret = rrdeng_init(NULL, dbi->path, dbi->disk_space_mb, dbi->tier);
    return ptr;
}
#endif

void dbengine_init(char *hostname) {
#ifdef ENABLE_DBENGINE
    use_direct_io = config_get_boolean(CONFIG_SECTION_DB, "dbengine use direct io", use_direct_io);

    unsigned read_num = (unsigned)config_get_number(CONFIG_SECTION_DB, "dbengine pages per extent", MAX_PAGES_PER_EXTENT);
    if (read_num > 0 && read_num <= MAX_PAGES_PER_EXTENT)
        rrdeng_pages_per_extent = read_num;
    else {
        error("Invalid dbengine pages per extent %u given. Using %u.", read_num, rrdeng_pages_per_extent);
        config_set_number(CONFIG_SECTION_DB, "dbengine pages per extent", rrdeng_pages_per_extent);
    }

    storage_tiers = config_get_number(CONFIG_SECTION_DB, "storage tiers", storage_tiers);
    if(storage_tiers < 1) {
        error("At least 1 storage tier is required. Assuming 1.");
        storage_tiers = 1;
        config_set_number(CONFIG_SECTION_DB, "storage tiers", storage_tiers);
    }
    if(storage_tiers > RRD_STORAGE_TIERS) {
        error("Up to %d storage tier are supported. Assuming %d.", RRD_STORAGE_TIERS, RRD_STORAGE_TIERS);
        storage_tiers = RRD_STORAGE_TIERS;
        config_set_number(CONFIG_SECTION_DB, "storage tiers", storage_tiers);
    }

    bool parallel_initialization = (storage_tiers <= (size_t)get_netdata_cpus()) ? true : false;
    parallel_initialization = config_get_boolean(CONFIG_SECTION_DB, "dbengine parallel initialization", parallel_initialization);

    struct dbengine_initialization tiers_init[RRD_STORAGE_TIERS] = {};

    size_t created_tiers = 0;
    char dbenginepath[FILENAME_MAX + 1];
    char dbengineconfig[200 + 1];
    int divisor = 1;
    for(size_t tier = 0; tier < storage_tiers ;tier++) {
        if(tier == 0)
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", netdata_configured_cache_dir);
        else
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine-tier%zu", netdata_configured_cache_dir, tier);

        int ret = mkdir(dbenginepath, 0775);
        if (ret != 0 && errno != EEXIST) {
            error("DBENGINE on '%s': cannot create directory '%s'", hostname, dbenginepath);
            break;
        }

        if(tier > 0)
            divisor *= 2;

        int disk_space_mb = default_multidb_disk_quota_mb / divisor;
        size_t grouping_iterations = storage_tiers_grouping_iterations[tier];
        RRD_BACKFILL backfill = storage_tiers_backfill[tier];

        if(tier > 0) {
            snprintfz(dbengineconfig, 200, "dbengine tier %zu multihost disk space MB", tier);
            disk_space_mb = config_get_number(CONFIG_SECTION_DB, dbengineconfig, disk_space_mb);

            snprintfz(dbengineconfig, 200, "dbengine tier %zu update every iterations", tier);
            grouping_iterations = config_get_number(CONFIG_SECTION_DB, dbengineconfig, grouping_iterations);
            if(grouping_iterations < 2) {
                grouping_iterations = 2;
                config_set_number(CONFIG_SECTION_DB, dbengineconfig, grouping_iterations);
                error("DBENGINE on '%s': 'dbegnine tier %zu update every iterations' cannot be less than 2. Assuming 2.", hostname, tier);
            }

            snprintfz(dbengineconfig, 200, "dbengine tier %zu backfill", tier);
            const char *bf = config_get(CONFIG_SECTION_DB, dbengineconfig, backfill == RRD_BACKFILL_NEW ? "new" : backfill == RRD_BACKFILL_FULL ? "full" : "none");
            if(strcmp(bf, "new") == 0) backfill = RRD_BACKFILL_NEW;
            else if(strcmp(bf, "full") == 0) backfill = RRD_BACKFILL_FULL;
            else if(strcmp(bf, "none") == 0) backfill = RRD_BACKFILL_NONE;
            else {
                error("DBENGINE: unknown backfill value '%s', assuming 'new'", bf);
                config_set(CONFIG_SECTION_DB, dbengineconfig, "new");
                backfill = RRD_BACKFILL_NEW;
            }
        }

        storage_tiers_grouping_iterations[tier] = grouping_iterations;
        storage_tiers_backfill[tier] = backfill;

        if(tier > 0 && get_tier_grouping(tier) > 65535) {
            storage_tiers_grouping_iterations[tier] = 1;
            error("DBENGINE on '%s': dbengine tier %zu gives aggregation of more than 65535 points of tier 0. Disabling tiers above %zu", hostname, tier, tier);
            break;
        }

        internal_error(true, "DBENGINE tier %zu grouping iterations is set to %zu", tier, storage_tiers_grouping_iterations[tier]);

        tiers_init[tier].disk_space_mb = disk_space_mb;
        tiers_init[tier].tier = tier;
        strncpyz(tiers_init[tier].path, dbenginepath, FILENAME_MAX);
        tiers_init[tier].ret = 0;

        if(parallel_initialization)
            netdata_thread_create(&tiers_init[tier].thread, "DBENGINE_INIT", NETDATA_THREAD_OPTION_JOINABLE,
                                  dbengine_tier_init, &tiers_init[tier]);
        else
            dbengine_tier_init(&tiers_init[tier]);
    }

    for(size_t tier = 0; tier < storage_tiers ;tier++) {
        void *ptr;

        if(parallel_initialization)
            netdata_thread_join(tiers_init[tier].thread, &ptr);

        if(tiers_init[tier].ret != 0) {
            error("DBENGINE on '%s': Failed to initialize multi-host database tier %zu on path '%s'",
                  hostname, tiers_init[tier].tier, tiers_init[tier].path);
        }
        else if(created_tiers == tier)
            created_tiers++;
    }

    if(created_tiers && created_tiers < storage_tiers) {
        error("DBENGINE on '%s': Managed to create %zu tiers instead of %zu. Continuing with %zu available.",
              hostname, created_tiers, storage_tiers, created_tiers);
        storage_tiers = created_tiers;
    }
    else if(!created_tiers)
        fatal("DBENGINE on '%s', failed to initialize databases at '%s'.", hostname, netdata_configured_cache_dir);

    for(size_t tier = 0; tier < storage_tiers ;tier++)
        rrdeng_readiness_wait(multidb_ctx[tier]);

    dbengine_enabled = true;
#else
    storage_tiers = config_get_number(CONFIG_SECTION_DB, "storage tiers", 1);
    if(storage_tiers != 1) {
        error("DBENGINE is not available on '%s', so only 1 database tier can be supported.", hostname);
        storage_tiers = 1;
        config_set_number(CONFIG_SECTION_DB, "storage tiers", storage_tiers);
    }
    dbengine_enabled = false;
#endif
}

int rrd_init(char *hostname, struct rrdhost_system_info *system_info, bool unittest) {
    rrdhost_init();

    if (unlikely(sql_init_database(DB_CHECK_NONE, system_info ? 0 : 1))) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
            set_late_global_environment(system_info);
            fatal("Failed to initialize SQLite");
        }
        info("Skipping SQLITE metadata initialization since memory mode is not dbengine");
    }

    if (unlikely(sql_init_context_database(system_info ? 0 : 1))) {
        error_report("Failed to initialize context metadata database");
    }

    if (unlikely(unittest)) {
        dbengine_enabled = true;
    }
    else {
        health_init();
        rrdpush_init();

        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE || rrdpush_receiver_needs_dbengine()) {
            info("DBENGINE: Initializing ...");
            dbengine_init(hostname);
        }
        else {
            info("DBENGINE: Not initializing ...");
            storage_tiers = 1;
        }

        if (!dbengine_enabled) {
            if (storage_tiers > 1) {
                error("dbengine is not enabled, but %zu tiers have been requested. Resetting tiers to 1",
                      storage_tiers);
                storage_tiers = 1;
            }

            if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
                error("dbengine is not enabled, but it has been given as the default db mode. Resetting db mode to alloc");
                default_rrd_memory_mode = RRD_MEMORY_MODE_ALLOC;
            }
        }
    }

    if(!unittest)
        metadata_sync_init();

    debug(D_RRDHOST, "Initializing localhost with hostname '%s'", hostname);
    localhost = rrdhost_create(
            hostname
            , registry_get_this_machine_hostname()
            , registry_get_this_machine_guid()
            , os_type
            , netdata_configured_timezone
            , netdata_configured_abbrev_timezone
            , netdata_configured_utc_offset
            , ""
            , program_name
            , program_version
            , default_rrd_update_every
            , default_rrd_history_entries
            , default_rrd_memory_mode
            , default_health_enabled
            , default_rrdpush_enabled
            , default_rrdpush_destination
            , default_rrdpush_api_key
            , default_rrdpush_send_charts_matching
            , default_rrdpush_enable_replication
            , default_rrdpush_seconds_to_replicate
            , default_rrdpush_replication_step
            , system_info
            , 1
            , 0
    );

    if (unlikely(!localhost)) {
        return 1;
    }

    if (likely(system_info)) {
        migrate_localhost(&localhost->host_uuid);
        sql_aclk_sync_init();
        web_client_api_v1_management_init();
    }
    return localhost==NULL;
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
    host->sender->capabilities = stream_our_capabilities();

    host->sender->rrdpush_sender_pipe[PIPE_READ] = -1;
    host->sender->rrdpush_sender_pipe[PIPE_WRITE] = -1;
    host->sender->rrdpush_sender_socket  = -1;

#ifdef ENABLE_COMPRESSION
    if(default_compression_enabled) {
        host->sender->flags |= SENDER_FLAG_COMPRESSION;
        host->sender->compressor = create_compressor();
    }
    else
        host->sender->flags &= ~SENDER_FLAG_COMPRESSION;
#endif

    netdata_mutex_init(&host->sender->mutex);
    replication_init_sender(host->sender);
}

static void rrdhost_streaming_sender_structures_free(RRDHOST *host)
{
    rrdhost_option_clear(host, RRDHOST_OPTION_SENDER_ENABLED);

    if (unlikely(!host->sender))
        return;

    rrdpush_sender_thread_stop(host, "HOST CLEANUP", true); // stop a possibly running thread
    cbuffer_free(host->sender->buffer);
#ifdef ENABLE_COMPRESSION
    if (host->sender->compressor)
        host->sender->compressor->destroy(&host->sender->compressor);
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
        info("RRD: 'host:%s' freeing memory...", rrdhost_hostname(host));

        // ------------------------------------------------------------------------
        // first remove it from the indexes, so that it will not be discoverable

        rrdhost_index_del_hostname(host);
        rrdhost_index_del_by_guid(host);

        if (host->prev)
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(localhost, host, prev, next);
    }

    // ------------------------------------------------------------------------
    // clean up streaming

    rrdhost_streaming_sender_structures_free(host);

    if (netdata_exit || force)
        stop_streaming_receiver(host, "HOST CLEANUP");


    // ------------------------------------------------------------------------
    // clean up alarms

    rrdcalc_delete_all(host);

    // ------------------------------------------------------------------------
    // release its children resources

#ifdef ENABLE_DBENGINE
    for(size_t tier = 0; tier < storage_tiers ;tier++) {
        if(host->db[tier].mode == RRD_MEMORY_MODE_DBENGINE
            && host->db[tier].instance
            && !is_storage_engine_shared(host->db[tier].instance))
            rrdeng_prepare_exit((struct rrdengine_instance *)host->db[tier].instance);
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
    for(size_t tier = 0; tier < storage_tiers ;tier++) {
        if(host->db[tier].mode == RRD_MEMORY_MODE_DBENGINE
            && host->db[tier].instance
            && !is_storage_engine_shared(host->db[tier].instance))
            rrdeng_exit((struct rrdengine_instance *)host->db[tier].instance);
    }
#endif

    if (!netdata_exit && !force) {
        info("RRD: 'host:%s' is now in archive mode...", rrdhost_hostname(host));
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
    netdata_rwlock_destroy(&host->health_log.alarm_log_rwlock);
    freez(host->node_id);

    rrdfamily_index_destroy(host);
    rrdfunctions_destroy(host);
    rrdvariables_destroy(host->rrdvars);
    if (host == localhost)
        rrdvariables_destroy(health_rrdvars);

    rrdhost_destroy_rrdcontexts(host);

    string_freez(host->hostname);
    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(RRDHOST), __ATOMIC_RELAXED);
    freez(host);
}

void rrdhost_free_all(void) {
    rrd_wrlock();

    /* Make sure child-hosts are released before the localhost. */
    while(localhost && localhost->next)
        rrdhost_free___while_having_rrd_wrlock(localhost->next, true);

    if(localhost)
        rrdhost_free___while_having_rrd_wrlock(localhost, true);

    rrd_unlock();
}

void rrd_finalize_collection_for_all_hosts(void) {
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host) {
        rrdhost_finalize_collection(host);
    }
    dfe_done(host);
}

// ----------------------------------------------------------------------------
// RRDHOST - save host files

void rrdhost_save_charts(RRDHOST *host) {
    if(!host) return;

    info("RRD: 'host:%s' saving / closing database...", rrdhost_hostname(host));

    RRDSET *st;

    // we get a write lock
    // to ensure only one thread is saving the database
    rrdset_foreach_write(st, host) {
        rrdset_save(st);
    }
    rrdset_foreach_done(st);
}

struct rrdhost_system_info *rrdhost_labels_to_system_info(DICTIONARY *labels) {
    struct rrdhost_system_info *info = callocz(1, sizeof(struct rrdhost_system_info));
    info->hops = 1;

    rrdlabels_get_value_strdup_or_null(labels, &info->cloud_provider_type, "_cloud_provider_type");
    rrdlabels_get_value_strdup_or_null(labels, &info->cloud_instance_type, "_cloud_instance_type");
    rrdlabels_get_value_strdup_or_null(labels, &info->cloud_instance_region, "_cloud_instance_region");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_os_name, "_os_name");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_os_version, "_os_version");
    rrdlabels_get_value_strdup_or_null(labels, &info->kernel_version, "_kernel_version");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_cores, "_system_cores");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_cpu_freq, "_system_cpu_freq");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_ram_total, "_system_ram_total");
    rrdlabels_get_value_strdup_or_null(labels, &info->host_disk_space, "_system_disk_space");
    rrdlabels_get_value_strdup_or_null(labels, &info->architecture, "_architecture");
    rrdlabels_get_value_strdup_or_null(labels, &info->virtualization, "_virtualization");
    rrdlabels_get_value_strdup_or_null(labels, &info->container, "_container");
    rrdlabels_get_value_strdup_or_null(labels, &info->container_detection, "_container_detection");
    rrdlabels_get_value_strdup_or_null(labels, &info->virt_detection, "_virt_detection");
    rrdlabels_get_value_strdup_or_null(labels, &info->is_k8s_node, "_is_k8s_node");
    rrdlabels_get_value_strdup_or_null(labels, &info->install_type, "_install_type");
    rrdlabels_get_value_strdup_or_null(labels, &info->prebuilt_arch, "_prebuilt_arch");
    rrdlabels_get_value_strdup_or_null(labels, &info->prebuilt_dist, "_prebuilt_dist");

    return info;
}

static void rrdhost_load_auto_labels(void) {
    DICTIONARY *labels = localhost->rrdlabels;

    if (localhost->system_info->cloud_provider_type)
        rrdlabels_add(labels, "_cloud_provider_type", localhost->system_info->cloud_provider_type, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->cloud_instance_type)
        rrdlabels_add(labels, "_cloud_instance_type", localhost->system_info->cloud_instance_type, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->cloud_instance_region)
        rrdlabels_add(labels, "_cloud_instance_region", localhost->system_info->cloud_instance_region, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->host_os_name)
        rrdlabels_add(labels, "_os_name", localhost->system_info->host_os_name, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->host_os_version)
        rrdlabels_add(labels, "_os_version", localhost->system_info->host_os_version, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->kernel_version)
        rrdlabels_add(labels, "_kernel_version", localhost->system_info->kernel_version, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->host_cores)
        rrdlabels_add(labels, "_system_cores", localhost->system_info->host_cores, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->host_cpu_freq)
        rrdlabels_add(labels, "_system_cpu_freq", localhost->system_info->host_cpu_freq, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->host_ram_total)
        rrdlabels_add(labels, "_system_ram_total", localhost->system_info->host_ram_total, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->host_disk_space)
        rrdlabels_add(labels, "_system_disk_space", localhost->system_info->host_disk_space, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->architecture)
        rrdlabels_add(labels, "_architecture", localhost->system_info->architecture, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->virtualization)
        rrdlabels_add(labels, "_virtualization", localhost->system_info->virtualization, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->container)
        rrdlabels_add(labels, "_container", localhost->system_info->container, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->container_detection)
        rrdlabels_add(labels, "_container_detection", localhost->system_info->container_detection, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->virt_detection)
        rrdlabels_add(labels, "_virt_detection", localhost->system_info->virt_detection, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->is_k8s_node)
        rrdlabels_add(labels, "_is_k8s_node", localhost->system_info->is_k8s_node, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->install_type)
        rrdlabels_add(labels, "_install_type", localhost->system_info->install_type, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->prebuilt_arch)
        rrdlabels_add(labels, "_prebuilt_arch", localhost->system_info->prebuilt_arch, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->prebuilt_dist)
        rrdlabels_add(labels, "_prebuilt_dist", localhost->system_info->prebuilt_dist, RRDLABEL_SRC_AUTO);

    add_aclk_host_labels();

    health_add_host_labels();

    rrdlabels_add(labels, "_is_parent", (localhost->connected_children_count > 0) ? "true" : "false", RRDLABEL_SRC_AUTO);

    if (localhost->rrdpush_send_destination)
        rrdlabels_add(labels, "_streams_to", localhost->rrdpush_send_destination, RRDLABEL_SRC_AUTO);
}

void rrdhost_set_is_parent_label(int count) {
    DICTIONARY *labels = localhost->rrdlabels;

    if (count == 0 || count == 1) {
        rrdlabels_add(labels, "_is_parent", (count) ? "true" : "false", RRDLABEL_SRC_AUTO);

        //queue a node info
#ifdef ENABLE_ACLK
        if (netdata_cloud_setting) {
            aclk_queue_node_info(localhost, false);
        }
#endif
    }
}

static void rrdhost_load_config_labels(void) {
    int status = config_load(NULL, 1, CONFIG_SECTION_HOST_LABEL);
    if(!status) {
        char *filename = CONFIG_DIR "/" CONFIG_FILENAME;
        error("RRDLABEL: Cannot reload the configuration file '%s', using labels in memory", filename);
    }

    struct section *co = appconfig_get_section(&netdata_config, CONFIG_SECTION_HOST_LABEL);
    if(co) {
        config_section_wrlock(co);
        struct config_option *cv;
        for(cv = co->values; cv ; cv = cv->next) {
            rrdlabels_add(localhost->rrdlabels, cv->name, cv->value, RRDLABEL_SRC_CONFIG);
            cv->flags |= CONFIG_VALUE_USED;
        }
        config_section_unlock(co);
    }
}

static void rrdhost_load_kubernetes_labels(void) {
    char label_script[sizeof(char) * (strlen(netdata_configured_primary_plugins_dir) + strlen("get-kubernetes-labels.sh") + 2)];
    sprintf(label_script, "%s/%s", netdata_configured_primary_plugins_dir, "get-kubernetes-labels.sh");

    if (unlikely(access(label_script, R_OK) != 0)) {
        error("Kubernetes pod label fetching script %s not found.",label_script);
        return;
    }

    debug(D_RRDHOST, "Attempting to fetch external labels via %s", label_script);

    pid_t pid;
    FILE *fp_child_input;
    FILE *fp_child_output = netdata_popen(label_script, &pid, &fp_child_input);
    if(!fp_child_output) return;

    char buffer[1000 + 1];
    while (fgets(buffer, 1000, fp_child_output) != NULL)
        rrdlabels_add_pair(localhost->rrdlabels, buffer, RRDLABEL_SRC_AUTO|RRDLABEL_SRC_K8S);

    // Non-zero exit code means that all the script output is error messages. We've shown already any message that didn't include a ':'
    // Here we'll inform with an ERROR that the script failed, show whatever (if anything) was added to the list of labels, free the memory and set the return to null
    int rc = netdata_pclose(fp_child_input, fp_child_output, pid);
    if(rc) error("%s exited abnormally. Failed to get kubernetes labels.", label_script);
}

void reload_host_labels(void) {
    if(!localhost->rrdlabels)
        localhost->rrdlabels = rrdlabels_create();

    rrdlabels_unmark_all(localhost->rrdlabels);

    // priority is important here
    rrdhost_load_config_labels();
    rrdhost_load_kubernetes_labels();
    rrdhost_load_auto_labels();

    rrdhost_flag_set(localhost,RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE);

    rrdpush_send_host_labels(localhost);
}

void rrdhost_finalize_collection(RRDHOST *host) {
    info("RRD: 'host:%s' stopping data collection...", rrdhost_hostname(host));

    RRDSET *st;
    rrdset_foreach_read(st, host)
        rrdset_finalize_collection(st, true);
    rrdset_foreach_done(st);
}

// ----------------------------------------------------------------------------
// RRDHOST - delete host files

void rrdhost_delete_charts(RRDHOST *host) {
    if(!host) return;

    info("RRD: 'host:%s' deleting disk files...", rrdhost_hostname(host));

    RRDSET *st;

    if(host->rrd_memory_mode == RRD_MEMORY_MODE_SAVE || host->rrd_memory_mode == RRD_MEMORY_MODE_MAP) {
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

void rrdhost_cleanup_charts(RRDHOST *host) {
    if(!host) return;

    info("RRD: 'host:%s' cleaning up disk files...", rrdhost_hostname(host));

    RRDSET *st;
    uint32_t rrdhost_delete_obsolete_charts = rrdhost_option_check(host, RRDHOST_OPTION_DELETE_OBSOLETE_CHARTS);

    // we get a write lock
    // to ensure only one thread is saving the database
    rrdset_foreach_write(st, host) {

        if(rrdhost_delete_obsolete_charts && rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE))
            rrdset_delete_files(st);

        else if(rrdhost_delete_obsolete_charts && rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS))
            rrdset_delete_obsolete_dimensions(st);

        else
            rrdset_save(st);

    }
    rrdset_foreach_done(st);
}


// ----------------------------------------------------------------------------
// RRDHOST - save all hosts to disk

void rrdhost_save_all(void) {
    info("RRD: saving databases [%zu hosts(s)]...", rrdhost_hosts_available());

    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host)
        rrdhost_save_charts(host);

    rrd_unlock();
}

// ----------------------------------------------------------------------------
// RRDHOST - save or delete all hosts from disk

void rrdhost_cleanup_all(void) {
    info("RRD: cleaning up database [%zu hosts(s)]...", rrdhost_hosts_available());

    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host) {
        if (host != localhost && rrdhost_option_check(host, RRDHOST_OPTION_DELETE_ORPHAN_HOST) && !host->receiver
            /* don't delete multi-host DB host files */
            && !(host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && is_storage_engine_shared(host->db[0].instance))
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
