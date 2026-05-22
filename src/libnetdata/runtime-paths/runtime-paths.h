// SPDX-License-Identifier: GPL-3.0-or-later
//
// Process-wide configuration values resolved at netdata startup.
// The daemon's config layer
// (src/daemon/config/netdata-conf-directories.c and friends) sets
// these once at startup by writing through the extern declarations.

#ifndef NETDATA_RUNTIME_PATHS_H
#define NETDATA_RUNTIME_PATHS_H

extern const char *netdata_configured_hostname;
extern const char *netdata_configured_user_config_dir;
extern const char *netdata_configured_stock_config_dir;
extern const char *netdata_configured_stock_data_dir;
extern const char *netdata_configured_log_dir;
extern const char *netdata_configured_primary_plugins_dir;
extern const char *netdata_configured_web_dir;
extern const char *netdata_configured_cache_dir;
extern const char *netdata_configured_varlib_dir;
extern const char *netdata_configured_cloud_dir;
extern const char *netdata_configured_home_dir;
extern const char *netdata_configured_host_prefix;

#endif // NETDATA_RUNTIME_PATHS_H
