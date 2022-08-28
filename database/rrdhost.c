// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"

int storage_tiers = 1;
int storage_tiers_grouping_iterations[RRD_STORAGE_TIERS] = { 1, 60, 60, 60, 60 };
RRD_BACKFILL storage_tiers_backfill[RRD_STORAGE_TIERS] = { RRD_BACKFILL_NEW, RRD_BACKFILL_NEW, RRD_BACKFILL_NEW, RRD_BACKFILL_NEW, RRD_BACKFILL_NEW };

#if RRD_STORAGE_TIERS != 5
#error RRD_STORAGE_TIERS is not 5 - you need to update the grouping iterations per tier
#endif

int get_tier_grouping(int tier) {
    if(unlikely(tier >= storage_tiers)) tier = storage_tiers - 1;
    if(unlikely(tier < 0)) tier = 0;

    int grouping = 1;
    // first tier is always 1 iteration of whatever update every the chart has
    for(int i = 1; i <= tier ;i++)
        grouping *= storage_tiers_grouping_iterations[i];

    return grouping;
}

RRDHOST *localhost = NULL;
size_t rrd_hosts_available = 0;
netdata_rwlock_t rrd_rwlock = NETDATA_RWLOCK_INITIALIZER;

time_t rrdset_free_obsolete_time = 3600;
time_t rrdhost_free_orphan_time = 3600;

bool is_storage_engine_shared(STORAGE_INSTANCE *engine) {
#ifdef ENABLE_DBENGINE
    for(int tier = 0; tier < storage_tiers ;tier++) {
        if (engine == (STORAGE_INSTANCE *)multidb_ctx[tier])
            return true;
    }
#endif

    return false;
}


// ----------------------------------------------------------------------------
// RRDHOST index

static DICTIONARY *rrdhost_root_index = NULL;
static DICTIONARY *rrdhost_root_index_hostname = NULL;

static inline void rrdhost_init() {
    if(unlikely(!rrdhost_root_index)) {
        rrdhost_root_index = dictionary_create(
              DICTIONARY_FLAG_NAME_LINK_DONT_CLONE
            | DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE
            | DICTIONARY_FLAG_DONT_OVERWRITE_VALUE
            );
    }

    if(unlikely(!rrdhost_root_index_hostname)) {
        rrdhost_root_index_hostname = dictionary_create(
              DICTIONARY_FLAG_NAME_LINK_DONT_CLONE
            | DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE
            | DICTIONARY_FLAG_DONT_OVERWRITE_VALUE
            );
    }
}

inline RRDHOST *rrdhost_find_by_guid(const char *guid) {
    return dictionary_get(rrdhost_root_index, guid);
}

inline RRDHOST *rrdhost_find_by_hostname(const char *hostname) {
    if(unlikely(!strcmp(hostname, "localhost")))
        return localhost;

    return dictionary_get(rrdhost_root_index_hostname, hostname);
}

inline long rrdhost_hosts_available(void) {
    return dictionary_stats_entries(rrdhost_root_index);
}

static inline RRDHOST *rrdhost_index_add(RRDHOST *host) {
    RRDHOST *ret_machine_guid = dictionary_set(rrdhost_root_index, host->machine_guid, host, sizeof(RRDHOST));
    if(ret_machine_guid == host)
        rrdhost_flag_set(host, RRDHOST_FLAG_INDEXED_MACHINE_GUID);
    else
        error("RRDHOST: host with machine guid '%s' is already indexed", host->machine_guid);

    return host;
}

static inline RRDHOST *rrdhost_index_del(RRDHOST *host) {
    if(rrdhost_flag_check(host, RRDHOST_FLAG_INDEXED_MACHINE_GUID)) {
        if(dictionary_del(rrdhost_root_index, host->machine_guid) !=  0)
            error("RRDHOST: failed to delete machine guid '%s' from index", host->machine_guid);

        rrdhost_flag_clear(host, RRDHOST_FLAG_INDEXED_MACHINE_GUID);
    }

    return host;
}

static inline RRDHOST *rrdhost_index_add_hostname(RRDHOST *host) {
    if(!host->hostname) return host;

    RRDHOST *ret_hostname = dictionary_set(rrdhost_root_index_hostname, rrdhost_hostname(host), host, sizeof(RRDHOST));
    if(ret_hostname == host)
        rrdhost_flag_set(host, RRDHOST_FLAG_INDEXED_HOSTNAME);
    else
        error("RRDHOST: host with hostname '%s' is already indexed", rrdhost_hostname(host));

    return host;
}

