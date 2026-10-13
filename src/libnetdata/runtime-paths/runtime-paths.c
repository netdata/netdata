// SPDX-License-Identifier: GPL-3.0-or-later
//
// Storage and netdata.conf loaders for the netdata_configured_*
// globals. Compile-time defaults come from build/config.h via the
// libnetdata.h chain.

#include "../libnetdata.h"

const char *netdata_configured_hostname            = NULL;
const char *netdata_configured_user_config_dir     = CONFIG_DIR;
const char *netdata_configured_stock_config_dir    = LIBCONFIG_DIR;
const char *netdata_configured_stock_data_dir      = STOCK_DATA_DIR;
const char *netdata_configured_log_dir             = LOG_DIR;
const char *netdata_configured_primary_plugins_dir = PLUGINS_DIR;
const char *netdata_configured_web_dir             = WEB_DIR;
const char *netdata_configured_cache_dir           = CACHE_DIR;
const char *netdata_configured_varlib_dir          = VARLIB_DIR;
const char *netdata_configured_cloud_dir           = VARLIB_DIR "/cloud.d";
const char *netdata_configured_home_dir            = VARLIB_DIR;
const char *netdata_configured_host_prefix         = NULL;

// ----------------------------------------------------------------------------
// netdata.conf loaders

static const char *get_varlib_subdir_from_config(const char *prefix, const char *dir) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/%s", prefix, dir);
    return inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, dir, filename);
}

void nd_runtime_paths_load_directories_from_inicfg(void) {
    FUNCTION_RUN_ONCE();

    netdata_configured_user_config_dir  = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "config",       netdata_configured_user_config_dir);
    netdata_configured_stock_config_dir = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "stock config", netdata_configured_stock_config_dir);
    netdata_configured_stock_data_dir   = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "stock data",   netdata_configured_stock_data_dir);
    netdata_configured_log_dir          = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "log",          netdata_configured_log_dir);
    netdata_configured_web_dir          = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "web",          netdata_configured_web_dir);
    netdata_configured_cache_dir        = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "cache",        netdata_configured_cache_dir);
    netdata_configured_varlib_dir       = inicfg_get_path(&netdata_config, CONFIG_SECTION_DIRECTORIES, "lib",          netdata_configured_varlib_dir);
    netdata_configured_cloud_dir        = get_varlib_subdir_from_config(netdata_configured_varlib_dir, "cloud.d");
}

void nd_runtime_paths_load_hostname_from_inicfg(void) {
    FUNCTION_RUN_ONCE();

    netdata_configured_host_prefix = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "host access prefix", "");
    (void) verify_netdata_host_prefix(true);

    char buf[HOST_NAME_MAX * 4 + 1] = "";
    if (!os_hostname(buf, sizeof(buf), netdata_configured_host_prefix))
        netdata_log_error("Cannot get machine hostname.");

    netdata_configured_hostname = inicfg_get(&netdata_config, CONFIG_SECTION_GLOBAL, "hostname", buf);
    netdata_log_debug(D_OPTIONS, "hostname set to '%s'", netdata_configured_hostname);
}
