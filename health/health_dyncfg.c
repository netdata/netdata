// SPDX-License-Identifier: GPL-3.0-or-later

#include "health_internals.h"

#define DYNCFG_HEALTH_ALERT_PROTOTYPE_PREFIX "health:alert:prototype"

static void health_dyncfg_register_prototype(RRD_ALERT_PROTOTYPE *ap);

// ---------------------------------------------------------------------------------------------------------------------
// parse the json object of an alert definition

#define JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, member, dst) do {                                            \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_boolean))             \
        dst = json_object_get_boolean(_j);                                                                      \
    else {                                                                                                      \
        buffer_sprintf(error, "missing or invalid type for '%s' boolean", member);                              \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, member, dst) do {                                      \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string)) {            \
        string_freez(dst);                                                                                      \
        dst = string_strdupz(json_object_get_string(_j));                                                       \
    }                                                                                                           \
    else {                                                                                                      \
        buffer_sprintf(error, "missing or invalid type for '%s' string", member);                               \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, member, dst) do {                                     \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string)) {            \
        string_freez(dst);                                                                                      \
        const char *_v = json_object_get_string(_j);                                                            \
        if(strcmp(_v, "*") == 0)                                                                                \
            dst = NULL;                                                                                         \
        else                                                                                                    \
            dst = string_strdupz(_v);                                                                           \
    }                                                                                                           \
    else {                                                                                                      \
        buffer_sprintf(error, "missing or invalid type for '%s' string", member);                               \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_TXT2EXPRESSION_OR_ERROR_AND_RETURN(jobj, member, dst) do {                                  \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string)) {            \
        const char *_t = json_object_get_string(_j);                                                            \
        if(_t && *_t && strcmp(_t, "*") != 0) {                                                                 \
            const char *_failed_at = NULL;                                                                      \
            int _err = 0;                                                                                       \
            expression_free(dst);                                                                               \
            dst = expression_parse(_t, &_failed_at, &_err);                                                     \
            if(!dst) {                                                                                          \
                buffer_sprintf(error, "expression '%s' has a non-parseable expression '%s': %s at '%s'",        \
                               member, _t, expression_strerror(_err), _failed_at);                              \
                return false;                                                                                   \
            }                                                                                                   \
        }                                                                                                       \
    }                                                                                                           \
    else {                                                                                                      \
        buffer_sprintf(error, "missing or invalid type for '%s' expression", member);                           \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, member, converter, dst) do {                  \
    json_object *_jarray;                                                                                       \
    if (json_object_object_get_ex(jobj, member, &_jarray) && json_object_is_type(_jarray, json_type_array)) {   \
        size_t _num_options = json_object_array_length(_jarray);                                                \
        dst = 0;                                                                                                \
        for (size_t _i = 0; _i < _num_options; ++_i) {                                                          \
            json_object *_joption = json_object_array_get_idx(_jarray, _i);                                     \
            if (!json_object_is_type(_joption, json_type_string)) {                                             \
                buffer_sprintf(error, "invalid type for '%s' at index %zu", member, _i);                        \
                return false;                                                                                   \
            }                                                                                                   \
            const char *_option_str = json_object_get_string(_joption);                                         \
            typeof(dst) _bit = converter(_option_str);                                                          \
            if (_bit == 0) {                                                                                    \
                buffer_sprintf(error, "unknown option '%s' in '%s' at index %zu", _option_str, member, _i);     \
                return false;                                                                                   \
            }                                                                                                   \
            dst |= _bit;                                                                                        \
        }                                                                                                       \
    } else {                                                                                                    \
        buffer_sprintf(error, "missing or invalid type for '%s' array", member);                                \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)


#define JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, member, converter, dst) do {                             \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j) && json_object_is_type(_j, json_type_string))              \
        dst = converter(json_object_get_string(_j));                                                            \
    else {                                                                                                      \
        buffer_sprintf(error, "missing or invalid type (expected text value) for '%s' enum", member);           \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, member, dst) do {                                             \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j)) {                                                         \
        if (_j != NULL && json_object_is_type(_j, json_type_int))                                               \
            dst = json_object_get_int(_j);                                                                      \
        else if (_j != NULL && json_object_is_type(_j, json_type_double))                                       \
            dst = (typeof(dst))json_object_get_double(_j);                                                      \
        else if (_j == NULL)                                                                                    \
            dst = 0;                                                                                            \
        else {                                                                                                  \
            buffer_sprintf(error, "not supported type (expected int) for '%s'", member);                        \
            return false;                                                                                       \
        }                                                                                                       \
    } else {                                                                                                    \
        buffer_sprintf(error, "missing or invalid type (expected double value or null) for '%s'", member);      \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_DOUBLE_OR_ERROR_AND_RETURN(jobj, member, dst) do {                                          \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j)) {                                                         \
        if (_j != NULL && json_object_is_type(_j, json_type_double))                                            \
            dst = json_object_get_double(_j);                                                                   \
        else if (_j != NULL && json_object_is_type(_j, json_type_int))                                          \
            dst = (typeof(dst))json_object_get_int(_j);                                                         \
        else if (_j == NULL)                                                                                    \
            dst = NAN;                                                                                          \
        else {                                                                                                  \
            buffer_sprintf(error, "not supported type (expected double) for '%s'", member);                     \
            return false;                                                                                       \
        }                                                                                                       \
    } else {                                                                                                    \
        buffer_sprintf(error, "missing or invalid type (expected double value or null) for '%s'", member);      \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

#define JSONC_PARSE_SUBOBJECT(jobj, member, dst, callback) do { \
    json_object *_j;                                                                                            \
    if (json_object_object_get_ex(jobj, member, &_j)) {                                                         \
        if (!callback(_j, dst, error)) {                                                                        \
            return false;                                                                                       \
        }                                                                                                       \
    } else {                                                                                                    \
        buffer_sprintf(error, "missing '%s' object", member);                                                   \
        return false;                                                                                           \
    }                                                                                                           \
} while(0)

