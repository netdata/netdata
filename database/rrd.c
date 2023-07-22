// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

// ----------------------------------------------------------------------------
// RRD - algorithms types

RRD_ALGORITHM rrd_algorithm_id(const char *name) {
    if(strcmp(name, RRD_ALGORITHM_INCREMENTAL_NAME) == 0)
        return RRD_ALGORITHM_INCREMENTAL;

    else if(strcmp(name, RRD_ALGORITHM_ABSOLUTE_NAME) == 0)
        return RRD_ALGORITHM_ABSOLUTE;

    else if(strcmp(name, RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL_NAME) == 0)
        return RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL;

    else if(strcmp(name, RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL_NAME) == 0)
        return RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL;

    else
        return RRD_ALGORITHM_ABSOLUTE;
}

const char *rrd_algorithm_name(RRD_ALGORITHM algorithm) {
    switch(algorithm) {
        case RRD_ALGORITHM_ABSOLUTE:
        default:
            return RRD_ALGORITHM_ABSOLUTE_NAME;

        case RRD_ALGORITHM_INCREMENTAL:
            return RRD_ALGORITHM_INCREMENTAL_NAME;

        case RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL:
            return RRD_ALGORITHM_PCENT_OVER_ROW_TOTAL_NAME;

        case RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL:
            return RRD_ALGORITHM_PCENT_OVER_DIFF_TOTAL_NAME;
    }
}


// ----------------------------------------------------------------------------
// RRD - chart types

RRDSET_TYPE rrdset_type_id(const char *name) {
    if(unlikely(strcmp(name, RRDSET_TYPE_AREA_NAME) == 0))
        return RRDSET_TYPE_AREA;

    else if(unlikely(strcmp(name, RRDSET_TYPE_STACKED_NAME) == 0))
        return RRDSET_TYPE_STACKED;

    else // if(unlikely(strcmp(name, RRDSET_TYPE_LINE_NAME) == 0))
        return RRDSET_TYPE_LINE;
}

const char *rrdset_type_name(RRDSET_TYPE chart_type) {
    switch(chart_type) {
        case RRDSET_TYPE_LINE:
        default:
            return RRDSET_TYPE_LINE_NAME;

        case RRDSET_TYPE_AREA:
            return RRDSET_TYPE_AREA_NAME;

        case RRDSET_TYPE_STACKED:
            return RRDSET_TYPE_STACKED_NAME;
    }
}

// ----------------------------------------------------------------------------
// RRD - string management

STRING *rrd_string_strdupz(const char *s) {
    if(unlikely(!s || !*s)) return string_strdupz(s);

    char *tmp = strdupz(s);
    json_fix_string(tmp);
    STRING *ret = string_strdupz(tmp);
    freez(tmp);
    return ret;
}

// ----------------------------------------------------------------------------
// rrd global / startup initialization

#ifdef ENABLE_DBENGINE
struct dbengine_initialization {
    netdata_thread_t thread;
    char path[FILENAME_MAX + 1];
    int disk_space_mb;
    size_t tier;
    int ret;
};

static void *dbengine_tier_init(void *ptr) {
    struct dbengine_initialization *dbi = ptr;
    dbi->ret = rrdeng_init(NULL, dbi->path, dbi->disk_space_mb, dbi->tier);
    return ptr;
}
#endif

