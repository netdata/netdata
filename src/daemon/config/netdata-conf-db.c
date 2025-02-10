// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-db.h"
#include "daemon/common.h"

#define DAYS 86400
int default_rrd_history_entries = RRD_DEFAULT_HISTORY_ENTRIES;

bool dbengine_enabled = false; // will become true if and when dbengine is initialized
bool dbengine_use_direct_io = true;
static size_t storage_tiers_grouping_iterations[RRD_STORAGE_TIERS] = {1, 60, 60, 60, 60};
static time_t storage_tiers_retention_time_s[RRD_STORAGE_TIERS] = {14 * DAYS, 90 * DAYS, 2 * 365 * DAYS, 2 * 365 * DAYS, 2 * 365 * DAYS};

time_t rrdset_free_obsolete_time_s = 3600;
time_t rrdhost_cleanup_orphan_to_archive_time_s = 3600;
time_t rrdhost_free_ephemeral_time_s = 0;

size_t get_tier_grouping(size_t tier) {
    if(unlikely(tier >= nd_profile.storage_tiers)) tier = nd_profile.storage_tiers - 1;

    size_t grouping = 1;
    // first tier is always 1 iteration of whatever update every the chart has
    for(size_t i = 1; i <= tier ;i++)
        grouping *= storage_tiers_grouping_iterations[i];

    return grouping;
}

static void netdata_conf_dbengine_pre_logs(void) {
    static bool run = false;
    if(run) return;
    run = true;

    errno_clear();

#ifdef ENABLE_DBENGINE
    // this is required for dbegnine to work, so call it here (it is ok, it won't run twice)
    netdata_conf_section_directories();

    // ------------------------------------------------------------------------
    // get default Database Engine page type

    const char *page_type = inicfg_get(&netdata_config, CONFIG_SECTION_DB, "dbengine page type", "gorilla");
    if (strcmp(page_type, "gorilla") == 0)
        tier_page_type[0] = RRDENG_PAGE_TYPE_GORILLA_32BIT;
    else if (strcmp(page_type, "raw") == 0)
        tier_page_type[0] = RRDENG_PAGE_TYPE_ARRAY_32BIT;
    else {
        tier_page_type[0] = RRDENG_PAGE_TYPE_ARRAY_32BIT;
        netdata_log_error("Invalid dbengine page type ''%s' given. Defaulting to 'raw'.", page_type);
    }

    // ------------------------------------------------------------------------
    // get default Database Engine page cache size in MiB

    default_rrdeng_page_cache_mb = (int) inicfg_get_size_mb(&netdata_config, CONFIG_SECTION_DB, "dbengine page cache size", default_rrdeng_page_cache_mb);
    default_rrdeng_extent_cache_mb = (int) inicfg_get_size_mb(&netdata_config, CONFIG_SECTION_DB, "dbengine extent cache size", default_rrdeng_extent_cache_mb);
    db_engine_journal_check = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_DB, "dbengine enable journal integrity check", CONFIG_BOOLEAN_NO);

    if(default_rrdeng_extent_cache_mb < 0) {
        default_rrdeng_extent_cache_mb = 0;
        inicfg_set_size_mb(&netdata_config, CONFIG_SECTION_DB, "dbengine extent cache size", default_rrdeng_extent_cache_mb);
    }

    if(default_rrdeng_page_cache_mb < RRDENG_MIN_PAGE_CACHE_SIZE_MB) {
        netdata_log_error("Invalid page cache size %d given. Defaulting to %d.", default_rrdeng_page_cache_mb, RRDENG_MIN_PAGE_CACHE_SIZE_MB);
        default_rrdeng_page_cache_mb = RRDENG_MIN_PAGE_CACHE_SIZE_MB;
        inicfg_set_size_mb(&netdata_config, CONFIG_SECTION_DB, "dbengine page cache size", default_rrdeng_page_cache_mb);
    }

    // ------------------------------------------------------------------------
    // get default Database Engine disk space quota in MiB
    //
    //    //    if (!config_exists(CONFIG_SECTION_DB, "dbengine disk space MB") && !config_exists(CONFIG_SECTION_DB, "dbengine multihost disk space MB"))
    //
    //    default_rrdeng_disk_quota_mb = (int) inicfg_get_number(&netdata_config, CONFIG_SECTION_DB, "dbengine disk space MB", default_rrdeng_disk_quota_mb);
    //    if(default_rrdeng_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB) {
    //        netdata_log_error("Invalid dbengine disk space %d given. Defaulting to %d.", default_rrdeng_disk_quota_mb, RRDENG_MIN_DISK_SPACE_MB);
    //        default_rrdeng_disk_quota_mb = RRDENG_MIN_DISK_SPACE_MB;
    //        inicfg_set_number(&netdata_config, CONFIG_SECTION_DB, "dbengine disk space MB", default_rrdeng_disk_quota_mb);
    //    }
    //
    //    default_multidb_disk_quota_mb = (int) inicfg_get_number(&netdata_config, CONFIG_SECTION_DB, "dbengine multihost disk space MB", compute_multidb_diskspace());
    //    if(default_multidb_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB) {
    //        netdata_log_error("Invalid multidb disk space %d given. Defaulting to %d.", default_multidb_disk_quota_mb, default_rrdeng_disk_quota_mb);
    //        default_multidb_disk_quota_mb = default_rrdeng_disk_quota_mb;
    //        inicfg_set_number(&netdata_config, CONFIG_SECTION_DB, "dbengine multihost disk space MB", default_multidb_disk_quota_mb);
    //    }

