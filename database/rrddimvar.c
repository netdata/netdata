// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrd.h"

// ----------------------------------------------------------------------------
// RRDDIMVAR management
// DIMENSION VARIABLES

#define RRDDIMVAR_ID_MAX 1024

static inline void rrddimvar_free_variables(RRDDIMVAR *rs) {
    RRDDIM *rd = rs->rrddim;
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    // CHART VARIABLES FOR THIS DIMENSION

    rrdvar_release_and_del(st->rrdvars, rs->rrdvar_local_dim_id);
    rs->rrdvar_local_dim_id = NULL;

    rrdvar_release_and_del(st->rrdvars, rs->rrdvar_local_dim_name);
    rs->rrdvar_local_dim_name = NULL;

    // FAMILY VARIABLES FOR THIS DIMENSION

    rrdvar_release_and_del(st->rrdfamily->rrdvars, rs->rrdvar_family_id);
    rs->rrdvar_family_id = NULL;

    rrdvar_release_and_del(st->rrdfamily->rrdvars, rs->rrdvar_family_name);
    rs->rrdvar_family_name = NULL;

    rrdvar_release_and_del(st->rrdfamily->rrdvars, rs->rrdvar_family_context_dim_id);
    rs->rrdvar_family_context_dim_id = NULL;

    rrdvar_release_and_del(st->rrdfamily->rrdvars, rs->rrdvar_family_context_dim_name);
    rs->rrdvar_family_context_dim_name = NULL;

    // HOST VARIABLES FOR THIS DIMENSION

    rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_id_dim_id);
    rs->rrdvar_host_chart_id_dim_id = NULL;

    rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_id_dim_name);
    rs->rrdvar_host_chart_id_dim_name = NULL;

    rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_name_dim_id);
    rs->rrdvar_host_chart_name_dim_id = NULL;

    rrdvar_release_and_del(host->rrdvars, rs->rrdvar_host_chart_name_dim_name);
    rs->rrdvar_host_chart_name_dim_name = NULL;
}

static inline void rrddimvar_create_variables(RRDDIMVAR *rs) {
    rrddimvar_free_variables(rs);

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

    rs->rrdvar_local_dim_id = rrdvar_add_and_acquire("local", st->rrdvars, key_dim_id, rs->type, RRDVAR_FLAG_NONE, rs->value);
    rs->rrdvar_local_dim_name = rrdvar_add_and_acquire("local", st->rrdvars, key_dim_name, rs->type, RRDVAR_FLAG_NONE, rs->value);

    // FAMILY VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $id                 (only the first, when multiple overlap)
    // - $name               (only the first, when multiple overlap)
    // - $chart-context.id
    // - $chart-context.name

    rs->rrdvar_family_id = rrdvar_add_and_acquire("family", st->rrdfamily->rrdvars, key_dim_id, rs->type, RRDVAR_FLAG_NONE, rs->value);
    rs->rrdvar_family_name = rrdvar_add_and_acquire("family", st->rrdfamily->rrdvars, key_dim_name, rs->type, RRDVAR_FLAG_NONE, rs->value);
    rs->rrdvar_family_context_dim_id = rrdvar_add_and_acquire("family", st->rrdfamily->rrdvars, key_context_dim_id, rs->type, RRDVAR_FLAG_NONE, rs->value);
    rs->rrdvar_family_context_dim_name = rrdvar_add_and_acquire("family", st->rrdfamily->rrdvars, key_context_dim_name, rs->type, RRDVAR_FLAG_NONE, rs->value);

    // HOST VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $chart-id.id
    // - $chart-id.name
    // - $chart-name.id
    // - $chart-name.name

    rs->rrdvar_host_chart_id_dim_id = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_id_dim_id, rs->type, RRDVAR_FLAG_NONE, rs->value);
    rs->rrdvar_host_chart_id_dim_name = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_id_dim_name, rs->type, RRDVAR_FLAG_NONE, rs->value);
    rs->rrdvar_host_chart_name_dim_id = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_name_dim_id, rs->type, RRDVAR_FLAG_NONE, rs->value);
    rs->rrdvar_host_chart_name_dim_name = rrdvar_add_and_acquire("host", host->rrdvars, key_chart_name_dim_name, rs->type, RRDVAR_FLAG_NONE, rs->value);

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

RRDDIMVAR *rrddimvar_create(RRDDIM *rd, RRDVAR_TYPE type, const char *prefix, const char *suffix, void *value, RRDVAR_FLAGS options) {
    RRDSET *st = rd->rrdset;
    (void)st;

    debug(D_VARIABLES, "RRDDIMSET create for chart id '%s' name '%s', dimension id '%s', name '%s%s%s'", rrdset_id(st), rrdset_name(st), rrddim_id(rd), (prefix)?prefix:"", rrddim_name(rd), (suffix)?suffix:"");

    if(!prefix) prefix = "";
    if(!suffix) suffix = "";

    RRDDIMVAR *rs = (RRDDIMVAR *)callocz(1, sizeof(RRDDIMVAR));

    rs->prefix = string_strdupz(prefix);
    rs->suffix = string_strdupz(suffix);

    rs->type = type;
    rs->value = value;
    rs->flags = options;
    rs->rrddim = rd;

    DOUBLE_LINKED_LIST_APPEND_UNSAFE(rd->variables, rs, prev, next);

    rrddimvar_create_variables(rs);

    return rs;
}

void rrddimvar_rename_all(RRDDIM *rd) {
    RRDSET *st = rd->rrdset;
    (void)st;

    debug(D_VARIABLES, "RRDDIMSET rename for chart id '%s' name '%s', dimension id '%s', name '%s'", rrdset_id(st), rrdset_name(st), rrddim_id(rd), rrddim_name(rd));

    RRDDIMVAR *rs, *next = rd->variables;
    while((rs = next)) {
        next = rs->next;
        rrddimvar_create_variables(rs);
    }
}

void rrddimvar_free(RRDDIMVAR *rs) {
    RRDDIM *rd = rs->rrddim;
    debug(D_VARIABLES, "RRDDIMSET free for chart id '%s' name '%s', dimension id '%s', name '%s', prefix='%s', suffix='%s'", rrdset_id(rd->rrdset), rrdset_name(rd->rrdset), rrddim_id(rd), rrddim_name(rd), string2str(rs->prefix), string2str(rs->suffix));

    rrddimvar_free_variables(rs);

    DOUBLE_LINKED_LIST_REMOVE_UNSAFE(rd->variables, rs, prev, next);

    string_freez(rs->prefix);
    string_freez(rs->suffix);
    freez(rs);
}
