// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdpush.h"

/*
 * rrdpush
 *
 * 3 threads are involved for all stream operations
 *
 * 1. a random data collection thread, calling rrdset_done_push()
 *    this is called for each chart.
 *
 *    the output of this work is kept in a thread BUFFER
 *    the sender thread is signalled via a pipe (in RRDHOST)
 *
 * 2. a sender thread running at the sending netdata
 *    this is spawned automatically on the first chart to be pushed
 *
 *    It tries to push the metrics to the remote netdata, as fast
 *    as possible (i.e. immediately after they are collected).
 *
 * 3. a receiver thread, running at the receiving netdata
 *    this is spawned automatically when the sender connects to
 *    the receiver.
 *
 */

struct config stream_config = APPCONFIG_INITIALIZER;

unsigned int default_rrdpush_enabled = 0;

unsigned int default_rrdpush_compression_enabled = 1;
const char *default_rrdpush_destination = NULL;
const char *default_rrdpush_api_key = NULL;
const char *default_rrdpush_send_charts_matching = "*";
bool default_rrdpush_enable_replication = true;
time_t default_rrdpush_seconds_to_replicate = 86400;
time_t default_rrdpush_replication_step = 600;
const char *netdata_ssl_ca_path = NULL;
const char *netdata_ssl_ca_file = NULL;

static void load_stream_conf() {
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

bool rrdpush_receiver_needs_dbengine(void) {
    return stream_conf_needs_dbengine(&stream_config);
}

int rrdpush_init() {
    // --------------------------------------------------------------------
    // load stream.conf
    load_stream_conf();

    default_rrdpush_enabled =
        (unsigned int)appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "enabled", default_rrdpush_enabled);

    default_rrdpush_destination =
        appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "destination", "");

    default_rrdpush_api_key =
        appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "api key", "");

    default_rrdpush_send_charts_matching =
        appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "send charts matching", default_rrdpush_send_charts_matching);

    default_rrdpush_enable_replication =
        config_get_boolean(CONFIG_SECTION_DB, "enable replication", default_rrdpush_enable_replication);

    default_rrdpush_seconds_to_replicate =
        config_get_duration_seconds(CONFIG_SECTION_DB, "replication period", default_rrdpush_seconds_to_replicate);

    default_rrdpush_replication_step =
        config_get_duration_seconds(CONFIG_SECTION_DB, "replication step", default_rrdpush_replication_step);

    rrdhost_free_orphan_time_s =
        config_get_duration_seconds(CONFIG_SECTION_DB, "cleanup orphan hosts after", rrdhost_free_orphan_time_s);

    default_rrdpush_compression_enabled =
        (unsigned int)appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM,
                                            "enable compression", default_rrdpush_compression_enabled);

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

    if(default_rrdpush_enabled && (!default_rrdpush_destination || !*default_rrdpush_destination || !default_rrdpush_api_key || !*default_rrdpush_api_key)) {
        nd_log_daemon(NDLP_WARNING, "STREAM [send]: cannot enable sending thread - information is missing.");
        default_rrdpush_enabled = 0;
    }

    netdata_ssl_validate_certificate_sender = !appconfig_get_boolean(&stream_config, CONFIG_SECTION_STREAM, "ssl skip certificate verification", !netdata_ssl_validate_certificate);

    if(!netdata_ssl_validate_certificate_sender)
        nd_log_daemon(NDLP_NOTICE, "SSL: streaming senders will skip SSL certificates verification.");

    netdata_ssl_ca_path = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "CApath", NULL);
    netdata_ssl_ca_file = appconfig_get(&stream_config, CONFIG_SECTION_STREAM, "CAfile", NULL);

    return default_rrdpush_enabled;
}

// data collection happens from multiple threads
// each of these threads calls rrdset_done()
// which in turn calls rrdset_done_push()
// which uses this pipe to notify the streaming thread
// that there are more data ready to be sent
#define PIPE_READ 0
#define PIPE_WRITE 1

// to have the remote netdata re-sync the charts
// to its current clock, we send for this many
// iterations a BEGIN line without microseconds
// this is for the first iterations of each chart
unsigned int remote_clock_resync_iterations = 60;

static inline bool should_send_chart_matching(RRDSET *st, RRDSET_FLAGS flags) {
    if(!(flags & RRDSET_FLAG_RECEIVER_REPLICATION_FINISHED))
        return false;

    if(unlikely(!(flags & (RRDSET_FLAG_UPSTREAM_SEND | RRDSET_FLAG_UPSTREAM_IGNORE)))) {
        RRDHOST *host = st->rrdhost;

        if (flags & RRDSET_FLAG_ANOMALY_DETECTION) {
            if(ml_streaming_enabled())
                rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_SEND);
            else
                rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_IGNORE);
        }
        else {
            int negative = 0, positive = 0;
            SIMPLE_PATTERN_RESULT r;

            r = simple_pattern_matches_string_extract(host->rrdpush.send.charts_matching, st->context, NULL, 0);
            if(r == SP_MATCHED_POSITIVE) positive++;
            else if(r == SP_MATCHED_NEGATIVE) negative++;

            if(!negative) {
                r = simple_pattern_matches_string_extract(host->rrdpush.send.charts_matching, st->name, NULL, 0);
                if (r == SP_MATCHED_POSITIVE) positive++;
                else if (r == SP_MATCHED_NEGATIVE) negative++;
            }

            if(!negative) {
                r = simple_pattern_matches_string_extract(host->rrdpush.send.charts_matching, st->id, NULL, 0);
                if (r == SP_MATCHED_POSITIVE) positive++;
                else if (r == SP_MATCHED_NEGATIVE) negative++;
            }

            if(!negative && positive)
                rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_SEND);
            else
                rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_IGNORE);
        }

        // get the flags again, to know how to respond
        flags = rrdset_flag_check(st, RRDSET_FLAG_UPSTREAM_SEND|RRDSET_FLAG_UPSTREAM_IGNORE);
    }

    return flags & RRDSET_FLAG_UPSTREAM_SEND;
}

int configured_as_parent() {
    return stream_conf_has_uuid_section(&stream_config);
}

