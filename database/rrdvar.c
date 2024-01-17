// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

typedef struct rrdvar {
    NETDATA_DOUBLE value;
} RRDVAR;

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

static bool rrdvar_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    RRDVAR *rv = old_value;
    RRDVAR *nrv = new_value;

    rv->value = nrv->value;
    return false;
}

DICTIONARY *rrdvariables_create(void) {
    DICTIONARY *dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                  &dictionary_stats_category_rrdhealth, sizeof(RRDVAR));
    dictionary_register_conflict_callback(dict, rrdvar_conflict_callback, NULL);
    return dict;
}

void rrdvariables_destroy(DICTIONARY *dict) {
    dictionary_destroy(dict);
}

static inline const RRDVAR_ACQUIRED *rrdvar_get_and_acquire(DICTIONARY *dict, STRING *name) {
    return (const RRDVAR_ACQUIRED *)dictionary_get_and_acquire_item_advanced(dict, string2str(name), (ssize_t)string_strlen(name));
}

inline const RRDVAR_ACQUIRED *rrdvar_add_and_acquire(DICTIONARY *dict, STRING *name, NETDATA_DOUBLE value) {
    if(unlikely(!dict || !name)) return NULL;
    RRDVAR tmp = {
        .value = value,
    };
    return (const RRDVAR_ACQUIRED *)dictionary_set_and_acquire_item_advanced(
        dict, string2str(name), (ssize_t)string_strlen(name),
        &tmp, sizeof(tmp), NULL);
}

void rrdvar_delete_all(DICTIONARY *dict) {
    dictionary_flush(dict);
}

void rrdvar_release(DICTIONARY *dict, const RRDVAR_ACQUIRED *rva) {
    if(unlikely(!dict || !rva)) return;  // when health is not enabled
    dictionary_acquired_item_release(dict, (const DICTIONARY_ITEM *)rva);
}

// ----------------------------------------------------------------------------
// CUSTOM HOST VARIABLES

inline int rrdvar_walkthrough_read(DICTIONARY *dict, int (*callback)(const DICTIONARY_ITEM *item, void *rrdvar, void *data), void *data) {
    if(unlikely(!dict)) return 0;  // when health is not enabled
    return dictionary_walkthrough_read(dict, callback, data);
}

const RRDVAR_ACQUIRED *rrdvar_host_variable_add_and_acquire(RRDHOST *host, const char *name) {
    if(unlikely(!host->rrdvars)) return NULL; // when health is not enabled

    STRING *name_string = rrdvar_name_to_string(name);
    const RRDVAR_ACQUIRED *rva = rrdvar_add_and_acquire(host->rrdvars, name_string, NAN);

    string_freez(name_string);
    return rva;
}

void rrdvar_host_variable_set(RRDHOST *host, const RRDVAR_ACQUIRED *rva, NETDATA_DOUBLE value) {
    if(unlikely(!host->rrdvars || !rva)) return; // when health is not enabled

    RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);
    if(rv->value != value) {
        rv->value = value;

        // if the host is streaming, send this variable upstream immediately
        rrdpush_sender_send_this_host_variable_now(host, rva);
    }
}

// ----------------------------------------------------------------------------
// CUSTOM CHART VARIABLES

const RRDVAR_ACQUIRED *rrdvar_chart_variable_add_and_acquire(RRDSET *st, const char *name) {
    if(unlikely(!st->rrdvars)) return NULL;

    STRING *name_string = rrdvar_name_to_string(name);
    const RRDVAR_ACQUIRED *rs = rrdvar_add_and_acquire(st->rrdvars, name_string, NAN);
    string_freez(name_string);
    return rs;
}

void rrdvar_chart_variable_set(RRDSET *st, const RRDVAR_ACQUIRED *rva, NETDATA_DOUBLE value) {
    if(unlikely(!st->rrdvars || !rva)) return;

    RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);
    if(rv->value != value) {
        rv->value = value;
        rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_SEND_VARIABLES);
    }
}

