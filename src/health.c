#include "common.h"

#define RRDVAR_MAX_LENGTH 1024

static const char *health_default_exec = PLUGINS_DIR "/alarm.sh";
int health_enabled = 1;

// ----------------------------------------------------------------------------
// RRDVAR management

static inline int rrdvar_fix_name(char *variable) {
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

int rrdvar_compare(void* a, void* b) {
    if(((RRDVAR *)a)->hash < ((RRDVAR *)b)->hash) return -1;
    else if(((RRDVAR *)a)->hash > ((RRDVAR *)b)->hash) return 1;
    else return strcmp(((RRDVAR *)a)->name, ((RRDVAR *)b)->name);
}

static inline RRDVAR *rrdvar_index_add(avl_tree_lock *tree, RRDVAR *rv) {
    RRDVAR *ret = (RRDVAR *)avl_insert_lock(tree, (avl *)(rv));
    if(ret != rv)
        debug(D_VARIABLES, "Request to insert RRDVAR '%s' into index failed. Already exists.", rv->name);

    return ret;
}

static inline RRDVAR *rrdvar_index_del(avl_tree_lock *tree, RRDVAR *rv) {
    RRDVAR *ret = (RRDVAR *)avl_remove_lock(tree, (avl *)(rv));
    if(!ret)
        error("Request to remove RRDVAR '%s' from index failed. Not Found.", rv->name);

    return ret;
}

static inline RRDVAR *rrdvar_index_find(avl_tree_lock *tree, const char *name, uint32_t hash) {
    RRDVAR tmp;
    tmp.name = (char *)name;
    tmp.hash = (hash)?hash:simple_hash(tmp.name);

    return (RRDVAR *)avl_search_lock(tree, (avl *)&tmp);
}

static inline void rrdvar_free(RRDHOST *host, avl_tree_lock *tree, RRDVAR *rv) {
    (void)host;

    if(!rv) return;

    if(tree)
        rrdvar_index_del(tree, rv);

    freez(rv->name);
    freez(rv);
}

static inline RRDVAR *rrdvar_create_and_index(const char *scope, avl_tree_lock *tree, const char *name, int type, calculated_number *value) {
    char *variable = strdupz(name);
    rrdvar_fix_name(variable);
    uint32_t hash = simple_hash(variable);

    RRDVAR *rv = rrdvar_index_find(tree, variable, hash);
    if(unlikely(!rv)) {
        debug(D_VARIABLES, "Variable '%s' not found in scope '%s'. Creating a new one.", variable, scope);

        rv = callocz(1, sizeof(RRDVAR));
        rv->name = variable;
        rv->hash = hash;
        rv->type = type;
        rv->value = value;

        RRDVAR *ret = rrdvar_index_add(tree, rv);
        if(unlikely(ret != rv)) {
            debug(D_VARIABLES, "Variable '%s' in scope '%s' already exists", variable, scope);
            rrdvar_free(NULL, NULL, rv);
            rv = NULL;
        }
        else
            debug(D_VARIABLES, "Variable '%s' created in scope '%s'", variable, scope);
    }
    else {
        // already exists
        freez(variable);
        rv = NULL;
    }

    return rv;
}

// ----------------------------------------------------------------------------
// RRDVAR lookup

calculated_number rrdvar2number(RRDVAR *rv) {
    switch(rv->type) {
        case RRDVAR_TYPE_CALCULATED: {
            calculated_number *n = (calculated_number *)rv->value;
            return *n;
        }
            break;

        case RRDVAR_TYPE_TIME_T: {
            time_t *n = (time_t *)rv->value;
            return *n;
        }
            break;

        case RRDVAR_TYPE_COLLECTED: {
            collected_number *n = (collected_number *)rv->value;
            return *n;
        }
            break;

        case RRDVAR_TYPE_TOTAL: {
            total_number *n = (total_number *)rv->value;
            return *n;
        }

        default:
            error("I don't know how to convert RRDVAR type %d to calculated_number", rv->type);
            return NAN;
            break;
    }
}

void dump_variable(void *data) {
    RRDVAR *rv = (RRDVAR *)data;
    debug(D_HEALTH, "%50s : %20.5Lf", rv->name, rrdvar2number(rv));
}

int health_variable_lookup(const char *variable, uint32_t hash, RRDCALC *rc, calculated_number *result) {
    RRDSET *st = rc->rrdset;
    RRDVAR *rv;

    if(!st) return 0;

    rv = rrdvar_index_find(&st->variables_root_index, variable, hash);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    rv = rrdvar_index_find(&st->rrdcontext->variables_root_index, variable, hash);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    rv = rrdvar_index_find(&st->rrdhost->variables_root_index, variable, hash);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    debug(D_HEALTH, "Available local chart '%s' variables:", st->id);
    avl_traverse_lock(&st->variables_root_index, dump_variable);

    debug(D_HEALTH, "Available context '%s' variables:", st->rrdcontext->id);
    avl_traverse_lock(&st->rrdcontext->variables_root_index, dump_variable);

    debug(D_HEALTH, "Available host '%s' variables:", st->rrdhost->hostname);
    avl_traverse_lock(&st->rrdhost->variables_root_index, dump_variable);

    return 0;
}

// ----------------------------------------------------------------------------
// RRDSETVAR management

RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *variable, int type, void *value, uint32_t options) {
    debug(D_VARIABLES, "RRDVARSET create for chart id '%s' name '%s' with variable name '%s'", st->id, st->name, variable);
    RRDSETVAR *rs = (RRDSETVAR *)callocz(1, sizeof(RRDSETVAR));

    char buffer[RRDVAR_MAX_LENGTH + 1];
    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", st->id, variable);
    rs->fullid = strdupz(buffer);

    snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", st->name, variable);
    rs->fullname = strdupz(buffer);

    rs->variable = strdupz(variable);

    rs->type = type;
    rs->value = value;
    rs->options = options;
    rs->rrdset = st;

    rs->local        = rrdvar_create_and_index("local",   &st->variables_root_index, rs->variable, rs->type, rs->value);
    rs->context      = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->fullid, rs->type, rs->value);
    rs->host         = rrdvar_create_and_index("host",    &st->rrdhost->variables_root_index, rs->fullid, rs->type, rs->value);
    rs->context_name = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->fullname, rs->type, rs->value);
    rs->host_name    = rrdvar_create_and_index("host",    &st->rrdhost->variables_root_index, rs->fullname, rs->type, rs->value);

    rs->next = st->variables;
    st->variables = rs;

    return rs;
}

void rrdsetvar_rename_all(RRDSET *st) {
    debug(D_VARIABLES, "RRDSETVAR rename for chart id '%s' name '%s'", st->id, st->name);

    // only these 2 can change name
    // rs->context_name
    // rs->host_name

    char buffer[RRDVAR_MAX_LENGTH + 1];
    RRDSETVAR *rs, *next = st->variables;
    while((rs = next)) {
        next = rs->next;

        snprintfz(buffer, RRDVAR_MAX_LENGTH, "%s.%s", st->name, rs->variable);

        if (strcmp(buffer, rs->fullname)) {
            // name changed
            rrdvar_free(st->rrdhost, &st->rrdcontext->variables_root_index, rs->context_name);
            rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->host_name);

            freez(rs->fullname);
            rs->fullname = strdupz(st->name);
            rs->context_name = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->fullname, rs->type, rs->value);
            rs->host_name    = rrdvar_create_and_index("host",    &st->rrdhost->variables_root_index, rs->fullname, rs->type, rs->value);
        }
    }

    rrdsetcalc_link_matching(st);
}

