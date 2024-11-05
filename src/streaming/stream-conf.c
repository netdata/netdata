// SPDX-License-Identifier: GPL-3.0-or-later

#include "stream-conf.h"

struct config stream_config = APPCONFIG_INITIALIZER;

bool stream_conf_send_enabled = false;
bool stream_conf_compression_enabled = true;
bool stream_conf_replication_enabled = true;

const char *stream_conf_send_destination = NULL;
const char *stream_conf_send_api_key = NULL;
const char *stream_conf_send_charts_matching = "*";

time_t stream_conf_replication_period = 86400;
time_t stream_conf_replication_step = 600;

const char *stream_conf_ssl_ca_path = NULL;
const char *stream_conf_ssl_ca_file = NULL;

// to have the remote netdata re-sync the charts
// to its current clock, we send for this many
// iterations a BEGIN line without microseconds
// this is for the first iterations of each chart
unsigned int stream_conf_initial_clock_resync_iterations = 60;

static void stream_conf_load() {
    errno_clear();
    char *filename = filename_from_path_entry_strdupz(netdata_configured_user_config_dir, "stream.conf");
    if(!appconfig_load(&stream_config, filename, 0, NULL)) {
        nd_log_daemon(NDLP_NOTICE, "CONFIG: cannot load user config '%s'. Will try stock config.", filename);
        freez(filename);

        filename = filename_from_path_entry_strdupz(netdata_configured_stock_config_dir, "stream.conf");
        if(!appconfig_load(&stream_config, filename, 0, NULL))
            nd_log_daemon(NDLP_NOTICE, "CONFIG: cannot load stock config '%s'. Running with internal defaults.", filename);
    }

    freez(filename);

    appconfig_move(&stream_config,
                   CONFIG_SECTION_STREAM, "timeout seconds",
                   CONFIG_SECTION_STREAM, "timeout");

    appconfig_move(&stream_config,
                   CONFIG_SECTION_STREAM, "reconnect delay seconds",
                   CONFIG_SECTION_STREAM, "reconnect delay");

    appconfig_move_everywhere(&stream_config, "default memory mode", "db");
    appconfig_move_everywhere(&stream_config, "memory mode", "db");
    appconfig_move_everywhere(&stream_config, "db mode", "db");
    appconfig_move_everywhere(&stream_config, "default history", "retention");
    appconfig_move_everywhere(&stream_config, "history", "retention");
    appconfig_move_everywhere(&stream_config, "default proxy enabled", "proxy enabled");
    appconfig_move_everywhere(&stream_config, "default proxy destination", "proxy destination");
    appconfig_move_everywhere(&stream_config, "default proxy api key", "proxy api key");
    appconfig_move_everywhere(&stream_config, "default proxy send charts matching", "proxy send charts matching");
    appconfig_move_everywhere(&stream_config, "default health log history", "health log retention");
    appconfig_move_everywhere(&stream_config, "health log history", "health log retention");
    appconfig_move_everywhere(&stream_config, "seconds to replicate", "replication period");
    appconfig_move_everywhere(&stream_config, "seconds per replication step", "replication step");
    appconfig_move_everywhere(&stream_config, "default postpone alarms on connect seconds", "postpone alerts on connect");
    appconfig_move_everywhere(&stream_config, "postpone alarms on connect seconds", "postpone alerts on connect");
}

bool stream_conf_receiver_needs_dbengine(void) {
    return stream_conf_needs_dbengine(&stream_config);
}

bool stream_conf_init() {
    // --------------------------------------------------------------------
    // load stream.conf
    stream_conf_load();

    stream_conf_send_enabled =
        appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "enabled", stream_conf_send_enabled);

    stream_conf_send_destination =
        appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "destination", "");

    stream_conf_send_api_key =
        appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "api key", "");

    stream_conf_send_charts_matching =
        appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "send charts matching", stream_conf_send_charts_matching);

    stream_conf_replication_enabled =
        config_get_boolean(CONFIG_SECTION_DB, "enable replication", stream_conf_replication_enabled);

    stream_conf_replication_period =
        config_get_duration_seconds(CONFIG_SECTION_DB, "replication period", stream_conf_replication_period);

    stream_conf_replication_step =
        config_get_duration_seconds(CONFIG_SECTION_DB, "replication step", stream_conf_replication_step);

    rrdhost_free_orphan_time_s =
        config_get_duration_seconds(CONFIG_SECTION_DB, "cleanup orphan hosts after", rrdhost_free_orphan_time_s);

    stream_conf_compression_enabled =
        appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM,
                              "enable compression", stream_conf_compression_enabled);

    rrdpush_compression_levels[COMPRESSION_ALGORITHM_BROTLI] = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "brotli compression level",
        rrdpush_compression_levels[COMPRESSION_ALGORITHM_BROTLI]);

    rrdpush_compression_levels[COMPRESSION_ALGORITHM_ZSTD] = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "zstd compression level",
        rrdpush_compression_levels[COMPRESSION_ALGORITHM_ZSTD]);

    rrdpush_compression_levels[COMPRESSION_ALGORITHM_LZ4] = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "lz4 compression acceleration",
        rrdpush_compression_levels[COMPRESSION_ALGORITHM_LZ4]);

    rrdpush_compression_levels[COMPRESSION_ALGORITHM_GZIP] = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "gzip compression level",
        rrdpush_compression_levels[COMPRESSION_ALGORITHM_GZIP]);

    if(stream_conf_send_enabled && (!stream_conf_send_destination || !*stream_conf_send_destination || !stream_conf_send_api_key || !*stream_conf_send_api_key)) {
        nd_log_daemon(NDLP_WARNING, "STREAM [send]: cannot enable sending thread - information is missing.");
        stream_conf_send_enabled = false;
    }

    netdata_ssl_validate_certificate_sender = !appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "ssl skip certificate verification", !netdata_ssl_validate_certificate);

    if(!netdata_ssl_validate_certificate_sender)
        nd_log_daemon(NDLP_NOTICE, "SSL: streaming senders will skip SSL certificates verification.");

    stream_conf_ssl_ca_path = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "CApath", NULL);
    stream_conf_ssl_ca_file = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "CAfile", NULL);

    return stream_conf_send_enabled;
}

bool stream_conf_configured_as_parent() {
    return stream_conf_has_uuid_section(&stream_config);
}
