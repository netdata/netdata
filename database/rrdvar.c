// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

// ----------------------------------------------------------------------------
// RRDVAR management

inline int rrdvar_fix_name(char *variable) {
    int fixed = 0;
    while(*variable) {
        if (!isalnum(*variable) && *variable != '.' && *variable != '_') {
            *variable++ = '_';
            fixed++;
        }
        else
            variable++;
    }

    return fixed;
}

inline STRING *rrdvar_name_to_string(const char *name) {
    char *variable = strdupz(name);
    rrdvar_fix_name(variable);
    STRING *name_string = string_strdupz(variable);
    freez(variable);
    return name_string;
}

struct rrdvar_constructor {
    STRING *name;
    void *value;
    RRDVAR_OPTIONS options:16;
    RRDVAR_TYPE type:8;

    enum {
        RRDVAR_REACT_NONE    = 0,
        RRDVAR_REACT_NEW     = (1 << 0),
    } react_action;
};

static void rrdvar_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdvar, void *constructor_data) {
    RRDVAR *rv = rrdvar;
    struct rrdvar_constructor *ctr = constructor_data;

    ctr->options &= ~RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS;

    rv->name = string_dup(ctr->name);
    rv->type = ctr->type;
    rv->options = ctr->options;

    if(!ctr->value) {
        NETDATA_DOUBLE *v = mallocz(sizeof(NETDATA_DOUBLE));
        *v = NAN;
        rv->value = v;
        rv->options |= RRDVAR_OPTION_ALLOCATED;
    }
    else
        rv->value = ctr->value;

    rv->last_updated = now_realtime_sec();

    ctr->react_action = RRDVAR_REACT_NEW;
}

static void rrdvar_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdvar, void *nothing __maybe_unused) {
    RRDVAR *rv = rrdvar;

    if(rv->options & RRDVAR_OPTION_ALLOCATED)
        freez(rv->value);

    string_freez(rv->name);
    rv->name = NULL;
}

DICTIONARY *rrdvariables_create(void) {
    DICTIONARY *dict = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);

    dictionary_register_insert_callback(dict, rrdvar_insert_callback, NULL);
    dictionary_register_delete_callback(dict, rrdvar_delete_callback, NULL);

    return dict;
}

void rrdvariables_destroy(DICTIONARY *dict) {
    dictionary_destroy(dict);
}

static inline RRDVAR *rrdvar_get(DICTIONARY *dict, STRING *name) {
    return dictionary_get_advanced(dict, string2str(name), (ssize_t)string_strlen(name));
}

inline void rrdvar_delete(DICTIONARY *dict, RRDVAR *rv) {
    if(unlikely(!dict || !rv)) return;

    if(dictionary_del(dict, rrdvar_name(rv)) != 0)
        error("Request to remove RRDVAR '%s' from index failed. Not Found.", rrdvar_name(rv));
}

inline RRDVAR *rrdvar_add(const char *scope __maybe_unused, DICTIONARY *dict, STRING *name, RRDVAR_TYPE type, RRDVAR_OPTIONS options, void *value) {

    struct rrdvar_constructor tmp = {
        .name = name,
        .value = value,
        .type = type,
        .options = options,
        .react_action = RRDVAR_REACT_NONE,
    };
    RRDVAR *rv = dictionary_set_advanced(dict, string2str(name), (ssize_t)string_strlen(name) + 1, NULL, sizeof(RRDVAR), &tmp);

    if(!(tmp.react_action & RRDVAR_REACT_NEW))
        rv = NULL;

    return rv;
}

// ----------------------------------------------------------------------------
// CUSTOM HOST VARIABLES

inline int rrdvar_walkthrough_read(DICTIONARY *dict, int (*callback)(const DICTIONARY_ITEM *item, void *rrdvar, void *data), void *data) {
    return dictionary_walkthrough_read(dict, callback, data);
}

RRDVAR *rrdvar_custom_host_variable_create(RRDHOST *host, const char *name) {
    DICTIONARY *dict = host->rrdvars;

    STRING *name_string = rrdvar_name_to_string(name);

    RRDVAR *rv = rrdvar_add("host", dict, name_string, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_CUSTOM_HOST_VAR, NULL);

    if(unlikely(!rv)) {
        debug(D_VARIABLES, "Requested variable '%s' already exists - possibly 2 plugins are updating it at the same time.", string2str(name_string));

        // find the existing one to return it
        rv = rrdvar_get(dict, name_string);
    }

    string_freez(name_string);
    return rv;
}

