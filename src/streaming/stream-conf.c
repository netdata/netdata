// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"
#include "stream-receiver-internals.h"
#include "stream-sender-internals.h"
#include "stream-replication-sender.h"

static struct config stream_config = APPCONFIG_INITIALIZER;

/**
 * Free stream configuration
 * 
 * Free all memory associated with the stream configuration.
 * Called during shutdown to prevent memory leaks.
 */
void stream_config_free(void) {
    // Free the configuration
    inicfg_free(&stream_config);
    
    // Free the allocated strings in stream_send structure
    string_freez(stream_send.api_key);
    string_freez(stream_send.send_charts_matching);
    string_freez(stream_send.parents.destination);
    string_freez(stream_send.parents.ssl_ca_path);
    string_freez(stream_send.parents.ssl_ca_file);
    
    // Reset the pointers to NULL
    stream_send.api_key = NULL;
    stream_send.send_charts_matching = NULL;
    stream_send.parents.destination = NULL;
    stream_send.parents.ssl_ca_path = NULL;
    stream_send.parents.ssl_ca_file = NULL;
}

struct _stream_send stream_send = {
    .enabled = false,
    .api_key = NULL,
    .send_charts_matching = NULL,
    .initial_clock_resync_iterations = 60,

    .buffer_max_size = CBUFFER_INITIAL_MAX_SIZE,

    .replication = {
        .prefetch = 0,
        .threads = 0,
    },

    .parents = {
        .destination = NULL,
        .default_port = 19999,
        .h2o = false,
        .timeout_s = 300,
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
            [COMPRESSION_ALGORITHM_GZIP]    = 3,    // 1 (faster)  -  9 (smaller)
        }
    },
};

struct _stream_receive stream_receive = {
    .replication = {
        .enabled = true,
        .period = 86400,
        .step = 3600,
    }
};

static const struct {
    compression_algorithm_t algorithm;
    const char *setting;
} sender_compression_level_settings[] = {
    { COMPRESSION_ALGORITHM_BROTLI, "brotli compression level" },
    { COMPRESSION_ALGORITHM_ZSTD,   "zstd compression level" },
    { COMPRESSION_ALGORITHM_LZ4,    "lz4 compression acceleration" },
    { COMPRESSION_ALGORITHM_GZIP,   "gzip compression level" },
};

typedef enum {
    STREAM_RECEIVER_KEEPALIVE_SOURCE_DEFAULT,
    STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY,
    STREAM_RECEIVER_KEEPALIVE_SOURCE_MACHINE_GUID,
} STREAM_RECEIVER_KEEPALIVE_SOURCE;

typedef struct {
    bool automatic;
    bool valid;
    bool bounded;
    int configured_s;
    uint32_t idle_s;
    STREAM_RECEIVER_KEEPALIVE_SOURCE source;
    const char *raw_value;
} STREAM_RECEIVER_KEEPALIVE_CONFIG;

static STREAM_RECEIVER_KEEPALIVE_CONFIG stream_conf_parse_receiver_keepalive(
    STREAM_RECEIVER_KEEPALIVE_SOURCE source,
    const char *value) {

    STREAM_RECEIVER_KEEPALIVE_CONFIG parsed = {
        .automatic = true,
        .valid = true,
        .idle_s = STREAM_RECEIVER_KEEPALIVE_IDLE_MIN_SECONDS,
        .source = source,
        .raw_value = value ? value : "auto",
    };

    if(!value || !strcasecmp(value, "auto"))
        return parsed;

    int configured_s;
    if(!duration_parse_seconds(value, &configured_s) || configured_s < 0) {
        parsed.valid = false;
        return parsed;
    }

    parsed.automatic = false;
    parsed.configured_s = configured_s;
    parsed.idle_s = (uint32_t)MIN(
        MAX(configured_s, (int)STREAM_RECEIVER_KEEPALIVE_IDLE_MIN_SECONDS),
        (int)STREAM_RECEIVER_KEEPALIVE_IDLE_MAX_SECONDS);
    parsed.bounded = parsed.idle_s != (uint32_t)configured_s;
    return parsed;
}

