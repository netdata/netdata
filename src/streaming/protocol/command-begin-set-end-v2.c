// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-sender-internals.h"
#include "plugins.d/pluginsd_internals.h"

void stream_send_rrddim_metrics_v2(RRDSET_STREAM_BUFFER *rsb, RRDDIM *rd, usec_t point_end_time_ut, NETDATA_DOUBLE n, SN_FLAGS flags) {
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
            buffer_print_uint64_encoded(wb, integer_encoding, rd->rrdset->stream.snd.chart_slot);
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
        buffer_print_uint64_encoded(wb, integer_encoding, rd->stream.snd.dim_slot);
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

ALWAYS_INLINE void stream_send_rrdset_metrics_finished(RRDSET_STREAM_BUFFER *rsb, RRDSET *st) {
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

