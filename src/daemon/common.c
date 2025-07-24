// SPDX-License-Identifier: GPL-3.0-or-later

#include "common.h"

const char *netdata_configured_hostname            = NULL;
const char *netdata_configured_user_config_dir     = CONFIG_DIR;
const char *netdata_configured_stock_config_dir    = LIBCONFIG_DIR;
const char *netdata_configured_log_dir             = LOG_DIR;
const char *netdata_configured_primary_plugins_dir = PLUGINS_DIR;
const char *netdata_configured_web_dir             = WEB_DIR;
const char *netdata_configured_cache_dir           = CACHE_DIR;
const char *netdata_configured_varlib_dir          = VARLIB_DIR;
const char *netdata_configured_cloud_dir           = VARLIB_DIR "/cloud.d";
const char *netdata_configured_home_dir            = VARLIB_DIR;
const char *netdata_configured_host_prefix         = NULL;
const char *netdata_configured_timezone            = NULL;
const char *netdata_configured_abbrev_timezone     = NULL;
int32_t netdata_configured_utc_offset              = 0;

bool netdata_ready = false;
