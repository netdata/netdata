// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"

#define DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX "health:alert:prototype"

static void health_dyncfg_register_prototype(RRD_ALERT_PROTOTYPE *ap);

// ---------------------------------------------------------------------------------------------------------------------
// parse the json object of an alert definition

static bool parse_match(json_object *jobj, const char *path, struct rrd_alert_match *match, BUFFER *error) {
    STRING *on = NULL;
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "on", on, error, true);
    if(match->is_template)
        match->on.context = on;
    else
        match->on.chart = on;

    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, path, "os", match->os, error);
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, path, "host", match->host, error);

    if(match->is_template)
        JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, path, "instances", match->charts, error);

    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, path, "plugin", match->plugin, error);
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, path, "module", match->module, error);
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, path, "host_labels", match->host_labels, error);
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, path, "instance_labels", match->chart_labels, error);

    return true;
}

static bool parse_config_value_database_lookup(json_object *jobj, const char *path, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, path, "after", config->after, error);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, path, "before", config->before, error);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, path, "grouping", time_grouping_txt2id, config->group, error);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "options", rrdr_options_parse_one, config->options, error);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "dimensions", config->dimensions, error, true);
    return true;
}
static bool parse_config_value(json_object *jobj, const char *path, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_SUBOBJECT(jobj, path, "database_lookup", config, parse_config_value_database_lookup, error);
    JSONC_PARSE_TXT2EXPRESSION_OR_ERROR_AND_RETURN(jobj, path, "calculation", config->calculation, error);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "units", config->units, error, true);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, path, "update_every", config->update_every, error);
    return true;
}

static bool parse_config_conditions(json_object *jobj, const char *path, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_DOUBLE_OR_ERROR_AND_RETURN(jobj, path, "green", config->green, error);
    JSONC_PARSE_DOUBLE_OR_ERROR_AND_RETURN(jobj, path, "red", config->red, error);
    JSONC_PARSE_TXT2EXPRESSION_OR_ERROR_AND_RETURN(jobj, path, "warning_condition", config->warning, error);
    JSONC_PARSE_TXT2EXPRESSION_OR_ERROR_AND_RETURN(jobj, path, "critical_condition", config->critical, error);
    return true;
}

static bool parse_config_action_delay(json_object *jobj, const char *path, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, path, "up", config->delay_up_duration, error);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, path, "down", config->delay_down_duration, error);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, path, "max", config->delay_max_duration, error);
    JSONC_PARSE_DOUBLE_OR_ERROR_AND_RETURN(jobj, path, "multiplier", config->delay_multiplier, error);
    return true;
}
static bool parse_config_action_repeat(json_object *jobj, const char *path, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, path, "enabled", config->has_custom_repeat_config, error);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, path, "warning", config->warn_repeat_every, error);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, path, "critical", config->crit_repeat_every, error);
    return true;
}

static bool parse_config_action(json_object *jobj, const char *path, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, path, "options", alert_action_options_parse_one, config->alert_action_options, error);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "execute", config->exec, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "recipient", config->recipient, error, true);
    JSONC_PARSE_SUBOBJECT(jobj, path, "delay", config, parse_config_action_delay, error);
    JSONC_PARSE_SUBOBJECT(jobj, path, "repeat", config, parse_config_action_repeat, error);
    return true;
}

static bool parse_config(json_object *jobj, const char *path, struct rrd_alert_config *config, BUFFER *error) {
    // we shouldn't parse these from the payload - they are given to us via the function call
    // JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, "source_type", dyncfg_source_type2id, config->source_type);
    // JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "source", config->source);

    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "summary", config->summary, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "info", config->info, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "type", config->type, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "component", config->component, error, true);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "classification", config->classification, error, true);

    JSONC_PARSE_SUBOBJECT(jobj, path, "value", config, parse_config_value, error);
    JSONC_PARSE_SUBOBJECT(jobj, path, "conditions", config, parse_config_conditions, error);
    JSONC_PARSE_SUBOBJECT(jobj, path, "action", config, parse_config_action, error);

    return true;
}