static inline RRDHOST *rrdhost_index_del_hostname(RRDHOST *host) {
    if(unlikely(!host->hostname)) return host;

    if(rrdhost_flag_check(host, RRDHOST_FLAG_INDEXED_HOSTNAME)) {
        if(dictionary_del(rrdhost_root_index_hostname, rrdhost_hostname(host)) !=  0)
            error("RRDHOST: failed to delete hostname '%s' from index", rrdhost_hostname(host));

        rrdhost_flag_clear(host, RRDHOST_FLAG_INDEXED_HOSTNAME);
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

static inline void rrdhost_init_hostname(RRDHOST *host, const char *hostname) {
    if(unlikely(hostname && !*hostname)) hostname = NULL;

    if(host->hostname && hostname && !strcmp(rrdhost_hostname(host), hostname))
        return;

    rrdhost_index_del_hostname(host);

    STRING *old = host->hostname;
    host->hostname = string_strdupz(hostname?hostname:"localhost");
    string_freez(old);

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
    if (host->timezone && timezone && !strcmp(host->timezone, timezone) && host->abbrev_timezone && abbrev_timezone &&
        !strcmp(host->abbrev_timezone, abbrev_timezone) && host->utc_offset == utc_offset)
        return;

    void *old = (void *)host->timezone;
    host->timezone = strdupz((timezone && *timezone)?timezone:"unknown");
    freez(old);

    old = (void *)host->abbrev_timezone;
    host->abbrev_timezone = strdupz((abbrev_timezone && *abbrev_timezone) ? abbrev_timezone : "UTC");
    freez(old);

    host->utc_offset = utc_offset;
}

static inline void rrdhost_init_machine_guid(RRDHOST *host, const char *machine_guid) {
    strncpy(host->machine_guid, machine_guid, GUID_LEN);
    host->machine_guid[GUID_LEN] = '\0';
}

void set_host_properties(RRDHOST *host, int update_every, RRD_MEMORY_MODE memory_mode, const char *hostname,
                         const char *registry_hostname, const char *guid, const char *os, const char *tags,
                         const char *tzone, const char *abbrev_tzone, int32_t utc_offset, const char *program_name,
                         const char *program_version)
{

    host->rrd_update_every = update_every;
    host->rrd_memory_mode = memory_mode;

    rrdhost_init_hostname(host, hostname);

    rrdhost_init_machine_guid(host, guid);

    rrdhost_init_os(host, os);
    rrdhost_init_timezone(host, tzone, abbrev_tzone, utc_offset);
    rrdhost_init_tags(host, tags);

    host->program_name = strdupz((program_name && *program_name) ? program_name : "unknown");
    host->program_version = strdupz((program_version && *program_version) ? program_version : "unknown");

    host->registry_hostname = strdupz((registry_hostname && *registry_hostname) ? registry_hostname : rrdhost_hostname(host));
}

// ----------------------------------------------------------------------------
// RRDHOST - add a host

RRDHOST *rrdhost_create(const char *hostname,
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
                        struct rrdhost_system_info *system_info,
                        int is_localhost,
                        bool archived
) {
    debug(D_RRDHOST, "Host '%s': adding with guid '%s'", hostname, guid);

#ifdef ENABLE_DBENGINE
    int is_legacy = (memory_mode == RRD_MEMORY_MODE_DBENGINE) && is_legacy_child(guid);
#else
    int is_legacy = 1;
#endif
    rrd_check_wrlock();

    int is_in_multihost = (memory_mode == RRD_MEMORY_MODE_DBENGINE && !is_legacy);
    RRDHOST *host = callocz(1, sizeof(RRDHOST));

    set_host_properties(host, (update_every > 0)?update_every:1, memory_mode, hostname, registry_hostname, guid, os,
                        tags, timezone, abbrev_timezone, utc_offset, program_name, program_version);

    host->rrd_history_entries = align_entries_to_pagesize(memory_mode, entries);
    host->health_enabled      = ((memory_mode == RRD_MEMORY_MODE_NONE)) ? 0 : health_enabled;

    sender_init(host);
    netdata_mutex_init(&host->receiver_lock);

    host->rrdpush_send_enabled     = (rrdpush_enabled && rrdpush_destination && *rrdpush_destination && rrdpush_api_key && *rrdpush_api_key) ? 1 : 0;
    host->rrdpush_send_destination = (host->rrdpush_send_enabled)?strdupz(rrdpush_destination):NULL;
    if (host->rrdpush_send_destination)
        host->destinations = destinations_init(host->rrdpush_send_destination);
    host->rrdpush_send_api_key     = (host->rrdpush_send_enabled)?strdupz(rrdpush_api_key):NULL;
    host->rrdpush_send_charts_matching = simple_pattern_create(rrdpush_send_charts_matching, NULL, SIMPLE_PATTERN_EXACT);

    host->rrdpush_sender_pipe[0] = -1;
    host->rrdpush_sender_pipe[1] = -1;
    host->rrdpush_sender_socket  = -1;

    //host->stream_version = STREAMING_PROTOCOL_CURRENT_VERSION;        Unused?
#ifdef ENABLE_HTTPS
    host->ssl.conn = NULL;
    host->ssl.flags = NETDATA_SSL_START;
    host->stream_ssl.conn = NULL;
    host->stream_ssl.flags = NETDATA_SSL_START;
#endif

    netdata_rwlock_init(&host->rrdhost_rwlock);
    host->host_labels = rrdlabels_create();

    netdata_mutex_init(&host->aclk_state_lock);

    host->system_info = system_info;

    host->rrdset_root_index      = dictionary_create(DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    host->rrdset_root_index_name = dictionary_create(DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    host->rrdfamily_root_index   = dictionary_create(DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);
    host->rrdvar_root_index      = dictionary_create(DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);

    if(config_get_boolean(CONFIG_SECTION_DB, "delete obsolete charts files", 1))
        rrdhost_flag_set(host, RRDHOST_FLAG_DELETE_OBSOLETE_CHARTS);

    if(config_get_boolean(CONFIG_SECTION_DB, "delete orphan hosts files", 1) && !is_localhost)
        rrdhost_flag_set(host, RRDHOST_FLAG_DELETE_ORPHAN_HOST);

    host->health_default_warn_repeat_every = config_get_duration(CONFIG_SECTION_HEALTH, "default repeat warning", "never");
    host->health_default_crit_repeat_every = config_get_duration(CONFIG_SECTION_HEALTH, "default repeat critical", "never");
    avl_init_lock(&(host->alarms_idx_health_log), alarm_compare_id);
    avl_init_lock(&(host->alarms_idx_name), alarm_compare_name);

    // ------------------------------------------------------------------------
    // initialize health variables

    host->health_log.next_log_id = 1;
    host->health_log.next_alarm_id = 1;
    host->health_log.max = 1000;
    host->health_log.next_log_id = (uint32_t)now_realtime_sec();
    host->health_log.next_alarm_id = 0;

    long n = config_get_number(CONFIG_SECTION_HEALTH, "in memory max health log entries", host->health_log.max);
    if(n < 10) {
        error("Host '%s': health configuration has invalid max log entries %ld. Using default %u", rrdhost_hostname(host), n, host->health_log.max);
        config_set_number(CONFIG_SECTION_HEALTH, "in memory max health log entries", (long)host->health_log.max);
    }
    else
        host->health_log.max = (unsigned int)n;

    netdata_rwlock_init(&host->health_log.alarm_log_rwlock);

    char filename[FILENAME_MAX + 1];

    if(is_localhost) {

        host->cache_dir  = strdupz(netdata_configured_cache_dir);
        host->varlib_dir = strdupz(netdata_configured_varlib_dir);

    }
    else {
        // this is not localhost - append our GUID to localhost path
        if (is_in_multihost) { // don't append to cache dir in multihost
            host->cache_dir  = strdupz(netdata_configured_cache_dir);
        } else {
            snprintfz(filename, FILENAME_MAX, "%s/%s", netdata_configured_cache_dir, host->machine_guid);
            host->cache_dir = strdupz(filename);
        }

        if((host->rrd_memory_mode == RRD_MEMORY_MODE_MAP || host->rrd_memory_mode == RRD_MEMORY_MODE_SAVE || (
           host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && is_legacy))) {
            int r = mkdir(host->cache_dir, 0775);
            if(r != 0 && errno != EEXIST)
                error("Host '%s': cannot create directory '%s'", rrdhost_hostname(host), host->cache_dir);
        }

        snprintfz(filename, FILENAME_MAX, "%s/%s", netdata_configured_varlib_dir, host->machine_guid);
        host->varlib_dir = strdupz(filename);

        if(host->health_enabled) {
            int r = mkdir(host->varlib_dir, 0775);
            if(r != 0 && errno != EEXIST)
                error("Host '%s': cannot create directory '%s'", rrdhost_hostname(host), host->varlib_dir);
       }

    }

    if(host->health_enabled) {
        snprintfz(filename, FILENAME_MAX, "%s/health", host->varlib_dir);
        int r = mkdir(filename, 0775);
        if(r != 0 && errno != EEXIST)
            error("Host '%s': cannot create directory '%s'", rrdhost_hostname(host), filename);
    }

    snprintfz(filename, FILENAME_MAX, "%s/health/health-log.db", host->varlib_dir);
    host->health_log_filename = strdupz(filename);

    snprintfz(filename, FILENAME_MAX, "%s/alarm-notify.sh", netdata_configured_primary_plugins_dir);
    host->health_default_exec = string_strdupz(config_get(CONFIG_SECTION_HEALTH, "script to execute on alarm", filename));
    host->health_default_recipient = string_strdupz("root");


    // ------------------------------------------------------------------------
    // load health configuration

    if(host->health_enabled) {
        rrdhost_wrlock(host);
        health_readdir(host, health_user_config_dir(), health_stock_config_dir(), NULL);
        rrdhost_unlock(host);
    }

    RRDHOST *t = rrdhost_index_add(host);

    if(t != host) {
        error("Host '%s': cannot add host with machine guid '%s' to index. It already exists as host '%s' with machine guid '%s'.", rrdhost_hostname(host), host->machine_guid, rrdhost_hostname(t), t->machine_guid);
        rrdhost_free(host, 1);
        return NULL;
    }

    if (likely(!uuid_parse(host->machine_guid, host->host_uuid))) {
        int rc;
        if (!archived) {
            rc = sql_store_host_info(host);
            if (unlikely(rc))
                error_report("Failed to store machine GUID to the database");
        }
        sql_load_node_id(host);
        if (host->health_enabled) {
            if (!file_is_migrated(host->health_log_filename)) {
                rc = sql_create_health_log_table(host);
                if (unlikely(rc)) {
                    error_report("Failed to create health log table in the database");
                    health_alarm_log_load(host);
                    health_alarm_log_open(host);
                }
                else {
                    health_alarm_log_load(host);
                    add_migrated_file(host->health_log_filename, 0);
                }
            } else {
                sql_create_health_log_table(host);
                sql_health_alarm_log_load(host);
            }
        }
    }
    else
        error_report("Host machine GUID %s is not valid", host->machine_guid);

    if (host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
#ifdef ENABLE_DBENGINE
        char dbenginepath[FILENAME_MAX + 1];
        int ret;

        snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", host->cache_dir);
        ret = mkdir(dbenginepath, 0775);
        if (ret != 0 && errno != EEXIST)
            error("Host '%s': cannot create directory '%s'", rrdhost_hostname(host), dbenginepath);
        else ret = 0; // succeed
        if (is_legacy) {
            // initialize legacy dbengine instance as needed

            ret = rrdeng_init(
                host,
                (struct rrdengine_instance **)&host->storage_instance[0],
                dbenginepath,
                default_rrdeng_page_cache_mb,
                default_rrdeng_disk_quota_mb,
                0); // may fail here for legacy dbengine initialization

            if(ret == 0) {
                // assign the rest of the shared storage instances to it
                // to allow them collect its metrics too
                for(int tier = 1; tier < storage_tiers ; tier++)
                    host->storage_instance[tier] = (STORAGE_INSTANCE *)multidb_ctx[tier];
            }
        }
        else {
            for(int tier = 0; tier < storage_tiers ; tier++)
                host->storage_instance[tier] = (STORAGE_INSTANCE *)multidb_ctx[tier];
        }
        if (ret) { // check legacy or multihost initialization success
            error(
                "Host '%s': cannot initialize host with machine guid '%s'. Failed to initialize DB engine at '%s'.",
                rrdhost_hostname(host), host->machine_guid, host->cache_dir);
            rrdhost_free(host, 1);
            host = NULL;
            //rrd_hosts_available++; //TODO: maybe we want this?

            return host;
        }

#else
        fatal("RRD_MEMORY_MODE_DBENGINE is not supported in this platform.");
#endif
    }
    else {
#ifdef ENABLE_DBENGINE
        // the first tier is reserved for the non-dbengine modes
        for(int tier = 1; tier < storage_tiers ; tier++)
            host->storage_instance[tier] = (STORAGE_INSTANCE *)multidb_ctx[tier];
#endif
    }

    // ------------------------------------------------------------------------
    // link it and add it to the index

    if(is_localhost) {
        host->next = localhost;
        localhost = host;
    }
    else {
        if(localhost) {
            host->next = localhost->next;
            localhost->next = host;
        }
        else localhost = host;
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
                 ", varlib_dir '%s'"
                 ", health_log '%s'"
                 ", alarms default handler '%s'"
                 ", alarms default recipient '%s'"
         , rrdhost_hostname(host)
         , host->registry_hostname
         , host->machine_guid
         , rrdhost_os(host)
         , host->timezone
         , rrdhost_tags(host)
         , host->program_name
         , host->program_version
         , host->rrd_update_every
         , rrd_memory_mode_name(host->rrd_memory_mode)
         , host->rrd_history_entries
         , host->rrdpush_send_enabled?"enabled":"disabled"
         , host->rrdpush_send_destination?host->rrdpush_send_destination:""
         , host->rrdpush_send_api_key?host->rrdpush_send_api_key:""
         , host->health_enabled?"enabled":"disabled"
         , host->cache_dir
         , host->varlib_dir
         , host->health_log_filename
         , string2str(host->health_default_exec)
         , string2str(host->health_default_recipient)
    );
    sql_store_host_system_info(&host->host_uuid, system_info);

    rrd_hosts_available++;

    rrdhost_load_rrdcontext_data(host);
    if (!archived)
        ml_new_host(host);
    else
        rrdhost_flag_set(host, RRDHOST_FLAG_ARCHIVED);
    return host;
}

void rrdhost_update(RRDHOST *host
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
                    , struct rrdhost_system_info *system_info
)
{
    UNUSED(guid);
    UNUSED(rrdpush_enabled);
    UNUSED(rrdpush_destination);
    UNUSED(rrdpush_api_key);
    UNUSED(rrdpush_send_charts_matching);

    host->health_enabled = (mode == RRD_MEMORY_MODE_NONE) ? 0 : health_enabled;
    //host->stream_version = STREAMING_PROTOCOL_CURRENT_VERSION;        Unused?

    rrdhost_system_info_free(host->system_info);
    host->system_info = system_info;
    sql_store_host_system_info(&host->host_uuid, system_info);

    rrdhost_init_os(host, os);
    rrdhost_init_timezone(host, timezone, abbrev_timezone, utc_offset);

    freez(host->registry_hostname);
    host->registry_hostname = strdupz((registry_hostname && *registry_hostname)?registry_hostname:hostname);

    if(strcmp(rrdhost_hostname(host), hostname) != 0) {
        info("Host '%s' has been renamed to '%s'. If this is not intentional it may mean multiple hosts are using the same machine_guid.", rrdhost_hostname(host), hostname);
        rrdhost_init_hostname(host, hostname);
    }

    if(strcmp(host->program_name, program_name) != 0) {
        info("Host '%s' switched program name from '%s' to '%s'", rrdhost_hostname(host), host->program_name, program_name);
        char *t = host->program_name;
        host->program_name = strdupz(program_name);
        freez(t);
    }

    if(strcmp(host->program_version, program_version) != 0) {
        info("Host '%s' switched program version from '%s' to '%s'", rrdhost_hostname(host), host->program_version, program_version);
        char *t = host->program_version;
        host->program_version = strdupz(program_version);
        freez(t);
    }

    if(host->rrd_update_every != update_every)
        error("Host '%s' has an update frequency of %d seconds, but the wanted one is %d seconds. Restart netdata here to apply the new settings.", rrdhost_hostname(host), host->rrd_update_every, update_every);

    if(host->rrd_history_entries < history)
        error("Host '%s' has history of %ld entries, but the wanted one is %ld entries. Restart netdata here to apply the new settings.", rrdhost_hostname(host), host->rrd_history_entries, history);

    if(host->rrd_memory_mode != mode)
        error("Host '%s' has memory mode '%s', but the wanted one is '%s'. Restart netdata here to apply the new settings.", rrdhost_hostname(host), rrd_memory_mode_name(host->rrd_memory_mode), rrd_memory_mode_name(mode));

    // update host tags
    rrdhost_init_tags(host, tags);

    if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_ARCHIVED);

        host->rrdpush_send_enabled     = (rrdpush_enabled && rrdpush_destination && *rrdpush_destination && rrdpush_api_key && *rrdpush_api_key) ? 1 : 0;
        host->rrdpush_send_destination = (host->rrdpush_send_enabled)?strdupz(rrdpush_destination):NULL;
        if (host->rrdpush_send_destination)
            host->destinations = destinations_init(host->rrdpush_send_destination);
        host->rrdpush_send_api_key     = (host->rrdpush_send_enabled)?strdupz(rrdpush_api_key):NULL;
        host->rrdpush_send_charts_matching = simple_pattern_create(rrdpush_send_charts_matching, NULL, SIMPLE_PATTERN_EXACT);

        if(host->health_enabled) {
            int r;
            char filename[FILENAME_MAX + 1];

            if (host != localhost) {
                r = mkdir(host->varlib_dir, 0775);
                if (r != 0 && errno != EEXIST)
                    error("Host '%s': cannot create directory '%s'", rrdhost_hostname(host), host->varlib_dir);
            }
            snprintfz(filename, FILENAME_MAX, "%s/health", host->varlib_dir);
            r = mkdir(filename, 0775);
            if(r != 0 && errno != EEXIST)
                error("Host '%s': cannot create directory '%s'", rrdhost_hostname(host), filename);

            rrdhost_wrlock(host);
            health_readdir(host, health_user_config_dir(), health_stock_config_dir(), NULL);
            rrdhost_unlock(host);

            if (!file_is_migrated(host->health_log_filename)) {
                int rc = sql_create_health_log_table(host);
                if (unlikely(rc)) {
                    error_report("Failed to create health log table in the database");

                    health_alarm_log_load(host);
                    health_alarm_log_open(host);
                } else {
                    health_alarm_log_load(host);
                    add_migrated_file(host->health_log_filename, 0);
                }
            } else {
                sql_create_health_log_table(host);
                sql_health_alarm_log_load(host);
            }
        }
        rrd_hosts_available++;
        ml_new_host(host);
        rrdhost_load_rrdcontext_data(host);
        info("Host %s is not in archived mode anymore", rrdhost_hostname(host));
    }

    return;
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
        , struct rrdhost_system_info *system_info
        , bool archived
) {
    debug(D_RRDHOST, "Searching for host '%s' with guid '%s'", hostname, guid);

    rrd_wrlock();
    RRDHOST *host = rrdhost_find_by_guid(guid);
    if (unlikely(host && RRD_MEMORY_MODE_DBENGINE != mode && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) {
        /* If a legacy memory mode instantiates all dbengine state must be discarded to avoid inconsistencies */
        error("Archived host '%s' has memory mode '%s', but the wanted one is '%s'. Discarding archived state.",
              rrdhost_hostname(host), rrd_memory_mode_name(host->rrd_memory_mode), rrd_memory_mode_name(mode));
        rrdhost_free(host, 1);
        host = NULL;
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
                , system_info
                , 0
                , archived
        );
    }
    else {
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
           , system_info);
    }
    if (host) {
        rrdhost_wrlock(host);
        rrdhost_flag_clear(host, RRDHOST_FLAG_ORPHAN);
        host->senders_disconnected_time = 0;
        rrdhost_unlock(host);
    }

    rrd_unlock();

    return host;
}
inline int rrdhost_should_be_removed(RRDHOST *host, RRDHOST *protected_host, time_t now) {
    if(host != protected_host
       && host != localhost
       && rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN)
       && !rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)
       && !host->receiver
       && host->senders_disconnected_time
       && host->senders_disconnected_time + rrdhost_free_orphan_time < now)
        return 1;

    return 0;
}