#else
    if (default_rrd_memory_mode == RRD_DB_MODE_DBENGINE) {
        error_report("RRD_DB_MODE_DBENGINE is not supported in this platform. The agent will use db mode 'save' instead.");
        default_rrd_memory_mode = RRD_DB_MODE_RAM;
    }
#endif
}

#ifdef ENABLE_DBENGINE
struct dbengine_initialization {
    ND_THREAD *thread;
    char path[FILENAME_MAX + 1];
    int disk_space_mb;
    size_t retention_seconds;
    size_t tier;
    int ret;
};

void *dbengine_tier_init(void *ptr) {
    struct dbengine_initialization *dbi = ptr;
    dbi->ret = rrdeng_init(NULL, dbi->path, dbi->disk_space_mb, dbi->tier, dbi->retention_seconds);
    return ptr;
}

RRD_BACKFILL get_dbengine_backfill(RRD_BACKFILL backfill)
{
    const char *bf = inicfg_get(&netdata_config, 
        CONFIG_SECTION_DB,
        "dbengine tier backfill",
        backfill == RRD_BACKFILL_NEW  ? "new" :
        backfill == RRD_BACKFILL_FULL ? "full" :
                                        "none");

    if (strcmp(bf, "new") == 0)
        backfill = RRD_BACKFILL_NEW;
    else if (strcmp(bf, "full") == 0)
        backfill = RRD_BACKFILL_FULL;
    else if (strcmp(bf, "none") == 0)
        backfill = RRD_BACKFILL_NONE;
    else {
        nd_log(NDLS_DAEMON, NDLP_WARNING, "DBENGINE: unknown backfill value '%s', assuming 'new'", bf);
        inicfg_set(&netdata_config, CONFIG_SECTION_DB, "dbengine tier backfill", "new");
        backfill = RRD_BACKFILL_NEW;
    }
    return backfill;
}
#endif

