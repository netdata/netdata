// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

typedef struct rrddimvar {
    struct rrddim *rrddim;

    STRING *prefix;
    STRING *suffix;
    void *value;

    const RRDVAR_ACQUIRED *rrdvar_local_dim_id;
    const RRDVAR_ACQUIRED *rrdvar_local_dim_name;

    const RRDVAR_ACQUIRED *rrdvar_family_id;
    const RRDVAR_ACQUIRED *rrdvar_family_name;
    const RRDVAR_ACQUIRED *rrdvar_family_context_dim_id;
    const RRDVAR_ACQUIRED *rrdvar_family_context_dim_name;

    const RRDVAR_ACQUIRED *rrdvar_host_chart_id_dim_id;
    const RRDVAR_ACQUIRED *rrdvar_host_chart_id_dim_name;
    const RRDVAR_ACQUIRED *rrdvar_host_chart_name_dim_id;
    const RRDVAR_ACQUIRED *rrdvar_host_chart_name_dim_name;

    RRDVAR_FLAGS flags:24;
    RRDVAR_TYPE type:8;
} RRDDIMVAR;

// ----------------------------------------------------------------------------
// RRDDIMVAR management
// DIMENSION VARIABLES

#define RRDDIMVAR_ID_MAX 1024

static inline void rrddimvar_free_variables_unsafe(RRDDIMVAR *rs) {
    RRDDIM *rd = rs->rrddim;
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    // CHART VARIABLES FOR THIS DIMENSION

    if(st->rrdvars) {
        rrdvar_release_and_del(st->rrdvars, rs->rrdvar_local_dim_id);
        rs->rrdvar_local_dim_id = NULL;

        rrdvar_release_and_del(st->rrdvars, rs->rrdvar_local_dim_name);
        rs->rrdvar_local_dim_name = NULL;
    }

    // FAMILY VARIABLES FOR THIS DIMENSION

    if(st->rrdfamily) {
        rrdvar_release_and_del(rrdfamily_rrdvars_dict(st->rrdfamily), rs->rrdvar_family_id);
        rs->rrdvar_family_id = NULL;

        rrdvar_release_and_del(rrdfamily_rrdvars_dict(st->rrdfamily), rs->rrdvar_family_name);
        rs->rrdvar_family_name = NULL;

        rrdvar_release_and_del(rrdfamily_rrdvars_dict(st->rrdfamily), rs->rrdvar_family_context_dim_id);
        rs->rrdvar_family_context_dim_id = NULL;

        rrdvar_release_and_del(rrdfamily_rrdvars_dict(st->rrdfamily), rs->rrdvar_family_context_dim_name);
        rs->rrdvar_family_context_dim_name = NULL;
    }

    // HOST VARIABLES FOR THIS DIMENSION

    if(host->rrdvars && host->health.health_enabled) {
        rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_id_dim_id);
        rs->rrdvar_host_chart_id_dim_id = NULL;

        rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_id_dim_name);
        rs->rrdvar_host_chart_id_dim_name = NULL;

        rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_name_dim_id);
        rs->rrdvar_host_chart_name_dim_id = NULL;

        rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_name_dim_name);
        rs->rrdvar_host_chart_name_dim_name = NULL;
    }
}

static inline void rrddimvar_update_variables_unsafe(RRDDIMVAR *rs) {
    rrddimvar_free_variables_unsafe(rs);

    RRDDIM *rd = rs->rrddim;
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    char buffer[RRDDIMVAR_ID_MAX + 1];

    // KEYS

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", string2str(rs->prefix), rrddim_id(rd), string2str(rs->suffix));
    STRING *key_dim_id = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", string2str(rs->prefix), rrddim_name(rd), string2str(rs->suffix));
    STRING *key_dim_name = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_id(st), string2str(key_dim_id));
    STRING *key_chart_id_dim_id = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_id(st), string2str(key_dim_name));
    STRING *key_chart_id_dim_name = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_context(st), string2str(key_dim_id));
    STRING *key_context_dim_id = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_context(st), string2str(key_dim_name));
    STRING *key_context_dim_name = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_name(st), string2str(key_dim_id));
    STRING *key_chart_name_dim_id = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_name(st), string2str(key_dim_name));
    STRING *key_chart_name_dim_name = string_strdupz(buffer);

    // CHART VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $id
    // - $name

    if(st->rrdvars) {
        rs->rrdvar_local_dim_id = rrdvar_add_and_acquire("local", st->rrdvars, key_dim_id, rs->type, RRDVAR_FLAG_NONE, rs->value);
        rs->rrdvar_local_dim_name = rrdvar_add_and_acquire("local", st->rrdvars, key_dim_name, rs->type, RRDVAR_FLAG_NONE, rs->value);
    }

    // FAMILY VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $id                 (only the first, when multiple overlap)
    // - $name               (only the first, when multiple overlap)
    // - $chart-context.id
    // - $chart-context.name

    if(st->rrdfamily) {
        rs->rrdvar_family_id = rrdvar_add_and_acquire("family", rrdfamily_rrdvars_dict(st->rrdfamily), key_dim_id, rs->type, RRDVAR_FLAG_NONE, rs->value);
        rs->rrdvar_family_name = rrdvar_add_and_acquire("family", rrdfamily_rrdvars_dict(st->rrdfamily), key_dim_name, rs->type, RRDVAR_FLAG_NONE, rs->value);
        rs->rrdvar_family_context_dim_id = rrdvar_add_and_acquire("family", rrdfamily_rrdvars_dict(st->rrdfamily), key_context_dim_id, rs->type, RRDVAR_FLAG_NONE, rs->value);
        rs->rrdvar_family_context_dim_name = rrdvar_add_and_acquire("family", rrdfamily_rrdvars_dict(st->rrdfamily), key_context_dim_name, rs->type, RRDVAR_FLAG_NONE, rs->value);
    }

    // HOST VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $chart-id.id
    // - $chart-id.name
    // - $chart-name.id
    // - $chart-name.name

    if(host->rrdvars && host->health.health_enabled) {
        rs->rrdvar_host_chart_id_dim_id = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_id_dim_id, rs->type, RRDVAR_FLAG_NONE, rs->value);
        rs->rrdvar_host_chart_id_dim_name = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_id_dim_name, rs->type, RRDVAR_FLAG_NONE, rs->value);
        rs->rrdvar_host_chart_name_dim_id = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_name_dim_id, rs->type, RRDVAR_FLAG_NONE, rs->value);
        rs->rrdvar_host_chart_name_dim_name = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_name_dim_name, rs->type, RRDVAR_FLAG_NONE, rs->value);
    }

    // free the keys

    string_freez(key_dim_id);
    string_freez(key_dim_name);
    string_freez(key_chart_id_dim_id);
    string_freez(key_chart_id_dim_name);
    string_freez(key_context_dim_id);
    string_freez(key_context_dim_name);
    string_freez(key_chart_name_dim_id);
    string_freez(key_chart_name_dim_name);
}