static STREAM_RECEIVER_KEEPALIVE_CONFIG stream_conf_resolve_receiver_keepalive(
    struct config *config,
    const char *api_key,
    const char *machine_guid) {

    const char *section = NULL;
    STREAM_RECEIVER_KEEPALIVE_SOURCE source = STREAM_RECEIVER_KEEPALIVE_SOURCE_DEFAULT;
    if(inicfg_exists(config, machine_guid, "tcp keepalive idle")) {
        section = machine_guid;
        source = STREAM_RECEIVER_KEEPALIVE_SOURCE_MACHINE_GUID;
    }
    else if(inicfg_exists(config, api_key, "tcp keepalive idle")) {
        section = api_key;
        source = STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY;
    }

    const char *value = section ? inicfg_get(config, section, "tcp keepalive idle", "auto") : "auto";
    return stream_conf_parse_receiver_keepalive(source, value);
}

static const char *stream_conf_receiver_keepalive_source_name(STREAM_RECEIVER_KEEPALIVE_SOURCE source) {
    switch(source) {
        case STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY: return "API key section";
        case STREAM_RECEIVER_KEEPALIVE_SOURCE_MACHINE_GUID: return "machine GUID section";
        case STREAM_RECEIVER_KEEPALIVE_SOURCE_DEFAULT: return "default";
    }
    return "default";
}

static void stream_conf_resolve_sender_compression_levels(
    struct config *config,
    ND_COMPRESSION_PROFILE profile,
    int levels[COMPRESSION_ALGORITHM_MAX]) {

    switch(profile) {
        default:
        case ND_COMPRESSION_DEFAULT:
            levels[COMPRESSION_ALGORITHM_ZSTD]      = 3;
            levels[COMPRESSION_ALGORITHM_LZ4]       = 1;
            levels[COMPRESSION_ALGORITHM_BROTLI]    = 3;
            levels[COMPRESSION_ALGORITHM_GZIP]      = 3;
            break;

        case ND_COMPRESSION_FASTEST:
            levels[COMPRESSION_ALGORITHM_ZSTD]      = 1;
            levels[COMPRESSION_ALGORITHM_LZ4]       = 9;
            levels[COMPRESSION_ALGORITHM_BROTLI]    = 1;
            levels[COMPRESSION_ALGORITHM_GZIP]      = 1;
            break;
    }

    for(size_t i = 0; i < _countof(sender_compression_level_settings); i++) {
        compression_algorithm_t algorithm = sender_compression_level_settings[i].algorithm;
        levels[algorithm] = (int)inicfg_get_number(
            config, CONFIG_SECTION_STREAM, sender_compression_level_settings[i].setting, levels[algorithm]);
    }
}

void stream_conf_configure_sender_compression_levels(ND_COMPRESSION_PROFILE profile) {
    stream_conf_resolve_sender_compression_levels(&stream_config, profile, stream_send.compression.levels);
}

