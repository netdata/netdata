#define NETDATA_HEALTH_INTERNALS
#include "common.h"

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

inline void rrdvar_free(RRDHOST *host, avl_tree_lock *tree, RRDVAR *rv) {
    (void)host;

    if(!rv) return;

    if(tree) {
        debug(D_VARIABLES, "Deleting variable '%s'", rv->name);
        if(unlikely(!rrdvar_index_del(tree, rv)))
            error("Attempted to delete variable '%s' from host '%s', but it is not found.", rv->name, host->hostname);
    }

    if(rv->type == RRDVAR_TYPE_CALCULATED_ALLOCATED)
        freez(rv->value);

    freez(rv->name);
    freez(rv);
}

inline RRDVAR *rrdvar_create_and_index(const char *scope, avl_tree_lock *tree, const char *name, RRDVAR_TYPE type, void *value) {
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

inline int rrdvar_callback_for_all_variables(RRDHOST *host, int (*callback)(void *rrdvar, void *data), void *data) {
    return avl_traverse_lock(&host->variables_root_index, callback, data);
}

RRDVAR *rrdvar_custom_host_variable_create(RRDHOST *host, const char *name) {
    calculated_number *v = callocz(1, sizeof(calculated_number));
    *v = NAN;
    RRDVAR *rv = rrdvar_create_and_index("host", &host->variables_root_index, name, RRDVAR_TYPE_CALCULATED_ALLOCATED, v);
    if(unlikely(!rv)) {
        free(v);
        error("Requested variable '%s' already exists - possibly 2 plugins are updating it at the same time.", name);

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

void rrdvar_custom_host_variable_set(RRDHOST *host, RRDVAR *rv, calculated_number value) {
    if(rv->type != RRDVAR_TYPE_CALCULATED_ALLOCATED)
        error("requested to set variable '%s' to value " CALCULATED_NUMBER_FORMAT " but the variable is not a custom one.", rv->name, value);
    else {
        calculated_number *v = rv->value;
        *v = value;

        // if the host is streaming, send this variable upstream immediately
        rrdpush_sender_send_this_variable_now(host, rv);
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
            error("I don't know how to convert RRDVAR type %u to calculated_number", rv->type);
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

