// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_HEALTH_INTERNALS
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

    rrdvar_free(host, st->rrdvar_root_index, rs->var_local_id);
    rs->var_local_id = NULL;

    rrdvar_free(host, st->rrdvar_root_index, rs->var_local_name);
    rs->var_local_name = NULL;

    // FAMILY VARIABLES FOR THIS DIMENSION

    rrdvar_free(host, st->rrdfamily->rrdvar_root_index, rs->var_family_id);
    rs->var_family_id = NULL;

    rrdvar_free(host, st->rrdfamily->rrdvar_root_index, rs->var_family_name);
    rs->var_family_name = NULL;

    rrdvar_free(host, st->rrdfamily->rrdvar_root_index, rs->var_family_contextid);
    rs->var_family_contextid = NULL;

    rrdvar_free(host, st->rrdfamily->rrdvar_root_index, rs->var_family_contextname);
    rs->var_family_contextname = NULL;

    // HOST VARIABLES FOR THIS DIMENSION

    rrdvar_free(host, host->rrdvar_root_index, rs->var_host_chartidid);
    rs->var_host_chartidid = NULL;

    rrdvar_free(host, host->rrdvar_root_index, rs->var_host_chartidname);
    rs->var_host_chartidname = NULL;

    rrdvar_free(host, host->rrdvar_root_index, rs->var_host_chartnameid);
    rs->var_host_chartnameid = NULL;

    rrdvar_free(host, host->rrdvar_root_index, rs->var_host_chartnamename);
    rs->var_host_chartnamename = NULL;

    // KEYS

    string_freez(rs->key_id);
    rs->key_id = NULL;

    string_freez(rs->key_name);
    rs->key_name = NULL;

    string_freez(rs->key_fullidid);
    rs->key_fullidid = NULL;

    string_freez(rs->key_fullidname);
    rs->key_fullidname = NULL;

    string_freez(rs->key_contextid);
    rs->key_contextid = NULL;

    string_freez(rs->key_contextname);
    rs->key_contextname = NULL;

    string_freez(rs->key_fullnameid);
    rs->key_fullnameid = NULL;

    string_freez(rs->key_fullnamename);
    rs->key_fullnamename = NULL;
}

static inline void rrddimvar_create_variables(RRDDIMVAR *rs) {
    rrddimvar_free_variables(rs);

    RRDDIM *rd = rs->rrddim;
    RRDSET *st = rd->rrdset;
    RRDHOST *host = st->rrdhost;

    char buffer[RRDDIMVAR_ID_MAX + 1];

    // KEYS

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", string2str(rs->prefix), rrddim_id(rd), string2str(rs->suffix));
    rs->key_id = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", string2str(rs->prefix), rrddim_name(rd), string2str(rs->suffix));
    rs->key_name = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_id(st), string2str(rs->key_id));
    rs->key_fullidid = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_id(st), string2str(rs->key_name));
    rs->key_fullidname = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_context(st), string2str(rs->key_id));
    rs->key_contextid = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_context(st), string2str(rs->key_name));
    rs->key_contextname = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_name(st), string2str(rs->key_id));
    rs->key_fullnameid = string_strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rrdset_name(st), string2str(rs->key_name));
    rs->key_fullnamename = string_strdupz(buffer);

    // CHART VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $id
    // - $name

    rs->var_local_id           = rrdvar_create_and_index("local", st->rrdvar_root_index, rs->key_id, rs->type, RRDVAR_OPTION_DEFAULT, rs->value);
    rs->var_local_name         = rrdvar_create_and_index("local", st->rrdvar_root_index, rs->key_name, rs->type, RRDVAR_OPTION_DEFAULT, rs->value);

    // FAMILY VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $id                 (only the first, when multiple overlap)
    // - $name               (only the first, when multiple overlap)
    // - $chart-context.id
    // - $chart-context.name

    rs->var_family_id          = rrdvar_create_and_index("family", st->rrdfamily->rrdvar_root_index, rs->key_id, rs->type, RRDVAR_OPTION_DEFAULT, rs->value);
    rs->var_family_name        = rrdvar_create_and_index("family", st->rrdfamily->rrdvar_root_index, rs->key_name, rs->type, RRDVAR_OPTION_DEFAULT, rs->value);
    rs->var_family_contextid   = rrdvar_create_and_index("family", st->rrdfamily->rrdvar_root_index, rs->key_contextid, rs->type, RRDVAR_OPTION_DEFAULT, rs->value);
    rs->var_family_contextname = rrdvar_create_and_index("family", st->rrdfamily->rrdvar_root_index, rs->key_contextname, rs->type, RRDVAR_OPTION_DEFAULT, rs->value);

    // HOST VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $chart-id.id
    // - $chart-id.name
    // - $chart-name.id
    // - $chart-name.name

    rs->var_host_chartidid      = rrdvar_create_and_index("host", host->rrdvar_root_index, rs->key_fullidid, rs->type, RRDVAR_OPTION_DEFAULT, rs->value);
    rs->var_host_chartidname    = rrdvar_create_and_index("host", host->rrdvar_root_index, rs->key_fullidname, rs->type, RRDVAR_OPTION_DEFAULT, rs->value);
    rs->var_host_chartnameid    = rrdvar_create_and_index("host", host->rrdvar_root_index, rs->key_fullnameid, rs->type, RRDVAR_OPTION_DEFAULT, rs->value);
    rs->var_host_chartnamename  = rrdvar_create_and_index("host", host->rrdvar_root_index, rs->key_fullnamename, rs->type, RRDVAR_OPTION_DEFAULT, rs->value);
}

RRDDIMVAR *rrddimvar_create(RRDDIM *rd, RRDVAR_TYPE type, const char *prefix, const char *suffix, void *value, RRDVAR_OPTIONS options) {
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
    rs->options = options;
    rs->rrddim = rd;

    rs->next = rd->variables;
    rd->variables = rs;

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
    RRDSET *st = rd->rrdset;
    debug(D_VARIABLES, "RRDDIMSET free for chart id '%s' name '%s', dimension id '%s', name '%s', prefix='%s', suffix='%s'", rrdset_id(st), rrdset_name(st), rrddim_id(rd), rrddim_name(rd), string2str(rs->prefix), string2str(rs->suffix));

    rrddimvar_free_variables(rs);

    if(rd->variables == rs) {
        debug(D_VARIABLES, "RRDDIMSET removing first entry for chart id '%s' name '%s', dimension id '%s', name '%s'", rrdset_id(st), rrdset_name(st), rrddim_id(rd), rrddim_name(rd));
        rd->variables = rs->next;
    }
    else {
        debug(D_VARIABLES, "RRDDIMSET removing non-first entry for chart id '%s' name '%s', dimension id '%s', name '%s'", rrdset_id(st), rrdset_name(st), rrddim_id(rd), rrddim_name(rd));
        RRDDIMVAR *t;
        for (t = rd->variables; t && t->next != rs; t = t->next) ;
        if(!t) error("RRDDIMVAR '%s' not found in dimension '%s/%s' variables linked list", string2str(rs->key_name), rrdset_id(st), rrddim_id(rd));
        else t->next = rs->next;
    }

    string_freez(rs->prefix);
    string_freez(rs->suffix);
    freez(rs);
}