static bool parse_prototype(json_object *jobj, const char *path, RRD_ALERT_PROTOTYPE *base, BUFFER *error, const char *name) {
    int64_t version;
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, path, "format_version", version, error);

    if(version != 1) {
        buffer_sprintf(error, "unsupported document version");
        return false;
    }

    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, path, "name", base->config.name, error, !name && !*name);

    json_object *rules;
    if (json_object_object_get_ex(jobj, "rules", &rules)) {
        size_t rules_len = json_object_array_length(rules);

        RRD_ALERT_PROTOTYPE *ap = base; // fill the first entry
        for (size_t i = 0; i < rules_len; i++) {
            if(!ap) {
                ap = callocz(1, sizeof(*base));
                ap->config.name = string_dup(base->config.name);
                DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(base->_internal.next, ap, _internal.prev, _internal.next);
            }

            json_object *rule = json_object_array_get_idx(rules, i);

            JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(rule, path, "enabled", ap->match.enabled, error);

            STRING *type = NULL;
            JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(rule, path, "type", type, error, true);
            if(string_strcmp(type, "template") == 0)
                ap->match.is_template = true;
            else if(string_strcmp(type, "instance") == 0)
                ap->match.is_template = false;
            else {
                buffer_sprintf(error, "type is '%s', but it can only be 'instance' or 'template'", string2str(type));
                return false;
            }

            JSONC_PARSE_SUBOBJECT(rule, path, "match", &ap->match, parse_match, error);
            JSONC_PARSE_SUBOBJECT(rule, path, "config", &ap->config, parse_config, error);

            ap = NULL; // so that we will create another one, if available
        }
    }
    else {
        buffer_sprintf(error, "the rules array is missing");
        return false;
    }

    return true;
}

static RRD_ALERT_PROTOTYPE *health_prototype_payload_parse(const char *payload, size_t payload_len, BUFFER *error, const char *name) {
    RRD_ALERT_PROTOTYPE *base = callocz(1, sizeof(*base));
    CLEAN_JSON_OBJECT *jobj = NULL;

    struct json_tokener *tokener = json_tokener_new();
    if (!tokener) {
        buffer_sprintf(error, "failed to allocate memory for json tokener");
        goto cleanup;
    }

    jobj = json_tokener_parse_ex(tokener, payload, (int)payload_len);
    if (json_tokener_get_error(tokener) != json_tokener_success) {
        const char *error_msg = json_tokener_error_desc(json_tokener_get_error(tokener));
        buffer_sprintf(error, "failed to parse json payload: %s", error_msg);
        json_tokener_free(tokener);
        goto cleanup;
    }
    json_tokener_free(tokener);

    if(!parse_prototype(jobj, "", base, error, name))
        goto cleanup;

    if(!base->config.name && name)
        base->config.name = string_strdupz(name);

    int i = 1;
    for(RRD_ALERT_PROTOTYPE *ap = base; ap; ap = ap->_internal.next, i++) {
        if(ap->config.name != base->config.name) {
            string_freez(ap->config.name);
            ap->config.name = string_dup(base->config.name);
        }

        if(!RRDCALC_HAS_DB_LOOKUP(ap) && !ap->config.calculation) {
            buffer_sprintf(error, "the rule No %d has neither database lookup nor calculation", i);
            goto cleanup;
        }

        if(ap->match.enabled)
            base->_internal.enabled = true;
    }

    if(string_strcmp(base->config.name, name) != 0) {
        buffer_sprintf(error,
                       "name parsed ('%s') does not match the name of the alert prototype ('%s')",
                       string2str(base->config.name), name);
        goto cleanup;
    }

    return base;

cleanup:
    health_prototype_free(base);
    return NULL;
}

// ---------------------------------------------------------------------------------------------------------------------
// generate the json object of an alert definition