// chart labels
static int send_clabels_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    BUFFER *wb = (BUFFER *)data;
    buffer_sprintf(wb, PLUGINSD_KEYWORD_CLABEL " \"%s\" \"%s\" %d\n", name, value, ls & ~(RRDLABEL_FLAG_INTERNAL));
    return 1;
}

static void rrdpush_send_clabels(BUFFER *wb, RRDSET *st) {
    if (st->rrdlabels) {
        if(rrdlabels_walkthrough_read(st->rrdlabels, send_clabels_callback, wb) > 0)
            buffer_sprintf(wb, PLUGINSD_KEYWORD_CLABEL_COMMIT "\n");
    }
}

// Send the current chart definition.
// Assumes that collector thread has already called sender_start for mutex / buffer state.
static inline bool rrdpush_send_chart_definition(BUFFER *wb, RRDSET *st) {
    uint32_t version = rrdset_metadata_version(st);

    RRDHOST *host = st->rrdhost;
    NUMBER_ENCODING integer_encoding = stream_has_capability(host->sender, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_HEX;
    bool with_slots = stream_has_capability(host->sender, STREAM_CAP_SLOTS) ? true : false;

    bool replication_progress = false;

    // properly set the name for the remote end to parse it
    char *name = "";
    if(likely(st->name)) {
        if(unlikely(st->id != st->name)) {
            // they differ
            name = strchr(rrdset_name(st), '.');
            if(name)
                name++;
            else
                name = "";
        }
    }

    buffer_fast_strcat(wb, PLUGINSD_KEYWORD_CHART, sizeof(PLUGINSD_KEYWORD_CHART) - 1);

    if(with_slots) {
        buffer_fast_strcat(wb, " "PLUGINSD_KEYWORD_SLOT":", sizeof(PLUGINSD_KEYWORD_SLOT) - 1 + 2);
        buffer_print_uint64_encoded(wb, integer_encoding, st->rrdpush.sender.chart_slot);
    }

    // send the chart
    buffer_sprintf(
            wb
            , " \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" %d %d \"%s %s %s %s\" \"%s\" \"%s\"\n"
            , rrdset_id(st)
            , name
            , rrdset_title(st)
            , rrdset_units(st)
            , rrdset_family(st)
            , rrdset_context(st)
            , rrdset_type_name(st->chart_type)
            , st->priority
            , st->update_every
            , rrdset_flag_check(st, RRDSET_FLAG_OBSOLETE)?"obsolete":""
            , rrdset_flag_check(st, RRDSET_FLAG_DETAIL)?"detail":""
            , rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST)?"store_first":""
            , rrdset_flag_check(st, RRDSET_FLAG_HIDDEN)?"hidden":""
            , rrdset_plugin_name(st)
            , rrdset_module_name(st)
    );

    // send the chart labels
    if (stream_has_capability(host->sender, STREAM_CAP_CLABELS))
        rrdpush_send_clabels(wb, st);

    // send the dimensions
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        buffer_fast_strcat(wb, PLUGINSD_KEYWORD_DIMENSION, sizeof(PLUGINSD_KEYWORD_DIMENSION) - 1);

        if(with_slots) {
            buffer_fast_strcat(wb, " "PLUGINSD_KEYWORD_SLOT":", sizeof(PLUGINSD_KEYWORD_SLOT) - 1 + 2);
            buffer_print_uint64_encoded(wb, integer_encoding, rd->rrdpush.sender.dim_slot);
        }

        buffer_sprintf(
            wb
            , " \"%s\" \"%s\" \"%s\" %d %d \"%s %s %s\"\n"
            , rrddim_id(rd)
            , rrddim_name(rd)
            , rrd_algorithm_name(rd->algorithm)
            , rd->multiplier
            , rd->divisor
            , rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)?"obsolete":""
            , rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN)?"hidden":""
            , rrddim_option_check(rd, RRDDIM_OPTION_DONT_DETECT_RESETS_OR_OVERFLOWS)?"noreset":""
        );
    }
    rrddim_foreach_done(rd);

    // send the chart functions
    if(stream_has_capability(host->sender, STREAM_CAP_FUNCTIONS))
        rrd_chart_functions_expose_rrdpush(st, wb);

    // send the chart local custom variables
    rrdvar_print_to_streaming_custom_chart_variables(st, wb);

    if (stream_has_capability(host->sender, STREAM_CAP_REPLICATION)) {
        time_t db_first_time_t, db_last_time_t;

        time_t now = now_realtime_sec();
        rrdset_get_retention_of_tier_for_collected_chart(st, &db_first_time_t, &db_last_time_t, now, 0);

        buffer_sprintf(wb, PLUGINSD_KEYWORD_CHART_DEFINITION_END " %llu %llu %llu\n",
                       (unsigned long long)db_first_time_t,
                       (unsigned long long)db_last_time_t,
                       (unsigned long long)now);

        if(!rrdset_flag_check(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS)) {
            rrdset_flag_set(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS);
            rrdset_flag_clear(st, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);
            rrdhost_sender_replicating_charts_plus_one(st->rrdhost);
        }
        replication_progress = true;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
        internal_error(true, "REPLAY: 'host:%s/chart:%s' replication starts",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st));
#endif
    }

    sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);

    // we can set the exposed flag, after we commit the buffer
    // because replication may pick it up prematurely
    rrddim_foreach_read(rd, st) {
        rrddim_metadata_exposed_upstream(rd, version);
    }
    rrddim_foreach_done(rd);
    rrdset_metadata_exposed_upstream(st, version);

    st->rrdpush.sender.resync_time_s = st->last_collected_time.tv_sec + (remote_clock_resync_iterations * st->update_every);
    return replication_progress;
}

