// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

#if defined(OS_MACOS)
int read_global_time(void) {
    static kernel_uint_t utime_raw = 0, stime_raw = 0, ntime_raw = 0;
    static usec_t collected_usec = 0, last_collected_usec = 0;

    host_cpu_load_info_data_t cpuinfo;
    mach_msg_type_number_t count = HOST_CPU_LOAD_INFO_COUNT;

    if (host_statistics(mach_host_self(), HOST_CPU_LOAD_INFO, (host_info_t)&cpuinfo, &count) != KERN_SUCCESS) {
        // Handle error
        goto cleanup;
    }

    last_collected_usec = collected_usec;
    collected_usec = now_monotonic_usec();

    calls_counter++;

    // Convert ticks to time
    // Note: MacOS does not separate nice time from user time in the CPU stats, so you might need to adjust this logic
    kernel_uint_t global_ntime = 0;  // Assuming you want to keep track of nice time separately

    incremental_rate(global_utime, utime_raw, cpuinfo.cpu_ticks[CPU_STATE_USER] + cpuinfo.cpu_ticks[CPU_STATE_NICE], collected_usec, last_collected_usec);
    incremental_rate(global_ntime, ntime_raw, cpuinfo.cpu_ticks[CPU_STATE_NICE], collected_usec, last_collected_usec);
    incremental_rate(global_stime, stime_raw, cpuinfo.cpu_ticks[CPU_STATE_SYSTEM], collected_usec, last_collected_usec);

    global_utime += global_ntime;

    if(unlikely(global_iterations_counter == 1)) {
        global_utime = 0;
        global_stime = 0;
        global_gtime = 0;
    }

    return 1;

cleanup:
    global_utime = 0;
    global_stime = 0;
    global_gtime = 0;
    return 0;
}
#endif


#if defined(OS_MACOS)
int read_global_time(void) {
    static kernel_uint_t utime_raw = 0, stime_raw = 0, ntime_raw = 0;
    static usec_t collected_usec = 0, last_collected_usec = 0;
    long cp_time[CPUSTATES];

    if (unlikely(CPUSTATES != 5)) {
        goto cleanup;
    } else {
        static int mib[2] = {0, 0};

        if (unlikely(GETSYSCTL_SIMPLE("kern.cp_time", mib, cp_time))) {
            goto cleanup;
        }
    }

    last_collected_usec = collected_usec;
    collected_usec = now_monotonic_usec();

    calls_counter++;

    // temporary - it is added global_ntime;
    kernel_uint_t global_ntime = 0;

    incremental_rate(global_utime, utime_raw, cp_time[0] * 100LLU / system_hz, collected_usec, last_collected_usec);
    incremental_rate(global_ntime, ntime_raw, cp_time[1] * 100LLU / system_hz, collected_usec, last_collected_usec);
    incremental_rate(global_stime, stime_raw, cp_time[2] * 100LLU / system_hz, collected_usec, last_collected_usec);

    global_utime += global_ntime;

    if(unlikely(global_iterations_counter == 1)) {
        global_utime = 0;
        global_stime = 0;
        global_gtime = 0;
    }

    return 1;

cleanup:
    global_utime = 0;
    global_stime = 0;
    global_gtime = 0;
    return 0;
}
#endif

#if defined(OS_WINDOWS)
int read_global_time(void) {
    // TODO: fix for windows
    return 0;
}
#endif

#if defined(OS_LINUX)
int read_global_time(void) {
    static char filename[FILENAME_MAX + 1] = "";
    static procfile *ff = NULL;
    static kernel_uint_t utime_raw = 0, stime_raw = 0, gtime_raw = 0, gntime_raw = 0, ntime_raw = 0;
    static usec_t collected_usec = 0, last_collected_usec = 0;

    if(unlikely(!ff)) {
        snprintfz(filename, FILENAME_MAX, "%s/proc/stat", netdata_configured_host_prefix);
        ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) goto cleanup;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) goto cleanup;

    last_collected_usec = collected_usec;
    collected_usec = now_monotonic_usec();

    calls_counter++;

    // temporary - it is added global_ntime;
    kernel_uint_t global_ntime = 0;

    incremental_rate(global_utime, utime_raw, str2kernel_uint_t(procfile_lineword(ff, 0,  1)), collected_usec, last_collected_usec);
    incremental_rate(global_ntime, ntime_raw, str2kernel_uint_t(procfile_lineword(ff, 0,  2)), collected_usec, last_collected_usec);
    incremental_rate(global_stime, stime_raw, str2kernel_uint_t(procfile_lineword(ff, 0,  3)), collected_usec, last_collected_usec);
    incremental_rate(global_gtime, gtime_raw, str2kernel_uint_t(procfile_lineword(ff, 0, 10)), collected_usec, last_collected_usec);

    global_utime += global_ntime;

    if(enable_guest_charts) {
        // temporary - it is added global_ntime;
        kernel_uint_t global_gntime = 0;

        // guest nice time, on guest time
        incremental_rate(global_gntime, gntime_raw, str2kernel_uint_t(procfile_lineword(ff, 0, 11)), collected_usec, last_collected_usec);

        global_gtime += global_gntime;

        // remove guest time from user time
        global_utime -= (global_utime > global_gtime) ? global_gtime : global_utime;
    }

    if(unlikely(global_iterations_counter == 1)) {
        global_utime = 0;
        global_stime = 0;
        global_gtime = 0;
    }

    return 1;

cleanup:
    global_utime = 0;
    global_stime = 0;
    global_gtime = 0;
    return 0;
}
#endif // !__FreeBSD__ !__APPLE__
