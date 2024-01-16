// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"

// ---------------------------------------------------------------------------------------------------------------------

static void health_prototype_free_unsafe(RRD_ALERT_PROTOTYPE *ap) {
    rrd_alert_match_free(&ap->match);
    rrd_alert_config_free(&ap->config);
}

void health_prototype_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    RRD_ALERT_PROTOTYPE *ap = value;
    spinlock_init(&ap->_internal.spinlock);
}

bool health_prototype_conflict_cb(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    RRD_ALERT_PROTOTYPE *ap = old_value;
    RRD_ALERT_PROTOTYPE *nap = callocz(1, sizeof(*nap));
    memcpy(nap, new_value, sizeof(*nap));

    spinlock_lock(&ap->_internal.spinlock);
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(ap->_internal.next, nap, _internal.prev, _internal.next);
    spinlock_unlock(&ap->_internal.spinlock);

    return true;
}

void health_prototype_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    RRD_ALERT_PROTOTYPE *ap = value;
    spinlock_lock(&ap->_internal.spinlock);

    while(ap->_internal.next) {
        RRD_ALERT_PROTOTYPE *t = ap->_internal.next;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(ap->_internal.next, t, _internal.prev, _internal.next);
        health_prototype_free_unsafe(t);
        freez(t);
    }

    spinlock_unlock(&ap->_internal.spinlock);

    health_prototype_free_unsafe(ap);
}

void health_init_prototypes(void) {
    if(health_globals.prototypes.dict)
        return;

    health_globals.prototypes.dict = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_register_insert_callback(health_globals.prototypes.dict, health_prototype_insert_cb, NULL);
    dictionary_register_conflict_callback(health_globals.prototypes.dict, health_prototype_conflict_cb, NULL);
    dictionary_register_delete_callback(health_globals.prototypes.dict, health_prototype_delete_cb, NULL);
}

// ---------------------------------------------------------------------------------------------------------------------

