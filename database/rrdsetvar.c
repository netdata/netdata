// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_HEALTH_INTERNALS
#include "rrd.h"

static inline void rrdsetvar_free_variables(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;
    RRDHOST *host = st->rrdhost;

    // ------------------------------------------------------------------------
    // CHART
    rrdvar_delete(st->rrdvariables, rs->var_local);
    rs->var_local = NULL;

    // ------------------------------------------------------------------------
    // FAMILY
    rrdvar_delete(st->rrdfamily->rrdvariables, rs->var_family);
    rs->var_family = NULL;

    rrdvar_delete(st->rrdfamily->rrdvariables, rs->var_family_name);
    rs->var_family_name = NULL;

    // ------------------------------------------------------------------------
    // HOST
    rrdvar_delete(host->rrdvariables_index, rs->var_host);
    rs->var_host = NULL;

    rrdvar_delete(host->rrdvariables_index, rs->var_host_name);
    rs->var_host_name = NULL;

    // ------------------------------------------------------------------------
    // KEYS
    string_freez(rs->key_fullid);
    rs->key_fullid = NULL;

    string_freez(rs->key_fullname);
    rs->key_fullname = NULL;
}

static inline void rrdsetvar_create_variables(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;
    RRDHOST *host = st->rrdhost;

    RRDVAR_OPTIONS options = rs->options;
    options &= ~RRDVAR_OPTIONS_REMOVED_WHEN_PROPAGATING_TO_RRDVAR;

    // ------------------------------------------------------------------------
    // free the old ones (if any)

    rrdsetvar_free_variables(rs);

    // ------------------------------------------------------------------------
    // KEYS

    char buffer[RRDVAR_MAX_LENGTH + 1];
    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_id(st), string2str(rs->variable));
    rs->key_fullid = string_strdupz(buffer);

    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_name(st), string2str(rs->variable));
    rs->key_fullname = string_strdupz(buffer);

    // ------------------------------------------------------------------------
    // CHART
    rs->var_local       = rrdvar_add("local", st->rrdvariables, rs->variable, rs->type, options, rs->value);

    // ------------------------------------------------------------------------
    // FAMILY
    rs->var_family      =
        rrdvar_add("family", st->rrdfamily->rrdvariables, rs->key_fullid, rs->type, options, rs->value);
    rs->var_family_name =
        rrdvar_add("family", st->rrdfamily->rrdvariables, rs->key_fullname, rs->type, options, rs->value);

    // ------------------------------------------------------------------------
    // HOST
    rs->var_host        = rrdvar_add("host", host->rrdvariables_index, rs->key_fullid, rs->type, options, rs->value);
    rs->var_host_name   = rrdvar_add("host", host->rrdvariables_index, rs->key_fullname, rs->type, options, rs->value);
}

static void rrdsetvar_free_value(RRDSETVAR *rs) {
    if(rs->options & RRDVAR_OPTION_ALLOCATED) {
        freez(rs->value);
        rs->value = NULL;
        rs->options &= ~RRDVAR_OPTION_ALLOCATED;
    }
}

static void rrdsetvar_set_value(RRDSETVAR *rs, void *new_value) {
    rrdsetvar_free_value(rs);

    if(new_value)
        rs->value = new_value;
    else {
        NETDATA_DOUBLE *n = mallocz(sizeof(NETDATA_DOUBLE));
        *n = NAN;
        rs->value = n;
        rs->options |= RRDVAR_OPTION_ALLOCATED;
    }
}

struct rrdsetvar_constructor {
    RRDSET *rrdset;
    const char *variable;
    void *value;
    RRDVAR_OPTIONS options:16;
    RRDVAR_TYPE type:8;

    enum {
        RRDSETVAR_REACT_NONE    = 0,
        RRDSETVAR_REACT_NEW     = (1 << 0),
        RRDSETVAR_REACT_UPDATED = (1 << 1),
    } react_action;
};

static void rrdsetvar_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdsetvar, void *constructor_data) {
    RRDSETVAR *rs = rrdsetvar;
    struct rrdsetvar_constructor *ctr = constructor_data;

    ctr->options &= ~RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS;

    rs->variable = string_strdupz(ctr->variable);
    rs->type = ctr->type;
    rs->options = ctr->options | RRDVAR_OPTION_INTERNAL_JUST_CREATED;
    rs->rrdset = ctr->rrdset;
    rrdsetvar_set_value(rs, ctr->value);

    ctr->react_action = RRDSETVAR_REACT_NEW;

    rrdsetvar_create_variables(rs);
}