void rrdhost_cleanup_orphan_hosts_nolock(RRDHOST *protected_host) {
    time_t now = now_realtime_sec();

    RRDHOST *host;

restart_after_removal:
    rrdhost_foreach_write(host) {
        if(rrdhost_should_be_removed(host, protected_host, now)) {
            info("Host '%s' with machine guid '%s' is obsolete - cleaning up.", rrdhost_hostname(host), host->machine_guid);

            if (rrdhost_flag_check(host, RRDHOST_FLAG_DELETE_ORPHAN_HOST)
#ifdef ENABLE_DBENGINE
                /* don't delete multi-host DB host files */
                && !(host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && is_storage_engine_shared(host->storage_instance[0]))
#endif
            )
                rrdhost_delete_charts(host);
            else
                rrdhost_save_charts(host);

            rrdhost_free(host, 0);
            goto restart_after_removal;
        }
    }
}

// ----------------------------------------------------------------------------
// RRDHOST global / startup initialization

int rrd_init(char *hostname, struct rrdhost_system_info *system_info) {
    rrdhost_init();

    if (unlikely(sql_init_database(DB_CHECK_NONE, system_info ? 0 : 1))) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            fatal("Failed to initialize SQLite");
        info("Skipping SQLITE metadata initialization since memory mode is not dbengine");
    }

    if (unlikely(sql_init_context_database(system_info ? 0 : 1))) {
        error_report("Failed to initialize context metadata database");
    }

    if (unlikely(!system_info))
        goto unittest;

