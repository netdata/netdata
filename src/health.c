#include "common.h"

// ----------------------------------------------------------------------------
// RRDVAR management

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
        fatal("Request to remove RRDVAR '%s' from index failed. Not Found.", rv->name);

    return ret;
}

static inline RRDVAR *rrdvar_index_find(avl_tree_lock *tree, const char *name, uint32_t hash) {
    RRDVAR tmp;
    tmp.name = (char *)name;
    tmp.hash = (hash)?hash:simple_hash(tmp.name);

    return (RRDVAR *)avl_search_lock(tree, (avl *)&tmp);
}

static inline RRDVAR *rrdvar_create(const char *name, uint32_t hash, int type, calculated_number *value) {
    RRDVAR *rv = callocz(1, sizeof(RRDVAR));

    rv->name = (char *)name;
    rv->hash = (hash)?hash:simple_hash((rv->name));
    rv->type = type;
    rv->value = value;

    return rv;
}

static inline void rrdvar_free(RRDHOST *host, RRDVAR *rv) {
    if(host) {
        // FIXME: we may need some kind of locking here
        EVAL_VARIABLE *rf;
        for (rf = host->references; rf; rf = rf->next)
            if (rf->rrdvar == rv) rf->rrdvar = NULL;
    }

    freez(rv);
}

static inline RRDVAR *rrdvar_create_and_index(const char *scope, avl_tree_lock *tree, const char *name, uint32_t hash, int type, calculated_number *value) {
    RRDVAR *rv = rrdvar_index_find(tree, name, hash);
    if(unlikely(!rv)) {
        debug(D_VARIABLES, "Variable '%s' not found in scope '%s'. Creating a new one.", name, scope);

        rv = rrdvar_create(name, hash, type, value);
        RRDVAR *ret = rrdvar_index_add(tree, rv);
        if(unlikely(ret != rv)) {
            debug(D_VARIABLES, "Variable '%s' in scope '%s' already exists", name, scope);
            rrdvar_free(NULL, rv);
            rv = NULL;
        }
        else
            debug(D_VARIABLES, "Variable '%s' created in scope '%s'", name, scope);
    }
    else {
        // already exists
        rv = NULL;
    }

    /*
     * check
    if(rv) {
        RRDVAR *ret = rrdvar_index_find(tree, name, hash);
        if(ret != rv) fatal("oops! 1");

        ret = rrdvar_index_del(tree, rv);
        if(ret != rv) fatal("oops! 2");

        ret = rrdvar_index_add(tree, rv);
        if(ret != rv) fatal("oops! 3");
    }
    */

    return rv;
}

// ----------------------------------------------------------------------------
// RRDSETVAR management

#define RRDSETVAR_ID_MAX 1024

RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *variable, int type, void *value, uint32_t options) {
    debug(D_VARIABLES, "RRDVARSET create for chart id '%s' name '%s' with variable name '%s'", st->id, st->name, variable);
    RRDSETVAR *rs = (RRDSETVAR *)callocz(1, sizeof(RRDSETVAR));

    char buffer[RRDSETVAR_ID_MAX + 1];
    snprintfz(buffer, RRDSETVAR_ID_MAX, "%s.%s", st->id, variable);
    rs->fullid = strdupz(buffer);
    rs->hash_fullid = simple_hash(rs->fullid);

    snprintfz(buffer, RRDSETVAR_ID_MAX, "%s.%s", st->name, variable);
    rs->fullname = strdupz(buffer);
    rs->hash_fullname = simple_hash(rs->fullname);

    rs->variable = strdupz(variable);
    rs->hash_variable = simple_hash(rs->variable);

    rs->type = type;
    rs->value = value;
    rs->options = options;
    rs->rrdset = st;

    rs->local        = rrdvar_create_and_index("local",   &st->variables_root_index, rs->variable, rs->hash_variable, rs->type, rs->value);
    rs->context      = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->fullid, rs->hash_fullid, rs->type, rs->value);
    rs->host         = rrdvar_create_and_index("host",    &st->rrdhost->variables_root_index, rs->fullid, rs->hash_fullid, rs->type, rs->value);
    rs->context_name = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->fullname, rs->hash_fullname, rs->type, rs->value);
    rs->host_name    = rrdvar_create_and_index("host",    &st->rrdhost->variables_root_index, rs->fullname, rs->hash_fullname, rs->type, rs->value);

    rs->next = st->variables;
    st->variables = rs;

    return rs;
}

