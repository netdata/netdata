// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf.h"

bool netdata_conf_load(char *filename, char overwrite_used, const char **user) {
    errno_clear();

    int ret = 0;

    if(filename && *filename) {
        ret = config_load(filename, overwrite_used, NULL);
        if(!ret)
            netdata_log_error("CONFIG: cannot load config file '%s'.", filename);
    }
    else {
        filename = filename_from_path_entry_strdupz(netdata_configured_user_config_dir, "netdata.conf");

        ret = config_load(filename, overwrite_used, NULL);
        if(!ret) {
            netdata_log_info("CONFIG: cannot load user config '%s'. Will try the stock version.", filename);
            freez(filename);

            filename = filename_from_path_entry_strdupz(netdata_configured_stock_config_dir, "netdata.conf");
            ret = config_load(filename, overwrite_used, NULL);
            if(!ret)
                netdata_log_info("CONFIG: cannot load stock config '%s'. Running with internal defaults.", filename);
        }

        freez(filename);
    }

    netdata_conf_section_global_run_as_user(user);
    return ret;
}
