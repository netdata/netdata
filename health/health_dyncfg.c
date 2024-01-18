// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"

#define DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX "health:alert:prototype"

static inline void health_prototype_rule_to_json_array_member(BUFFER *wb, RRD_ALERT_PROTOTYPE *ap, bool for_hashing) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_object(wb, "match");
        {
            buffer_json_member_add_boolean(wb, "enabled", ap->match.enabled);
            buffer_json_member_add_boolean(wb, "template", ap->match.is_template);

            if(ap->match.is_template)
                buffer_json_member_add_string(wb, "on", string2str(ap->match.on.context));
            else
                buffer_json_member_add_string(wb, "on", string2str(ap->match.on.chart));

            buffer_json_member_add_string_or_empty(wb, "os", ap->match.os ? string2str(ap->match.os) : "*");
            buffer_json_member_add_string_or_empty(wb, "host", ap->match.host ? string2str(ap->match.host) : "*");
            buffer_json_member_add_string_or_empty(wb, "instances", ap->match.charts ? string2str(ap->match.charts) : "*");
            buffer_json_member_add_string_or_empty(wb, "plugin", ap->match.charts ? string2str(ap->match.plugin) : "*");
            buffer_json_member_add_string_or_empty(wb, "module", ap->match.module ? string2str(ap->match.module) : "*");
            buffer_json_member_add_string_or_empty(wb, "host_labels", ap->match.host_labels ? string2str(ap->match.host_labels) : "*");
            buffer_json_member_add_string_or_empty(wb, "instance_labels", ap->match.chart_labels ? string2str(ap->match.chart_labels) : "*");
        }
        buffer_json_object_close(wb); // match

        buffer_json_member_add_object(wb, "config");
        {
            if(!for_hashing) {
                buffer_json_member_add_uuid(wb, "hash", &ap->config.hash_id);
                buffer_json_member_add_string(wb, "source_type", dyncfg_id2source_type(ap->config.source_type));
                buffer_json_member_add_string(wb, "source", string2str(ap->config.source));
            }

            buffer_json_member_add_string(wb, "summary", string2str(ap->config.summary));
            buffer_json_member_add_string(wb, "info", string2str(ap->config.info));

            buffer_json_member_add_string(wb, "type", string2str(ap->config.type));
            buffer_json_member_add_string(wb, "component", string2str(ap->config.component));
            buffer_json_member_add_string(wb, "classification", string2str(ap->config.classification));

            buffer_json_member_add_object(wb, "value");
            {
                buffer_json_member_add_object(wb, "database_lookup");
                {
                    buffer_json_member_add_int64(wb, "after", ap->config.after);
                    buffer_json_member_add_int64(wb, "before", ap->config.before);
                    buffer_json_member_add_string(wb, "grouping", time_grouping_method2string(ap->config.group));
                    web_client_api_request_v1_rrdcalc_options_to_buffer_json_array(wb, "options", ap->config.options);
                    buffer_json_member_add_string(wb, "dimensions", string2str(ap->config.dimensions));
                }
                buffer_json_object_close(wb); // database lookup

                buffer_json_member_add_string(wb, "calculation", expression_source(ap->config.calculation));
                buffer_json_member_add_string(wb, "units", string2str(ap->config.units));
            }
            buffer_json_object_close(wb); // value

            buffer_json_member_add_object(wb, "conditions");
            {
                buffer_json_member_add_double(wb, "green", ap->config.green);
                buffer_json_member_add_double(wb, "red", ap->config.red);
                buffer_json_member_add_string(wb, "warning_condition", expression_source(ap->config.warning));
                buffer_json_member_add_string(wb, "critical_condition", expression_source(ap->config.critical));
            }
            buffer_json_object_close(wb); // conditions

            buffer_json_member_add_object(wb, "action");
            {
                buffer_json_member_add_string(wb, "execute", string2str(ap->config.exec));
                buffer_json_member_add_string(wb, "recipient", string2str(ap->config.recipient));

                buffer_json_member_add_object(wb, "delay");
                {
                    buffer_json_member_add_int64(wb, "up", ap->config.delay_up_duration);
                    buffer_json_member_add_int64(wb, "down", ap->config.delay_down_duration);
                    buffer_json_member_add_int64(wb, "max", ap->config.delay_max_duration);
                    buffer_json_member_add_double(wb, "multiplier", ap->config.delay_multiplier);
                }
                buffer_json_object_close(wb); // delay

                buffer_json_member_add_object(wb, "repeat");
                {
                    buffer_json_member_add_boolean(wb, "enabled", ap->config.has_custom_repeat_config);
                    buffer_json_member_add_uint64(wb, "warning", ap->config.has_custom_repeat_config ? ap->config.warn_repeat_every : 0);
                    buffer_json_member_add_uint64(wb, "critical", ap->config.has_custom_repeat_config ? ap->config.crit_repeat_every : 0);
                }
                buffer_json_object_close(wb); // repeat
            }
            buffer_json_object_close(wb); // action
        }
        buffer_json_object_close(wb); // match
    }
    buffer_json_object_close(wb); // array item
}

