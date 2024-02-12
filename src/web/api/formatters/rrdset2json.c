// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdset2json.h"

static int process_label_callback(const char *name, const char *value, RRDLABEL_SRC ls __maybe_unused, void *data) {
    BUFFER *wb = data;
    buffer_json_member_add_string_or_empty(wb, name,  value);
    return 1;
}

void chart_labels2json(RRDSET *st, BUFFER *wb)
{
    if(unlikely(!st->rrdlabels))
        return;

    rrdlabels_walkthrough_read(st->rrdlabels, process_label_callback, wb);
}

// generate JSON for the /api/v1/chart API call
void rrdset2json(RRDSET *st, BUFFER *wb, size_t *dimensions_count, size_t *memory_used)
{
    time_t first_entry_t = rrdset_first_entry_s(st);
    time_t last_entry_t  = rrdset_last_entry_s(st);
    char buf[RRD_ID_LENGTH_MAX + 16];

    buffer_json_member_add_string(wb, "id", rrdset_id(st));
    buffer_json_member_add_string(wb, "name", rrdset_name(st));
    buffer_json_member_add_string(wb, "type", rrdset_parts_type(st));
    buffer_json_member_add_string(wb, "family", rrdset_family(st));
    buffer_json_member_add_string(wb, "context", rrdset_context(st));
    snprintfz(buf, RRD_ID_LENGTH_MAX + 15, "%s (%s)", rrdset_title(st), rrdset_name(st));
    buffer_json_member_add_string(wb, "title", buf);
    buffer_json_member_add_int64(wb, "priority", st->priority);
    buffer_json_member_add_string(wb, "plugin", rrdset_plugin_name(st));
    buffer_json_member_add_string(wb, "module", rrdset_module_name(st));
    buffer_json_member_add_string(wb, "units", rrdset_units(st));

    snprintfz(buf, RRD_ID_LENGTH_MAX + 15, "/api/v1/data?chart=%s", rrdset_name(st));
    buffer_json_member_add_string(wb, "data_url", buf);

    buffer_json_member_add_string(wb, "chart_type", rrdset_type_name(st->chart_type));
    buffer_json_member_add_int64(wb, "duration", (int64_t)(last_entry_t - first_entry_t + st->update_every));
    buffer_json_member_add_int64(wb, "first_entry", (int64_t)first_entry_t);
    buffer_json_member_add_int64(wb, "last_entry", (int64_t)last_entry_t);
    buffer_json_member_add_int64(wb, "update_every", (int64_t)st->update_every);

    unsigned long memory = sizeof(RRDSET);

    size_t dimensions = 0;
    buffer_json_member_add_object(wb, "dimensions");
    {
        RRDDIM *rd;
        rrddim_foreach_read(rd, st)
        {
            if (rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN) || rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE))
                continue;

            memory += rrddim_size() + rd->db.memsize;

            buffer_json_member_add_object(wb, rrddim_id(rd));
            buffer_json_member_add_string(wb, "name", rrddim_name(rd));
            buffer_json_object_close(wb);

            dimensions++;
        }
        rrddim_foreach_done(rd);
    }
    buffer_json_object_close(wb);

    if(dimensions_count) *dimensions_count += dimensions;
    if(memory_used) *memory_used += memory;

    buffer_json_member_add_object(wb, "chart_variables");
    health_api_v1_chart_custom_variables2json(st, wb);
    buffer_json_object_close(wb);

    buffer_json_member_add_double(wb, "green", st->green);
    buffer_json_member_add_double(wb, "red", st->red);

    {
        buffer_json_member_add_object(wb, "alarms");
        RRDCALC *rc;
        rw_spinlock_read_lock(&st->alerts.spinlock);
        DOUBLE_LINKED_LIST_FOREACH_FORWARD(st->alerts.base, rc, prev, next)
        {
            {
                buffer_json_member_add_object(wb, rrdcalc_name(rc));
                buffer_json_member_add_string_or_empty(wb, "id", rrdcalc_name(rc));
                buffer_json_member_add_string_or_empty(wb, "status", rrdcalc_status2string(rc->status));
                buffer_json_member_add_string_or_empty(wb, "units", rrdcalc_units(rc));
                buffer_json_member_add_int64(wb, "duration", (int64_t)rc->config.update_every);
                buffer_json_object_close(wb);
            }
        }
        rw_spinlock_read_unlock(&st->alerts.spinlock);
        buffer_json_object_close(wb);
    }

    buffer_json_member_add_object(wb, "chart_labels");
    chart_labels2json(st, wb);
    buffer_json_object_close(wb);

    buffer_json_member_add_object(wb, "functions");
    chart_functions2json(st, wb);
    buffer_json_object_close(wb);
}
