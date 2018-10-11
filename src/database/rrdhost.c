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

    return (RRDHOST *)avl_search_lock(&(rrdhost_root_index), (avl *) &tmp);
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

#define rrdhost_index_add(rrdhost) (RRDHOST *)avl_insert_lock(&(rrdhost_root_index), (avl *)(rrdhost))
#define rrdhost_index_del(rrdhost) (RRDHOST *)avl_remove_lock(&(rrdhost_root_index), (avl *)(rrdhost))


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

static inline void rrdhost_init_timezone(RRDHOST *host, const char *timezone) {
    if(host->timezone && timezone && !strcmp(host->timezone, timezone))
        return;

    void *old = (void *)host->timezone;
    host->timezone = strdupz((timezone && *timezone)?timezone:"unknown");
    freez(old);
}

static inline void rrdhost_init_machine_guid(RRDHOST *host, const char *machine_guid) {
    strncpy(host->machine_guid, machine_guid, GUID_LEN);
    host->machine_guid[GUID_LEN] = '\0';
    host->hash_machine_guid = simple_hash(host->machine_guid);
}


// ----------------------------------------------------------------------------
// RRDHOST - add a host

RRDHOST *rrdhost_create(const char *hostname,
                        const char *registry_hostname,
                        const char *guid,
                        const char *os,
                        const char *timezone,
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
                        int is_localhost
) {
    debug(D_RRDHOST, "Host '%s': adding with guid '%s'", hostname, guid);

    rrd_check_wrlock();

    RRDHOST *host = callocz(1, sizeof(RRDHOST));

    host->rrd_update_every    = (update_every > 0)?update_every:1;
    host->rrd_history_entries = align_entries_to_pagesize(memory_mode, entries);
    host->rrd_memory_mode     = memory_mode;
    host->health_enabled      = (memory_mode == RRD_MEMORY_MODE_NONE)? 0 : health_enabled;
    host->rrdpush_send_enabled     = (rrdpush_enabled && rrdpush_destination && *rrdpush_destination && rrdpush_api_key && *rrdpush_api_key) ? 1 : 0;
    host->rrdpush_send_destination = (host->rrdpush_send_enabled)?strdupz(rrdpush_destination):NULL;
    host->rrdpush_send_api_key     = (host->rrdpush_send_enabled)?strdupz(rrdpush_api_key):NULL;

    host->rrdpush_sender_pipe[0] = -1;
    host->rrdpush_sender_pipe[1] = -1;
    host->rrdpush_sender_socket  = -1;

    netdata_mutex_init(&host->rrdpush_sender_buffer_mutex);
    netdata_rwlock_init(&host->rrdhost_rwlock);

    rrdhost_init_hostname(host, hostname);
    rrdhost_init_machine_guid(host, guid);
    rrdhost_init_os(host, os);
    rrdhost_init_timezone(host, timezone);
    rrdhost_init_tags(host, tags);

    host->program_name = strdupz((program_name && *program_name)?program_name:"unknown");
    host->program_version = strdupz((program_version && *program_version)?program_version:"unknown");
    host->registry_hostname = strdupz((registry_hostname && *registry_hostname)?registry_hostname:hostname);

    avl_init_lock(&(host->rrdset_root_index),      rrdset_compare);
    avl_init_lock(&(host->rrdset_root_index_name), rrdset_compare_name);
    avl_init_lock(&(host->rrdfamily_root_index),   rrdfamily_compare);
    avl_init_lock(&(host->rrdvar_root_index),   rrdvar_compare);

    if(config_get_boolean(CONFIG_SECTION_GLOBAL, "delete obsolete charts files", 1))
        rrdhost_flag_set(host, RRDHOST_FLAG_DELETE_OBSOLETE_CHARTS);

    if(config_get_boolean(CONFIG_SECTION_GLOBAL, "delete orphan hosts files", 1) && !is_localhost)
        rrdhost_flag_set(host, RRDHOST_FLAG_DELETE_ORPHAN_HOST);


    // ------------------------------------------------------------------------
    // initialize health variables

    host->health_log.next_log_id = 1;
    host->health_log.next_alarm_id = 1;
    host->health_log.max = 1000;
    host->health_log.next_log_id =
    host->health_log.next_alarm_id = (uint32_t)now_realtime_sec();

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

        snprintfz(filename, FILENAME_MAX, "%s/%s", netdata_configured_cache_dir, host->machine_guid);
        host->cache_dir = strdupz(filename);

        if(host->rrd_memory_mode == RRD_MEMORY_MODE_MAP || host->rrd_memory_mode == RRD_MEMORY_MODE_SAVE) {
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

    snprintfz(filename, FILENAME_MAX, "%s/alarm-notify.sh", netdata_configured_plugins_dir);
    host->health_default_exec = strdupz(config_get(CONFIG_SECTION_HEALTH, "script to execute on alarm", filename));
    host->health_default_recipient = strdup("root");


    // ------------------------------------------------------------------------
    // load health configuration

    if(host->health_enabled) {
        health_alarm_log_load(host);
        health_alarm_log_open(host);

        rrdhost_wrlock(host);
        health_readdir(host, health_user_config_dir(), health_stock_config_dir(), NULL);
        rrdhost_unlock(host);
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

    RRDHOST *t = rrdhost_index_add(host);

    if(t != host) {
        error("Host '%s': cannot add host with machine guid '%s' to index. It already exists as host '%s' with machine guid '%s'.", host->hostname, host->machine_guid, t->hostname, t->machine_guid);
        rrdhost_free(host);
        host = NULL;
    }
    else {
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
    }

    rrd_hosts_available++;

    return host;
}

RRDHOST *rrdhost_find_or_create(
          const char *hostname
        , const char *registry_hostname
        , const char *guid
        , const char *os
        , const char *timezone
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
) {
    debug(D_RRDHOST, "Searching for host '%s' with guid '%s'", hostname, guid);

    rrd_wrlock();
    RRDHOST *host = rrdhost_find_by_guid(guid, 0);
    if(!host) {
        host = rrdhost_create(
                hostname
                , registry_hostname
                , guid
                , os
                , timezone
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
                , 0
        );
    }
    else {
        host->health_enabled = health_enabled;

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
    }

    rrdhost_cleanup_orphan_hosts_nolock(host);

    rrd_unlock();

    return host;
}

inline int rrdhost_should_be_removed(RRDHOST *host, RRDHOST *protected, time_t now) {
    if(host != protected
       && host != localhost
       && rrdhost_flag_check(host, RRDHOST_FLAG_ORPHAN)
       && !host->connected_senders
       && host->senders_disconnected_time
       && host->senders_disconnected_time + rrdhost_free_orphan_time < now)
        return 1;

    return 0;
}

void rrdhost_cleanup_orphan_hosts_nolock(RRDHOST *protected) {
    time_t now = now_realtime_sec();

    RRDHOST *host;

restart_after_removal:
    rrdhost_foreach_write(host) {
        if(rrdhost_should_be_removed(host, protected, now)) {
            info("Host '%s' with machine guid '%s' is obsolete - cleaning up.", host->hostname, host->machine_guid);

            if(rrdhost_flag_check(host, RRDHOST_FLAG_DELETE_ORPHAN_HOST))
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

void rrd_init(char *hostname) {
    rrdset_free_obsolete_time = config_get_number(CONFIG_SECTION_GLOBAL, "cleanup obsolete charts after seconds", rrdset_free_obsolete_time);
    gap_when_lost_iterations_above = (int)config_get_number(CONFIG_SECTION_GLOBAL, "gap when lost iterations above", gap_when_lost_iterations_above);
    if(gap_when_lost_iterations_above < 1)
        gap_when_lost_iterations_above = 1;

    health_init();
    registry_init();
    rrdpush_init();

    debug(D_RRDHOST, "Initializing localhost with hostname '%s'", hostname);
    rrd_wrlock();
    localhost = rrdhost_create(
            hostname
            , registry_get_this_machine_hostname()
            , registry_get_this_machine_guid()
            , os_type
            , netdata_configured_timezone
            , config_get(CONFIG_SECTION_BACKEND, "host tags", "")
            , program_name
            , program_version
            , default_rrd_update_every
            , default_rrd_history_entries
            , default_rrd_memory_mode
            , default_health_enabled
            , default_rrdpush_enabled
            , default_rrdpush_destination
            , default_rrdpush_api_key
            , 1
    );
    rrd_unlock();
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

void rrdhost_free(RRDHOST *host) {
    if(!host) return;

    info("Freeing all memory for host '%s'...", host->hostname);

    rrd_check_wrlock();     // make sure the RRDs are write locked

    // stop a possibly running thread
    rrdpush_sender_thread_stop(host);

    rrdhost_wrlock(host);   // lock this RRDHOST

    // ------------------------------------------------------------------------
    // release its children resources

    while(host->rrdset_root)
        rrdset_free(host->rrdset_root);

    while(host->alarms)
        rrdcalc_unlink_and_free(host, host->alarms);

    while(host->templates)
        rrdcalctemplate_unlink_and_free(host, host->templates);

    debug(D_RRD_CALLS, "RRDHOST: Cleaning up remaining host variables for host '%s'", host->hostname);
    rrdvar_free_remaining_variables(host, &host->rrdvar_root_index);

    health_alarm_log_free(host);

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

    freez((void *)host->tags);
    freez((void *)host->os);
    freez((void *)host->timezone);
    freez(host->program_version);
    freez(host->program_name);
    freez(host->cache_dir);
    freez(host->varlib_dir);
    freez(host->rrdpush_send_api_key);
    freez(host->rrdpush_send_destination);
    freez(host->health_default_exec);
    freez(host->health_default_recipient);
    freez(host->health_log_filename);
    freez(host->hostname);
    freez(host->registry_hostname);
    rrdhost_unlock(host);
    netdata_rwlock_destroy(&host->health_log.alarm_log_rwlock);
    netdata_rwlock_destroy(&host->rrdhost_rwlock);
    freez(host);

    rrd_hosts_available--;
}

void rrdhost_free_all(void) {
    rrd_wrlock();
    while(localhost) rrdhost_free(localhost);
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
        if(host != localhost && rrdhost_flag_check(host, RRDHOST_FLAG_DELETE_OBSOLETE_CHARTS) && !host->connected_senders)
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

            rrdset_rdlock(st);

            if(rrdhost_delete_obsolete_charts)
                rrdset_delete(st);
            else
                rrdset_save(st);

            rrdset_unlock(st);

            rrdset_free(st);
            goto restart_after_removal;
        }
    }
}
