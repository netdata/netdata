// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon/common.h"

static struct config stream_config = APPCONFIG_INITIALIZER;

struct _stream_send stream_send = {
    .enabled = false,
    .api_key = NULL,
    .send_charts_matching = NULL,
    .initial_clock_resync_iterations = 60,

    .buffer_max_size = 10 * 1024 * 1024,

    .parents = {
        .destination = NULL,
        .default_port = 19999,
        .h2o = false,
        .timeout_s = 600,
        .reconnect_delay_s = 15,
        .ssl_ca_path = NULL,
        .ssl_ca_file = NULL,
    },

    .compression = {
        .enabled = true,
        .levels = {
            [COMPRESSION_ALGORITHM_NONE]    = 0,
            [COMPRESSION_ALGORITHM_ZSTD]    = 3,    // 1 (faster)  - 22 (smaller)
            [COMPRESSION_ALGORITHM_LZ4]     = 1,    // 1 (smaller) -  9 (faster)
            [COMPRESSION_ALGORITHM_BROTLI]  = 3,    // 0 (faster)  - 11 (smaller)
            [COMPRESSION_ALGORITHM_GZIP]    = 1,    // 1 (faster)  -  9 (smaller)
        }
    },
};

struct _stream_receive stream_receive = {
    .replication = {
        .enabled = true,
        .period = 86400,
        .step = 600,
    }
};

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
    appconfig_move_everywhere(&stream_config, "health enabled by default", "health enabled");
}

bool stream_conf_receiver_needs_dbengine(void) {
    return stream_conf_needs_dbengine(&stream_config);
}

bool stream_conf_init() {
    stream_conf_load();

    stream_send.enabled =
        appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "enabled", stream_send.enabled);

    stream_send.parents.destination =
        string_strdupz(appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "destination", ""));

    stream_send.api_key =
        string_strdupz(appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "api key", ""));

    stream_send.send_charts_matching =
        string_strdupz(appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "send charts matching", "*"));

    stream_receive.replication.enabled =
        config_get_boolean(CONFIG_SECTION_DB, "enable replication",
                           stream_receive.replication.enabled);

    stream_receive.replication.period =
        config_get_duration_seconds(CONFIG_SECTION_DB, "replication period",
                                    stream_receive.replication.period);

    stream_receive.replication.step =
        config_get_duration_seconds(CONFIG_SECTION_DB, "replication step",
                                    stream_receive.replication.step);

    stream_send.buffer_max_size = (size_t)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "buffer size bytes",
        stream_send.buffer_max_size);

    stream_send.parents.reconnect_delay_s = (unsigned int)appconfig_get_duration_seconds(
        &stream_config, CONFIG_SECTION_STREAM, "reconnect delay",
        stream_send.parents.reconnect_delay_s);
    if(stream_send.parents.reconnect_delay_s < SENDER_MIN_RECONNECT_DELAY)
        stream_send.parents.reconnect_delay_s = SENDER_MIN_RECONNECT_DELAY;

    stream_send.compression.enabled =
        appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "enable compression",
                              stream_send.compression.enabled);

    stream_send.compression.levels[COMPRESSION_ALGORITHM_BROTLI] = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "brotli compression level",
        stream_send.compression.levels[COMPRESSION_ALGORITHM_BROTLI]);

    stream_send.compression.levels[COMPRESSION_ALGORITHM_ZSTD] = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "zstd compression level",
        stream_send.compression.levels[COMPRESSION_ALGORITHM_ZSTD]);

    stream_send.compression.levels[COMPRESSION_ALGORITHM_LZ4] = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "lz4 compression acceleration",
        stream_send.compression.levels[COMPRESSION_ALGORITHM_LZ4]);

    stream_send.compression.levels[COMPRESSION_ALGORITHM_GZIP] = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "gzip compression level",
        stream_send.compression.levels[COMPRESSION_ALGORITHM_GZIP]);

    stream_send.parents.h2o = appconfig_get_boolean(
        &stream_config, CONFIG_SECTION_STREAM, "parent using h2o",
        stream_send.parents.h2o);

    stream_send.parents.timeout_s = (int)appconfig_get_duration_seconds(
        &stream_config, CONFIG_SECTION_STREAM, "timeout",
        stream_send.parents.timeout_s);

    stream_send.buffer_max_size = (size_t)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "buffer size bytes",
        stream_send.buffer_max_size);

    stream_send.parents.default_port = (int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "default port",
        stream_send.parents.default_port);

    stream_send.initial_clock_resync_iterations = (unsigned int)appconfig_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "initial clock resync iterations",
        stream_send.initial_clock_resync_iterations); // TODO: REMOVE FOR SLEW / GAPFILLING

    netdata_ssl_validate_certificate_sender = !appconfig_get_boolean(
        &stream_config, CONFIG_SECTION_STREAM, "ssl skip certificate verification",
        !netdata_ssl_validate_certificate);

    if(!netdata_ssl_validate_certificate_sender)
        nd_log_daemon(NDLP_NOTICE, "SSL: streaming senders will skip SSL certificates verification.");

    stream_send.parents.ssl_ca_path = string_strdupz(appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "CApath", NULL));
    stream_send.parents.ssl_ca_file = string_strdupz(appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "CAfile", NULL));

    if(stream_send.enabled && (!stream_send.parents.destination || !stream_send.api_key)) {
        nd_log_daemon(NDLP_ERR, "STREAM [send]: cannot enable sending thread - information is missing.");
        stream_send.enabled = false;
    }

    return stream_send.enabled;
}