void rrdsetvar_rename_all(RRDSET *st) {
    debug(D_VARIABLES, "RRDSETVAR rename for chart id '%s' name '%s'", st->id, st->name);

    // only these 2 can change name
    // rs->context_name
    // rs->host_name

    char buffer[RRDSETVAR_ID_MAX + 1];
    RRDSETVAR *rs, *next = st->variables;
    while((rs = next)) {
        next = rs->next;

        snprintfz(buffer, RRDSETVAR_ID_MAX, "%s.%s", st->name, rs->variable);

        if (strcmp(buffer, rs->fullname)) {
            // name changed
            if (rs->context_name) {
                rrdvar_index_del(&st->rrdcontext->variables_root_index, rs->context_name);
                rrdvar_free(st->rrdhost, rs->context_name);
            }

            if (rs->host_name) {
                rrdvar_index_del(&st->rrdhost->variables_root_index, rs->host_name);
                rrdvar_free(st->rrdhost, rs->host_name);
            }

            freez(rs->fullname);
            rs->fullname = strdupz(st->name);
            rs->hash_fullname = simple_hash(rs->fullname);
            rs->context_name = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->fullname, rs->hash_fullname, rs->type, rs->value);
            rs->host_name    = rrdvar_create_and_index("host",    &st->rrdhost->variables_root_index, rs->fullname, rs->hash_fullname, rs->type, rs->value);
        }
    }

    rrdsetcalc_link_matching(st);
}

