// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

typedef struct rrdsetvar {
    STRING *name;               // variable name
    void *value;                // we need this to maintain the allocation for custom chart variables

    const RRDVAR_ACQUIRED *rrdvar_local;
    const RRDVAR_ACQUIRED *rrdvar_family_chart_id;
    const RRDVAR_ACQUIRED *rrdvar_family_chart_name;
    const RRDVAR_ACQUIRED *rrdvar_host_chart_id;
    const RRDVAR_ACQUIRED *rrdvar_host_chart_name;

    RRDVAR_FLAGS flags:24;
    RRDVAR_TYPE type:8;
} RRDSETVAR;

// should only be called while the rrdsetvar dict is write locked
// otherwise, 2+ threads may be setting the same variables at the same time
static inline void rrdsetvar_free_rrdvars_unsafe(RRDSET *st, RRDSETVAR *rs) {
    RRDHOST *host = st->rrdhost;

    // ------------------------------------------------------------------------
    // CHART

    if(st->rrdvars) {
        rrdvar_release_and_del(st->rrdvars, rs->rrdvar_local);
        rs->rrdvar_local = NULL;
    }

    // ------------------------------------------------------------------------
    // FAMILY

    if(st->rrdfamily) {
        rrdvar_release_and_del(rrdfamily_rrdvars_dict(st->rrdfamily), rs->rrdvar_family_chart_id);
        rs->rrdvar_family_chart_id = NULL;

        rrdvar_release_and_del(rrdfamily_rrdvars_dict(st->rrdfamily), rs->rrdvar_family_chart_name);
        rs->rrdvar_family_chart_name = NULL;
    }

    // ------------------------------------------------------------------------
    // HOST

    if(host->rrdvars && host->health.health_enabled) {
        rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_id);
        rs->rrdvar_host_chart_id = NULL;

        rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_name);
        rs->rrdvar_host_chart_name = NULL;
    }
}

// should only be called while the rrdsetvar dict is write locked
// otherwise, 2+ threads may be setting the same variables at the same time
static inline void rrdsetvar_update_rrdvars_unsafe(RRDSET *st, RRDSETVAR *rs) {
    RRDHOST *host = st->rrdhost;

    RRDVAR_FLAGS options = rs->flags;
    options &= ~RRDVAR_OPTIONS_REMOVED_WHEN_PROPAGATING_TO_RRDVAR;

    // ------------------------------------------------------------------------
    // free the old ones (if any)

    rrdsetvar_free_rrdvars_unsafe(st, rs);

    // ------------------------------------------------------------------------
    // KEYS

    char buffer[RRDVAR_MAX_LENGTH + 1];
    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_id(st), string2str(rs->name));
    STRING *key_chart_id = string_strdupz(buffer);

    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_name(st), string2str(rs->name));
    STRING *key_chart_name = string_strdupz(buffer);

    // ------------------------------------------------------------------------
    // CHART

    if(st->rrdvars) {
        rs->rrdvar_local = rrdvar_add_and_acquire("local", st->rrdvars, rs->name, rs->type, options, rs->value);
    }

    // ------------------------------------------------------------------------
    // FAMILY

    if(st->rrdfamily) {
        rs->rrdvar_family_chart_id = rrdvar_add_and_acquire("family", rrdfamily_rrdvars_dict(st->rrdfamily), key_chart_id, rs->type, options, rs->value);
        rs->rrdvar_family_chart_name = rrdvar_add_and_acquire("family", rrdfamily_rrdvars_dict(st->rrdfamily), key_chart_name, rs->type, options, rs->value);
    }

    // ------------------------------------------------------------------------
    // HOST

    if(host->rrdvars && host->health.health_enabled) {
        rs->rrdvar_host_chart_id = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_id, rs->type, options, rs->value);
        rs->rrdvar_host_chart_name = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_name, rs->type, options, rs->value);
    }

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
};

