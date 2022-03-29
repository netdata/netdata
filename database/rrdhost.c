// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"

RRDHOST *localhost = NULL;
size_t rrd_hosts_available = 0;
netdata_rwlock_t rrd_rwlock = NETDATA_RWLOCK_INITIALIZER;

time_t rrdset_free_obsolete_time = 3600;
time_t rrdhost_free_orphan_time = 3600;

// ----------------------------------------------------------------------------
// RRDHOST index

int rrdhost_compare(void* a, void* b) {
    if(((RRDHOST *)a)->hash_machine_guid < ((RRDHOST *)b)->hash_machine_guid) return -1;
    else if(((RRDHOST *)a)->hash_machine_guid > ((RRDHOST *)b)->hash_machine_guid) return 1;
    else return strcmp(((RRDHOST *)a)->machine_guid, ((RRDHOST *)b)->machine_guid);
}

avl_tree_lock rrdhost_root_index = {
        .avl_tree = { NULL, rrdhost_compare },
        .rwlock = AVL_LOCK_INITIALIZER
};

RRDHOST *rrdhost_find_by_guid(const char *guid, uint32_t hash) {
    debug(D_RRDHOST, "Searching in index for host with guid '%s'", guid);

    RRDHOST tmp;
    strncpyz(tmp.machine_guid, guid, GUID_LEN);
    tmp.hash_machine_guid = (hash)?hash:simple_hash(tmp.machine_guid);

    return (RRDHOST *)avl_search_lock(&(rrdhost_root_index), (avl_t *) &tmp);
}

RRDHOST *rrdhost_find_by_hostname(const char *hostname, uint32_t hash) {
    if(unlikely(!strcmp(hostname, "localhost")))
        return localhost;

    if(unlikely(!hash)) hash = simple_hash(hostname);

    rrd_rdlock();
    RRDHOST *host;
    rrdhost_foreach_read(host) {
        if(unlikely((hash == host->hash_hostname && !strcmp(hostname, host->hostname)))) {
            rrd_unlock();
            return host;
        }
    }
    rrd_unlock();

    return NULL;
}

#define rrdhost_index_add(rrdhost) (RRDHOST *)avl_insert_lock(&(rrdhost_root_index), (avl_t *)(rrdhost))
#define rrdhost_index_del(rrdhost) (RRDHOST *)avl_remove_lock(&(rrdhost_root_index), (avl_t *)(rrdhost))


// ----------------------------------------------------------------------------
// RRDHOST - internal helpers

static inline void rrdhost_init_tags(RRDHOST *host, const char *tags) {
    if(host->tags && tags && !strcmp(host->tags, tags))
        return;

    void *old = (void *)host->tags;
    host->tags = (tags && *tags)?strdupz(tags):NULL;
    freez(old);
}

static inline void rrdhost_init_hostname(RRDHOST *host, const char *hostname) {
    if(host->hostname && hostname && !strcmp(host->hostname, hostname))
        return;

    void *old = host->hostname;
    host->hostname = strdupz(hostname?hostname:"localhost");
    host->hash_hostname = simple_hash(host->hostname);
    freez(old);
}

