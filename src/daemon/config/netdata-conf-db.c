// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-db.h"
#include "daemon/common.h"

#define DAYS 86400
int default_rrd_history_entries = RRD_DEFAULT_HISTORY_ENTRIES;

bool dbengine_enabled = false; // will become true if and when dbengine is initialized
bool dbengine_datafiles_present = false; // detected at startup, regardless of the configured memory mode
#ifdef ENABLE_DBENGINE
struct dbengine_config netdata_conf_dbengine = DBENGINE_CONFIG_DEFAULTS;
static uint8_t dbengine_tier0_page_type = RRDENG_PAGE_TYPE_GORILLA_32BIT;
int default_rrdeng_disk_quota_mb = RRDENG_DEFAULT_TIER_DISK_SPACE_MB;
int default_multidb_disk_quota_mb = RRDENG_DEFAULT_TIER_DISK_SPACE_MB;
bool new_dbengine_defaults = false;
bool legacy_multihost_db_space = false;
#endif
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
    FUNCTION_RUN_ONCE();

    errno_clear();

#ifdef ENABLE_DBENGINE
    // this is required for dbegnine to work, so call it here (it is ok, it won't run twice)
    netdata_conf_section_directories();

    // ------------------------------------------------------------------------
    // get default Database Engine page type

    const char *page_type = inicfg_get(&netdata_config, CONFIG_SECTION_DB, "dbengine page type", "gorilla");
    if (strcmp(page_type, "gorilla") == 0)
        dbengine_tier0_page_type = RRDENG_PAGE_TYPE_GORILLA_32BIT;
    else if (strcmp(page_type, "raw") == 0)
        dbengine_tier0_page_type = RRDENG_PAGE_TYPE_ARRAY_32BIT;
    else {
        dbengine_tier0_page_type = RRDENG_PAGE_TYPE_ARRAY_32BIT;
        netdata_log_error("Invalid dbengine page type ''%s' given. Defaulting to 'raw'.", page_type);
    }

    // ------------------------------------------------------------------------
    // get default Database Engine page cache size in MiB

    // read as int so that negative or too small values are visible before they become sizes
    int page_cache_mb = (int) inicfg_get_size_mb(&netdata_config, CONFIG_SECTION_DB, "dbengine page cache size", (int) netdata_conf_dbengine.page_cache_mb);
    int extent_cache_mb = (int) inicfg_get_size_mb(&netdata_config, CONFIG_SECTION_DB, "dbengine extent cache size", (int) netdata_conf_dbengine.extent_cache_mb);
    netdata_conf_dbengine.journal_integrity_check = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_DB, "dbengine enable journal integrity check", CONFIG_BOOLEAN_NO);

    if(extent_cache_mb < 0) {
        extent_cache_mb = 0;
        inicfg_set_size_mb(&netdata_config, CONFIG_SECTION_DB, "dbengine extent cache size", extent_cache_mb);
    }

    if(page_cache_mb < RRDENG_MIN_PAGE_CACHE_SIZE_MB) {
        netdata_log_error("Invalid page cache size %d given. Defaulting to %d.", page_cache_mb, RRDENG_MIN_PAGE_CACHE_SIZE_MB);
        page_cache_mb = RRDENG_MIN_PAGE_CACHE_SIZE_MB;
        inicfg_set_size_mb(&netdata_config, CONFIG_SECTION_DB, "dbengine page cache size", page_cache_mb);
    }

    netdata_conf_dbengine.page_cache_mb = (size_t) page_cache_mb;
    netdata_conf_dbengine.extent_cache_mb = (size_t) extent_cache_mb;

#else
    if (default_rrd_memory_mode == RRD_DB_MODE_DBENGINE) {
        error_report("RRD_DB_MODE_DBENGINE is not supported in this platform. The agent will use db mode 'save' instead.");
        default_rrd_memory_mode = RRD_DB_MODE_RAM;
    }
#endif
}

#ifdef ENABLE_DBENGINE
uint8_t netdata_conf_dbengine_page_type(size_t tier) {
    // only tier 0 is configurable; the higher tiers hold aggregates, which one page type stores
    return tier ? RRDENG_PAGE_TYPE_ARRAY_TIER1 : dbengine_tier0_page_type;
}

void netdata_conf_dbengine_tier_config(size_t tier, struct rrdeng_tier_config *out) {
    memset(out, 0, sizeof(*out));
    out->tier = tier;
    out->page_type = netdata_conf_dbengine_page_type(tier);
    out->grouping = get_tier_grouping(tier);
}