#ifdef ENABLE_DBENGINE
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

    default_rrdeng_page_fetch_timeout = (int) config_get_number(CONFIG_SECTION_DB, "dbengine page fetch timeout secs", PAGE_CACHE_FETCH_WAIT_TIMEOUT);
    if (default_rrdeng_page_fetch_timeout < 1) {
        info("'dbengine page fetch timeout secs' cannot be %d, using 1", default_rrdeng_page_fetch_timeout);
        default_rrdeng_page_fetch_timeout = 1;
        config_set_number(CONFIG_SECTION_DB, "dbengine page fetch timeout secs", default_rrdeng_page_fetch_timeout);
    }

    default_rrdeng_page_fetch_retries = (int) config_get_number(CONFIG_SECTION_DB, "dbengine page fetch retries", MAX_PAGE_CACHE_FETCH_RETRIES);
    if (default_rrdeng_page_fetch_retries < 1) {
        info("\"dbengine page fetch retries\" found in netdata.conf cannot be %d, using 1", default_rrdeng_page_fetch_retries);
        default_rrdeng_page_fetch_retries = 1;
        config_set_number(CONFIG_SECTION_DB, "dbengine page fetch retries", default_rrdeng_page_fetch_retries);
    }

    if(config_get_boolean(CONFIG_SECTION_DB, "dbengine page descriptors in file mapped memory", rrdeng_page_descr_is_mmap()) == CONFIG_BOOLEAN_YES)
        rrdeng_page_descr_use_mmap();
    else
        rrdeng_page_descr_use_malloc();

    int created_tiers = 0;
    char dbenginepath[FILENAME_MAX + 1];
    char dbengineconfig[200 + 1];
    for(int tier = 0; tier < storage_tiers ;tier++) {
        if(tier == 0)
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", netdata_configured_cache_dir);
        else
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine-tier%d", netdata_configured_cache_dir, tier);

        int ret = mkdir(dbenginepath, 0775);
        if (ret != 0 && errno != EEXIST) {
            error("DBENGINE on '%s': cannot create directory '%s'", hostname, dbenginepath);
            break;
        }

        int page_cache_mb = default_rrdeng_page_cache_mb;
        int disk_space_mb = default_multidb_disk_quota_mb;
        int grouping_iterations = storage_tiers_grouping_iterations[tier];
        RRD_BACKFILL backfill = storage_tiers_backfill[tier];

        if(tier > 0) {
            snprintfz(dbengineconfig, 200, "dbengine tier %d page cache size MB", tier);
            page_cache_mb = config_get_number(CONFIG_SECTION_DB, dbengineconfig, page_cache_mb);

            snprintfz(dbengineconfig, 200, "dbengine tier %d multihost disk space MB", tier);
            disk_space_mb = config_get_number(CONFIG_SECTION_DB, dbengineconfig, disk_space_mb);

            snprintfz(dbengineconfig, 200, "dbengine tier %d update every iterations", tier);
            grouping_iterations = config_get_number(CONFIG_SECTION_DB, dbengineconfig, grouping_iterations);
            if(grouping_iterations < 2) {
                grouping_iterations = 2;
                config_set_number(CONFIG_SECTION_DB, dbengineconfig, grouping_iterations);
                error("DBENGINE on '%s': 'dbegnine tier %d update every iterations' cannot be less than 2. Assuming 2.", hostname, tier);
            }

            snprintfz(dbengineconfig, 200, "dbengine tier %d backfill", tier);
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
            error("DBENGINE on '%s': dbengine tier %d gives aggregation of more than 65535 points of tier 0. Disabling tiers above %d", hostname, tier, tier);
            break;
        }
        
        internal_error(true, "DBENGINE tier %d grouping iterations is set to %d", tier, storage_tiers_grouping_iterations[tier]);
        ret = rrdeng_init(NULL, NULL, dbenginepath, page_cache_mb, disk_space_mb, tier);
        if(ret != 0) {
            error("DBENGINE on '%s': Failed to initialize multi-host database tier %d on path '%s'",
                  hostname, tier, dbenginepath);
            break;
        }
        else
            created_tiers++;
    }

    if(created_tiers && created_tiers < storage_tiers) {
        error("DBENGINE on '%s': Managed to create %d tiers instead of %d. Continuing with %d available.",
              hostname, created_tiers, storage_tiers, created_tiers);
        storage_tiers = created_tiers;
    }
    else if(!created_tiers)
        fatal("DBENGINE on '%s', failed to initialize databases at '%s'.", hostname, netdata_configured_cache_dir);