static inline void rrdhost_init_os(RRDHOST *host, const char *os) {
    if(host->os && os && !strcmp(host->os, os))
        return;

    void *old = (void *)host->os;
    host->os = strdupz(os?os:"unknown");
    freez(old);
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
    host->hash_machine_guid = simple_hash(host->machine_guid);
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

    host->registry_hostname = strdupz((registry_hostname && *registry_hostname) ? registry_hostname : host->hostname);
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
                        int is_localhost
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

    host->sender = mallocz(sizeof(*host->sender));
    sender_init(host->sender, host);
    netdata_mutex_init(&host->receiver_lock);

    host->rrdpush_send_enabled     = (rrdpush_enabled && rrdpush_destination && *rrdpush_destination && rrdpush_api_key && *rrdpush_api_key) ? 1 : 0;
    host->rrdpush_send_destination = (host->rrdpush_send_enabled)?strdupz(rrdpush_destination):NULL;
    host->rrdpush_send_api_key     = (host->rrdpush_send_enabled)?strdupz(rrdpush_api_key):NULL;
    host->rrdpush_send_charts_matching = simple_pattern_create(rrdpush_send_charts_matching, NULL, SIMPLE_PATTERN_EXACT);

    host->rrdpush_sender_pipe[0] = -1;
    host->rrdpush_sender_pipe[1] = -1;
    host->rrdpush_sender_socket  = -1;

#ifdef ENABLE_HTTPS
    host->ssl.conn = NULL;
    host->ssl.flags = NETDATA_SSL_START;
    host->stream_ssl.conn = NULL;
    host->stream_ssl.flags = NETDATA_SSL_START;
#endif

    netdata_rwlock_init(&host->rrdhost_rwlock);
    netdata_rwlock_init(&host->labels.labels_rwlock);

    netdata_mutex_init(&host->aclk_state_lock);

    host->system_info = system_info;

    avl_init_lock(&(host->rrdset_root_index),      rrdset_compare);
    avl_init_lock(&(host->rrdset_root_index_name), rrdset_compare_name);
    avl_init_lock(&(host->rrdfamily_root_index),   rrdfamily_compare);
    avl_init_lock(&(host->rrdvar_root_index),   rrdvar_compare);

    if(config_get_boolean(CONFIG_SECTION_GLOBAL, "delete obsolete charts files", 1))
        rrdhost_flag_set(host, RRDHOST_FLAG_DELETE_OBSOLETE_CHARTS);

    if(config_get_boolean(CONFIG_SECTION_GLOBAL, "delete orphan hosts files", 1) && !is_localhost)
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
        error("Host '%s': health configuration has invalid max log entries %ld. Using default %u", host->hostname, n, host->health_log.max);
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
                error("Host '%s': cannot create directory '%s'", host->hostname, host->cache_dir);
        }

        snprintfz(filename, FILENAME_MAX, "%s/%s", netdata_configured_varlib_dir, host->machine_guid);
        host->varlib_dir = strdupz(filename);

        if(host->health_enabled) {
            int r = mkdir(host->varlib_dir, 0775);
            if(r != 0 && errno != EEXIST)
                error("Host '%s': cannot create directory '%s'", host->hostname, host->varlib_dir);
       }

    }

    if(host->health_enabled) {
        snprintfz(filename, FILENAME_MAX, "%s/health", host->varlib_dir);
        int r = mkdir(filename, 0775);
        if(r != 0 && errno != EEXIST)
            error("Host '%s': cannot create directory '%s'", host->hostname, filename);
    }

    snprintfz(filename, FILENAME_MAX, "%s/health/health-log.db", host->varlib_dir);
    host->health_log_filename = strdupz(filename);

    snprintfz(filename, FILENAME_MAX, "%s/alarm-notify.sh", netdata_configured_primary_plugins_dir);
    host->health_default_exec = strdupz(config_get(CONFIG_SECTION_HEALTH, "script to execute on alarm", filename));
    host->health_default_recipient = strdupz("root");


    // ------------------------------------------------------------------------
    // load health configuration

    if(host->health_enabled) {
        rrdhost_wrlock(host);
        health_readdir(host, health_user_config_dir(), health_stock_config_dir(), NULL);
        rrdhost_unlock(host);
    }

    RRDHOST *t = rrdhost_index_add(host);

    if(t != host) {
        error("Host '%s': cannot add host with machine guid '%s' to index. It already exists as host '%s' with machine guid '%s'.", host->hostname, host->machine_guid, t->hostname, t->machine_guid);
        rrdhost_free(host);
        return NULL;
    }

    if (likely(!uuid_parse(host->machine_guid, host->host_uuid))) {
        int rc = sql_store_host(&host->host_uuid, hostname, registry_hostname, update_every, os, timezone, tags);
        if (unlikely(rc))
            error_report("Failed to store machine GUID to the database");
        sql_load_node_id(host);
        if (host->health_enabled) {
            if (!file_is_migrated(host->health_log_filename)) {
                int rc = sql_create_health_log_table(host);
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
            error("Host '%s': cannot create directory '%s'", host->hostname, dbenginepath);
        else ret = 0; // succeed
        if (is_legacy) // initialize legacy dbengine instance as needed
            ret = rrdeng_init(host, &host->rrdeng_ctx, dbenginepath, default_rrdeng_page_cache_mb,
                              default_rrdeng_disk_quota_mb); // may fail here for legacy dbengine initialization
        else
            host->rrdeng_ctx = &multidb_ctx;
        if (ret) { // check legacy or multihost initialization success
            error(
                "Host '%s': cannot initialize host with machine guid '%s'. Failed to initialize DB engine at '%s'.",
                host->hostname, host->machine_guid, host->cache_dir);
            rrdhost_free(host);
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
        host->rrdeng_ctx = &multidb_ctx;
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
    }

    ml_new_host(host);

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
         , host->hostname
         , host->registry_hostname
         , host->machine_guid
         , host->os
         , host->timezone
         , (host->tags)?host->tags:""
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
         , host->health_default_exec
         , host->health_default_recipient
    );
    
#ifdef  ENABLE_REPLICATION
    // ------------------------------------------------------------------------
    //GAPs struct initialization only for child hosts
    if(strcmp(host->machine_guid, localhost->machine_guid))
        gaps_init(&host);
    host->replication = (REPLICATION *)callocz(1, sizeof(REPLICATION));
    replication_sender_init(host);
#endif  //ENABLE_REPLICATION

    rrd_hosts_available++;

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

    rrdhost_init_os(host, os);
    rrdhost_init_timezone(host, timezone, abbrev_timezone, utc_offset);

    freez(host->registry_hostname);
    host->registry_hostname = strdupz((registry_hostname && *registry_hostname)?registry_hostname:hostname);

    if(strcmp(host->hostname, hostname) != 0) {
        info("Host '%s' has been renamed to '%s'. If this is not intentional it may mean multiple hosts are using the same machine_guid.", host->hostname, hostname);
        char *t = host->hostname;
        host->hostname = strdupz(hostname);
        host->hash_hostname = simple_hash(host->hostname);
        freez(t);
    }

    if(strcmp(host->program_name, program_name) != 0) {
        info("Host '%s' switched program name from '%s' to '%s'", host->hostname, host->program_name, program_name);
        char *t = host->program_name;
        host->program_name = strdupz(program_name);
        freez(t);
    }

    if(strcmp(host->program_version, program_version) != 0) {
        info("Host '%s' switched program version from '%s' to '%s'", host->hostname, host->program_version, program_version);
        char *t = host->program_version;
        host->program_version = strdupz(program_version);
        freez(t);
    }

    if(host->rrd_update_every != update_every)
        error("Host '%s' has an update frequency of %d seconds, but the wanted one is %d seconds. Restart netdata here to apply the new settings.", host->hostname, host->rrd_update_every, update_every);

    if(host->rrd_history_entries < history)
        error("Host '%s' has history of %ld entries, but the wanted one is %ld entries. Restart netdata here to apply the new settings.", host->hostname, host->rrd_history_entries, history);

    if(host->rrd_memory_mode != mode)
        error("Host '%s' has memory mode '%s', but the wanted one is '%s'. Restart netdata here to apply the new settings.", host->hostname, rrd_memory_mode_name(host->rrd_memory_mode), rrd_memory_mode_name(mode));

    // update host tags
    rrdhost_init_tags(host, tags);

    if (rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {
        rrdhost_flag_clear(host, RRDHOST_FLAG_ARCHIVED);
        if(host->health_enabled) {
            int r;
            char filename[FILENAME_MAX + 1];

            if (host != localhost) {
                r = mkdir(host->varlib_dir, 0775);
                if (r != 0 && errno != EEXIST)
                    error("Host '%s': cannot create directory '%s'", host->hostname, host->varlib_dir);
            }
            snprintfz(filename, FILENAME_MAX, "%s/health", host->varlib_dir);
            r = mkdir(filename, 0775);
            if(r != 0 && errno != EEXIST)
                error("Host '%s': cannot create directory '%s'", host->hostname, filename);

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
        info("Host %s is not in archived mode anymore", host->hostname);
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
) {
    debug(D_RRDHOST, "Searching for host '%s' with guid '%s'", hostname, guid);

    rrd_wrlock();
    RRDHOST *host = rrdhost_find_by_guid(guid, 0);
    if (unlikely(host && RRD_MEMORY_MODE_DBENGINE != mode && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) {
        /* If a legacy memory mode instantiates all dbengine state must be discarded to avoid inconsistencies */
        error("Archived host '%s' has memory mode '%s', but the wanted one is '%s'. Discarding archived state.",
              host->hostname, rrd_memory_mode_name(host->rrd_memory_mode), rrd_memory_mode_name(mode));
        rrdhost_free(host);
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
            info("Host '%s' with machine guid '%s' is obsolete - cleaning up.", host->hostname, host->machine_guid);

            if (rrdhost_flag_check(host, RRDHOST_FLAG_DELETE_ORPHAN_HOST)
#ifdef ENABLE_DBENGINE
                /* don't delete multi-host DB host files */
                && !(host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && host->rrdeng_ctx == &multidb_ctx)
#endif
            )
                rrdhost_delete_charts(host);
            else
                rrdhost_save_charts(host);

            rrdhost_free(host);
            goto restart_after_removal;
        }
    }
}

// ----------------------------------------------------------------------------
// RRDHOST global / startup initialization

int rrd_init(char *hostname, struct rrdhost_system_info *system_info) {
    rrdset_free_obsolete_time = config_get_number(CONFIG_SECTION_GLOBAL, "cleanup obsolete charts after seconds", rrdset_free_obsolete_time);
    // Current chart locking and invalidation scheme doesn't prevent Netdata from segmentation faults if a short
    // cleanup delay is set. Extensive stress tests showed that 10 seconds is quite a safe delay. Look at
    // https://github.com/netdata/netdata/pull/11222#issuecomment-868367920 for more information.
    if (rrdset_free_obsolete_time < 10) {
        rrdset_free_obsolete_time = 10;
        info("The \"cleanup obsolete charts after seconds\" option was set to 10 seconds. A lower delay can potentially cause a segmentation fault.");
    }
    gap_when_lost_iterations_above = (int)config_get_number(CONFIG_SECTION_GLOBAL, "gap when lost iterations above", gap_when_lost_iterations_above);
    if (gap_when_lost_iterations_above < 1)
        gap_when_lost_iterations_above = 1;

    if (unlikely(sql_init_database(DB_CHECK_NONE))) {
        if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE)
            fatal("Failed to initialize SQLite");
        info("Skipping SQLITE metadata initialization since memory mode is not db engine");
    }

    health_init();

    rrdpush_init();

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
    );
    if (unlikely(!localhost)) {
        rrd_unlock();
        return 1;
    }

#ifdef ENABLE_DBENGINE
    char dbenginepath[FILENAME_MAX + 1];
    int ret;
    snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", localhost->cache_dir);
    ret = mkdir(dbenginepath, 0775);
    if (ret != 0 && errno != EEXIST)
        error("Host '%s': cannot create directory '%s'", localhost->hostname, dbenginepath);
    else  // Unconditionally create multihost db to support on demand host creation
        ret = rrdeng_init(NULL, NULL, dbenginepath, default_rrdeng_page_cache_mb, default_multidb_disk_quota_mb);
    if (ret) {
        error(
            "Host '%s' with machine guid '%s' failed to initialize multi-host DB engine instance at '%s'.",
            localhost->hostname, localhost->machine_guid, localhost->cache_dir);
        rrdhost_free(localhost);
        localhost = NULL;
        rrd_unlock();
        fatal("Failed to initialize dbengine");
    }
#endif
    sql_aclk_sync_init();
    rrd_unlock();

    web_client_api_v1_management_init();
    return localhost==NULL;
}

// ----------------------------------------------------------------------------
// RRDHOST - lock validations
// there are only used when NETDATA_INTERNAL_CHECKS is set

void __rrdhost_check_rdlock(RRDHOST *host, const char *file, const char *function, const unsigned long line) {
    debug(D_RRDHOST, "Checking read lock on host '%s'", host->hostname);

    int ret = netdata_rwlock_trywrlock(&host->rrdhost_rwlock);
    if(ret == 0)
        fatal("RRDHOST '%s' should be read-locked, but it is not, at function %s() at line %lu of file '%s'", host->hostname, function, line, file);
}

void __rrdhost_check_wrlock(RRDHOST *host, const char *file, const char *function, const unsigned long line) {
    debug(D_RRDHOST, "Checking write lock on host '%s'", host->hostname);

    int ret = netdata_rwlock_tryrdlock(&host->rrdhost_rwlock);
    if(ret == 0)
        fatal("RRDHOST '%s' should be write-locked, but it is not, at function %s() at line %lu of file '%s'", host->hostname, function, line, file);
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
void rrdhost_free(RRDHOST *host) {
    if(!host) return;

    info("Freeing all memory for host '%s'...", host->hostname);

    rrd_check_wrlock();     // make sure the RRDs are write locked

    rrdhost_wrlock(host);
    ml_delete_host(host);
    rrdhost_unlock(host);

    // ------------------------------------------------------------------------
#ifdef  ENABLE_REPLICATION
    replication_sender_thread_stop(host); // stop a possibly running Tx replication thread and clean-up the state of the REP Tx thread.
#endif
    rrdpush_sender_thread_stop(host); // stop a possibly running thread
    cbuffer_free(host->sender->buffer);
    buffer_free(host->sender->build);
#ifdef ENABLE_COMPRESSION
    if (host->sender->compressor)
        host->sender->compressor->destroy(&host->sender->compressor);
#endif
    freez(host->sender);
    host->sender = NULL;
    if (netdata_exit) {
        netdata_mutex_lock(&host->receiver_lock);
        if (host->receiver) {
            if(!host->receiver->exited)
                netdata_thread_cancel(host->receiver->thread);
            netdata_mutex_unlock(&host->receiver_lock);
            struct receiver_state *rpt = host->receiver;
            while (host->receiver && !rpt->exited)
                sleep_usec(50 * USEC_PER_MS);
            // If the receiver detached from the host then its thread will destroy the state
            if (host->receiver == rpt)
                destroy_receiver_state(host->receiver);
        }
        else
            netdata_mutex_unlock(&host->receiver_lock);
    }
#ifdef  ENABLE_REPLICATION
    if(strcmp(host->machine_guid, localhost->machine_guid))
        gaps_destroy(&host);
#endif  //ENABLE_REPLICATION 

    rrdhost_wrlock(host);   // lock this RRDHOST
#if defined(ENABLE_ACLK) && defined(ENABLE_NEW_CLOUD_PROTOCOL)
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
    // release its children resources

#ifdef ENABLE_DBENGINE
    if (host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
        if (host->rrdeng_ctx != &multidb_ctx)
            rrdeng_prepare_exit(host->rrdeng_ctx);
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

    debug(D_RRD_CALLS, "RRDHOST: Cleaning up remaining host variables for host '%s'", host->hostname);
    rrdvar_free_remaining_variables(host, &host->rrdvar_root_index);

    health_alarm_log_free(host);

#ifdef ENABLE_DBENGINE
    if (host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && host->rrdeng_ctx != &multidb_ctx)
        rrdeng_exit(host->rrdeng_ctx);
#endif

    // ------------------------------------------------------------------------
    // remove it from the indexes

    if(rrdhost_index_del(host) != host)
        error("RRDHOST '%s' removed from index, deleted the wrong entry.", host->hostname);


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
        else error("Request to free RRDHOST '%s': cannot find it", host->hostname);
    }



    // ------------------------------------------------------------------------
    // free it

    pthread_mutex_destroy(&host->aclk_state_lock);
    freez(host->aclk_state.claimed_id);
    freez(host->aclk_state.prev_claimed_id);
    freez((void *)host->tags);
    free_label_list(host->labels.head);
    freez((void *)host->os);
    freez((void *)host->timezone);
    freez((void *)host->abbrev_timezone);
    freez(host->program_version);
    freez(host->program_name);
    rrdhost_system_info_free(host->system_info);
    freez(host->cache_dir);
    freez(host->varlib_dir);
    freez(host->rrdpush_send_api_key);
    freez(host->rrdpush_send_destination);
    freez(host->health_default_exec);
    freez(host->health_default_recipient);
    freez(host->health_log_filename);
    freez(host->hostname);
    freez(host->registry_hostname);
    simple_pattern_free(host->rrdpush_send_charts_matching);
    rrdhost_unlock(host);
    netdata_rwlock_destroy(&host->labels.labels_rwlock);
    netdata_rwlock_destroy(&host->health_log.alarm_log_rwlock);
    netdata_rwlock_destroy(&host->rrdhost_rwlock);
    freez(host->node_id);

    freez(host);
#if defined(ENABLE_ACLK) && defined(ENABLE_NEW_CLOUD_PROTOCOL)
    if (wc)
        wc->is_orphan = 0;
#endif
    rrd_hosts_available--;
}

void rrdhost_free_all(void) {
    rrd_wrlock();
    /* Make sure child-hosts are released before the localhost. */
    while(localhost->next) rrdhost_free(localhost->next);
    rrdhost_free(localhost);
    rrd_unlock();
}

// ----------------------------------------------------------------------------
// RRDHOST - save host files

void rrdhost_save_charts(RRDHOST *host) {
    if(!host) return;

    info("Saving/Closing database of host '%s'...", host->hostname);

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

static struct label *rrdhost_load_auto_labels(void)
{
    struct label *label_list = NULL;

    if (localhost->system_info->cloud_provider_type)
        label_list =
            add_label_to_list(label_list, "_cloud_provider_type", localhost->system_info->cloud_provider_type, LABEL_SOURCE_AUTO);

    if (localhost->system_info->cloud_instance_type)
        label_list =
            add_label_to_list(label_list, "_cloud_instance_type", localhost->system_info->cloud_instance_type, LABEL_SOURCE_AUTO);

    if (localhost->system_info->cloud_instance_region)
        label_list =
            add_label_to_list(label_list, "_cloud_instance_region", localhost->system_info->cloud_instance_region, LABEL_SOURCE_AUTO);

    if (localhost->system_info->host_os_name)
        label_list =
            add_label_to_list(label_list, "_os_name", localhost->system_info->host_os_name, LABEL_SOURCE_AUTO);

    if (localhost->system_info->host_os_version)
        label_list =
            add_label_to_list(label_list, "_os_version", localhost->system_info->host_os_version, LABEL_SOURCE_AUTO);

    if (localhost->system_info->kernel_version)
        label_list =
            add_label_to_list(label_list, "_kernel_version", localhost->system_info->kernel_version, LABEL_SOURCE_AUTO);

    if (localhost->system_info->host_cores)
        label_list =
                add_label_to_list(label_list, "_system_cores", localhost->system_info->host_cores, LABEL_SOURCE_AUTO);

    if (localhost->system_info->host_cpu_freq)
        label_list =
                add_label_to_list(label_list, "_system_cpu_freq", localhost->system_info->host_cpu_freq, LABEL_SOURCE_AUTO);

    if (localhost->system_info->host_ram_total)
        label_list =
                add_label_to_list(label_list, "_system_ram_total", localhost->system_info->host_ram_total, LABEL_SOURCE_AUTO);

    if (localhost->system_info->host_disk_space)
        label_list =
                add_label_to_list(label_list, "_system_disk_space", localhost->system_info->host_disk_space, LABEL_SOURCE_AUTO);

    if (localhost->system_info->architecture)
        label_list =
            add_label_to_list(label_list, "_architecture", localhost->system_info->architecture, LABEL_SOURCE_AUTO);

    if (localhost->system_info->virtualization)
        label_list =
            add_label_to_list(label_list, "_virtualization", localhost->system_info->virtualization, LABEL_SOURCE_AUTO);

    if (localhost->system_info->container)
        label_list =
            add_label_to_list(label_list, "_container", localhost->system_info->container, LABEL_SOURCE_AUTO);

    if (localhost->system_info->container_detection)
        label_list =
            add_label_to_list(label_list, "_container_detection", localhost->system_info->container_detection, LABEL_SOURCE_AUTO);

    if (localhost->system_info->virt_detection)
        label_list =
            add_label_to_list(label_list, "_virt_detection", localhost->system_info->virt_detection, LABEL_SOURCE_AUTO);

    if (localhost->system_info->is_k8s_node)
        label_list =
            add_label_to_list(label_list, "_is_k8s_node", localhost->system_info->is_k8s_node, LABEL_SOURCE_AUTO);

    if (localhost->system_info->install_type)
        label_list =
            add_label_to_list(label_list, "_install_type", localhost->system_info->install_type, LABEL_SOURCE_AUTO);

    if (localhost->system_info->prebuilt_arch)
        label_list =
            add_label_to_list(label_list, "_prebuilt_arch", localhost->system_info->prebuilt_arch, LABEL_SOURCE_AUTO);

    if (localhost->system_info->prebuilt_dist)
        label_list =
            add_label_to_list(label_list, "_prebuilt_dist", localhost->system_info->prebuilt_dist, LABEL_SOURCE_AUTO);

    label_list = add_aclk_host_labels(label_list);

    label_list = add_label_to_list(
        label_list, "_is_parent", (localhost->next || configured_as_parent()) ? "true" : "false", LABEL_SOURCE_AUTO);

    if (localhost->rrdpush_send_destination)
        label_list =
            add_label_to_list(label_list, "_streams_to", localhost->rrdpush_send_destination, LABEL_SOURCE_AUTO);

    return label_list;
}

static inline int rrdhost_is_valid_label_config_option(char *name, char *value)
{
    return (is_valid_label_key(name) && is_valid_label_value(value) && strcmp(name, "from environment") &&
            strcmp(name, "from kubernetes pods"));
}

static struct label *rrdhost_load_config_labels()
{
    int status = config_load(NULL, 1, CONFIG_SECTION_HOST_LABEL);
    if(!status) {
        char *filename = CONFIG_DIR "/" CONFIG_FILENAME;
        error("LABEL: Cannot reload the configuration file '%s', using labels in memory", filename);
    }

    struct label *l = NULL;
    struct section *co = appconfig_get_section(&netdata_config, CONFIG_SECTION_HOST_LABEL);
    if(co) {
        config_section_wrlock(co);
        struct config_option *cv;
        for(cv = co->values; cv ; cv = cv->next) {
            if(rrdhost_is_valid_label_config_option(cv->name, cv->value)) {
                l = add_label_to_list(l, cv->name, cv->value, LABEL_SOURCE_NETDATA_CONF);
                cv->flags |= CONFIG_VALUE_USED;
            } else {
                error("LABELS: It was not possible to create the label '%s' because it contains invalid character(s) or values."
                       , cv->name);
            }
        }
        config_section_unlock(co);
    }

    return l;
}

struct label *parse_simple_tags(
    struct label *label_list,
    const char *tags,
    char key_value_separator,
    char label_separator,
    STRIP_QUOTES_OPTION strip_quotes_from_key,
    STRIP_QUOTES_OPTION strip_quotes_from_value,
    SKIP_ESCAPED_CHARACTERS_OPTION skip_escaped_characters)
{
    const char *end = tags;

    while (*end) {
        const char *start = end;
        char key[CONFIG_MAX_VALUE + 1];
        char value[CONFIG_MAX_VALUE + 1];

        while (*end && *end != key_value_separator)
            end++;
        strncpyz(key, start, end - start);

        if (*end)
            start = ++end;
        while (*end && *end != label_separator)
            end++;
        strncpyz(value, start, end - start);

        label_list = add_label_to_list(
            label_list,
            strip_quotes_from_key ? strip_double_quotes(trim(key), skip_escaped_characters) : trim(key),
            strip_quotes_from_value ? strip_double_quotes(trim(value), skip_escaped_characters) : trim(value),
            LABEL_SOURCE_NETDATA_CONF);

        if (*end)
            end++;
    }

    return label_list;
}

struct label *parse_json_tags(struct label *label_list, const char *tags)
{
    char tags_buf[CONFIG_MAX_VALUE + 1];
    strncpy(tags_buf, tags, CONFIG_MAX_VALUE);
    char *str = tags_buf;

    switch (*str) {
    case '{':
        str++;
        strip_last_symbol(str, '}', SKIP_ESCAPED_CHARACTERS);

        label_list = parse_simple_tags(label_list, str, ':', ',', STRIP_QUOTES, STRIP_QUOTES, SKIP_ESCAPED_CHARACTERS);

        break;
    case '[':
        str++;
        strip_last_symbol(str, ']', SKIP_ESCAPED_CHARACTERS);

        char *end = str + strlen(str);
        size_t i = 0;

        while (str < end) {
            char key[CONFIG_MAX_VALUE + 1];
            snprintfz(key, CONFIG_MAX_VALUE, "host_tag%zu", i);

            str = strip_double_quotes(trim(str), SKIP_ESCAPED_CHARACTERS);

            label_list = add_label_to_list(label_list, key, str, LABEL_SOURCE_NETDATA_CONF);

            // skip to the next element in the array
            str += strlen(str) + 1;
            while (*str && *str != ',')
                str++;
            str++;
            i++;
        }

        break;
    case '"':
        label_list = add_label_to_list(
            label_list, "host_tag", strip_double_quotes(str, SKIP_ESCAPED_CHARACTERS), LABEL_SOURCE_NETDATA_CONF);
        break;
    default:
        label_list = add_label_to_list(label_list, "host_tag", str, LABEL_SOURCE_NETDATA_CONF);
        break;
    }

    return label_list;
}

static struct label *rrdhost_load_kubernetes_labels(void)
{
    struct label *l=NULL;
    char *label_script = mallocz(sizeof(char) * (strlen(netdata_configured_primary_plugins_dir) + strlen("get-kubernetes-labels.sh") + 2));
    sprintf(label_script, "%s/%s", netdata_configured_primary_plugins_dir, "get-kubernetes-labels.sh");
    if (unlikely(access(label_script, R_OK) != 0)) {
        error("Kubernetes pod label fetching script %s not found.",label_script);
        freez(label_script);
    } else {
        pid_t command_pid;

        debug(D_RRDHOST, "Attempting to fetch external labels via %s", label_script);

        FILE *fp = mypopen(label_script, &command_pid);
        if(fp) {
            int MAX_LINE_SIZE=300;
            char buffer[MAX_LINE_SIZE + 1];
            while (fgets(buffer, MAX_LINE_SIZE, fp) != NULL) {
                char *name=buffer;
                char *value=buffer;
                while (*value && *value != ':') value++;
                if (*value == ':') {
                    *value = '\0';
                    value++;
                }
                char *eos=value;
                while (*eos && *eos != '\n') eos++;
                if (*eos == '\n') *eos = '\0';
                if (strlen(value)>0) {
                    if (is_valid_label_key(name)){
                        l = add_label_to_list(l, name, value, LABEL_SOURCE_KUBERNETES);
                    } else {
                        info("Ignoring invalid label name '%s'", name);
                    }
                } else {
                    error("%s outputted unexpected result: '%s'", label_script, name);
                }
            };
            // Non-zero exit code means that all the script output is error messages. We've shown already any message that didn't include a ':'
            // Here we'll inform with an ERROR that the script failed, show whatever (if anything) was added to the list of labels, free the memory and set the return to null
            int retcode=mypclose(fp, command_pid);
            if (retcode) {
                error("%s exited abnormally. No kubernetes labels will be added to the host.", label_script);
                struct label *ll=l;
                while (ll != NULL) {
                    info("Ignoring Label [source id=%s]: \"%s\" -> \"%s\"\n", translate_label_source(ll->label_source), ll->key, ll->value);
                    ll = ll->next;
                    freez(l);
                    l=ll;
                }
            }
        }
        freez(label_script);
    }

    return l;
}

void reload_host_labels(void)
{
    struct label *from_auto = rrdhost_load_auto_labels();
    struct label *from_k8s = rrdhost_load_kubernetes_labels();
    struct label *from_config = rrdhost_load_config_labels();

    struct label *new_labels = merge_label_lists(from_auto, from_k8s);
    new_labels = merge_label_lists(new_labels, from_config);

    rrdhost_rdlock(localhost);
    replace_label_list(&localhost->labels, new_labels);

    health_label_log_save(localhost);
    rrdhost_unlock(localhost);

/*  TODO-GAPS - fix this so that it looks properly at the state and version of the sender
    if(localhost->rrdpush_send_enabled && localhost->rrdpush_sender_buffer){
        localhost->labels.labels_flag |= LABEL_FLAG_UPDATE_STREAM;
        rrdpush_send_labels(localhost);
    }
*/
    health_reload();
}

// ----------------------------------------------------------------------------
// RRDHOST - delete host files

void rrdhost_delete_charts(RRDHOST *host) {
    if(!host) return;

    info("Deleting database of host '%s'...", host->hostname);

    RRDSET *st;

    // we get a write lock
    // to ensure only one thread is saving the database
    rrdhost_wrlock(host);

    rrdset_foreach_write(st, host) {
        rrdset_rdlock(st);
        rrdset_delete(st);
        rrdset_unlock(st);
    }

    recursively_delete_dir(host->cache_dir, "left over host");

    rrdhost_unlock(host);
}

// ----------------------------------------------------------------------------
// RRDHOST - cleanup host files

void rrdhost_cleanup_charts(RRDHOST *host) {
    if(!host) return;

    info("Cleaning up database of host '%s'...", host->hostname);

    RRDSET *st;
    uint32_t rrdhost_delete_obsolete_charts = rrdhost_flag_check(host, RRDHOST_FLAG_DELETE_OBSOLETE_CHARTS);

    // we get a write lock
    // to ensure only one thread is saving the database
    rrdhost_wrlock(host);

    rrdset_foreach_write(st, host) {
        rrdset_rdlock(st);

        if(rrdhost_delete_obsolete_charts && rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE))
            rrdset_delete(st);
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
            && !(host->rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE && host->rrdeng_ctx == &multidb_ctx)
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
                        uint8_t can_delete_metric = rd->state->collect_ops.finalize(rd);
                        if (can_delete_metric) {
                            /* This metric has no data and no references */
                            delete_dimension_uuid(&rd->state->metric_uuid);
                            rrddim_free(st, rd);
                            if (unlikely(!last)) {
                                rd = st->dimensions;
                            }
                            else {
                                rd = last->next;
                            }
                            continue;
                        }
#if defined(ENABLE_ACLK) && defined(ENABLE_NEW_CLOUD_PROTOCOL)
                        else
                            queue_dimension_to_aclk(rd);
#endif
                    }
                    last = rd;
                    rd = rd->next;
                }
                rrdset_unlock(st);

                debug(D_RRD_CALLS, "RRDSET: Cleaning up remaining chart variables for host '%s', chart '%s'", host->hostname, st->id);
                rrdvar_free_remaining_variables(host, &st->rrdvar_root_index);

                rrdset_flag_clear(st, RRDSET_FLAG_OBSOLETE);
                
                if (st->dimensions) {
                    /* If the chart still has dimensions don't delete it from the metadata log */
                    continue;
                }
            }
#endif
            rrdset_rdlock(st);

            if(rrdhost_delete_obsolete_charts)
                rrdset_delete(st);
            else
                rrdset_save(st);

            rrdset_unlock(st);

            rrdset_free(st);
            goto restart_after_removal;
        }
#if defined(ENABLE_ACLK) && defined(ENABLE_NEW_CLOUD_PROTOCOL)
        else
            sql_check_chart_liveness(st);
#endif
    }
}