struct dbengine_initialization {
    ND_THREAD *thread;
    char path[FILENAME_MAX + 1];
    struct rrdeng_tier_config config;
    int ret;
};

void dbengine_tier_init(void *ptr) {
    struct dbengine_initialization *dbi = ptr;
    dbi->ret = rrdeng_init(NULL, &dbi->config);
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

void netdata_conf_dbengine_apply(void) {
#ifdef ENABLE_DBENGINE
    // settings the daemon resolves elsewhere, and on some paths (the unit tests) never from netdata.conf:
    // snapshot them at the moment the engine needs them
    netdata_conf_dbengine.cpus = netdata_conf_cpus();
    netdata_conf_dbengine.arals_for_large_pages = netdata_conf_is_parent();
    netdata_conf_dbengine.cache_statistics = pulse_enabled;
    netdata_conf_dbengine.compression_statistics = pulse_extended_enabled;
    netdata_conf_dbengine.default_update_every_s = nd_profile.update_every;
    netdata_conf_dbengine.libuv_worker_threads = libuv_worker_threads;
    netdata_conf_dbengine.reserved_libuv_worker_threads = RESERVED_LIBUV_WORKER_THREADS;
    netdata_conf_dbengine.on_db_rotation = rrdcontext_db_rotation;
    netdata_conf_dbengine.preload_metrics = populate_metrics_from_database;

    dbengine_init(&netdata_conf_dbengine);
#endif
}

void netdata_conf_dbengine_init(const char *hostname) {
#ifdef ENABLE_DBENGINE

    // ----------------------------------------------------------------------------------------------------------------
    // out of memory protection and use all ram for caches

    netdata_conf_dbengine.out_of_memory_protection_bytes = 0; // will be calculated below
    OS_SYSTEM_MEMORY sm = os_system_memory(true);
    if(OS_SYSTEM_MEMORY_OK(sm) && sm.ram_total_bytes > sm.ram_available_bytes) {
        // calculate the default out of memory protection size
        uint64_t keep_free = sm.ram_total_bytes / 10;
        if(keep_free > 5ULL * 1024 * 1024 * 1024)
            keep_free = 5ULL * 1024 * 1024 * 1024;
        char buf[64];
        size_snprintf(buf, sizeof(buf), keep_free, "B", false);
        size_parse(buf, &netdata_conf_dbengine.out_of_memory_protection_bytes, "B");
    }

    if(netdata_conf_dbengine.out_of_memory_protection_bytes) {
        netdata_conf_dbengine.use_all_ram_for_caches = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_DB, "dbengine use all ram for caches", netdata_conf_dbengine.use_all_ram_for_caches);
        netdata_conf_dbengine.out_of_memory_protection_bytes = inicfg_get_size_bytes(&netdata_config, CONFIG_SECTION_DB, "dbengine out of memory protection", netdata_conf_dbengine.out_of_memory_protection_bytes);

        char buf_total[64], buf_avail[64], buf_oom[64];
        size_snprintf(buf_total, sizeof(buf_total), sm.ram_total_bytes, "B", false);
        size_snprintf(buf_avail, sizeof(buf_avail), sm.ram_available_bytes, "B", false);
        size_snprintf(buf_oom, sizeof(buf_oom), netdata_conf_dbengine.out_of_memory_protection_bytes, "B", false);

        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "DBENGINE memory protection enabled. "
               "Netdata will limit DBENGINE memory usage to help keep at least %s of system RAM available when possible and reduce OOM risk. "
               "System memory total: %s, currently available: %s, use all RAM for caches: %s",
               buf_oom, buf_total, buf_avail, netdata_conf_dbengine.use_all_ram_for_caches ? "enabled" : "disabled");
    }
    else {
        netdata_conf_dbengine.out_of_memory_protection_bytes = 0;
        netdata_conf_dbengine.use_all_ram_for_caches = false;

        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "DBENGINE memory protection is disabled because Netdata could not detect system memory size. "
               "\"use all RAM for caches\" is also disabled.");
    }

    // ----------------------------------------------------------------------------------------------------------------

    netdata_conf_dbengine.direct_io = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_DB, "dbengine use direct io", netdata_conf_dbengine.direct_io);
    netdata_conf_dbengine.journal_v2_unmount_time_s = inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "dbengine journal v2 unmount time", nd_profile.dbengine_journal_v2_unmount_time);

    unsigned read_num = (unsigned)inicfg_get_number(&netdata_config, CONFIG_SECTION_DB, "dbengine pages per extent", DBENGINE_DEFAULT_PAGES_PER_EXTENT);
    if (read_num > 0 && read_num <= DBENGINE_DEFAULT_PAGES_PER_EXTENT)
        netdata_conf_dbengine.pages_per_extent = read_num;
    else {
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "Invalid dbengine pages per extent %u given. Using %u.",
               read_num, netdata_conf_dbengine.pages_per_extent);

        inicfg_set_number(&netdata_config, CONFIG_SECTION_DB, "dbengine pages per extent", netdata_conf_dbengine.pages_per_extent);
    }

    // the process-wide configuration is complete: hand it to the engine before any tier starts
    netdata_conf_dbengine_apply();

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

    for (size_t tier = 1; tier < nd_profile.storage_tiers; tier++) {
        size_t grouping_iterations = storage_tiers_grouping_iterations[tier];
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

        strncpyz(tiers_init[tier].path, dbenginepath, FILENAME_MAX);
        netdata_conf_dbengine_tier_config(tier, &tiers_init[tier].config);
        tiers_init[tier].config.dbfiles_path = tiers_init[tier].path;
        tiers_init[tier].config.disk_space_mb = (unsigned) disk_space_mb;
        tiers_init[tier].config.max_retention_s = storage_tiers_retention_time_s[tier];
        tiers_init[tier].ret = 0;

        if(parallel_initialization) {
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, "DBENGINIT[%zu]", tier);
            tiers_init[tier].thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, dbengine_tier_init, &tiers_init[tier]);
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
                   hostname, tiers_init[tier].config.tier, tiers_init[tier].path);
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
    FUNCTION_RUN_ONCE();

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

