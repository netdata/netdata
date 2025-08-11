// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMMON_H
#define NETDATA_COMMON_H 1

#ifdef __cplusplus
extern "C" {
#endif

#include "libnetdata/libnetdata.h"
#include "config/netdata-conf.h"
#include "libuv_workers.h"

// ----------------------------------------------------------------------------
// netdata include files

#include "web/api/maps/maps.h"

#include "daemon/config/netdata-conf.h"
#include "daemon/dyncfg/dyncfg.h"

#include "daemon/pulse/pulse.h"

// health monitoring and alarm notifications
#include "health/health.h"

// the netdata database
#include "database/rrd.h"

// the netdata webserver(s)
#include "web/server/web_server.h"

// streaming metrics between netdata servers
#include "streaming/stream.h"

// anomaly detection
#include "ml/ml_public.h"

// the netdata registry
// the registry is actually an API feature
#include "registry/registry.h"

// exporting engine for archiving the metrics
#include "exporting/exporting_engine.h"

// the netdata API
#include "web/server/web_client.h"
#include "web/rtc/webrtc.h"

// all data collection plugins
#include "collectors/all.h"

// netdata unit tests
#include "unit_test.h"

// netdata agent claiming
#include "claim/claim.h"

// netdata agent cloud link
#include "aclk/aclk.h"

// global GUID map functions

// the netdata daemon
#include "daemon.h"
#include "main.h"
#include "static_threads.h"
#include "signal-handler.h"
#include "commands.h"
#include "pipename.h"
#include "analytics.h"

// global netdata daemon variables
extern const char *netdata_configured_hostname;
extern const char *netdata_configured_user_config_dir;
extern const char *netdata_configured_stock_config_dir;
extern const char *netdata_configured_log_dir;
extern const char *netdata_configured_primary_plugins_dir;
extern const char *netdata_configured_web_dir;
extern const char *netdata_configured_cache_dir;
extern const char *netdata_configured_varlib_dir;
extern const char *netdata_configured_cloud_dir;
extern const char *netdata_configured_home_dir;
extern const char *netdata_configured_host_prefix;
extern const char *netdata_configured_timezone;
extern const char *netdata_configured_abbrev_timezone;
extern int32_t netdata_configured_utc_offset;
extern bool netdata_anonymous_statistics_enabled;

extern bool netdata_ready;
extern time_t netdata_start_time;

void set_environment_for_plugins_and_scripts(void);

#ifdef __cplusplus
}
#endif

#endif /* NETDATA_COMMON_H */