#else
    storage_tiers = config_get_number(CONFIG_SECTION_DB, "storage tiers", 1);
    if(storage_tiers != 1) {
        error("DBENGINE is not available on '%s', so only 1 database tier can be supported.", hostname);
        storage_tiers = 1;
        config_set_number(CONFIG_SECTION_DB, "storage tiers", storage_tiers);
    }
#endif

    health_init();
    rrdpush_init();

unittest:
    debug(D_RRDHOST, "Initializing localhost with hostname '%s'", hostname);
    rrd_wrlock();
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
        , system_info
        , 1
        , 0
    );
    if (unlikely(!localhost)) {
        rrd_unlock();
        return 1;
    }

    rrd_unlock();

    if (likely(system_info)) {
        migrate_localhost(&localhost->host_uuid);
        sql_aclk_sync_init();
        web_client_api_v1_management_init();
    }
    return localhost==NULL;
}

// ----------------------------------------------------------------------------
// RRDHOST - lock validations
// there are only used when NETDATA_INTERNAL_CHECKS is set

void __rrdhost_check_rdlock(RRDHOST *host, const char *file, const char *function, const unsigned long line) {
    debug(D_RRDHOST, "Checking read lock on host '%s'", rrdhost_hostname(host));

    int ret = netdata_rwlock_trywrlock(&host->rrdhost_rwlock);
    if(ret == 0)
        fatal("RRDHOST '%s' should be read-locked, but it is not, at function %s() at line %lu of file '%s'", rrdhost_hostname(host), function, line, file);
}

void __rrdhost_check_wrlock(RRDHOST *host, const char *file, const char *function, const unsigned long line) {
    debug(D_RRDHOST, "Checking write lock on host '%s'", rrdhost_hostname(host));

    int ret = netdata_rwlock_tryrdlock(&host->rrdhost_rwlock);
    if(ret == 0)
        fatal("RRDHOST '%s' should be write-locked, but it is not, at function %s() at line %lu of file '%s'", rrdhost_hostname(host), function, line, file);
}