static bool parse_match(json_object *jobj, struct rrd_alert_match *match, BUFFER *error) {
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, "enabled", match->enabled);
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, "template", match->is_template);

    STRING *on = NULL;
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "on", on);
    if(match->is_template)
        match->on.context = on;
    else
        match->on.chart = on;

    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, "os", match->os);
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, "host", match->host);
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, "instances", match->charts);
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, "plugin", match->plugin);
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, "module", match->module);
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, "host_labels", match->host_labels);
    JSONC_PARSE_TXT2PATTERN_OR_ERROR_AND_RETURN(jobj, "instance_labels", match->chart_labels);

    return true;
}

static bool parse_config_value_database_lookup(json_object *jobj, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, "after", config->after);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, "before", config->before);
    JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, "grouping", time_grouping_txt2id, config->group);
    JSONC_PARSE_ARRAY_OF_TXT2BITMAP_OR_ERROR_AND_RETURN(jobj, "options", rrdr_options_parse_one, config->options);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "dimensions", config->dimensions);
    return true;
}
static bool parse_config_value(json_object *jobj, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_SUBOBJECT(jobj, "database_lookup", config, parse_config_value_database_lookup);
    JSONC_PARSE_TXT2EXPRESSION_OR_ERROR_AND_RETURN(jobj, "calculation", config->calculation);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "units", config->units);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, "update_every", config->update_every);
    return true;
}

static bool parse_config_conditions(json_object *jobj, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_DOUBLE_OR_ERROR_AND_RETURN(jobj, "green", config->green);
    JSONC_PARSE_DOUBLE_OR_ERROR_AND_RETURN(jobj, "red", config->red);
    JSONC_PARSE_TXT2EXPRESSION_OR_ERROR_AND_RETURN(jobj, "warning_condition", config->warning);
    JSONC_PARSE_TXT2EXPRESSION_OR_ERROR_AND_RETURN(jobj, "critical_condition", config->critical);
    return true;
}

static bool parse_config_action_delay(json_object *jobj, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, "up", config->delay_up_duration);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, "down", config->delay_down_duration);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, "max", config->delay_max_duration);
    JSONC_PARSE_DOUBLE_OR_ERROR_AND_RETURN(jobj, "multiplier", config->delay_multiplier);
    return true;
}
static bool parse_config_action_repeat(json_object *jobj, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_BOOL_OR_ERROR_AND_RETURN(jobj, "enabled", config->has_custom_repeat_config);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, "warning", config->warn_repeat_every);
    JSONC_PARSE_INT_OR_ERROR_AND_RETURN(jobj, "critical", config->crit_repeat_every);
    return true;
}

static bool parse_config_action(json_object *jobj, struct rrd_alert_config *config, BUFFER *error) {
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "execute", config->exec);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "recipient", config->recipient);
    JSONC_PARSE_SUBOBJECT(jobj, "delay", config, parse_config_action_delay);
    JSONC_PARSE_SUBOBJECT(jobj, "repeat", config, parse_config_action_repeat);
    return true;
}

static bool parse_config(json_object *jobj, struct rrd_alert_config *config, BUFFER *error) {
    // JSONC_PARSE_TXT2ENUM_OR_ERROR_AND_RETURN(jobj, "source_type", dyncfg_source_type2id, config->source_type);
    // JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "source", config->source);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "summary", config->summary);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "info", config->info);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "type", config->type);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "component", config->component);
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "classification", config->classification);

    JSONC_PARSE_SUBOBJECT(jobj, "value", config, parse_config_value);
    JSONC_PARSE_SUBOBJECT(jobj, "conditions", config, parse_config_conditions);
    JSONC_PARSE_SUBOBJECT(jobj, "action", config, parse_config_action);

    return true;
}