void rrdsetvar_free(RRDSETVAR *rs) {
    RRDSET *st = rs->rrdset;
    debug(D_VARIABLES, "RRDSETVAR free for chart id '%s' name '%s', variable '%s'", st->id, st->name, rs->variable);

    if(rs->local) {
        rrdvar_index_del(&st->variables_root_index, rs->local);
        rrdvar_free(st->rrdhost, rs->local);
    }

    if(rs->context) {
        rrdvar_index_del(&st->rrdcontext->variables_root_index, rs->context);
        rrdvar_free(st->rrdhost, rs->context);
    }

    if(rs->host) {
        rrdvar_index_del(&st->rrdhost->variables_root_index, rs->host);
        rrdvar_free(st->rrdhost, rs->host);
    }

    if(rs->context_name) {
        rrdvar_index_del(&st->rrdcontext->variables_root_index, rs->context_name);
        rrdvar_free(st->rrdhost, rs->context_name);
    }

    if(rs->host_name) {
        rrdvar_index_del(&st->rrdhost->variables_root_index, rs->host_name);
        rrdvar_free(st->rrdhost, rs->host_name);
    }

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
    rs->hash = simple_hash(rs->id);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", rs->prefix, rd->name, rs->suffix);
    rs->name = strdupz(buffer);
    rs->hash_name = simple_hash(rs->name);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rd->rrdset->id, rs->id);
    rs->fullidid = strdupz(buffer);
    rs->hash_fullidid = simple_hash(rs->fullidid);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rd->rrdset->id, rs->name);
    rs->fullidname = strdupz(buffer);
    rs->hash_fullidname = simple_hash(rs->fullidname);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rd->rrdset->name, rs->id);
    rs->fullnameid = strdupz(buffer);
    rs->hash_fullnameid = simple_hash(rs->fullnameid);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", rd->rrdset->name, rs->name);
    rs->fullnamename = strdupz(buffer);
    rs->hash_fullnamename = simple_hash(rs->fullnamename);

    rs->type = type;
    rs->value = value;
    rs->options = options;
    rs->rrddim = rd;

    rs->local_id     = rrdvar_create_and_index("local",   &st->variables_root_index, rs->id, rs->hash, rs->type, rs->value);
    rs->local_name   = rrdvar_create_and_index("local",   &st->variables_root_index, rs->name, rs->hash_name, rs->type, rs->value);

    rs->context_fullidid     = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->fullidid, rs->hash_fullidid, rs->type, rs->value);
    rs->context_fullidname   = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->fullidname, rs->hash_fullidname, rs->type, rs->value);
    rs->context_fullnameid   = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->fullnameid, rs->hash_fullnameid, rs->type, rs->value);
    rs->context_fullnamename = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rs->fullnamename, rs->hash_fullnamename, rs->type, rs->value);

    rs->host_fullidid     = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->fullidid, rs->hash_fullidid, rs->type, rs->value);
    rs->host_fullidname   = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->fullidname, rs->hash_fullidname, rs->type, rs->value);
    rs->host_fullnameid   = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->fullnameid, rs->hash_fullnameid, rs->type, rs->value);
    rs->host_fullnamename = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->fullnamename, rs->hash_fullnamename, rs->type, rs->value);

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
            if (rs->local_name) {
                rrdvar_index_del(&st->variables_root_index, rs->local_name);
                rrdvar_free(st->rrdhost, rs->local_name);
            }
            freez(rs->name);
            snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", rs->prefix, rd->name, rs->suffix);
            rs->name = strdupz(buffer);
            rs->hash_name = simple_hash(rs->name);
            rs->local_name = rrdvar_create_and_index("local", &st->variables_root_index, rs->name, rs->hash_name, rs->type, rs->value);

            // fullidname
            if (rs->context_fullidname) {
                rrdvar_index_del(&st->rrdcontext->variables_root_index, rs->context_fullidname);
                rrdvar_free(st->rrdhost, rs->context_fullidname);
            }
            if (rs->host_fullidname) {
                rrdvar_index_del(&st->rrdhost->variables_root_index, rs->context_fullidname);
                rrdvar_free(st->rrdhost, rs->host_fullidname);
            }
            freez(rs->fullidname);
            snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->id, rs->name);
            rs->fullidname = strdupz(buffer);
            rs->hash_fullidname = simple_hash(rs->fullidname);
            rs->context_fullidname = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index,
                                                             rs->fullidname, rs->hash_fullidname, rs->type, rs->value);
            rs->host_fullidname = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index,
                                                             rs->fullidname, rs->hash_fullidname, rs->type, rs->value);

            // fullnameid
            if (rs->context_fullnameid) {
                rrdvar_index_del(&st->rrdcontext->variables_root_index, rs->context_fullnameid);
                rrdvar_free(st->rrdhost, rs->context_fullnameid);
            }
            if (rs->host_fullnameid) {
                rrdvar_index_del(&st->rrdhost->variables_root_index, rs->context_fullnameid);
                rrdvar_free(st->rrdhost, rs->host_fullnameid);
            }
            freez(rs->fullnameid);
            snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->name, rs->id);
            rs->fullnameid = strdupz(buffer);
            rs->hash_fullnameid = simple_hash(rs->fullnameid);
            rs->context_fullnameid = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index,
                                                             rs->fullnameid, rs->hash_fullnameid, rs->type, rs->value);
            rs->host_fullnameid = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index,
                                                          rs->fullnameid, rs->hash_fullnameid, rs->type, rs->value);

            // fullnamename
            if (rs->context_fullnamename) {
                rrdvar_index_del(&st->rrdcontext->variables_root_index, rs->context_fullnamename);
                rrdvar_free(st->rrdhost, rs->context_fullnamename);
            }
            if (rs->host_fullnamename) {
                rrdvar_index_del(&st->rrdhost->variables_root_index, rs->context_fullnamename);
                rrdvar_free(st->rrdhost, rs->host_fullnamename);
            }
            freez(rs->fullnamename);
            snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->name, rs->name);
            rs->fullnamename = strdupz(buffer);
            rs->hash_fullnamename = simple_hash(rs->fullnamename);
            rs->context_fullnamename = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index,
                                                             rs->fullnamename, rs->hash_fullnamename, rs->type, rs->value);
            rs->host_fullnamename = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index,
                                                          rs->fullnamename, rs->hash_fullnamename, rs->type, rs->value);
        }
    }
}