void rrdvar_custom_host_variable_set(RRDHOST *host, RRDVAR *rv, NETDATA_DOUBLE value) {
    if(rv->type != RRDVAR_TYPE_CALCULATED || !(rv->options & RRDVAR_OPTION_CUSTOM_HOST_VAR) || !(rv->options & RRDVAR_OPTION_ALLOCATED))
        error("requested to set variable '%s' to value " NETDATA_DOUBLE_FORMAT " but the variable is not a custom one.", rrdvar_name(rv), value);
    else {
        NETDATA_DOUBLE *v = rv->value;
        if(*v != value) {
            *v = value;

            rv->last_updated = now_realtime_sec();

            // if the host is streaming, send this variable upstream immediately
            rrdpush_sender_send_this_host_variable_now(host, rv);
        }
    }
}

// ----------------------------------------------------------------------------
// RRDVAR lookup

NETDATA_DOUBLE rrdvar2number(RRDVAR *rv) {
    switch(rv->type) {
        case RRDVAR_TYPE_CALCULATED: {
            NETDATA_DOUBLE *n = (NETDATA_DOUBLE *)rv->value;
            return *n;
        }

        case RRDVAR_TYPE_TIME_T: {
            time_t *n = (time_t *)rv->value;
            return (NETDATA_DOUBLE)*n;
        }

        case RRDVAR_TYPE_COLLECTED: {
            collected_number *n = (collected_number *)rv->value;
            return (NETDATA_DOUBLE)*n;
        }

        case RRDVAR_TYPE_TOTAL: {
            total_number *n = (total_number *)rv->value;
            return (NETDATA_DOUBLE)*n;
        }

        case RRDVAR_TYPE_INT: {
            int *n = (int *)rv->value;
            return *n;
        }

        default:
            error("I don't know how to convert RRDVAR type %u to NETDATA_DOUBLE", rv->type);
            return NAN;
    }
}

int health_variable_lookup(STRING *variable, RRDCALC *rc, NETDATA_DOUBLE *result) {
    RRDSET *st = rc->rrdset;
    if(!st) return 0;

    RRDHOST *host = st->rrdhost;
    RRDVAR *rv;

    rv = rrdvar_get(st->rrdvars, variable);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    rv = rrdvar_get(st->rrdfamily->rrdvars, variable);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    rv = rrdvar_get(host->rrdvars, variable);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    return 0;
}

// ----------------------------------------------------------------------------
// RRDVAR to JSON

struct variable2json_helper {
    BUFFER *buf;
    size_t counter;
    RRDVAR_OPTIONS options;
};

static int single_variable2json(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *data) {
    struct variable2json_helper *helper = (struct variable2json_helper *)data;
    RRDVAR *rv = (RRDVAR *)entry;
    NETDATA_DOUBLE value = rrdvar2number(rv);

    if (helper->options == RRDVAR_OPTION_DEFAULT || rv->options & helper->options) {
        if(unlikely(isnan(value) || isinf(value)))
            buffer_sprintf(helper->buf, "%s\n\t\t\"%s\": null", helper->counter?",":"", rrdvar_name(rv));
        else
            buffer_sprintf(helper->buf, "%s\n\t\t\"%s\": %0.5" NETDATA_DOUBLE_MODIFIER, helper->counter?",":"", rrdvar_name(rv), (NETDATA_DOUBLE)value);

        helper->counter++;
    }

    return 0;
}

void health_api_v1_chart_custom_variables2json(RRDSET *st, BUFFER *buf) {
    struct variable2json_helper helper = {
            .buf = buf,
            .counter = 0,
            .options = RRDVAR_OPTION_CUSTOM_CHART_VAR
    };

    buffer_sprintf(buf, "{");
    rrdvar_walkthrough_read(st->rrdvars, single_variable2json, &helper);
    buffer_strcat(buf, "\n\t\t\t}");
}

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *buf) {
    RRDHOST *host = st->rrdhost;

    struct variable2json_helper helper = {
            .buf = buf,
            .counter = 0,
            .options = RRDVAR_OPTION_DEFAULT
    };

    buffer_sprintf(buf, "{\n\t\"chart\": \"%s\",\n\t\"chart_name\": \"%s\",\n\t\"chart_context\": \"%s\",\n\t\"chart_variables\": {", rrdset_id(st), rrdset_name(st), rrdset_context(st));
    rrdvar_walkthrough_read(st->rrdvars, single_variable2json, &helper);

    buffer_sprintf(buf, "\n\t},\n\t\"family\": \"%s\",\n\t\"family_variables\": {", rrdset_family(st));
    helper.counter = 0;
    rrdvar_walkthrough_read(st->rrdfamily->rrdvars, single_variable2json, &helper);

    buffer_sprintf(buf, "\n\t},\n\t\"host\": \"%s\",\n\t\"host_variables\": {", rrdhost_hostname(host));
    helper.counter = 0;
    rrdvar_walkthrough_read(host->rrdvars, single_variable2json, &helper);

    buffer_strcat(buf, "\n\t}\n}\n");
}