void rrdset_check_obsoletion(RRDHOST *host)
{
    RRDSET *st;
    rrdset_foreach_write(st, host) {
        if (rrdset_last_entry_t(st) < host->trigger_chart_obsoletion_check) {
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
#ifdef ENABLE_ACLK
            host->deleted_charts_count = 0;
#endif
            rrdhost_cleanup_obsolete_charts(host);
#ifdef ENABLE_ACLK
            if (host->deleted_charts_count)
                aclk_update_chart(host, "dummy-chart", 0);
#endif
            rrdhost_unlock(host);
        }

        if (host != localhost &&
            host->trigger_chart_obsoletion_check &&
            host->trigger_chart_obsoletion_check + 120 < now_realtime_sec()) {
            rrdhost_wrlock(host);
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
    RRDCALC *in1 = (RRDCALC *)a;
    RRDCALC *in2 = (RRDCALC *)b;

    if(in1->hash < in2->hash) return -1;
    else if(in1->hash > in2->hash) return 1;

    return strcmp(in1->name,in2->name);
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

time_t rrdhost_first_entry_t(RRDHOST *h) {
    rrdhost_rdlock(h);
    RRDSET *st;
    time_t result = LONG_MAX;
    rrdset_foreach_read(st, h) {
        time_t st_first = rrdset_first_entry_t(st);
        if (st_first < result)
            result = st_first;
    }
    rrdhost_unlock(h);
    return result;    
}