static void rrdsetvar_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdsetvar, void *constructor_data) {
    RRDSETVAR *rs = rrdsetvar;
    struct rrdsetvar_constructor *ctr = constructor_data;

    ctr->flags &= ~RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS;

    rs->name = string_strdupz(ctr->name);
    rs->type = ctr->type;
    rs->flags = ctr->flags;
    rrdsetvar_set_value_unsafe(rs, ctr->value);

    // create the rrdvariables while we are having a write lock to the dictionary
    rrdsetvar_update_rrdvars_unsafe(ctr->rrdset, rs);
}

static bool rrdsetvar_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdsetvar, void *new_rrdsetvar __maybe_unused, void *constructor_data) {
    RRDSETVAR *rs = rrdsetvar;
    struct rrdsetvar_constructor *ctr = constructor_data;

    ctr->flags &= ~RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS;

    RRDVAR_FLAGS options = rs->flags;
    options &= ~RRDVAR_OPTIONS_REMOVED_ON_NEW_OBJECTS;

    if(((ctr->value == NULL && rs->value != NULL && rs->flags & RRDVAR_FLAG_ALLOCATED) || (rs->value == ctr->value))
        && ctr->flags == options && rs->type == ctr->type) {
        // don't reset it - everything is the same, or as it should...
        return false;
    }

    internal_error(true, "RRDSETVAR: resetting variable '%s' of chart '%s' of host '%s', options from 0x%x to 0x%x, type from %d to %d",
                   string2str(rs->name), rrdset_id(ctr->rrdset), rrdhost_hostname(ctr->rrdset->rrdhost),
                   options, ctr->flags, rs->type, ctr->type);

    rrdsetvar_free_value_unsafe(rs); // we are going to change the options, so free it before setting it
    rs->flags = ctr->flags;
    rs->type = ctr->type;
    rrdsetvar_set_value_unsafe(rs, ctr->value);

    // recreate the rrdvariables while we are having a write lock to the dictionary
    rrdsetvar_update_rrdvars_unsafe(ctr->rrdset, rs);
    return true;
}

static void rrdsetvar_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrdsetvar, void *rrdset __maybe_unused) {
    RRDSET *st = rrdset;
    RRDSETVAR *rs = rrdsetvar;

    rrdsetvar_free_rrdvars_unsafe(st, rs);
    rrdsetvar_free_value_unsafe(rs);
    string_freez(rs->name);
    rs->name = NULL;
}

void rrdsetvar_index_init(RRDSET *st) {
    if(!st->rrdsetvar_root_index) {
        st->rrdsetvar_root_index = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                              &dictionary_stats_category_rrdhealth, sizeof(RRDSETVAR));

        dictionary_register_insert_callback(st->rrdsetvar_root_index, rrdsetvar_insert_callback, NULL);
        dictionary_register_conflict_callback(st->rrdsetvar_root_index, rrdsetvar_conflict_callback, NULL);
        dictionary_register_delete_callback(st->rrdsetvar_root_index, rrdsetvar_delete_callback, st);
    }
}

void rrdsetvar_index_destroy(RRDSET *st) {
    dictionary_destroy(st->rrdsetvar_root_index);
    st->rrdsetvar_root_index = NULL;
}

const RRDSETVAR_ACQUIRED *rrdsetvar_add_and_acquire(RRDSET *st, const char *name, RRDVAR_TYPE type, void *value, RRDVAR_FLAGS flags) {
    struct rrdsetvar_constructor tmp = {
        .name = name,
        .type = type,
        .value = value,
        .flags = flags,
        .rrdset = st,
    };

    const RRDSETVAR_ACQUIRED *rsa = (const RRDSETVAR_ACQUIRED *)dictionary_set_and_acquire_item_advanced(st->rrdsetvar_root_index, name, -1, NULL, sizeof(RRDSETVAR), &tmp);
    return rsa;
}