// sends the current chart dimensions
static void rrdpush_send_chart_metrics(BUFFER *wb, RRDSET *st, struct sender_state *s __maybe_unused, RRDSET_FLAGS flags) {
    buffer_fast_strcat(wb, "BEGIN \"", 7);
    buffer_fast_strcat(wb, rrdset_id(st), string_strlen(st->id));
    buffer_fast_strcat(wb, "\" ", 2);

    if(st->last_collected_time.tv_sec > st->rrdpush.sender.resync_time_s)
        buffer_print_uint64(wb, st->usec_since_last_update);
    else
        buffer_fast_strcat(wb, "0", 1);

    buffer_fast_strcat(wb, "\n", 1);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(unlikely(!rrddim_check_updated(rd)))
            continue;

        if(likely(rrddim_check_upstream_exposed_collector(rd))) {
            buffer_fast_strcat(wb, "SET \"", 5);
            buffer_fast_strcat(wb, rrddim_id(rd), string_strlen(rd->id));
            buffer_fast_strcat(wb, "\" = ", 4);
            buffer_print_int64(wb, rd->collector.collected_value);
            buffer_fast_strcat(wb, "\n", 1);
        }
        else {
            internal_error(true, "STREAM: 'host:%s/chart:%s/dim:%s' flag 'exposed' is updated but not exposed",
                           rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_id(rd));
            // we will include it in the next iteration
            rrddim_metadata_updated(rd);
        }
    }
    rrddim_foreach_done(rd);

    if(unlikely(flags & RRDSET_FLAG_UPSTREAM_SEND_VARIABLES))
        rrdvar_print_to_streaming_custom_chart_variables(st, wb);

    buffer_fast_strcat(wb, "END\n", 4);
}

static void rrdpush_sender_thread_spawn(RRDHOST *host);

// Called from the internal collectors to mark a chart obsolete.
bool rrdset_push_chart_definition_now(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    if(unlikely(!rrdhost_can_send_definitions_to_parent(host)
        || !should_send_chart_matching(st, rrdset_flag_get(st)))) {
        return false;
    }

    BUFFER *wb = sender_start(host->sender);
    rrdpush_send_chart_definition(wb, st);
    sender_thread_buffer_free();

    return true;
}

void rrdset_push_metrics_v1(RRDSET_STREAM_BUFFER *rsb, RRDSET *st) {
    RRDHOST *host = st->rrdhost;
    rrdpush_send_chart_metrics(rsb->wb, st, host->sender, rsb->rrdset_flags);
}

void rrddim_push_metrics_v2(RRDSET_STREAM_BUFFER *rsb, RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags) {
    if(!rsb->wb || !rsb->v2 || !netdata_double_isnumber(n) || !does_storage_number_exist(flags))
        return;

    bool with_slots = stream_has_capability(rsb, STREAM_CAP_SLOTS) ? true : false;
    NUMBER_ENCODING integer_encoding = stream_has_capability(rsb, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_HEX;
    NUMBER_ENCODING doubles_encoding = stream_has_capability(rsb, STREAM_CAP_IEEE754) ? NUMBER_ENCODING_BASE64 : NUMBER_ENCODING_DECIMAL;
    BUFFER *wb = rsb->wb;
    time_t point_end_time_s = (time_t)(point_end_time_ut / USEC_PER_SEC);
    if(unlikely(rsb->last_point_end_time_s != point_end_time_s)) {

        if(unlikely(rsb->begin_v2_added))
            buffer_fast_strcat(wb, PLUGINSD_KEYWORD_END_V2 "\n", sizeof(PLUGINSD_KEYWORD_END_V2) - 1 + 1);

        buffer_fast_strcat(wb, PLUGINSD_KEYWORD_BEGIN_V2, sizeof(PLUGINSD_KEYWORD_BEGIN_V2) - 1);

        if(with_slots) {
            buffer_fast_strcat(wb, " "PLUGINSD_KEYWORD_SLOT":", sizeof(PLUGINSD_KEYWORD_SLOT) - 1 + 2);
            buffer_print_uint64_encoded(wb, integer_encoding, rd->rrdset->rrdpush.sender.chart_slot);
        }

        buffer_fast_strcat(wb, " '", 2);
        buffer_fast_strcat(wb, rrdset_id(rd->rrdset), string_strlen(rd->rrdset->id));
        buffer_fast_strcat(wb, "' ", 2);
        buffer_print_uint64_encoded(wb, integer_encoding, rd->rrdset->update_every);
        buffer_fast_strcat(wb, " ", 1);
        buffer_print_uint64_encoded(wb, integer_encoding, point_end_time_s);
        buffer_fast_strcat(wb, " ", 1);
        if(point_end_time_s == rsb->wall_clock_time)
            buffer_fast_strcat(wb, "#", 1);
        else
            buffer_print_uint64_encoded(wb, integer_encoding, rsb->wall_clock_time);
        buffer_fast_strcat(wb, "\n", 1);

        rsb->last_point_end_time_s = point_end_time_s;
        rsb->begin_v2_added = true;
    }

    buffer_fast_strcat(wb, PLUGINSD_KEYWORD_SET_V2, sizeof(PLUGINSD_KEYWORD_SET_V2) - 1);

    if(with_slots) {
        buffer_fast_strcat(wb, " "PLUGINSD_KEYWORD_SLOT":", sizeof(PLUGINSD_KEYWORD_SLOT) - 1 + 2);
        buffer_print_uint64_encoded(wb, integer_encoding, rd->rrdpush.sender.dim_slot);
    }

    buffer_fast_strcat(wb, " '", 2);
    buffer_fast_strcat(wb, rrddim_id(rd), string_strlen(rd->id));
    buffer_fast_strcat(wb, "' ", 2);
    buffer_print_int64_encoded(wb, integer_encoding, rd->collector.last_collected_value);
    buffer_fast_strcat(wb, " ", 1);

    if((NETDATA_DOUBLE)rd->collector.last_collected_value == n)
        buffer_fast_strcat(wb, "#", 1);
    else
        buffer_print_netdata_double_encoded(wb, doubles_encoding, n);

    buffer_fast_strcat(wb, " ", 1);
    buffer_print_sn_flags(wb, flags, true);
    buffer_fast_strcat(wb, "\n", 1);
}

void rrdset_push_metrics_finished(RRDSET_STREAM_BUFFER *rsb, RRDSET *st) {
    if(!rsb->wb)
        return;

    if(rsb->v2 && rsb->begin_v2_added) {
        if(unlikely(rsb->rrdset_flags & RRDSET_FLAG_UPSTREAM_SEND_VARIABLES))
            rrdvar_print_to_streaming_custom_chart_variables(st, rsb->wb);

        buffer_fast_strcat(rsb->wb, PLUGINSD_KEYWORD_END_V2 "\n", sizeof(PLUGINSD_KEYWORD_END_V2) - 1 + 1);
    }

    sender_commit(st->rrdhost->sender, rsb->wb, STREAM_TRAFFIC_TYPE_DATA);

    *rsb = (RRDSET_STREAM_BUFFER){ .wb = NULL, };
}

RRDSET_STREAM_BUFFER rrdset_push_metric_initialize(RRDSET *st, time_t wall_clock_time) {
    RRDHOST *host = st->rrdhost;

    // fetch the flags we need to check with one atomic operation
    RRDHOST_FLAGS host_flags = __atomic_load_n(&host->flags, __ATOMIC_SEQ_CST);

    // check if we are not connected
    if(unlikely(!(host_flags & RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS))) {

        if(unlikely(!(host_flags & (RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN | RRDHOST_FLAG_RRDPUSH_RECEIVER_DISCONNECTED))))
            rrdpush_sender_thread_spawn(host);

        if(unlikely(!(host_flags & RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS))) {
            rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS);
            nd_log_daemon(NDLP_NOTICE, "STREAM %s [send]: not ready - collected metrics are not sent to parent.", rrdhost_hostname(host));
        }

        return (RRDSET_STREAM_BUFFER) { .wb = NULL, };
    }
    else if(unlikely(host_flags & RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS)) {
        nd_log_daemon(NDLP_INFO, "STREAM %s [send]: sending metrics to parent...", rrdhost_hostname(host));
        rrdhost_flag_clear(host, RRDHOST_FLAG_RRDPUSH_SENDER_LOGGED_STATUS);
    }

    if(unlikely(host_flags & RRDHOST_FLAG_GLOBAL_FUNCTIONS_UPDATED)) {
        BUFFER *wb = sender_start(host->sender);
        rrd_global_functions_expose_rrdpush(host, wb, stream_has_capability(host->sender, STREAM_CAP_DYNCFG));
        sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_FUNCTIONS);
    }

    bool exposed_upstream = rrdset_check_upstream_exposed(st);
    RRDSET_FLAGS rrdset_flags = rrdset_flag_get(st);
    bool replication_in_progress = !(rrdset_flags & RRDSET_FLAG_SENDER_REPLICATION_FINISHED);

    if(unlikely((exposed_upstream && replication_in_progress) ||
                !should_send_chart_matching(st, rrdset_flags)))
        return (RRDSET_STREAM_BUFFER) { .wb = NULL, };

    if(unlikely(!exposed_upstream)) {
        BUFFER *wb = sender_start(host->sender);
        replication_in_progress = rrdpush_send_chart_definition(wb, st);
    }

    if(replication_in_progress)
        return (RRDSET_STREAM_BUFFER) { .wb = NULL, };

    return (RRDSET_STREAM_BUFFER) {
        .capabilities = host->sender->capabilities,
        .v2 = stream_has_capability(host->sender, STREAM_CAP_INTERPOLATED),
        .rrdset_flags = rrdset_flags,
        .wb = sender_start(host->sender),
        .wall_clock_time = wall_clock_time,
    };
}

