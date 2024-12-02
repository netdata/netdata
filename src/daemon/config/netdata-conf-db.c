// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-db.h"

static void netdata_conf_dbengine(void) {
    static bool run = false;
    if(run) return;
    run = true;

#ifdef ENABLE_DBENGINE
    // this is required for dbegnine to work, so call it here (it is ok, it won't run twice)
    netdata_conf_section_directories();

    // ------------------------------------------------------------------------
    // get default Database Engine page type

    const char *page_type = config_get(CONFIG_SECTION_DB, "dbengine page type", "gorilla");
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

    default_rrdeng_page_cache_mb = (int) config_get_size_mb(CONFIG_SECTION_DB, "dbengine page cache size", default_rrdeng_page_cache_mb);
    default_rrdeng_extent_cache_mb = (int) config_get_size_mb(CONFIG_SECTION_DB, "dbengine extent cache size", default_rrdeng_extent_cache_mb);
    db_engine_journal_check = config_get_boolean(CONFIG_SECTION_DB, "dbengine enable journal integrity check", CONFIG_BOOLEAN_NO);

    if(default_rrdeng_extent_cache_mb < 0) {
        default_rrdeng_extent_cache_mb = 0;
        config_set_size_mb(CONFIG_SECTION_DB, "dbengine extent cache size", default_rrdeng_extent_cache_mb);
    }

    if(default_rrdeng_page_cache_mb < RRDENG_MIN_PAGE_CACHE_SIZE_MB) {
        netdata_log_error("Invalid page cache size %d given. Defaulting to %d.", default_rrdeng_page_cache_mb, RRDENG_MIN_PAGE_CACHE_SIZE_MB);
        default_rrdeng_page_cache_mb = RRDENG_MIN_PAGE_CACHE_SIZE_MB;
        config_set_size_mb(CONFIG_SECTION_DB, "dbengine page cache size", default_rrdeng_page_cache_mb);
    }

    // ------------------------------------------------------------------------
    // out of memory protection and use all ram for caches

    dbengine_out_of_memory_protection = 0; // will be calculated below
    OS_SYSTEM_MEMORY sm = os_system_memory(true);
    if(sm.ram_total_bytes && sm.ram_available_bytes && sm.ram_total_bytes > sm.ram_available_bytes) {
        char buf[128];
        size_snprintf(buf, sizeof(buf), sm.ram_total_bytes / 10, "B", false);
        size_parse(buf, &dbengine_out_of_memory_protection, "B");
    }

    if(dbengine_out_of_memory_protection) {
        dbengine_use_all_ram_for_caches = config_get_boolean(CONFIG_SECTION_DB, "dbengine use all ram for caches", dbengine_use_all_ram_for_caches);
        dbengine_out_of_memory_protection = config_get_size_bytes(CONFIG_SECTION_DB, "dbengine out of memory protection", dbengine_out_of_memory_protection);
    }
    else {
        nd_log(NDLS_DAEMON, NDLP_WARNING, "Cannot get total and available RAM on this system, so cannot enable out of memory protection for dbengine, or to use all ram for caches.");
        dbengine_out_of_memory_protection = 0;
        dbengine_use_all_ram_for_caches = false;
    }

    // ------------------------------------------------------------------------
    // get default Database Engine disk space quota in MiB
    //
    //    //    if (!config_exists(CONFIG_SECTION_DB, "dbengine disk space MB") && !config_exists(CONFIG_SECTION_DB, "dbengine multihost disk space MB"))
    //
    //    default_rrdeng_disk_quota_mb = (int) config_get_number(CONFIG_SECTION_DB, "dbengine disk space MB", default_rrdeng_disk_quota_mb);
    //    if(default_rrdeng_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB) {
    //        netdata_log_error("Invalid dbengine disk space %d given. Defaulting to %d.", default_rrdeng_disk_quota_mb, RRDENG_MIN_DISK_SPACE_MB);
    //        default_rrdeng_disk_quota_mb = RRDENG_MIN_DISK_SPACE_MB;
    //        config_set_number(CONFIG_SECTION_DB, "dbengine disk space MB", default_rrdeng_disk_quota_mb);
    //    }
    //
    //    default_multidb_disk_quota_mb = (int) config_get_number(CONFIG_SECTION_DB, "dbengine multihost disk space MB", compute_multidb_diskspace());
    //    if(default_multidb_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB) {
    //        netdata_log_error("Invalid multidb disk space %d given. Defaulting to %d.", default_multidb_disk_quota_mb, default_rrdeng_disk_quota_mb);
    //        default_multidb_disk_quota_mb = default_rrdeng_disk_quota_mb;
    //        config_set_number(CONFIG_SECTION_DB, "dbengine multihost disk space MB", default_multidb_disk_quota_mb);
    //    }

#else
    if (default_rrd_memory_mode == RRD_MEMORY_MODE_DBENGINE) {
        error_report("RRD_MEMORY_MODE_DBENGINE is not supported in this platform. The agent will use db mode 'save' instead.");
        default_rrd_memory_mode = RRD_MEMORY_MODE_RAM;
    }
#endif
}


