// SPDX-License-Identifier: GPL-3.0-or-later

#include "debugfs_plugin.h"
#include "libnetdata/required_dummies.h"

static char *user_config_dir = CONFIG_DIR;
static char *stock_config_dir = LIBCONFIG_DIR;

static int update_every = 1;

netdata_mutex_t stdout_mutex;

static void __attribute__((constructor)) init_mutex(void) {
    netdata_mutex_init(&stdout_mutex);
}

static void __attribute__((destructor)) destroy_mutex(void) {
    netdata_mutex_destroy(&stdout_mutex);
}

static struct debugfs_module {
    const char *name;
    int enabled;
    int (*func)(int update_every, const char *name);
}  debugfs_modules[] = {
    {
        // Memory Fragmentation
        .name = "/sys/kernel/debug/extfrag",
        .enabled = CONFIG_BOOLEAN_YES,
        .func = do_module_numa_extfrag
    },
    {
        .name = "/sys/kernel/debug/zswap",
        .enabled = CONFIG_BOOLEAN_YES,
        .func = do_module_zswap
    },
    {
        // Linux powercap metrics is here because it needs privilege to read each RAPL zone
        .name = "/sys/devices/virtual/powercap",
        .enabled = CONFIG_BOOLEAN_YES,
        .func = do_module_devices_powercap
    },
    {
     .name = "libsensors",
     .enabled = CONFIG_BOOLEAN_YES,
     .func = do_module_libsensors
    },

    // The terminator
    {.name = NULL, .enabled = CONFIG_BOOLEAN_NO, .func = NULL}
};

#ifdef HAVE_CAPABILITY
static int debugfs_check_capabilities()
{
    cap_t caps = cap_get_proc();
    if (!caps) {
        netdata_log_error("Cannot get current capabilities.");
        return 0;
    }

    int ret = 1;
    cap_flag_value_t cfv = CAP_CLEAR;
    if (cap_get_flag(caps, CAP_DAC_READ_SEARCH, CAP_EFFECTIVE, &cfv) == -1) {
        netdata_log_error("Cannot find if CAP_DAC_READ_SEARCH is effective.");
        ret = 0;
    } else {
        if (cfv != CAP_SET) {
            netdata_log_error("debugfs.plugin should run with CAP_DAC_READ_SEARCH.");
            ret = 0;
        }
    }
    cap_free(caps);

    return ret;
}
#else
static int debugfs_check_capabilities()
{
    return 0;
}
#endif

// TODO: This is a function used by 3 different collector, we should do it global (next PR)
static int debugfs_am_i_running_as_root()
{
    uid_t uid = getuid(), euid = geteuid();

    if (uid == 0 || euid == 0) {
        return 1;
    }

    return 0;
}

void debugfs2lower(char *name)
{
    while (*name) {
        *name = tolower(*name);
        name++;
    }
}

// Consiidering our goal to redce binaries, I preferred to copy function, instead to force link with unecessary libs
const char *debugfs_rrdset_type_name(RRDSET_TYPE chart_type) {
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

const char *debugfs_rrd_algorithm_name(RRD_ALGORITHM algorithm) {
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

int debugfs_check_sys_permission() {
    int ret = 0;

    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s/sys/kernel/debug/extfrag/extfrag_index", netdata_configured_host_prefix);

    procfile *ff = procfile_open(filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(!ff) goto dcsp_cleanup;

    ff = procfile_readall(ff);
    if(!ff) goto dcsp_cleanup;

    ret = 1;

dcsp_cleanup:
    if (!ret)
        perror("Cannot open /sys/kernel/debug/extfrag/extfrag_index file");
    procfile_close(ff);
    return ret;
}

static void debugfs_parse_args(int argc, char **argv)
{
    int i, freq = 0;
    for(i = 1; i < argc; i++) {
        if(!freq) {
            int n = (int)str2l(argv[i]);
            if(n > 0) {
                freq = n;
                continue;
            }
        }

        if(strcmp("test-permissions", argv[i]) == 0 || strcmp("-t", argv[i]) == 0) {
            if(!debugfs_check_sys_permission()) {
                exit(2);
            }
            printf("OK\n");
            exit(0);
        }
    }

    if(freq > 0) update_every = freq;
}

int main(int argc, char **argv)
{
    nd_log_initialize_for_external_plugins("debugfs.plugin");
    netdata_threads_init_for_external_plugins(0);

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if (verify_netdata_host_prefix(true) == -1)
        exit(1);

    user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    if (user_config_dir == NULL) {
        user_config_dir = CONFIG_DIR;
    }

    stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    if (stock_config_dir == NULL) {
        // netdata_log_info("NETDATA_CONFIG_DIR is not passed from netdata");
        stock_config_dir = LIBCONFIG_DIR;
    }

    // FIXME: should first check if /sys/kernel/debug is mounted

    // FIXME: remove debugfs_check_sys_permission() after https://github.com/netdata/netdata/issues/15048 is fixed
    if (!debugfs_check_capabilities() && !debugfs_am_i_running_as_root() && !debugfs_check_sys_permission()) {
        uid_t uid = getuid(), euid = geteuid();
#ifdef HAVE_CAPABILITY
        netdata_log_error(
            "debugfs.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
            "Without these, debugfs.plugin cannot access /sys/kernel/debug. "
            "To enable capabilities run: sudo setcap cap_dac_read_search,cap_sys_ptrace+ep %s; "
            "To enable setuid to root run: sudo chown root:netdata %s; sudo chmod 4750 %s; ",
            uid,
            euid,
            argv[0],
            argv[0],
            argv[0]);
#else
        netdata_log_error(
            "debugfs.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
            "Without these, debugfs.plugin cannot access /sys/kernel/debug."
            "Your system does not support capabilities. "
            "To enable setuid to root run: sudo chown root:netdata %s; sudo chmod 4750 %s; ",
            uid,
            euid,
            argv[0],
            argv[0]);
#endif
        exit(1);
    }

    debugfs_parse_args(argc, argv);

    size_t iteration;
    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);

    for (iteration = 0; iteration < 86400; iteration++) {
        heartbeat_next(&hb);
        int enabled = 0;

        for (int i = 0; debugfs_modules[i].name; i++) {
            struct debugfs_module *pm = &debugfs_modules[i];
            if (unlikely(!pm->enabled))
                continue;

            pm->enabled = !pm->func(update_every, pm->name);
            if (likely(pm->enabled))
                enabled++;
        }

        if (!enabled) {
            netdata_log_info("all modules are disabled, exiting...");
            return 1;
        }

        netdata_mutex_lock(&stdout_mutex);
        fprintf(stdout, "\n");
        fflush(stdout);
        netdata_mutex_unlock(&stdout_mutex);

        if (ferror(stdout) && errno == EPIPE) {
            netdata_log_error("error writing to stdout: EPIPE. Exiting...");
            return 1;
        }
    }

    module_libsensors_cleanup();

    netdata_mutex_lock(&stdout_mutex);
    fprintf(stdout, "EXIT\n");
    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);
    return 0;
}