void rrddimvar_free(RRDDIMVAR *rs) {
    RRDDIM *rd = rs->rrddim;
    RRDSET *st = rd->rrdset;
    debug(D_VARIABLES, "RRDDIMSET free for chart id '%s' name '%s', dimension id '%s', name '%s', prefix='%s', suffix='%s'", st->id, st->name, rd->id, rd->name, rs->prefix, rs->suffix);

    if(rs->local_id) {
        rrdvar_index_del(&st->variables_root_index, rs->local_id);
        rrdvar_free(st->rrdhost, rs->local_id);
    }
    if(rs->local_name) {
        rrdvar_index_del(&st->variables_root_index, rs->local_name);
        rrdvar_free(st->rrdhost, rs->local_name);
    }

    if(rs->context_fullidid) {
        rrdvar_index_del(&st->rrdcontext->variables_root_index, rs->context_fullidid);
        rrdvar_free(st->rrdhost, rs->context_fullidid);
    }
    if(rs->context_fullidname) {
        rrdvar_index_del(&st->rrdcontext->variables_root_index, rs->context_fullidname);
        rrdvar_free(st->rrdhost, rs->context_fullidname);
    }
    if(rs->context_fullnameid) {
        rrdvar_index_del(&st->rrdcontext->variables_root_index, rs->context_fullnameid);
        rrdvar_free(st->rrdhost, rs->context_fullnameid);
    }
    if(rs->context_fullnamename) {
        rrdvar_index_del(&st->rrdcontext->variables_root_index, rs->context_fullnamename);
        rrdvar_free(st->rrdhost, rs->context_fullnamename);
    }

    if(rs->host_fullidid) {
        rrdvar_index_del(&st->rrdhost->variables_root_index, rs->host_fullidid);
        rrdvar_free(st->rrdhost, rs->host_fullidid);
    }
    if(rs->host_fullidname) {
        rrdvar_index_del(&st->rrdhost->variables_root_index, rs->host_fullidname);
        rrdvar_free(st->rrdhost, rs->host_fullidname);
    }
    if(rs->host_fullnameid) {
        rrdvar_index_del(&st->rrdhost->variables_root_index, rs->host_fullnameid);
        rrdvar_free(st->rrdhost, rs->host_fullnameid);
    }
    if(rs->host_fullnamename) {
        rrdvar_index_del(&st->rrdhost->variables_root_index, rs->host_fullnamename);
        rrdvar_free(st->rrdhost, rs->host_fullnamename);
    }

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

// this has to be called while the caller has locked
// the RRDHOST
static inline void rrdset_linked_optimize_rrdhost(RRDHOST *host, RRDCALC *rc) {
    rrdhost_check_wrlock(host);

    // move it to be last

    if(!rc->next)
        // we are last already
        return;

    RRDCALC *t, *last = NULL, *prev = NULL;
    for (t = host->calculations; t ; t = t->next) {
        if(t->next == rc)
            prev = t;

        if(!t->next)
            last = t;
    }

    if(!last) {
        error("RRDCALC '%s' cannot be linked to the end of host '%s' list", rc->name, host->hostname);
        return;
    }

    if(prev)
        prev->next = rc->next;
    else {
        if(host->calculations == rc)
            host->calculations = rc->next;
        else {
            error("RRDCALC '%s' is not found in host '%s' list", rc->name, host->hostname);
            return;
        }
    }

    last->next = rc;
    rc->next = NULL;
}

// this has to be called while the caller has locked
// the RRDHOST
static inline void rrdcalc_unlinked_optimize_rrdhost(RRDHOST *host, RRDCALC *rc) {
    rrdhost_check_wrlock(host);
    
    // move it to be first

    if(host->calculations == rc) {
        // ok, we are the first
        return;
    }
    else {
        // find the previous one
        RRDCALC *t;
        for (t = host->calculations; t && t->next != rc; rc = rc->next) ;
        if(unlikely(!t)) {
            error("RRDCALC '%s' is not linked to host '%s'.", rc->name, host->hostname);
            return;
        }
        t->next = rc->next;
        rc->next = host->calculations;
        host->calculations = rc;
    }
}

static void rrdsetcalc_link(RRDSET *st, RRDCALC *rc) {
    rc->rrdset = st;

    rc->local   = rrdvar_create_and_index("local", &st->variables_root_index, rc->name, rc->hash, RRDVAR_TYPE_CALCULATED, &rc->value);
    rc->context = rrdvar_create_and_index("context", &st->rrdcontext->variables_root_index, rc->name, rc->hash, RRDVAR_TYPE_CALCULATED, &rc->value);
    rc->host    = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rc->name, rc->hash, RRDVAR_TYPE_CALCULATED, &rc->value);

    rrdset_linked_optimize_rrdhost(st->rrdhost, rc);
}

static inline int rrdcalc_is_matching_this_rrdset(RRDCALC *rc, RRDSET *st) {
    if((rc->hash_chart == st->hash && !strcmp(rc->name, st->id)) ||
            (rc->hash_chart == st->hash_name && !strcmp(rc->name, st->name)))
        return 1;

    return 0;
}