int stream_conf_compression_levels_unittest(void) {
    static const struct {
        const char *name;
        ND_COMPRESSION_PROFILE profile;
        const char *configured[COMPRESSION_ALGORITHM_MAX];
        int expected[COMPRESSION_ALGORITHM_MAX];
    } tests[] = {
        {
            .name = "balanced defaults",
            .profile = ND_COMPRESSION_DEFAULT,
            .expected = {
                [COMPRESSION_ALGORITHM_ZSTD] = 3,
                [COMPRESSION_ALGORITHM_LZ4] = 1,
                [COMPRESSION_ALGORITHM_GZIP] = 3,
                [COMPRESSION_ALGORITHM_BROTLI] = 3,
            },
        },
        {
            .name = "fastest defaults",
            .profile = ND_COMPRESSION_FASTEST,
            .expected = {
                [COMPRESSION_ALGORITHM_ZSTD] = 1,
                [COMPRESSION_ALGORITHM_LZ4] = 9,
                [COMPRESSION_ALGORITHM_GZIP] = 1,
                [COMPRESSION_ALGORITHM_BROTLI] = 1,
            },
        },
        {
            .name = "partial zstd override",
            .profile = ND_COMPRESSION_FASTEST,
            .configured = {
                [COMPRESSION_ALGORITHM_ZSTD] = "19",
            },
            .expected = {
                [COMPRESSION_ALGORITHM_ZSTD] = 19,
                [COMPRESSION_ALGORITHM_LZ4] = 9,
                [COMPRESSION_ALGORITHM_GZIP] = 1,
                [COMPRESSION_ALGORITHM_BROTLI] = 1,
            },
        },
        {
            .name = "all explicit overrides",
            .profile = ND_COMPRESSION_DEFAULT,
            .configured = {
                [COMPRESSION_ALGORITHM_ZSTD] = "19",
                [COMPRESSION_ALGORITHM_LZ4] = "7",
                [COMPRESSION_ALGORITHM_GZIP] = "8",
                [COMPRESSION_ALGORITHM_BROTLI] = "10",
            },
            .expected = {
                [COMPRESSION_ALGORITHM_ZSTD] = 19,
                [COMPRESSION_ALGORITHM_LZ4] = 7,
                [COMPRESSION_ALGORITHM_GZIP] = 8,
                [COMPRESSION_ALGORITHM_BROTLI] = 10,
            },
        },
        {
            .name = "unknown profile fallback",
            .profile = (ND_COMPRESSION_PROFILE)255,
            .expected = {
                [COMPRESSION_ALGORITHM_ZSTD] = 3,
                [COMPRESSION_ALGORITHM_LZ4] = 1,
                [COMPRESSION_ALGORITHM_GZIP] = 3,
                [COMPRESSION_ALGORITHM_BROTLI] = 3,
            },
        },
    };

    int errors = 0;

    for(size_t i = 0; i < _countof(tests); i++) {
        struct config config = APPCONFIG_INITIALIZER;
        int levels[COMPRESSION_ALGORITHM_MAX] = { 0 };

        for(size_t j = 0; j < _countof(sender_compression_level_settings); j++) {
            compression_algorithm_t algorithm = sender_compression_level_settings[j].algorithm;
            if(tests[i].configured[algorithm])
                inicfg_set(&config, CONFIG_SECTION_STREAM, sender_compression_level_settings[j].setting,
                           tests[i].configured[algorithm]);
        }

        stream_conf_resolve_sender_compression_levels(&config, tests[i].profile, levels);

        for(size_t j = 0; j < _countof(sender_compression_level_settings); j++) {
            compression_algorithm_t algorithm = sender_compression_level_settings[j].algorithm;
            if(levels[algorithm] != tests[i].expected[algorithm]) {
                fprintf(stderr,
                        "STREAM CONF COMPRESSION TEST '%s': %s expected %d, got %d\n",
                        tests[i].name,
                        sender_compression_level_settings[j].setting,
                        tests[i].expected[algorithm],
                        levels[algorithm]);
                errors++;
            }
        }

        inicfg_free(&config);
    }

    static const struct {
        const char *name;
        const char *api_value;
        const char *machine_value;
        bool automatic;
        bool valid;
        bool bounded;
        int configured_s;
        uint32_t idle_s;
        STREAM_RECEIVER_KEEPALIVE_SOURCE source;
        const char *raw_value;
    } keepalive_tests[] = {
        { "default auto", NULL, NULL, true, true, false, 0, 30, STREAM_RECEIVER_KEEPALIVE_SOURCE_DEFAULT, "auto" },
        { "API auto", "auto", NULL, true, true, false, 0, 30, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "auto" },
        { "uppercase auto", "AUTO", NULL, true, true, false, 0, 30, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "AUTO" },
        { "API duration", "2m", NULL, false, true, false, 120, 120, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "2m" },
        { "machine duration overrides API", "2m", "3m", false, true, false, 180, 180, STREAM_RECEIVER_KEEPALIVE_SOURCE_MACHINE_GUID, "3m" },
        { "machine auto overrides API", "2m", "auto", true, true, false, 0, 30, STREAM_RECEIVER_KEEPALIVE_SOURCE_MACHINE_GUID, "auto" },
        { "machine invalid overrides API", "2m", "invalid", true, false, false, 0, 30,
          STREAM_RECEIVER_KEEPALIVE_SOURCE_MACHINE_GUID, "invalid" },
        { "zero clamps to lower bound", "0", NULL, false, true, true, 0, 30, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "0" },
        { "off clamps to lower bound", "off", NULL, false, true, true, 0, 30, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "off" },
        { "never clamps to lower bound", "never", NULL, false, true, true, 0, 30, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "never" },
        { "negative falls back to auto", "-1s", NULL, true, false, false, 0, 30, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "-1s" },
        { "exact lower bound", "30s", NULL, false, true, false, 30, 30, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "30s" },
        { "below lower bound", "29s", NULL, false, true, true, 29, 30, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "29s" },
        { "exact upper bound", "1h", NULL, false, true, false, 3600, 3600, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "1h" },
        { "above upper bound", "3601s", NULL, false, true, true, 3601, 3600, STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "3601s" },
        { "huge duration falls back to auto", "999999999999999d", NULL, true, false, false, 0, 30,
          STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "999999999999999d" },
        { "unparseable falls back to auto", "not-a-duration", NULL, true, false, false, 0, 30,
          STREAM_RECEIVER_KEEPALIVE_SOURCE_API_KEY, "not-a-duration" },
    };

    for(size_t i = 0; i < _countof(keepalive_tests); i++) {
        struct config config = APPCONFIG_INITIALIZER;
        if(keepalive_tests[i].api_value)
            inicfg_set(&config, "api", "tcp keepalive idle", keepalive_tests[i].api_value);
        if(keepalive_tests[i].machine_value)
            inicfg_set(&config, "machine", "tcp keepalive idle", keepalive_tests[i].machine_value);

        STREAM_RECEIVER_KEEPALIVE_CONFIG actual =
            stream_conf_resolve_receiver_keepalive(&config, "api", "machine");
        if(actual.automatic != keepalive_tests[i].automatic ||
           actual.valid != keepalive_tests[i].valid ||
           actual.bounded != keepalive_tests[i].bounded ||
           actual.configured_s != keepalive_tests[i].configured_s ||
           actual.idle_s != keepalive_tests[i].idle_s ||
           strcmp(actual.raw_value, keepalive_tests[i].raw_value) ||
           actual.source != keepalive_tests[i].source) {
            fprintf(stderr,
                    "STREAM CONF KEEPALIVE TEST '%s': got automatic=%d valid=%d bounded=%d configured=%d idle=%u section=%s raw=%s\n",
                    keepalive_tests[i].name, actual.automatic, actual.valid, actual.bounded,
                    actual.configured_s, actual.idle_s,
                    stream_conf_receiver_keepalive_source_name(actual.source), actual.raw_value);
            errors++;
        }
        inicfg_free(&config);
    }

    if(!errors)
        fprintf(stderr, "STREAM CONF COMPRESSION AND RECEIVER KEEPALIVE TESTS PASSED\n");

    return errors;
}

