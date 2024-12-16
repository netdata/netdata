// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-global.h"

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

    p = config_get_number(CONFIG_SECTION_GLOBAL, "cpu cores", p);
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

static int get_hostname(char *buf, size_t buf_size) {
    if (netdata_configured_host_prefix && *netdata_configured_host_prefix) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/etc/hostname", netdata_configured_host_prefix);

        if (!read_txt_file(filename, buf, buf_size)) {
            trim(buf);
            return 0;
        }
    }

    int rc = gethostname(buf, buf_size);
    buf[buf_size - 1] = '\0';
    return rc;
}

static void glibc_initialize(void) {
    const char *pmax = config_get(CONFIG_SECTION_GLOBAL, "glibc malloc arena max for plugins", "1");
    if(pmax && *pmax)
        setenv("MALLOC_ARENA_MAX", pmax, 1);

#if defined(HAVE_C_MALLOPT)
    int i = (int)config_get_number(CONFIG_SECTION_GLOBAL, "glibc malloc arena max for netdata", 1);
    if(i > 0)
        mallopt(M_ARENA_MAX, 1);

#ifdef NETDATA_INTERNAL_CHECKS
    mallopt(M_PERTURB, 0x5A);
    // mallopt(M_MXFAST, 0);
#endif
#endif
}

static void libuv_initialize(void) {
    libuv_worker_threads = (int)netdata_conf_cpus() * 6;

    if(libuv_worker_threads < MIN_LIBUV_WORKER_THREADS)
        libuv_worker_threads = MIN_LIBUV_WORKER_THREADS;

    if(libuv_worker_threads > MAX_LIBUV_WORKER_THREADS)
        libuv_worker_threads = MAX_LIBUV_WORKER_THREADS;


    libuv_worker_threads = config_get_number(CONFIG_SECTION_GLOBAL, "libuv worker threads", libuv_worker_threads);
    if(libuv_worker_threads < MIN_LIBUV_WORKER_THREADS) {
        libuv_worker_threads = MIN_LIBUV_WORKER_THREADS;
        config_set_number(CONFIG_SECTION_GLOBAL, "libuv worker threads", libuv_worker_threads);
    }

    char buf[20 + 1];
    snprintfz(buf, sizeof(buf) - 1, "%d", libuv_worker_threads);
    setenv("UV_THREADPOOL_SIZE", buf, 1);
}

void netdata_conf_section_global(void) {
    // ------------------------------------------------------------------------
    // get the hostname

    netdata_configured_host_prefix = config_get(CONFIG_SECTION_GLOBAL, "host access prefix", "");
    (void) verify_netdata_host_prefix(true);

    char buf[HOSTNAME_MAX + 1];
    if (get_hostname(buf, sizeof(buf)))
        netdata_log_error("Cannot get machine hostname.");

    netdata_configured_hostname = config_get(CONFIG_SECTION_GLOBAL, "hostname", buf);
    netdata_log_debug(D_OPTIONS, "hostname set to '%s'", netdata_configured_hostname);

    netdata_conf_section_directories();
    netdata_conf_section_db();

    // --------------------------------------------------------------------
    // get various system parameters

    os_get_system_cpus_uncached();
    os_get_system_pid_max();

    glibc_initialize();
    libuv_initialize();
}

void netdata_conf_section_global_run_as_user(const char **user) {
    // --------------------------------------------------------------------
    // get the user we should run

    // IMPORTANT: this is required before web_files_uid()
    if(getuid() == 0) {
        *user = config_get(CONFIG_SECTION_GLOBAL, "run as user", NETDATA_USER);
    }
    else {
        struct passwd *passwd = getpwuid(getuid());
        *user = config_get(CONFIG_SECTION_GLOBAL, "run as user", (passwd && passwd->pw_name)?passwd->pw_name:"");
    }
}