static inline void health_prototype_rule_to_json_array_member(BUFFER *wb, RRD_ALERT_PROTOTYPE *ap, bool for_hashing) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_object(wb, "match");
        {
            buffer_json_member_add_boolean(wb, "template", ap->match.is_template);

            if(ap->match.is_template)
                buffer_json_member_add_string(wb, "context", string2str(ap->match.on.context));
            else
                buffer_json_member_add_string(wb, "instance", string2str(ap->match.on.chart));

            buffer_json_member_add_string_or_empty(wb, "os", string2str(ap->match.os));
            buffer_json_member_add_string_or_empty(wb, "host", string2str(ap->match.host));

            if(ap->match.is_template)
                buffer_json_member_add_string_or_empty(wb, "instances", string2str(ap->match.charts));

            buffer_json_member_add_string_or_empty(wb, "plugin", string2str(ap->match.plugin));
            buffer_json_member_add_string_or_empty(wb, "module", string2str(ap->match.module));
            buffer_json_member_add_string_or_empty(wb, "host_labels", string2str(ap->match.host_labels));
            buffer_json_member_add_string_or_empty(wb, "instance_labels", string2str(ap->match.chart_labels));
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

                buffer_json_member_add_string(wb, "calculation", ap->config.calculation ? string2str(ap->config.calculation->source) : "");
                buffer_json_member_add_string(wb, "units", string2str(ap->config.units));
            }
            buffer_json_object_close(wb); // value

            buffer_json_member_add_object(wb, "conditions");
            {
                buffer_json_member_add_double(wb, "green", ap->config.green);
                buffer_json_member_add_double(wb, "red", ap->config.red);
                buffer_json_member_add_string(wb, "warning_condition", ap->config.warning ? string2str(ap->config.warning->source) : "");
                buffer_json_member_add_string(wb, "critical_condition", ap->config.critical ? string2str(ap->config.critical->source) : "");
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

static inline void health_prototype_to_json(BUFFER *wb, RRD_ALERT_PROTOTYPE *ap, bool for_hashing) {
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

// If needed, add a prefix key to all possible values in the range
static inline char *health_config_add_key_to_values(char *value) {
    BUFFER *wb = buffer_create(HEALTH_CONF_MAX_LINE + 1, NULL);
    char key[HEALTH_CONF_MAX_LINE + 1];
    char data[HEALTH_CONF_MAX_LINE + 1];

    char *s = value;
    size_t i = 0;

    key[0] = '\0';
    while(*s) {
        if (*s == '=') {
            //hold the key
            data[i]='\0';
            strncpyz(key, data, HEALTH_CONF_MAX_LINE);
            i=0;
        } else if (*s == ' ') {
            data[i]='\0';
            if (data[0]=='!')
                buffer_snprintf(wb, HEALTH_CONF_MAX_LINE, "!%s=%s ", key, data + 1);
            else
                buffer_snprintf(wb, HEALTH_CONF_MAX_LINE, "%s=%s ", key, data);
            i=0;
        } else {
            data[i++] = *s;
        }
        s++;
    }

    data[i]='\0';
    if (data[0]) {
        if (data[0]=='!')
            buffer_snprintf(wb, HEALTH_CONF_MAX_LINE, "!%s=%s ", key, data + 1);
        else
            buffer_snprintf(wb, HEALTH_CONF_MAX_LINE, "%s=%s ", key, data);
    }

    char *final = strdupz(buffer_tostring(wb));
    buffer_free(wb);

    return final;
}

static void health_prototype_activate_match_patterns(struct rrd_alert_match *am) {
    if(am->os) {
        simple_pattern_free(am->os_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->os));
        am->os_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->host) {
        simple_pattern_free(am->host_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->host));
        am->host_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->charts) {
        simple_pattern_free(am->charts_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->charts));
        am->charts_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->plugin) {
        simple_pattern_free(am->plugin_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->plugin));
        am->plugin_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->module) {
        simple_pattern_free(am->module_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->module));
        am->module_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->host_labels) {
        simple_pattern_free(am->host_labels_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->host_labels));
        am->host_labels_pattern = simple_pattern_create(
            tmp, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp);
    }

    if(am->chart_labels) {
        simple_pattern_free(am->chart_labels_pattern);

        char *tmp = simple_pattern_trim_around_equal(string2str(am->chart_labels));
        char *tmp2 = health_config_add_key_to_values(tmp);
        am->chart_labels_pattern = simple_pattern_create(
            tmp2, NULL, SIMPLE_PATTERN_EXACT, true);
        freez(tmp2);
        freez(tmp);
    }
}

bool health_prototype_add(RRD_ALERT_PROTOTYPE *ap) {

    if(!ap->match.is_template) {
        if(!ap->match.on.chart) {
            netdata_log_error(
                "HEALTH: alert '%s' does not define a instance (parameter 'on'). Source: %s",
                string2str(ap->config.name), string2str(ap->config.source));
            return false;
        }
    }
    else {
        if(!ap->match.on.context) {
            netdata_log_error(
                "HEALTH: alert '%s' does not define a context (parameter 'on'). Source: %s",
                string2str(ap->config.name), string2str(ap->config.source));
            return false;
        }
    }

    if(!ap->config.update_every) {
        netdata_log_error(
            "HEALTH: alert '%s' has no frequency (parameter 'every'). Source: %s",
            string2str(ap->config.name), string2str(ap->config.source));
        return false;
    }

    if(!RRDCALC_HAS_DB_LOOKUP(ap) && !ap->config.calculation && !ap->config.warning && !ap->config.critical) {
        netdata_log_error(
            "HEALTH: alert '%s' is useless (no db lookup, no calculation, no warning and no critical expressions). Source: %s",
            string2str(ap->config.name), string2str(ap->config.source));
        return false;
    }

    // generate the hash id
    CLEAN_BUFFER *wb = buffer_create(100, NULL);
    health_prototype_to_json(wb, ap, true);
    UUID uuid = UUID_generate_from_hash(buffer_tostring(wb), buffer_strlen(wb));
    uuid_copy(ap->config.hash_id, uuid.uuid);

    // store it in SQL
    sql_alert_hash_and_store_config(ap);

    // activate the match patterns in it
    health_prototype_activate_match_patterns(&ap->match);

    if(!ap->config.exec)
        ap->config.exec = string_dup(health_globals.config.default_exec);

    if(!ap->config.recipient)
        ap->config.recipient = string_dup(health_globals.config.default_recipient);

    // add it to the prototypes
    dictionary_set(health_globals.prototypes.dict, string2str(ap->config.name), ap, sizeof(*ap));

    return true;
}