void health_prototype_to_json(BUFFER *wb, RRD_ALERT_PROTOTYPE *ap, bool for_hashing) {
    buffer_flush(wb);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_string(wb, "name", string2str(ap->config.name));
    buffer_json_member_add_array(wb, "rules");
    {
        for(RRD_ALERT_PROTOTYPE *t = ap; t ; t = t->_internal.next)
            health_prototype_rule_to_json_array_member(wb, t, for_hashing);
    }
    buffer_json_array_close(wb); // rules
    buffer_json_finalize(wb);
}

// ---------------------------------------------------------------------------------------------------------------------

static int dyncfg_health_prototype_template_action(BUFFER *result, DYNCFG_CMDS cmd, BUFFER *payload, const char *source) {
    int code = HTTP_RESP_INTERNAL_SERVER_ERROR;
    switch(cmd) {
        case DYNCFG_CMD_ADD:
            code = dyncfg_default_response(result, HTTP_RESP_NOT_IMPLEMENTED, "add not implemented yet for prototype templates");
            break;

        case DYNCFG_CMD_SCHEMA:
            code = dyncfg_default_response(result, HTTP_RESP_NOT_IMPLEMENTED, "schema not implemented yet for prototype templates");
            break;

        case DYNCFG_CMD_REMOVE:
        case DYNCFG_CMD_RESTART:
        case DYNCFG_CMD_DISABLE:
        case DYNCFG_CMD_ENABLE:
        case DYNCFG_CMD_UPDATE:
        case DYNCFG_CMD_TEST:
        case DYNCFG_CMD_GET:
            code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "action given is not supported for prototype templates");
            break;

        case DYNCFG_CMD_NONE:
            code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "invalid action received for prototype templates");
            break;
    }

    return code;
}

static int dyncfg_health_prototype_action(BUFFER *result, DYNCFG_CMDS cmd, BUFFER *payload, const char *source, const char *alert_name) {
    int code = HTTP_RESP_INTERNAL_SERVER_ERROR;
    switch(cmd) {
        case DYNCFG_CMD_ADD:
            code = dyncfg_default_response(result, HTTP_RESP_NOT_IMPLEMENTED, "add not implemented yet");
            break;

        case DYNCFG_CMD_SCHEMA:
            code = dyncfg_default_response(result, HTTP_RESP_NOT_IMPLEMENTED, "schema not implemented yet");
            break;

        case DYNCFG_CMD_GET:
        {
            const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(health_globals.prototypes.dict, alert_name);
            if(!item)
                return dyncfg_default_response(result, HTTP_RESP_NOT_FOUND, "no alert prototype is available by the name given");

            RRD_ALERT_PROTOTYPE *ap = dictionary_acquired_item_value(item);
            health_prototype_to_json(result, ap, false);
            dictionary_acquired_item_release(health_globals.prototypes.dict, item);
            code = HTTP_RESP_OK;
        }
        break;

        case DYNCFG_CMD_REMOVE:
        case DYNCFG_CMD_RESTART:
        case DYNCFG_CMD_DISABLE:
        case DYNCFG_CMD_ENABLE:
        case DYNCFG_CMD_UPDATE:
        case DYNCFG_CMD_TEST:
            code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "action given is not supported for the prototype template");
            break;

        case DYNCFG_CMD_NONE:
            code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "invalid action received");
            break;
    }

    return code;
}

static int dyncfg_health_rrdcalc_action(BUFFER *result, DYNCFG_CMDS cmd, BUFFER *payload, const char *source, const char *hostname, const char *alert_name) {
    // find the host

    RRDHOST *host = rrdhost_find_by_hostname(hostname);
    if(!host)
        return dyncfg_default_response(result, HTTP_RESP_NOT_FOUND, "the hostname given is not found");

    // find the alert

    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(host->rrdcalc_root_index, alert_name);
    if(!item)
        return dyncfg_default_response(result, HTTP_RESP_NOT_FOUND, "the alert instance given is not found");

    int code = HTTP_RESP_INTERNAL_SERVER_ERROR;

    RRDCALC *rc = dictionary_acquired_item_value(item);

    switch(cmd) {
        case DYNCFG_CMD_NONE:
        case DYNCFG_CMD_ADD:
        case DYNCFG_CMD_RESTART:
            code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "invalid action received");
            break;

        case DYNCFG_CMD_REMOVE:
            if(rc->config.source_type != DYNCFG_SOURCE_TYPE_DYNCFG)
                code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "remove action is not supported for not dynamically configured alerts, use disable");
            else {
                dictionary_del(host->rrdcalc_root_index, alert_name);
                code = dyncfg_default_response(result, HTTP_RESP_OK, "alert removed");
            }
            break;

        case DYNCFG_CMD_DISABLE:
        case DYNCFG_CMD_ENABLE:
        case DYNCFG_CMD_UPDATE:
        case DYNCFG_CMD_TEST:
        case DYNCFG_CMD_SCHEMA:
            code = dyncfg_default_response(result, HTTP_RESP_NOT_IMPLEMENTED, "action not implemented yet");
            break;

        case DYNCFG_CMD_GET:
        {
            RRD_ALERT_PROTOTYPE ap = { 0 };
            ap.match = rc->match;
            ap.config = rc->config;
            health_prototype_to_json(result, &ap, false);
            code = HTTP_RESP_OK;
        }
        break;
    }

    dictionary_acquired_item_release(host->rrdcalc_root_index, item);

    return code;
}

