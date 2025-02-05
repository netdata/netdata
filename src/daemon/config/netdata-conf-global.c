// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-global.h"
#include "daemon/common.h"

size_t netdata_conf_cpus(void) {
    static size_t processors = 0;

    if(processors)
        return processors;

    SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);
    size_t p = 0;

    if(processors)
        goto skip;

#if defined(OS_LINUX)
    p = os_read_cpuset_cpus("/sys/fs/cgroup/cpuset.cpus", p);
    if(!p)
        p = os_read_cpuset_cpus("/sys/fs/cgroup/cpuset/cpuset.cpus", p);
#endif

    if(!p)
        p = os_get_system_cpus_uncached();

    p = inicfg_get_number(&netdata_config, CONFIG_SECTION_GLOBAL, "cpu cores", p);
    if(p < 1)
        p = 1;

    processors = p;

    char buf[24];
    snprintfz(buf, sizeof(buf), "%zu", processors);
    nd_setenv("NETDATA_CONF_CPUS", buf, 1);

skip:
    spinlock_unlock(&spinlock);
    return processors;
}

void netdata_conf_glibc_malloc_initialize(size_t wanted_arenas, size_t trim_threshold __maybe_unused) {
    wanted_arenas = inicfg_get_number(&netdata_config, CONFIG_SECTION_GLOBAL, "glibc malloc arena max for plugins", wanted_arenas);
    if(wanted_arenas < 1 || wanted_arenas > os_get_system_cpus_cached(true)) {
        if(wanted_arenas < 1) wanted_arenas = 1;
        else wanted_arenas = os_get_system_cpus_cached(true);
        inicfg_set_number(&netdata_config, CONFIG_SECTION_GLOBAL, "glibc malloc arena max for plugins", wanted_arenas);
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "malloc arenas can be from 1 to %zu. Setting it to %zu",
               os_get_system_cpus_cached(true), wanted_arenas);
    }

    char buf[32];
    snprintfz(buf, sizeof(buf), "%zu", wanted_arenas);
    setenv("MALLOC_ARENA_MAX", buf, true);

#if defined(HAVE_C_MALLOPT)
    wanted_arenas = inicfg_get_number(&netdata_config, CONFIG_SECTION_GLOBAL, "glibc malloc arena max for netdata", wanted_arenas);
    if(wanted_arenas < 1 || wanted_arenas > os_get_system_cpus_cached(true)) {
        if(wanted_arenas < 1) wanted_arenas = 1;
        else wanted_arenas = os_get_system_cpus_cached(true);
        inicfg_set_number(&netdata_config, CONFIG_SECTION_GLOBAL, "glibc malloc arena max for netdata", wanted_arenas);
        nd_log(NDLS_DAEMON, NDLP_NOTICE,
               "malloc arenas can be from 1 to %zu. Setting it to %zu",
               os_get_system_cpus_cached(true), wanted_arenas);
    }
    mallopt(M_ARENA_MAX, (int)wanted_arenas);
    mallopt(M_TRIM_THRESHOLD, (int)trim_threshold);

#ifdef NETDATA_INTERNAL_CHECKS
    mallopt(M_PERTURB, 0x5A);
    // mallopt(M_MXFAST, 0);
#endif
#endif
}

void libuv_initialize(void) {
    libuv_worker_threads = (int)inicfg_get_number_range(
        &netdata_config, CONFIG_SECTION_GLOBAL, "libuv worker threads",
        (int)netdata_conf_cpus() * 6, MIN_LIBUV_WORKER_THREADS, MAX_LIBUV_WORKER_THREADS);

    char buf[20 + 1];
    snprintfz(buf, sizeof(buf) - 1, "%d", libuv_worker_threads);
    setenv("UV_THREADPOOL_SIZE", buf, 1);
}

void netdata_conf_section_global(void) {
    // ------------------------------------------------------------------------
    // get the hostname

    netdata_configured_host_prefix = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "host access prefix", "");
    (void) verify_netdata_host_prefix(true);

    char buf[HOST_NAME_MAX * 4 + 1];
    if (!os_hostname(buf, sizeof(buf), netdata_configured_host_prefix))
        netdata_log_error("Cannot get machine hostname.");

    netdata_configured_hostname = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "hostname", buf);
    netdata_log_debug(D_OPTIONS, "hostname set to '%s'", netdata_configured_hostname);

    netdata_conf_section_directories();

    nd_profile_setup(); // required for configuring the database
    netdata_conf_section_db();

    // --------------------------------------------------------------------
    // get various system parameters

    os_get_system_cpus_uncached();
    os_get_system_pid_max();
}

void netdata_conf_section_global_run_as_user(const char **user) {
    // --------------------------------------------------------------------
    // get the user we should run

    // IMPORTANT: this is required before web_files_uid()
    if(getuid() == 0) {
        *user = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "run as user", NETDATA_USER);
    }
    else {
        struct passwd *passwd = getpwuid(getuid());
        *user = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "run as user", (passwd && passwd->pw_name)?passwd->pw_name:"");
    }
}

