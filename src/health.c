#define NETDATA_HEALTH_INTERNALS
#include "common.h"

#define RRDVAR_MAX_LENGTH 1024

int default_localhost_health_enabled = 1;

// ----------------------------------------------------------------------------
// RRDVAR management

inline int rrdvar_fix_name(char *variable) {
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

    if(tree) {
        debug(D_VARIABLES, "Deleting variable '%s'", rv->name);
        if(unlikely(!rrdvar_index_del(tree, rv)))
            error("Attempted to delete variable '%s' from host '%s', but it is not found.", rv->name, host->hostname);
    }

    freez(rv->name);
    freez(rv);
}

static inline RRDVAR *rrdvar_create_and_index(const char *scope, avl_tree_lock *tree, const char *name, int type, void *value) {
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
        debug(D_VARIABLES, "Variable '%s' is already found in scope '%s'.", variable, scope);

        // already exists
        freez(variable);

        // this is important
        // it must return NULL - not the existing variable - or double-free will happen
        rv = NULL;
    }

    return rv;
}

// ----------------------------------------------------------------------------
// CUSTOM VARIABLES

RRDVAR *rrdvar_custom_host_variable_create(RRDHOST *host, const char *name) {
    calculated_number *v = callocz(1, sizeof(calculated_number));
    *v = NAN;
    RRDVAR *rv = rrdvar_create_and_index("host", &host->variables_root_index, name, RRDVAR_TYPE_CALCULATED_ALLOCATED, v);
    if(unlikely(!rv)) {
        free(v);
        error("Requested variable '%s' already exists - possibly 2 plugins will be updating it at the same time", name);

        char *variable = strdupz(name);
        rrdvar_fix_name(variable);
        uint32_t hash = simple_hash(variable);

        rv = rrdvar_index_find(&host->variables_root_index, variable, hash);
    }

    return rv;
}

void rrdvar_custom_host_variable_destroy(RRDHOST *host, const char *name) {
    char *variable = strdupz(name);
    rrdvar_fix_name(variable);
    uint32_t hash = simple_hash(variable);

    RRDVAR *rv = rrdvar_index_find(&host->variables_root_index, variable, hash);
    freez(variable);

    if(!rv) {
        error("Attempted to remove variable '%s' from host '%s', but it does not exist.", name, host->hostname);
        return;
    }

    if(rv->type != RRDVAR_TYPE_CALCULATED_ALLOCATED) {
        error("Attempted to remove variable '%s' from host '%s', but it does not a custom allocated variable.", name, host->hostname);
        return;
    }

    if(!rrdvar_index_del(&host->variables_root_index, rv)) {
        error("Attempted to remove variable '%s' from host '%s', but it cannot be found.", name, host->hostname);
        return;
    }

    freez(rv->name);
    freez(rv->value);
    freez(rv);
}

void rrdvar_custom_host_variable_set(RRDVAR *rv, calculated_number value) {
    if(rv->type != RRDVAR_TYPE_CALCULATED_ALLOCATED)
        error("requested to set variable '%s' to value " CALCULATED_NUMBER_FORMAT " but the variable is not a custom one.", rv->name, value);
    else {
        calculated_number *v = rv->value;
        *v = value;
    }
}

// ----------------------------------------------------------------------------
// RRDVAR lookup

static calculated_number rrdvar2number(RRDVAR *rv) {
    switch(rv->type) {
        case RRDVAR_TYPE_CALCULATED_ALLOCATED:
        case RRDVAR_TYPE_CALCULATED: {
            calculated_number *n = (calculated_number *)rv->value;
            return *n;
        }

        case RRDVAR_TYPE_TIME_T: {
            time_t *n = (time_t *)rv->value;
            return *n;
        }

        case RRDVAR_TYPE_COLLECTED: {
            collected_number *n = (collected_number *)rv->value;
            return *n;
        }

        case RRDVAR_TYPE_TOTAL: {
            total_number *n = (total_number *)rv->value;
            return *n;
        }

        case RRDVAR_TYPE_INT: {
            int *n = (int *)rv->value;
            return *n;
        }

        default:
            error("I don't know how to convert RRDVAR type %d to calculated_number", rv->type);
            return NAN;
    }
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

    rv = rrdvar_index_find(&st->rrdfamily->variables_root_index, variable, hash);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    rv = rrdvar_index_find(&st->rrdhost->variables_root_index, variable, hash);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    return 0;
}

// ----------------------------------------------------------------------------
// RRDVAR to JSON

struct variable2json_helper {
    BUFFER *buf;
    size_t counter;
};

static int single_variable2json(void *entry, void *data) {
    struct variable2json_helper *helper = (struct variable2json_helper *)data;
    RRDVAR *rv = (RRDVAR *)entry;
    calculated_number value = rrdvar2number(rv);

    if(unlikely(isnan(value) || isinf(value)))
        buffer_sprintf(helper->buf, "%s\n\t\t\"%s\": null", helper->counter?",":"", rv->name);
    else
        buffer_sprintf(helper->buf, "%s\n\t\t\"%s\": %0.5Lf", helper->counter?",":"", rv->name, (long double)value);

    helper->counter++;

    return 0;
}

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *buf) {
    struct variable2json_helper helper = {
            .buf = buf,
            .counter = 0
    };

    buffer_sprintf(buf, "{\n\t\"chart\": \"%s\",\n\t\"chart_name\": \"%s\",\n\t\"chart_context\": \"%s\",\n\t\"chart_variables\": {", st->id, st->name, st->context);
    avl_traverse_lock(&st->variables_root_index, single_variable2json, (void *)&helper);
    buffer_sprintf(buf, "\n\t},\n\t\"family\": \"%s\",\n\t\"family_variables\": {", st->family);
    helper.counter = 0;
    avl_traverse_lock(&st->rrdfamily->variables_root_index, single_variable2json, (void *)&helper);
    buffer_sprintf(buf, "\n\t},\n\t\"host\": \"%s\",\n\t\"host_variables\": {", st->rrdhost->hostname);
    helper.counter = 0;
    avl_traverse_lock(&st->rrdhost->variables_root_index, single_variable2json, (void *)&helper);
    buffer_strcat(buf, "\n\t}\n}\n");
}


// ----------------------------------------------------------------------------
// RRDDIMVAR management
// DIMENSION VARIABLES

#define RRDDIMVAR_ID_MAX 1024