void netdata_conf_dbengine_init(const char *hostname) {
#ifdef ENABLE_DBENGINE

    // ----------------------------------------------------------------------------------------------------------------
    // out of memory protection and use all ram for caches

    dbengine_out_of_memory_protection = 0; // will be calculated below
    OS_SYSTEM_MEMORY sm = os_system_memory(true);
    if(sm.ram_total_bytes && sm.ram_available_bytes && sm.ram_total_bytes > sm.ram_available_bytes) {
        // calculate the default out of memory protection size
        uint64_t keep_free = sm.ram_total_bytes / 10;
        if(keep_free > 5ULL * 1024 * 1024 * 1024)
            keep_free = 5ULL * 1024 * 1024 * 1024;
        char buf[64];
        size_snprintf(buf, sizeof(buf), keep_free, "B", false);
        size_parse(buf, &dbengine_out_of_memory_protection, "B");
    }

    if(dbengine_out_of_memory_protection) {
        dbengine_use_all_ram_for_caches = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_DB, "dbengine use all ram for caches", dbengine_use_all_ram_for_caches);
        dbengine_out_of_memory_protection = inicfg_get_size_bytes(&netdata_config, CONFIG_SECTION_DB, "dbengine out of memory protection", dbengine_out_of_memory_protection);

        char buf_total[64], buf_avail[64], buf_oom[64];
        size_snprintf(buf_total, sizeof(buf_total), sm.ram_total_bytes, "B", false);
        size_snprintf(buf_avail, sizeof(buf_avail), sm.ram_available_bytes, "B", false);
        size_snprintf(buf_oom, sizeof(buf_oom), dbengine_out_of_memory_protection, "B", false);

        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "DBENGINE Out of Memory Protection. "
               "System Memory Total: %s, Currently Available: %s, Out of Memory Protection: %s, Use All RAM: %s",
               buf_total, buf_avail, buf_oom, dbengine_use_all_ram_for_caches ? "enabled" : "disabled");
    }
    else {
        dbengine_out_of_memory_protection = 0;
        dbengine_use_all_ram_for_caches = false;

        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "DBENGINE Out of Memory Protection and Use All Ram cannot be enabled. "
               "Failed to detect memory size on this system.");
    }

    // ----------------------------------------------------------------------------------------------------------------

    dbengine_use_direct_io = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_DB, "dbengine use direct io", dbengine_use_direct_io);

    unsigned read_num = (unsigned)inicfg_get_number(&netdata_config, CONFIG_SECTION_DB, "dbengine pages per extent", DEFAULT_PAGES_PER_EXTENT);
    if (read_num > 0 && read_num <= DEFAULT_PAGES_PER_EXTENT)
        rrdeng_pages_per_extent = read_num;
    else {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Invalid dbengine pages per extent %u given. Using %u.",
               read_num, rrdeng_pages_per_extent);

        inicfg_set_number(&netdata_config, CONFIG_SECTION_DB, "dbengine pages per extent", rrdeng_pages_per_extent);
    }

    nd_profile.storage_tiers = inicfg_get_number(&netdata_config, CONFIG_SECTION_DB, "storage tiers", nd_profile.storage_tiers);
    if(nd_profile.storage_tiers < 1) {
        nd_log(NDLS_DAEMON, NDLP_WARNING, "At least 1 storage tier is required. Assuming 1.");

        nd_profile.storage_tiers = 1;
        inicfg_set_number(&netdata_config, CONFIG_SECTION_DB, "storage tiers", nd_profile.storage_tiers);
    }
    if(nd_profile.storage_tiers > RRD_STORAGE_TIERS) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Up to %d storage tier are supported. Assuming %d.",
               RRD_STORAGE_TIERS, RRD_STORAGE_TIERS);

        nd_profile.storage_tiers = RRD_STORAGE_TIERS;
        inicfg_set_number(&netdata_config, CONFIG_SECTION_DB, "storage tiers", nd_profile.storage_tiers);
    }

    new_dbengine_defaults =
        (!legacy_multihost_db_space &&
         !inicfg_exists(&netdata_config, CONFIG_SECTION_DB, "dbengine tier 1 update every iterations") &&
         !inicfg_exists(&netdata_config, CONFIG_SECTION_DB, "dbengine tier 2 update every iterations") &&
         !inicfg_exists(&netdata_config, CONFIG_SECTION_DB, "dbengine tier 3 update every iterations") &&
         !inicfg_exists(&netdata_config, CONFIG_SECTION_DB, "dbengine tier 4 update every iterations") &&
         !inicfg_exists(&netdata_config, CONFIG_SECTION_DB, "dbengine tier 1 retention size") &&
         !inicfg_exists(&netdata_config, CONFIG_SECTION_DB, "dbengine tier 2 retention size") &&
         !inicfg_exists(&netdata_config, CONFIG_SECTION_DB, "dbengine tier 3 retention size") &&
         !inicfg_exists(&netdata_config, CONFIG_SECTION_DB, "dbengine tier 4 retention size"));

    default_backfill = get_dbengine_backfill(RRD_BACKFILL_NEW);
    char dbengineconfig[200 + 1];

    size_t grouping_iterations = nd_profile.update_every;
    storage_tiers_grouping_iterations[0] = nd_profile.update_every;

    for (size_t tier = 1; tier < nd_profile.storage_tiers; tier++) {
        grouping_iterations = storage_tiers_grouping_iterations[tier];
        snprintfz(dbengineconfig, sizeof(dbengineconfig) - 1, "dbengine tier %zu update every iterations", tier);
        grouping_iterations = inicfg_get_number(&netdata_config, CONFIG_SECTION_DB, dbengineconfig, grouping_iterations);
        if(grouping_iterations < 2) {
            grouping_iterations = 2;
            inicfg_set_number(&netdata_config, CONFIG_SECTION_DB, dbengineconfig, grouping_iterations);
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "DBENGINE on '%s': 'dbegnine tier %zu update every iterations' cannot be less than 2. Assuming 2.",
                   hostname, tier);
        }
        storage_tiers_grouping_iterations[tier] = grouping_iterations;
    }

    default_multidb_disk_quota_mb = (int) inicfg_get_size_mb(&netdata_config, CONFIG_SECTION_DB, "dbengine tier 0 retention size", RRDENG_DEFAULT_TIER_DISK_SPACE_MB);
    if(default_multidb_disk_quota_mb && default_multidb_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB) {
        netdata_log_error("Invalid disk space %d for tier 0 given. Defaulting to %d.", default_multidb_disk_quota_mb, RRDENG_MIN_DISK_SPACE_MB);
        default_multidb_disk_quota_mb = RRDENG_MIN_DISK_SPACE_MB;
        inicfg_set_size_mb(&netdata_config, CONFIG_SECTION_DB, "dbengine tier 0 retention size", default_multidb_disk_quota_mb);
    }

