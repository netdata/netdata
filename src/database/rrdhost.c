// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

#if RRD_STORAGE_TIERS != 5
#error RRD_STORAGE_TIERS is not 5 - you need to update the grouping iterations per tier
#endif

RRDHOST *localhost = NULL;
netdata_rwlock_t rrd_rwlock;

static void __attribute__((constructor)) init_lock(void) {
    netdata_rwlock_init(&rrd_rwlock);
}

static void __attribute__((destructor)) destroy_lock(void) {
    netdata_rwlock_destroy(&rrd_rwlock);
}

RRDHOST *rrdhost_find_by_node_id(const char *node_id) {

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

// Runs `cb` on the host with this machine_guid while holding the rrd read lock, so the host and
// everything hanging off it stay allocated for the duration of the callback. For callers that hold no
// reference to the host.
//
// The rrd read lock - NOT the host index lock - is what provides the lifetime. The index lock does
// not: dict_item_del() takes only the index write lock, and for an item a traversal is referencing
// it merely flags it deleted and returns without waiting (dictionary-item.h). rrdhost_root_index is
// also DICT_OPTION_VALUE_LINK_DONT_CLONE with no delete callback, so the dictionary never owns the
// RRDHOST and freez(host) is not serialized by it at all.
//
// What makes the rrd read lock sufficient is that every teardown removes the host from
// rrdhost_root_index under rrd_wrlock() before freeing it
// (rrdhost_unlink___while_having_rrd_wrlock(), then rrdhost_free_unlinked()). So a lookup performed
// while holding this read lock either happens before the unlink - and then the read lock blocks the
// writer from unlinking and freeing until `cb` returns - or after it, and finds nothing. That covers
// rrdhost_free___consume_metadata_lifetime_writelock() too, which frees with no lock held: by the
// time it gets there the host is already out of the index, so it can no longer be found here.
//
// `may_block` selects how the read lock is taken, and a caller that can run on more than one thread
// MUST decide it per call rather than once:
//   true  - wait for the lock. Correct for any thread nobody can be waiting on while holding
//           rrd_wrlock().
//   false - take it only if it is free, and skip the callback otherwise. Required on a thread that
//           an rrd_wrlock() holder may be blocked on: the ACLK sync event loop is one, because a host
//           teardown calls destroy_aclk_config() under rrd_wrlock() and waits for that loop. Waiting
//           for the read lock there would close the cycle and wedge the agent.
// With false, "not applied" is a normal outcome the caller MUST tolerate.
//
// Keyed by machine_guid, which is immutable for the host's lifetime and is this index's key, so the
// lookup is exact. Do NOT key such a callback on node_id: host->node_id and the ACLK config's
// node_id can disagree (command-nodeid.c assigns host->node_id from the parent without touching the
// config), so a node_id resolves to the wrong host or to none.
//
// `cb` runs with the rrd read lock held: keep it short, and do not take a lock from it that an
// rrd_wrlock() holder may be waiting behind.
//
// Returns whether `cb` ran - false when no host matched, and also when the lock was skipped.
bool rrdhost_apply_by_machine_guid(const char *machine_guid, void (*cb)(RRDHOST *host, void *data), void *data, bool may_block) {

    if (unlikely(!machine_guid || !*machine_guid || !cb))
        return false;

    if (may_block)
        rrd_rdlock();
    else if (rrd_tryrdlock() != 0)
        return false;

    RRDHOST *host = rrdhost_find_by_guid(machine_guid);
    if (host)
        cb(host, data);
    rrd_rdunlock();

    return host != NULL;
}

RRDHOST *rrdhost_find_by_hostname(const char *hostname) {
    if(unlikely(!hostname))
        return NULL;

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

// Caller must hold rrdhost_update_lock (or be in single-threaded host creation).
// Returns true if anything actually changed.
static inline bool rrdhost_init_timezone(RRDHOST *host, const char *timezone, const char *abbrev_timezone, int32_t utc_offset) {
    const char *cur_tz = host->timezone ? string2str(host->timezone) : NULL;
    const char *cur_abbrev = host->abbrev_timezone ? string2str(host->abbrev_timezone) : NULL;

    if (cur_tz && timezone && !strcmp(cur_tz, timezone) &&
        cur_abbrev && abbrev_timezone && !strcmp(cur_abbrev, abbrev_timezone) &&
        host->utc_offset == utc_offset)
        return false;

    STRING *old = host->timezone;
    host->timezone = string_strdupz((timezone && *timezone)?timezone:"unknown");
    string_freez(old);

    old = (void *)host->abbrev_timezone;
    host->abbrev_timezone = string_strdupz((abbrev_timezone && *abbrev_timezone) ? abbrev_timezone : "UTC");
    string_freez(old);

    host->utc_offset = utc_offset;
    return true;
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

bool rrdhost_update_timezone(RRDHOST *host, const char *timezone, const char *abbrev_timezone, int32_t utc_offset) {
    spinlock_lock(&host->rrdhost_update_lock);
    bool changed = rrdhost_init_timezone(host, timezone, abbrev_timezone, utc_offset);
    spinlock_unlock(&host->rrdhost_update_lock);

    if (changed)
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_UPDATE);

    return changed;
}

RRDHOST_TZ rrdhost_tz_get(RRDHOST *host) {
    RRDHOST_TZ tz;
    spinlock_lock(&host->rrdhost_update_lock);
    tz.timezone = strdupz(host->timezone ? string2str(host->timezone) : "unknown");
    tz.abbrev_timezone = strdupz(host->abbrev_timezone ? string2str(host->abbrev_timezone) : "UTC");
    tz.utc_offset = host->utc_offset;
    spinlock_unlock(&host->rrdhost_update_lock);
    return tz;
}

void rrdhost_tz_free(RRDHOST_TZ *tz) {
    freez(tz->timezone);
    freez(tz->abbrev_timezone);
    tz->timezone = NULL;
    tz->abbrev_timezone = NULL;
    tz->utc_offset = 0;
}

static inline RRDHOST_IDENTITY rrdhost_identity_acquire_unsafe(RRDHOST *host) {
    return (RRDHOST_IDENTITY) {
        .hostname = string_dup(host->hostname),
        .prog_name = string_dup(host->program_name),
        .prog_version = string_dup(host->program_version),
    };
}

RRDHOST_IDENTITY rrdhost_identity_acquire(RRDHOST *host) {
    spinlock_lock(&host->rrdhost_update_lock);
    RRDHOST_IDENTITY identity = rrdhost_identity_acquire_unsafe(host);
    spinlock_unlock(&host->rrdhost_update_lock);

    return identity;
}

void rrdhost_identity_release(RRDHOST_IDENTITY *identity) {
    string_freez(identity->hostname);
    string_freez(identity->prog_name);
    string_freez(identity->prog_version);
    identity->hostname = NULL;
    identity->prog_name = NULL;
    identity->prog_version = NULL;
}

RRDHOST_METADATA_IDENTITY rrdhost_metadata_identity_acquire(RRDHOST *host) {
    spinlock_lock(&host->rrdhost_update_lock);
    RRDHOST_METADATA_IDENTITY identity = {
        .common = rrdhost_identity_acquire_unsafe(host),
        .registry_hostname = string_dup(host->registry_hostname),
        .os = string_dup(host->os),
    };
    spinlock_unlock(&host->rrdhost_update_lock);

    return identity;
}

void rrdhost_metadata_identity_release(RRDHOST_METADATA_IDENTITY *identity) {
    rrdhost_identity_release(&identity->common);
    string_freez(identity->registry_hostname);
    string_freez(identity->os);
    identity->registry_hostname = NULL;
    identity->os = NULL;
}

// ----------------------------------------------------------------------------
// the host's nRPC function-registry owner vtable
//
// The nRPC component is host-agnostic: everything it needs from an RRDHOST is
// supplied here at registry-entry creation, and the component's synchronous
// disarm (inside nrpc_registry_destroy) guarantees none of it is used after
// the entry left the component index.
//
// HOW THIS HOST HONOURS THE NRPC_OWNER CONTRACT (see nrpc.h) - the component
// states the requirement, this is the proof for RRDHOST:
//
// - One token, one object: the token IS the RRDHOST pointer, and a host object
//   is never recycled for a different host while its entry lives.
//
// - destroy precedes freez(host): rrdhost_free_unlinked() calls
//   rrdhost_cleanup_data_collection_and_health() at its top and frees the host
//   at its bottom, and every free path funnels through it. That is what makes
//   address reuse harmless - a later RRDHOST allocated at the same address
//   finds no entry to inherit.
//
// WHAT THIS HOST DOES NOT GUARANTEE, deliberately recorded so nobody builds on
// the opposite: init and destroy CAN overlap for one host. The create-side
// init runs under rrd_wrlock, but the un-archive init (rrdhost_update(), below)
// does NOT - rrdhost_find_or_create() releases rrd_wrlock before calling it,
// and rrdhost_update_lock is the only lock it takes. The orphan reaper in
// svc_rrdhost_cleanup_orphan_hosts() holds rrd_wrlock and the host's
// metadata_lifetime_lock, neither of which excludes that init. So a child
// reconnecting to a long-archived host can un-archive it at the same moment
// the reaper tears it down.
//
// The component tolerates this - every interleaving is memory-safe, and the
// worst case is that the host comes out live with no function registry until
// its next archive/un-archive cycle. It is also not new: the same window
// existed when entries were keyed on the machine guid. Note that the same
// interleaving has a larger, pre-existing problem that has nothing to do with
// nRPC: rrdhost_update() writes into a host that rrdhost_free_unlinked() may be
// freeing. Fixing that serialization is what would close this properly.

static void rrdhost_nrpc_changed(NRPC_OWNER id, bool arm_manifest) {
    RRDHOST *host = rrdhost_from_nrpc_owner(id);

    rrdhost_flag_set(host, RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED);

    if(arm_manifest)
        aclk_arm_node_manifest(host);
}

static bool rrdhost_nrpc_wants_del_journal(NRPC_OWNER id) {
    return rrdhost_has_stream_sender_enabled(rrdhost_from_nrpc_owner(id));
}

void rrdhost_nrpc_registry_owner(RRDHOST *host, struct nrpc_registry_owner *owner) {
    *owner = (struct nrpc_registry_owner) {
        .id = rrdhost_nrpc_owner(host),
        .name = rrdhost_hostname(host),
        .epoch = &host->state_id,
        .changed = rrdhost_nrpc_changed,
        .wants_del_journal = rrdhost_nrpc_wants_del_journal,
    };
}

// ----------------------------------------------------------------------------
// RRDHOST - add a host

#ifdef ENABLE_DBENGINE
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
    spinlock_init(&host->rrdhost_update_lock);
    rw_spinlock_init(&host->metadata_lifetime_lock);
    rw_spinlock_init(&host->ml_host_rwlock);
    __atomic_store_n(&host->ml_running, false, __ATOMIC_RELAXED);
    spinlock_init(&host->aclk.spinlock);

    if (likely(!archived)) {
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

    // The function-registry entry is created only AFTER this host won the
    // machine-guid index insertion above, still under rrd_wrlock, so entry
    // existence tracks index membership atomically - which is what the ACLK
    // teardown ordering keys on. The entry is keyed on the host OBJECT, so a
    // dying same-guid predecessor that still holds its own entry is simply a
    // different key; nothing has to be taken over.
    if (likely(!archived)) {
        struct nrpc_registry_owner owner;
        rrdhost_nrpc_registry_owner(host, &owner);
        nrpc_registry_init(&owner);
    }

    rrd_wrunlock();

    // ------------------------------------------------------------------------

    RRDHOST_TZ host_tz = rrdhost_tz_get(host);
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
         , host_tz.timezone
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
    rrdhost_tz_free(&host_tz);

    if(!archived) {
        rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_INFO | RRDHOST_FLAG_METADATA_UPDATE);
        if (is_localhost)
            store_host_info_and_metadata(host);
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

    // Streaming children may omit the User-Agent header, leaving prog_name/prog_version NULL.
    // Match the defaults assigned on the host create path (for example in set_host_properties()/rrdhost_create())
    // so the strcmp/string_strdupz calls below are safe.
    if(!prog_name || !*prog_name)       prog_name = "unknown";
    if(!prog_version || !*prog_version) prog_version = "unknown";

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

        {
            struct nrpc_registry_owner owner;
            rrdhost_nrpc_registry_owner(host, &owner);
            nrpc_registry_init(&owner);
        }

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

        rrd_wrlock();
        host = rrdhost_find_by_guid(guid);
        if (host && host->rrd_memory_mode != mode && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED)) {
            if (likely(!archived && rrdhost_flag_check(host, RRDHOST_FLAG_PENDING_CONTEXT_LOAD))) {
                rrd_wrunlock();
                return host;
            }

            if (!rw_spinlock_trywrite_lock(&host->metadata_lifetime_lock)) {
                rrd_wrunlock();
                return NULL;
            }

            /* If a legacy memory mode instantiates all dbengine state must be discarded to avoid inconsistencies */
            nd_log(NDLS_DAEMON, NDLP_INFO,
                   "Archived host '%s' has memory mode '%s', but the wanted one is '%s'. Discarding archived state.",
                   rrdhost_hostname(host),
                   rrd_memory_mode_name(host->rrd_memory_mode),
                   rrd_memory_mode_name(mode));

            rw_spinlock_write_unlock(&host->metadata_lifetime_lock);
            rrdhost_free___while_having_rrd_wrlock(host);
            host = NULL;
        }
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

#ifdef NETDATA_INTERNAL_CHECKS
// Test-only pause point, compiled out of production builds. It blocks ONE claiming thread in the
// window between the accounting increment and the flag publish - the exact interleaving the claim-before-
// publish ordering exists to make safe - so a unit test can drive a deterministic race instead of a
// thread-soup that passes by luck. Same shape as the hooks in src/libnetdata/aral/aral.c.
static struct {
    RRDSET *st;             // fire only for this chart
    bool enabled;
    bool waiting;           // set by the paused claimer
    bool release;           // set by the test to let it continue
} rrdhost_receiver_replication_race_hook = { 0 };

void rrdhost_receiver_replication_race_hook_arm(RRDSET *st) {
    rrdhost_receiver_replication_race_hook.st = st;
    rrdhost_receiver_replication_race_hook.waiting = false;
    rrdhost_receiver_replication_race_hook.release = false;
    __atomic_store_n(&rrdhost_receiver_replication_race_hook.enabled, true, __ATOMIC_RELEASE);
}

bool rrdhost_receiver_replication_race_hook_is_waiting(void) {
    return __atomic_load_n(&rrdhost_receiver_replication_race_hook.waiting, __ATOMIC_ACQUIRE);
}

void rrdhost_receiver_replication_race_hook_release(void) {
    __atomic_store_n(&rrdhost_receiver_replication_race_hook.release, true, __ATOMIC_RELEASE);
}

void rrdhost_receiver_replication_race_hook_disarm(void) {
    __atomic_store_n(&rrdhost_receiver_replication_race_hook.enabled, false, __ATOMIC_RELEASE);
    rrdhost_receiver_replication_race_hook.st = NULL;
}

static ALWAYS_INLINE void rrdhost_receiver_replication_race_pause(RRDSET *st) {
    if(likely(!__atomic_load_n(&rrdhost_receiver_replication_race_hook.enabled, __ATOMIC_ACQUIRE)))
        return;

    if(rrdhost_receiver_replication_race_hook.st != st)
        return;

    __atomic_store_n(&rrdhost_receiver_replication_race_hook.waiting, true, __ATOMIC_RELEASE);
    while(!__atomic_load_n(&rrdhost_receiver_replication_race_hook.release, __ATOMIC_ACQUIRE))
        tinysleep();
}
#else
#define rrdhost_receiver_replication_race_pause(st) debug_dummy()
#endif

// Defined below, next to the release it shares its underflow guard with.
uint32_t rrdhost_receiver_replication_withdraw(RRDHOST *host);

// ONE implementation of the receiver-replication claim, used by the parser and by the tests.
// Returns true when THIS call caused the not-replicating -> replicating transition, i.e. when the
// caller now owns the chart's contribution.
//
// Ordering, which the whole accounting rests on: both accounting halves are incremented together, and
// only then is RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS published. Publishing first would make the
// flag visible while the contribution does not yet exist, and any release site would then decrement
// something nobody owns.
//
// The publish edge is atomic_flags_set_and_clear()'s CAS, which is __ATOMIC_RELEASE on success - that
// is what keeps the relaxed accounting add above from being reordered after it. The matching ACQUIRE
// is a fence in the release, NOT part of the flag op: the same helper's load is relaxed, so observing
// the flag does not on its own order the observer against this add on a weak-memory target.
bool rrdhost_receiver_replication_claim(RRDSET *st) {
    // `st` cannot be freed under us: svc_rrdset_lock_for_deletion() (daemon/service.c) frees a chart
    // only after last_accessed, last_updated and last_collected are ALL older than
    // rrdset_free_obsolete_time_s (default 3600s), and the CHART that put this chart in the parser's
    // scope touched it microseconds ago.
    RRDHOST *host = st->rrdhost;

    __atomic_add_fetch(&host->stream.rcv.status.replication.charts_started_and_remaining,
                       RRDHOST_RCV_REPLICATION_UNIT, __ATOMIC_RELAXED);

    rrdhost_receiver_replication_race_pause(st);

    RRDSET_FLAGS old = rrdset_flag_set_and_clear(
        st, RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED);

    if(old & RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS) {
        // Lost the race, or a duplicate CHART_DEFINITION_END: the chart already holds a contribution,
        // so ours is one too many. Withdraw our own increment - this is NOT a release of the chart's
        // contribution and deliberately does not clear the flag. Withdraw both speculative accounting
        // halves together, so a duplicate CHART_DEFINITION_END changes neither the cohort nor completion.
        // Checked like a release: our own increment keeps the outstanding half above zero, so a refusal
        // here means that reasoning has broken and a raw subtract would have eaten the cohort half.
        if(rrdhost_receiver_replication_withdraw(host) == 0)
            pulse_host_status(host, PULSE_HOST_STATUS_RCV_RUNNING, 0);

        return false;
    }

    // Decide the pulse AFTER the CAS, the way the pre-extraction code did: the winner asks whether the
    // host now holds exactly one outstanding contribution. Deciding it from this thread's own increment
    // BEFORE the CAS is wrong - a claimer can increment 0->1, be preempted, and lose the CAS to a second
    // claimer whose increment made it 2, so the winner would publish replication while nobody emits
    // RCV_REPLICATING.
    //
    // Residual, and NOT fixed here: this is still a read separated from the CAS, so a concurrent
    // duplicate claim that has added its unit but not yet withdrawn it makes the winner read 2 and
    // skip the pulse, while the duplicate publishes nothing when it withdraws. Concretely, the gauge
    // then stays on whatever the connection last published - RCV_REPLICATION_WAIT on a fresh
    // connection - until something else moves it, and the parent streaming charts read that gauge.
    // Bounded to the gauge, though: the accounting word, and therefore `InStatus`,
    // `InReplInstances`, `InReplCompletion` and the idle-disconnect suppression, all read the counter
    // and are unaffected. Fixing it needs the publish linearized with the CAS - a separate change,
    // because deciding the pulse from this thread's own increment instead is wrong for the reason
    // above.
    if(rrdhost_receiver_replicating_charts(host) == 1)
        pulse_host_status(host, PULSE_HOST_STATUS_RCV_REPLICATING, 0);

    return true;
}

// ONE implementation of the receiver-replication release, used by replay completion (both branches),
// the connect/disconnect reset, chart teardown, and the tests. `also_clear` carries any extra flags the
// caller wants cleared in the same atomic transition (RRDSET_FLAG_SYNC_CLOCK for the replay paths).
//
// Returns the OLD flags, so a caller can additionally test what it found - the replay path uses it to
// decide whether to log "there was no replication in progress for this chart".
//
// The contribution is released ONLY when this call is the one that cleared
// RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS. Releasing on "was not FINISHED" is wrong: a chart can
// be not-FINISHED without holding a contribution.
RRDSET_FLAGS rrdhost_receiver_replication_release_with_caller(RRDSET *st, RRDSET_FLAGS also_clear, const char *function, bool pulse) {
    RRDHOST *host = st->rrdhost;

    RRDSET_FLAGS old = rrdset_flag_set_and_clear(
        st, RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED,
        RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS | also_clear);

    if(old & RRDSET_FLAG_RECEIVER_REPLICATION_IN_PROGRESS) {
        // We won the clear, so the claim that set the flag is the one whose contribution we are about
        // to consume. atomic_flags_set_and_clear() is RELEASE on success and RELAXED on its load, so
        // observing the flag does NOT by itself order us against that claim's accounting increment:
        // on a weak-memory target (netdata ships armv6l, armv7l and aarch64) we could see the flag and
        // a stale word, refuse the decrement, and leak the contribution - pinning the host in
        // `replicating`, the exact failure this whole path exists to prevent. This ACQUIRE fence pairs
        // with the claim's releasing flag CAS, which our load above read from.
        __atomic_thread_fence(__ATOMIC_ACQUIRE);

        // `pulse` is false only for the connect/disconnect reset, which is draining the whole
        // generation and whose caller has already published the receiver state this host is really in.
        // Pulsing RCV_RUNNING from there would overwrite the RCV_OFFLINE that
        // stream_receiver_remove_internal() published moments earlier and latch the disconnected child
        // as running until it reconnects.
        if(rrdhost_receiver_replicating_charts_decrement(host, function) == 0 && pulse)
            pulse_host_status(host, PULSE_HOST_STATUS_RCV_RUNNING, 0);
    }

    return old;
}

NETDATA_DOUBLE rrdhost_receiver_replication_completion(RRDHOST *host, uint32_t *instances) {
    uint64_t accounting = rrdhost_receiver_replication_accounting(host);
    uint32_t remaining = (uint32_t)accounting;
    uint32_t started = (uint32_t)(accounting >> 32);

    if(instances)
        *instances = remaining;

    // nothing outstanding: complete, whatever the cohort was
    if(!remaining)
        return 100.0;

    // Outstanding work with no cohort to measure it against. Unreachable by construction: every unit
    // of `remaining` was added together with a unit of `started`, and releases only lower `remaining`.
    // Retained as a corruption / lifecycle diagnostic: report an explicit unknown rather than invent a
    // ratio. Reaches the API as `null`, so seeing one in the field means this reasoning has broken.
    if(!started || started < remaining)
        return NAN;

    return (NETDATA_DOUBLE)(started - remaining) * 100.0 / (NETDATA_DOUBLE)started;
}

#ifdef NETDATA_INTERNAL_CHECKS
// Counts refused (would-be-underflow) releases. A test cannot see an over-release from the counter
// alone: the refusal is exactly what stops it going below zero, so the counter still lands on its
// baseline and the fault is invisible. Tests assert this is unchanged across their run.
size_t rrdhost_receiver_replication_refusals = 0;
#endif

// Subtract `delta` from the packed accounting word, refusing when the outstanding half is already
// zero. Both callers must be checked: on a 64-bit word an unguarded subtract does not merely wrap the
// outstanding half to UINT32_MAX and pin the host in `replicating`, it BORROWS out of the cohort half
// and destroys the completion denominator too.
static uint32_t rrdhost_receiver_replication_subtract(RRDHOST *host, uint64_t delta, const char *what, const char *function) {
    // Test EACH half against the matching half of delta. Testing only the outstanding half would be
    // right for the release (delta moves that half alone) but not for the withdrawal (delta moves
    // both): a word with an empty cohort and outstanding work would pass and the subtract would
    // borrow the cohort down to UINT32_MAX - the corruption this guard exists to prevent.
    uint32_t need_remaining = rrdhost_receiver_replication_remaining_of(delta);
    uint32_t need_started = rrdhost_receiver_replication_started_of(delta);

    uint64_t cur = rrdhost_receiver_replication_accounting(host);

    while(rrdhost_receiver_replication_remaining_of(cur) >= need_remaining
       && rrdhost_receiver_replication_started_of(cur) >= need_started) {
        if(__atomic_compare_exchange_n(&host->stream.rcv.status.replication.charts_started_and_remaining, &cur, cur - delta,
                                       true, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
            return rrdhost_receiver_replication_remaining_of(cur - delta);
        // cur has been reloaded by the failed exchange; re-test it
    }

#ifdef NETDATA_INTERNAL_CHECKS
    __atomic_add_fetch(&rrdhost_receiver_replication_refusals, 1, __ATOMIC_RELAXED);
#endif

    nd_log(NDLS_DAEMON, NDLP_ERR,
           "STREAM REPLAY ERROR: 'host:%s': %s() %s a receiver replication contribution it does not own - "
           "the counter is already zero. Refusing to wrap it.",
           rrdhost_hostname(host), function, what);

    return 0;
}

uint32_t rrdhost_receiver_replicating_charts_decrement(RRDHOST *host, const char *function) {
    return rrdhost_receiver_replication_subtract(host, 1, "released", function);
}

// The duplicate-CHART_DEFINITION_END rollback: withdraw the speculative increment this same thread
// just made. Both halves move together, so the attempt leaves the completion figure untouched. This
// is NOT a release - it does not clear the flag, because the chart still holds its own contribution.
uint32_t rrdhost_receiver_replication_withdraw(RRDHOST *host) {
    return rrdhost_receiver_replication_subtract(host, RRDHOST_RCV_REPLICATION_UNIT, "withdrew",
                                                 "rrdhost_receiver_replication_claim");
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

    // flush, not destroy: this runs when the host transitions to archived
    // (service.c) while queries may still hold acquired charts; the indexes
    // must stay allocated until rrdhost_free_unlinked() destroys them
    rrdset_index_flush(host);

    rrdcalc_rrdhost_index_destroy(host);
    health_alarm_log_free(host);

    ml_host_delete(host);

    freez(host->exporting_flags);
    host->exporting_flags = NULL;

    rrdvariables_destroy(host->rrdvars);
    host->rrdvars = NULL;

    rrdhost_stream_path_clear(host, true);
    stream_sender_structures_free(host);

    // ORDERING (both directions load-bearing):
    // - the registry entry MUST be destroyed AFTER
    //   stream_sender_structures_free(): until the sender thread is joined
    //   there, it can still resolve the entry and run the global-functions
    //   renderer against it. Destroy synchronously DISARMS the entry (owner
    //   callbacks, epoch and name cleared under the entry's lock), so from
    //   that point the component can no longer call back into this host or
    //   read its epoch - anything still holding the entry degrades to
    //   no-ops. (The receiver stop at the top of this function is a BOUNDED
    //   ~2s wait that can give up on a stalled receiver thread - see
    //   stream_receiver_signal_to_stop_and_wait() - so the receiver side is
    //   best-effort, not a guarantee; that pre-existing residual is tracked
    //   separately and is not widened by this ordering.)
    // - it MUST be destroyed BEFORE destroy_aclk_config() in
    //   rrdhost_free_unlinked() - the disarm is what guarantees the
    //   component cannot arm the manifest after the config is freed; the
    //   full ACLK teardown contract is documented in sqlite_aclk.c
    //   (aclk_arm_node_manifest).
    nrpc_registry_destroy(rrdhost_nrpc_owner(host));

    // an archived host keeps its aclk config, so it is still reached by the manifest send loop -
    // tell it the function list is now empty (arming is a single atomic CAS, safe under rrd_wrlock)
    aclk_arm_node_manifest(host);

    rrdhost_flag_set(host, RRDHOST_FLAG_ARCHIVED | RRDHOST_FLAG_ORPHAN);

    nd_log(NDLS_DAEMON, NDLP_DEBUG,
           "RRD: 'host:%s' is now in archive mode...",
           rrdhost_hostname(host));
}

static void rrdhost_unlink___while_having_rrd_wrlock(RRDHOST *host) {
    // Remove it from the indexes first, so blocking teardown cannot rediscover it.
    rrdhost_index_del_by_guid(host);

    if (host->prev)
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(localhost, host, prev, next);
}

static void rrdhost_free_unlinked(RRDHOST *host) {
    rrdhost_cleanup_data_collection_and_health(host);

    // the host is already unlinked, so no new queries can find it;
    // now it is safe to destroy the chart indexes the archive path only flushed
    rrdset_index_destroy(host);

    // ------------------------------------------------------------------------
    // free it

    pulse_host_status(host, PULSE_HOST_STATUS_DELETED, 0);
    __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(RRDHOST), __ATOMIC_RELAXED);

    freez(host->cache_dir);
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

void rrdhost_free___while_having_rrd_wrlock(RRDHOST *host) {
    if(!host) return;

    rrdhost_unlink___while_having_rrd_wrlock(host);
    rrdhost_free_unlinked(host);
}

void rrdhost_free___consume_metadata_lifetime_writelock(RRDHOST *host) {
    if(!host) return;

    rrd_wrlock();
    rrdhost_unlink___while_having_rrd_wrlock(host);
    rw_spinlock_write_unlock(&host->metadata_lifetime_lock);
    rrd_wrunlock();

    rrdhost_free_unlinked(host);
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
