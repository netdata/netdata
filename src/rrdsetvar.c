#define NETDATA_HEALTH_INTERNALS
#include "common.h"

// ----------------------------------------------------------------------------
// RRDSETVAR management
// CHART VARIABLES

static inline void rrdsetvar_free_variables(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;

    // CHART

    rrdvar_free(st->rrdhost, &st->variables_root_index, rs->var_local);
    rs->var_local = NULL;

    // FAMILY

    rrdvar_free(st->rrdhost, &st->rrdfamily->variables_root_index, rs->var_family);
    rs->var_family = NULL;

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->var_host);
    rs->var_host = NULL;

    // HOST

    rrdvar_free(st->rrdhost, &st->rrdfamily->variables_root_index, rs->var_family_name);
    rs->var_family_name = NULL;

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->var_host_name);
    rs->var_host_name = NULL;

    // KEYS

    freez(rs->key_fullid);
    rs->key_fullid = NULL;

    freez(rs->key_fullname);
    rs->key_fullname = NULL;
}

static inline void rrdsetvar_create_variables(RRDSETVAR *rs) {
    rrdsetvar_free_variables(rs);

    RRDSET *st = rs->rrdset;

    // KEYS

    char buffer[RRDVAR_MAX_LENGTH + 1];
    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", st->id, rs->variable);
    rs->key_fullid = strdupz(buffer);

    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", st->name, rs->variable);
    rs->key_fullname = strdupz(buffer);

    // CHART

    rs->var_local       = rrdvar_create_and_index("local",  &st->variables_root_index,               rs->variable, rs->type, rs->value);

    // FAMILY

    rs->var_family      = rrdvar_create_and_index("family", &st->rrdfamily->variables_root_index,    rs->key_fullid,   rs->type, rs->value);
    rs->var_family_name = rrdvar_create_and_index("family", &st->rrdfamily->variables_root_index,    rs->key_fullname, rs->type, rs->value);

    // HOST

    rs->var_host        = rrdvar_create_and_index("host",   &st->rrdhost->variables_root_index,      rs->key_fullid,   rs->type, rs->value);
    rs->var_host_name   = rrdvar_create_and_index("host",   &st->rrdhost->variables_root_index,      rs->key_fullname, rs->type, rs->value);

}

RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *variable, RRDVAR_TYPE type, void *value, RRDVAR_OPTIONS options) {
    debug(D_VARIABLES, "RRDVARSET create for chart id '%s' name '%s' with variable name '%s'", st->id, st->name, variable);
    RRDSETVAR *rs = (RRDSETVAR *)callocz(1, sizeof(RRDSETVAR));

    rs->variable = strdupz(variable);
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
    debug(D_VARIABLES, "RRDSETVAR rename for chart id '%s' name '%s'", st->id, st->name);

    RRDSETVAR *rs, *next = st->variables;
    while((rs = next)) {
        next = rs->next;
        rrdsetvar_create_variables(rs);
    }

    rrdsetcalc_link_matching(st);
}

void rrdsetvar_free(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;
    debug(D_VARIABLES, "RRDSETVAR free for chart id '%s' name '%s', variable '%s'", st->id, st->name, rs->variable);

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
    freez(rs);
}