static inline void rrddimvar_free_variables(RRDDIMVAR *rs) {
    RRDDIM *rd = rs->rrddim;
    RRDSET *st = rd->rrdset;

    // CHART VARIABLES FOR THIS DIMENSION

    rrdvar_free(st->rrdhost, &st->variables_root_index, rs->var_local_id);
    rs->var_local_id = NULL;

    rrdvar_free(st->rrdhost, &st->variables_root_index, rs->var_local_name);
    rs->var_local_name = NULL;

    // FAMILY VARIABLES FOR THIS DIMENSION

    rrdvar_free(st->rrdhost, &st->rrdfamily->variables_root_index, rs->var_family_id);
    rs->var_family_id = NULL;

    rrdvar_free(st->rrdhost, &st->rrdfamily->variables_root_index, rs->var_family_name);
    rs->var_family_name = NULL;

    rrdvar_free(st->rrdhost, &st->rrdfamily->variables_root_index, rs->var_family_contextid);
    rs->var_family_contextid = NULL;

    rrdvar_free(st->rrdhost, &st->rrdfamily->variables_root_index, rs->var_family_contextname);
    rs->var_family_contextname = NULL;

    // HOST VARIABLES FOR THIS DIMENSION

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->var_host_chartidid);
    rs->var_host_chartidid = NULL;

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->var_host_chartidname);
    rs->var_host_chartidname = NULL;

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->var_host_chartnameid);
    rs->var_host_chartnameid = NULL;

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rs->var_host_chartnamename);
    rs->var_host_chartnamename = NULL;

    // KEYS

    freez(rs->key_id);
    rs->key_id = NULL;

    freez(rs->key_name);
    rs->key_name = NULL;

    freez(rs->key_fullidid);
    rs->key_fullidid = NULL;

    freez(rs->key_fullidname);
    rs->key_fullidname = NULL;

    freez(rs->key_contextid);
    rs->key_contextid = NULL;

    freez(rs->key_contextname);
    rs->key_contextname = NULL;

    freez(rs->key_fullnameid);
    rs->key_fullnameid = NULL;

    freez(rs->key_fullnamename);
    rs->key_fullnamename = NULL;
}

static inline void rrddimvar_create_variables(RRDDIMVAR *rs) {
    rrddimvar_free_variables(rs);

    RRDDIM *rd = rs->rrddim;
    RRDSET *st = rd->rrdset;

    char buffer[RRDDIMVAR_ID_MAX + 1];

    // KEYS

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", rs->prefix, rd->id, rs->suffix);
    rs->key_id = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s%s%s", rs->prefix, rd->name, rs->suffix);
    rs->key_name = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->id, rs->key_id);
    rs->key_fullidid = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->id, rs->key_name);
    rs->key_fullidname = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->context, rs->key_id);
    rs->key_contextid = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->context, rs->key_name);
    rs->key_contextname = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->name, rs->key_id);
    rs->key_fullnameid = strdupz(buffer);

    snprintfz(buffer, RRDDIMVAR_ID_MAX, "%s.%s", st->name, rs->key_name);
    rs->key_fullnamename = strdupz(buffer);

    // CHART VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $id
    // - $name

    rs->var_local_id           = rrdvar_create_and_index("local", &st->variables_root_index, rs->key_id, rs->type, rs->value);
    rs->var_local_name         = rrdvar_create_and_index("local", &st->variables_root_index, rs->key_name, rs->type, rs->value);

    // FAMILY VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $id                 (only the first, when multiple overlap)
    // - $name               (only the first, when multiple overlap)
    // - $chart-context.id
    // - $chart-context.name

    rs->var_family_id          = rrdvar_create_and_index("family", &st->rrdfamily->variables_root_index, rs->key_id, rs->type, rs->value);
    rs->var_family_name        = rrdvar_create_and_index("family", &st->rrdfamily->variables_root_index, rs->key_name, rs->type, rs->value);
    rs->var_family_contextid   = rrdvar_create_and_index("family", &st->rrdfamily->variables_root_index, rs->key_contextid, rs->type, rs->value);
    rs->var_family_contextname = rrdvar_create_and_index("family", &st->rrdfamily->variables_root_index, rs->key_contextname, rs->type, rs->value);

    // HOST VARIABLES FOR THIS DIMENSION
    // -----------------------------------
    //
    // dimensions are available as:
    // - $chart-id.id
    // - $chart-id.name
    // - $chart-name.id
    // - $chart-name.name

    rs->var_host_chartidid      = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->key_fullidid, rs->type, rs->value);
    rs->var_host_chartidname    = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->key_fullidname, rs->type, rs->value);
    rs->var_host_chartnameid    = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->key_fullnameid, rs->type, rs->value);
    rs->var_host_chartnamename  = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, rs->key_fullnamename, rs->type, rs->value);
}

RRDDIMVAR *rrddimvar_create(RRDDIM *rd, int type, const char *prefix, const char *suffix, void *value, uint32_t options) {
    RRDSET *st = rd->rrdset;

    debug(D_VARIABLES, "RRDDIMSET create for chart id '%s' name '%s', dimension id '%s', name '%s%s%s'", st->id, st->name, rd->id, (prefix)?prefix:"", rd->name, (suffix)?suffix:"");

    if(!prefix) prefix = "";
    if(!suffix) suffix = "";

    RRDDIMVAR *rs = (RRDDIMVAR *)callocz(1, sizeof(RRDDIMVAR));

    rs->prefix = strdupz(prefix);
    rs->suffix = strdupz(suffix);

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
    debug(D_VARIABLES, "RRDDIMSET rename for chart id '%s' name '%s', dimension id '%s', name '%s'", st->id, st->name, rd->id, rd->name);

    RRDDIMVAR *rs, *next = rd->variables;
    while((rs = next)) {
        next = rs->next;
        rrddimvar_create_variables(rs);
    }
}

void rrddimvar_free(RRDDIMVAR *rs) {
    RRDDIM *rd = rs->rrddim;
    RRDSET *st = rd->rrdset;
    debug(D_VARIABLES, "RRDDIMSET free for chart id '%s' name '%s', dimension id '%s', name '%s', prefix='%s', suffix='%s'", st->id, st->name, rd->id, rd->name, rs->prefix, rs->suffix);

    rrddimvar_free_variables(rs);

    if(rd->variables == rs) {
        debug(D_VARIABLES, "RRDDIMSET removing first entry for chart id '%s' name '%s', dimension id '%s', name '%s'", st->id, st->name, rd->id, rd->name);
        rd->variables = rs->next;
    }
    else {
        debug(D_VARIABLES, "RRDDIMSET removing non-first entry for chart id '%s' name '%s', dimension id '%s', name '%s'", st->id, st->name, rd->id, rd->name);
        RRDDIMVAR *t;
        for (t = rd->variables; t && t->next != rs; t = t->next) ;
        if(!t) error("RRDDIMVAR '%s' not found in dimension '%s/%s' variables linked list", rs->key_name, st->id, rd->id);
        else t->next = rs->next;
    }

    freez(rs->prefix);
    freez(rs->suffix);
    freez(rs);
}

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