int dyncfg_health_cb(const char *transaction __maybe_unused, const char *id, DYNCFG_CMDS cmd,
                     BUFFER *payload, usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
                     BUFFER *result, const char *source, void *data __maybe_unused) {

    char buf[strlen(id) + 1];
    memcpy(buf, id, sizeof(buf));

    char *words[100] = { NULL };
    size_t num_words = quoted_strings_splitter_dyncfg_id(buf, words, 100);
    size_t i = 0;
    int code = HTTP_RESP_INTERNAL_SERVER_ERROR;

    char *health_prefix = get_word(words, num_words, i++);
    if(!health_prefix || !*health_prefix || strcmp(health_prefix, "health") != 0)
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "first component of id is not 'health'");

    char *alert_prefix = get_word(words, num_words, i++);
    if(!alert_prefix || !*alert_prefix || strcmp(alert_prefix, "alert") != 0)
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "second component of id is not 'alert'");

    char *type_prefix = get_word(words, num_words, i++);
    if(type_prefix && *type_prefix && strcmp(type_prefix, "prototype") == 0) {
        char *alert_name = get_word(words, num_words, i++);
        if(!alert_name || !*alert_name) {
            // action on the prototype template

            code = dyncfg_health_prototype_template_action(result, cmd, payload, source);
        }
        else {
            // action on a specific alert prototype

            code = dyncfg_health_prototype_action(result, cmd, payload, source, alert_name);
        }
    }
    else if(type_prefix && *type_prefix && strncmp(type_prefix, "node[", 5) == 0) {
        // action on a specific alert instance

        char *hostname = &type_prefix[5];
        if(*hostname)
            hostname[strlen(hostname) - 1] = '\0'; // remove the ']'

        if(!*hostname)
            return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "no hostname name found in the id");

        char *alert_name = get_word(words, num_words, i++);
        if(!alert_name || !*alert_name)
            return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "no alert name found in the id");

        code = dyncfg_health_rrdcalc_action(result, cmd, payload, source, hostname, alert_name);
    }
    else
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "third component of id is not 'prototype' or 'node'");

    return code;
}

void health_dyncfg_unregister_all_prototypes(void) {
    char key[HEALTH_CONF_MAX_LINE];
    RRD_ALERT_PROTOTYPE *ap;

    // remove dyncfg
    // it is ok if they are not added before

    dfe_start_read(health_globals.prototypes.dict, ap) {
        snprintfz(key, sizeof(key), DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX ":%s", string2str(ap->config.name));
        dyncfg_del(localhost, key);
    }
    dfe_done(ap);
    dyncfg_del(localhost, DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX);
}

void health_dyncfg_register_all_prototypes(void) {
    char key[HEALTH_CONF_MAX_LINE];
    RRD_ALERT_PROTOTYPE *ap;

    dyncfg_add(localhost,
               DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX, "/health/alerts/prototypes",
               DYNCFG_STATUS_ACCEPTED, DYNCFG_TYPE_TEMPLATE,
               DYNCFG_SOURCE_TYPE_INTERNAL, "internal",
               DYNCFG_CMD_SCHEMA | DYNCFG_CMD_ADD, dyncfg_health_cb, NULL);

    dfe_start_read(health_globals.prototypes.dict, ap) {
        snprintfz(key, sizeof(key), DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX ":%s", string2str(ap->config.name));
        dyncfg_add(localhost, key, "/health/alerts/prototypes",
                   ap->match.enabled ? DYNCFG_STATUS_ACCEPTED : DYNCFG_STATUS_DISABLED, DYNCFG_TYPE_JOB,
                   ap->config.source_type, string2str(ap->config.source),
                   DYNCFG_CMD_SCHEMA | DYNCFG_CMD_GET | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE | DYNCFG_CMD_UPDATE | DYNCFG_CMD_TEST,
                   dyncfg_health_cb, NULL);
    }
    dfe_done(ap);

}
