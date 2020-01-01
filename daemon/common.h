// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMMON_H
#define NETDATA_COMMON_H 1

#include "../libnetdata/libnetdata.h"

// ----------------------------------------------------------------------------
// shortcuts for the default netdata configuration

#define config_load(filename, overwrite_used, section) appconfig_load(&netdata_config, filename, overwrite_used, section)
#define config_get(section, name, default_value) appconfig_get(&netdata_config, section, name, default_value)
#define config_get_number(section, name, value) appconfig_get_number(&netdata_config, section, name, value)
#define config_get_float(section, name, value) appconfig_get_float(&netdata_config, section, name, value)
#define config_get_boolean(section, name, value) appconfig_get_boolean(&netdata_config, section, name, value)
#define config_get_boolean_ondemand(section, name, value) appconfig_get_boolean_ondemand(&netdata_config, section, name, value)
#define config_get_duration(section, name, value) appconfig_get_duration(&netdata_config, section, name, value)

#define config_set(section, name, default_value) appconfig_set(&netdata_config, section, name, default_value)
#define config_set_default(section, name, value) appconfig_set_default(&netdata_config, section, name, value)
#define config_set_number(section, name, value) appconfig_set_number(&netdata_config, section, name, value)
#define config_set_float(section, name, value) appconfig_set_float(&netdata_config, section, name, value)
#define config_set_boolean(section, name, value) appconfig_set_boolean(&netdata_config, section, name, value)

#define config_exists(section, name) appconfig_exists(&netdata_config, section, name)
#define config_move(section_old, name_old, section_new, name_new) appconfig_move(&netdata_config, section_old, name_old, section_new, name_new)

#define config_generate(buffer, only_changed) appconfig_generate(&netdata_config, buffer, only_changed)

// ----------------------------------------------------------------------------
// netdata include files

#include "global_statistics.h"

// the netdata database
#include "database/rrd.h"

// the netdata webserver(s)
#include "web/server/web_server.h"

// streaming metrics between netdata servers
#include "streaming/rrdpush.h"

// health monitoring and alarm notifications
#include "health/health.h"

// the netdata registry
// the registry is actually an API feature
#include "registry/registry.h"

// backends for archiving the metrics
#include "backends/backends.h"

// the netdata API
#include "web/api/web_api_v1.h"

// all data collection plugins
#include "collectors/all.h"

// netdata unit tests
#include "unit_test.h"

// netdata agent claiming
#include "claim/claim.h"

// the netdata deamon
#include "daemon.h"
#include "main.h"
#include "signals.h"
#include "commands.h"

// global netdata daemon variables
extern char *netdata_configured_hostname;
extern char *netdata_configured_user_config_dir;
extern char *netdata_configured_stock_config_dir;
extern char *netdata_configured_log_dir;
extern char *netdata_configured_primary_plugins_dir;
extern char *netdata_configured_web_dir;
extern char *netdata_configured_cache_dir;
extern char *netdata_configured_varlib_dir;
extern char *netdata_configured_home_dir;
extern char *netdata_configured_host_prefix;
extern char *netdata_configured_timezone;
extern int netdata_zero_metrics_enabled;
extern int netdata_anonymous_statistics_enabled;

int netdata_ready;

#endif /* NETDATA_COMMON_H */
