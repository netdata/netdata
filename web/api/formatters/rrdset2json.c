// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdset2json.h"

#ifdef ENABLE_JSONC
#ifdef ACLK_LEGACY
void rrdset2json(RRDSET *st, BUFFER *wb, size_t *dimensions_count, size_t *memory_used, int skip_volatile) {
    json_object *j = rrdset_json(st, dimensions_count, memory_used, skip_volatile);
    buffer_strcat(wb, json_object_to_json_string_ext(j, JSON_C_TO_STRING_PLAIN));
    json_object_put(j);
}
#endif
#else /* !defined(ENABLE_JSONC) */
void chart_labels2json(RRDSET *st, BUFFER *wb, size_t indentation)
{
    if(unlikely(!st->rrdlabels))
        return;

    char tabs[11];

    if (indentation > 10)
        indentation = 10;

    tabs[0] = '\0';
    while (indentation) {
        strcat(tabs, "\t\t");
        indentation--;
    }

    rrdlabels_to_buffer(st->rrdlabels, wb, tabs, ":", "\"", ",\n", NULL, NULL, NULL, NULL);
    buffer_strcat(wb, "\n");
}

// generate JSON for the /api/v1/chart API call

void rrdset2json(RRDSET *st, BUFFER *wb, size_t *dimensions_count, size_t *memory_used, int skip_volatile) {
    time_t first_entry_t = rrdset_first_entry_s(st);
    time_t last_entry_t  = rrdset_last_entry_s(st);

    buffer_sprintf(
        wb,
        "\t\t{\n"
        "\t\t\t\"id\": \"%s\",\n"
        "\t\t\t\"name\": \"%s\",\n"
        "\t\t\t\"type\": \"%s\",\n"
        "\t\t\t\"family\": \"%s\",\n"
        "\t\t\t\"context\": \"%s\",\n"
        "\t\t\t\"title\": \"%s (%s)\",\n"
        "\t\t\t\"priority\": %ld,\n"
        "\t\t\t\"plugin\": \"%s\",\n"
        "\t\t\t\"module\": \"%s\",\n"
        "\t\t\t\"units\": \"%s\",\n"
        "\t\t\t\"data_url\": \"/api/v1/data?chart=%s\",\n"
        "\t\t\t\"chart_type\": \"%s\",\n",
        rrdset_id(st),
        rrdset_name(st),
        rrdset_parts_type(st),
        rrdset_family(st),
        rrdset_context(st),
        rrdset_title(st),
        rrdset_name(st),
        st->priority,
        rrdset_plugin_name(st),
        rrdset_module_name(st),
        rrdset_units(st),
        rrdset_name(st),
        rrdset_type_name(st->chart_type));

    if (likely(!skip_volatile))
        buffer_sprintf(
            wb,
            "\t\t\t\"duration\": %"PRId64",\n",
            (int64_t)(last_entry_t - first_entry_t + st->update_every) //st->entries * st->update_every
        );

    buffer_sprintf(
        wb,
        "\t\t\t\"first_entry\": %"PRId64",\n",
        (int64_t)first_entry_t //rrdset_first_entry_t(st)
    );

    if (likely(!skip_volatile))
        buffer_sprintf(
            wb,
            "\t\t\t\"last_entry\": %"PRId64",\n",
            (int64_t)last_entry_t //rrdset_last_entry_t(st)
        );

    buffer_sprintf(
        wb,
        "\t\t\t\"update_every\": %d,\n"
        "\t\t\t\"dimensions\": {\n",
        st->update_every);

    unsigned long memory = sizeof(RRDSET);

    size_t dimensions = 0;
    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        if(rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN) || rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) continue;

        memory += sizeof(RRDDIM) + rd->memsize;

        if (dimensions)
            buffer_strcat(wb, ",\n\t\t\t\t\"");
        else
            buffer_strcat(wb, "\t\t\t\t\"");
        buffer_strcat_jsonescape(wb, rrddim_id(rd));
        buffer_strcat(wb, "\": { \"name\": \"");
        buffer_strcat_jsonescape(wb, rrddim_name(rd));
        buffer_strcat(wb, "\" }");

        dimensions++;
    }
    rrddim_foreach_done(rd);

    if(dimensions_count) *dimensions_count += dimensions;
    if(memory_used) *memory_used += memory;

    buffer_sprintf(wb, "\n\t\t\t},\n\t\t\t\"chart_variables\": ");
    health_api_v1_chart_custom_variables2json(st, wb);

    buffer_strcat(wb, ",\n\t\t\t\"green\": ");
    buffer_rrd_value(wb, st->green);
    buffer_strcat(wb, ",\n\t\t\t\"red\": ");
    buffer_rrd_value(wb, st->red);

    if (likely(!skip_volatile)) {
        buffer_strcat(wb, ",\n\t\t\t\"alarms\": {\n");
        size_t alarms = 0;
        RRDCALC *rc;
        netdata_rwlock_rdlock(&st->alerts.rwlock);
        DOUBLE_LINKED_LIST_FOREACH_FORWARD(st->alerts.base, rc, prev, next) {
            buffer_sprintf(
                wb,
                "%s"
                "\t\t\t\t\"%s\": {\n"
                "\t\t\t\t\t\"id\": %u,\n"
                "\t\t\t\t\t\"status\": \"%s\",\n"
                "\t\t\t\t\t\"units\": \"%s\",\n"
                "\t\t\t\t\t\"update_every\": %d\n"
                "\t\t\t\t}",
                (alarms) ? ",\n" : "", rrdcalc_name(rc), rc->id, rrdcalc_status2string(rc->status), rrdcalc_units(rc),
                rc->update_every);

            alarms++;
        }
        netdata_rwlock_unlock(&st->alerts.rwlock);
        buffer_sprintf(wb,
                       "\n\t\t\t}"
        );
    }
    buffer_strcat(wb, ",\n\t\t\t\"chart_labels\": {\n");
    chart_labels2json(st, wb, 2);
    buffer_strcat(wb, "\t\t\t}");

    buffer_strcat(wb, ",\n\t\t\t\"functions\": {\n");
    chart_functions2json(st, wb, 4, "\"", "\"");
    buffer_strcat(wb, "\t\t\t}");

    buffer_sprintf(wb,
            "\n\t\t}"
    );
}
#endif /* ENABLE_JSONC */


