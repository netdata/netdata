// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-sender-internals.h"
#include "plugins.d/pluginsd_internals.h"

// chart labels
static int stream_send_clabels_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    BUFFER *wb = (BUFFER *)data;
    buffer_sprintf(wb, PLUGINSD_KEYWORD_CLABEL " \"%s\" \"%s\" %d\n", name, value, ls & ~(RRDLABEL_FLAG_INTERNAL));
    return 1;
}

static void stream_send_clabels(BUFFER *wb, RRDSET *st) {
    if (st->rrdlabels) {
        if(rrdlabels_walkthrough_read(st->rrdlabels, stream_send_clabels_callback, wb) > 0)
            buffer_sprintf(wb, PLUGINSD_KEYWORD_CLABEL_COMMIT "\n");
    }
}

// Send the current chart definition.
// Assumes that collector thread has already called sender_start for mutex / buffer state.
bool stream_sender_send_rrdset_definition(BUFFER *wb, RRDSET *st) {
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
        buffer_print_uint64_encoded(wb, integer_encoding, st->stream.snd.chart_slot);
    }

    // send the chart
    buffer_sprintf(
        wb
        , " \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" \"%s\" %d %d \"%s %s %s\" \"%s\" \"%s\"\n"
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
        , rrdset_flag_check(st, RRDSET_FLAG_STORE_FIRST)?"store_first":""
        , rrdset_flag_check(st, RRDSET_FLAG_HIDDEN)?"hidden":""
        , rrdset_plugin_name(st)
        , rrdset_module_name(st)
    );

    // send the chart labels
    if (stream_has_capability(host->sender, STREAM_CAP_CLABELS))
        stream_send_clabels(wb, st);

    // send the dimensions
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        buffer_fast_strcat(wb, PLUGINSD_KEYWORD_DIMENSION, sizeof(PLUGINSD_KEYWORD_DIMENSION) - 1);

        if(with_slots) {
            buffer_fast_strcat(wb, " "PLUGINSD_KEYWORD_SLOT":", sizeof(PLUGINSD_KEYWORD_SLOT) - 1 + 2);
            buffer_print_uint64_encoded(wb, integer_encoding, rd->stream.snd.dim_slot);
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
        stream_sender_send_rrdset_functions(st, wb);

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

        RRDSET_FLAGS old = rrdset_flag_set_and_clear(st, RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS, RRDSET_FLAG_SENDER_REPLICATION_FINISHED);
        if(!(old & RRDSET_FLAG_SENDER_REPLICATION_IN_PROGRESS)) {
            if(rrdhost_sender_replicating_charts_plus_one(st->rrdhost) == 1)
                pulse_host_status(st->rrdhost, PULSE_HOST_STATUS_SND_REPLICATING, 0);
        }

        replication_progress = true;

#ifdef NETDATA_LOG_REPLICATION_REQUESTS
        internal_error(true, "REPLAY: 'host:%s/chart:%s' replication starts",
                       rrdhost_hostname(st->rrdhost), rrdset_id(st));
#endif
    }

    // we can set the exposed flag, after we commit the buffer
    // because replication may pick it up prematurely
    rrddim_foreach_read(rd, st) {
        rrddim_metadata_exposed_upstream(rd, version);
    }
    rrddim_foreach_done(rd);
    rrdset_metadata_exposed_upstream(st, version);

    st->stream.snd.resync_time_s = st->last_collected_time.tv_sec + (stream_send.initial_clock_resync_iterations * st->update_every);
    return replication_progress;
}

bool should_send_rrdset_matching(RRDSET *st, RRDSET_FLAGS flags) {
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

            r = simple_pattern_matches_string_extract(host->stream.snd.charts_matching, st->context, NULL, 0);
            if(r == SP_MATCHED_POSITIVE) positive++;
            else if(r == SP_MATCHED_NEGATIVE) negative++;

            if(!negative) {
                r = simple_pattern_matches_string_extract(host->stream.snd.charts_matching, st->name, NULL, 0);
                if (r == SP_MATCHED_POSITIVE) positive++;
                else if (r == SP_MATCHED_NEGATIVE) negative++;
            }

            if(!negative) {
                r = simple_pattern_matches_string_extract(host->stream.snd.charts_matching, st->id, NULL, 0);
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

// Called from the internal collectors to mark a chart obsolete.
bool stream_sender_send_rrdset_definition_now(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    if(unlikely(!rrdhost_can_stream_metadata_to_parent(host) || !should_send_rrdset_matching(st, rrdset_flag_get(st))))
        return false;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    stream_sender_send_rrdset_definition(wb, st);
    sender_commit_clean_buffer(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);

    return true;
}