void rrdsetvar_add_and_leave_released(RRDSET *st, const char *name, RRDVAR_TYPE type, void *value, RRDVAR_FLAGS flags) {
    const RRDSETVAR_ACQUIRED *rsa = rrdsetvar_add_and_acquire(st, name, type, value, flags);
    dictionary_acquired_item_release(st->rrdsetvar_root_index, (const DICTIONARY_ITEM *)rsa);
}

void rrdsetvar_rename_all(RRDSET *st) {
    debug(D_VARIABLES, "RRDSETVAR rename for chart id '%s' name '%s'", rrdset_id(st), rrdset_name(st));

    RRDSETVAR *rs;
    dfe_start_write(st->rrdsetvar_root_index, rs) {
        // should only be called while the rrdsetvar dict is write locked
        rrdsetvar_update_rrdvars_unsafe(st, rs);
    }
    dfe_done(rs);

    rrdcalc_link_matching_alerts_to_rrdset(st);
}

void rrdsetvar_release_and_delete_all(RRDSET *st) {
    RRDSETVAR *rs;
    dfe_start_write(st->rrdsetvar_root_index, rs) {
        dictionary_del_advanced(st->rrdsetvar_root_index, string2str(rs->name), (ssize_t)string_strlen(rs->name) + 1);
    }
    dfe_done(rs);
}

void rrdsetvar_release(DICTIONARY *dict, const RRDSETVAR_ACQUIRED *rsa) {
    dictionary_acquired_item_release(dict, (const DICTIONARY_ITEM *)rsa);
}

// --------------------------------------------------------------------------------------------------------------------
// custom chart variables

const RRDSETVAR_ACQUIRED *rrdsetvar_custom_chart_variable_add_and_acquire(RRDSET *st, const char *name) {
    STRING *name_string = rrdvar_name_to_string(name);
    const RRDSETVAR_ACQUIRED *rs = rrdsetvar_add_and_acquire(st, string2str(name_string), RRDVAR_TYPE_CALCULATED, NULL, RRDVAR_FLAG_CUSTOM_CHART_VAR);
    string_freez(name_string);
    return rs;
}

void rrdsetvar_custom_chart_variable_set(RRDSET *st, const RRDSETVAR_ACQUIRED *rsa, NETDATA_DOUBLE value) {
    if(!rsa) return;

    RRDSETVAR *rs = dictionary_acquired_item_value((const DICTIONARY_ITEM *)rsa);

    if(rs->type != RRDVAR_TYPE_CALCULATED || !(rs->flags & RRDVAR_FLAG_CUSTOM_CHART_VAR) || !(rs->flags & RRDVAR_FLAG_ALLOCATED)) {
        error("RRDSETVAR: requested to set variable '%s' of chart '%s' on host '%s' to value " NETDATA_DOUBLE_FORMAT
            " but the variable is not a custom chart one (it has options 0x%x, value pointer %p). Ignoring request.", string2str(rs->name), rrdset_id(st), rrdhost_hostname(st->rrdhost), value, (uint32_t)rs->flags, rs->value);
    }
    else {
        NETDATA_DOUBLE *v = rs->value;
        if(*v != value) {
            *v = value;
            rrdset_flag_set(st, RRDSET_FLAG_UPSTREAM_SEND_VARIABLES);
        }
    }
}

void rrdsetvar_print_to_streaming_custom_chart_variables(RRDSET *st, BUFFER *wb) {
    rrdset_flag_clear(st, RRDSET_FLAG_UPSTREAM_SEND_VARIABLES);

    // send the chart local custom variables
    RRDSETVAR *rs;
    dfe_start_read(st->rrdsetvar_root_index, rs) {
        if(unlikely(rs->type == RRDVAR_TYPE_CALCULATED && rs->flags & RRDVAR_FLAG_CUSTOM_CHART_VAR)) {
            NETDATA_DOUBLE *value = (NETDATA_DOUBLE *) rs->value;

            buffer_sprintf(wb
                , "VARIABLE CHART %s = " NETDATA_DOUBLE_FORMAT "\n"
                , string2str(rs->name)
                , *value
            );
        }
    }
    dfe_done(rs);
}