// ---------------------------------------------------------------------------------------------------------------------

#define DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX "health:alert:prototype"

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

void health_reload_prototypes(void) {
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

    // clear old prototypes from memory

    dictionary_flush(health_globals.prototypes.dict);

    // load the prototypes from disk

    recursive_config_double_dir_load(
        health_user_config_dir(),
        health_globals.config.stock_enabled ? health_stock_config_dir() : NULL,
        NULL,
        health_readfile,
        NULL, 0);

    // add dyncfg
    // this will update them with current dyncfg configuration

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

// ---------------------------------------------------------------------------------------------------------------------

static bool prototype_matches_host(RRDHOST *host, RRD_ALERT_PROTOTYPE *ap) {
    if(health_globals.config.enabled_alerts &&
        !simple_pattern_matches(health_globals.config.enabled_alerts, string2str(ap->config.name)))
        return false;

    if(ap->match.os_pattern && !simple_pattern_matches_string(ap->match.os_pattern, host->os))
        return false;

    if(ap->match.host_pattern && !simple_pattern_matches_string(ap->match.host_pattern, host->hostname))
        return false;

    if(host->rrdlabels && ap->match.host_labels_pattern &&
        !rrdlabels_match_simple_pattern_parsed(
            host->rrdlabels, ap->match.host_labels_pattern, '=', NULL))
        return false;

    return true;
}

static bool prototype_matches_rrdset(RRDSET *st, RRD_ALERT_PROTOTYPE *ap) {
    // match the chart id
    if(!ap->match.is_template && ap->match.on.chart &&
        ap->match.on.chart != st->id && ap->match.on.chart != st->name)
        return false;

    // match the chart context
    if(ap->match.is_template && ap->match.on.context &&
        ap->match.on.context != st->context)
        return false;

    // match the chart pattern
    if(ap->match.is_template && ap->match.charts && ap->match.charts_pattern &&
        !simple_pattern_matches_string(ap->match.charts_pattern, st->id) &&
        !simple_pattern_matches_string(ap->match.charts_pattern, st->name))
        return false;

    // match the plugin pattern
    if(ap->match.plugin && ap->match.plugin_pattern &&
        !simple_pattern_matches_string(ap->match.plugin_pattern, st->plugin_name))
        return false;

    // match the module pattern
    if(ap->match.module && ap->match.module_pattern &&
        !simple_pattern_matches_string(ap->match.module_pattern, st->module_name))
        return false;

    if (st->rrdlabels && ap->match.chart_labels && ap->match.chart_labels_pattern &&
        !rrdlabels_match_simple_pattern_parsed(
            st->rrdlabels, ap->match.chart_labels_pattern, '=', NULL))
        return false;

    return true;
}

void health_prototype_copy_match_without_patterns(struct rrd_alert_match *dst, struct rrd_alert_match *src) {
    dst->enabled = src->enabled;
    dst->is_template = src->is_template;

    if(dst->is_template)
        dst->on.context = string_dup(src->on.context);
    else
        dst->on.chart = string_dup(src->on.chart);

    dst->os = string_dup(src->os);
    dst->host = string_dup(src->host);
    dst->charts = string_dup(src->charts);
    dst->plugin = string_dup(src->plugin);
    dst->module = string_dup(src->module);
    dst->host_labels = string_dup(src->host_labels);
    dst->chart_labels = string_dup(src->chart_labels);
}

void health_prototype_copy_config(struct rrd_alert_config *dst, struct rrd_alert_config *src) {
    uuid_copy(dst->hash_id, src->hash_id);

    dst->name = string_dup(src->name);

    dst->exec = string_dup(src->exec);
    dst->recipient = string_dup(src->recipient);

    dst->classification = string_dup(src->classification);
    dst->component = string_dup(src->component);
    dst->type = string_dup(src->type);

    dst->source_type = src->source_type;
    dst->source = string_dup(src->source);
    dst->units = string_dup(src->units);
    dst->summary = string_dup(src->summary);
    dst->info = string_dup(src->info);

    dst->update_every = src->update_every;

    dst->green = src->green;
    dst->red = src->red;

    dst->dimensions = string_dup(src->dimensions);

    dst->group = src->group;
    dst->before = src->before;
    dst->after = src->after;
    dst->options = src->options;

    const char *failed_at = NULL;
    int error = 0;

    if(src->calculation)
        dst->calculation = expression_parse(string2str(src->calculation->source), &failed_at, &error);

    if(src->warning)
        dst->warning = expression_parse(string2str(src->warning->source), &failed_at, &error);

    if(src->critical)
        dst->critical = expression_parse(string2str(src->critical->source), &failed_at, &error);


    dst->delay_up_duration = src->delay_up_duration;
    dst->delay_down_duration = src->delay_down_duration;
    dst->delay_max_duration = src->delay_max_duration;
    dst->delay_multiplier = src->delay_multiplier;

    dst->has_custom_repeat_config = src->has_custom_repeat_config;
    dst->warn_repeat_every = src->warn_repeat_every;
    dst->crit_repeat_every = src->crit_repeat_every;
}

void health_prototype_alerts_for_rrdset_incrementally(RRDSET *st) {
    RRDHOST *host = st->rrdhost;

    RRD_ALERT_PROTOTYPE *ap;
    dfe_start_read(health_globals.prototypes.dict, ap) {
        RRD_ALERT_PROTOTYPE *t;

        spinlock_lock(&ap->_internal.spinlock);
        for(t = ap; t ; t = t->_internal.next) {
            if(!t->match.enabled)
                continue;

            if(!prototype_matches_host(host, t))
                continue;

            if(!prototype_matches_rrdset(st, t))
                continue;

            rrdcalc_add_from_prototype(host, st, ap);
        }
        spinlock_unlock(&ap->_internal.spinlock);
    }
    dfe_done(ap);
}

void health_prototype_reset_alerts_for_rrdset(RRDSET *st) {
    rrdcalc_unlink_all_rrdset_alerts(st);
    health_prototype_alerts_for_rrdset_incrementally(st);
}

// ---------------------------------------------------------------------------------------------------------------------

void health_apply_prototypes_to_host(RRDHOST *host) {
    if(unlikely(!host->health.health_enabled) && !rrdhost_flag_check(host, RRDHOST_FLAG_INITIALIZED_HEALTH))
        return;

    // free all running alarms
    rrdcalc_delete_all(host);

    // invalidate all previous entries in the alarm log
    rw_spinlock_read_lock(&host->health_log.spinlock);
    ALARM_ENTRY *t;
    for(t = host->health_log.alarms ; t ; t = t->next) {
        if(t->new_status != RRDCALC_STATUS_REMOVED)
            t->flags |= HEALTH_ENTRY_FLAG_UPDATED;
    }
    rw_spinlock_read_unlock(&host->health_log.spinlock);

    // reset all thresholds to all charts
    RRDSET *st;
    rrdset_foreach_read(st, host) {
        st->green = NAN;
        st->red = NAN;

        health_prototype_reset_alerts_for_rrdset(st);
    }
    rrdset_foreach_done(st);

#ifdef ENABLE_ACLK
    if (netdata_cloud_enabled) {
        struct aclk_sync_cfg_t *wc = host->aclk_config;
        if (likely(wc)) {
            wc->alert_queue_removed = SEND_REMOVED_AFTER_HEALTH_LOOPS;
        }
    }
#endif
}

void health_apply_prototypes_to_all_hosts(void) {
    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host){
        health_apply_prototypes_to_host(host);
    }
    dfe_done(host);
}

