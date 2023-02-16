// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

// the variables as stored in the variables indexes
// there are 3 indexes:
// 1. at each chart   (RRDSET.rrdvar_root_index)
// 2. at each context (RRDFAMILY.rrdvar_root_index)
// 3. at each host    (RRDHOST.rrdvar_root_index)
typedef struct rrdvar {
    STRING *name;
    void *value;
    RRDVAR_FLAGS flags:24;
    RRDVAR_TYPE type:8;
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

struct rrdvar_constructor {
    STRING *name;
    void *value;
    RRDVAR_FLAGS options:16;
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
    rv->flags = ctr->options;

    if(!ctr->value) {
        NETDATA_DOUBLE *v = mallocz(sizeof(NETDATA_DOUBLE));
        *v = NAN;
        rv->value = v;
        rv->flags |= RRDVAR_FLAG_ALLOCATED;
    }
    else
        rv->value = ctr->value;

    ctr->react_action = RRDVAR_REACT_NEW;
}

static void rrdvar_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdvar, void *nothing __maybe_unused) {
    RRDVAR *rv = rrdvar;

    if(rv->flags & RRDVAR_FLAG_ALLOCATED)
        freez(rv->value);

    string_freez(rv->name);
    rv->name = NULL;
}

DICTIONARY *rrdvariables_create(void) {
    DICTIONARY *dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                  &dictionary_stats_category_rrdhealth, sizeof(RRDVAR));

    dictionary_register_insert_callback(dict, rrdvar_insert_callback, NULL);
    dictionary_register_delete_callback(dict, rrdvar_delete_callback, NULL);

    return dict;
}

DICTIONARY *health_rrdvariables_create(void) {
    DICTIONARY *dict = dictionary_create_advanced(DICT_OPTION_NONE, &dictionary_stats_category_rrdhealth, 0);

    dictionary_register_insert_callback(dict, rrdvar_insert_callback, NULL);
    dictionary_register_delete_callback(dict, rrdvar_delete_callback, NULL);

    return dict;
}

void rrdvariables_destroy(DICTIONARY *dict) {
    dictionary_destroy(dict);
}

static inline const RRDVAR_ACQUIRED *rrdvar_get_and_acquire(DICTIONARY *dict, STRING *name) {
    return (const RRDVAR_ACQUIRED *)dictionary_get_and_acquire_item_advanced(dict, string2str(name), (ssize_t)string_strlen(name) + 1);
}

inline void rrdvar_release_and_del(DICTIONARY *dict, const RRDVAR_ACQUIRED *rva) {
    if(unlikely(!dict || !rva)) return;

    RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);

    dictionary_del_advanced(dict, string2str(rv->name), (ssize_t)string_strlen(rv->name) + 1);

    dictionary_acquired_item_release(dict, (const DICTIONARY_ITEM *)rva);
}

inline const RRDVAR_ACQUIRED *rrdvar_add_and_acquire(const char *scope __maybe_unused, DICTIONARY *dict, STRING *name, RRDVAR_TYPE type, RRDVAR_FLAGS options, void *value) {
    if(unlikely(!dict || !name)) return NULL;

    struct rrdvar_constructor tmp = {
        .name = name,
        .value = value,
        .type = type,
        .options = options,
        .react_action = RRDVAR_REACT_NONE,
    };
    return (const RRDVAR_ACQUIRED *)dictionary_set_and_acquire_item_advanced(dict, string2str(name), (ssize_t)string_strlen(name) + 1, NULL, sizeof(RRDVAR), &tmp);
}

inline void rrdvar_add(const char *scope __maybe_unused, DICTIONARY *dict, STRING *name, RRDVAR_TYPE type, RRDVAR_FLAGS options, void *value) {
    if(unlikely(!dict || !name)) return;

    struct rrdvar_constructor tmp = {
        .name = name,
        .value = value,
        .type = type,
        .options = options,
        .react_action = RRDVAR_REACT_NONE,
    };
    dictionary_set_advanced(dict, string2str(name), (ssize_t)string_strlen(name) + 1, NULL, sizeof(RRDVAR), &tmp);
}

void rrdvar_delete_all(DICTIONARY *dict) {
    dictionary_flush(dict);
}


// ----------------------------------------------------------------------------
// CUSTOM HOST VARIABLES

inline int rrdvar_walkthrough_read(DICTIONARY *dict, int (*callback)(const DICTIONARY_ITEM *item, void *rrdvar, void *data), void *data) {
    if(unlikely(!dict)) return 0;  // when health is not enabled
    return dictionary_walkthrough_read(dict, callback, data);
}

