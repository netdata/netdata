// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_HEALTH_INTERNALS
#include "rrd.h"

// should only be called while the rrdsetvar dict is write locked
// otherwise, 2+ threads may be setting the same variables at the same time
static inline void rrdsetvar_free_rrdvars_unsafe(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;
    RRDHOST *host = st->rrdhost;

    // ------------------------------------------------------------------------
    // CHART
    rrdvar_release_and_del(st->rrdvars, rs->rrdvar_local);
    rs->rrdvar_local = NULL;

    // ------------------------------------------------------------------------
    // FAMILY
    rrdvar_release_and_del(st->rrdfamily->rrdvars, rs->rrdvar_family_chart_id);
    rs->rrdvar_family_chart_id = NULL;

    rrdvar_release_and_del(st->rrdfamily->rrdvars, rs->rrdvar_family_chart_name);
    rs->rrdvar_family_chart_name = NULL;

    // ------------------------------------------------------------------------
    // HOST
    rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_id);
    rs->rrdvar_host_chart_id = NULL;

    rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_name);
    rs->rrdvar_host_chart_name = NULL;
}

// should only be called while the rrdsetvar dict is write locked
// otherwise, 2+ threads may be setting the same variables at the same time
static inline void rrdsetvar_update_rrdvars_unsafe(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;
    RRDHOST *host = st->rrdhost;

    RRDVAR_FLAGS options = rs->flags;
    options &= ~RRDVAR_OPTIONS_REMOVED_WHEN_PROPAGATING_TO_RRDVAR;

    // ------------------------------------------------------------------------
    // free the old ones (if any)

    rrdsetvar_free_rrdvars_unsafe(rs);

    // ------------------------------------------------------------------------
    // KEYS

    char buffer[RRDVAR_MAX_LENGTH + 1];
    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_id(st), string2str(rs->name));
    STRING *key_chart_id = string_strdupz(buffer);

    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_name(st), string2str(rs->name));
    STRING *key_chart_name = string_strdupz(buffer);

    // ------------------------------------------------------------------------
    // CHART
    rs->rrdvar_local = rrdvar_add_and_acquire("local", st->rrdvars, rs->name, rs->type, options, rs->value);

    // ------------------------------------------------------------------------
    // FAMILY
    rs->rrdvar_family_chart_id = rrdvar_add_and_acquire("family", st->rrdfamily->rrdvars, key_chart_id, rs->type, options, rs->value);
    rs->rrdvar_family_chart_name = rrdvar_add_and_acquire("family", st->rrdfamily->rrdvars, key_chart_name, rs->type, options, rs->value);

    // ------------------------------------------------------------------------
    // HOST
    rs->rrdvar_host_chart_id = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_id, rs->type, options, rs->value);
    rs->rrdvar_host_chart_name = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_name, rs->type, options, rs->value);

    // free the keys
    string_freez(key_chart_id);
    string_freez(key_chart_name);
}

static void rrdsetvar_free_value_unsafe(RRDSETVAR *rs) {
    if(rs->flags & RRDVAR_FLAG_ALLOCATED) {
        void *old = rs->value;
        rs->value = NULL;
        rs->flags &= ~RRDVAR_FLAG_ALLOCATED;
        freez(old);
    }
}

static void rrdsetvar_set_value_unsafe(RRDSETVAR *rs, void *new_value) {
    rrdsetvar_free_value_unsafe(rs);

    if(new_value)
        rs->value = new_value;
    else {
        NETDATA_DOUBLE *n = mallocz(sizeof(NETDATA_DOUBLE));
        *n = NAN;
        rs->value = n;
        rs->flags |= RRDVAR_FLAG_ALLOCATED;
    }
}

struct rrdsetvar_constructor {
    RRDSET *rrdset;
    const char *name;
    void *value;
    RRDVAR_FLAGS flags :16;
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

    ctr->flags &= ~RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS;

    rs->name = string_strdupz(ctr->name);
    rs->type = ctr->type;
    rs->flags = ctr->flags;
    rs->rrdset = ctr->rrdset;
    rrdsetvar_set_value_unsafe(rs, ctr->value);

    ctr->react_action = RRDSETVAR_REACT_NEW;

    // create the rrdvariables while we are having a write lock to the dictionary
    rrdsetvar_update_rrdvars_unsafe(rs);
}