// labels
static int send_labels_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    BUFFER *wb = (BUFFER *)data;
    buffer_sprintf(wb, "LABEL \"%s\" = %d \"%s\"\n", name, ls, value);
    return 1;
}

void rrdpush_send_host_labels(RRDHOST *host) {
    if(unlikely(!rrdhost_can_send_definitions_to_parent(host)
                 || !stream_has_capability(host->sender, STREAM_CAP_HLABELS)))
        return;

    BUFFER *wb = sender_start(host->sender);

    rrdlabels_walkthrough_read(host->rrdlabels, send_labels_callback, wb);
    buffer_sprintf(wb, "OVERWRITE %s\n", "labels");

    sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);

    sender_thread_buffer_free();
}

void rrdpush_send_global_functions(RRDHOST *host) {
    if(!stream_has_capability(host->sender, STREAM_CAP_FUNCTIONS))
        return;

    if(unlikely(!rrdhost_can_send_definitions_to_parent(host)))
        return;

    BUFFER *wb = sender_start(host->sender);

    rrd_global_functions_expose_rrdpush(host, wb, stream_has_capability(host->sender, STREAM_CAP_DYNCFG));

    sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_FUNCTIONS);

    sender_thread_buffer_free();
}

int connect_to_one_of_destinations(
    RRDHOST *host,
    int default_port,
    struct timeval *timeout,
    size_t *reconnects_counter,
    char *connected_to,
    size_t connected_to_size,
    struct rrdpush_destinations **destination)
{
    int sock = -1;

    for (struct rrdpush_destinations *d = host->destinations; d; d = d->next) {
        time_t now = now_realtime_sec();

        if(nd_thread_signaled_to_cancel())
            return -1;

        if(d->postpone_reconnection_until > now)
            continue;

        nd_log(NDLS_DAEMON, NDLP_DEBUG,
            "STREAM %s: connecting to '%s' (default port: %d)...",
            rrdhost_hostname(host), string2str(d->destination), default_port);

        if (reconnects_counter)
            *reconnects_counter += 1;

        d->since = now;
        d->attempts++;
        sock = connect_to_this(string2str(d->destination), default_port, timeout);

        if (sock != -1) {
            if (connected_to && connected_to_size)
                strncpyz(connected_to, string2str(d->destination), connected_to_size);

            *destination = d;

            // move the current item to the end of the list
            // without this, this destination will break the loop again and again
            // not advancing the destinations to find one that may work
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->destinations, d, prev, next);
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(host->destinations, d, prev, next);

            break;
        }
    }

    return sock;
}

struct destinations_init_tmp {
    RRDHOST *host;
    struct rrdpush_destinations *list;
    int count;
};