void __rrd_check_rdlock(const char *file, const char *function, const unsigned long line) {
    debug(D_RRDHOST, "Checking read lock on all RRDs");

    int ret = netdata_rwlock_trywrlock(&rrd_rwlock);
    if(ret == 0)
        fatal("RRDs should be read-locked, but it are not, at function %s() at line %lu of file '%s'", function, line, file);
}

void __rrd_check_wrlock(const char *file, const char *function, const unsigned long line) {
    debug(D_RRDHOST, "Checking write lock on all RRDs");

    int ret = netdata_rwlock_tryrdlock(&rrd_rwlock);
    if(ret == 0)
        fatal("RRDs should be write-locked, but it are not, at function %s() at line %lu of file '%s'", function, line, file);
}

// ----------------------------------------------------------------------------
// RRDHOST - free

void rrdhost_system_info_free(struct rrdhost_system_info *system_info) {
    info("SYSTEM_INFO: free %p", system_info);

    if(likely(system_info)) {
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

void destroy_receiver_state(struct receiver_state *rpt);

void stop_streaming_sender(RRDHOST *host)
{
    if (unlikely(!host->sender))
        return;

    rrdpush_sender_thread_stop(host); // stop a possibly running thread
    cbuffer_free(host->sender->buffer);
    buffer_free(host->sender->build);
#ifdef ENABLE_COMPRESSION
    if (host->sender->compressor)
        host->sender->compressor->destroy(&host->sender->compressor);
#endif
    freez(host->sender);
    host->sender = NULL;
}

void stop_streaming_receiver(RRDHOST *host)
{
    netdata_mutex_lock(&host->receiver_lock);
    if (host->receiver) {
        if (!host->receiver->exited)
            netdata_thread_cancel(host->receiver->thread);
        netdata_mutex_unlock(&host->receiver_lock);
        struct receiver_state *rpt = host->receiver;
        while (host->receiver && !rpt->exited)
            sleep_usec(50 * USEC_PER_MS);
        // If the receiver detached from the host then its thread will destroy the state
        if (host->receiver == rpt)
            destroy_receiver_state(host->receiver);
    } else
        netdata_mutex_unlock(&host->receiver_lock);
}

void rrdhost_free(RRDHOST *host, bool force) {
    if(!host) return;

    if (netdata_exit || force)
        info("Freeing all memory for host '%s'...", rrdhost_hostname(host));

    rrd_check_wrlock();     // make sure the RRDs are write locked

    rrdhost_wrlock(host);
    ml_delete_host(host);
    rrdhost_unlock(host);

    // ------------------------------------------------------------------------
    // clean up streaming
    stop_streaming_sender(host);

    if (netdata_exit || force)
        stop_streaming_receiver(host);

    rrdhost_wrlock(host);   // lock this RRDHOST
    // ------------------------------------------------------------------------
    // release its children resources

#ifdef ENABLE_DBENGINE
    for(int tier = 0; tier < storage_tiers ;tier++) {
        if(host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE &&
            host->storage_instance[tier] &&
            !is_storage_engine_shared(host->storage_instance[tier]))
            rrdeng_prepare_exit((struct rrdengine_instance *)host->storage_instance[tier]);
    }
#endif

    while(host->rrdset_root)
        rrdset_free(host->rrdset_root);

    freez(host->exporting_flags);

    while(host->alarms)
        rrdcalc_unlink_and_free(host, host->alarms);

    RRDCALC *rc,*nc;
    for(rc = host->alarms_with_foreach; rc ; rc = nc) {
        nc = rc->next;
        rrdcalc_free(rc);
    }
    host->alarms_with_foreach = NULL;

    while(host->templates)
        rrdcalctemplate_unlink_and_free(host, host->templates);

    RRDCALCTEMPLATE *rt,*next;
    for(rt = host->alarms_template_with_foreach; rt ; rt = next) {
        next = rt->next;
        rrdcalctemplate_free(rt);
    }
    host->alarms_template_with_foreach = NULL;

    debug(D_RRD_CALLS, "RRDHOST: Cleaning up remaining host variables for host '%s'", rrdhost_hostname(host));
    rrdvar_free_remaining_variables(host, host->rrdvar_root_index);

    health_alarm_log_free(host);

#ifdef ENABLE_DBENGINE
    for(int tier = 0; tier < storage_tiers ;tier++) {
        if(host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE &&
            host->storage_instance[tier] &&
            !is_storage_engine_shared(host->storage_instance[tier]))
            rrdeng_exit((struct rrdengine_instance *)host->storage_instance[tier]);
    }
#endif

    if (!netdata_exit && !force) {
        info("Setting archive mode for host '%s'...", rrdhost_hostname(host));
        rrdhost_flag_set(host, RRDHOST_FLAG_ARCHIVED);
        rrdhost_unlock(host);
        return;
    }

#ifdef ENABLE_ACLK
    struct aclk_database_worker_config *wc =  host->dbsync_worker;
    if (wc && !netdata_exit) {
        struct aclk_database_cmd cmd;
        memset(&cmd, 0, sizeof(cmd));
        cmd.opcode = ACLK_DATABASE_ORPHAN_HOST;
        struct aclk_completion compl ;
        init_aclk_completion(&compl );
        cmd.completion = &compl ;
        aclk_database_enq_cmd(wc, &cmd);
        wait_for_aclk_completion(&compl );
        destroy_aclk_completion(&compl );
    }
#endif

    // ------------------------------------------------------------------------
    // remove it from the indexes

    if(rrdhost_index_del(host) != host)
        error("RRDHOST '%s' removed from index, deleted the wrong entry.", rrdhost_hostname(host));

    // ------------------------------------------------------------------------
    // unlink it from the host

    if(host == localhost) {
        localhost = host->next;
    }
    else {
        // find the previous one
        RRDHOST *h;
        for(h = localhost; h && h->next != host ; h = h->next) ;

        // bypass it
        if(h) h->next = host->next;
        else error("Request to free RRDHOST '%s': cannot find it", rrdhost_hostname(host));
    }

    // ------------------------------------------------------------------------
    // free it

    pthread_mutex_destroy(&host->aclk_state_lock);
    freez(host->aclk_state.claimed_id);
    freez(host->aclk_state.prev_claimed_id);
    string_freez(host->tags);
    rrdlabels_destroy(host->host_labels);
    string_freez(host->os);
    freez((void *)host->timezone);
    freez((void *)host->abbrev_timezone);
    freez(host->program_version);
    freez(host->program_name);
    rrdhost_system_info_free(host->system_info);
    freez(host->cache_dir);
    freez(host->varlib_dir);
    freez(host->rrdpush_send_api_key);
    freez(host->rrdpush_send_destination);
    struct rrdpush_destinations *tmp_destination;
    while (host->destinations) {
        tmp_destination = host->destinations->next;
        freez(host->destinations);
        host->destinations = tmp_destination;
    }
    string_freez(host->health_default_exec);
    string_freez(host->health_default_recipient);
    freez(host->health_log_filename);
    freez(host->registry_hostname);
    simple_pattern_free(host->rrdpush_send_charts_matching);
    rrdhost_unlock(host);
    netdata_rwlock_destroy(&host->health_log.alarm_log_rwlock);
    netdata_rwlock_destroy(&host->rrdhost_rwlock);
    freez(host->node_id);

    dictionary_destroy(host->rrdset_root_index);
    dictionary_destroy(host->rrdset_root_index_name);
    dictionary_destroy(host->rrdfamily_root_index);
    dictionary_destroy(host->rrdvar_root_index);

    rrdhost_destroy_rrdcontexts(host);

    string_freez(host->hostname);
    freez(host);
#ifdef ENABLE_ACLK
    if (wc)
        wc->is_orphan = 0;
#endif
    rrd_hosts_available--;
}

void rrdhost_free_all(void) {
    rrd_wrlock();
    /* Make sure child-hosts are released before the localhost. */
    while(localhost->next) rrdhost_free(localhost->next, 1);
    rrdhost_free(localhost, 1);
    rrd_unlock();
}

// ----------------------------------------------------------------------------
// RRDHOST - save host files

void rrdhost_save_charts(RRDHOST *host) {
    if(!host) return;

    info("Saving/Closing database of host '%s'...", rrdhost_hostname(host));

    RRDSET *st;

    // we get a write lock
    // to ensure only one thread is saving the database
    rrdhost_wrlock(host);

    rrdset_foreach_write(st, host) {
        rrdset_rdlock(st);
        rrdset_save(st);
        rrdset_unlock(st);
    }

    rrdhost_unlock(host);
}

static void rrdhost_load_auto_labels(void) {
    DICTIONARY *labels = localhost->host_labels;

    if (localhost->system_info->cloud_provider_type)
        rrdlabels_add(labels, "_cloud_provider_type", localhost->system_info->cloud_provider_type, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->cloud_instance_type)
        rrdlabels_add(labels, "_cloud_instance_type", localhost->system_info->cloud_instance_type, RRDLABEL_SRC_AUTO);

    if (localhost->system_info->cloud_instance_region)
        rrdlabels_add(
            labels, "_cloud_instance_region", localhost->system_info->cloud_instance_region, RRDLABEL_SRC_AUTO);

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

    rrdlabels_add(
        labels, "_is_parent", (rrdhost_hosts_available() > 1 || configured_as_parent()) ? "true" : "false", RRDLABEL_SRC_AUTO);

    if (localhost->rrdpush_send_destination)
        rrdlabels_add(labels, "_streams_to", localhost->rrdpush_send_destination, RRDLABEL_SRC_AUTO);
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
            rrdlabels_add(localhost->host_labels, cv->name, cv->value, RRDLABEL_SRC_CONFIG);
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
    FILE *fp = mypopen(label_script, &pid);
    if(!fp) return;

    char buffer[1000 + 1];
    while (fgets(buffer, 1000, fp) != NULL)
        rrdlabels_add_pair(localhost->host_labels, buffer, RRDLABEL_SRC_AUTO|RRDLABEL_SRC_K8S);

    // Non-zero exit code means that all the script output is error messages. We've shown already any message that didn't include a ':'
    // Here we'll inform with an ERROR that the script failed, show whatever (if anything) was added to the list of labels, free the memory and set the return to null
    int rc = mypclose(fp, pid);
    if(rc) error("%s exited abnormally. Failed to get kubernetes labels.", label_script);
}

void reload_host_labels(void) {
    if(!localhost->host_labels)
        localhost->host_labels = rrdlabels_create();

    rrdlabels_unmark_all(localhost->host_labels);

    // priority is important here
    rrdhost_load_config_labels();
    rrdhost_load_kubernetes_labels();
    rrdhost_load_auto_labels();

    rrdlabels_remove_all_unmarked(localhost->host_labels);
    sql_store_host_labels(localhost);

    health_label_log_save(localhost);

/*  TODO-GAPS - fix this so that it looks properly at the state and version of the sender
    if(localhost->rrdpush_send_enabled && localhost->rrdpush_sender_buffer){
        localhost->labels.labels_flag |= RRDHOST_FLAG_STREAM_LABELS_UPDATE;
        rrdpush_send_labels(localhost);
    }
*/
    health_reload();
}

// ----------------------------------------------------------------------------
// RRDHOST - delete host files

void rrdhost_delete_charts(RRDHOST *host) {
    if(!host) return;

    info("Deleting database of host '%s'...", rrdhost_hostname(host));

    RRDSET *st;

    // we get a write lock
    // to ensure only one thread is saving the database
    rrdhost_wrlock(host);

    rrdset_foreach_write(st, host) {
        rrdset_rdlock(st);
        rrdset_delete_files(st);
        rrdset_unlock(st);
    }

    recursively_delete_dir(host->cache_dir, "left over host");

    rrdhost_unlock(host);
}

// ----------------------------------------------------------------------------
// RRDHOST - cleanup host files

void rrdhost_cleanup_charts(RRDHOST *host) {
    if(!host) return;

    info("Cleaning up database of host '%s'...", rrdhost_hostname(host));

    RRDSET *st;
    uint32_t rrdhost_delete_obsolete_charts = rrdhost_flag_check(host, RRDHOST_FLAG_DELETE_OBSOLETE_CHARTS);

    // we get a write lock
    // to ensure only one thread is saving the database
    rrdhost_wrlock(host);

    rrdset_foreach_write(st, host) {
        rrdset_rdlock(st);

        if(rrdhost_delete_obsolete_charts && rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE))
            rrdset_delete_files(st);
        else if(rrdhost_delete_obsolete_charts && rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE_DIMENSIONS))
            rrdset_delete_obsolete_dimensions(st);
        else
            rrdset_save(st);

        rrdset_unlock(st);
    }

    rrdhost_unlock(host);
}