const RRDVAR_ACQUIRED *rrdvar_custom_host_variable_add_and_acquire(RRDHOST *host, const char *name) {
    DICTIONARY *dict = host->rrdvars;
    if(unlikely(!dict)) return NULL; // when health is not enabled

    STRING *name_string = rrdvar_name_to_string(name);

    const RRDVAR_ACQUIRED *rva = rrdvar_add_and_acquire("host", dict, name_string, RRDVAR_TYPE_CALCULATED, RRDVAR_FLAG_CUSTOM_HOST_VAR, NULL);

    string_freez(name_string);
    return rva;
}

void rrdvar_custom_host_variable_set(RRDHOST *host, const RRDVAR_ACQUIRED *rva, NETDATA_DOUBLE value) {
    if(unlikely(!host->rrdvars || !rva)) return; // when health is not enabled

    if(rrdvar_type(rva) != RRDVAR_TYPE_CALCULATED || !(rrdvar_flags(rva) & (RRDVAR_FLAG_CUSTOM_HOST_VAR | RRDVAR_FLAG_ALLOCATED)))
        error("requested to set variable '%s' to value " NETDATA_DOUBLE_FORMAT " but the variable is not a custom one.", rrdvar_name(rva), value);
    else {
        RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);
        NETDATA_DOUBLE *v = rv->value;
        if(*v != value) {
            *v = value;

            // if the host is streaming, send this variable upstream immediately
            rrdpush_sender_send_this_host_variable_now(host, rva);
        }
    }
}

void rrdvar_release(DICTIONARY *dict, const RRDVAR_ACQUIRED *rva) {
    if(unlikely(!dict || !rva)) return;  // when health is not enabled
    dictionary_acquired_item_release(dict, (const DICTIONARY_ITEM *)rva);
}

// ----------------------------------------------------------------------------
// RRDVAR lookup

NETDATA_DOUBLE rrdvar2number(const RRDVAR_ACQUIRED *rva) {
    if(unlikely(!rva)) return NAN;

    RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);

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

int health_variable_check(DICTIONARY *dict, RRDSET *st, RRDDIM *rd) {
    if (!dict || !st || !rd) return 0;

    STRING *helper_str;
    char helper[RRDVAR_MAX_LENGTH + 1];
    snprintfz(helper, RRDVAR_MAX_LENGTH, "%s.%s", string2str(st->name), string2str(rd->name));
    helper_str = string_strdupz(helper);

    const RRDVAR_ACQUIRED *rva;
    rva = rrdvar_get_and_acquire(dict, helper_str);
    if(rva) {
        dictionary_acquired_item_release(dict, (const DICTIONARY_ITEM *)rva);
        string_freez(helper_str);
        return 1;
    }

    string_freez(helper_str);

    return 0;
}

void rrdvar_store_for_chart(RRDHOST *host, RRDSET *st) {
    if (!st) return;

    if(!st->rrdfamily)
        st->rrdfamily = rrdfamily_add_and_acquire(host, rrdset_family(st));

    if(!st->rrdvars)
        st->rrdvars = rrdvariables_create();

    rrddimvar_index_init(st);

    rrdsetvar_add_and_leave_released(st, "last_collected_t", RRDVAR_TYPE_TIME_T, &st->last_collected_time.tv_sec, RRDVAR_FLAG_NONE);
    rrdsetvar_add_and_leave_released(st, "green", RRDVAR_TYPE_CALCULATED, &st->green, RRDVAR_FLAG_NONE);
    rrdsetvar_add_and_leave_released(st, "red", RRDVAR_TYPE_CALCULATED, &st->red, RRDVAR_FLAG_NONE);
    rrdsetvar_add_and_leave_released(st, "update_every", RRDVAR_TYPE_INT, &st->update_every, RRDVAR_FLAG_NONE);

    RRDDIM *rd;
    rrddim_foreach_read(rd, st) {
        rrddimvar_add_and_leave_released(rd, RRDVAR_TYPE_CALCULATED, NULL, NULL, &rd->last_stored_value, RRDVAR_FLAG_NONE);
        rrddimvar_add_and_leave_released(rd, RRDVAR_TYPE_COLLECTED, NULL, "_raw", &rd->last_collected_value, RRDVAR_FLAG_NONE);
        rrddimvar_add_and_leave_released(rd, RRDVAR_TYPE_TIME_T, NULL, "_last_collected_t", &rd->last_collected_time.tv_sec, RRDVAR_FLAG_NONE);
    }
    rrddim_foreach_done(rd);
}