static inline void health_prototype_rule_to_json_array_member(BUFFER *wb, RRD_ALERT_PROTOTYPE *ap, bool for_hashing) {
    buffer_json_add_array_item_object(wb);
    {
        buffer_json_member_add_boolean(wb, "enabled", ap->match.enabled);
        buffer_json_member_add_string(wb, "type", ap->match.is_template ? "template" : "instance");

        buffer_json_member_add_object(wb, "match");
        {
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
                    buffer_json_member_add_string(wb, "grouping", time_grouping_id2txt(ap->config.group));
                    rrdr_options_to_buffer_json_array(wb, "options", ap->config.options);
                    buffer_json_member_add_string(wb, "dimensions", string2str(ap->config.dimensions));
                }
                buffer_json_object_close(wb); // database lookup

                buffer_json_member_add_string(wb, "calculation", expression_source(ap->config.calculation));
                buffer_json_member_add_string(wb, "units", string2str(ap->config.units));
                buffer_json_member_add_uint64(wb, "update_every", ap->config.update_every);
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
                alert_action_options_to_buffer_json_array(wb, "options", ap->config.alert_action_options);
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

    buffer_json_member_add_uint64(wb, "format_version", 1);
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

static size_t dyncfg_health_remove_all_rrdcalc_of_prototype(STRING *alert_name) {
    size_t removed = 0;

    RRDHOST *host;
    dfe_start_reentrant(rrdhost_root_index, host) {
        RRDCALC *rc;
        foreach_rrdcalc_in_rrdhost_read(host, rc) {
            if(rc->config.name != alert_name)
                continue;

            rrdcalc_unlink_and_delete(host, rc, false);
            removed++;
        }
        foreach_rrdcalc_in_rrdhost_done(rc);
    }
    dfe_done(host);

    return removed;
}

static void dyncfg_health_prototype_reapply(RRD_ALERT_PROTOTYPE *ap) {
    dyncfg_health_remove_all_rrdcalc_of_prototype(ap->config.name);
    health_prototype_apply_to_all_hosts(ap);
}

static int dyncfg_health_prototype_template_action(BUFFER *result, DYNCFG_CMDS cmd, const char *add_name, BUFFER *payload, const char *source __maybe_unused) {
    int code = HTTP_RESP_INTERNAL_SERVER_ERROR;
    switch(cmd) {
        case DYNCFG_CMD_ADD: {
            CLEAN_BUFFER *error = buffer_create(0, NULL);
            RRD_ALERT_PROTOTYPE *nap = health_prototype_payload_parse(buffer_tostring(payload), buffer_strlen(payload), error, add_name);
            if(!nap)
                code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, buffer_tostring(error));
            else {
                nap->config.source_type = DYNCFG_SOURCE_TYPE_DYNCFG;
                bool added = health_prototype_add(nap); // this swaps ap <-> nap

                if(!added) {
                    health_prototype_free(nap);
                    return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "required attributes are missing");
                }
                else
                    freez(nap);

                const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(health_globals.prototypes.dict, add_name);
                if(!item)
                    return dyncfg_default_response(result, HTTP_RESP_INTERNAL_SERVER_ERROR, "added prototype is not found");

                RRD_ALERT_PROTOTYPE *ap = dictionary_acquired_item_value(item);

                dyncfg_health_prototype_reapply(ap);
                health_dyncfg_register_prototype(ap);
                code = ap->_internal.enabled ? DYNCFG_RESP_ACCEPTED : DYNCFG_RESP_ACCEPTED_DISABLED;
                dictionary_acquired_item_release(health_globals.prototypes.dict, item);

                code = dyncfg_default_response(result, code, "accepted");
            }
        }
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

static int dyncfg_health_prototype_job_action(BUFFER *result, DYNCFG_CMDS cmd, BUFFER *payload, const char *source __maybe_unused, const char *alert_name) {
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(health_globals.prototypes.dict, alert_name);
    if(!item)
        return dyncfg_default_response(result, HTTP_RESP_NOT_FOUND, "no alert prototype is available by the name given");

    RRD_ALERT_PROTOTYPE *ap = dictionary_acquired_item_value(item);

    char alert_name_dyncfg[strlen(DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX) + strlen(alert_name) + 10];
    snprintfz(alert_name_dyncfg, sizeof(alert_name_dyncfg), DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX ":%s", alert_name);

    int code = HTTP_RESP_INTERNAL_SERVER_ERROR;

    switch(cmd) {
        case DYNCFG_CMD_SCHEMA:
            code = dyncfg_default_response(result, HTTP_RESP_NOT_IMPLEMENTED, "schema not implemented yet");
            break;

        case DYNCFG_CMD_GET:
            health_prototype_to_json(result, ap, false);
            code = HTTP_RESP_OK;
            break;

        case DYNCFG_CMD_DISABLE:
            if(ap->_internal.enabled) {
                ap->_internal.enabled = false;
                dyncfg_health_prototype_reapply(ap);
                dyncfg_status(localhost, alert_name_dyncfg, DYNCFG_STATUS_DISABLED);
                code = dyncfg_default_response(result, HTTP_RESP_OK, "disabled");
            }
            else
                code = dyncfg_default_response(result, HTTP_RESP_OK, "already disabled");
            break;

        case DYNCFG_CMD_ENABLE:
            if(ap->_internal.enabled)
                code = dyncfg_default_response(result, HTTP_RESP_OK, "already enabled");
            else {
                size_t matches_enabled = 0;
                spinlock_lock(&ap->_internal.spinlock);
                for(RRD_ALERT_PROTOTYPE *t = ap; t ;t = t->_internal.next)
                    if(t->match.enabled)
                        matches_enabled++;
                spinlock_unlock(&ap->_internal.spinlock);

                if(!matches_enabled) {
                    code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "all rules in this alert are disabled, so enabling the alert has no effect");
                }
                else {
                    ap->_internal.enabled = true;
                    dyncfg_health_prototype_reapply(ap);
                    dyncfg_status(localhost, alert_name_dyncfg, DYNCFG_STATUS_ACCEPTED);
                    code = dyncfg_default_response(result, DYNCFG_RESP_ACCEPTED, "enabled");
                }
            }
            break;

        case DYNCFG_CMD_UPDATE: {
                CLEAN_BUFFER *error = buffer_create(0, NULL);
                RRD_ALERT_PROTOTYPE *nap = health_prototype_payload_parse(buffer_tostring(payload), buffer_strlen(payload), error, alert_name);
                if(!nap)
                    code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, buffer_tostring(error));
                else {
                    nap->config.source_type = DYNCFG_SOURCE_TYPE_DYNCFG;
                    bool added = health_prototype_add(nap); // this swaps ap <-> nap

                    if(!added) {
                        health_prototype_free(nap);
                        return dyncfg_default_response( result, HTTP_RESP_BAD_REQUEST, "required attributes are missing");
                    }
                    else
                        freez(nap);

                    dyncfg_health_prototype_reapply(ap);
                    code = ap->_internal.enabled ? DYNCFG_RESP_ACCEPTED : DYNCFG_RESP_ACCEPTED_DISABLED;
                    code = dyncfg_default_response(result, code, "updated");
                }
            }
            break;

        case DYNCFG_CMD_REMOVE:
            dyncfg_health_remove_all_rrdcalc_of_prototype(ap->config.name);
            dictionary_del(health_globals.prototypes.dict, dictionary_acquired_item_name(item));
            code = dyncfg_default_response(result, HTTP_RESP_OK, "deleted");
            dyncfg_del(localhost, alert_name_dyncfg);
            break;

        case DYNCFG_CMD_TEST:
        case DYNCFG_CMD_ADD:
        case DYNCFG_CMD_RESTART:
            code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "action given is not supported for the prototype job");
            break;

        case DYNCFG_CMD_NONE:
            code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "invalid action received");
            break;
    }

    dictionary_acquired_item_release(health_globals.prototypes.dict, item);
    return code;
}

