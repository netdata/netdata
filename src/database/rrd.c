// SPDX-License-Identifier: GPL-3.0-or-later

#define RRDHOST_INTERNALS
#include "rrd.h"

// --------------------------------------------------------------------------------------------------------------------
// globals

/*
// if not zero it gives the time (in seconds) to remove un-updated dimensions
// DO NOT ENABLE
// if dimensions are removed, the chart generation will have to run again
int rrd_delete_unupdated_dimensions = 0;
*/

#ifdef ENABLE_DBENGINE
RRD_DB_MODE default_rrd_memory_mode = RRD_DB_MODE_DBENGINE;
#else
RRD_DB_MODE default_rrd_memory_mode = RRD_DB_MODE_RAM;
#endif
int gap_when_lost_iterations_above = 1;


// --------------------------------------------------------------------------------------------------------------------
// RRD - string management

STRING *rrd_string_strdupz(const char *s) {
    if(unlikely(!s || !*s)) return string_strdupz(s);

    char *tmp = strdupz(s);
    json_fix_string(tmp);
    STRING *ret = string_strdupz(tmp);
    freez(tmp);
    return ret;
}

// --------------------------------------------------------------------------------------------------------------------

inline long align_entries_to_pagesize(RRD_DB_MODE mode, long entries) {
    if(mode == RRD_DB_MODE_DBENGINE) return 0;
    if(mode == RRD_DB_MODE_NONE) return 5;

    if(entries < 5) entries = 5;
    if(entries > RRD_HISTORY_ENTRIES_MAX) entries = RRD_HISTORY_ENTRIES_MAX;

    if(mode == RRD_DB_MODE_RAM) {
        long header_size = 0;

        long page = (long)sysconf(_SC_PAGESIZE);
        long size = (long)(header_size + entries * sizeof(storage_number));
        if (unlikely(size % page)) {
            size -= (size % page);
            size += page;

            long n = (long)((size - header_size) / sizeof(storage_number));
            return n;
        }
    }

    return entries;
}

// --------------------------------------------------------------------------------------------------------------------

void api_v1_management_init(void);

int rrd_init(const char *hostname, struct rrdhost_system_info *system_info, bool unittest) {
    rrdhost_init();

    if (unlikely(sql_init_meta_database(DB_CHECK_NONE, system_info ? 0 : 1))) {
        if (default_rrd_memory_mode == RRD_DB_MODE_DBENGINE) {
            set_late_analytics_variables(system_info);
            fatal("Failed to initialize SQLite");
        }

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "Skipping SQLITE metadata initialization since memory mode is not dbengine");
    }

    if (unlikely(sql_init_context_database(system_info ? 0 : 1))) {
        error_report("Failed to initialize context metadata database");
    }

    if (unlikely(unittest)) {
        dbengine_enabled = true;
    }
    else {
        if (default_rrd_memory_mode == RRD_DB_MODE_DBENGINE || stream_conf_receiver_needs_dbengine()) {
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "DBENGINE: Initializing ...");

            netdata_conf_dbengine_init(hostname);
        }
        else
            nd_profile.storage_tiers = 1;

        if (!dbengine_enabled) {
            if (nd_profile.storage_tiers > 1) {
                nd_log(NDLS_DAEMON, NDLP_WARNING,
                       "dbengine is not enabled, but %zu tiers have been requested. Resetting tiers to 1",
                       nd_profile.storage_tiers);

                nd_profile.storage_tiers = 1;
            }

            if (default_rrd_memory_mode == RRD_DB_MODE_DBENGINE) {
                nd_log(NDLS_DAEMON, NDLP_WARNING,
                       "dbengine is not enabled, but it has been given as the default db mode. "
                       "Resetting db mode to alloc");

                default_rrd_memory_mode = RRD_DB_MODE_ALLOC;
            }
        }
    }

    if(!unittest) {
        metadata_sync_init();
        health_load_config_defaults();
    }

    localhost = rrdhost_create(
        hostname
        , registry_get_this_machine_hostname()
        , machine_guid_get_txt()
        , os_type
        , netdata_configured_timezone
        , netdata_configured_abbrev_timezone
        , netdata_configured_utc_offset
        , program_name
        , NETDATA_VERSION
        , nd_profile.update_every, default_rrd_history_entries
        , default_rrd_memory_mode
        , health_plugin_enabled()
        , stream_send.enabled
        , stream_send.parents.destination
        , stream_send.api_key
        , stream_send.send_charts_matching
        , stream_receive.replication.enabled
        , stream_receive.replication.period
        , stream_receive.replication.step
        , system_info
        , 1
        , 0
    );
    rrdhost_system_info_free(system_info);

    if (unlikely(!localhost))
        return 1;

    rrdhost_flag_set(localhost, RRDHOST_FLAG_COLLECTOR_ONLINE);
    object_state_activate(&localhost->state_id);
    pulse_host_status(localhost, 0, 0); // this will detect the receiver status

    ml_host_start(localhost);
    dyncfg_host_init(localhost);

    if(!unittest)
        health_plugin_init();

    global_functions_add();

    if (likely(system_info)) {
        detect_machine_guid_change(&localhost->host_id.uuid);
        aclk_synchronization_init();
        api_v1_management_init();
    }

    return 0;
}
