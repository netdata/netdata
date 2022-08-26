// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_HEALTH_INTERNALS
#include "rrd.h"

// ----------------------------------------------------------------------------
// RRDSETVAR management
// CHART VARIABLES

static inline void rrdsetvar_free_variables(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;
    RRDHOST *host = st->rrdhost;

    // ------------------------------------------------------------------------
    // CHART
    rrdvar_free(host, &st->rrdvar_root_index, rs->var_local);
    rs->var_local = NULL;

    // ------------------------------------------------------------------------
    // FAMILY
    rrdvar_free(host, &st->rrdfamily->rrdvar_root_index, rs->var_family);
    rs->var_family = NULL;

    rrdvar_free(host, &st->rrdfamily->rrdvar_root_index, rs->var_family_name);
    rs->var_family_name = NULL;

    // ------------------------------------------------------------------------
    // HOST
    rrdvar_free(host, &host->rrdvar_root_index, rs->var_host);
    rs->var_host = NULL;

    rrdvar_free(host, &host->rrdvar_root_index, rs->var_host_name);
    rs->var_host_name = NULL;

    // ------------------------------------------------------------------------
    // KEYS
    freez(rs->key_fullid);
    rs->key_fullid = NULL;

    freez(rs->key_fullname);
    rs->key_fullname = NULL;
}

static inline void rrdsetvar_create_variables(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;
    RRDHOST *host = st->rrdhost;

    RRDVAR_OPTIONS options = rs->options;
    if(rs->options & RRDVAR_OPTION_ALLOCATED)
        options &= ~ RRDVAR_OPTION_ALLOCATED;

    // ------------------------------------------------------------------------
    // free the old ones (if any)

    rrdsetvar_free_variables(rs);

    // ------------------------------------------------------------------------
    // KEYS

    char buffer[RRDVAR_MAX_LENGTH + 1];
    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", st->id, rs->variable);
    rs->key_fullid = strdupz(buffer);

    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", rrdset_name(st), rs->variable);
    rs->key_fullname = strdupz(buffer);

    // ------------------------------------------------------------------------
    // CHART
    rs->var_local       = rrdvar_create_and_index("local",  &st->rrdvar_root_index, rs->variable, rs->type, options, rs->value);

    // ------------------------------------------------------------------------
    // FAMILY
    rs->var_family      = rrdvar_create_and_index("family", &st->rrdfamily->rrdvar_root_index, rs->key_fullid,   rs->type, options, rs->value);
    rs->var_family_name = rrdvar_create_and_index("family", &st->rrdfamily->rrdvar_root_index, rs->key_fullname, rs->type, options, rs->value);

    // ------------------------------------------------------------------------
    // HOST
    rs->var_host        = rrdvar_create_and_index("host",   &host->rrdvar_root_index, rs->key_fullid,   rs->type, options, rs->value);
    rs->var_host_name   = rrdvar_create_and_index("host",   &host->rrdvar_root_index, rs->key_fullname, rs->type, options, rs->value);
}

RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *variable, RRDVAR_TYPE type, void *value, RRDVAR_OPTIONS options) {
    debug(D_VARIABLES, "RRDVARSET create for chart id '%s' name '%s' with variable name '%s'", st->id, rrdset_name(st), variable);
    RRDSETVAR *rs = (RRDSETVAR *)callocz(1, sizeof(RRDSETVAR));

    rs->variable = strdupz(variable);
    rs->hash = simple_hash(rs->variable);
    rs->type = type;
    rs->value = value;
    rs->options = options;
    rs->rrdset = st;

    rs->next = st->variables;
    st->variables = rs;

    rrdsetvar_create_variables(rs);

    return rs;
}

void rrdsetvar_rename_all(RRDSET *st) {
    debug(D_VARIABLES, "RRDSETVAR rename for chart id '%s' name '%s'", st->id, rrdset_name(st));

    RRDSETVAR *rs;
    for(rs = st->variables; rs ; rs = rs->next)
        rrdsetvar_create_variables(rs);

    rrdsetcalc_link_matching(st);
}

void rrdsetvar_free(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;
    debug(D_VARIABLES, "RRDSETVAR free for chart id '%s' name '%s', variable '%s'", st->id, rrdset_name(st), rs->variable);

    if(st->variables == rs) {
        st->variables = rs->next;
    }
    else {
        RRDSETVAR *t;
        for (t = st->variables; t && t->next != rs; t = t->next);
        if(!t) error("RRDSETVAR '%s' not found in chart '%s' variables linked list", rs->key_fullname, st->id);
        else t->next = rs->next;
    }

    rrdsetvar_free_variables(rs);

    freez(rs->variable);

    if(rs->options & RRDVAR_OPTION_ALLOCATED)
        freez(rs->value);

    freez(rs);
}

// --------------------------------------------------------------------------------------------------------------------
// custom chart variables

RRDSETVAR *rrdsetvar_custom_chart_variable_create(RRDSET *st, const char *name) {
    RRDHOST *host = st->rrdhost;

    char *n = strdupz(name);
    rrdvar_fix_name(n);
    uint32_t hash = simple_hash(n);

    rrdset_wrlock(st);

    // find it
    RRDSETVAR *rs;
    for(rs = st->variables; rs ; rs = rs->next) {
        if(hash == rs->hash && strcmp(n, rs->variable) == 0) {
            rrdset_unlock(st);
            if(rs->options & RRDVAR_OPTION_CUSTOM_CHART_VAR) {
                freez(n);
                return rs;
            }
            else {
                error("RRDSETVAR: custom variable '%s' on chart '%s' of host '%s', conflicts with an internal chart variable", n, st->id, host->hostname);
                freez(n);
                return NULL;
            }
        }
    }

    // not found, allocate one

    NETDATA_DOUBLE *v = mallocz(sizeof(NETDATA_DOUBLE));
    *v = NAN;

    rs = rrdsetvar_create(st, n, RRDVAR_TYPE_CALCULATED, v, RRDVAR_OPTION_ALLOCATED|RRDVAR_OPTION_CUSTOM_CHART_VAR);
    rrdset_unlock(st);

    freez(n);
    return rs;
}

void rrdsetvar_custom_chart_variable_set(RRDSETVAR *rs, NETDATA_DOUBLE value) {
    if(rs->type != RRDVAR_TYPE_CALCULATED || !(rs->options & RRDVAR_OPTION_CUSTOM_CHART_VAR) || !(rs->options & RRDVAR_OPTION_ALLOCATED)) {
        error("RRDSETVAR: requested to set variable '%s' of chart '%s' on host '%s' to value " NETDATA_DOUBLE_FORMAT
            " but the variable is not a custom chart one.", rs->variable, rs->rrdset->id, rs->rrdset->rrdhost->hostname, value);
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