int dyncfg_health_cb(const char *transaction __maybe_unused, const char *id, DYNCFG_CMDS cmd, const char *add_name,
                     BUFFER *payload, usec_t *stop_monotonic_ut __maybe_unused, bool *cancelled __maybe_unused,
                     BUFFER *result, HTTP_ACCESS access __maybe_unused, const char *source, void *data __maybe_unused) {

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
    if(!type_prefix || !*type_prefix || strcmp(type_prefix, "prototype") != 0)
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "third component of id is not 'prototype'");

    char *alert_name = get_word(words, num_words, i++);
    if(!alert_name || !*alert_name) {
        // action on the prototype template

        code = dyncfg_health_prototype_template_action(result, cmd, add_name, payload, source);
    }
    else {
        // action on a specific alert prototype

        code = dyncfg_health_prototype_job_action(result, cmd, payload, source, alert_name);
    }
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

static void health_dyncfg_register_prototype(RRD_ALERT_PROTOTYPE *ap) {
    char key[HEALTH_CONF_MAX_LINE];

//    bool trace = false;
//    if(string_strcmp(ap->config.name, "ram_available") == 0)
//        trace = true;

    snprintfz(key, sizeof(key), DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX ":%s", string2str(ap->config.name));
    dyncfg_add(localhost, key, "/health/alerts/prototypes",
               ap->_internal.enabled ? DYNCFG_STATUS_ACCEPTED : DYNCFG_STATUS_DISABLED, DYNCFG_TYPE_JOB,
               ap->config.source_type, string2str(ap->config.source),
               DYNCFG_CMD_SCHEMA | DYNCFG_CMD_GET | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE |
                   DYNCFG_CMD_UPDATE | DYNCFG_CMD_TEST |
                   (ap->config.source_type == DYNCFG_SOURCE_TYPE_DYNCFG && !ap->_internal.is_on_disk ? DYNCFG_CMD_REMOVE : 0),
               HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_VIEW_AGENT_CONFIG,
               HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_EDIT_AGENT_CONFIG,
               dyncfg_health_cb, NULL);

#ifdef NETDATA_TEST_HEALTH_PROTOTYPES_JSON_AND_PARSING
    {
        // make sure we can generate valid json, parse it back and come up to the same object

        CLEAN_BUFFER *original = buffer_create(0, NULL);
        CLEAN_BUFFER *parsed = buffer_create(0, NULL);
        CLEAN_BUFFER *error = buffer_create(0, NULL);
        health_prototype_to_json(original, ap, true);
        RRD_ALERT_PROTOTYPE *t = health_prototype_payload_parse(buffer_tostring(original), buffer_strlen(original), error, string2str(ap->config.name));
        if(!t)
            fatal("hey! cannot parse: %s", buffer_tostring(error));

        health_prototype_to_json(parsed, t, true);

        if(strcmp(buffer_tostring(original), buffer_tostring(parsed)) != 0)
            fatal("hey! they are different!");
    }
#endif
}

void health_dyncfg_register_all_prototypes(void) {
    RRD_ALERT_PROTOTYPE *ap;

    dyncfg_add(localhost,
               DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX, "/health/alerts/prototypes",
               DYNCFG_STATUS_ACCEPTED, DYNCFG_TYPE_TEMPLATE,
               DYNCFG_SOURCE_TYPE_INTERNAL, "internal",
               DYNCFG_CMD_SCHEMA | DYNCFG_CMD_ADD | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
               HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_VIEW_AGENT_CONFIG,
               HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_EDIT_AGENT_CONFIG,
               dyncfg_health_cb, NULL);

    dfe_start_read(health_globals.prototypes.dict, ap) {
        if(ap->config.source_type != DYNCFG_SOURCE_TYPE_DYNCFG)
            health_dyncfg_register_prototype(ap);
    }
    dfe_done(ap);
}
