// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMMON_H
#define NETDATA_COMMON_H 1

#include "../libnetdata/libnetdata.h"

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

// the netdata deamon
#include "daemon.h"
#include "main.h"
#include "signals.h"

// global netdata daemon variables
extern char *netdata_configured_hostname;
extern char *netdata_configured_user_config_dir;
extern char *netdata_configured_stock_config_dir;
extern char *netdata_configured_log_dir;
extern char *netdata_configured_plugins_dir_base;
extern char *netdata_configured_plugins_dir;
extern char *netdata_configured_web_dir;
extern char *netdata_configured_cache_dir;
extern char *netdata_configured_varlib_dir;
extern char *netdata_configured_home_dir;
extern char *netdata_configured_host_prefix;
extern char *netdata_configured_timezone;

#endif /* NETDATA_COMMON_H */