RRDSETVAR *rrdsetvar_create(RRDSET *st, const char *variable, int type, void *value, uint32_t options) {
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

// ----------------------------------------------------------------------------
// RRDCALC management

inline const char *rrdcalc_status2string(int status) {
    switch(status) {
        case RRDCALC_STATUS_REMOVED:
            return "REMOVED";

        case RRDCALC_STATUS_UNDEFINED:
            return "UNDEFINED";

        case RRDCALC_STATUS_UNINITIALIZED:
            return "UNINITIALIZED";

        case RRDCALC_STATUS_CLEAR:
            return "CLEAR";

        case RRDCALC_STATUS_RAISED:
            return "RAISED";

        case RRDCALC_STATUS_WARNING:
            return "WARNING";

        case RRDCALC_STATUS_CRITICAL:
            return "CRITICAL";

        default:
            error("Unknown alarm status %d", status);
            return "UNKNOWN";
    }
}

static void rrdsetcalc_link(RRDSET *st, RRDCALC *rc) {
    debug(D_HEALTH, "Health linking alarm '%s.%s' to chart '%s' of host '%s'", rc->chart?rc->chart:"NOCHART", rc->name, st->id, st->rrdhost->hostname);

    rc->last_status_change = now_realtime_sec();
    rc->rrdset = st;

    rc->rrdset_next = st->alarms;
    rc->rrdset_prev = NULL;
    
    if(rc->rrdset_next)
        rc->rrdset_next->rrdset_prev = rc;

    st->alarms = rc;

    if(rc->update_every < rc->rrdset->update_every) {
        error("Health alarm '%s.%s' has update every %d, less than chart update every %d. Setting alarm update frequency to %d.", rc->rrdset->id, rc->name, rc->update_every, rc->rrdset->update_every, rc->rrdset->update_every);
        rc->update_every = rc->rrdset->update_every;
    }

    if(!isnan(rc->green) && isnan(st->green)) {
        debug(D_HEALTH, "Health alarm '%s.%s' green threshold set from %Lf to %Lf.", rc->rrdset->id, rc->name, rc->rrdset->green, rc->green);
        st->green = rc->green;
    }

    if(!isnan(rc->red) && isnan(st->red)) {
        debug(D_HEALTH, "Health alarm '%s.%s' red threshold set from %Lf to %Lf.", rc->rrdset->id, rc->name, rc->rrdset->red, rc->red);
        st->red = rc->red;
    }

    rc->local  = rrdvar_create_and_index("local",  &st->variables_root_index, rc->name, RRDVAR_TYPE_CALCULATED, &rc->value);
    rc->family = rrdvar_create_and_index("family", &st->rrdfamily->variables_root_index, rc->name, RRDVAR_TYPE_CALCULATED, &rc->value);

    char fullname[RRDVAR_MAX_LENGTH + 1];
    snprintfz(fullname, RRDVAR_MAX_LENGTH, "%s.%s", st->id, rc->name);
    rc->hostid   = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, fullname, RRDVAR_TYPE_CALCULATED, &rc->value);

    snprintfz(fullname, RRDVAR_MAX_LENGTH, "%s.%s", st->name, rc->name);
    rc->hostname = rrdvar_create_and_index("host", &st->rrdhost->variables_root_index, fullname, RRDVAR_TYPE_CALCULATED, &rc->value);

	if(!rc->units) rc->units = strdupz(st->units);

    {
        time_t now = now_realtime_sec();
        health_alarm_log(
                st->rrdhost,
                rc->id,
                rc->next_event_id++,
                now,
                rc->name,
                rc->rrdset->id,
                rc->rrdset->family,
                rc->exec,
                rc->recipient,
                now - rc->last_status_change,
                rc->old_value,
                rc->value,
                rc->status,
                RRDCALC_STATUS_UNINITIALIZED,
                rc->source,
                rc->units,
                rc->info,
                0,
                0
        );
    }
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
        if(unlikely(rc->rrdset))
            continue;

        if(unlikely(rrdcalc_is_matching_this_rrdset(rc, st)))
            rrdsetcalc_link(st, rc);
    }
}

// this has to be called while the RRDHOST is locked
inline void rrdsetcalc_unlink(RRDCALC *rc) {
    RRDSET *st = rc->rrdset;

    if(!st) {
        debug(D_HEALTH, "Requested to unlink RRDCALC '%s.%s' which is not linked to any RRDSET", rc->chart?rc->chart:"NOCHART", rc->name);
        error("Requested to unlink RRDCALC '%s.%s' which is not linked to any RRDSET", rc->chart?rc->chart:"NOCHART", rc->name);
        return;
    }

    {
        time_t now = now_realtime_sec();
        health_alarm_log(
                st->rrdhost,
                rc->id,
                rc->next_event_id++,
                now,
                rc->name,
                rc->rrdset->id,
                rc->rrdset->family,
                rc->exec,
                rc->recipient,
                now - rc->last_status_change,
                rc->old_value,
                rc->value,
                rc->status,
                RRDCALC_STATUS_REMOVED,
                rc->source,
                rc->units,
                rc->info,
                0,
                0
        );
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

    rrdvar_free(st->rrdhost, &st->rrdfamily->variables_root_index, rc->family);
    rc->family = NULL;

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rc->hostid);
    rc->hostid = NULL;

    rrdvar_free(st->rrdhost, &st->rrdhost->variables_root_index, rc->hostname);
    rc->hostname = NULL;

    rc->rrdset = NULL;

    // RRDCALC will remain in RRDHOST
    // so that if the matching chart is found in the future
    // it will be applied automatically
}

RRDCALC *rrdcalc_find(RRDSET *st, const char *name) {
    RRDCALC *rc;
    uint32_t hash = simple_hash(name);

    for( rc = st->alarms; rc ; rc = rc->rrdset_next ) {
        if(unlikely(rc->hash == hash && !strcmp(rc->name, name)))
            return rc;
    }

    return NULL;
}