bool destinations_init_add_one(char *entry, void *data) {
    struct destinations_init_tmp *t = data;

    struct rrdpush_destinations *d = callocz(1, sizeof(struct rrdpush_destinations));
    char *colon_ssl = strstr(entry, ":SSL");
    if(colon_ssl) {
        *colon_ssl = '\0';
        d->ssl = true;
    }
    else
        d->ssl = false;

    d->destination = string_strdupz(entry);

    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(struct rrdpush_destinations), __ATOMIC_RELAXED);

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(t->list, d, prev, next);

    t->count++;
    nd_log_daemon(NDLP_INFO, "STREAM: added streaming destination No %d: '%s' to host '%s'", t->count, string2str(d->destination), rrdhost_hostname(t->host));

    return false; // we return false, so that we will get all defined destinations
}

void rrdpush_destinations_init(RRDHOST *host) {
    if(!host->rrdpush.send.destination) return;

    rrdpush_destinations_free(host);

    struct destinations_init_tmp t = {
        .host = host,
        .list = NULL,
        .count = 0,
    };

    foreach_entry_in_connection_string(host->rrdpush.send.destination, destinations_init_add_one, &t);

    host->destinations = t.list;
}

void rrdpush_destinations_free(RRDHOST *host) {
    while (host->destinations) {
        struct rrdpush_destinations *tmp = host->destinations;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(host->destinations, tmp, prev, next);
        string_freez(tmp->destination);
        freez(tmp);
        __atomic_sub_fetch(&netdata_buffers_statistics.rrdhost_senders, sizeof(struct rrdpush_destinations), __ATOMIC_RELAXED);
    }

    host->destinations = NULL;
}

// ----------------------------------------------------------------------------
// rrdpush sender thread

// Either the receiver lost the connection or the host is being destroyed.
// The sender mutex guards thread creation, any spurious data is wiped on reconnection.
void rrdpush_sender_thread_stop(RRDHOST *host, STREAM_HANDSHAKE reason, bool wait) {
    if (!host->sender)
        return;

    sender_lock(host->sender);

    if(rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)) {

        host->sender->exit.shutdown = true;
        host->sender->exit.reason = reason;

        // signal it to cancel
        nd_thread_signal_cancel(host->rrdpush_sender_thread);
    }

    sender_unlock(host->sender);

    if(wait) {
        sender_lock(host->sender);
        while(host->sender->tid) {
            sender_unlock(host->sender);
            sleep_usec(10 * USEC_PER_MS);
            sender_lock(host->sender);
        }
        sender_unlock(host->sender);
    }
}

// ----------------------------------------------------------------------------
// rrdpush receiver thread

static void rrdpush_sender_thread_spawn(RRDHOST *host) {
    sender_lock(host->sender);

    if(!rrdhost_flag_check(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN)) {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_SENDER "[%s]", rrdhost_hostname(host));

        host->rrdpush_sender_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT,
                                                       rrdpush_sender_thread, (void *)host->sender);
        if(!host->rrdpush_sender_thread)
            nd_log_daemon(NDLP_ERR, "STREAM %s [send]: failed to create new thread for client.", rrdhost_hostname(host));
        else
            rrdhost_flag_set(host, RRDHOST_FLAG_RRDPUSH_SENDER_SPAWN);
    }

    sender_unlock(host->sender);
}

int rrdpush_receiver_permission_denied(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, START_STREAMING_ERROR_NOT_PERMITTED);
    return HTTP_RESP_UNAUTHORIZED;
}

int rrdpush_receiver_too_busy_now(struct web_client *w) {
    // we always respond with the same message and error code
    // to prevent an attacker from gaining info about the error
    buffer_flush(w->response.data);
    buffer_strcat(w->response.data, START_STREAMING_ERROR_BUSY_TRY_LATER);
    return HTTP_RESP_SERVICE_UNAVAILABLE;
}

static void rrdpush_receiver_takeover_web_connection(struct web_client *w, struct receiver_state *rpt) {
    rpt->fd                = w->ifd;

    rpt->ssl.conn          = w->ssl.conn;
    rpt->ssl.state         = w->ssl.state;

    w->ssl = NETDATA_SSL_UNSET_CONNECTION;

    WEB_CLIENT_IS_DEAD(w);

    if(web_server_mode == WEB_SERVER_MODE_STATIC_THREADED) {
        web_client_flag_set(w, WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET);
    }
    else {
        if(w->ifd == w->ofd)
            w->ifd = w->ofd = -1;
        else
            w->ifd = -1;
    }

    buffer_flush(w->response.data);
}