static void rrdsetvar_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdsetvar, void *new_rrdsetvar __maybe_unused, void *constructor_data) {
    RRDSETVAR *rs = rrdsetvar;
    struct rrdsetvar_constructor *ctr = constructor_data;

    ctr->flags &= ~RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS;

    RRDVAR_FLAGS options = rs->flags;
    options &= ~RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS;

    ctr->react_action = RRDSETVAR_REACT_NONE;

    if(((ctr->value == NULL && rs->value != NULL && rs->flags & RRDVAR_FLAG_ALLOCATED) || (rs->value == ctr->value))
        && ctr->flags == options && rs->type == ctr->type) {
        // don't reset it - everything is the same, or as it should...
        ;
    }
    else {
        internal_error(true, "RRDSETVAR: resetting variable '%s' of chart '%s' of host '%s', options from 0x%x to 0x%x, type from %d to %d",
                       string2str(rs->name), rrdset_id(rs->rrdset), rrdhost_hostname(rs->rrdset->rrdhost),
                       options, ctr->flags, rs->type, ctr->type);

        rrdsetvar_free_value_unsafe(rs); // we are going to change the options, so free it before setting it
        rs->flags = ctr->flags;
        rs->type = ctr->type;
        rrdsetvar_set_value_unsafe(rs, ctr->value);

        // recreate the rrdvariables while we are having a write lock to the dictionary
        rrdsetvar_update_rrdvars_unsafe(rs);

        ctr->react_action = RRDSETVAR_REACT_UPDATED;
    }
}

static void rrdsetvar_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdsetvar, void *rrdset __maybe_unused) {
    RRDSETVAR *rs = rrdsetvar;

    rrdsetvar_free_rrdvars_unsafe(rs);
    rrdsetvar_free_value_unsafe(rs);
    string_freez(rs->name);
    rs->name = NULL;
}

void rrdsetvar_index_init(RRDSET *st) {
    if(!st->rrdsetvar_root_index) {
        st->rrdsetvar_root_index = dictionary_create(DICTIONARY_FLAG_DONT_OVERWRITE_VALUE);

        dictionary_register_insert_callback(st->rrdsetvar_root_index, rrdsetvar_insert_callback, NULL);
        dictionary_register_conflict_callback(st->rrdsetvar_root_index, rrdsetvar_conflict_callback, NULL);
        dictionary_register_delete_callback(st->rrdsetvar_root_index, rrdsetvar_delete_callback, st);
    }
}

void rrdsetvar_index_destroy(RRDSET *st) {
    dictionary_destroy(st->rrdsetvar_root_index);
}

RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *name, RRDVAR_TYPE type, void *value, RRDVAR_FLAGS options) {
    struct rrdsetvar_constructor tmp = {
        .name = name,
        .type = type,
        .value = value,
        .flags = options,
        .rrdset = st,
    };

    RRDSETVAR *rs = dictionary_set_advanced(st->rrdsetvar_root_index, name, -1, NULL, sizeof(RRDSETVAR), &tmp);
    return rs;
}

void rrdsetvar_rename_all(RRDSET *st) {
    debug(D_VARIABLES, "RRDSETVAR rename for chart id '%s' name '%s'", rrdset_id(st), rrdset_name(st));

    RRDSETVAR *rs;
    dfe_start_write(st->rrdsetvar_root_index, rs) {
        // should only be called while the rrdsetvar dict is write locked
        rrdsetvar_update_rrdvars_unsafe(rs);
    }
    dfe_done(rs);

    rrdsetcalc_link_matching(st);
}

void rrdsetvar_free_all(RRDSET *st) {
    RRDSETVAR *rs;
    dfe_start_write(st->rrdsetvar_root_index, rs)
        dictionary_del_having_write_lock(st->rrdsetvar_root_index, string2str(rs->name));
    dfe_done(rs);
}

// --------------------------------------------------------------------------------------------------------------------
// custom chart variables

RRDSETVAR *rrdsetvar_custom_chart_variable_create(RRDSET *st, const char *name) {
    STRING *name_string = rrdvar_name_to_string(name);
    RRDSETVAR *rs = rrdsetvar_create(st, string2str(name_string), RRDVAR_TYPE_CALCULATED, NULL, RRDVAR_FLAG_CUSTOM_CHART_VAR);
    string_freez(name_string);
    return rs;
}

void rrdsetvar_custom_chart_variable_set(RRDSETVAR *rs, NETDATA_DOUBLE value) {
    if(rs->type != RRDVAR_TYPE_CALCULATED || !(rs->flags & RRDVAR_FLAG_CUSTOM_CHART_VAR) || !(rs->flags & RRDVAR_FLAG_ALLOCATED)) {
        error("RRDSETVAR: requested to set variable '%s' of chart '%s' on host '%s' to value " NETDATA_DOUBLE_FORMAT
            " but the variable is not a custom chart one (it has options 0x%x, value pointer %p). Ignoring request.", string2str(rs->name), rrdset_id(rs->rrdset), rrdhost_hostname(rs->rrdset->rrdhost), value, rs->flags, rs->value);
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