#ifdef OS_WINDOWS
    // FIXME: for whatever reason joining the initialization threads
    // fails on Windows.
    bool parallel_initialization = false;
#else
    bool parallel_initialization = (nd_profile.storage_tiers <= netdata_conf_cpus()) ? true : false;
#endif

    struct dbengine_initialization tiers_init[RRD_STORAGE_TIERS] = {};

    size_t created_tiers = 0;
    char dbenginepath[FILENAME_MAX + 1];

    for (size_t tier = 0; tier < nd_profile.storage_tiers; tier++) {

        if (tier == 0)
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine", netdata_configured_cache_dir);
        else
            snprintfz(dbenginepath, FILENAME_MAX, "%s/dbengine-tier%zu", netdata_configured_cache_dir, tier);

        int ret = mkdir(dbenginepath, 0775);
        if (ret != 0 && errno != EEXIST) {
            nd_log(NDLS_DAEMON, NDLP_CRIT, "DBENGINE on '%s': cannot create directory '%s'", hostname, dbenginepath);
            continue;
        }

        int disk_space_mb = tier ? RRDENG_DEFAULT_TIER_DISK_SPACE_MB : default_multidb_disk_quota_mb;
        snprintfz(dbengineconfig, sizeof(dbengineconfig) - 1, "dbengine tier %zu retention size", tier);
        disk_space_mb = inicfg_get_size_mb(&netdata_config, CONFIG_SECTION_DB, dbengineconfig, disk_space_mb);

        snprintfz(dbengineconfig, sizeof(dbengineconfig) - 1, "dbengine tier %zu retention time", tier);
        storage_tiers_retention_time_s[tier] = inicfg_get_duration_days_to_seconds(
            &netdata_config, CONFIG_SECTION_DB,
            dbengineconfig, new_dbengine_defaults ? storage_tiers_retention_time_s[tier] : 0);

        tiers_init[tier].disk_space_mb = (int) disk_space_mb;
        tiers_init[tier].tier = tier;
        tiers_init[tier].retention_seconds = (size_t) storage_tiers_retention_time_s[tier];
        strncpyz(tiers_init[tier].path, dbenginepath, FILENAME_MAX);
        tiers_init[tier].ret = 0;

        if(parallel_initialization) {
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, "DBENGINIT[%zu]", tier);
            tiers_init[tier].thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_JOINABLE, dbengine_tier_init, &tiers_init[tier]);
        }
        else
            dbengine_tier_init(&tiers_init[tier]);
    }

    for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++) {
        if(parallel_initialization)
            nd_thread_join(tiers_init[tier].thread);

        if(tiers_init[tier].ret != 0) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "DBENGINE on '%s': Failed to initialize multi-host database tier %zu on path '%s'",
                   hostname, tiers_init[tier].tier, tiers_init[tier].path);
        }
        else if(created_tiers == tier)
            created_tiers++;
    }

    if(created_tiers && created_tiers < nd_profile.storage_tiers) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "DBENGINE on '%s': Managed to create %zu tiers instead of %zu. Continuing with %zu available.",
               hostname, created_tiers,
            nd_profile.storage_tiers, created_tiers);

        nd_profile.storage_tiers = created_tiers;
    }
    else if(!created_tiers)
        fatal("DBENGINE on '%s', failed to initialize databases at '%s'.", hostname, netdata_configured_cache_dir);

    for(size_t tier = 0; tier < nd_profile.storage_tiers;tier++)
        rrdeng_readiness_wait(multidb_ctx[tier]);

    rrdeng_calculate_tier_disk_space_percentage();

    dbengine_enabled = true;