void rrdsetvar_free(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;
    debug(D_VARIABLES, "RRDSETVAR free for chart id '%s' name '%s', variable '%s'", st->id, st->name, rs->variable);

    rrdvar_free(st->rrdhost, &st->variables_root_index, rs->local);
    rrdvar_free(st->rrdhost, &st->rrdcontext->variables_root_index, rs->context);
    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->host);
    rrdvar_free(st->rrdhost, &st->rrdcontext->variables_root_index, rs->context_name);
    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->host_name);

    if(st->variables == rs) {
        st->variables = rs->next;
    }
    else {
        RRDSETVAR *t;
        for (t = st->variables; t && t->next != rs; t = t->next);
        if(!t) error("RRDSETVAR '%s' not found in chart '%s' variables linked list", rs->fullname, st->id);
        else t->next = rs->next;
    }

    freez(rs->fullid);
    freez(rs->fullname);
    freez(rs->variable);
    freez(rs);
}

// ----------------------------------------------------------------------------
// RRDDIMVAR management

#define RRDDIMVAR_ID_MAX 1024

RRDDIMVAR *rrddimvar_create(RRDDIM *rd, int type, const char *prefix, const char *suffix, void *value, uint32_t options) {
    RRDSET *st = rd->rrdset;

    debug(D_VARIABLES, "RRDDIMSET create for chart id '%s' name '%s', dimension id '%s', name '%s%s%s'", st->id, st->name, rd->id, (prefix)?prefix:"", rd->name, (suffix)?suffix:"");

    if(!prefix) prefix = "";
    if(!suffix) suffix = "";

    char buffer[RRDDIMVAR_ID_MAX + 1];
    RRDDIMVAR *rs = (RRDDIMVAR *)callocz(1, sizeof(RRDDIMVAR));

    rs->prefix = strdupz(prefix);
    rs->suffix = strdupz(suffix);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", rs->prefix, rd->id, rs->suffix);
    rs->id = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", rs->prefix, rd->name, rs->suffix);
    rs->name = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rd->rrdset->id, rs->id);
    rs->fullidid = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rd->rrdset->id, rs->name);
    rs->fullidname = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rd->rrdset->name, rs->id);
    rs->fullnameid = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rd->rrdset->name, rs->name);
    rs->fullnamename = strdupz(buffer);

    rs->type = type;
    rs->value = value;
    rs->options = options;
    rs->rrddim = rd;

    rs->local_id     = rrdvar_create_and_index("local",   &st->variables_root_index, rs->id, rs->type, rs->value);
    rs->local_name   = rrdvar_create_and_index("local",   &st->variables_root_index, rs->name, rs->type, rs->value);

    rs->context_id   = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->id, rs->type, rs->value);
    rs->context_name = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->name, rs->type, rs->value);

    rs->host_fullidid     = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->fullidid, rs->type, rs->value);
    rs->host_fullidname   = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->fullidname, rs->type, rs->value);
    rs->host_fullnameid   = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->fullnameid, rs->type, rs->value);
    rs->host_fullnamename = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->fullnamename, rs->type, rs->value);

    rs->next = rd->variables;
    rd->variables = rs;

    return rs;
}

void rrddimvar_rename_all(RRDDIM *rd) {
    RRDSET *st = rd->rrdset;
    debug(D_VARIABLES, "RRDDIMSET rename for chart id '%s' name '%s', dimension id '%s', name '%s'", st->id, st->name, rd->id, rd->name);

    RRDDIMVAR *rs, *next = rd->variables;
    while((rs = next)) {
        next = rs->next;

        if (strcmp(rd->name, rs->name)) {
            char buffer[RRDDIMVAR_ID_MAX + 1];
            // name changed

            // name
            rrdvar_free(st->rrdhost, &st->variables_root_index, rs->local_name);
            freez(rs->name);
            snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", rs->prefix, rd->name, rs->suffix);
            rs->name = strdupz(buffer);
            rs->local_name = rrdvar_create_and_index("local", &st->variables_root_index, rs->name, rs->type, rs->value);

            rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->host_fullidname);
            freez(rs->fullidname);
            snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->id, rs->name);
            rs->fullidname = strdupz(buffer);
            rs->host_fullidname = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index,
                                                             rs->fullidname, rs->type, rs->value);

            // fullnameid
            rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->host_fullnameid);
            freez(rs->fullnameid);
            snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->name, rs->id);
            rs->fullnameid = strdupz(buffer);
            rs->host_fullnameid = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index,
                                                          rs->fullnameid, rs->type, rs->value);

            // fullnamename
            rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->host_fullnamename);
            freez(rs->fullnamename);
            snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->name, rs->name);
            rs->fullnamename = strdupz(buffer);
            rs->host_fullnamename = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index,
                                                          rs->fullnamename, rs->type, rs->value);
        }
    }
}

void rrddimvar_free(RRDDIMVAR *rs) {
    RRDDIM *rd = rs->rrddim;
    RRDSET *st = rd->rrdset;
    debug(D_VARIABLES, "RRDDIMSET free for chart id '%s' name '%s', dimension id '%s', name '%s', prefix='%s', suffix='%s'", st->id, st->name, rd->id, rd->name, rs->prefix, rs->suffix);

    rrdvar_free(st->rrdhost, &st->variables_root_index, rs->local_id);
    rrdvar_free(st->rrdhost, &st->variables_root_index, rs->local_name);

    rrdvar_free(st->rrdhost, &st->rrdcontext->variables_root_index, rs->context_id);
    rrdvar_free(st->rrdhost, &st->rrdcontext->variables_root_index, rs->context_name);

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->host_fullidid);
    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->host_fullidname);
    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->host_fullnameid);
    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->host_fullnamename);

    if(rd->variables == rs) {
        debug(D_VARIABLES, "RRDDIMSET removing first entry for chart id '%s' name '%s', dimension id '%s', name '%s'", st->id, st->name, rd->id, rd->name);
        rd->variables = rs->next;
    }
    else {
        debug(D_VARIABLES, "RRDDIMSET removing non-first entry for chart id '%s' name '%s', dimension id '%s', name '%s'", st->id, st->name, rd->id, rd->name);
        RRDDIMVAR *t;
        for (t = rd->variables; t && t->next != rs; t = t->next) ;
        if(!t) error("RRDDIMVAR '%s' not found in dimension '%s/%s' variables linked list", rs->name, st->id, rd->id);
        else t->next = rs->next;
    }

    freez(rs->prefix);
    freez(rs->suffix);
    freez(rs->id);
    freez(rs->name);
    freez(rs->fullidid);
    freez(rs->fullidname);
    freez(rs->fullnameid);
    freez(rs->fullnamename);
    freez(rs);
}

// ----------------------------------------------------------------------------
// RRDCALC management