// ----------------------------------------------------------------------------
// RRDHOST - save all hosts to disk

void rrdhost_save_all(void) {
    info("Saving database [%zu hosts(s)]...", rrd_hosts_available);

    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host)
        rrdhost_save_charts(host);

    rrd_unlock();
}

// ----------------------------------------------------------------------------
// RRDHOST - save or delete all hosts from disk

void rrdhost_cleanup_all(void) {
    info("Cleaning up database [%zu hosts(s)]...", rrd_hosts_available);

    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host) {
        if (host != localhost && rrdhost_flag_check(host, RRDHOST_FLAG_DELETE_ORPHAN_HOST) && !host->receiver
#ifdef ENABLE_DBENGINE
            /* don't delete multi-host DB host files */
            && !(host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && is_storage_engine_shared(host->storage_instance[0]))
#endif
        )
            rrdhost_delete_charts(host);
        else
            rrdhost_cleanup_charts(host);
    }

    rrd_unlock();
}


// ----------------------------------------------------------------------------
// RRDHOST - save or delete all the host charts from disk

void rrdhost_cleanup_obsolete_charts(RRDHOST *host) {
    time_t now = now_realtime_sec();

    RRDSET *st;

    uint32_t rrdhost_delete_obsolete_charts = rrdhost_flag_check(host, RRDHOST_FLAG_DELETE_OBSOLETE_CHARTS);

restart_after_removal:
    rrdset_foreach_write(st, host) {
        if(unlikely(rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)
                    && st->last_accessed_time + rrdset_free_obsolete_time < now
                    && st->last_updated.tv_sec + rrdset_free_obsolete_time < now
                    && st->last_collected_time.tv_sec + rrdset_free_obsolete_time < now
        )) {
            st->rrdhost->obsolete_charts_count--;
#ifdef ENABLE_DBENGINE
            if(st->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
                RRDDIM *rd, *last;

                rrdset_flag_set(st, RRDSET_FLAG_ARCHIVED);
                while (st->variables)  rrdsetvar_free(st->variables);
                while (st->alarms)     rrdsetcalc_unlink(st->alarms);
                rrdset_wrlock(st);
                for (rd = st->dimensions, last = NULL ; likely(rd) ; ) {
                    if (rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)) {
                        last = rd;
                        rd = rd->next;
                        continue;
                    }

                    if (rrddim_flag_check(rd, RRDDIM_FLAG_ACLK)) {
                        last = rd;
                        rd = rd->next;
                        continue;
                    }
                    rrddim_flag_set(rd, RRDDIM_FLAG_ARCHIVED);
                    while (rd->variables)
                        rrddimvar_free(rd->variables);

                    if (rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) {
                        rrddim_flag_clear(rd, RRDDIM_FLAG_OBSOLETE);

                        /* only a collector can mark a chart as obsolete, so we must remove the reference */

                        size_t tiers_available = 0, tiers_said_yes = 0;
                        for(int tier = 0; tier < storage_tiers ;tier++) {
                            if(rd->tiers[tier]) {
                                tiers_available++;

                                if(rd->tiers[tier]->collect_ops.finalize(rd->tiers[tier]->db_collection_handle))
                                    tiers_said_yes++;

                                rd->tiers[tier]->db_collection_handle = NULL;
                            }
                        }

                        if (tiers_available == tiers_said_yes && tiers_said_yes) {
                            /* This metric has no data and no references */
                            delete_dimension_uuid(&rd->metric_uuid);
                            rrddim_free(st, rd);
                            if (unlikely(!last)) {
                                rd = st->dimensions;
                            }
                            else {
                                rd = last->next;
                            }
                            continue;
                        }
#ifdef ENABLE_ACLK
                        else
                            queue_dimension_to_aclk(rd, rd->last_collected_time.tv_sec);
#endif
                    }
                    last = rd;
                    rd = rd->next;
                }
                rrdset_unlock(st);

                debug(D_RRD_CALLS, "RRDSET: Cleaning up remaining chart variables for host '%s', chart '%s'", rrdhost_hostname(host), rrdset_id(st));
                rrdvar_free_remaining_variables(host, st->rrdvar_root_index);

                rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE);
                
                if (st->dimensions) {
                    /* If the chart still has dimensions don't delete it from the metadata log */
                    continue;
                }
            }
