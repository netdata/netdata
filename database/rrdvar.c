// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_HEALTH_INTERNALS
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

static inline RRDVAR *rrdvar_index_add(DICTIONARY *dict, RRDVAR *rv) {
    return dictionary_set(dict, rrdvar_name(rv), rv, sizeof(RRDVAR));
}

static inline RRDVAR *rrdvar_index_del(DICTIONARY *dict, RRDVAR *rv) {
    if(dictionary_del(dict, rrdvar_name(rv)) != 0) {
        error("Request to remove RRDVAR '%s' from index failed. Not Found.", rrdvar_name(rv));
        return NULL;
    }

    return rv;
}

static inline RRDVAR *rrdvar_index_find(DICTIONARY *dict, STRING *name) {
    return dictionary_get(dict, string2str(name));
}

inline void rrdvar_free(RRDHOST *host, DICTIONARY *dict, RRDVAR *rv) {
    (void)host;

    if(!rv) return;

    if(dict) {
        debug(D_VARIABLES, "Deleting variable '%s'", rrdvar_name(rv));
        if(unlikely(!rrdvar_index_del(dict, rv)))
            error("RRDVAR: Attempted to delete variable '%s' from host '%s', but it is not found.", rrdvar_name(rv), rrdhost_hostname(host));
    }

    if(rv->options & RRDVAR_OPTION_ALLOCATED)
        freez(rv->value);

    string_freez(rv->name);
    freez(rv);
}

inline STRING *rrdvar_name_to_string(const char *name) {
    char *variable = strdupz(name);
    rrdvar_fix_name(variable);
    STRING *name_string = string_strdupz(variable);
    freez(variable);
    return name_string;
}

inline RRDVAR *rrdvar_create_and_index(const char *scope __maybe_unused, DICTIONARY *dict, STRING *name,
                                       RRDVAR_TYPE type, RRDVAR_OPTIONS options, void *value) {

    RRDVAR *rv = rrdvar_index_find(dict, name);
    if(unlikely(!rv)) {
        debug(D_VARIABLES, "Variable '%s' not found in scope '%s'. Creating a new one.", string2str(name), scope);

        rv = callocz(1, sizeof(RRDVAR));
        rv->name = string_dup(name);
        rv->type = type;
        rv->options = options;
        rv->value = value;
        rv->last_updated = now_realtime_sec();

        RRDVAR *ret = rrdvar_index_add(dict, rv);
        if(unlikely(ret != rv)) {
            debug(D_VARIABLES, "Variable '%s' in scope '%s' already exists", string2str(name), scope);
            freez(rv);
            rv = NULL;
        }
        else
            debug(D_VARIABLES, "Variable '%s' created in scope '%s'", string2str(name), scope);
    }
    else {
        debug(D_VARIABLES, "Variable '%s' is already found in scope '%s'.", string2str(name), scope);

        // this is important
        // it must return NULL - not the existing variable - or double-free will happen
        rv = NULL;
    }

    return rv;
}

void rrdvar_free_remaining_variables(RRDHOST *host, DICTIONARY *dict) {
    RRDVAR *rv;
    dfe_start_rw(dict, rv, DICTIONARY_LOCK_REETRANT) {
        rrdvar_free(host, dict, rv);
    }
    dfe_done(rv);
}

// ----------------------------------------------------------------------------
// CUSTOM HOST VARIABLES

inline int rrdvar_walkthrough_read(DICTIONARY *dict, int (*callback)(const char *name, void *rrdvar, void *data), void *data) {
    return dictionary_walkthrough_read(dict, callback, data);
}

static RRDVAR *rrdvar_custom_variable_create(const char *scope, DICTIONARY *dict, const char *name) {
    NETDATA_DOUBLE *v = callocz(1, sizeof(NETDATA_DOUBLE));
    *v = NAN;

    STRING *name_string = rrdvar_name_to_string(name);

    RRDVAR *rv = rrdvar_create_and_index(scope, dict, name_string, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_CUSTOM_HOST_VAR|RRDVAR_OPTION_ALLOCATED, v);
    if(unlikely(!rv)) {
        freez(v);
        debug(D_VARIABLES, "Requested variable '%s' already exists - possibly 2 plugins are updating it at the same time.", string2str(name_string));

        // find the existing one to return it
        rv = rrdvar_index_find(dict, name_string);
    }

    string_freez(name_string);

    return rv;
}

RRDVAR *rrdvar_custom_host_variable_create(RRDHOST *host, const char *name) {
    return rrdvar_custom_variable_create("host", host->rrdvar_root_index, name);
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
            return *n;
        }

        case RRDVAR_TYPE_COLLECTED: {
            collected_number *n = (collected_number *)rv->value;
            return *n;
        }

        case RRDVAR_TYPE_TOTAL: {
            total_number *n = (total_number *)rv->value;
            return *n;
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

    rv = rrdvar_index_find(st->rrdvar_root_index, variable);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    rv = rrdvar_index_find(st->rrdfamily->rrdvar_root_index, variable);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    rv = rrdvar_index_find(host->rrdvar_root_index, variable);
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

static int single_variable2json(const char *name __maybe_unused, void *entry, void *data) {
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
    rrdvar_walkthrough_read(st->rrdvar_root_index, single_variable2json, &helper);
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
    rrdvar_walkthrough_read(st->rrdvar_root_index, single_variable2json, &helper);

    buffer_sprintf(buf, "\n\t},\n\t\"family\": \"%s\",\n\t\"family_variables\": {", rrdset_family(st));
    helper.counter = 0;
    rrdvar_walkthrough_read(st->rrdfamily->rrdvar_root_index, single_variable2json, &helper);

    buffer_sprintf(buf, "\n\t},\n\t\"host\": \"%s\",\n\t\"host_variables\": {", rrdhost_hostname(host));
    helper.counter = 0;
    rrdvar_walkthrough_read(host->rrdvar_root_index, single_variable2json, &helper);

    buffer_strcat(buf, "\n\t}\n}\n");
}

