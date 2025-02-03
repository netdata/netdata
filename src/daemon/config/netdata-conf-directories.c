// SPDX-License-Identifier: GPL-3.0-or-later

#include "netdata-conf-directories.h"
#include "daemon/common.h"

static const char *get_varlib_subdir_from_config(const char *prefix, const char *dir) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/%s", prefix, dir);
    return inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, dir, filename);
}

void netdata_conf_section_directories(void) {
    static bool run = false;
    if(run) return;
    run = true;

    // ------------------------------------------------------------------------
    // get system paths

    netdata_configured_user_config_dir  = inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "config",       netdata_configured_user_config_dir);
    netdata_configured_stock_config_dir = inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "stock config", netdata_configured_stock_config_dir);
    netdata_configured_log_dir          = inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "log",          netdata_configured_log_dir);
    netdata_configured_web_dir          = inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "web",          netdata_configured_web_dir);
    netdata_configured_cache_dir        = inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "cache",        netdata_configured_cache_dir);
    netdata_configured_varlib_dir       = inicfg_get(&netdata_config, CONFIG_SECTION_DIRECTORIES, "lib",          netdata_configured_varlib_dir);

    netdata_configured_lock_dir = get_varlib_subdir_from_config(netdata_configured_varlib_dir, "lock");
    netdata_configured_cloud_dir = get_varlib_subdir_from_config(netdata_configured_varlib_dir, "cloud.d");

    pluginsd_initialize_plugin_directories();
    netdata_configured_primary_plugins_dir = plugin_directories[PLUGINSD_STOCK_PLUGINS_DIRECTORY_PATH];
}