static void rrdsetcalc_link(RRDSET *st, RRDCALC *rc) {
    debug(D_HEALTH, "Health linking alarm '%s.%s' from chart '%s' of host '%s'", rc->chart?rc->chart:"NOCHART", rc->name, st->id, st->rrdhost->hostname);

    rc->rrdset = st;

    if(rc->green && !st->green)
        st->green = rc->green;

    if(rc->red && !st->red)
        st->red = rc->red;

    rc->local    = rrdvar_create_and_index("local", &st->variables_root_index, rc->name, RRDVAR_TYPE_CALCULATED, &rc->value);
    rc->context  = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rc->name, RRDVAR_TYPE_CALCULATED, &rc->value);

    char fullname[RRDVAR_MAX_LENGTH + 1];
    snprintfz(fullname, RRDVAR_MAX_LENGTH, "%s.%s", st->id, rc->name);
    rc->hostid   = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, fullname, RRDVAR_TYPE_CALCULATED, &rc->value);

    snprintfz(fullname, RRDVAR_MAX_LENGTH, "%s.%s", st->name, rc->name);
    rc->hostname = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, fullname, RRDVAR_TYPE_CALCULATED, &rc->value);
}

static inline int rrdcalc_is_matching_this_rrdset(RRDCALC *rc, RRDSET *st) {
    if(     (rc->hash_chart == st->hash      && !strcmp(rc->chart, st->id)) ||
            (rc->hash_chart == st->hash_name && !strcmp(rc->chart, st->name)))
        return 1;

    return 0;
}

// this has to be called while the RRDHOST is locked
inline void rrdsetcalc_link_matching(RRDSET *st) {
    // debug(D_HEALTH, "find matching alarms for chart '%s'", st->id);

    RRDCALC *rc;
    for(rc = st->rrdhost->alarms; rc ; rc = rc->next) {
        if(rc->rrdset) continue;

        if(rrdcalc_is_matching_this_rrdset(rc, st))
            rrdsetcalc_link(st, rc);
    }
}

// this has to be called while the RRDHOST is locked
inline void rrdsetcalc_unlink(RRDCALC *rc) {
    RRDSET *st = rc->rrdset;

    if(!st) {
        error("Requested to unlink RRDCALC '%s.%s' which is not linked to any RRDSET", rc->chart?rc->chart:"NOCHART", rc->name);
        return;
    }

    RRDHOST *host = st->rrdhost;

    debug(D_HEALTH, "Health unlinking alarm '%s.%s' from chart '%s' of host '%s'", rc->chart?rc->chart:"NOCHART", rc->name, st->id, host->hostname);

    // unlink it
    if(rc->rrdset_prev)
        rc->rrdset_prev->rrdset_next = rc->rrdset_next;

    if(rc->rrdset_next)
        rc->rrdset_next->rrdset_prev = rc->rrdset_prev;

    if(st->alarms == rc)
        st->alarms = rc->rrdset_next;

    rc->rrdset_prev = rc->rrdset_next = NULL;

    rrdvar_free(st->rrdhost, &st->variables_root_index, rc->local);
    rc->local = NULL;

    rrdvar_free(st->rrdhost, &st->rrdcontext->variables_root_index, rc->context);
    rc->context = NULL;

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rc->hostid);
    rc->hostid = NULL;

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rc->hostname);
    rc->hostname = NULL;

    rc->rrdset = NULL;

    // RRDCALC will remain in RRDHOST
    // so that if the matching chart is found in the future
    // it will be applied automatically
}

static inline int rrdcalc_exists(RRDHOST *host, const char *name, uint32_t hash) {
    RRDCALC *rc;

    // make sure it does not already exist
    for(rc = host->alarms; rc ; rc = rc->next) {
        if (rc->hash == hash && !strcmp(name, rc->name)) {
            error("Health alarm '%s' already exists in host '%s'.", name, host->hostname);
            return 1;
        }
    }

    return 0;
}

static inline void rrdcalc_create_part2(RRDHOST *host, RRDCALC *rc) {
    rrdhost_check_rdlock(host);

    if(rc->calculation) {
        rc->calculation->this = &rc->value;
        rc->calculation->rrdcalc = rc;
    }

    if(rc->warning) {
        rc->warning->this = &rc->value;
        rc->warning->rrdcalc = rc;
    }

    if(rc->critical) {
        rc->critical->this = &rc->value;
        rc->critical->rrdcalc = rc;
    }

    // link it to the host
    rc->next = host->alarms;
    host->alarms = rc;

    // link it to its chart
    RRDSET *st;
    for(st = host->rrdset_root; st ; st = st->next) {
        if(rrdcalc_is_matching_this_rrdset(rc, st)) {
            rrdsetcalc_link(st, rc);
            break;
        }
    }
}

static inline uint32_t rrdcalc_fullname(char *fullname, size_t len, const char *chart, const char *name) {
    snprintfz(fullname, len - 1, "%s%s%s", chart?chart:"", chart?".":"", name);
    rrdvar_fix_name(fullname);
    return simple_hash(fullname);
}

static inline RRDCALC *rrdcalc_create(RRDHOST *host, const char *name, const char *chart, const char *dimensions, int group_method,
                        int after, int before, int update_every, uint32_t options,
                        calculated_number green, calculated_number red,
                        const char *exec, const char *source,
                        const char *calc, const char *warn, const char *crit) {

    char fullname[RRDVAR_MAX_LENGTH + 1];
    uint32_t hash = rrdcalc_fullname(fullname, RRDVAR_MAX_LENGTH + 1, chart, name);

    if(rrdcalc_exists(host, fullname, hash))
        return NULL;

    RRDCALC *rc = callocz(1, sizeof(RRDCALC));

    rc->name = strdupz(name);
    rc->hash = simple_hash(rc->name);

    rc->chart = strdupz(chart);
    rc->hash_chart = simple_hash(rc->chart);

    if(dimensions) rc->dimensions = strdupz(dimensions);

    rc->group = group_method;
    rc->after = after;
    rc->before = before;
    rc->update_every = update_every;
    rc->options = options;

    rc->green = green;
    rc->red = red;

    if(exec) rc->exec = strdupz(exec);
    if(source) rc->source = strdupz(source);

    if(calc) {
        rc->calculation = expression_parse(calc, NULL, NULL);
        if(!rc->calculation)
            error("Health alarm '%s.%s': failed to parse calculation expression '%s'", chart, name, calc);
    }
    if(warn) {
        rc->warning = expression_parse(warn, NULL, NULL);
        if(!rc->warning)
            error("Health alarm '%s.%s': failed to re-parse warning expression '%s'", chart, name, warn);
    }
    if(crit) {
        rc->critical = expression_parse(crit, NULL, NULL);
        if(!rc->critical)
            error("Health alarm '%s.%s': failed to re-parse critical expression '%s'", chart, name, crit);
    }

    debug(D_HEALTH, "Health runtime added alarm '%s.%s': exec '%s', green %Lf, red %Lf, lookup: group %d, after %d, before %d, options %u, dimensions '%s', update every %d, calculation '%s', warning '%s', critical '%s', source '%s",
          (rc->chart)?rc->chart:"NOCHART",
          rc->name,
          (rc->exec)?rc->exec:"DEFAULT",
          rc->green,
          rc->red,
          rc->group,
          rc->after,
          rc->before,
          rc->options,
          (rc->dimensions)?rc->dimensions:"NONE",
          rc->update_every,
          (rc->calculation)?rc->calculation->parsed_as:"NONE",
          (rc->warning)?rc->warning->parsed_as:"NONE",
          (rc->critical)?rc->critical->parsed_as:"NONE",
          rc->source
    );

    rrdcalc_create_part2(host, rc);
    return rc;
}