static void stream_conf_load_internal() {
    errno_clear();
    char *filename = filename_from_path_entry_strdupz(netdata_configured_user_config_dir, "stream.conf");
    if(!inicfg_load(&stream_config, filename, 0, NULL)) {
        nd_log_daemon(NDLP_NOTICE, "CONFIG: cannot load user config '%s'. Will try stock config.", filename);
        freez(filename);

        filename = filename_from_path_entry_strdupz(netdata_configured_stock_config_dir, "stream.conf");
        if(!inicfg_load(&stream_config, filename, 0, NULL))
            nd_log_daemon(NDLP_NOTICE, "CONFIG: cannot load stock config '%s'. Running with internal defaults.", filename);
    }

    freez(filename);

    inicfg_move(&stream_config,
                   CONFIG_SECTION_STREAM, "timeout seconds",
                   CONFIG_SECTION_STREAM, "timeout");

    inicfg_move(&stream_config,
                   CONFIG_SECTION_STREAM, "reconnect delay seconds",
                   CONFIG_SECTION_STREAM, "reconnect delay");

    inicfg_move_everywhere(&stream_config, "default memory mode", "db");
    inicfg_move_everywhere(&stream_config, "memory mode", "db");
    inicfg_move_everywhere(&stream_config, "db mode", "db");
    inicfg_move_everywhere(&stream_config, "default history", "retention");
    inicfg_move_everywhere(&stream_config, "history", "retention");
    inicfg_move_everywhere(&stream_config, "default proxy enabled", "proxy enabled");
    inicfg_move_everywhere(&stream_config, "default proxy destination", "proxy destination");
    inicfg_move_everywhere(&stream_config, "default proxy api key", "proxy api key");
    inicfg_move_everywhere(&stream_config, "default proxy send charts matching", "proxy send charts matching");
    inicfg_move_everywhere(&stream_config, "default health log history", "health log retention");
    inicfg_move_everywhere(&stream_config, "health log history", "health log retention");
    inicfg_move_everywhere(&stream_config, "seconds to replicate", "replication period");
    inicfg_move_everywhere(&stream_config, "seconds per replication step", "replication step");
    inicfg_move_everywhere(&stream_config, "default postpone alarms on connect seconds", "postpone alerts on connect");
    inicfg_move_everywhere(&stream_config, "postpone alarms on connect seconds", "postpone alerts on connect");
    inicfg_move_everywhere(&stream_config, "health enabled by default", "health enabled");
    inicfg_move_everywhere(&stream_config, "buffer size bytes", "buffer size");
}

