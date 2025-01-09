// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-sender-internals.h"

void stream_send_rrdset_metrics_v1(RRDSET_STREAM_BUFFER *rsb, RRDSET *st) {
    RRDHOST *host = st->rrdhost; (void)host;
    BUFFER *wb = rsb->wb;
    struct sender_state *s = host->sender; (void)s;
    RRDSET_FLAGS flags = rsb->rrdset_flags;

    buffer_fast_strcat(wb, PLUGINSD_KEYWORD_BEGIN " \"", 7);
    buffer_fast_strcat(wb, rrdset_id(st), string_strlen(st->id));
    buffer_fast_strcat(wb, "\" ", 2);

    if(st->last_collected_time.tv_sec > st->stream.snd.resync_time_s)
        buffer_print_uint64(wb, st->usec_since_last_update);
    else
        buffer_fast_strcat(wb, "0", 1);

    buffer_fast_strcat(wb, "\n", 1);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(unlikely(!rrddim_check_updated(rd)))
            continue;

        if(likely(rrddim_check_upstream_exposed_collector(rd))) {
            buffer_fast_strcat(wb, PLUGINSD_KEYWORD_SET " \"", 5);
            buffer_fast_strcat(wb, rrddim_id(rd), string_strlen(rd->id));
            buffer_fast_strcat(wb, "\" = ", 4);
            buffer_print_int64(wb, rd->collector.collected_value);
            buffer_fast_strcat(wb, "\n", 1);
        }
        else {
            internal_error(true, "STREAM SND '%s': 'chart:%s/dim:%s' flag 'exposed' is updated but not exposed",
                           rrdhost_hostname(st->rrdhost), rrdset_id(st), rrddim_id(rd));
            // we will include it in the next iteration
            rrddim_metadata_updated(rd);
        }
    }
    rrddim_foreach_done(rd);

    if(unlikely(flags & RRDSET_FLAG_UPSTREAM_SEND_VARIABLES))
        rrdvar_print_to_streaming_custom_chart_variables(st, wb);

    buffer_fast_strcat(wb, PLUGINSD_KEYWORD_END "\n", 4);
}