#ifdef ENABLE_DBENGINE
    // Detect an actual dbengine datafile (".ndf") in ANY tier directory. Two
    // reasons to scan datafiles across every tier rather than test one directory:
    //   - an empty dbengine dir left by a failed/aborted init is NOT persisted
    //     data and must not disable RAM-mode metadata cleanup;
    //   - tier 0 (<cache>/dbengine) can have no datafiles while a higher tier
    //     (<cache>/dbengine-tierN) still retains history, so a tier-0-only check
    //     could wrongly report "no data" and delete metadata still backing
    //     tier-N data (data loss).
    // Iterate the compile-time maximum RRD_STORAGE_TIERS, not the configured
    // nd_profile.storage_tiers: in a non-dbengine mode the configured count is
    // forced to 1, but the on-disk data was written by a previous dbengine run
    // that may have created up to RRD_STORAGE_TIERS tier directories. Tier dirs
    // that do not exist are simply skipped.
    // A datafile in any tier means this agent has dbengine data on disk even when
    // currently running a non-dbengine mode, so RAM cleanup keeps dimension rows
    // that still back that data (agent temporarily switched dbengine -> ram/alloc).
    // Only meaningful in a dbengine-capable build; without dbengine the data can
    // never be read back, so the flag stays false and RAM cleanup proceeds.
    for (size_t tier = 0; tier < RRD_STORAGE_TIERS && !dbengine_datafiles_present; tier++) {
        char dbenginepath[FILENAME_MAX + 1];
        if (tier == 0)
            snprintfz(dbenginepath, sizeof(dbenginepath) - 1, "%s/dbengine", netdata_configured_cache_dir);
        else
            snprintfz(dbenginepath, sizeof(dbenginepath) - 1, "%s/dbengine-tier%zu", netdata_configured_cache_dir, tier);

        DIR *dir = opendir(dbenginepath);
        if (!dir)
            continue;

        struct dirent *de;
        while ((de = readdir(dir))) {
            // Validate the name exactly as dbengine generates and scans it
            // ("datafile-<tier>-<fileno>.ndf"), using the same sscanf template as
            // scan_data_files(). Requiring both numeric fields to parse means an
            // unrelated *.ndf entry is not mistaken for persisted data.
            unsigned int df_tier, df_fileno;
            if (sscanf(de->d_name, DATAFILE_PREFIX RRDENG_FILE_NUMBER_SCAN_TMPL DATAFILE_EXTENSION,
                       &df_tier, &df_fileno) == 2) {
                dbengine_datafiles_present = true;
                break;
            }
        }
        closedir(dir);
    }
#endif

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