bool stream_conf_receiver_needs_dbengine(void) {
    return stream_conf_needs_dbengine(&stream_config);
}

void stream_conf_load() {
    FUNCTION_RUN_ONCE();

    stream_conf_load_internal();
    check_local_streaming_capabilities();

    stream_send.enabled =
        inicfg_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "enabled", stream_send.enabled);

    stream_send.parents.destination =
        string_strdupz(inicfg_get(&stream_config, CONFIG_SECTION_STREAM, "destination", ""));

    stream_send.api_key =
        string_strdupz(inicfg_get(&stream_config, CONFIG_SECTION_STREAM, "api key", ""));

    stream_send.send_charts_matching =
        string_strdupz(inicfg_get(&stream_config, CONFIG_SECTION_STREAM, "send charts matching", "*"));

    stream_receive.replication.enabled =
        inicfg_get_boolean(&netdata_config, CONFIG_SECTION_DB, "enable replication",
                           stream_receive.replication.enabled);

    stream_receive.replication.period =
        inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "replication period",
                                    stream_receive.replication.period);

    stream_receive.replication.step =
        inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_DB, "replication step",
                                    stream_receive.replication.step);

    stream_send.replication.threads = inicfg_get_number_range(
        &netdata_config, CONFIG_SECTION_DB, "replication threads",
        replication_threads_default(), 1, MAX_REPLICATION_THREADS);

    stream_send.replication.prefetch = inicfg_get_number_range(
        &netdata_config, CONFIG_SECTION_DB, "replication prefetch",
        replication_prefetch_default(), 1, MAX_REPLICATION_PREFETCH);

    stream_send.buffer_max_size = (size_t)inicfg_get_size_bytes(
        &stream_config, CONFIG_SECTION_STREAM, "buffer size",
        stream_send.buffer_max_size);

    stream_send.parents.reconnect_delay_s = (unsigned int)inicfg_get_duration_seconds(
        &stream_config, CONFIG_SECTION_STREAM, "reconnect delay",
        stream_send.parents.reconnect_delay_s);
    if(stream_send.parents.reconnect_delay_s < SENDER_MIN_RECONNECT_DELAY)
        stream_send.parents.reconnect_delay_s = SENDER_MIN_RECONNECT_DELAY;

    stream_send.compression.enabled =
        inicfg_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "enable compression",
                              stream_send.compression.enabled);

    stream_send.parents.h2o = inicfg_get_boolean(
        &stream_config, CONFIG_SECTION_STREAM, "parent using h2o",
        stream_send.parents.h2o);

    stream_send.parents.timeout_s = (int)inicfg_get_duration_seconds(
        &stream_config, CONFIG_SECTION_STREAM, "timeout",
        stream_send.parents.timeout_s);

    stream_send.buffer_max_size = (size_t)inicfg_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "buffer size bytes",
        stream_send.buffer_max_size);

    stream_send.parents.default_port = (int)inicfg_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "default port",
        stream_send.parents.default_port);

    stream_send.initial_clock_resync_iterations = (unsigned int)inicfg_get_number(
        &stream_config, CONFIG_SECTION_STREAM, "initial clock resync iterations",
        stream_send.initial_clock_resync_iterations); // TODO: REMOVE FOR SLEW / GAPFILLING

    netdata_ssl_validate_certificate_sender = !inicfg_get_boolean(
        &stream_config, CONFIG_SECTION_STREAM, "ssl skip certificate verification",
        !netdata_ssl_validate_certificate);

    if(!netdata_ssl_validate_certificate_sender)
        nd_log_daemon(NDLP_NOTICE, "SSL: streaming senders will skip SSL certificates verification.");

    stream_send.parents.ssl_ca_path = string_strdupz(inicfg_get_path(&stream_config, CONFIG_SECTION_STREAM, "CApath", NULL));
    stream_send.parents.ssl_ca_file = string_strdupz(inicfg_get_filename(&stream_config, CONFIG_SECTION_STREAM, "CAfile", NULL));

    if(stream_send.enabled && (!stream_send.parents.destination || !stream_send.api_key)) {
        nd_log_daemon(
            NDLP_ERR,
            "STREAM [send]: cannot enable sending thread - missing required fields (destination: %s, api key: %s)",
            stream_send.parents.destination ? "present" : "missing",
            stream_send.api_key ? "present" : "missing");
        stream_send.enabled = false;
    }

    stream_conf_is_parent(true);
}

