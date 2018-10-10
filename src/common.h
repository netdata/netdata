// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_COMMON_H
#define NETDATA_COMMON_H 1

#include "libnetdata/libnetdata.h"

// ----------------------------------------------------------------------------
// netdata include files

#include "global_statistics.h"
#include "web_buffer_svg.h"
#include "url.h"

#include "rrd.h"
#include "rrd2json.h"
#include "web_client.h"
#include "web_server.h"
#include "daemon.h"
#include "main.h"
#include "unit_test.h"
#include "backends.h"
#include "backend_prometheus.h"
#include "rrdpush.h"
#include "web_api_v1.h"
#include "signals.h"

#include "health/health.h"
#include "registry/registry.h"
#include "plugins/all.h"

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
