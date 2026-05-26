// SPDX-License-Identifier: GPL-3.0-or-later
//
// Process-wide configuration values resolved at netdata startup.
//
// libnetdata owns these globals. They start at compile-time defaults
// (from build/config.h) and are re-resolved against netdata_config by
// nd_runtime_paths_load_directories_from_inicfg() and
// nd_runtime_paths_load_hostname_from_inicfg(), which the daemon
// startup path invokes once after the config is loaded. Callers must
// not assign to these variables directly; use the loaders declared
// below.

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

// Re-resolve the directory globals from the loaded netdata_config.
// Values absent from the config keep their compile-time defaults.
// Idempotent.
void nd_runtime_paths_load_directories_from_inicfg(void);

// Re-resolve host_prefix and hostname from netdata_config.
// host_prefix is loaded first because os_hostname uses it.
// Idempotent.
void nd_runtime_paths_load_hostname_from_inicfg(void);

#endif // NETDATA_RUNTIME_PATHS_H
