// SPDX-License-Identifier: GPL-3.0-or-later

#define RRDHOST_INTERNALS
#include "rrd.h"

// Windows-only: append timestamped messages to %TEMP%\netdata-trace.log.
// File-local static; same destination as nd_win_trace() in main.c and environment.c.
#if defined(OS_WINDOWS)
__attribute__((format(printf, 1, 2)))
static void nd_win_trace_rrd(const char *fmt, ...) {
    char path[MAX_PATH + 1];
    DWORD len = GetTempPathA(MAX_PATH, path);
    if (!len || len >= (DWORD)(MAX_PATH - 22)) return;
    strcat(path, "netdata-trace.log");
    // FILE_FLAG_NO_REPARSE_POINT: fail if the target is a symlink or junction
    // rather than following it — the service runs with elevated privileges.
    HANDLE h = CreateFileA(path, FILE_APPEND_DATA,
                           FILE_SHARE_READ | FILE_SHARE_WRITE,
                           NULL, OPEN_ALWAYS,
                           FILE_ATTRIBUTE_NORMAL | FILE_FLAG_NO_REPARSE_POINT,
                           NULL);
    if (h == INVALID_HANDLE_VALUE) return;
    int fd = _open_osfhandle((intptr_t)h, _O_APPEND | _O_WRONLY | _O_TEXT);
    if (fd < 0) { CloseHandle(h); return; }
    FILE *f = _fdopen(fd, "a");
    if (!f) { _close(fd); return; }
    SYSTEMTIME t;
    GetSystemTime(&t);
    fprintf(f, "%02d:%02d:%02d.%03d - ", t.wHour, t.wMinute, t.wSecond, t.wMilliseconds);
    va_list args;
    va_start(args, fmt);
    vfprintf(f, fmt, args);
    va_end(args);
    fputc('\n', f);
    fflush(f);
    fclose(f);
}
#else
#define nd_win_trace_rrd(fmt, ...) do {} while(0)
#endif

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

    size_t len = strlen(s);
    size_t dst_size = (len * 2) + 1;
    char *buf = mallocz(dst_size);

    // Sanitize the string, preserving valid UTF-8
    text_sanitize((unsigned char *)buf, (const unsigned char *)s, dst_size,
                  rrd_string_allowed_chars, true, "", NULL);

    STRING *result = string_strdupz(buf);
    freez(buf);
    return result;
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

// Thread-exit cleanup callbacks. Registered with libnetdata below;
// called once per thread at exit time, in registration order. All
// callbacks except rrdset_thread_rda_free are declared in their
// owning headers (included via rrd.h); rrdset_thread_rda_free has no
// public header so it is declared locally here.
void rrdset_thread_rda_free(void);

int rrd_init(const char *hostname, struct rrdhost_system_info *system_info, bool unittest) {
    nd_thread_register_cleanup(rrd_collector_finished);
    nd_thread_register_cleanup(sender_thread_buffer_free);
    nd_thread_register_cleanup(rrdset_thread_rda_free);
    nd_thread_register_cleanup(query_target_free);

    nd_win_trace_rrd("rrd_init: rrdhost_init...");
    rrdhost_init();
    nd_win_trace_rrd("rrd_init: rrdhost_init done");

    nd_win_trace_rrd("rrd_init: sql_init_meta_database...");
    if (unlikely(sql_init_meta_database(DB_CHECK_NONE, system_info ? 0 : 1))) {
        if (default_rrd_memory_mode == RRD_DB_MODE_DBENGINE) {
            set_late_analytics_variables(system_info);
            nd_win_trace_rrd("rrd_init: sql_init_meta_database FAILED (dbengine mode), calling fatal");
            fatal("Failed to initialize SQLite");
        }

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "Skipping SQLITE metadata initialization since memory mode is not dbengine");
    }
    nd_win_trace_rrd("rrd_init: sql_init_meta_database done");

    nd_win_trace_rrd("rrd_init: sql_init_context_database...");
    if (unlikely(sql_init_context_database(system_info ? 0 : 1))) {
        error_report("Failed to initialize context metadata database");
    }
    nd_win_trace_rrd("rrd_init: sql_init_context_database done");

    if (unlikely(unittest)) {
        dbengine_enabled = true;
    }
    else {
        if (default_rrd_memory_mode == RRD_DB_MODE_DBENGINE || stream_conf_receiver_needs_dbengine()) {
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "DBENGINE: Initializing ...");

            nd_win_trace_rrd("rrd_init: netdata_conf_dbengine_init...");
            netdata_conf_dbengine_init(hostname);
            nd_win_trace_rrd("rrd_init: netdata_conf_dbengine_init done, dbengine_enabled=%d", (int)dbengine_enabled);
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
        nd_win_trace_rrd("rrd_init: metadata_sync_init...");
        metadata_sync_init();
        nd_win_trace_rrd("rrd_init: metadata_sync_init done");
        nd_win_trace_rrd("rrd_init: health_load_config_defaults...");
        health_load_config_defaults();
        nd_win_trace_rrd("rrd_init: health_load_config_defaults done");
    }

    nd_win_trace_rrd("rrd_init: rrdhost_create('%s')...", hostname);
    SYSTEM_TZ tz = system_tz_get();
    localhost = rrdhost_create(
        hostname
        , registry_get_this_machine_hostname()
        , machine_guid_get_txt()
        , os_type
        , tz.timezone
        , tz.abbrev_timezone
        , tz.utc_offset
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
    system_tz_free(&tz);
    rrdhost_system_info_free(system_info);
    nd_win_trace_rrd("rrd_init: rrdhost_create done, localhost=%p", (void *)localhost);

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

    nd_win_trace_rrd("rrd_init: complete, returning 0");
    return 0;
}