// ----------------------------------------------------------------------------
// RRDVAR lookup

NETDATA_DOUBLE rrdvar2number(const RRDVAR_ACQUIRED *rva) {
    if(unlikely(!rva)) return NAN;
    RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);
    return rv->value;
}

static inline bool rrdvar_get_value(DICTIONARY *dict, STRING *variable, NETDATA_DOUBLE *result) {
    bool found = false;

    const RRDVAR_ACQUIRED *rva = rrdvar_get_and_acquire(dict, variable);
    if(rva) {
        *result = rrdvar2number(rva);
        found = true;
        dictionary_acquired_item_release(dict, (const DICTIONARY_ITEM *)rva);
    }

    return found;
}

bool rrdvar_get_custom_host_variable_value(RRDHOST *host, STRING *variable, NETDATA_DOUBLE *result) {
    return rrdvar_get_value(host->rrdvars, variable, result);
}

bool rrdvar_get_custom_chart_variable_value(RRDSET *st, STRING *variable, NETDATA_DOUBLE *result) {
    return rrdvar_get_value(st->rrdvars, variable, result);
}

// ----------------------------------------------------------------------------
// RRDVAR to JSON

struct variable2json_helper {
    BUFFER *buf;
};

static int single_variable2json_callback(const DICTIONARY_ITEM *item __maybe_unused, void *entry __maybe_unused, void *helper_data) {
    struct variable2json_helper *helper = (struct variable2json_helper *)helper_data;
    const RRDVAR_ACQUIRED *rva = (const RRDVAR_ACQUIRED *)item;
    NETDATA_DOUBLE value = rrdvar2number(rva);

    if(unlikely(isnan(value) || isinf(value)))
        buffer_json_member_add_string(helper->buf, rrdvar_name(rva), NULL);
    else
        buffer_json_member_add_double(helper->buf, rrdvar_name(rva), (NETDATA_DOUBLE)value);

    return 0;
}

void health_api_v1_chart_custom_variables2json(RRDSET *st, BUFFER *buf) {
    struct variable2json_helper helper = {.buf = buf };

    rrdvar_walkthrough_read(st->rrdvars, single_variable2json_callback, &helper);
}

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *buf) {
    RRDHOST *host = st->rrdhost;

    struct variable2json_helper helper = {.buf = buf };

    buffer_json_initialize(buf, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(buf, "chart", rrdset_id(st));
    buffer_json_member_add_string(buf, "chart_name", rrdset_name(st));
    buffer_json_member_add_string(buf, "chart_context", rrdset_context(st));

    {
        buffer_json_member_add_object(buf, "chart_variables");
        rrdvar_walkthrough_read(st->rrdvars, single_variable2json_callback, &helper);
        buffer_json_object_close(buf);
    }

    buffer_json_member_add_string(buf, "family", rrdset_family(st));

    buffer_json_member_add_string(buf, "host", rrdhost_hostname(host));

    {
        buffer_json_member_add_object(buf, "host_variables");
        rrdvar_walkthrough_read(host->rrdvars, single_variable2json_callback, &helper);
        buffer_json_object_close(buf);
    }

    buffer_json_finalize(buf);
}

// ----------------------------------------------------------------------------
// RRDVAR private members examination

const char *rrdvar_name(const RRDVAR_ACQUIRED *rva) {
    return dictionary_acquired_item_name((const DICTIONARY_ITEM *)rva);
}


void rrdvar_print_to_streaming_custom_chart_variables(RRDSET *st, BUFFER *wb) {
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_SEND_VARIABLES);

    // send the chart local custom variables
    RRDVAR *rv;
    dfe_start_read(st->rrdvars, rv) {
        buffer_sprintf(wb
                       , "VARIABLE CHART %s = " NETDATA_DOUBLE_FORMAT "\n"
                       , rv_dfe.name, rv->value
        );
    }
    dfe_done(rv);
}