static void dbengine_init(const char *hostname) {
#ifdef ENABLE_DBENGINE
    rrdb.use_direct_io = config_get_boolean(CONFIG_SECTION_DB, "dbengine use direct io", rrdb.use_direct_io);

    unsigned read_num = (unsigned)config_get_number(CONFIG_SECTION_DB, "dbengine pages per extent", MAX_PAGES_PER_EXTENT);
    if (read_num > 0 && read_num <= MAX_PAGES_PER_EXTENT)
        rrdeng_pages_per_extent = read_num;
    else {
        netdata_log_error("Invalid dbengine pages per extent %u given. Using %u.", read_num, rrdeng_pages_per_extent);
        config_set_number(CONFIG_SECTION_DB, "dbengine pages per extent", rrdeng_pages_per_extent);
    }

    rrdb.storage_tiers = config_get_number(CONFIG_SECTION_DB, "storage tiers", rrdb.storage_tiers);
    if (rrdb.storage_tiers < 1) {
        netdata_log_error("At least 1 storage tier is required. Assuming 1.");
        rrdb.storage_tiers = 1;
        config_set_number(CONFIG_SECTION_DB, "storage tiers", rrdb.storage_tiers);
    }
    if (rrdb.storage_tiers > RRD_STORAGE_TIERS) {
        netdata_log_error("Up to %d storage tier are supported. Assuming %d.", RRD_STORAGE_TIERS, RRD_STORAGE_TIERS);
        rrdb.storage_tiers = RRD_STORAGE_TIERS;
        config_set_number(CONFIG_SECTION_DB, "storage tiers", rrdb.storage_tiers);
    }

    bool parallel_initialization = (rrdb.storage_tiers <= (size_t)get_netdata_cpus()) ? true : false;
    parallel_initialization = config_get_boolean(CONFIG_SECTION_DB, "dbengine parallel initialization", parallel_initialization);

    struct dbengine_initialization tiers_init[RRD_STORAGE_TIERS] = {};

    size_t created_tiers = 0;
    char dbenginepath[FILENAME_MAX + 1];
    char dbengineconfig[200 + 1];
    int divisor = 1;
    for(size_t tier = 0; tier < rrdb.storage_tiers ;tier++) {
        if(tier == 0)
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", netdata_configured_cache_dir);
        else
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine-tier%zu", netdata_configured_cache_dir, tier);

        int ret = mkdir(dbenginepath, 0775);
        if (ret != 0 && errno != EEXIST) {
            netdata_log_error("DBENGINE on '%s': cannot create directory '%s'", hostname, dbenginepath);
            break;
        }

        if(tier > 0)
            divisor *= 2;

        int disk_space_mb = rrdb.default_multidb_disk_quota_mb / divisor;
        size_t grouping_iterations = rrdb.storage_tiers_grouping_iterations[tier];
        RRD_BACKFILL backfill = rrdb.storage_tiers_backfill[tier];

        if(tier > 0) {
            snprintfz(dbengineconfig, 200, "dbengine tier %zu multihost disk space MB", tier);
            disk_space_mb = config_get_number(CONFIG_SECTION_DB, dbengineconfig, disk_space_mb);

            snprintfz(dbengineconfig, 200, "dbengine tier %zu update every iterations", tier);
            grouping_iterations = config_get_number(CONFIG_SECTION_DB, dbengineconfig, grouping_iterations);
            if(grouping_iterations < 2) {
                grouping_iterations = 2;
                config_set_number(CONFIG_SECTION_DB, dbengineconfig, grouping_iterations);
                netdata_log_error("DBENGINE on '%s': 'dbegnine tier %zu update every iterations' cannot be less than 2. Assuming 2.",
                                  hostname,
                                  tier);
            }

            snprintfz(dbengineconfig, 200, "dbengine tier %zu backfill", tier);
            const char *bf = config_get(CONFIG_SECTION_DB, dbengineconfig, backfill == RRD_BACKFILL_NEW ? "new" : backfill == RRD_BACKFILL_FULL ? "full" : "none");
            if(strcmp(bf, "new") == 0) backfill = RRD_BACKFILL_NEW;
            else if(strcmp(bf, "full") == 0) backfill = RRD_BACKFILL_FULL;
            else if(strcmp(bf, "none") == 0) backfill = RRD_BACKFILL_NONE;
            else {
                netdata_log_error("DBENGINE: unknown backfill value '%s', assuming 'new'", bf);
                config_set(CONFIG_SECTION_DB, dbengineconfig, "new");
                backfill = RRD_BACKFILL_NEW;
            }
        }

        rrdb.storage_tiers_grouping_iterations[tier] = grouping_iterations;
        rrdb.storage_tiers_backfill[tier] = backfill;

        if(tier > 0 && get_tier_grouping(tier) > 65535) {
            rrdb.storage_tiers_grouping_iterations[tier] = 1;
            netdata_log_error("DBENGINE on '%s': dbengine tier %zu gives aggregation of more than 65535 points of tier 0. Disabling tiers above %zu",
                              hostname,
                              tier,
                              tier);
            break;
        }

        internal_error(true, "DBENGINE tier %zu grouping iterations is set to %zu", tier, rrdb.storage_tiers_grouping_iterations[tier]);

        tiers_init[tier].disk_space_mb = disk_space_mb;
        tiers_init[tier].tier = tier;
        strncpyz(tiers_init[tier].path, dbenginepath, FILENAME_MAX);
        tiers_init[tier].ret = 0;

        if(parallel_initialization) {
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, "DBENGINIT[%zu]", tier);
            netdata_thread_create(&tiers_init[tier].thread, tag, NETDATA_THREAD_OPTION_JOINABLE,
                                  dbengine_tier_init, &tiers_init[tier]);
        }
        else
            dbengine_tier_init(&tiers_init[tier]);
    }

    for (size_t tier = 0; tier < rrdb.storage_tiers ;tier++) {
        void *ptr;

        if(parallel_initialization)
            netdata_thread_join(tiers_init[tier].thread, &ptr);

        if(tiers_init[tier].ret != 0) {
            netdata_log_error("DBENGINE on '%s': Failed to initialize multi-host database tier %zu on path '%s'",
                              hostname,
                              tiers_init[tier].tier,
                              tiers_init[tier].path);
        }
        else if(created_tiers == tier)
            created_tiers++;
    }

    if(created_tiers && created_tiers < rrdb.storage_tiers) {
        netdata_log_error("DBENGINE on '%s': Managed to create %zu tiers instead of %zu. Continuing with %zu available.",
                          hostname,
                          created_tiers,
                          rrdb.storage_tiers,
                          created_tiers);
        rrdb.storage_tiers = created_tiers;
    }
    else if(!created_tiers)
        fatal("DBENGINE on '%s', failed to initialize databases at '%s'.", hostname, netdata_configured_cache_dir);

    for(size_t tier = 0; tier < rrdb.storage_tiers ;tier++)
        rrdeng_readiness_wait(rrdb.multidb_ctx[tier]);

    rrdb.dbengine_enabled = true;