#endif
            rrdset_rdlock(st);

            if(rrdhost_delete_obsolete_charts)
                rrdset_delete_files(st);
            else
                rrdset_save(st);

            rrdset_unlock(st);

            rrdset_free(st);
            goto restart_after_removal;
        }
#ifdef ENABLE_ACLK
        else
            sql_check_chart_liveness(st);
#endif
    }
}

void rrdset_check_obsoletion(RRDHOST *host)
{
    RRDSET *st;
    time_t last_entry_t;
    rrdset_foreach_read(st, host) {
        last_entry_t = rrdset_last_entry_t(st);
        if (last_entry_t && last_entry_t < host->senders_connect_time) {
            rrdset_is_obsolete(st);
        }
    }
}

void rrd_cleanup_obsolete_charts()
{
    rrd_rdlock();

    RRDHOST *host;
    rrdhost_foreach_read(host)
    {
        if (host->obsolete_charts_count) {
            rrdhost_wrlock(host);
            rrdhost_cleanup_obsolete_charts(host);
            rrdhost_unlock(host);
        }

        if ( host != localhost &&
             host->trigger_chart_obsoletion_check &&
             ((host->senders_last_chart_command &&
             host->senders_last_chart_command + host->health_delay_up_to < now_realtime_sec())
              || (host->senders_connect_time + 300 < now_realtime_sec())) ) {
            rrdhost_rdlock(host);
            rrdset_check_obsoletion(host);
            rrdhost_unlock(host);
            host->trigger_chart_obsoletion_check = 0;
        }
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

/**
 * Alarm Compare ID
 *
 * Callback function used with the binary trees to compare the id of RRDCALC
 *
 * @param a a pointer to the RRDCAL item to insert,compare or update the binary tree
 * @param b the pointer to the binary tree.
 *
 * @return It returns 0 case the values are equal, 1 case a is bigger than b and -1 case a is smaller than b.
 */
int alarm_compare_id(void *a, void *b) {
    register uint32_t hash1 = ((RRDCALC *)a)->id;
    register uint32_t hash2 = ((RRDCALC *)b)->id;

    if(hash1 < hash2) return -1;
    else if(hash1 > hash2) return 1;

    return 0;
}

/**
 * Alarm Compare NAME
 *
 * Callback function used with the binary trees to compare the name of RRDCALC
 *
 * @param a a pointer to the RRDCAL item to insert,compare or update the binary tree
 * @param b the pointer to the binary tree.
 *
 * @return It returns 0 case the values are equal, 1 case a is bigger than b and -1 case a is smaller than b.
 */
int alarm_compare_name(void *a, void *b) {
    RRDCALC *A = (RRDCALC *)a;
    RRDCALC *B = (RRDCALC *)b;

    return (int)((uintptr_t)A->name - (uintptr_t)B->name);
}

// Added for gap-filling, if this proves to be a bottleneck in large-scale systems then we will need to cache
// the last entry times as the metric updates, but let's see if it is a problem first.
time_t rrdhost_last_entry_t(RRDHOST *h) {
    rrdhost_rdlock(h);
    RRDSET *st;
    time_t result = 0;
    rrdset_foreach_read(st, h) {
        time_t st_last = rrdset_last_entry_t(st);
        if (st_last > result)
            result = st_last;
    }
    rrdhost_unlock(h);
    return result;
}
