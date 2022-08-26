// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_HEALTH_INTERNALS
#include "rrd.h"

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
    RRDVAR *ret = (RRDVAR *)avl_insert_lock(tree, (avl_t *)(rv));
    if(ret != rv)
        debug(D_VARIABLES, "Request to insert RRDVAR '%s' into index failed. Already exists.", rv->name);

    return ret;
}

static inline RRDVAR *rrdvar_index_del(avl_tree_lock *tree, RRDVAR *rv) {
    RRDVAR *ret = (RRDVAR *)avl_remove_lock(tree, (avl_t *)(rv));
    if(!ret)
        error("Request to remove RRDVAR '%s' from index failed. Not Found.", rv->name);

    return ret;
}

static inline RRDVAR *rrdvar_index_find(avl_tree_lock *tree, const char *name, uint32_t hash) {
    RRDVAR tmp;
    tmp.name = (char *)name;
    tmp.hash = (hash)?hash:simple_hash(tmp.name);

    return (RRDVAR *)avl_search_lock(tree, (avl_t *)&tmp);
}

inline void rrdvar_free(RRDHOST *host, avl_tree_lock *tree, RRDVAR *rv) {
    (void)host;

    if(!rv) return;

    if(tree) {
        debug(D_VARIABLES, "Deleting variable '%s'", rv->name);
        if(unlikely(!rrdvar_index_del(tree, rv)))
            error("RRDVAR: Attempted to delete variable '%s' from host '%s', but it is not found.", rv->name, host->hostname);
    }

    if(rv->options & RRDVAR_OPTION_ALLOCATED)
        freez(rv->value);

    freez(rv->name);
    freez(rv);
}

