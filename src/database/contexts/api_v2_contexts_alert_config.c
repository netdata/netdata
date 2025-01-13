// SPDX-License-Identifier: GPL-3.0-or-later

#include "api_v2_contexts_alerts.h"

void contexts_v2_alert_config_to_json_from_sql_alert_config_data(struct sql_alert_config_data *t, void *data) {
    struct alert_transitions_callback_data *d = data;
    BUFFER *wb = d->wb;
    bool debug = d->debug;
    d->configs_added++;

    if(d->only_one_config)
        buffer_json_add_array_item_object(wb); // alert config

    {
        buffer_json_member_add_string(wb, "name", t->name);
        buffer_json_member_add_uuid_ptr(wb, "config_hash_id", t->config_hash_id);

        buffer_json_member_add_object(wb, "selectors");
        {
            bool is_template = t->selectors.on_template && *t->selectors.on_template ? true : false;
            buffer_json_member_add_string(wb, "type", is_template ? "template" : "alarm");
            buffer_json_member_add_string(wb, "on", is_template ? t->selectors.on_template : t->selectors.on_key);

            buffer_json_member_add_string(wb, "families", t->selectors.families);
            buffer_json_member_add_string(wb, "host_labels", t->selectors.host_labels);
            buffer_json_member_add_string(wb, "chart_labels", t->selectors.chart_labels);
        }
        buffer_json_object_close(wb); // selectors

        buffer_json_member_add_object(wb, "value"); // value
        {
            // buffer_json_member_add_string(wb, "every", t->value.every); // does not exist in Netdata Cloud
            buffer_json_member_add_string(wb, "units", t->value.units);
            buffer_json_member_add_uint64(wb, "update_every", t->value.update_every);

            if (t->value.db.after || debug) {
                buffer_json_member_add_object(wb, "db");
                {
                    // buffer_json_member_add_string(wb, "lookup", t->value.db.lookup); // does not exist in Netdata Cloud

                    buffer_json_member_add_time_t(wb, "after", t->value.db.after);
                    buffer_json_member_add_time_t(wb, "before", t->value.db.before);
                    buffer_json_member_add_string(wb, "time_group_condition", alerts_group_conditions_id2txt(t->value.db.time_group_condition));
                    buffer_json_member_add_double(wb, "time_group_value", t->value.db.time_group_value);
                    buffer_json_member_add_string(wb, "dims_group", alerts_dims_grouping_id2group(t->value.db.dims_group));
                    buffer_json_member_add_string(wb, "data_source", alerts_data_source_id2source(t->value.db.data_source));
                    buffer_json_member_add_string(wb, "method", t->value.db.method);
                    buffer_json_member_add_string(wb, "dimensions", t->value.db.dimensions);
                    rrdr_options_to_buffer_json_array(wb, "options", (RRDR_OPTIONS)t->value.db.options);
                }
                buffer_json_object_close(wb); // db
            }

            if (t->value.calc || debug)
                buffer_json_member_add_string(wb, "calc", t->value.calc);
        }
        buffer_json_object_close(wb); // value

        if (t->status.warn || t->status.crit || debug) {
            buffer_json_member_add_object(wb, "status"); // status
            {
                NETDATA_DOUBLE green = t->status.green ? str2ndd(t->status.green, NULL) : NAN;
                NETDATA_DOUBLE red = t->status.red ? str2ndd(t->status.red, NULL) : NAN;

                if (!isnan(green) || debug)
                    buffer_json_member_add_double(wb, "green", green);

                if (!isnan(red) || debug)
                    buffer_json_member_add_double(wb, "red", red);

                if (t->status.warn || debug)
                    buffer_json_member_add_string(wb, "warn", t->status.warn);

                if (t->status.crit || debug)
                    buffer_json_member_add_string(wb, "crit", t->status.crit);
            }
            buffer_json_object_close(wb); // status
        }

        buffer_json_member_add_object(wb, "notification");
        {
            buffer_json_member_add_string(wb, "type", "agent");
            buffer_json_member_add_string(wb, "exec", t->notification.exec ? t->notification.exec : NULL);
            buffer_json_member_add_string(wb, "to", t->notification.to_key ? t->notification.to_key : string2str(localhost->health.default_recipient));
            buffer_json_member_add_string(wb, "delay", t->notification.delay);
            buffer_json_member_add_string(wb, "repeat", t->notification.repeat);
            buffer_json_member_add_string(wb, "options", t->notification.options);
        }
        buffer_json_object_close(wb); // notification

        buffer_json_member_add_string(wb, "class", t->classification);
        buffer_json_member_add_string(wb, "component", t->component);
        buffer_json_member_add_string(wb, "type", t->type);
        buffer_json_member_add_string(wb, "info", t->info);
        buffer_json_member_add_string(wb, "summary", t->summary);
        // buffer_json_member_add_string(wb, "source", t->source); // moved to alert instance
    }

    if(d->only_one_config)
        buffer_json_object_close(wb);
}

int contexts_v2_alert_config_to_json(struct web_client *w, const char *config_hash_id) {
    struct alert_transitions_callback_data data = {
        .wb = w->response.data,
        .debug = false,
        .only_one_config = false,
    };
    DICTIONARY *configs = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_set(configs, config_hash_id, NULL, 0);

    buffer_flush(w->response.data);

    buffer_json_initialize(w->response.data, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    int added = sql_get_alert_configuration(configs, contexts_v2_alert_config_to_json_from_sql_alert_config_data, &data, false);
    buffer_json_finalize(w->response.data);

    int ret = HTTP_RESP_OK;

    if(added <= 0) {
        buffer_flush(w->response.data);
        w->response.data->content_type = CT_TEXT_PLAIN;
        if(added < 0) {
            buffer_strcat(w->response.data, "Failed to execute SQL query.");
            ret = HTTP_RESP_INTERNAL_SERVER_ERROR;
        }
        else {
            buffer_strcat(w->response.data, "Config is not found.");
            ret = HTTP_RESP_NOT_FOUND;
        }
    }

    dictionary_destroy(configs);
    return ret;
}