static bool parse_prototype(json_object *jobj, RRD_ALERT_PROTOTYPE *base, BUFFER *error) {
    JSONC_PARSE_TXT2STRING_OR_ERROR_AND_RETURN(jobj, "name", base->config.name);

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

            JSONC_PARSE_SUBOBJECT(rule, "match", &ap->match, parse_match);
            JSONC_PARSE_SUBOBJECT(rule, "config", &ap->config, parse_config);

            ap = NULL; // so that we will create another one, if available
        }
    }
    else {
        buffer_sprintf(error, "the rules array is missing");
        return false;
    }

    return true;
}

static RRD_ALERT_PROTOTYPE *health_prototype_payload_parse(const char *payload, size_t payload_len, BUFFER *error) {
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

    if(!parse_prototype(jobj, base, error))
        goto cleanup;

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
            RRD_ALERT_PROTOTYPE *nap = health_prototype_payload_parse(buffer_tostring(payload), buffer_strlen(payload), error);
            if(!nap)
                code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, buffer_tostring(error));
            else if(string_strcmp(nap->config.name, add_name) != 0) {
                code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "the name of the alert in the payload does not match the name of alert trying to add");
                health_prototype_free(nap);
            }
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
                dictionary_acquired_item_release(health_globals.prototypes.dict, item);

                code = dyncfg_default_response(result, DYNCFG_RESP_ACCEPTED, "accepted");
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

static int dyncfg_health_prototype_action(BUFFER *result, DYNCFG_CMDS cmd, BUFFER *payload, const char *source __maybe_unused, const char *alert_name) {
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
                ap->_internal.enabled = true;
                dyncfg_health_prototype_reapply(ap);
                dyncfg_status(localhost, alert_name_dyncfg, DYNCFG_STATUS_ACCEPTED);
                code = dyncfg_default_response(result, DYNCFG_RESP_ACCEPTED, "enabled");
            }
            break;

        case DYNCFG_CMD_UPDATE: {
                CLEAN_BUFFER *error = buffer_create(0, NULL);
                RRD_ALERT_PROTOTYPE *nap = health_prototype_payload_parse(buffer_tostring(payload), buffer_strlen(payload), error);
                if(!nap)
                    code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, buffer_tostring(error));
                else if(ap->config.name != nap->config.name) {
                    code = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "name cannot be changed, add a new alert and remove the old one");
                    health_prototype_free(nap);
                }
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
                    code = dyncfg_default_response(result, DYNCFG_RESP_ACCEPTED, "updated");
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
    if(!type_prefix || !*type_prefix || strcmp(type_prefix, "prototype") != 0)
        return dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, "third component of id is not 'prototype'");

    char *alert_name = get_word(words, num_words, i++);
    if(!alert_name || !*alert_name) {
        // action on the prototype template

        code = dyncfg_health_prototype_template_action(result, cmd, add_name, payload, source);
    }
    else {
        // action on a specific alert prototype

        code = dyncfg_health_prototype_action(result, cmd, payload, source, alert_name);
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
               ap->match.enabled ? DYNCFG_STATUS_ACCEPTED : DYNCFG_STATUS_DISABLED, DYNCFG_TYPE_JOB,
               ap->config.source_type, string2str(ap->config.source),
               DYNCFG_CMD_SCHEMA | DYNCFG_CMD_GET | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE |
                   DYNCFG_CMD_UPDATE | DYNCFG_CMD_TEST |
                   (ap->config.source_type == DYNCFG_SOURCE_TYPE_DYNCFG && !ap->_internal.is_on_disk ? DYNCFG_CMD_REMOVE : 0),
               dyncfg_health_cb, NULL);

#ifdef NETDATA_TEST_HEALTH_PROTOTYPES_JSON_AND_PARSING
    {
        // make sure we can generate valid json, parse it back and come up to the same object

        CLEAN_BUFFER *original = buffer_create(0, NULL);
        CLEAN_BUFFER *parsed = buffer_create(0, NULL);
        CLEAN_BUFFER *error = buffer_create(0, NULL);
        health_prototype_to_json(original, ap, true);
        RRD_ALERT_PROTOTYPE *t = health_prototype_payload_parse(buffer_tostring(original), buffer_strlen(original), error);
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
               dyncfg_health_cb, NULL);

    dfe_start_read(health_globals.prototypes.dict, ap) {
        if(ap->config.source_type != DYNCFG_SOURCE_TYPE_DYNCFG)
            health_dyncfg_register_prototype(ap);
    }
    dfe_done(ap);
}
