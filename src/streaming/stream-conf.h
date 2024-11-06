// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STREAM_CONF_H
#define NETDATA_STREAM_CONF_H

#include "libnetdata/libnetdata.h"
#include "stream-compression/compression.h"
#include "stream-capabilities.h"
#include "database/rrd-database-mode.h"

struct _stream_send {
    bool enabled;

    STRING *api_key;
    STRING *send_charts_matching;

    // to have the remote netdata re-sync the charts
    // to its current clock, we send for this many
    // iterations a BEGIN line without microseconds
    // this is for the first iterations of each chart
    uint16_t initial_clock_resync_iterations;

    uint32_t buffer_max_size;

    struct {
        STRING *destination;
        STRING *ssl_ca_path;
        STRING *ssl_ca_file;
        bool h2o;
        uint16_t default_port;
        time_t timeout_s;
        time_t reconnect_delay_s;
    } parents;

    struct {
        bool enabled;
        int levels[COMPRESSION_ALGORITHM_MAX];
    } compression;
};
extern struct _stream_send stream_send;

struct _stream_receive {
    struct {
        bool enabled;
        time_t period;
        time_t step;
    } replication;
};
extern struct _stream_receive stream_receive;

struct stream_receiver_config {
    RRD_MEMORY_MODE mode;
    bool ephemeral;
    int history;
    int update_every;

    struct {
        bool enabled;                   // enable replication on this child
        time_t period;
        time_t step;
    } replication;

    struct {
        int enabled;                    // CONFIG_BOOLEAN_YES, CONFIG_BOOLEAN_NO, CONFIG_BOOLEAN_AUTO
        time_t delay;
        uint32_t history;
    } health;

    struct {
        bool enabled;
        STRING *api_key;
        STRING *parents;
        STRING *charts_matching;
    } send;

    struct {
        bool enabled;
        STREAM_CAPABILITIES priorities[COMPRESSION_ALGORITHM_MAX];
    } compression;
};

void stream_conf_receiver_config(struct receiver_state *rpt, struct stream_receiver_config *config, const char *api_key, const char *machine_guid);

bool stream_conf_init();
bool stream_conf_receiver_needs_dbengine();
bool stream_conf_configured_as_parent();

bool stream_conf_is_key_type(const char *api_key, const char *type);
bool stream_conf_api_key_is_enabled(const char *api_key, bool enabled);
bool stream_conf_api_key_allows_client(const char *api_key, const char *client_ip);

#endif //NETDATA_STREAM_CONF_H
