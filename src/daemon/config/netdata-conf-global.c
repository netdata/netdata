// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-global.h"

static int get_hostname(char *buf, size_t buf_size) {
    if (netdata_configured_host_prefix && *netdata_configured_host_prefix) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/etc/hostname", netdata_configured_host_prefix);

        if (!read_txt_file(filename, buf, buf_size)) {
            trim(buf);
            return 0;
        }
    }

    return gethostname(buf, buf_size);
}

void netdata_conf_section_global(void) {
    netdata_conf_backwards_compatibility();

    // ------------------------------------------------------------------------
    // get the hostname

    netdata_configured_host_prefix = config_get(CONFIG_SECTION_GLOBAL, "host access prefix", "");
    (void) verify_netdata_host_prefix(true);

    char buf[HOSTNAME_MAX + 1];
    if (get_hostname(buf, HOSTNAME_MAX))
        netdata_log_error("Cannot get machine hostname.");

    netdata_configured_hostname = config_get(CONFIG_SECTION_GLOBAL, "hostname", buf);
    netdata_log_debug(D_OPTIONS, "hostname set to '%s'", netdata_configured_hostname);

    netdata_conf_section_directories();
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
        *user = config_get(CONFIG_SECTION_GLOBAL, "run as user", NETDATA_USER);
    }
    else {
        struct passwd *passwd = getpwuid(getuid());
        *user = config_get(CONFIG_SECTION_GLOBAL, "run as user", (passwd && passwd->pw_name)?passwd->pw_name:"");
    }
}
