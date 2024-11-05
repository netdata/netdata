// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_CONF_H
#define NETDATA_STREAM_CONF_H

#include "libnetdata/libnetdata.h"
#include "daemon/common.h"

extern bool stream_conf_send_enabled;
extern bool stream_conf_compression_enabled;
extern bool stream_conf_replication_enabled;

extern const char *stream_conf_send_destination;
extern const char *stream_conf_send_api_key;
extern const char *stream_conf_send_charts_matching;
extern time_t stream_conf_replication_period;
extern time_t stream_conf_replication_step;
extern unsigned int stream_conf_initial_clock_resync_iterations;

extern struct config stream_config;
extern const char *stream_conf_ssl_ca_path;
extern const char *stream_conf_ssl_ca_file;

bool stream_conf_init();
bool stream_conf_receiver_needs_dbengine();
bool stream_conf_configured_as_parent();

#endif //NETDATA_STREAM_CONF_H