#else
    nd_profile.storage_tiers = inicfg_get_number(&netdata_config, CONFIG_SECTION_DB, "storage tiers", 1);
    if(nd_profile.storage_tiers != 1) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "DBENGINE is not available on '%s', so only 1 database tier can be supported.",
               hostname);

        nd_profile.storage_tiers = 1;
        inicfg_set_number(&netdata_config, CONFIG_SECTION_DB, "storage tiers", nd_profile.storage_tiers);
    }
    dbengine_enabled = false;
#endif
}

void netdata_conf_section_db(void) {
    static bool run = false;
    if(run) return;
    run = true;

    // ------------------------------------------------------------------------
    // get default database update frequency

    nd_profile.update_every = (int) inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "update every", nd_profile.update_every);
    if(nd_profile.update_every < UPDATE_EVERY_MIN) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Data collection frequency in netdata.conf ([" CONFIG_SECTION_DB "].update every), changed from %d to %d",
               (int)nd_profile.update_every, UPDATE_EVERY_MIN);
        nd_profile.update_every = UPDATE_EVERY_MIN;
        inicfg_set_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "update every", nd_profile.update_every);
    }
    if(nd_profile.update_every > UPDATE_EVERY_MAX) {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Data collection frequency in netdata.conf ([" CONFIG_SECTION_DB "].update every), changed from %d to %d",
               (int)nd_profile.update_every, UPDATE_EVERY_MIN);
        nd_profile.update_every = UPDATE_EVERY_MAX;
        inicfg_set_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "update every", nd_profile.update_every);
    }

    // ------------------------------------------------------------------------
    // get the database selection

    {
        const char *mode = inicfg_get(&netdata_config, CONFIG_SECTION_DB, "db", rrd_memory_mode_name(default_rrd_memory_mode));
        default_rrd_memory_mode = rrd_memory_mode_id(mode);
        if(strcmp(mode, rrd_memory_mode_name(default_rrd_memory_mode)) != 0) {
            netdata_log_error("Invalid memory mode '%s' given. Using '%s'", mode, rrd_memory_mode_name(default_rrd_memory_mode));
            inicfg_set(&netdata_config, CONFIG_SECTION_DB, "db", rrd_memory_mode_name(default_rrd_memory_mode));
        }
    }

    // ------------------------------------------------------------------------
    // get default database size

    if(default_rrd_memory_mode != RRD_DB_MODE_DBENGINE && default_rrd_memory_mode != RRD_DB_MODE_NONE) {
        default_rrd_history_entries = (int)inicfg_get_duration_seconds(&netdata_config, 
            CONFIG_SECTION_DB, "retention",
            align_entries_to_pagesize(default_rrd_memory_mode, RRD_DEFAULT_HISTORY_ENTRIES));

        long h = align_entries_to_pagesize(default_rrd_memory_mode, default_rrd_history_entries);
        if (h != default_rrd_history_entries) {
            inicfg_set_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "retention", h);
            default_rrd_history_entries = (int)h;
        }
    }

    // --------------------------------------------------------------------
    // get KSM settings