int health_variable_lookup(STRING *variable, RRDCALC *rc, NETDATA_DOUBLE *result) {
    RRDSET *st = rc->rrdset;
    if(!st) return 0;

    RRDHOST *host = st->rrdhost;
    const RRDVAR_ACQUIRED *rva;

    rva = rrdvar_get_and_acquire(st->rrdvars, variable);
    if(rva) {
        *result = rrdvar2number(rva);
        dictionary_acquired_item_release(st->rrdvars, (const DICTIONARY_ITEM *)rva);
        return 1;
    }

    rva = rrdvar_get_and_acquire(rrdfamily_rrdvars_dict(st->rrdfamily), variable);
    if(rva) {
        *result = rrdvar2number(rva);
        dictionary_acquired_item_release(rrdfamily_rrdvars_dict(st->rrdfamily), (const DICTIONARY_ITEM *)rva);
        return 1;
    }

    rva = rrdvar_get_and_acquire(host->rrdvars, variable);
    if(rva) {
        *result = rrdvar2number(rva);
        dictionary_acquired_item_release(host->rrdvars, (const DICTIONARY_ITEM *)rva);
        return 1;
    }

    return 0;
}

// ----------------------------------------------------------------------------
// RRDVAR to JSON

struct variable2json_helper {
    BUFFER *buf;
    size_t counter;
    RRDVAR_FLAGS options;
};

static int single_variable2json_callback(const DICTIONARY_ITEM *item __maybe_unused, void *entry __maybe_unused, void *helper_data) {
    struct variable2json_helper *helper = (struct variable2json_helper *)helper_data;
    const RRDVAR_ACQUIRED *rva = (const RRDVAR_ACQUIRED *)item;
    NETDATA_DOUBLE value = rrdvar2number(rva);

    if (helper->options == RRDVAR_FLAG_NONE || rrdvar_flags(rva) & helper->options) {
        if(unlikely(isnan(value) || isinf(value)))
            buffer_sprintf(helper->buf, "%s\n\t\t\"%s\": null", helper->counter?",":"", rrdvar_name(rva));
        else
            buffer_sprintf(helper->buf, "%s\n\t\t\"%s\": %0.5" NETDATA_DOUBLE_MODIFIER, helper->counter?",":"", rrdvar_name(rva), (NETDATA_DOUBLE)value);

        helper->counter++;
    }

    return 0;
}

void health_api_v1_chart_custom_variables2json(RRDSET *st, BUFFER *buf) {
    struct variable2json_helper helper = {
            .buf = buf,
            .counter = 0,
            .options = RRDVAR_FLAG_CUSTOM_CHART_VAR};

    buffer_sprintf(buf, "{");
    rrdvar_walkthrough_read(st->rrdvars, single_variable2json_callback, &helper);
    buffer_strcat(buf, "\n\t\t\t}");
}

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *buf) {
    RRDHOST *host = st->rrdhost;

    struct variable2json_helper helper = {
            .buf = buf,
            .counter = 0,
            .options = RRDVAR_FLAG_NONE};

    buffer_sprintf(buf, "{\n\t\"chart\": \"%s\",\n\t\"chart_name\": \"%s\",\n\t\"chart_context\": \"%s\",\n\t\"chart_variables\": {", rrdset_id(st), rrdset_name(st), rrdset_context(st));
    rrdvar_walkthrough_read(st->rrdvars, single_variable2json_callback, &helper);

    buffer_sprintf(buf, "\n\t},\n\t\"family\": \"%s\",\n\t\"family_variables\": {", rrdset_family(st));
    helper.counter = 0;
    rrdvar_walkthrough_read(rrdfamily_rrdvars_dict(st->rrdfamily), single_variable2json_callback, &helper);

    buffer_sprintf(buf, "\n\t},\n\t\"host\": \"%s\",\n\t\"host_variables\": {", rrdhost_hostname(host));
    helper.counter = 0;
    rrdvar_walkthrough_read(host->rrdvars, single_variable2json_callback, &helper);

    buffer_strcat(buf, "\n\t}\n}\n");
}

// ----------------------------------------------------------------------------
// RRDVAR private members examination

const char *rrdvar_name(const RRDVAR_ACQUIRED *rva) {
    return dictionary_acquired_item_name((const DICTIONARY_ITEM *)rva);
}

RRDVAR_FLAGS rrdvar_flags(const RRDVAR_ACQUIRED *rva) {
    RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);
    return rv->flags;
}
RRDVAR_TYPE rrdvar_type(const RRDVAR_ACQUIRED *rva) {
    RRDVAR *rv = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rva);
    return rv->type;
}