void rrdcalc_free(RRDHOST *host, RRDCALC *rc) {
    if(!rc) return;

    debug(D_HEALTH, "Health removing alarm '%s.%s' of host '%s'", rc->chart?rc->chart:"NOCHART", rc->name, host->hostname);

    // unlink it from RRDSET
    if(rc->rrdset) rrdsetcalc_unlink(rc);

    // unlink it from RRDHOST
    if(rc == host->alarms)
        host->alarms = rc->next;

    else if(host->alarms) {
        RRDCALC *t, *last = host->alarms;

        for(t = last->next; t && t != rc; last = t, t = t->next) ;
        if(last && last->next == rc)
            last->next = rc->next;
        else
            error("Cannot unlink alarm '%s.%s' from host '%s': not found", rc->chart?rc->chart:"NOCHART", rc->name, host->hostname);
    }
    else
        error("Cannot unlink unlink '%s.%s' from host '%s': This host does not have any calculations", rc->chart?rc->chart:"NOCHART", rc->name, host->hostname);

    expression_free(rc->calculation);
    expression_free(rc->warning);
    expression_free(rc->critical);

    freez(rc->source);
    freez(rc->name);
    freez(rc->chart);
    freez(rc->dimensions);
    freez(rc->exec);
    freez(rc);
}

// ----------------------------------------------------------------------------
// RRDCALCTEMPLATE management

void rrdcalctemplate_link_matching(RRDSET *st) {
    RRDCALCTEMPLATE *rt;

    for(rt = st->rrdhost->templates; rt ; rt = rt->next) {
        if(rt->hash_context == st->hash_context && !strcmp(rt->context, st->context)) {

            RRDCALC *rc = rrdcalc_create(st->rrdhost, rt->name, st->id,
                           rt->dimensions, rt->group, rt->after, rt->before, rt->update_every, rt->options,
                           rt->green, rt->red, rt->exec, rt->source,
                           (rt->calculation)?rt->calculation->source:NULL,
                           (rt->warning)?rt->warning->source:NULL,
                           (rt->critical)?rt->critical->source:NULL);

            if(!rc)
                error("Health tried to create alarm from template '%s', but it failed", rt->name);

#ifdef NETDATA_INTERNAL_CHECKS
            else if(rc->rrdset != st)
                error("Health alarm '%s.%s' should be linked to chart '%s', but it is not", rc->chart?rc->chart:"NOCHART", rc->name, st->id);
#else
            (void)rc;
#endif
        }
    }
}

static inline void rrdcalctemplate_free(RRDHOST *host, RRDCALCTEMPLATE *rt) {
    debug(D_HEALTH, "Health removing template '%s' of host '%s'", rt->name, host->hostname);

    if(host->templates) {
        if(host->templates == rt) {
            host->templates = rt->next;
        }
        else {
            RRDCALCTEMPLATE *t, *last = host->templates;
            for (t = last->next; t && t != rt; last = t, t = t->next ) ;
            if(last && last->next == rt) {
                last->next = rt->next;
                rt->next = NULL;
            }
            else
                error("Cannot find RRDCALCTEMPLATE '%s' linked in host '%s'", rt->name, host->hostname);
        }
    }

    expression_free(rt->calculation);
    expression_free(rt->warning);
    expression_free(rt->critical);

    freez(rt->dimensions);
    freez(rt->context);
    freez(rt->name);
    freez(rt->exec);
    freez(rt->source);
    freez(rt);
}

// ----------------------------------------------------------------------------
// load health configuration

#define HEALTH_CONF_MAX_LINE 4096

#define HEALTH_ALARM_KEY "alarm"
#define HEALTH_TEMPLATE_KEY "template"
#define HEALTH_ON_KEY "on"
#define HEALTH_LOOKUP_KEY "lookup"
#define HEALTH_CALC_KEY "calc"
#define HEALTH_EVERY_KEY "every"
#define HEALTH_GREEN_KEY "green"
#define HEALTH_RED_KEY "red"
#define HEALTH_WARN_KEY "warn"
#define HEALTH_CRIT_KEY "crit"
#define HEALTH_EXEC_KEY "exec"