inline int rrdcalc_exists(RRDHOST *host, const char *chart, const char *name, uint32_t hash_chart, uint32_t hash_name) {
    RRDCALC *rc;

    if(unlikely(!chart)) {
        error("attempt to find RRDCALC '%s' without giving a chart name", name);
        return 1;
    }

    if(unlikely(!hash_chart)) hash_chart = simple_hash(chart);
    if(unlikely(!hash_name))  hash_name  = simple_hash(name);

    // make sure it does not already exist
    for(rc = host->alarms; rc ; rc = rc->next) {
        if (unlikely(rc->chart && rc->hash == hash_name && rc->hash_chart == hash_chart && !strcmp(name, rc->name) && !strcmp(chart, rc->chart))) {
            debug(D_HEALTH, "Health alarm '%s.%s' already exists in host '%s'.", chart, name, host->hostname);
            error("Health alarm '%s.%s' already exists in host '%s'.", chart, name, host->hostname);
            return 1;
        }
    }

    return 0;
}

inline uint32_t rrdcalc_get_unique_id(RRDHOST *host, const char *chart, const char *name, uint32_t *next_event_id) {
    if(chart && name) {
        uint32_t hash_chart = simple_hash(chart);
        uint32_t hash_name = simple_hash(name);

        // re-use old IDs, by looking them up in the alarm log
        ALARM_ENTRY *ae;
        for(ae = host->health_log.alarms; ae ;ae = ae->next) {
            if(unlikely(ae->hash_name == hash_name && ae->hash_chart == hash_chart && !strcmp(name, ae->name) && !strcmp(chart, ae->chart))) {
                if(next_event_id) *next_event_id = ae->alarm_event_id + 1;
                return ae->alarm_id;
            }
        }
    }

    return host->health_log.next_alarm_id++;
}

inline void rrdcalc_create_part2(RRDHOST *host, RRDCALC *rc) {
    rrdhost_check_rdlock(host);

    if(rc->calculation) {
        rc->calculation->status = &rc->status;
        rc->calculation->this = &rc->value;
        rc->calculation->after = &rc->db_after;
        rc->calculation->before = &rc->db_before;
        rc->calculation->rrdcalc = rc;
    }

    if(rc->warning) {
        rc->warning->status = &rc->status;
        rc->warning->this = &rc->value;
        rc->warning->after = &rc->db_after;
        rc->warning->before = &rc->db_before;
        rc->warning->rrdcalc = rc;
    }

    if(rc->critical) {
        rc->critical->status = &rc->status;
        rc->critical->this = &rc->value;
        rc->critical->after = &rc->db_after;
        rc->critical->before = &rc->db_before;
        rc->critical->rrdcalc = rc;
    }

    // link it to the host
    if(likely(host->alarms)) {
        // append it
        RRDCALC *t;
        for(t = host->alarms; t && t->next ; t = t->next) ;
        t->next = rc;
    }
    else {
        host->alarms = rc;
    }

    // link it to its chart
    RRDSET *st;
    for(st = host->rrdset_root; st ; st = st->next) {
        if(rrdcalc_is_matching_this_rrdset(rc, st)) {
            rrdsetcalc_link(st, rc);
            break;
        }
    }
}

static inline RRDCALC *rrdcalc_create(RRDHOST *host, RRDCALCTEMPLATE *rt, const char *chart) {

    debug(D_HEALTH, "Health creating dynamic alarm (from template) '%s.%s'", chart, rt->name);

    if(rrdcalc_exists(host, chart, rt->name, 0, 0))
        return NULL;

    RRDCALC *rc = callocz(1, sizeof(RRDCALC));
    rc->next_event_id = 1;
    rc->id = rrdcalc_get_unique_id(host, chart, rt->name, &rc->next_event_id);
    rc->name = strdupz(rt->name);
    rc->hash = simple_hash(rc->name);
    rc->chart = strdupz(chart);
    rc->hash_chart = simple_hash(rc->chart);

    if(rt->dimensions) rc->dimensions = strdupz(rt->dimensions);

    rc->green = rt->green;
    rc->red = rt->red;
    rc->value = NAN;
    rc->old_value = NAN;

    rc->delay_up_duration = rt->delay_up_duration;
    rc->delay_down_duration = rt->delay_down_duration;
    rc->delay_max_duration = rt->delay_max_duration;
    rc->delay_multiplier = rt->delay_multiplier;

    rc->group = rt->group;
    rc->after = rt->after;
    rc->before = rt->before;
    rc->update_every = rt->update_every;
    rc->options = rt->options;

    if(rt->exec) rc->exec = strdupz(rt->exec);
    if(rt->recipient) rc->recipient = strdupz(rt->recipient);
    if(rt->source) rc->source = strdupz(rt->source);
    if(rt->units) rc->units = strdupz(rt->units);
    if(rt->info) rc->info = strdupz(rt->info);

    if(rt->calculation) {
        rc->calculation = expression_parse(rt->calculation->source, NULL, NULL);
        if(!rc->calculation)
            error("Health alarm '%s.%s': failed to parse calculation expression '%s'", chart, rt->name, rt->calculation->source);
    }
    if(rt->warning) {
        rc->warning = expression_parse(rt->warning->source, NULL, NULL);
        if(!rc->warning)
            error("Health alarm '%s.%s': failed to re-parse warning expression '%s'", chart, rt->name, rt->warning->source);
    }
    if(rt->critical) {
        rc->critical = expression_parse(rt->critical->source, NULL, NULL);
        if(!rc->critical)
            error("Health alarm '%s.%s': failed to re-parse critical expression '%s'", chart, rt->name, rt->critical->source);
    }

    debug(D_HEALTH, "Health runtime added alarm '%s.%s': exec '%s', recipient '%s', green %Lf, red %Lf, lookup: group %d, after %d, before %d, options %u, dimensions '%s', update every %d, calculation '%s', warning '%s', critical '%s', source '%s', delay up %d, delay down %d, delay max %d, delay_multiplier %f",
          (rc->chart)?rc->chart:"NOCHART",
          rc->name,
          (rc->exec)?rc->exec:"DEFAULT",
          (rc->recipient)?rc->recipient:"DEFAULT",
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
          rc->source,
          rc->delay_up_duration,
          rc->delay_down_duration,
          rc->delay_max_duration,
          rc->delay_multiplier
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
    if(unlikely(rc == host->alarms))
        host->alarms = rc->next;

    else if(likely(host->alarms)) {
        RRDCALC *t, *last = host->alarms;
        for(t = last->next; t && t != rc; last = t, t = t->next) ;
        if(last->next == rc)
            last->next = rc->next;
        else
            error("Cannot unlink alarm '%s.%s' from host '%s': not found", rc->chart?rc->chart:"NOCHART", rc->name, host->hostname);
    }
    else
        error("Cannot unlink unlink '%s.%s' from host '%s': This host does not have any calculations", rc->chart?rc->chart:"NOCHART", rc->name, host->hostname);

    expression_free(rc->calculation);
    expression_free(rc->warning);
    expression_free(rc->critical);

    freez(rc->name);
    freez(rc->chart);
    freez(rc->family);
    freez(rc->dimensions);
    freez(rc->exec);
    freez(rc->recipient);
    freez(rc->source);
    freez(rc->units);
    freez(rc->info);
    freez(rc);
}

// ----------------------------------------------------------------------------
// RRDCALCTEMPLATE management

void rrdcalctemplate_link_matching(RRDSET *st) {
    RRDCALCTEMPLATE *rt;

    for(rt = st->rrdhost->templates; rt ; rt = rt->next) {
        if(rt->hash_context == st->hash_context && !strcmp(rt->context, st->context)
                && (!rt->family_pattern || simple_pattern_matches(rt->family_pattern, st->family))) {
            RRDCALC *rc = rrdcalc_create(st->rrdhost, rt, st->id);
            if(unlikely(!rc))
                error("Health tried to create alarm from template '%s', but it failed", rt->name);

#ifdef NETDATA_INTERNAL_CHECKS
            else if(rc->rrdset != st)
                error("Health alarm '%s.%s' should be linked to chart '%s', but it is not", rc->chart?rc->chart:"NOCHART", rc->name, st->id);
#endif
        }
    }
}

inline void rrdcalctemplate_free(RRDHOST *host, RRDCALCTEMPLATE *rt) {
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

    freez(rt->family_match);
    simple_pattern_free(rt->family_pattern);

    freez(rt->name);
    freez(rt->exec);
    freez(rt->recipient);
    freez(rt->context);
    freez(rt->source);
    freez(rt->units);
    freez(rt->info);
    freez(rt->dimensions);
    freez(rt);
}

// ----------------------------------------------------------------------------
// health initialization

inline char *health_config_dir(void) {
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "%s/health.d", netdata_configured_config_dir);
    return config_get("health", "health configuration directory", buffer);
}