#else
    rrdb.storage_tiers = config_get_number(CONFIG_SECTION_DB, "storage tiers", 1);
    if(rrdb.storage_tiers != 1) {
        netdata_log_error("DBENGINE is not available on '%s', so only 1 database tier can be supported.", hostname);
        rrdb.storage_tiers = 1;
        config_set_number(CONFIG_SECTION_DB, "storage tiers", rrdb.storage_tiers);
    }
    rrdb.dbengine_enabled = false;
#endif
}

static void init_host_indexes() {
    internal_fatal(rrdb.rrdhost_root_index || rrdb.rrdhost_root_index_hostname,
                   "Host indexes have already been initialized");

    DICT_OPTIONS dict_opts = DICT_OPTION_NAME_LINK_DONT_CLONE |
                             DICT_OPTION_VALUE_LINK_DONT_CLONE |
                             DICT_OPTION_DONT_OVERWRITE_VALUE;

    rrdb.rrdhost_root_index = dictionary_create_advanced(dict_opts, &dictionary_stats_category_rrdhost, 0);
    rrdb.rrdhost_root_index_hostname = dictionary_create_advanced(dict_opts, &dictionary_stats_category_rrdhost, 0);
}

int rrd_init(char *hostname, struct rrdhost_system_info *system_info, bool unittest) {
    init_host_indexes();

    if (unlikely(sql_init_database(DB_CHECK_NONE, system_info ? 0 : 1))) {
        if (default_storage_engine_id == STORAGE_ENGINE_DBENGINE) {
            set_late_global_environment(system_info);
            fatal("Failed to initialize SQLite");
        }
        netdata_log_info("Skipping SQLITE metadata initialization since memory mode is not dbengine");
    }

    if (unlikely(sql_init_context_database(system_info ? 0 : 1))) {
        error_report("Failed to initialize context metadata database");
    }

    if (unlikely(unittest)) {
        rrdb.dbengine_enabled = true;
    }
    else {
        health_init();
        rrdpush_init();

        if (default_storage_engine_id == STORAGE_ENGINE_DBENGINE || rrdpush_receiver_needs_dbengine()) {
            netdata_log_info("DBENGINE: Initializing ...");
            dbengine_init(hostname);
        }
        else {
            netdata_log_info("DBENGINE: Not initializing ...");
            rrdb.storage_tiers = 1;
        }

        if (!rrdb.dbengine_enabled) {
            if (rrdb.storage_tiers > 1) {
                netdata_log_error("dbengine is not enabled, but %zu tiers have been requested. Resetting tiers to 1",
                                  rrdb.storage_tiers);
                rrdb.storage_tiers = 1;
            }

            if (default_storage_engine_id == STORAGE_ENGINE_DBENGINE) {
                netdata_log_error("dbengine is not enabled, but it has been given as the default db mode. Resetting db mode to alloc");
                default_storage_engine_id = STORAGE_ENGINE_ALLOC;
            }
        }
    }

    if(!unittest)
        metadata_sync_init();

    netdata_log_debug(D_RRDHOST, "Initializing localhost with hostname '%s'", hostname);
    rrdb.localhost = rrdhost_create(
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
            , rrdb.default_update_every
            , rrdb.default_rrd_history_entries
            , default_storage_engine_id
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

    if (unlikely(!rrdb.localhost)) {
        return 1;
    }

#if NETDATA_DEV_MODE
    // we register this only on localhost
    // for the other nodes, the origin server should register it
    rrd_collector_started(); // this creates a collector that runs for as long as netdata runs
    rrd_collector_add_function(rrdb.localhost, NULL, "streaming", 10,
                               RRDFUNCTIONS_STREAMING_HELP, true,
                               rrdhost_function_streaming, NULL);
#endif

    if (likely(system_info)) {
        migrate_localhost(&rrdb.localhost->host_uuid);
        sql_aclk_sync_init();
        web_client_api_v1_management_init();
    }
    return rrdb.localhost == NULL;
}

struct rrdb rrdb = {
    .rrdhost_root_index = NULL,
    .rrdhost_root_index_hostname = NULL,
    .unittest_running = false,
    .dbengine_enabled = false,
    .storage_tiers = 3,
    .use_direct_io = true,
    .storage_tiers_grouping_iterations = {
        1,
        60,
        60,
        60,
        60
    },
    .storage_tiers_backfill = {
        RRD_BACKFILL_NEW,
        RRD_BACKFILL_NEW,
        RRD_BACKFILL_NEW,
        RRD_BACKFILL_NEW,
        RRD_BACKFILL_NEW
    },
    .default_update_every = UPDATE_EVERY_MIN,
    .default_rrd_history_entries = RRD_DEFAULT_HISTORY_ENTRIES,
    .gap_when_lost_iterations_above = 1,
    .rrdset_free_obsolete_time_s = RRD_DEFAULT_HISTORY_ENTRIES,
    .libuv_worker_threads = 8,
    .ieee754_doubles = false,
    .rrdhost_free_orphan_time_s = RRD_DEFAULT_HISTORY_ENTRIES,
    .rrd_rwlock = NETDATA_RWLOCK_INITIALIZER,
    .localhost = NULL,

#if defined(ENV32BIT)
    .default_rrdeng_page_cache_mb = 16,
    .default_rrdeng_extent_cache_mb = 0,
#else
    .default_rrdeng_page_cache_mb = 32,
    .default_rrdeng_extent_cache_mb = 0,
#endif

    .db_engine_journal_check = CONFIG_BOOLEAN_NO,

    .default_rrdeng_disk_quota_mb = 256,
    .default_multidb_disk_quota_mb = 256,

    .multidb_ctx = {
        NULL,
        NULL,
        NULL,
        NULL,
        NULL,
    },

    .page_type_size = {
        sizeof(storage_number),
        sizeof(storage_number_tier1_t)
    },

#if defined(ENV32BIT)
    .tier_page_size = {
        2048,
        1024,
        192,
        192,
        192
    },
#else
    .tier_page_size = {
        4096,
        2048,
        384,
        384,
        384
    },
#endif
};