#ifdef ENABLE_JSONC
#define JSON_ADD_STRING(name, str, obj)                                                                                \
    {                                                                                                                  \
        tmp = json_object_new_string(str);                                                                             \
        json_object_object_add(obj, name, tmp);                                                                        \
    }
#define JSON_ADD_INT64(name, val, obj)                                                                                 \
    {                                                                                                                  \
        tmp = json_object_new_int64(val);                                                                              \
        json_object_object_add(obj, name, tmp);                                                                        \
    }

#define JSON_ADD_INT(name, val, obj)                                                                                   \
    {                                                                                                                  \
        tmp = json_object_new_int(val);                                                                                \
        json_object_object_add(obj, name, tmp);                                                                        \
    }

json_object *chart_labels_json(RRDSET *st)
{
    struct label_index *labels = &st->state->labels;
    json_object *tmp, *j = json_object_new_object();

    netdata_rwlock_rdlock(&labels->labels_rwlock);
    for (struct label *label = labels->head; label; label = label->next)
        JSON_ADD_STRING(label->key, label->value, j)
    netdata_rwlock_unlock(&labels->labels_rwlock);
    return j;
}

extern json_object *rrdset_json(RRDSET *st, size_t *dimensions_count, size_t *memory_used, int skip_volatile)
{
    json_object *j = json_object_new_object();
    json_object *tmp;

    rrdset_rdlock(st);

    time_t first_entry_t = rrdset_first_entry_t_nolock(st);
    time_t last_entry_t  = rrdset_last_entry_t_nolock(st);

    JSON_ADD_STRING("id", st->id, j)
    JSON_ADD_STRING("name", st->name, j)
    JSON_ADD_STRING("type", st->type, j)
    JSON_ADD_STRING("family", st->family, j)
    JSON_ADD_STRING("context", st->context, j)

    BUFFER *buf = buffer_create(1024);
    buffer_sprintf(buf, "%s (%s)", st->title, st->name);
    JSON_ADD_STRING("title", buffer_tostring(buf), j)

    JSON_ADD_INT64("priority", st->priority, j)

    JSON_ADD_STRING("plugin", st->plugin_name ? st->plugin_name : "", j) // "" (empty string) instead of json null to keep API compat with legacy impl.
    JSON_ADD_STRING("module", st->module_name ? st->module_name : "", j) // "" (empty string) instead of json null to keep API compat with legacy impl.

    tmp = json_object_new_boolean(rrdset_flag_check(st, RRDSET_FLAG_ENABLED));
    json_object_object_add(j, "enabled", tmp);

    JSON_ADD_STRING("units", st->units, j)

    buffer_flush(buf);
    buffer_sprintf(buf, "/api/v1/data?chart=%s", st->name);
    tmp = json_object_new_string(buffer_tostring(buf));
    json_object_object_add(j, "data_url", tmp);

    JSON_ADD_STRING("chart_type", rrdset_type_name(st->chart_type), j)

    if (likely(!skip_volatile))
        JSON_ADD_INT64("duration", last_entry_t - first_entry_t + st->update_every, j); //st->entries * st->update_every
    
    JSON_ADD_INT64("first_entry", first_entry_t, j)

    if (likely(!skip_volatile))
        JSON_ADD_INT64("last_entry", last_entry_t, j)

    JSON_ADD_INT("update_every", st->update_every, j)

    json_object *obj = json_object_new_object();
    RRDDIM *rd;
    if (memory_used) *memory_used += st->memsize;
    rrddim_foreach_read(rd, st) {
        if (rrddim_flag_check(rd, RRDDIM_FLAG_HIDDEN) || rrddim_flag_check(rd, RRDDIM_FLAG_OBSOLETE)) continue;

        if (memory_used) *memory_used += rd->memsize;
        if (dimensions_count) (*dimensions_count)++;

        // to keep API same with legacy implementation
        // which also creates dictionary with fixed one item
        // funny but... (maybe /api/v2 in future :) )
        json_object *tmp_obj = json_object_new_object();
        JSON_ADD_STRING("name", rd->name, tmp_obj);
        json_object_object_add(obj, rd->id, tmp_obj);
    }
    json_object_object_add(j, "dimensions", obj);

    buffer_flush(buf);
    health_api_v1_chart_custom_variables2json(st, buf); // TODO jsonc-ify this
    tmp = json_tokener_parse(buffer_tostring(buf));
    json_object_object_add(j, "chart_variables", tmp);

    if (isnan(st->green) || isinf(st->green)) {
        json_object_object_add(j, "green", NULL);
    } else {
        buffer_flush(buf);
        buffer_rrd_value(buf, st->green);
        JSON_ADD_STRING("green", buffer_tostring(buf), j)
    }

    if (isnan(st->green) || isinf(st->green)) {
        json_object_object_add(j, "red", NULL);
    } else {
        buffer_flush(buf);
        buffer_rrd_value(buf, st->red);
        JSON_ADD_STRING("red", buffer_tostring(buf), j)
    }

    if (likely(!skip_volatile)) {
        obj = json_object_new_object();
        RRDCALC *rc;
        for (rc = st->alarms; rc; rc = rc->rrdset_next) {
            json_object *tmp_obj = json_object_new_object();
            JSON_ADD_INT("id", rc->id, tmp_obj)
            JSON_ADD_STRING("status", rrdcalc_status2string(rc->status), tmp_obj);
            JSON_ADD_STRING("units", rc->units, tmp_obj)
            JSON_ADD_INT("update_every", rc->update_every, tmp_obj)
        }
        json_object_object_add(j, "alarms", obj);
    }

    tmp = chart_labels_json(st);
    json_object_object_add(j, "chart_labels", tmp);

    rrdset_unlock(st);
    buffer_free(buf);
    return j;
}
#endif