void health_init(void) {
    debug(D_HEALTH, "Health configuration initializing");

    if(!(default_localhost_health_enabled = config_get_boolean("health", "enabled", 1))) {
        debug(D_HEALTH, "Health is disabled.");
        return;
    }

    char pathname[FILENAME_MAX + 1];
    snprintfz(pathname, FILENAME_MAX, "%s/health", netdata_configured_varlib_dir);
    if(mkdir(pathname, 0770) == -1 && errno != EEXIST)
        fatal("Cannot create directory '%s'.", pathname);
}

// ----------------------------------------------------------------------------
// re-load health configuration

inline void health_free_host_nolock(RRDHOST *host) {
    while(host->templates)
        rrdcalctemplate_free(host, host->templates);

    while(host->alarms)
        rrdcalc_free(host, host->alarms);
}

void health_reload_host(RRDHOST *host) {
    char *path = health_config_dir();

    // free all running alarms
    rrdhost_wrlock(host);
    health_free_host_nolock(host);
    rrdhost_unlock(host);

    // invalidate all previous entries in the alarm log
    ALARM_ENTRY *t;
    for(t = host->health_log.alarms ; t ; t = t->next) {
        if(t->new_status != RRDCALC_STATUS_REMOVED)
            t->flags |= HEALTH_ENTRY_FLAG_UPDATED;
    }

    // reset all thresholds to all charts
    RRDSET *st;
    for(st = host->rrdset_root; st ; st = st->next) {
        st->green = NAN;
        st->red = NAN;
    }

    // load the new alarms
    rrdhost_wrlock(host);
    health_readdir(host, path);
    rrdhost_unlock(host);

    // link the loaded alarms to their charts
    for(st = host->rrdset_root; st ; st = st->next) {
        rrdhost_wrlock(host);

        rrdsetcalc_link_matching(st);
        rrdcalctemplate_link_matching(st);

        rrdhost_unlock(host);
    }
}

void health_reload(void) {
    RRDHOST *host;

    for(host = localhost; host ; host = host->next)
        health_reload_host(host);
}

// ----------------------------------------------------------------------------
// health main thread and friends

static inline int rrdcalc_value2status(calculated_number n) {
    if(isnan(n) || isinf(n)) return RRDCALC_STATUS_UNDEFINED;
    if(n) return RRDCALC_STATUS_RAISED;
    return RRDCALC_STATUS_CLEAR;
}

#define ALARM_EXEC_COMMAND_LENGTH 8192