static void rrdsetvar_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdsetvar, void *new_rrdsetvar __maybe_unused, void *constructor_data) {
    RRDSETVAR *rs = rrdsetvar;
    struct rrdsetvar_constructor *ctr = constructor_data;

    ctr->options &= ~RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS;

    RRDVAR_OPTIONS options = rs->options;
    options &= ~RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS;

    ctr->react_action = RRDSETVAR_REACT_NONE;

    if(((ctr->value == NULL && rs->value != NULL && rs->options & RRDVAR_OPTION_ALLOCATED) || (rs->value == ctr->value))
        && ctr->options == options && rs->type == ctr->type) {
        // don't reset it - everything is the same, or as it should...
        ;
    }
    else {
        internal_error(true, "RRDSETVAR: resetting variable '%s' of chart '%s' of host '%s', options from 0x%x to 0x%x, type from %d to %d",
                       string2str(rs->variable), rrdset_id(rs->rrdset), rrdhost_hostname(rs->rrdset->rrdhost),
                       options, ctr->options, rs->type, ctr->type);

        rrdsetvar_free_value(rs); // we are going to change the options, so free it before setting it
        rs->options = ctr->options | RRDVAR_OPTION_INTERNAL_JUST_UPDATED;
        rs->type = ctr->type;
        rrdsetvar_set_value(rs, ctr->value);
        ctr->react_action = RRDSETVAR_REACT_UPDATED;
    }
}

static void rrdsetvar_react_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdsetvar, void *constructor_data) {
    RRDSETVAR *rs = rrdsetvar;
    struct rrdsetvar_constructor *ctr = constructor_data;

    if(ctr->react_action & (RRDSETVAR_REACT_NEW|RRDSETVAR_REACT_UPDATED))
        rrdsetvar_create_variables(rs);
}

static void rrdsetvar_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdsetvar, void *rrdset __maybe_unused) {
    RRDSETVAR *rs = rrdsetvar;

    rrdsetvar_free_variables(rs);

    string_freez(rs->variable);
    rs->variable = NULL;

    rrdsetvar_free_value(rs);
}

void rrdsetvar_index_init(RRDSET *st) {
    if(!st->rrdsetvar_root_index) {
        st->rrdsetvar_root_index = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);

        dictionary_register_insert_callback(st->rrdsetvar_root_index, rrdsetvar_insert_callback, NULL);
        dictionary_register_conflict_callback(st->rrdsetvar_root_index, rrdsetvar_conflict_callback, NULL);
        dictionary_register_react_callback(st->rrdsetvar_root_index, rrdsetvar_react_callback, NULL);
        dictionary_register_delete_callback(st->rrdsetvar_root_index, rrdsetvar_delete_callback, st);
    }
}

void rrdsetvar_index_destroy(RRDSET *st) {
    dictionary_destroy(st->rrdsetvar_root_index);
}

RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *variable, RRDVAR_TYPE type, void *value, RRDVAR_OPTIONS options) {
    struct rrdsetvar_constructor tmp = {
        .variable = variable,
        .type = type,
        .value = value,
        .options = options,
        .rrdset = st,
    };

    RRDSETVAR *rs = dictionary_set_advanced(st->rrdsetvar_root_index, variable, -1, NULL, sizeof(RRDSETVAR), &tmp);
    rs->options &= ~(RRDVAR_OPTION_INTERNAL_JUST_CREATED | RRDVAR_OPTION_INTERNAL_JUST_UPDATED);

    return rs;
}

void rrdsetvar_rename_all(RRDSET *st) {
    debug(D_VARIABLES, "RRDSETVAR rename for chart id '%s' name '%s'", rrdset_id(st), rrdset_name(st));

    RRDSETVAR *rs;
    dfe_start_read(st->rrdsetvar_root_index, rs)
        rrdsetvar_create_variables(rs);
    dfe_done(rs);

    rrdsetcalc_link_matching(st);
}

void rrdsetvar_free_all(RRDSET *st) {
    RRDSETVAR *rs;
    dfe_start_write(st->rrdsetvar_root_index, rs)
        dictionary_del_having_write_lock(st->rrdsetvar_root_index, string2str(rs->variable));
    dfe_done(rs);
}

// --------------------------------------------------------------------------------------------------------------------
// custom chart variables

RRDSETVAR *rrdsetvar_custom_chart_variable_create(RRDSET *st, const char *name) {
    STRING *n = rrdvar_name_to_string(name);
    RRDSETVAR *rs = rrdsetvar_create(st, string2str(n), RRDVAR_TYPE_CALCULATED, NULL, RRDVAR_OPTION_CUSTOM_CHART_VAR);
    string_freez(n);
    return rs;
}

void rrdsetvar_custom_chart_variable_set(RRDSETVAR *rs, NETDATA_DOUBLE value) {
    if(rs->type != RRDVAR_TYPE_CALCULATED || !(rs->options & RRDVAR_OPTION_CUSTOM_CHART_VAR) || !(rs->options & RRDVAR_OPTION_ALLOCATED)) {
        error("RRDSETVAR: requested to set variable '%s' of chart '%s' on host '%s' to value " NETDATA_DOUBLE_FORMAT
            " but the variable is not a custom chart one (it has options 0x%x, value pointer %p). Ignoring request.", string2str(rs->variable), rrdset_id(rs->rrdset), rrdhost_hostname(rs->rrdset->rrdhost), value, rs->options, rs->value);
    }
    else {
        NETDATA_DOUBLE *v = rs->value;
        if(*v != value) {
            *v = value;

            // mark the chart to be sent upstream
            rrdset_flag_clear(rs->rrdset, RRDSET_FLAG_UPSTREAM_EXPOSED);
        }
    }
}