bool stream_conf_is_parent(bool recheck) {
    static bool rc = false, queried = false;
    if(!recheck && queried)
        return rc;

    rc = stream_conf_has_api_enabled(&stream_config);
    queried = true;

    return rc;
}

bool stream_conf_is_child(void) {
    return stream_send.enabled;
}

void stream_conf_receiver_config(struct receiver_state *rpt, struct stream_receiver_config *config, const char *api_key, const char *machine_guid) {
    config->mode = rrd_memory_mode_id(
        inicfg_get(&stream_config, machine_guid, "db",
        inicfg_get(&stream_config, api_key, "db",
        rrd_memory_mode_name(default_rrd_memory_mode))));

    if (unlikely(config->mode == RRD_DB_MODE_DBENGINE && !dbengine_enabled)) {
        netdata_log_error("STREAM RCV '%s' [from [%s]:%s]: "
                          "dbengine is not enabled, falling back to default."
                          , rpt->hostname
                          , rpt->remote_ip, rpt->remote_port);
        config->mode = default_rrd_memory_mode;
    }

    config->history = (int)
        inicfg_get_number(&stream_config, machine_guid, "retention",
        inicfg_get_number(&stream_config, api_key, "retention",
        default_rrd_history_entries));
    if(config->history < 5) config->history = 5;

    config->health.enabled =
        inicfg_get_boolean_ondemand(&stream_config, machine_guid, "health enabled",
        inicfg_get_boolean_ondemand(&stream_config, api_key, "health enabled",
        health_plugin_enabled()));

    config->health.delay =
        inicfg_get_duration_seconds(&stream_config, machine_guid, "postpone alerts on connect",
        inicfg_get_duration_seconds(&stream_config, api_key, "postpone alerts on connect",
        60));

    config->update_every = (int)inicfg_get_duration_seconds(&stream_config, machine_guid, "update every", config->update_every);
    if(config->update_every < 0) config->update_every = 1;

    config->health.history =
        inicfg_get_duration_seconds(&stream_config, machine_guid, "health log retention",
        inicfg_get_duration_seconds(&stream_config, api_key, "health log retention",
        HEALTH_LOG_RETENTION_DEFAULT));

    config->send.enabled =
        inicfg_get_boolean(&stream_config, machine_guid, "proxy enabled",
        inicfg_get_boolean(&stream_config, api_key, "proxy enabled",
        stream_send.enabled));

    config->send.parents = string_strdupz(
        inicfg_get(&stream_config, machine_guid, "proxy destination",
        inicfg_get(&stream_config, api_key, "proxy destination",
        string2str(stream_send.parents.destination))));

    config->send.api_key = string_strdupz(
        inicfg_get(&stream_config, machine_guid, "proxy api key",
        inicfg_get(&stream_config, api_key, "proxy api key",
        string2str(stream_send.api_key))));

    config->send.charts_matching = string_strdupz(
        inicfg_get(&stream_config, machine_guid, "proxy send charts matching",
        inicfg_get(&stream_config, api_key, "proxy send charts matching",
        string2str(stream_send.send_charts_matching))));

    config->replication.enabled =
        inicfg_get_boolean(&stream_config, machine_guid, "enable replication",
        inicfg_get_boolean(&stream_config, api_key, "enable replication",
        stream_receive.replication.enabled));

    config->replication.period =
        inicfg_get_duration_seconds(&stream_config, machine_guid, "replication period",
        inicfg_get_duration_seconds(&stream_config, api_key, "replication period",
        stream_receive.replication.period));

    config->replication.step =
        inicfg_get_duration_seconds(&stream_config, machine_guid, "replication step",
        inicfg_get_duration_seconds(&stream_config, api_key, "replication step",
        stream_receive.replication.step));

    config->compression.enabled =
        inicfg_get_boolean(&stream_config, machine_guid, "enable compression",
        inicfg_get_boolean(&stream_config, api_key, "enable compression",
        stream_send.compression.enabled));

    if(config->compression.enabled) {
        stream_parse_compression_order(
            config,
            inicfg_get(
                &stream_config, machine_guid, "compression algorithms order",
                inicfg_get(&stream_config, api_key, "compression algorithms order", STREAM_COMPRESSION_ALGORITHMS_ORDER)));
    }

    STREAM_RECEIVER_KEEPALIVE_CONFIG keepalive =
        stream_conf_resolve_receiver_keepalive(&stream_config, api_key, machine_guid);
    config->tcp_keepalive.automatic = keepalive.automatic;
    config->tcp_keepalive.idle_s = keepalive.idle_s;

    if(!keepalive.valid)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM RCV '%s' [from [%s]:%s]: tcp keepalive idle = %s in the %s is invalid; using automatic receiver cadence",
               rpt->hostname, rpt->remote_ip, rpt->remote_port,
               keepalive.raw_value, stream_conf_receiver_keepalive_source_name(keepalive.source));
    else if(!keepalive.automatic && keepalive.bounded)
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "STREAM RCV '%s' [from [%s]:%s]: tcp keepalive idle = %s in the %s resolves to %d seconds "
               "outside the supported range; using %u seconds",
               rpt->hostname, rpt->remote_ip, rpt->remote_port,
               keepalive.raw_value, stream_conf_receiver_keepalive_source_name(keepalive.source),
               keepalive.configured_s, keepalive.idle_s);
}

bool stream_conf_is_key_type(const char *api_key, const char *type) {
    const char *api_key_type = inicfg_get(&stream_config, api_key, "type", type);
    if(!api_key_type || !*api_key_type) api_key_type = "unknown";
    return strcmp(api_key_type, type) == 0;
}

bool stream_conf_api_key_is_enabled(const char *api_key, bool enabled) {
    return inicfg_get_boolean(&stream_config, api_key, "enabled", enabled);
}

bool stream_conf_api_key_allows_client(const char *api_key, const char *client_ip) {
    SIMPLE_PATTERN *key_allow_from = simple_pattern_create(
        inicfg_get(&stream_config, api_key, "allow from", "*"),
        NULL, SIMPLE_PATTERN_EXACT, true);

    bool rc = true;

    if(key_allow_from) {
        rc = simple_pattern_matches(key_allow_from, client_ip);
        simple_pattern_free(key_allow_from);
    }

    return rc;
}
