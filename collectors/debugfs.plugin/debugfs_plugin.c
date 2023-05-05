// SPDX-License-Identifier: GPL-3.0-or-later

#include "collectors/all.h"
#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

static char *user_config_dir = CONFIG_DIR;
static char *stock_config_dir = LIBCONFIG_DIR;

#ifdef HAVE_CAPABILITY
static int debugfs_check_capabilities()
{
    cap_t caps = cap_get_proc();
    if (!caps) {
        error("Cannot get current capabilities.");
        return 0;
    }

    int ret = 1;
    cap_flag_value_t cfv = CAP_CLEAR;
    if (cap_get_flag(caps, CAP_DAC_READ_SEARCH, CAP_EFFECTIVE, &cfv) == -1) {
        error("Cannot find if CAP_DAC_READ_SEARCH is effective.");
        ret = 0;
    } else {
        if (cfv != CAP_SET) {
            error("debugfs.plugin should run with CAP_DAC_READ_SEARCH.");
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
static int debugfs_am_i_running_as_root() {
    uid_t uid = getuid(), euid = geteuid();

    if(uid == 0 || euid == 0) {
        return 1;
    }

    return 0;
}

int main(int argc, char **argv)
{
    // debug_flags = D_PROCFILE;
    stderror = stderr;

    // set the name for logging
    program_name = "debugfs.plugin";

    // disable syslog for debugfs.plugin
    error_log_syslog = 0;

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if (verify_netdata_host_prefix() == -1)
        exit(1);

    user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    if (user_config_dir == NULL) {
        user_config_dir = CONFIG_DIR;
    }

    stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    if (stock_config_dir == NULL) {
        // info("NETDATA_CONFIG_DIR is not passed from netdata");
        stock_config_dir = LIBCONFIG_DIR;
    }

    if(!debugfs_check_capabilities() && !debugfs_am_i_running_as_root()) {
        uid_t uid = getuid(), euid = geteuid();
#ifdef HAVE_CAPABILITY
        error("debugfs.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
              "Without these, debugfs.plugin cannot access /sys/kernel/debug. "
              "To enable capabilities run: sudo setcap cap_dac_read_search,cap_sys_ptrace+ep %s; "
              "To enable setuid to root run: sudo chown root:netdata %s; sudo chmod 4750 %s; "
              , uid, euid, argv[0], argv[0], argv[0]
              );
#else
        error("debugfs.plugin should either run as root (now running with uid %u, euid %u) or have special capabilities. "
              "Without these, debugfs.plugin cannot access /sys/kernel/debug."
              "Your system does not support capabilities. "
              "To enable setuid to root run: sudo chown root:netdata %s; sudo chmod 4750 %s; "
              , uid, euid, argv[0], argv[0]
              );
#endif
    }

    return 0;
}