static inline void health_alarm_execute(RRDHOST *host, ALARM_ENTRY *ae) {
    ae->flags |= HEALTH_ENTRY_FLAG_PROCESSED;

    if(unlikely(ae->new_status < RRDCALC_STATUS_CLEAR)) {
        // do not send notifications for internal statuses
        debug(D_HEALTH, "Health not sending notification for alarm '%s.%s' status %s (internal statuses)", ae->chart, ae->name, rrdcalc_status2string(ae->new_status));
        goto done;
    }

    if(unlikely(ae->new_status <= RRDCALC_STATUS_CLEAR && (ae->flags & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION))) {
        // do not send notifications for disabled statuses
        debug(D_HEALTH, "Health not sending notification for alarm '%s.%s' status %s (it has no-clear-notification enabled)", ae->chart, ae->name, rrdcalc_status2string(ae->new_status));
        // mark it as run, so that we will send the same alarm if it happens again
        goto done;
    }

    // find the previous notification for the same alarm
    // which we have run the exec script
    // exception: alarms with HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION set
    if(likely(!(ae->flags & HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION))) {
        uint32_t id = ae->alarm_id;
        ALARM_ENTRY *t;
        for(t = ae->next; t ; t = t->next) {
            if(t->alarm_id == id && t->flags & HEALTH_ENTRY_FLAG_EXEC_RUN)
                break;
        }

        if(likely(t)) {
            // we have executed this alarm notification in the past
            if(t && t->new_status == ae->new_status) {
                // don't send the notification for the same status again
                debug(D_HEALTH, "Health not sending again notification for alarm '%s.%s' status %s", ae->chart, ae->name
                      , rrdcalc_status2string(ae->new_status));
                goto done;
            }
        }
        else {
            // we have not executed this alarm notification in the past
            // so, don't send CLEAR notifications
            if(unlikely(ae->new_status == RRDCALC_STATUS_CLEAR)) {
                debug(D_HEALTH, "Health not sending notification for first initialization of alarm '%s.%s' status %s"
                      , ae->chart, ae->name, rrdcalc_status2string(ae->new_status));
                goto done;
            }
        }
    }

    static char command_to_run[ALARM_EXEC_COMMAND_LENGTH + 1];
    pid_t command_pid;

    const char *exec      = (ae->exec)      ? ae->exec      : host->health_default_exec;
    const char *recipient = (ae->recipient) ? ae->recipient : host->health_default_recipient;

    snprintfz(command_to_run, ALARM_EXEC_COMMAND_LENGTH, "exec %s '%s' '%s' '%u' '%u' '%u' '%lu' '%s' '%s' '%s' '%s' '%s' '%0.0Lf' '%0.0Lf' '%s' '%u' '%u' '%s' '%s' '%s' '%s'",
              exec,
              recipient,
              host->hostname,
              ae->unique_id,
              ae->alarm_id,
              ae->alarm_event_id,
              (unsigned long)ae->when,
              ae->name,
              ae->chart?ae->chart:"NOCAHRT",
              ae->family?ae->family:"NOFAMILY",
              rrdcalc_status2string(ae->new_status),
              rrdcalc_status2string(ae->old_status),
              ae->new_value,
              ae->old_value,
              ae->source?ae->source:"UNKNOWN",
              (uint32_t)ae->duration,
              (uint32_t)ae->non_clear_duration,
              ae->units?ae->units:"",
              ae->info?ae->info:"",
              ae->new_value_string,
              ae->old_value_string
    );

    ae->flags |= HEALTH_ENTRY_FLAG_EXEC_RUN;
    ae->exec_run_timestamp = now_realtime_sec();

    debug(D_HEALTH, "executing command '%s'", command_to_run);
    FILE *fp = mypopen(command_to_run, &command_pid);
    if(!fp) {
        error("HEALTH: Cannot popen(\"%s\", \"r\").", command_to_run);
        goto done;
    }
    debug(D_HEALTH, "HEALTH reading from command");
    char *s = fgets(command_to_run, FILENAME_MAX, fp);
    (void)s;
    ae->exec_code = mypclose(fp, command_pid);
    debug(D_HEALTH, "done executing command - returned with code %d", ae->exec_code);

    if(ae->exec_code != 0)
        ae->flags |= HEALTH_ENTRY_FLAG_EXEC_FAILED;

done:
    health_alarm_log_save(host, ae);
    return;
}

static inline void health_process_notifications(RRDHOST *host, ALARM_ENTRY *ae) {
    debug(D_HEALTH, "Health alarm '%s.%s' = %0.2Lf - changed status from %s to %s",
         ae->chart?ae->chart:"NOCHART", ae->name,
         ae->new_value,
         rrdcalc_status2string(ae->old_status),
         rrdcalc_status2string(ae->new_status)
    );

    health_alarm_execute(host, ae);
}

static inline void health_alarm_log_process(RRDHOST *host) {
    static uint32_t stop_at_id = 0;
    uint32_t first_waiting = (host->health_log.alarms)?host->health_log.alarms->unique_id:0;
    time_t now = now_realtime_sec();

    pthread_rwlock_rdlock(&host->health_log.alarm_log_rwlock);

    ALARM_ENTRY *ae;
    for(ae = host->health_log.alarms; ae && ae->unique_id >= stop_at_id ; ae = ae->next) {
        if(unlikely(
            !(ae->flags & HEALTH_ENTRY_FLAG_PROCESSED) &&
            !(ae->flags & HEALTH_ENTRY_FLAG_UPDATED)
            )) {

            if(unlikely(ae->unique_id < first_waiting))
                first_waiting = ae->unique_id;

            if(likely(now >= ae->delay_up_to_timestamp))
                health_process_notifications(host, ae);
        }
    }

    // remember this for the next iteration
    stop_at_id = first_waiting;

    pthread_rwlock_unlock(&host->health_log.alarm_log_rwlock);

    if(host->health_log.count <= host->health_log.max)
        return;

    // cleanup excess entries in the log
    pthread_rwlock_wrlock(&host->health_log.alarm_log_rwlock);

    ALARM_ENTRY *last = NULL;
    unsigned int count = host->health_log.max * 2 / 3;
    for(ae = host->health_log.alarms; ae && count ; count--, last = ae, ae = ae->next) ;

    if(ae && last && last->next == ae)
        last->next = NULL;
    else
        ae = NULL;

    while(ae) {
        debug(D_HEALTH, "Health removing alarm log entry with id: %u", ae->unique_id);

        ALARM_ENTRY *t = ae->next;

        freez(ae->name);
        freez(ae->chart);
        freez(ae->family);
        freez(ae->exec);
        freez(ae->recipient);
        freez(ae->source);
        freez(ae->units);
        freez(ae->info);
        freez(ae->old_value_string);
        freez(ae->new_value_string);
        freez(ae);

        ae = t;
        host->health_log.count--;
    }

    pthread_rwlock_unlock(&host->health_log.alarm_log_rwlock);
}

static inline int rrdcalc_isrunnable(RRDCALC *rc, time_t now, time_t *next_run) {
    if(unlikely(!rc->rrdset)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. It is not linked to a chart.", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    if(unlikely(rc->next_update > now)) {
        if (unlikely(*next_run > rc->next_update)) {
            // update the next_run time of the main loop
            // to run this alarm precisely the time required
            *next_run = rc->next_update;
        }

        debug(D_HEALTH, "Health not examining alarm '%s.%s' yet (will do in %d secs).", rc->chart?rc->chart:"NOCHART", rc->name, (int) (rc->next_update - now));
        return 0;
    }

    if(unlikely(!rc->update_every)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. It does not have an update frequency", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    if(unlikely(!rc->rrdset->last_collected_time.tv_sec || rc->rrdset->counter_done < 2)) {
        debug(D_HEALTH, "Health not running alarm '%s.%s'. Chart is not fully collected yet.", rc->chart?rc->chart:"NOCHART", rc->name);
        return 0;
    }

    int update_every = rc->rrdset->update_every;
    time_t first = rrdset_first_entry_t(rc->rrdset);
    time_t last = rrdset_last_entry_t(rc->rrdset);

    if(unlikely(now + update_every < first /* || now - update_every > last */)) {
        debug(D_HEALTH
              , "Health not examining alarm '%s.%s' yet (wanted time is out of bounds - we need %lu but got %lu - %lu)."
              , rc->chart ? rc->chart : "NOCHART", rc->name, (unsigned long) now, (unsigned long) first
              , (unsigned long) last);
        return 0;
    }

    if(RRDCALC_HAS_DB_LOOKUP(rc)) {
        time_t needed = now + rc->before + rc->after;

        if(needed + update_every < first || needed - update_every > last) {
            debug(D_HEALTH
                  , "Health not examining alarm '%s.%s' yet (not enough data yet - we need %lu but got %lu - %lu)."
                  , rc->chart ? rc->chart : "NOCHART", rc->name, (unsigned long) needed, (unsigned long) first
                  , (unsigned long) last);
            return 0;
        }
    }

    return 1;
}