// ---------------------------------------------------------------------------------------------------------------------

static inline void buffer_append_prototype_key(BUFFER *wb, const char *key, const char *txt) {
    if(unlikely(!txt || !*txt || strcmp(txt, "*") == 0 || strcmp(txt, "!*") == 0 || strcmp(txt, "!* *") == 0)) return;

    if(key) {
        buffer_putc(wb, ',');
        buffer_strcat(wb, key);
        buffer_putc(wb, '[');
    }

    while(txt && *txt) {
        if(*txt <= ' ' || *txt == ':' || *txt == '"' || *txt == '\'') {
            buffer_putc(wb, ',');
            txt++;
            while(*txt && (*txt <= ' ' || *txt == ':' || *txt == '"' || *txt == '\'')) txt++;
        }
        else {
            buffer_putc(wb, *txt);
            txt++;
        }
    }
    if(key) {
        buffer_putc(wb, ']');
    }
}

STRING *health_alert_config_dyncfg_key(struct rrd_alert_match *am, const char *name, RRDHOST *host, RRDSET *st) {
    CLEAN_BUFFER *buffer = buffer_create(1024, NULL);

    if(!host && st)
        host = st->rrdhost;

    if(host && st) {
        buffer_sprintf(buffer, "health:alert:node[%s]:", rrdhost_hostname(host));
        buffer_append_prototype_key(buffer, NULL, name);
        buffer_append_prototype_key(buffer, "on", rrdset_name(st));
    }
    else if(host && !st) {
        if (am->is_template)
            buffer_sprintf(buffer, "health:alert:prototype:node[%s]:template:", rrdhost_hostname(host));
        else
            buffer_sprintf(buffer, "health:alert:prototype:node[%s]:alert:", rrdhost_hostname(host));

        buffer_append_prototype_key(buffer, NULL, name);

        if (am->is_template)
            buffer_append_prototype_key(buffer, "on", string2str(am->on.context));
        else
            buffer_append_prototype_key(buffer, "on", string2str(am->on.chart));

        if (am->plugin)
            buffer_append_prototype_key(buffer, "plugin", string2str(am->plugin));

        if (am->module)
            buffer_append_prototype_key(buffer, "module", string2str(am->module));

        if (am->charts)
            buffer_append_prototype_key(buffer, "instances", string2str(am->charts));

        if (am->chart_labels)
            buffer_append_prototype_key(buffer, "instance_labels", string2str(am->chart_labels));
    }
    else {
        // both rrdhost and rrdset are missing
        const char *type;
        if (am->is_template)
            type = "health:alert:prototype:global:template:";
        else
            type = "health:alert:prototype:global:alert:";

        buffer_strcat(buffer, type);
        buffer_append_prototype_key(buffer, NULL, name);

        if (am->is_template)
            buffer_append_prototype_key(buffer, "on", string2str(am->on.context));
        else
            buffer_append_prototype_key(buffer, "on", string2str(am->on.chart));

        if (am->host)
            buffer_append_prototype_key(buffer, "node", string2str(am->host));

        if (am->os)
            buffer_append_prototype_key(buffer, "os", string2str(am->os));

        if (am->host_labels)
            buffer_append_prototype_key(buffer, "node_labels", string2str(am->host_labels));

        if (am->plugin)
            buffer_append_prototype_key(buffer, "plugin", string2str(am->plugin));

        if (am->module)
            buffer_append_prototype_key(buffer, "module", string2str(am->module));

        if (am->charts)
            buffer_append_prototype_key(buffer, "instances", string2str(am->charts));

        if (am->chart_labels)
            buffer_append_prototype_key(buffer, "instance_labels", string2str(am->chart_labels));
    }

    const char *final = buffer_tostring(buffer);
    return string_strdupz(final);
}