void *rrdpush_receiver_thread(void *ptr);
int rrdpush_receiver_thread_spawn(struct web_client *w, char *decoded_query_string, void *h2o_ctx __maybe_unused) {

    if(!service_running(ABILITY_STREAMING_CONNECTIONS))
        return rrdpush_receiver_too_busy_now(w);

    struct receiver_state *rpt = callocz(1, sizeof(*rpt));
    rpt->connected_since_s = now_realtime_sec();
    rpt->last_msg_t = now_monotonic_sec();
    rpt->hops = 1;

    rpt->capabilities = STREAM_CAP_INVALID;

#ifdef ENABLE_H2O
    rpt->h2o_ctx = h2o_ctx;
#endif

    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_receivers, sizeof(*rpt), __ATOMIC_RELAXED);
    __atomic_add_fetch(&netdata_buffers_statistics.rrdhost_allocations_size, sizeof(struct rrdhost_system_info), __ATOMIC_RELAXED);

    rpt->system_info = callocz(1, sizeof(struct rrdhost_system_info));
    rpt->system_info->hops = rpt->hops;

    rpt->fd                = -1;
    rpt->client_ip         = strdupz(w->client_ip);
    rpt->client_port       = strdupz(w->client_port);

    rpt->ssl = NETDATA_SSL_UNSET_CONNECTION;

    rpt->config.update_every = default_rrd_update_every;

    // parse the parameters and fill rpt and rpt->system_info

    while(decoded_query_string) {
        char *value = strsep_skip_consecutive_separators(&decoded_query_string, "&");
        if(!value || !*value) continue;

        char *name = strsep_skip_consecutive_separators(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        if(!strcmp(name, "key") && !rpt->key)
            rpt->key = strdupz(value);

        else if(!strcmp(name, "hostname") && !rpt->hostname)
            rpt->hostname = strdupz(value);

        else if(!strcmp(name, "registry_hostname") && !rpt->registry_hostname)
            rpt->registry_hostname = strdupz(value);

        else if(!strcmp(name, "machine_guid") && !rpt->machine_guid)
            rpt->machine_guid = strdupz(value);

        else if(!strcmp(name, "update_every"))
            rpt->config.update_every = (int)strtoul(value, NULL, 0);

        else if(!strcmp(name, "os") && !rpt->os)
            rpt->os = strdupz(value);

        else if(!strcmp(name, "timezone") && !rpt->timezone)
            rpt->timezone = strdupz(value);

        else if(!strcmp(name, "abbrev_timezone") && !rpt->abbrev_timezone)
            rpt->abbrev_timezone = strdupz(value);

        else if(!strcmp(name, "utc_offset"))
            rpt->utc_offset = (int32_t)strtol(value, NULL, 0);

        else if(!strcmp(name, "hops"))
            rpt->hops = rpt->system_info->hops = (uint16_t) strtoul(value, NULL, 0);

        else if(!strcmp(name, "ml_capable"))
            rpt->system_info->ml_capable = strtoul(value, NULL, 0);

        else if(!strcmp(name, "ml_enabled"))
            rpt->system_info->ml_enabled = strtoul(value, NULL, 0);

        else if(!strcmp(name, "mc_version"))
            rpt->system_info->mc_version = strtoul(value, NULL, 0);

        else if(!strcmp(name, "ver") && (rpt->capabilities & STREAM_CAP_INVALID))
            rpt->capabilities = convert_stream_version_to_capabilities(strtoul(value, NULL, 0), NULL, false);

        else {
            // An old Netdata child does not have a compatible streaming protocol, map to something sane.
            if (!strcmp(name, "NETDATA_SYSTEM_OS_NAME"))
                name = "NETDATA_HOST_OS_NAME";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_ID"))
                name = "NETDATA_HOST_OS_ID";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_ID_LIKE"))
                name = "NETDATA_HOST_OS_ID_LIKE";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_VERSION"))
                name = "NETDATA_HOST_OS_VERSION";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_VERSION_ID"))
                name = "NETDATA_HOST_OS_VERSION_ID";

            else if (!strcmp(name, "NETDATA_SYSTEM_OS_DETECTION"))
                name = "NETDATA_HOST_OS_DETECTION";

            else if(!strcmp(name, "NETDATA_PROTOCOL_VERSION") && (rpt->capabilities & STREAM_CAP_INVALID))
                rpt->capabilities = convert_stream_version_to_capabilities(1, NULL, false);

            if (unlikely(rrdhost_set_system_info_variable(rpt->system_info, name, value))) {
                nd_log_daemon(NDLP_NOTICE, "STREAM '%s' [receive from [%s]:%s]: "
                     "request has parameter '%s' = '%s', which is not used."
                     , (rpt->hostname && *rpt->hostname) ? rpt->hostname : "-"
                     , rpt->client_ip, rpt->client_port
                     , name, value);
            }
        }
    }

    if (rpt->capabilities & STREAM_CAP_INVALID)
        // no version is supplied, assume version 0;
        rpt->capabilities = convert_stream_version_to_capabilities(0, NULL, false);

    // find the program name and version
    if(w->user_agent && w->user_agent[0]) {
        char *t = strchr(w->user_agent, '/');
        if(t && *t) {
            *t = '\0';
            t++;
        }

        rpt->program_name = strdupz(w->user_agent);
        if(t && *t) rpt->program_version = strdupz(t);
    }

    // check if we should accept this connection

    if(!rpt->key || !*rpt->key) {
        rrdpush_receive_log_status(
                rpt, "request without an API key, rejecting connection",
                RRDPUSH_STATUS_NO_API_KEY, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!rpt->hostname || !*rpt->hostname) {
        rrdpush_receive_log_status(
                rpt, "request without a hostname, rejecting connection",
                RRDPUSH_STATUS_NO_HOSTNAME, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!rpt->registry_hostname)
        rpt->registry_hostname = strdupz(rpt->hostname);

    if(!rpt->machine_guid || !*rpt->machine_guid) {
        rrdpush_receive_log_status(
                rpt, "request without a machine GUID, rejecting connection",
                RRDPUSH_STATUS_NO_MACHINE_GUID, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        char buf[GUID_LEN + 1];

        if (regenerate_guid(rpt->key, buf) == -1) {
            rrdpush_receive_log_status(
                    rpt, "API key is not a valid UUID (use the command uuidgen to generate one)",
                    RRDPUSH_STATUS_INVALID_API_KEY, NDLP_WARNING);

            receiver_state_free(rpt);
            return rrdpush_receiver_permission_denied(w);
        }

        if (regenerate_guid(rpt->machine_guid, buf) == -1) {
            rrdpush_receive_log_status(
                    rpt, "machine GUID is not a valid UUID",
                    RRDPUSH_STATUS_INVALID_MACHINE_GUID, NDLP_WARNING);

            receiver_state_free(rpt);
            return rrdpush_receiver_permission_denied(w);
        }
    }

    const char *api_key_type = appconfig_get(&stream_config, rpt->key, "type", "api");
    if(!api_key_type || !*api_key_type) api_key_type = "unknown";
    if(strcmp(api_key_type, "api") != 0) {
        rrdpush_receive_log_status(
                rpt, "API key is a machine GUID",
                RRDPUSH_STATUS_INVALID_API_KEY, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    if(!appconfig_get_boolean(&stream_config, rpt->key, "enabled", 0)) {
        rrdpush_receive_log_status(
                rpt, "API key is not enabled",
                RRDPUSH_STATUS_API_KEY_DISABLED, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *key_allow_from = simple_pattern_create(
                appconfig_get(&stream_config, rpt->key, "allow from", "*"),
                NULL, SIMPLE_PATTERN_EXACT, true);

        if(key_allow_from) {
            if(!simple_pattern_matches(key_allow_from, w->client_ip)) {
                simple_pattern_free(key_allow_from);

                rrdpush_receive_log_status(
                        rpt, "API key is not allowed from this IP",
                        RRDPUSH_STATUS_NOT_ALLOWED_IP, NDLP_WARNING);

                receiver_state_free(rpt);
                return rrdpush_receiver_permission_denied(w);
            }

            simple_pattern_free(key_allow_from);
        }
    }

    {
        const char *machine_guid_type = appconfig_get(&stream_config, rpt->machine_guid, "type", "machine");
        if (!machine_guid_type || !*machine_guid_type) machine_guid_type = "unknown";

        if (strcmp(machine_guid_type, "machine") != 0) {
            rrdpush_receive_log_status(
                    rpt, "machine GUID is an API key",
                    RRDPUSH_STATUS_INVALID_MACHINE_GUID, NDLP_WARNING);

            receiver_state_free(rpt);
            return rrdpush_receiver_permission_denied(w);
        }
    }

    if(!appconfig_get_boolean(&stream_config, rpt->machine_guid, "enabled", 1)) {
        rrdpush_receive_log_status(
                rpt, "machine GUID is not enabled",
                RRDPUSH_STATUS_MACHINE_GUID_DISABLED, NDLP_WARNING);

        receiver_state_free(rpt);
        return rrdpush_receiver_permission_denied(w);
    }

    {
        SIMPLE_PATTERN *machine_allow_from = simple_pattern_create(
                appconfig_get(&stream_config, rpt->machine_guid, "allow from", "*"),
                NULL, SIMPLE_PATTERN_EXACT, true);

        if(machine_allow_from) {
            if(!simple_pattern_matches(machine_allow_from, w->client_ip)) {
                simple_pattern_free(machine_allow_from);

                rrdpush_receive_log_status(
                        rpt, "machine GUID is not allowed from this IP",
                        RRDPUSH_STATUS_NOT_ALLOWED_IP, NDLP_WARNING);

                receiver_state_free(rpt);
                return rrdpush_receiver_permission_denied(w);
            }

            simple_pattern_free(machine_allow_from);
        }
    }

    if (strcmp(rpt->machine_guid, localhost->machine_guid) == 0) {

        rrdpush_receiver_takeover_web_connection(w, rpt);

        rrdpush_receive_log_status(
                rpt, "machine GUID is my own",
                RRDPUSH_STATUS_LOCALHOST, NDLP_DEBUG);

        char initial_response[HTTP_HEADER_SIZE + 1];
        snprintfz(initial_response, HTTP_HEADER_SIZE, "%s", START_STREAMING_ERROR_SAME_LOCALHOST);

        if(send_timeout(
                &rpt->ssl,
                rpt->fd, initial_response, strlen(initial_response), 0, 60) != (ssize_t)strlen(initial_response)) {

            nd_log_daemon(NDLP_ERR, "STREAM '%s' [receive from [%s]:%s]: "
                                    "failed to reply."
                                    , rpt->hostname
                                    , rpt->client_ip, rpt->client_port
                                    );
        }

        receiver_state_free(rpt);
        return HTTP_RESP_OK;
    }

    if(unlikely(web_client_streaming_rate_t > 0)) {
        static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
        static time_t last_stream_accepted_t = 0;

        time_t now = now_realtime_sec();
        spinlock_lock(&spinlock);

        if(unlikely(last_stream_accepted_t == 0))
            last_stream_accepted_t = now;

        if(now - last_stream_accepted_t < web_client_streaming_rate_t) {
            spinlock_unlock(&spinlock);

            char msg[100 + 1];
            snprintfz(msg, sizeof(msg) - 1,
                      "rate limit, will accept new connection in %ld secs",
                      (long)(web_client_streaming_rate_t - (now - last_stream_accepted_t)));

            rrdpush_receive_log_status(
                    rpt, msg,
                    RRDPUSH_STATUS_RATE_LIMIT, NDLP_NOTICE);

            receiver_state_free(rpt);
            return rrdpush_receiver_too_busy_now(w);
        }

        last_stream_accepted_t = now;
        spinlock_unlock(&spinlock);
    }

    /*
     * Quick path for rejecting multiple connections. The lock taken is fine-grained - it only protects the receiver
     * pointer within the host (if a host exists). This protects against multiple concurrent web requests hitting
     * separate threads within the web-server and landing here. The lock guards the thread-shutdown sequence that
     * detaches the receiver from the host. If the host is being created (first time-access) then we also use the
     * lock to prevent race-hazard (two threads try to create the host concurrently, one wins and the other does a
     * lookup to the now-attached structure).
     */

    {
        time_t age = 0;
        bool receiver_stale = false;
        bool receiver_working = false;

        rrd_rdlock();
        RRDHOST *host = rrdhost_find_by_guid(rpt->machine_guid);
        if (unlikely(host && rrdhost_flag_check(host, RRDHOST_FLAG_ARCHIVED))) /* Ignore archived hosts. */
            host = NULL;

        if (host) {
            spinlock_lock(&host->receiver_lock);
            if (host->receiver) {
                age = now_monotonic_sec() - host->receiver->last_msg_t;

                if (age < 30)
                    receiver_working = true;
                else
                    receiver_stale = true;
            }
            spinlock_unlock(&host->receiver_lock);
        }
        rrd_rdunlock();

        if (receiver_stale && stop_streaming_receiver(host, STREAM_HANDSHAKE_DISCONNECT_STALE_RECEIVER)) {
            // we stopped the receiver
            // we can proceed with this connection
            receiver_stale = false;

            nd_log_daemon(NDLP_NOTICE, "STREAM '%s' [receive from [%s]:%s]: "
                                       "stopped previous stale receiver to accept this one."
                                       , rpt->hostname
                                       , rpt->client_ip, rpt->client_port
                                       );
        }

        if (receiver_working || receiver_stale) {
            // another receiver is already connected
            // try again later

            char msg[200 + 1];
            snprintfz(msg, sizeof(msg) - 1,
                      "multiple connections for same host, "
                      "old connection was last used %ld secs ago%s",
                      age, receiver_stale ? " (signaled old receiver to stop)" : " (new connection not accepted)");

            rrdpush_receive_log_status(
                    rpt, msg,
                    RRDPUSH_STATUS_ALREADY_CONNECTED, NDLP_DEBUG);

            // Have not set WEB_CLIENT_FLAG_DONT_CLOSE_SOCKET - caller should clean up
            buffer_flush(w->response.data);
            buffer_strcat(w->response.data, START_STREAMING_ERROR_ALREADY_STREAMING);
            receiver_state_free(rpt);
            return HTTP_RESP_CONFLICT;
        }
    }

    rrdpush_receiver_takeover_web_connection(w, rpt);

    char tag[NETDATA_THREAD_TAG_MAX + 1];
    snprintfz(tag, NETDATA_THREAD_TAG_MAX, THREAD_TAG_STREAM_RECEIVER "[%s]", rpt->hostname);
    tag[NETDATA_THREAD_TAG_MAX] = '\0';

    rpt->thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, rrdpush_receiver_thread, (void *)rpt);
    if(!rpt->thread) {
        rrdpush_receive_log_status(
                rpt, "can't create receiver thread",
                RRDPUSH_STATUS_INTERNAL_SERVER_ERROR, NDLP_ERR);

        buffer_flush(w->response.data);
        buffer_strcat(w->response.data, "Can't handle this request");
        receiver_state_free(rpt);
        return HTTP_RESP_INTERNAL_SERVER_ERROR;
    }

    // prevent the caller from closing the streaming socket
    return HTTP_RESP_OK;
}

void rrdpush_reset_destinations_postpone_time(RRDHOST *host) {
    uint32_t wait = (host->sender) ? host->sender->reconnect_delay : 5;
    time_t now = now_realtime_sec();
    for (struct rrdpush_destinations *d = host->destinations; d; d = d->next)
        d->postpone_reconnection_until = now + wait;
}

static struct {
    STREAM_HANDSHAKE err;
    const char *str;
} handshake_errors[] = {
    { STREAM_HANDSHAKE_OK_V3, "CONNECTED" },
    { STREAM_HANDSHAKE_OK_V2, "CONNECTED" },
    { STREAM_HANDSHAKE_OK_V1, "CONNECTED" },
    { STREAM_HANDSHAKE_NEVER, "" },
    { STREAM_HANDSHAKE_ERROR_BAD_HANDSHAKE, "BAD HANDSHAKE" },
    { STREAM_HANDSHAKE_ERROR_LOCALHOST, "LOCALHOST" },
    { STREAM_HANDSHAKE_ERROR_ALREADY_CONNECTED, "ALREADY CONNECTED" },
    { STREAM_HANDSHAKE_ERROR_DENIED, "DENIED" },
    { STREAM_HANDSHAKE_ERROR_SEND_TIMEOUT, "SEND TIMEOUT" },
    { STREAM_HANDSHAKE_ERROR_RECEIVE_TIMEOUT, "RECEIVE TIMEOUT" },
    { STREAM_HANDSHAKE_ERROR_INVALID_CERTIFICATE, "INVALID CERTIFICATE" },
    { STREAM_HANDSHAKE_ERROR_SSL_ERROR, "SSL ERROR" },
    { STREAM_HANDSHAKE_ERROR_CANT_CONNECT, "CANT CONNECT" },
    { STREAM_HANDSHAKE_BUSY_TRY_LATER, "BUSY TRY LATER" },
    { STREAM_HANDSHAKE_INTERNAL_ERROR, "INTERNAL ERROR" },
    { STREAM_HANDSHAKE_INITIALIZATION, "REMOTE IS INITIALIZING" },
    { STREAM_HANDSHAKE_DISCONNECT_HOST_CLEANUP, "DISCONNECTED HOST CLEANUP" },
    { STREAM_HANDSHAKE_DISCONNECT_STALE_RECEIVER, "DISCONNECTED STALE RECEIVER" },
    { STREAM_HANDSHAKE_DISCONNECT_SHUTDOWN, "DISCONNECTED SHUTDOWN REQUESTED" },
    { STREAM_HANDSHAKE_DISCONNECT_NETDATA_EXIT, "DISCONNECTED NETDATA EXIT" },
    { STREAM_HANDSHAKE_DISCONNECT_PARSER_EXIT, "DISCONNECTED PARSE ENDED" },
    {STREAM_HANDSHAKE_DISCONNECT_UNKNOWN_SOCKET_READ_ERROR, "DISCONNECTED UNKNOWN SOCKET READ ERROR" },
    { STREAM_HANDSHAKE_DISCONNECT_PARSER_FAILED, "DISCONNECTED PARSE ERROR" },
    { STREAM_HANDSHAKE_DISCONNECT_RECEIVER_LEFT, "DISCONNECTED RECEIVER LEFT" },
    { STREAM_HANDSHAKE_DISCONNECT_ORPHAN_HOST, "DISCONNECTED ORPHAN HOST" },
    { STREAM_HANDSHAKE_NON_STREAMABLE_HOST, "NON STREAMABLE HOST" },
    { STREAM_HANDSHAKE_DISCONNECT_NOT_SUFFICIENT_READ_BUFFER, "DISCONNECTED NOT SUFFICIENT READ BUFFER" },
    {STREAM_HANDSHAKE_DISCONNECT_SOCKET_EOF, "DISCONNECTED SOCKET EOF" },
    {STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_FAILED, "DISCONNECTED SOCKET READ FAILED" },
    {STREAM_HANDSHAKE_DISCONNECT_SOCKET_READ_TIMEOUT, "DISCONNECTED SOCKET READ TIMEOUT" },
    { 0, NULL },
};

const char *stream_handshake_error_to_string(STREAM_HANDSHAKE handshake_error) {
    if(handshake_error >= STREAM_HANDSHAKE_OK_V1)
        // handshake_error is the whole version / capabilities number
        return "CONNECTED";

    for(size_t i = 0; handshake_errors[i].str ; i++) {
        if(handshake_error == handshake_errors[i].err)
            return handshake_errors[i].str;
    }

    return "UNKNOWN";
}