// this has to be called while the RRDHOST is locked
void rrdsetcalc_link_matching(RRDSET *st) {
    RRDCALC *rc;

    for(rc = st->rrdhost->calculations; rc ; rc = rc->next) {
        // since unlinked ones are in front and linked at the end
        // we stop on the first linked RRDCALC
        if(rc->rrdset != NULL) break;

        if(rrdcalc_is_matching_this_rrdset(rc, st))
            rrdsetcalc_link(st, rc);
    }
}

// this has to be called while the RRDHOST is locked
void rrdsetcalc_unlink(RRDCALC *rc) {
    RRDSET *st = rc->rrdset;

    if(!st) {
        error("Requested to unlink RRDCALC '%s' which is not linked to any RRDSET", rc->name);
        return;
    }

    RRDHOST *host = st->rrdhost;

    // unlink it
    if(rc->rrdset_prev)
        rc->rrdset_prev->rrdset_next = rc->rrdset_next;

    if(rc->rrdset_next)
        rc->rrdset_next->rrdset_prev = rc->rrdset_prev;

    if(st->calculations == rc)
        st->calculations = rc->rrdset_next;

    rc->rrdset_prev = rc->rrdset_next = NULL;

    if(rc->local) {
        rrdvar_index_del(&st->variables_root_index, rc->local);
        rrdvar_free(st->rrdhost, rc->local);
        rc->local = NULL;
    }

    if(rc->context) {
        rrdvar_index_del(&st->rrdcontext->variables_root_index, rc->context);
        rc->context = NULL;
    }

    if(rc->host) {
        rrdvar_index_del(&st->rrdhost->variables_root_index, rc->host);
        rc->host = NULL;
    }

    rc->rrdset = NULL;

    // RRDCALC will remain in RRDHOST
    // so that if the matching chart is found in the future
    // it will be applied automatically

    rrdcalc_unlinked_optimize_rrdhost(host, rc);
}

RRDCALC *rrdcalc_create(RRDHOST *host, const char *name, const char *chart, const char *dimensions, int group_method, uint32_t after, uint32_t before, int update_every, uint32_t options) {
    uint32_t hash = simple_hash(name);

    RRDCALC *rc;

    // make sure it does not already exist
    for(rc = host->calculations; rc ; rc = rc->next) {
        if (rc->hash == hash && !strcmp(name, rc->name)) {
            error("Attempted to create RRDCAL '%s' in host '%s', but it already exists. Ignoring it.", name, host->hostname);
            return NULL;
        }
    }

    rc = callocz(1, sizeof(RRDCALC));

    rc->name = strdupz(name);
    rc->hash = simple_hash(rc->name);

    rc->chart = strdupz(chart);
    rc->hash_chart = simple_hash(rc->chart);

    if(dimensions) {
        rc->dimensions = strdupz(dimensions);
        rc->hash_chart = simple_hash(rc->chart);
    }

    rc->group = group_method;
    rc->after = after;
    rc->before = before;
    rc->update_every = update_every;
    rc->options = options;

    // link it to the host
    rc->next = host->calculations;
    host->calculations = rc;

    // link it to its chart
    RRDSET *st;
    for(st = host->rrdset_root; st ; st = st->next) {
        if(rrdcalc_is_matching_this_rrdset(rc, st)) {
            rrdsetcalc_link(st, rc);
            break;
        }
    }

    return NULL;
}

void rrdcalc_free(RRDHOST *host, RRDCALC *rc) {
    if(!rc) return;

    // unlink it from RRDSET
    if(rc->rrdset) rrdsetcalc_unlink(rc);

    // unlink it from RRDHOST
    if(rc == host->calculations)
        host->calculations = rc->next;

    else if(host->calculations) {
        RRDCALC *t, *last = host->calculations;

        for(t = last->next; t ; last = t, t = t->next)
            if(t == rc) break;
        
        if(last)
            last->next = rc->next;
        else
            error("Cannot unlink RRDCALC '%s' from RRDHOST '%s': not found", rc->name, host->hostname);
    }
    else
        error("Cannot unlink RRDCALC '%s' from RRDHOST '%s': RRDHOST does not have any calculations", rc->name, host->hostname);

    freez(rc->name);
    freez(rc->chart);
    freez(rc->dimensions);

    freez(rc);
}
