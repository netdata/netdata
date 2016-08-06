#include "common.h"

int rrdvar_compare(void* a, void* b) {
    if(((RRDVAR *)a)->hash < ((RRDVAR *)b)->hash) return -1;
    else if(((RRDVAR *)a)->hash > ((RRDVAR *)b)->hash) return 1;
    else return strcmp(((RRDVAR *)a)->name, ((RRDVAR *)b)->name);
}

#define rrdvar_index_add(tree, rv) (RRDVAR *)avl_insert_lock(tree, (avl *)(rv))
#define rrdvar_index_del(tree, rv) (RRDVAR *)avl_remove_lock(tree, (avl *)(rv))

static RRDVAR *rrdvar_index_find(avl_tree_lock *tree, const char *name, uint32_t hash) {
    RRDVAR tmp;
    tmp.name = (char *)name;
    tmp.hash = (hash)?hash:simple_hash(tmp.name);

    return (RRDVAR *)avl_search_lock(tree, (avl *)&tmp);
}


RRDVAR *rrdvar_get(RRDHOST *host, RRDCONTEXT *co, RRDSET *st, const char *name) {
    uint32_t hash = simple_hash(name);

    RRDVAR *ret = NULL;
    RRDVAR *rv = rrdvar_index_find(&st->variables_root_index, name, hash);
    if(!rv) {
        rv = calloc(1, sizeof(RRDVAR));
        if(!rv) fatal("Cannot allocate memory for RRDVAR");

        rv->name = strdup(name);
        if(!rv->name) fatal("Cannot allocate memory for RRDVAR name");

        rv->hash = hash;

        ret = rrdvar_index_add(&st->variables_root_index, rv);
        if(ret != rv) {
            error("Duplicate RRDVAR '%s' found on chart '%s'", name, st->id);
            free(rv->name);
            free(rv);
            rv = ret;
        }

        ret = rrdvar_index_add(&co->variables_root_index, rv);
        if(ret != rv)
            debug(D_VARIABLES, "Variable '%s' in context '%s' does not come from chart '%s'", name, co->id, st->id);

        ret = rrdvar_index_add(&host->variables_root_index, rv);
        if(ret != rv)
            debug(D_VARIABLES, "Variable '%s' in host '%s' does not come from chart '%s'", name, host->hostname, st->id);
    }

    return rv;
}