inline RRDVAR *rrdvar_create_and_index(const char *scope __maybe_unused, avl_tree_lock *tree, const char *name,
                                       RRDVAR_TYPE type, RRDVAR_OPTIONS options, void *value) {
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
        rv->options = options;
        rv->value = value;
        rv->last_updated = now_realtime_sec();

        RRDVAR *ret = rrdvar_index_add(tree, rv);
        if(unlikely(ret != rv)) {
            debug(D_VARIABLES, "Variable '%s' in scope '%s' already exists", variable, scope);
            freez(rv);
            freez(variable);
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

void rrdvar_free_remaining_variables(RRDHOST *host, avl_tree_lock *tree_lock) {
    // This is not bullet proof - avl should support some means to destroy it
    // with a callback for each item already in the index

    RRDVAR *rv, *last = NULL;
    while((rv = (RRDVAR *)tree_lock->avl_tree.root)) {
        if(unlikely(rv == last)) {
            error("RRDVAR: INTERNAL ERROR: Cannot cleanup tree of RRDVARs");
            break;
        }
        last = rv;
        rrdvar_free(host, tree_lock, rv);
    }
}

// ----------------------------------------------------------------------------
// CUSTOM HOST VARIABLES

inline int rrdvar_callback_for_all_host_variables(RRDHOST *host, int (*callback)(void * /*rrdvar*/, void * /*data*/), void *data) {
    return avl_traverse_lock(&host->rrdvar_root_index, callback, data);
}

static RRDVAR *rrdvar_custom_variable_create(const char *scope, avl_tree_lock *tree_lock, const char *name) {
    NETDATA_DOUBLE *v = callocz(1, sizeof(NETDATA_DOUBLE));
    *v = NAN;

    RRDVAR *rv = rrdvar_create_and_index(scope, tree_lock, name, RRDVAR_TYPE_CALCULATED, RRDVAR_OPTION_CUSTOM_HOST_VAR|RRDVAR_OPTION_ALLOCATED, v);
    if(unlikely(!rv)) {
        freez(v);
        debug(D_VARIABLES, "Requested variable '%s' already exists - possibly 2 plugins are updating it at the same time.", name);

        char *variable = strdupz(name);
        rrdvar_fix_name(variable);
        uint32_t hash = simple_hash(variable);

        // find the existing one to return it
        rv = rrdvar_index_find(tree_lock, variable, hash);

        freez(variable);
    }

    return rv;
}

RRDVAR *rrdvar_custom_host_variable_create(RRDHOST *host, const char *name) {
    return rrdvar_custom_variable_create("host", &host->rrdvar_root_index, name);
}

void rrdvar_custom_host_variable_set(RRDHOST *host, RRDVAR *rv, NETDATA_DOUBLE value) {
    if(rv->type != RRDVAR_TYPE_CALCULATED || !(rv->options & RRDVAR_OPTION_CUSTOM_HOST_VAR) || !(rv->options & RRDVAR_OPTION_ALLOCATED))
        error("requested to set variable '%s' to value " NETDATA_DOUBLE_FORMAT " but the variable is not a custom one.", rv->name, value);
    else {
        NETDATA_DOUBLE *v = rv->value;
        if(*v != value) {
            *v = value;

            rv->last_updated = now_realtime_sec();

            // if the host is streaming, send this variable upstream immediately
            rrdpush_sender_send_this_host_variable_now(host, rv);
        }
    }
}

int foreach_host_variable_callback(RRDHOST *host, int (*callback)(RRDVAR * /*rv*/, void * /*data*/), void *data) {
    return avl_traverse_lock(&host->rrdvar_root_index, (int (*)(void *, void *))callback, data);
}

// ----------------------------------------------------------------------------
// RRDVAR lookup

NETDATA_DOUBLE rrdvar2number(RRDVAR *rv) {
    switch(rv->type) {
        case RRDVAR_TYPE_CALCULATED: {
            NETDATA_DOUBLE *n = (NETDATA_DOUBLE *)rv->value;
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
            error("I don't know how to convert RRDVAR type %u to NETDATA_DOUBLE", rv->type);
            return NAN;
    }
}

int health_variable_lookup(const char *variable, uint32_t hash, RRDCALC *rc, NETDATA_DOUBLE *result) {
    RRDSET *st = rc->rrdset;
    if(!st) return 0;

    RRDHOST *host = st->rrdhost;
    RRDVAR *rv;

    rv = rrdvar_index_find(&st->rrdvar_root_index, variable, hash);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    rv = rrdvar_index_find(&st->rrdfamily->rrdvar_root_index, variable, hash);
    if(rv) {
        *result = rrdvar2number(rv);
        return 1;
    }

    rv = rrdvar_index_find(&host->rrdvar_root_index, variable, hash);
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
    RRDVAR_OPTIONS options;
};

static int single_variable2json(void *entry, void *data) {
    struct variable2json_helper *helper = (struct variable2json_helper *)data;
    RRDVAR *rv = (RRDVAR *)entry;
    NETDATA_DOUBLE value = rrdvar2number(rv);

    if (helper->options == RRDVAR_OPTION_DEFAULT || rv->options & helper->options) {
        if(unlikely(isnan(value) || isinf(value)))
            buffer_sprintf(helper->buf, "%s\n\t\t\"%s\": null", helper->counter?",":"", rv->name);
        else
            buffer_sprintf(helper->buf, "%s\n\t\t\"%s\": %0.5" NETDATA_DOUBLE_MODIFIER, helper->counter?",":"", rv->name, (NETDATA_DOUBLE)value);

        helper->counter++;
    }

    return 0;
}

void health_api_v1_chart_custom_variables2json(RRDSET *st, BUFFER *buf) {
    struct variable2json_helper helper = {
            .buf = buf,
            .counter = 0,
            .options = RRDVAR_OPTION_CUSTOM_CHART_VAR
    };

    buffer_sprintf(buf, "{");
    avl_traverse_lock(&st->rrdvar_root_index, single_variable2json, (void *)&helper);
    buffer_strcat(buf, "\n\t\t\t}");
}

void health_api_v1_chart_variables2json(RRDSET *st, BUFFER *buf) {
    RRDHOST *host = st->rrdhost;

    struct variable2json_helper helper = {
            .buf = buf,
            .counter = 0,
            .options = RRDVAR_OPTION_DEFAULT
    };

    buffer_sprintf(buf, "{\n\t\"chart\": \"%s\",\n\t\"chart_name\": \"%s\",\n\t\"chart_context\": \"%s\",\n\t\"chart_variables\": {", rrdset_id(st), rrdset_name(st), rrdset_context(st));
    avl_traverse_lock(&st->rrdvar_root_index, single_variable2json, (void *)&helper);

    buffer_sprintf(buf, "\n\t},\n\t\"family\": \"%s\",\n\t\"family_variables\": {", rrdset_family(st));
    helper.counter = 0;
    avl_traverse_lock(&st->rrdfamily->rrdvar_root_index, single_variable2json, (void *)&helper);

    buffer_sprintf(buf, "\n\t},\n\t\"host\": \"%s\",\n\t\"host_variables\": {", host->hostname);
    helper.counter = 0;
    avl_traverse_lock(&host->rrdvar_root_index, single_variable2json, (void *)&helper);

    buffer_strcat(buf, "\n\t}\n}\n");
}

