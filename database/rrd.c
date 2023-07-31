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

static void dbengine_init(const char *hostname, dbengine_config_t *cfg) {
    struct dbengine_initialization tiers_init[STORAGE_ENGINE_TIERS] = {};

    for (size_t tier = 0; tier < cfg->storage_tiers ;tier++) {
        char dbenginepath[FILENAME_MAX + 1];

        if (tier == 0)
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", cfg->base_path);
        else
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine-tier%zu", cfg->base_path, tier);

        int ret = mkdir(dbenginepath, 0775);
        if (ret != 0 && errno != EEXIST) {
            netdata_log_error("DBENGINE on '%s': cannot create directory '%s'", hostname, dbenginepath);
            break;
        }

        tiers_init[tier].disk_space_mb = cfg->multidb_disk_quota_mb[tier];
        tiers_init[tier].tier = tier;
        strncpyz(tiers_init[tier].path, dbenginepath, FILENAME_MAX);
        tiers_init[tier].ret = 0;

        if (cfg->parallel_initialization) {
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, "DBENGINIT[%zu]", tier);
            netdata_thread_create(&tiers_init[tier].thread, tag, NETDATA_THREAD_OPTION_JOINABLE,
                                  dbengine_tier_init, &tiers_init[tier]);
        }
        else
            dbengine_tier_init(&tiers_init[tier]);
    }

    size_t created_tiers = 0;

    for (size_t tier = 0; tier < cfg->storage_tiers ;tier++) {
        void *ptr;

        if (cfg->parallel_initialization)
            netdata_thread_join(tiers_init[tier].thread, &ptr);

        if (tiers_init[tier].ret != 0) {
            netdata_log_error("DBENGINE on '%s': Failed to initialize multi-host database tier %zu on path '%s'",
                              hostname,
                              tiers_init[tier].tier,
                              tiers_init[tier].path);
        }
        else if (created_tiers == tier)
            created_tiers++;
    }

    if (created_tiers && created_tiers < cfg->storage_tiers) {
        netdata_log_error("DBENGINE on '%s': Managed to create %zu tiers instead of %zu. Continuing with %zu available.",
                          hostname,
                          created_tiers,
                          cfg->storage_tiers,
                          created_tiers);
        cfg->storage_tiers = created_tiers;
    }
    else if (!created_tiers)
        fatal("DBENGINE on '%s', failed to initialize databases at '%s'.", hostname, netdata_configured_cache_dir);

    for (size_t tier = 0; tier < cfg->storage_tiers ;tier++)
        rrdeng_readiness_wait(rrdb.dbengine_cfg.multidb_ctx[tier]);

    cfg->enabled = true;
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
        rrdb.dbengine_cfg.enabled = true;
    }
    else {
        health_init();
        rrdpush_init();

        if (default_storage_engine_id == STORAGE_ENGINE_DBENGINE || rrdpush_receiver_needs_dbengine()) {
            netdata_log_info("DBENGINE: Initializing ...");

#ifdef ENABLE_DBENGINE
            dbengine_config_t cfg = rrdb.dbengine_cfg;

            // check journal
            cfg.check_journal = config_get_boolean(CONFIG_SECTION_DB, "dbengine enable journal integrity check", cfg.check_journal);

            // use direct io
            cfg.use_direct_io = config_get_boolean(CONFIG_SECTION_DB, "dbengine use direct io", cfg.use_direct_io);

            // parallel initialization
            cfg.parallel_initialization = (cfg.storage_tiers <= (size_t) get_netdata_cpus()) ? true : false;
            cfg.parallel_initialization = config_get_boolean(CONFIG_SECTION_DB, "dbengine parallel initialization", cfg.parallel_initialization);

            // disk quota size
            cfg.disk_quota_mb = (int) config_get_number(CONFIG_SECTION_DB, "dbengine disk space MB", cfg.disk_quota_mb);
            if (cfg.disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB) {
                netdata_log_error("Invalid dbengine disk space %d given. Defaulting to %d.", cfg.disk_quota_mb, RRDENG_MIN_DISK_SPACE_MB);
                cfg.disk_quota_mb = RRDENG_MIN_DISK_SPACE_MB;
                config_set_number(CONFIG_SECTION_DB, "dbengine disk space MB", cfg.disk_quota_mb);
            }

            // page cache size
            cfg.page_cache_mb = (int) config_get_number(CONFIG_SECTION_DB, "dbengine page cache size MB", cfg.page_cache_mb);
            if (cfg.page_cache_mb < RRDENG_MIN_PAGE_CACHE_SIZE_MB) {
                netdata_log_error("Invalid page cache size %d given. Defaulting to %d.",
                                  cfg.page_cache_mb, RRDENG_MIN_PAGE_CACHE_SIZE_MB);
                cfg.page_cache_mb = RRDENG_MIN_PAGE_CACHE_SIZE_MB;
                config_set_number(CONFIG_SECTION_DB, "dbengine page cache size MB", cfg.page_cache_mb);
            }

            // extent cache size
            cfg.extent_cache_mb = (int) config_get_number(CONFIG_SECTION_DB, "dbengine extent cache size MB", cfg.extent_cache_mb);
            if (cfg.extent_cache_mb < 0)
                cfg.extent_cache_mb = 0;

            // pages per extent
            unsigned pages_per_extent = (unsigned) config_get_number(CONFIG_SECTION_DB, "dbengine pages per extent", cfg.pages_per_extent);
            if (pages_per_extent > 0 && pages_per_extent <= cfg.pages_per_extent)
                cfg.pages_per_extent = pages_per_extent;
            else {
                netdata_log_error("Invalid dbengine pages per extent %u given. Using %u.", pages_per_extent, cfg.pages_per_extent);
                config_set_number(CONFIG_SECTION_DB, "dbengine pages per extent", cfg.pages_per_extent);
            }

            // number of storage tiers
            cfg.storage_tiers = config_get_number(CONFIG_SECTION_DB, "storage tiers", cfg.storage_tiers);
            if (cfg.storage_tiers < 1) {
                netdata_log_error("At least 1 storage tier is required. Assuming 1.");
                cfg.storage_tiers = 1;
                config_set_number(CONFIG_SECTION_DB, "storage tiers", cfg.storage_tiers);
            }
            if (cfg.storage_tiers > STORAGE_ENGINE_TIERS) {
                netdata_log_error("Up to %d storage tier are supported. Assuming %d.", STORAGE_ENGINE_TIERS, RRD_STORAGE_TIERS);
                cfg.storage_tiers = STORAGE_ENGINE_TIERS;
                config_set_number(CONFIG_SECTION_DB, "storage tiers", cfg.storage_tiers);
            }

            // multi-db disk quota size
            {
                cfg.multidb_disk_quota_mb[0] = (int) config_get_number(CONFIG_SECTION_DB, "dbengine multihost disk space MB", compute_multidb_diskspace());
                if (cfg.multidb_disk_quota_mb[0] < RRDENG_MIN_DISK_SPACE_MB) {
                    netdata_log_error("Invalid multidb disk space %d given. Defaulting to %d.", cfg.multidb_disk_quota_mb[0], cfg.disk_quota_mb);
                    cfg.multidb_disk_quota_mb[0] = cfg.disk_quota_mb;
                    config_set_number(CONFIG_SECTION_DB, "dbengine multihost disk space MB", cfg.multidb_disk_quota_mb[0]);
                }

                // figure out the default non-zero tier disk size
                for (size_t tier = 1; tier != cfg.storage_tiers; tier++) {
                    int prev_tier_size = cfg.multidb_disk_quota_mb[tier - 1];
                    int curr_tier_size = prev_tier_size >> 1;

                    char buf[200 + 1];
                    snprintfz(buf, 200, "dbengine tier %zu multihost disk space MB", tier);
                    cfg.multidb_disk_quota_mb[tier] = config_get_number(CONFIG_SECTION_DB, buf, curr_tier_size);
                }
            }

            // tier grouping
            {
                for (size_t tier = 1; tier < cfg.storage_tiers; tier++) {
                    char buf[200 + 1];
                    snprintfz(buf, 200, "dbengine tier %zu update every iterations", tier);

                    size_t grouping_iterations = cfg.storage_tiers_grouping_iterations[tier];
                    grouping_iterations = config_get_number(CONFIG_SECTION_DB, buf, grouping_iterations);

                    if (grouping_iterations < 2) {
                        grouping_iterations = 2;
                        config_set_number(CONFIG_SECTION_DB, buf, grouping_iterations);
                        netdata_log_error("DBENGINE on '%s': 'dbegnine tier %zu update every iterations' cannot be less than 2. Assuming 2.",
                                          hostname, tier);
                    }

                    cfg.storage_tiers_grouping_iterations[tier] = grouping_iterations;

                    if(tier > 0 && get_tier_grouping(tier) > 65535) {
                        cfg.storage_tiers_grouping_iterations[tier] = 1;
                        netdata_log_error("DBENGINE on '%s': dbengine tier %zu gives aggregation of more than 65535 points of tier 0. Disabling tiers above %zu",
                                          hostname,
                                          tier,
                                          tier);
                        cfg.storage_tiers = tier;
                        break;
                    }

                    internal_error(true, "DBENGINE tier %zu grouping iterations is set to %zu", tier, cfg.storage_tiers_grouping_iterations[tier]);
                }
            }

            // tier backfilling
            {
                for (size_t tier = 0; tier != cfg.storage_tiers; tier++) {
                    STORAGE_TIER_BACKFILL backfill = cfg.storage_tiers_backfill[tier];

                    if (tier > 0) {
                        char buf[200 + 1];
                        snprintfz(buf, 200, "dbengine tier %zu backfill", tier);

                        const char *bf = config_get(CONFIG_SECTION_DB, buf, backfill == STORAGE_TIER_BACKFILL_NEW ? "new" : backfill == STORAGE_TIER_BACKFILL_FULL ? "full" : "none");

                        if(strcmp(bf, "new") == 0)
                            backfill = STORAGE_TIER_BACKFILL_NEW;
                        else if(strcmp(bf, "full") == 0)
                            backfill = STORAGE_TIER_BACKFILL_FULL;
                        else if(strcmp(bf, "none") == 0)
                            backfill = STORAGE_TIER_BACKFILL_NONE;
                        else {
                            netdata_log_error("DBENGINE: unknown backfill value '%s', assuming 'new'", bf);
                            config_set(CONFIG_SECTION_DB, buf, "new");
                            backfill = STORAGE_TIER_BACKFILL_NEW;
                        }
                    }

                    cfg.storage_tiers_backfill[tier] = backfill;
                }
            }

            rrdb.dbengine_cfg = cfg;

            dbengine_init(hostname, &rrdb.dbengine_cfg);
#else
            rrdb.storage_tiers = config_get_number(CONFIG_SECTION_DB, "storage tiers", 1);
            if(rrdb.storage_tiers != 1) {
                netdata_log_error("DBENGINE is not available on '%s', so only 1 database tier can be supported.", hostname);
                rrdb.storage_tiers = 1;
                config_set_number(CONFIG_SECTION_DB, "storage tiers", rrdb.storage_tiers);
            }
            rrdb.dbengine_enabled = false;
#endif // ENABLE_DBENGINE
        }
        else {
            netdata_log_info("DBENGINE: Not initializing ...");
            rrdb.dbengine_cfg.storage_tiers = 1;
        }

        if (!rrdb.dbengine_cfg.enabled) {
            if (rrdb.dbengine_cfg.storage_tiers > 1) {
                netdata_log_error("dbengine is not enabled, but %zu tiers have been requested. Resetting tiers to 1",
                                  rrdb.dbengine_cfg.storage_tiers);
                rrdb.dbengine_cfg.storage_tiers = 1;
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
    .default_update_every = UPDATE_EVERY_MIN,
    .default_rrd_history_entries = RRD_DEFAULT_HISTORY_ENTRIES,
    .gap_when_lost_iterations_above = 1,
    .rrdset_free_obsolete_time_s = RRD_DEFAULT_HISTORY_ENTRIES,
    .libuv_worker_threads = 8,
    .ieee754_doubles = false,
    .rrdhost_free_orphan_time_s = RRD_DEFAULT_HISTORY_ENTRIES,
    .rrd_rwlock = NETDATA_RWLOCK_INITIALIZER,
    .localhost = NULL,

    .dbengine_cfg = {
        .enabled = false,
        .base_path = CACHE_DIR,

        .check_journal  = CONFIG_BOOLEAN_NO,
        .use_direct_io = true,
        .parallel_initialization = false,

        .disk_quota_mb = 256,

        #if defined(ENV32BIT)
            .page_cache_mb = 16,
            .extent_cache_mb = 0,
        #else
            .page_cache_mb = 32,
            .extent_cache_mb = 0,
        #endif

        .pages_per_extent = MAX_PAGES_PER_EXTENT,

        .page_type_size = {
            sizeof(storage_number),
            sizeof(storage_number_tier1_t)
        },

        .storage_tiers = 3,

        .multidb_ctx = {
            NULL,
            NULL,
            NULL,
            NULL,
            NULL,
        },

        .multidb_disk_quota_mb = {
            256,
            128,
            64,
            32,
            16,
        },

        .storage_tiers_grouping_iterations = {
            1,
            60,
            60,
            60,
            60
        },

        .storage_tiers_backfill = {
            STORAGE_TIER_BACKFILL_NEW,
            STORAGE_TIER_BACKFILL_NEW,
            STORAGE_TIER_BACKFILL_NEW,
            STORAGE_TIER_BACKFILL_NEW,
            STORAGE_TIER_BACKFILL_NEW,
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
    },
};
