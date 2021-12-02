// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

char *netdata_configured_hostname            = NULL;
char *netdata_configured_user_config_dir     = CONFIG_DIR;
char *netdata_configured_stock_config_dir    = LIBCONFIG_DIR;
char *netdata_configured_log_dir             = LOG_DIR;
char *netdata_configured_primary_plugins_dir = NULL;
char *netdata_configured_web_dir             = WEB_DIR;
char *netdata_configured_cache_dir           = CACHE_DIR;
char *netdata_configured_varlib_dir          = VARLIB_DIR;
char *netdata_configured_lock_dir            = NULL;
char *netdata_configured_home_dir            = VARLIB_DIR;
char *netdata_configured_host_prefix         = NULL;
char *netdata_configured_timezone            = NULL;
char *netdata_configured_abbrev_timezone     = NULL;
int32_t netdata_configured_utc_offset        = 0;
int netdata_ready;
int netdata_cloud_setting;