struct rrddimvar_constructor {
    RRDDIM *rrddim;
    const char *prefix;
    const char *suffix;
    void *value;
    RRDVAR_FLAGS flags :16;
    RRDVAR_TYPE type:8;
};

static void rrddimvar_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrddimvar, void *constructor_data) {
    RRDDIMVAR *rs = rrddimvar;
    struct rrddimvar_constructor *ctr = constructor_data;

    if(!ctr->prefix) ctr->prefix = "";
    if(!ctr->suffix) ctr->suffix = "";

    rs->prefix = string_strdupz(ctr->prefix);
    rs->suffix = string_strdupz(ctr->suffix);

    rs->type = ctr->type;
    rs->value = ctr->value;
    rs->flags = ctr->flags;
    rs->rrddim = ctr->rrddim;

    rrddimvar_update_variables_unsafe(rs);
}

static bool rrddimvar_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrddimvar, void *new_rrddimvar __maybe_unused, void *constructor_data __maybe_unused) {
    RRDDIMVAR *rs = rrddimvar;
    rrddimvar_update_variables_unsafe(rs);

    return true;
}

static void rrddimvar_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *rrddimvar, void *rrdset __maybe_unused) {
    RRDDIMVAR *rs = rrddimvar;
    rrddimvar_free_variables_unsafe(rs);
    string_freez(rs->prefix);
    string_freez(rs->suffix);
}

void rrddimvar_index_init(RRDSET *st) {
    if(!st->rrddimvar_root_index) {
        st->rrddimvar_root_index = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                              &dictionary_stats_category_rrdhealth, sizeof(RRDDIMVAR));

        dictionary_register_insert_callback(st->rrddimvar_root_index, rrddimvar_insert_callback, NULL);
        dictionary_register_conflict_callback(st->rrddimvar_root_index, rrddimvar_conflict_callback, NULL);
        dictionary_register_delete_callback(st->rrddimvar_root_index, rrddimvar_delete_callback, st);
    }
}

void rrddimvar_index_destroy(RRDSET *st) {
    dictionary_destroy(st->rrddimvar_root_index);
    st->rrddimvar_root_index = NULL;
}

void rrddimvar_add_and_leave_released(RRDDIM *rd, RRDVAR_TYPE type, const char *prefix, const char *suffix, void *value, RRDVAR_FLAGS flags) {
    if(!prefix) prefix = "";
    if(!suffix) suffix = "";

    char key[RRDDIMVAR_ID_MAX + 1];
    size_t key_len = snprintfz(key, RRDDIMVAR_ID_MAX, "%s_%s_%s", prefix, rrddim_id(rd), suffix);

    struct rrddimvar_constructor tmp = {
        .suffix = suffix,
        .prefix = prefix,
        .type = type,
        .flags = flags,
        .value = value,
        .rrddim = rd
    };
    dictionary_set_advanced(rd->rrdset->rrddimvar_root_index, key, (ssize_t)(key_len + 1), NULL, sizeof(RRDDIMVAR), &tmp);
}

void rrddimvar_rename_all(RRDDIM *rd) {
    RRDSET *st = rd->rrdset;

    debug(D_VARIABLES, "RRDDIMVAR rename for chart id '%s' name '%s', dimension id '%s', name '%s'", rrdset_id(st), rrdset_name(st), rrddim_id(rd), rrddim_name(rd));

    RRDDIMVAR *rs;
    dfe_start_write(st->rrddimvar_root_index, rs) {
        if(unlikely(rs->rrddim == rd))
            rrddimvar_update_variables_unsafe(rs);
    }
    dfe_done(rs);
}

void rrddimvar_delete_all(RRDDIM *rd) {
    RRDSET *st = rd->rrdset;

    debug(D_VARIABLES, "RRDDIMVAR delete for chart id '%s' name '%s', dimension id '%s', name '%s'", rrdset_id(st), rrdset_name(st), rrddim_id(rd), rrddim_name(rd));

    RRDDIMVAR *rs;
    dfe_start_write(st->rrddimvar_root_index, rs) {
        if(unlikely(rs->rrddim == rd))
            dictionary_del(st->rrddimvar_root_index, rs_dfe.name);
    }
    dfe_done(rs);
}