#ifdef MADV_MERGEABLE
    enable_ksm = inicfg_get_boolean_ondemand(&netdata_config, CONFIG_SECTION_DB, "memory deduplication (ksm)", enable_ksm);
#endif

    // --------------------------------------------------------------------

    rrdhost_cleanup_orphan_to_archive_time_s =
        inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "cleanup orphan hosts after", rrdhost_cleanup_orphan_to_archive_time_s);
    if(rrdhost_cleanup_orphan_to_archive_time_s < 10) {
        rrdhost_cleanup_orphan_to_archive_time_s = 10;
        inicfg_set_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "cleanup orphan hosts after", rrdhost_cleanup_orphan_to_archive_time_s);
    }

    rrdhost_free_ephemeral_time_s =
        inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "cleanup ephemeral hosts after", rrdhost_free_ephemeral_time_s);
    if(rrdhost_free_ephemeral_time_s && rrdhost_free_ephemeral_time_s < rrdhost_cleanup_orphan_to_archive_time_s) {
        // the free ephemeral time cannot be less than the cleanup orphan time
        rrdhost_free_ephemeral_time_s = rrdhost_cleanup_orphan_to_archive_time_s;
        inicfg_set_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "cleanup ephemeral hosts after", rrdhost_free_ephemeral_time_s);
    }

    rrdset_free_obsolete_time_s =
        inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "cleanup obsolete charts after", rrdset_free_obsolete_time_s);
    if (rrdset_free_obsolete_time_s < 10) {
        // Current chart locking and invalidation scheme doesn't prevent Netdata from segmentation faults if a short
        // cleanup delay is set. Extensive stress tests showed that 10 seconds is quite a safe delay. Look at
        // https://github.com/netdata/netdata/pull/11222#issuecomment-868367920 for more information.
        rrdset_free_obsolete_time_s = 10;
        netdata_log_info("The \"cleanup obsolete charts after\" option was set to 10 seconds.");
        inicfg_set_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "cleanup obsolete charts after", rrdset_free_obsolete_time_s);
    }

    gap_when_lost_iterations_above = (int)inicfg_get_number(&netdata_config, CONFIG_SECTION_DB, "gap when lost iterations above", gap_when_lost_iterations_above);
    if (gap_when_lost_iterations_above < 1) {
        gap_when_lost_iterations_above = 1;
        inicfg_set_number(&netdata_config, CONFIG_SECTION_DB, "gap when lost iterations above", gap_when_lost_iterations_above);
    }
    gap_when_lost_iterations_above += 2;

    // ------------------------------------------------------------------------

    netdata_conf_dbengine_pre_logs();
}