bool stream_conf_configured_as_parent() {
    return stream_conf_has_uuid_section(&stream_config);
}

void stream_conf_receiver_config(struct receiver_state *rpt, struct stream_receiver_config *config, const char *api_key, const char *machine_guid) {
    config->mode = rrd_memory_mode_id(
        appconfig_get(&stream_config, machine_guid, "db",
        appconfig_get(&stream_config, api_key, "db",
        rrd_memory_mode_name(default_rrd_memory_mode))));

    if (unlikely(config->mode == RRD_MEMORY_MODE_DBENGINE && !dbengine_enabled)) {
        netdata_log_error("STREAM '%s' [receive from %s:%s]: "
                          "dbengine is not enabled, falling back to default."
                          , rpt->hostname
                          , rpt->client_ip, rpt->client_port
        );
        config->mode = default_rrd_memory_mode;
    }

    config->history = (int)
        appconfig_get_number(&stream_config, machine_guid, "retention",
        appconfig_get_number(&stream_config, api_key, "retention",
        default_rrd_history_entries));
    if(config->history < 5) config->history = 5;

    config->health.enabled =
        appconfig_get_boolean_ondemand(&stream_config, machine_guid, "health enabled",
        appconfig_get_boolean_ondemand(&stream_config, api_key, "health enabled",
        health_plugin_enabled()));

    config->health.delay =
        appconfig_get_duration_seconds(&stream_config, machine_guid, "postpone alerts on connect",
        appconfig_get_duration_seconds(&stream_config, api_key, "postpone alerts on connect",
        60));

    config->update_every = (int)appconfig_get_duration_seconds(&stream_config, machine_guid, "update every", config->update_every);
    if(config->update_every < 0) config->update_every = 1;

    config->health.history =
        appconfig_get_duration_seconds(&stream_config, machine_guid, "health log retention",
        appconfig_get_duration_seconds(&stream_config, api_key, "health log retention",
        HEALTH_LOG_RETENTION_DEFAULT));

    config->send.enabled =
        appconfig_get_boolean(&stream_config, machine_guid, "proxy enabled",
        appconfig_get_boolean(&stream_config, api_key, "proxy enabled",
        stream_send.enabled));

    config->send.parents = string_strdupz(
        appconfig_get(&stream_config, machine_guid, "proxy destination",
        appconfig_get(&stream_config, api_key, "proxy destination",
        string2str(stream_send.parents.destination))));

    config->send.api_key = string_strdupz(
        appconfig_get(&stream_config, machine_guid, "proxy api key",
        appconfig_get(&stream_config, api_key, "proxy api key",
        string2str(stream_send.api_key))));

    config->send.charts_matching = string_strdupz(
        appconfig_get(&stream_config, machine_guid, "proxy send charts matching",
        appconfig_get(&stream_config, api_key, "proxy send charts matching",
        string2str(stream_send.send_charts_matching))));

    config->replication.enabled =
        appconfig_get_boolean(&stream_config, machine_guid, "enable replication",
        appconfig_get_boolean(&stream_config, api_key, "enable replication",
        stream_receive.replication.enabled));

    config->replication.period =
        appconfig_get_duration_seconds(&stream_config, machine_guid, "replication period",
        appconfig_get_duration_seconds(&stream_config, api_key, "replication period",
        stream_receive.replication.period));

    config->replication.step =
        appconfig_get_number(&stream_config, machine_guid, "replication step",
        appconfig_get_number(&stream_config, api_key, "replication step",
        stream_receive.replication.step));

    config->compression.enabled =
        appconfig_get_boolean(&stream_config, machine_guid, "enable compression",
        appconfig_get_boolean(&stream_config, api_key, "enable compression",
        stream_send.compression.enabled));

    if(config->compression.enabled) {
        rrdpush_parse_compression_order(config,
            appconfig_get(&stream_config, machine_guid, "compression algorithms order",
            appconfig_get(&stream_config, api_key, "compression algorithms order",
            RRDPUSH_COMPRESSION_ALGORITHMS_ORDER)));
    }

    config->ephemeral =
        appconfig_get_boolean(&stream_config, machine_guid, "is ephemeral node",
        appconfig_get_boolean(&stream_config, api_key, "is ephemeral node",
        CONFIG_BOOLEAN_NO));
}

bool stream_conf_is_key_type(const char *api_key, const char *type) {
    const char *api_key_type = appconfig_get(&stream_config, api_key, "type", type);
    if(!api_key_type || !*api_key_type) api_key_type = "unknown";
    return strcmp(api_key_type, type) == 0;
}

bool stream_conf_api_key_is_enabled(const char *api_key, bool enabled) {
    return appconfig_get_boolean(&stream_config, api_key, "enabled", enabled);
}

bool stream_conf_api_key_allows_client(const char *api_key, const char *client_ip) {
    SIMPLE_PATTERN *key_allow_from = simple_pattern_create(
        appconfig_get(&stream_config, api_key, "allow from", "*"),
        NULL, SIMPLE_PATTERN_EXACT, true);

    bool rc = true;

    if(key_allow_from) {
        rc = simple_pattern_matches(key_allow_from, client_ip);
        simple_pattern_free(key_allow_from);
    }

    return rc;
}