void netdata_conf_section_db(void) {
    static bool run = false;
    if(run) return;
    run = true;

    // ------------------------------------------------------------------------

    rrdhost_free_orphan_time_s =
        config_get_duration_seconds(CONFIG_SECTION_DB, "cleanup orphan hosts after", rrdhost_free_orphan_time_s);

    // ------------------------------------------------------------------------
    // get default database update frequency

    default_rrd_update_every = (int) config_get_duration_seconds(CONFIG_SECTION_DB, "update every", UPDATE_EVERY);
    if(default_rrd_update_every < 1 || default_rrd_update_every > 600) {
        netdata_log_error("Invalid data collection frequency (update every) %d given. Defaulting to %d.", default_rrd_update_every, UPDATE_EVERY);
        default_rrd_update_every = UPDATE_EVERY;
        config_set_duration_seconds(CONFIG_SECTION_DB, "update every", default_rrd_update_every);
    }

    // ------------------------------------------------------------------------
    // get the database selection

    {
        const char *mode = config_get(CONFIG_SECTION_DB, "db", rrd_memory_mode_name(default_rrd_memory_mode));
        default_rrd_memory_mode = rrd_memory_mode_id(mode);
        if(strcmp(mode, rrd_memory_mode_name(default_rrd_memory_mode)) != 0) {
            netdata_log_error("Invalid memory mode '%s' given. Using '%s'", mode, rrd_memory_mode_name(default_rrd_memory_mode));
            config_set(CONFIG_SECTION_DB, "db", rrd_memory_mode_name(default_rrd_memory_mode));
        }
    }

    // ------------------------------------------------------------------------
    // get default database size

    if(default_rrd_memory_mode != RRD_MEMORY_MODE_DBENGINE && default_rrd_memory_mode != RRD_MEMORY_MODE_NONE) {
        default_rrd_history_entries = (int)config_get_number(
            CONFIG_SECTION_DB, "retention",
            align_entries_to_pagesize(default_rrd_memory_mode, RRD_DEFAULT_HISTORY_ENTRIES));

        long h = align_entries_to_pagesize(default_rrd_memory_mode, default_rrd_history_entries);
        if (h != default_rrd_history_entries) {
            config_set_number(CONFIG_SECTION_DB, "retention", h);
            default_rrd_history_entries = (int)h;
        }
    }

    // --------------------------------------------------------------------
    // get KSM settings

#ifdef MADV_MERGEABLE
    enable_ksm = config_get_boolean_ondemand(CONFIG_SECTION_DB, "memory deduplication (ksm)", enable_ksm);
#endif

    // --------------------------------------------------------------------

    rrdhost_free_ephemeral_time_s =
        config_get_duration_seconds(CONFIG_SECTION_DB, "cleanup ephemeral hosts after", rrdhost_free_ephemeral_time_s);

    rrdset_free_obsolete_time_s =
        config_get_duration_seconds(CONFIG_SECTION_DB, "cleanup obsolete charts after", rrdset_free_obsolete_time_s);

    // Current chart locking and invalidation scheme doesn't prevent Netdata from segmentation faults if a short
    // cleanup delay is set. Extensive stress tests showed that 10 seconds is quite a safe delay. Look at
    // https://github.com/netdata/netdata/pull/11222#issuecomment-868367920 for more information.
    if (rrdset_free_obsolete_time_s < 10) {
        rrdset_free_obsolete_time_s = 10;
        netdata_log_info("The \"cleanup obsolete charts after\" option was set to 10 seconds.");
        config_set_duration_seconds(CONFIG_SECTION_DB, "cleanup obsolete charts after", rrdset_free_obsolete_time_s);
    }

    gap_when_lost_iterations_above = (int)config_get_number(CONFIG_SECTION_DB, "gap when lost iterations above", gap_when_lost_iterations_above);
    if (gap_when_lost_iterations_above < 1) {
        gap_when_lost_iterations_above = 1;
        config_set_number(CONFIG_SECTION_DB, "gap when lost iterations above", gap_when_lost_iterations_above);
    }
    gap_when_lost_iterations_above += 2;

    // ------------------------------------------------------------------------

    netdata_conf_dbengine();
}