void *health_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("HEALTH thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    int min_run_every = (int)config_get_number("health", "run at least every seconds", 10);
    if(min_run_every < 1) min_run_every = 1;

    BUFFER *wb = buffer_create(100);

    unsigned int loop = 0;
    while(!netdata_exit) {
        loop++;
        debug(D_HEALTH, "Health monitoring iteration no %u started", loop);

        int oldstate, runnable = 0;
        time_t now = now_realtime_sec();
        time_t next_run = now + min_run_every;
        RRDCALC *rc;

        if(unlikely(pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, &oldstate) != 0))
            error("Cannot set pthread cancel state to DISABLE.");

        RRDHOST *host;
        for(host = localhost; host ; host = host->next) {
            if(unlikely(!host->health_enabled)) continue;

            rrdhost_rdlock(host);

            // the first loop is to lookup values from the db
            for(rc = host->alarms; rc; rc = rc->next) {
                if(unlikely(!rrdcalc_isrunnable(rc, now, &next_run))) {
                    if(unlikely(rc->rrdcalc_flags & RRDCALC_FLAG_RUNNABLE))
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_RUNNABLE;
                    continue;
                }

                runnable++;
                rc->old_value = rc->value;
                rc->rrdcalc_flags |= RRDCALC_FLAG_RUNNABLE;

                // 1. if there is database lookup, do it
                // 2. if there is calculation expression, run it

                if(unlikely(RRDCALC_HAS_DB_LOOKUP(rc))) {
                    /* time_t old_db_timestamp = rc->db_before; */
                    int value_is_null = 0;

                    int ret = rrd2value(rc->rrdset, wb, &rc->value, rc->dimensions, 1, rc->after, rc->before, rc->group, rc->options, &rc->db_after, &rc->db_before, &value_is_null);

                    if(unlikely(ret != 200)) {
                        // database lookup failed
                        rc->value = NAN;

                        debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database lookup returned error %d", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name, ret);

                        if(unlikely(!(rc->rrdcalc_flags & RRDCALC_FLAG_DB_ERROR))) {
                            rc->rrdcalc_flags |= RRDCALC_FLAG_DB_ERROR;
                            error("Health on host '%s', alarm '%s.%s': database lookup returned error %d", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name, ret);
                        }
                    }
                    else if(unlikely(rc->rrdcalc_flags & RRDCALC_FLAG_DB_ERROR))
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_DB_ERROR;

                    /* - RRDCALC_FLAG_DB_STALE not currently used
                    if (unlikely(old_db_timestamp == rc->db_before)) {
                        // database is stale

                        debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database is stale", host->hostname, rc->chart?rc->chart:"NOCHART", rc->name);

                        if (unlikely(!(rc->rrdcalc_flags & RRDCALC_FLAG_DB_STALE))) {
                            rc->rrdcalc_flags |= RRDCALC_FLAG_DB_STALE;
                            error("Health on host '%s', alarm '%s.%s': database is stale", host->hostname, rc->chart?rc->chart:"NOCHART", rc->name);
                        }
                    }
                    else if (unlikely(rc->rrdcalc_flags & RRDCALC_FLAG_DB_STALE))
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_DB_STALE;
                    */

                    if(unlikely(value_is_null)) {
                        // collected value is null

                        rc->value = NAN;

                        debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database lookup returned empty value (possibly value is not collected yet)", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name);

                        if(unlikely(!(rc->rrdcalc_flags & RRDCALC_FLAG_DB_NAN))) {
                            rc->rrdcalc_flags |= RRDCALC_FLAG_DB_NAN;
                            error("Health on host '%s', alarm '%s.%s': database lookup returned empty value (possibly value is not collected yet)", host->hostname,  rc->chart ? rc->chart : "NOCHART", rc->name);
                        }
                    }
                    else if(unlikely(rc->rrdcalc_flags & RRDCALC_FLAG_DB_NAN))
                        rc->rrdcalc_flags &= ~RRDCALC_FLAG_DB_NAN;

                    debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': database lookup gave value " CALCULATED_NUMBER_FORMAT, host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name, rc->value);
                }

                if(unlikely(rc->calculation)) {
                    if(unlikely(!expression_evaluate(rc->calculation))) {
                        // calculation failed

                        rc->value = NAN;

                        debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': expression '%s' failed: %s", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name, rc->calculation->parsed_as, buffer_tostring(rc->calculation->error_msg));

                        if(unlikely(!(rc->rrdcalc_flags & RRDCALC_FLAG_CALC_ERROR))) {
                            rc->rrdcalc_flags |= RRDCALC_FLAG_CALC_ERROR;
                            error("Health on host '%s', alarm '%s.%s': expression '%s' failed: %s", rc->chart ? rc->chart : "NOCHART", host->hostname,  rc->name, rc->calculation->parsed_as, buffer_tostring(rc->calculation->error_msg));
                        }
                    }
                    else {
                        if(unlikely(rc->rrdcalc_flags & RRDCALC_FLAG_CALC_ERROR))
                            rc->rrdcalc_flags &= ~RRDCALC_FLAG_CALC_ERROR;

                        debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': expression '%s' gave value "
                                CALCULATED_NUMBER_FORMAT
                                ": %s (source: %s)", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name
                              , rc->calculation->parsed_as, rc->calculation->result,
                                buffer_tostring(rc->calculation->error_msg), rc->source
                        );

                        rc->value = rc->calculation->result;
                    }
                }
            }
            rrdhost_unlock(host);

            if(unlikely(runnable && !netdata_exit)) {
                rrdhost_rdlock(host);

                for(rc = host->alarms; rc; rc = rc->next) {
                    if(unlikely(!(rc->rrdcalc_flags & RRDCALC_FLAG_RUNNABLE)))
                        continue;

                    int warning_status = RRDCALC_STATUS_UNDEFINED;
                    int critical_status = RRDCALC_STATUS_UNDEFINED;

                    if(likely(rc->warning)) {
                        if(unlikely(!expression_evaluate(rc->warning))) {
                            // calculation failed

                            debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': warning expression failed with error: %s", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name, buffer_tostring(rc->warning->error_msg));

                            if(unlikely(!(rc->rrdcalc_flags & RRDCALC_FLAG_WARN_ERROR))) {
                                rc->rrdcalc_flags |= RRDCALC_FLAG_WARN_ERROR;
                                error("Health on host '%s', alarm '%s.%s': warning expression failed with error: %s", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name, buffer_tostring(rc->warning->error_msg));
                            }
                        }
                        else {
                            if(unlikely(rc->rrdcalc_flags & RRDCALC_FLAG_WARN_ERROR))
                                rc->rrdcalc_flags &= ~RRDCALC_FLAG_WARN_ERROR;

                            debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': warning expression gave value " CALCULATED_NUMBER_FORMAT ": %s (source: %s)", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name, rc->warning->result, buffer_tostring(rc->warning->error_msg), rc->source);

                            warning_status = rrdcalc_value2status(rc->warning->result);
                        }
                    }

                    if(likely(rc->critical)) {
                        if(unlikely(!expression_evaluate(rc->critical))) {
                            // calculation failed

                            debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': critical expression failed with error: %s", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name, buffer_tostring(rc->critical->error_msg));

                            if(unlikely(!(rc->rrdcalc_flags & RRDCALC_FLAG_CRIT_ERROR))) {
                                rc->rrdcalc_flags |= RRDCALC_FLAG_CRIT_ERROR;
                                error("Health on host '%s', alarm '%s.%s': critical expression failed with error: %s", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name, buffer_tostring(rc->critical->error_msg));
                            }
                        }
                        else {
                            if(unlikely(rc->rrdcalc_flags & RRDCALC_FLAG_CRIT_ERROR))
                                rc->rrdcalc_flags &= ~RRDCALC_FLAG_CRIT_ERROR;

                            debug(D_HEALTH, "Health on host '%s', alarm '%s.%s': critical expression gave value " CALCULATED_NUMBER_FORMAT ": %s (source: %s)", host->hostname, rc->chart ? rc->chart : "NOCHART", rc->name , rc->critical->result, buffer_tostring(rc->critical->error_msg), rc->source);

                            critical_status = rrdcalc_value2status(rc->critical->result);
                        }
                    }

                    int status = RRDCALC_STATUS_UNDEFINED;

                    switch(warning_status) {
                        case RRDCALC_STATUS_CLEAR:
                            status = RRDCALC_STATUS_CLEAR;
                            break;

                        case RRDCALC_STATUS_RAISED:
                            status = RRDCALC_STATUS_WARNING;
                            break;

                        default:
                            break;
                    }

                    switch(critical_status) {
                        case RRDCALC_STATUS_CLEAR:
                            if(status == RRDCALC_STATUS_UNDEFINED)
                                status = RRDCALC_STATUS_CLEAR;
                            break;

                        case RRDCALC_STATUS_RAISED:
                            status = RRDCALC_STATUS_CRITICAL;
                            break;

                        default:
                            break;
                    }

                    if(status != rc->status) {
                        int delay = 0;

                        if(now > rc->delay_up_to_timestamp) {
                            rc->delay_up_current = rc->delay_up_duration;
                            rc->delay_down_current = rc->delay_down_duration;
                            rc->delay_last = 0;
                            rc->delay_up_to_timestamp = 0;
                        }
                        else {
                            rc->delay_up_current = (int) (rc->delay_up_current * rc->delay_multiplier);
                            if(rc->delay_up_current > rc->delay_max_duration)
                                rc->delay_up_current = rc->delay_max_duration;

                            rc->delay_down_current = (int) (rc->delay_down_current * rc->delay_multiplier);
                            if(rc->delay_down_current > rc->delay_max_duration)
                                rc->delay_down_current = rc->delay_max_duration;
                        }

                        if(status > rc->status)
                            delay = rc->delay_up_current;
                        else
                            delay = rc->delay_down_current;

                        // COMMENTED: because we do need to send raising alarms
                        // if(now + delay < rc->delay_up_to_timestamp)
                        //    delay = (int)(rc->delay_up_to_timestamp - now);

                        rc->delay_last = delay;
                        rc->delay_up_to_timestamp = now + delay;
                        health_alarm_log(
                                host, rc->id, rc->next_event_id++, now, rc->name, rc->rrdset->id
                                , rc->rrdset->family, rc->exec, rc->recipient, now - rc->last_status_change
                                , rc->old_value, rc->value, rc->status, status, rc->source, rc->units, rc->info
                                , rc->delay_last, (rc->options & RRDCALC_FLAG_NO_CLEAR_NOTIFICATION)
                                                  ? HEALTH_ENTRY_FLAG_NO_CLEAR_NOTIFICATION : 0
                        );
                        rc->last_status_change = now;
                        rc->status = status;
                    }

                    rc->last_updated = now;
                    rc->next_update = now + rc->update_every;

                    if(next_run > rc->next_update)
                        next_run = rc->next_update;
                }

                rrdhost_unlock(host);
            }

            if(unlikely(netdata_exit))
                break;

            // execute notifications
            // and cleanup
            health_alarm_log_process(host);

            if(unlikely(netdata_exit))
                break;

        } /* host loop */

        if(unlikely(pthread_setcancelstate(oldstate, NULL) != 0))
            error("Cannot set pthread cancel state to RESTORE (%d).", oldstate);

        if(unlikely(netdata_exit))
            break;

        now = now_realtime_sec();
        if(now < next_run) {
            debug(D_HEALTH, "Health monitoring iteration no %u done. Next iteration in %d secs", loop, (int) (next_run - now));
            sleep_usec(USEC_PER_SEC * (usec_t) (next_run - now));
        }
        else
            debug(D_HEALTH, "Health monitoring iteration no %u done. Next iteration now", loop);
    }

    buffer_free(wb);

    info("HEALTH thread exiting");

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}