static inline int rrdcalc_add_alarm_from_config(RRDHOST *host, RRDCALC *rc) {
    {
        char fullname[RRDVAR_MAX_LENGTH + 1];
        uint32_t hash = rrdcalc_fullname(fullname, RRDVAR_MAX_LENGTH + 1, rc->chart, rc->name);

        if (rrdcalc_exists(host, fullname, hash))
            return 0;
    }

    if(!rc->chart) {
        error("Health configuration for alarm '%s' does not have a chart", rc->name);
        return 0;
    }

    if(!RRDCALC_HAS_DB_LOOKUP(rc) && !rc->warning && !rc->critical) {
        error("Health configuration for alarm '%s.%s' is useless (no calculation, no warning and no critical evaluation)", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    debug(D_HEALTH, "Health configuration adding alarm '%s.%s': exec '%s', green %Lf, red %Lf, lookup: group %d, after %d, before %d, options %u, dimensions '%s', update every %d, calculation '%s', warning '%s', critical '%s', source '%s",
          rc->chart?rc->chart:"NOCHART",
          rc->name,
          (rc->exec)?rc->exec:"DEFAULT",
          rc->green,
          rc->red,
          rc->group,
          rc->after,
          rc->before,
          rc->options,
          (rc->dimensions)?rc->dimensions:"NONE",
          rc->update_every,
          (rc->calculation)?rc->calculation->parsed_as:"NONE",
          (rc->warning)?rc->warning->parsed_as:"NONE",
          (rc->critical)?rc->critical->parsed_as:"NONE",
          rc->source
    );

    rrdcalc_create_part2(host, rc);
    return 1;
}

static inline int rrdcalctemplate_add_template_from_config(RRDHOST *host, RRDCALCTEMPLATE *rt) {
    if(!rt->context) {
        error("Health configuration for template '%s' does not have a context", rt->name);
        return 0;
    }

    if(!RRDCALCTEMPLATE_HAS_CALCULATION(rt) && !rt->warning && !rt->critical) {
        error("Health configuration for template '%s' is useless (no calculation, no warning and no critical evaluation)", rt->name);
        return 0;
    }

    RRDCALCTEMPLATE *t;
    for (t = host->templates; t ; t = t->next) {
        if(t->hash_name == rt->hash_name && !strcmp(t->name, rt->name)) {
            error("Health configuration template '%s' already exists for host '%s'.", rt->name, host->hostname);
            return 0;
        }
    }

    debug(D_HEALTH, "Health configuration adding template '%s': context '%s', exec '%s', green %Lf, red %Lf, lookup: group %d, after %d, before %d, options %u, dimensions '%s', update every %d, calculation '%s', warning '%s', critical '%s', source '%s'",
          rt->name,
          (rt->context)?rt->context:"NONE",
          (rt->exec)?rt->exec:"DEFAULT",
          rt->green,
          rt->red,
          rt->group,
          rt->after,
          rt->before,
          rt->options,
          (rt->dimensions)?rt->dimensions:"NONE",
          rt->update_every,
          (rt->calculation)?rt->calculation->parsed_as:"NONE",
          (rt->warning)?rt->warning->parsed_as:"NONE",
          (rt->critical)?rt->critical->parsed_as:"NONE",
          rt->source
    );

    rt->next = host->templates;
    host->templates = rt;
    return 1;
}

static inline int health_parse_duration(char *string, int *result) {
    // make sure it is a number
    if(!*string || !(isdigit(*string) || *string == '+' || *string == '-')) {
        *result = 0;
        return 0;
    }

    char *e = NULL;
    calculated_number n = strtold(string, &e);
    if(e && *e) {
        switch (*e) {
            case 'Y':
                *result = (int) (n * 86400 * 365);
                break;
            case 'M':
                *result = (int) (n * 86400 * 30);
                break;
            case 'w':
                *result = (int) (n * 86400 * 7);
                break;
            case 'd':
                *result = (int) (n * 86400);
                break;
            case 'h':
                *result = (int) (n * 3600);
                break;
            case 'm':
                *result = (int) (n * 60);
                break;

            default:
            case 's':
                *result = (int) (n);
                break;
        }
    }
    else
       *result = (int)(n);

    return 1;
}

static inline int health_parse_db_lookup(
        size_t line, const char *path, const char *file, char *string,
        int *group_method, int *after, int *before, int *every,
        uint32_t *options, char **dimensions
) {
    debug(D_HEALTH, "Health configuration parsing database lookup %zu@%s/%s: %s", line, path, file, string);

    if(*dimensions) freez(*dimensions);
    *dimensions = NULL;
    *after = 0;
    *before = 0;
    *every = 0;
    *options = 0;

    char *s = string, *key;

    // first is the group method
    key = s;
    while(*s && !isspace(*s)) s++;
    while(*s && isspace(*s)) *s++ = '\0';
    if(!*s) {
        error("Health configuration invalid chart calculation at line %zu of file '%s/%s': expected group method followed by the 'after' time, but got '%s'",
              line, path, file, key);
        return 0;
    }

    if((*group_method = web_client_api_request_v1_data_group(key, -1)) == -1) {
        error("Health configuration at line %zu of file '%s/%s': invalid group method '%s'",
              line, path, file, key);
        return 0;
    }

    // then is the 'after' time
    key = s;
    while(*s && !isspace(*s)) s++;
    while(*s && isspace(*s)) *s++ = '\0';

    if(!health_parse_duration(key, after)) {
        error("Health configuration at line %zu of file '%s/%s': invalid duration '%s' after group method",
              line, path, file, key);
        return 0;
    }

    // sane defaults
    *every = abs(*after);

    // now we may have optional parameters
    while(*s) {
        key = s;
        while(*s && !isspace(*s)) s++;
        while(*s && isspace(*s)) *s++ = '\0';
        if(!*key) break;

        if(!strcasecmp(key, "at")) {
            char *value = s;
            while(*s && !isspace(*s)) s++;
            while(*s && isspace(*s)) *s++ = '\0';

            if (!health_parse_duration(value, before)) {
                error("Health configuration at line %zu of file '%s/%s': invalid duration '%s' for '%s' keyword",
                      line, path, file, value, key);
            }
        }
        else if(!strcasecmp(key, HEALTH_EVERY_KEY)) {
            char *value = s;
            while(*s && !isspace(*s)) s++;
            while(*s && isspace(*s)) *s++ = '\0';

            if (!health_parse_duration(value, every)) {
                error("Health configuration at line %zu of file '%s/%s': invalid duration '%s' for '%s' keyword",
                      line, path, file, value, key);
            }
        }
        else if(!strcasecmp(key, "absolute") || !strcasecmp(key, "abs") || !strcasecmp(key, "absolute_sum")) {
            *options |= RRDR_OPTION_ABSOLUTE;
        }
        else if(!strcasecmp(key, "min2max")) {
            *options |= RRDR_OPTION_MIN2MAX;
        }
        else if(!strcasecmp(key, "null2zero")) {
            *options |= RRDR_OPTION_NULL2ZERO;
        }
        else if(!strcasecmp(key, "percentage")) {
            *options |= RRDR_OPTION_PERCENTAGE;
        }
        else if(!strcasecmp(key, "unaligned")) {
            *options |= RRDR_OPTION_NOT_ALIGNED;
        }
        else if(!strcasecmp(key, "of")) {
            if(*s && strcasecmp(s, "all"))
               *dimensions = strdupz(s);
            break;
        }
        else {
            error("Health configuration at line %zu of file '%s/%s': unknown keyword '%s'",
                  line, path, file, key);
        }
    }

    return 1;
}

static inline char *health_source_file(int line, const char *path, const char *filename) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%d@%s/%s", line, path, filename);
    return strdupz(buffer);
}

int health_readfile(const char *path, const char *filename) {
    debug(D_HEALTH, "Health configuration reading file '%s/%s'", path, filename);

    static uint32_t hash_alarm = 0, hash_template = 0, hash_on = 0, hash_calc = 0, hash_green = 0, hash_red = 0, hash_warn = 0, hash_crit = 0, hash_exec = 0, hash_every = 0, hash_lookup = 0;
    char buffer[HEALTH_CONF_MAX_LINE + 1];

    if(unlikely(!hash_alarm)) {
        hash_alarm = simple_uhash(HEALTH_ALARM_KEY);
        hash_template = simple_uhash(HEALTH_TEMPLATE_KEY);
        hash_on = simple_uhash(HEALTH_ON_KEY);
        hash_calc = simple_uhash(HEALTH_CALC_KEY);
        hash_lookup = simple_uhash(HEALTH_LOOKUP_KEY);
        hash_green = simple_uhash(HEALTH_GREEN_KEY);
        hash_red = simple_uhash(HEALTH_RED_KEY);
        hash_warn = simple_uhash(HEALTH_WARN_KEY);
        hash_crit = simple_uhash(HEALTH_CRIT_KEY);
        hash_exec = simple_uhash(HEALTH_EXEC_KEY);
        hash_every = simple_uhash(HEALTH_EVERY_KEY);
    }

    snprintfz(buffer, HEALTH_CONF_MAX_LINE, "%s/%s", path, filename);
    FILE *fp = fopen(buffer, "r");
    if(!fp) {
        error("Health configuration cannot read file '%s'.", buffer);
        return 0;
    }

    RRDCALC *rc = NULL;
    RRDCALCTEMPLATE *rt = NULL;

    size_t line = 0, append = 0;
    char *s;
    while((s = fgets(&buffer[append], (int)(HEALTH_CONF_MAX_LINE - append), fp)) || append) {
        int stop_appending = !s;
        line++;
        // info("Line %zu of file '%s/%s': '%s'", line, path, filename, s);
        s = trim(buffer);
        if(!s) continue;
        // info("Trimmed line %zu of file '%s/%s': '%s'", line, path, filename, s);

        append = strlen(s);
        if(!stop_appending && s[append - 1] == '\\') {
            s[append - 1] = ' ';
            append = &s[append] - buffer;
            if(append < HEALTH_CONF_MAX_LINE)
                continue;
            continue;
        }
        append = 0;

        char *key = s;
        while(*s && *s != ':') s++;
        if(!*s) {
            error("Health configuration has invalid line %zu of file '%s/%s'. It does not contain a ':'. Ignoring it.", line, path, filename);
            continue;
        }
        *s = '\0';
        s++;

        char *value = s;
        key = trim(key);
        value = trim(value);

        if(!key) {
            error("Health configuration has invalid line %zu of file '%s/%s'. Keyword is empty. Ignoring it.", line, path, filename);
            continue;
        }

        if(!value) {
            error("Health configuration has invalid line %zu of file '%s/%s'. value is empty. Ignoring it.", line, path, filename);
            continue;
        }

        // info("Health file '%s/%s', key '%s', value '%s'", path, filename, key, value);
        uint32_t hash = simple_uhash(key);

        if(hash == hash_alarm && !strcasecmp(key, HEALTH_ALARM_KEY)) {
            if(rc && !rrdcalc_add_alarm_from_config(&localhost, rc))
                rrdcalc_free(&localhost, rc);

            if(rt) {
                if (!rrdcalctemplate_add_template_from_config(&localhost, rt))
                    rrdcalctemplate_free(&localhost, rt);
                rt = NULL;
            }

            rc = callocz(1, sizeof(RRDCALC));
            rc->name = strdupz(value);
            rc->hash = simple_hash(rc->name);
            rc->source = health_source_file(line, path, filename);

            if(rrdvar_fix_name(rc->name))
                error("Health configuration renamed alarm '%s' to '%s'", value, rc->name);
        }
        else if(hash == hash_template && !strcasecmp(key, HEALTH_TEMPLATE_KEY)) {
            if(rc) {
                if(!rrdcalc_add_alarm_from_config(&localhost, rc))
                    rrdcalc_free(&localhost, rc);
                rc = NULL;
            }

            if(rt && !rrdcalctemplate_add_template_from_config(&localhost, rt))
                rrdcalctemplate_free(&localhost, rt);

            rt = callocz(1, sizeof(RRDCALCTEMPLATE));
            rt->name = strdupz(value);
            rt->hash_name = simple_hash(rt->name);
            rt->source = health_source_file(line, path, filename);

            if(rrdvar_fix_name(rt->name))
                error("Health configuration renamed template '%s' to '%s'", value, rt->name);
        }
        else if(rc) {
            if(hash == hash_on && !strcasecmp(key, HEALTH_ON_KEY)) {
                if(rc->chart) {
                    if(strcmp(rc->chart, value))
                        info("Health configuration at line %zu of file '%s/%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                             line, path, filename, rc->name, key, rc->chart, value, value);

                    freez(rc->chart);
                }
                rc->chart = strdupz(value);
                rc->hash_chart = simple_hash(rc->chart);
            }
            else if(hash == hash_lookup && !strcasecmp(key, HEALTH_LOOKUP_KEY)) {
                health_parse_db_lookup(line, path, filename, value, &rc->group, &rc->after, &rc->before,
                                       &rc->update_every,
                                       &rc->options, &rc->dimensions);
            }
            else if(hash == hash_every && !strcasecmp(key, HEALTH_EVERY_KEY)) {
                if(!health_parse_duration(value, &rc->update_every))
                    info("Health configuration at line %zu of file '%s/%s' for alarm '%s' at key '%s' cannot parse duration: '%s'.",
                         line, path, filename, rc->name, key, value);
            }
            else if(hash == hash_green && !strcasecmp(key, HEALTH_GREEN_KEY)) {
                char *e;
                rc->green = strtold(value, &e);
                if(e && *e) {
                    info("Health configuration at line %zu of file '%s/%s' for alarm '%s' at key '%s' leaves this string unmatched: '%s'.",
                         line, path, filename, rc->name, key, e);
                }
            }
            else if(hash == hash_red && !strcasecmp(key, HEALTH_RED_KEY)) {
                char *e;
                rc->red = strtold(value, &e);
                if(e && *e) {
                    info("Health configuration at line %zu of file '%s/%s' for alarm '%s' at key '%s' leaves this string unmatched: '%s'.",
                         line, path, filename, rc->name, key, e);
                }
            }
            else if(hash == hash_calc && !strcasecmp(key, HEALTH_CALC_KEY)) {
                const char *failed_at = NULL;
                int error = 0;
                rc->calculation = expression_parse(value, &failed_at, &error);
                if(!rc->calculation) {
                    error("Health configuration at line %zu of file '%s/%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                          line, path, filename, rc->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_warn && !strcasecmp(key, HEALTH_WARN_KEY)) {
                const char *failed_at = NULL;
                int error = 0;
                rc->warning = expression_parse(value, &failed_at, &error);
                if(!rc->warning) {
                    error("Health configuration at line %zu of file '%s/%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                          line, path, filename, rc->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_crit && !strcasecmp(key, HEALTH_CRIT_KEY)) {
                const char *failed_at = NULL;
                int error = 0;
                rc->critical = expression_parse(value, &failed_at, &error);
                if(!rc->critical) {
                    error("Health configuration at line %zu of file '%s/%s' for alarm '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                          line, path, filename, rc->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_exec && !strcasecmp(key, HEALTH_EXEC_KEY)) {
                if(rc->exec) {
                    if(strcmp(rc->exec, value))
                        info("Health configuration at line %zu of file '%s/%s' for alarm '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                             line, path, filename, rc->name, key, rc->exec, value, value);

                    freez(rc->exec);
                }
                rc->exec = strdupz(value);
            }
            else {
                error("Health configuration at line %zu of file '%s/%s' for alarm '%s' has unknown key '%s'.",
                     line, path, filename, rc->name, key);
            }
        }
        else if(rt) {
            if(hash == hash_on && !strcasecmp(key, HEALTH_ON_KEY)) {
                if(rt->context) {
                    if(strcmp(rt->context, value))
                        info("Health configuration at line %zu of file '%s/%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                             line, path, filename, rt->name, key, rt->context, value, value);

                    freez(rt->context);
                }
                rt->context = strdupz(value);
                rt->hash_context = simple_hash(rt->context);
            }
            else if(hash == hash_lookup && !strcasecmp(key, HEALTH_LOOKUP_KEY)) {
                health_parse_db_lookup(line, path, filename, value, &rt->group, &rt->after, &rt->before,
                                       &rt->update_every,
                                       &rt->options, &rt->dimensions);
            }
            else if(hash == hash_every && !strcasecmp(key, HEALTH_EVERY_KEY)) {
                if(!health_parse_duration(value, &rt->update_every))
                    info("Health configuration at line %zu of file '%s/%s' for template '%s' at key '%s' cannot parse duration: '%s'.",
                         line, path, filename, rt->name, key, value);
            }
            else if(hash == hash_green && !strcasecmp(key, HEALTH_GREEN_KEY)) {
                char *e;
                rt->green = strtold(value, &e);
                if(e && *e) {
                    info("Health configuration at line %zu of file '%s/%s' for template '%s' at key '%s' leaves this string unmatched: '%s'.",
                         line, path, filename, rt->name, key, e);
                }
            }
            else if(hash == hash_red && !strcasecmp(key, HEALTH_RED_KEY)) {
                char *e;
                rt->red = strtold(value, &e);
                if(e && *e) {
                    info("Health configuration at line %zu of file '%s/%s' for template '%s' at key '%s' leaves this string unmatched: '%s'.",
                         line, path, filename, rt->name, key, e);
                }
            }
            else if(hash == hash_calc && !strcasecmp(key, HEALTH_CALC_KEY)) {
                const char *failed_at = NULL;
                int error = 0;
                rt->calculation = expression_parse(value, &failed_at, &error);
                if(!rt->calculation) {
                    error("Health configuration at line %zu of file '%s/%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                          line, path, filename, rt->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_warn && !strcasecmp(key, HEALTH_WARN_KEY)) {
                const char *failed_at = NULL;
                int error = 0;
                rt->warning = expression_parse(value, &failed_at, &error);
                if(!rt->warning) {
                    error("Health configuration at line %zu of file '%s/%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                          line, path, filename, rt->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_crit && !strcasecmp(key, HEALTH_CRIT_KEY)) {
                const char *failed_at = NULL;
                int error = 0;
                rt->critical = expression_parse(value, &failed_at, &error);
                if(!rt->critical) {
                    error("Health configuration at line %zu of file '%s/%s' for template '%s' at key '%s' has unparse-able expression '%s': %s at '%s'",
                          line, path, filename, rt->name, key, value, expression_strerror(error), failed_at);
                }
            }
            else if(hash == hash_exec && !strcasecmp(key, HEALTH_EXEC_KEY)) {
                if(rt->exec) {
                    if(strcmp(rt->exec, value))
                        info("Health configuration at line %zu of file '%s/%s' for template '%s' has key '%s' twice, once with value '%s' and later with value '%s'. Using ('%s').",
                             line, path, filename, rt->name, key, rt->exec, value, value);

                    freez(rt->exec);
                }
                rt->exec = strdupz(value);
            }
            else {
                error("Health configuration at line %zu of file '%s/%s' for template '%s' has unknown key '%s'.",
                      line, path, filename, rt->name, key);
            }
        }
        else {
            error("Health configuration at line %zu of file '%s/%s' has unknown key '%s'. Expected either '" HEALTH_ALARM_KEY "' or '" HEALTH_TEMPLATE_KEY "'.",
                  line, path, filename, key);
        }
    }

    if(rc && !rrdcalc_add_alarm_from_config(&localhost, rc))
        rrdcalc_free(&localhost, rc);

    if(rt && !rrdcalctemplate_add_template_from_config(&localhost, rt))
        rrdcalctemplate_free(&localhost, rt);

    fclose(fp);
    return 1;
}

void health_readdir(const char *path) {
    size_t pathlen = strlen(path);

    debug(D_HEALTH, "Health configuration reading directory '%s'", path);

    DIR *dir = opendir(path);
    if (!dir) {
        error("Health configuration cannot open directory '%s'.", path);
        return;
    }

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        size_t len = strlen(de->d_name);

        if(de->d_type == DT_DIR
           && (
                   (de->d_name[0] == '.' && de->d_name[1] == '\0')
                   || (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
           ))
            continue;

        else if(de->d_type == DT_DIR) {
            char *s = mallocz(pathlen + strlen(de->d_name) + 2);
            strcpy(s, path);
            strcat(s, "/");
            strcat(s, de->d_name);
            health_readdir(s);
            freez(s);
            continue;
        }

        else if((de->d_type == DT_LNK || de->d_type == DT_REG) &&
                len > 5 && !strcmp(&de->d_name[len - 5], ".conf")) {
            health_readfile(path, de->d_name);
        }
    }

    closedir(dir);
}

static inline char *health_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", config_get("global", "config directory", CONFIG_DIR));
    return config_get("health", "health configuration directory", buffer);
}

void health_init(void) {
    debug(D_HEALTH, "Health configuration initializing");

    if(!(health_enabled = config_get_boolean("health", "enabled", 1))) {
        debug(D_HEALTH, "Health is disabled.");
        return;
    }

    char *path = health_config_dir();

    {
        char buffer[FILENAME_MAX + 1];
        snprintfz(buffer, FILENAME_MAX, "%s/alarm.sh", config_get("global", "plugins directory", PLUGINS_DIR));
        health_default_exec = config_get("health", "script to execute on alarm", buffer);
    }

    rrdhost_rwlock(&localhost);
    health_readdir(path);
    rrdhost_unlock(&localhost);
}

// ----------------------------------------------------------------------------
// re-load health configuration

static inline void health_free_all_nolock(RRDHOST *host) {
    while(host->templates)
        rrdcalctemplate_free(host, host->templates);

    while(host->alarms)
        rrdcalc_free(host, host->alarms);
}

void health_reload(void) {
    if(!health_enabled) {
        error("Health reload is requested, but health is not enabled.");
        return;
    }

    char *path = health_config_dir();

    rrdhost_rwlock(&localhost);
    health_free_all_nolock(&localhost);
    rrdhost_unlock(&localhost);

    rrdhost_rwlock(&localhost);
    health_readdir(path);
    rrdhost_unlock(&localhost);

    RRDSET *st;
    for(st = localhost.rrdset_root; st ; st = st->next) {
        rrdhost_rwlock(&localhost);

        rrdsetcalc_link_matching(st);
        rrdcalctemplate_link_matching(st);

        rrdhost_unlock(&localhost);
    }
}


// ----------------------------------------------------------------------------
// health main thread and friends

static inline int rrdcalc_isrunnable(RRDCALC *rc, time_t now, time_t *next_run) {
    if (unlikely(!rc->rrdset)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. It is not linked to a chart.", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    if (unlikely(!rc->update_every)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. It does not have an update frequency", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    if (unlikely(rc->next_update > now)) {
        if (*next_run > rc->next_update)
            *next_run = rc->next_update;

        debug(D_HEALTH, "Health not examining alarm '%s.%s' yet (will do in %d secs).", rc->chart?rc->chart:"NOCHART", rc->name, (int) (rc->next_update - now));
        return 0;
    }

    return 1;
}

static inline int rrdcalc_value2status(calculated_number n) {
    if(isnan(n)) return RRDCALC_STATUS_UNDEFINED;
    if(n) return RRDCALC_STATUS_RAISED;
    return RRDCALC_STATUS_OFF;
}

static inline const char *rrdcalc_status2string(int status) {
    switch(status) {
        case RRDCALC_STATUS_UNINITIALIZED:
            return "UNINITIALIZED";

        case RRDCALC_STATUS_UNDEFINED:
            return "UNDEFINED";

        case RRDCALC_STATUS_RAISED:
            return "RAISED";

        case RRDCALC_STATUS_OFF:
            return "OFF";

        default:
            return "UNKNOWN";
    }
}

void rrdcalc_check_critical_event(RRDCALC *rc) {
    calculated_number n = rc->critical->result;

    int old_status = rc->critical_status;
    int new_status = rrdcalc_value2status(n);

    if(new_status != old_status) {
        info("Health alarm '%s.%s' - CRITICAL condition changed status from %s to %s",
             rc->chart?rc->chart:"NOCHART", rc->name,
             rrdcalc_status2string(old_status),
             rrdcalc_status2string(new_status)
        );

        rc->critical_status = new_status;
    }
}

void rrdcalc_check_warning_event(RRDCALC *rc) {
    calculated_number n = rc->warning->result;

    int old_status = rc->warning_status;
    int new_status = rrdcalc_value2status(n);

    if(new_status != old_status) {
        info("Health alarm '%s.%s' - WARNING condition changed status from %s to %s",
             rc->chart?rc->chart:"NOCHART", rc->name,
             rrdcalc_status2string(old_status),
             rrdcalc_status2string(new_status)
        );

        rc->warning_status = new_status;
    }
}

void *health_main(void *ptr) {
    (void)ptr;

    info("HEALTH thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    int min_run_every = (int)config_get_number("health", "run at least every seconds", 10);
    if(min_run_every < 1) min_run_every = 1;

    BUFFER *wb = buffer_create(100);

    unsigned int loop = 0;
    while(health_enabled) {
        loop++;
        debug(D_HEALTH, "Health monitoring iteration no %u started", loop);

        int oldstate, runnable = 0;
        time_t now = time(NULL);
        time_t next_run = now + min_run_every;
        RRDCALC *rc;

        if (unlikely(pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, &oldstate) != 0))
            error("Cannot set pthread cancel state to DISABLE.");

        rrdhost_rdlock(&localhost);

        // the first loop is to lookup values from the db
        for (rc = localhost.alarms; rc; rc = rc->next) {
            if (unlikely(!rrdcalc_isrunnable(rc, now, &next_run)))
                continue;

            runnable++;

            // 1. if there is database lookup, do it
            // 2. if there is calculation expression, run it

            if (unlikely(RRDCALC_HAS_DB_LOOKUP(rc))) {
                time_t old_db_timestamp = rc->db_timestamp;
                int value_is_null = 0;

                int ret = rrd2value(rc->rrdset, wb, &rc->value,
                                    rc->dimensions, 1, rc->after, rc->before, rc->group,
                                    rc->options, &rc->db_timestamp, &value_is_null);

                if (unlikely(ret != 200)) {
                    // database lookup failed
                    rc->value = NAN;

                    if (unlikely(!(rc->rrdcalc_options & RRDCALC_OPTION_DB_ERROR))) {
                        rc->rrdcalc_options |= RRDCALC_OPTION_DB_ERROR;
                        error("Health alarm '%s.%s': database lookup returned error %d", rc->chart?rc->chart:"NOCHART", rc->name, ret);
                    }
                }
                else if (unlikely(rc->rrdcalc_options & RRDCALC_OPTION_DB_ERROR))
                    rc->rrdcalc_options &= ~RRDCALC_OPTION_DB_ERROR;

                if (unlikely(old_db_timestamp == rc->db_timestamp)) {
                    // database is stale

                    if (unlikely(!(rc->rrdcalc_options & RRDCALC_OPTION_DB_STALE))) {
                        rc->rrdcalc_options |= RRDCALC_OPTION_DB_STALE;
                        error("Health alarm '%s.%s': database is stale", rc->chart?rc->chart:"NOCHART", rc->name);
                    }
                }
                else if (unlikely(rc->rrdcalc_options & RRDCALC_OPTION_DB_STALE))
                    rc->rrdcalc_options &= ~RRDCALC_OPTION_DB_STALE;

                if (unlikely(value_is_null)) {
                    // collected value is null

                    rc->value = NAN;

                    if (unlikely(!(rc->rrdcalc_options & RRDCALC_OPTION_DB_NAN))) {
                        rc->rrdcalc_options |= RRDCALC_OPTION_DB_NAN;
                        error("Health alarm '%s.%s': database lookup returned empty value (possibly value is not collected yet)",
                              rc->chart?rc->chart:"NOCHART", rc->name);
                    }
                }
                else if (unlikely(rc->rrdcalc_options & RRDCALC_OPTION_DB_NAN))
                    rc->rrdcalc_options &= ~RRDCALC_OPTION_DB_NAN;

                debug(D_HEALTH, "Health alarm '%s.%s': database lookup gave value "
                        CALCULATED_NUMBER_FORMAT, rc->chart?rc->chart:"NOCHART", rc->name, rc->value);
            }

            if(unlikely(rc->calculation)) {
                if (unlikely(!expression_evaluate(rc->calculation))) {
                    // calculation failed

                    rc->value = NAN;

                    if (unlikely(!(rc->rrdcalc_options & RRDCALC_OPTION_CALC_ERROR))) {
                        rc->rrdcalc_options |= RRDCALC_OPTION_CALC_ERROR;
                        error("Health alarm '%s.%s': failed to evaluate calculation with error: %s",
                              rc->chart?rc->chart:"NOCHART", rc->name, buffer_tostring(rc->calculation->error_msg));
                    }
                }
                else {
                    if (unlikely(rc->rrdcalc_options & RRDCALC_OPTION_CALC_ERROR))
                        rc->rrdcalc_options &= ~RRDCALC_OPTION_CALC_ERROR;

                    debug(D_HEALTH, "Health alarm '%s.%s': calculation expression gave value "
                            CALCULATED_NUMBER_FORMAT
                            ": %s (source: %s)",
                          rc->chart?rc->chart:"NOCHART", rc->name,
                          rc->calculation->result,
                          buffer_tostring(rc->calculation->error_msg),
                          rc->source
                    );

                    rc->value = rc->calculation->result;
                }
            }
        }
        rrdhost_unlock(&localhost);

        if (runnable) {
            rrdhost_rdlock(&localhost);

            for (rc = localhost.alarms; rc; rc = rc->next) {
                if (unlikely(!rrdcalc_isrunnable(rc, now, &next_run)))
                    continue;

                if(unlikely(rc->warning)) {
                    if(unlikely(!expression_evaluate(rc->warning))) {
                        // calculation failed
                        if (unlikely(!(rc->rrdcalc_options & RRDCALC_OPTION_WARN_ERROR))) {
                            rc->rrdcalc_options |= RRDCALC_OPTION_WARN_ERROR;
                            error("Health alarm '%s.%s': warning expression failed with error: %s",
                                  rc->chart?rc->chart:"NOCHART", rc->name, buffer_tostring(rc->warning->error_msg));
                        }
                    }
                    else {
                        if(unlikely(rc->rrdcalc_options & RRDCALC_OPTION_WARN_ERROR))
                            rc->rrdcalc_options &= ~RRDCALC_OPTION_WARN_ERROR;

                        debug(D_HEALTH, "Health alarm '%s.%s': warning expression gave value "
                                CALCULATED_NUMBER_FORMAT
                                ": %s (source: %s)",
                              rc->chart?rc->chart:"NOCHART", rc->name,
                              rc->warning->result,
                              buffer_tostring(rc->warning->error_msg),
                              rc->source
                        );
                    }

                    rrdcalc_check_warning_event(rc);
                }

                if(unlikely(rc->critical)) {
                    if(unlikely(!expression_evaluate(rc->critical))) {
                        // calculation failed
                        if (unlikely(!(rc->rrdcalc_options & RRDCALC_OPTION_CRIT_ERROR))) {
                            rc->rrdcalc_options |= RRDCALC_OPTION_CRIT_ERROR;
                            error("Health alarm '%s.%s': critical expression failed with error: %s",
                                  rc->chart?rc->chart:"NOCHART", rc->name, buffer_tostring(rc->critical->error_msg));
                        }
                    }
                    else {
                        if(unlikely(rc->rrdcalc_options & RRDCALC_OPTION_CRIT_ERROR))
                            rc->rrdcalc_options &= ~RRDCALC_OPTION_CRIT_ERROR;

                        debug(D_HEALTH, "Health alarm '%s.%s': critical expression gave value "
                                CALCULATED_NUMBER_FORMAT
                                ": %s (source: %s)",
                              rc->chart?rc->chart:"NOCHART", rc->name,
                              rc->critical->result,
                              buffer_tostring(rc->critical->error_msg),
                              rc->source
                        );
                    }

                    rrdcalc_check_critical_event(rc);
                }

                rc->last_updated = now;
                rc->next_update = now + rc->update_every;

                if (next_run > rc->next_update)
                    next_run = rc->next_update;
            }

            rrdhost_unlock(&localhost);
        }


        if (unlikely(pthread_setcancelstate(oldstate, NULL) != 0))
            error("Cannot set pthread cancel state to RESTORE (%d).", oldstate);

        debug(D_HEALTH, "Health monitoring iteration no %u done. Next iteration in %d secs",
              loop, (int) (next_run - now));

        sleep_usec(1000000 * (unsigned long long) (next_run - now));
    }

    buffer_free(wb);

    info("HEALTH thread exiting");
    pthread_exit(NULL);
    return NULL;
}
