// SPDX-License-Identifier: GPL-3.0+

#include "common.h"

#define RRD_TYPE_TC "tc"

// ----------------------------------------------------------------------------
// /sbin/tc processor
// this requires the script plugins.d/tc-qos-helper.sh

#define TC_LINE_MAX 1024

struct tc_class {
    avl avl;

    char *id;
    uint32_t hash;

    char *name;

    char *leafid;
    uint32_t leaf_hash;

    char *parentid;
    uint32_t parent_hash;

    char hasparent;
    char isleaf;
    char isqdisc;
    char render;

    unsigned long long bytes;
    unsigned long long packets;
    unsigned long long dropped;
    unsigned long long overlimits;
    unsigned long long requeues;
    unsigned long long lended;
    unsigned long long borrowed;
    unsigned long long giants;
    unsigned long long tokens;
    unsigned long long ctokens;

    RRDDIM *rd_bytes;
    RRDDIM *rd_packets;
    RRDDIM *rd_dropped;
    RRDDIM *rd_tokens;
    RRDDIM *rd_ctokens;

    char name_updated;
    char updated;   // updated bytes
    int  unupdated; // the number of times, this has been found un-updated

    struct tc_class *next;
    struct tc_class *prev;
};

struct tc_device {
    avl avl;

    char *id;
    uint32_t hash;

    char *name;
    char *family;

    char name_updated;
    char family_updated;

    char enabled;
    char enabled_bytes;
    char enabled_packets;
    char enabled_dropped;
    char enabled_tokens;
    char enabled_ctokens;
    char enabled_all_classes_qdiscs;

    RRDSET *st_bytes;
    RRDSET *st_packets;
    RRDSET *st_dropped;
    RRDSET *st_tokens;
    RRDSET *st_ctokens;

    avl_tree classes_index;

    struct tc_class *classes;
    struct tc_class *last_class;

    struct tc_device *next;
    struct tc_device *prev;
};


struct tc_device *tc_device_root = NULL;

// ----------------------------------------------------------------------------
// tc_device index

static int tc_device_compare(void* a, void* b) {
    if(((struct tc_device *)a)->hash < ((struct tc_device *)b)->hash) return -1;
    else if(((struct tc_device *)a)->hash > ((struct tc_device *)b)->hash) return 1;
    else return strcmp(((struct tc_device *)a)->id, ((struct tc_device *)b)->id);
}

avl_tree tc_device_root_index = {
        NULL,
        tc_device_compare
};

#define tc_device_index_add(st) (struct tc_device *)avl_insert(&tc_device_root_index, (avl *)(st))
#define tc_device_index_del(st) (struct tc_device *)avl_remove(&tc_device_root_index, (avl *)(st))

static inline struct tc_device *tc_device_index_find(const char *id, uint32_t hash) {
    struct tc_device tmp;
    tmp.id = (char *)id;
    tmp.hash = (hash)?hash:simple_hash(tmp.id);

    return (struct tc_device *)avl_search(&(tc_device_root_index), (avl *)&tmp);
}


// ----------------------------------------------------------------------------
// tc_class index

static int tc_class_compare(void* a, void* b) {
    if(((struct tc_class *)a)->hash < ((struct tc_class *)b)->hash) return -1;
    else if(((struct tc_class *)a)->hash > ((struct tc_class *)b)->hash) return 1;
    else return strcmp(((struct tc_class *)a)->id, ((struct tc_class *)b)->id);
}

#define tc_class_index_add(st, rd) (struct tc_class *)avl_insert(&((st)->classes_index), (avl *)(rd))
#define tc_class_index_del(st, rd) (struct tc_class *)avl_remove(&((st)->classes_index), (avl *)(rd))

static inline struct tc_class *tc_class_index_find(struct tc_device *st, const char *id, uint32_t hash) {
    struct tc_class tmp;
    tmp.id = (char *)id;
    tmp.hash = (hash)?hash:simple_hash(tmp.id);

    return (struct tc_class *)avl_search(&(st->classes_index), (avl *) &tmp);
}

// ----------------------------------------------------------------------------

static inline void tc_class_free(struct tc_device *n, struct tc_class *c) {
    if(c == n->classes) {
        if(likely(c->next))
            n->classes = c->next;
        else
            n->classes = c->prev;
    }

    if(c == n->last_class) {
        if(unlikely(c->next))
            n->last_class = c->next;
        else
            n->last_class = c->prev;
    }

    if(c->next) c->next->prev = c->prev;
    if(c->prev) c->prev->next = c->next;

    debug(D_TC_LOOP, "Removing from device '%s' class '%s', parentid '%s', leafid '%s', unused=%d", n->id, c->id, c->parentid?c->parentid:"", c->leafid?c->leafid:"", c->unupdated);

    if(unlikely(tc_class_index_del(n, c) != c))
        error("plugin_tc: INTERNAL ERROR: attempt remove class '%s' from device '%s': removed a different calls", c->id, n->id);

    freez(c->id);
    freez(c->name);
    freez(c->leafid);
    freez(c->parentid);
    freez(c);
}

static inline void tc_device_classes_cleanup(struct tc_device *d) {
    static int cleanup_every = 999;

    if(unlikely(cleanup_every > 0)) {
        cleanup_every = (int) config_get_number("plugin:tc", "cleanup unused classes every", 120);
        if(cleanup_every < 0) cleanup_every = -cleanup_every;
    }

    d->name_updated = 0;
    d->family_updated = 0;

    struct tc_class *c = d->classes;
    while(c) {
        if(unlikely(cleanup_every && c->unupdated >= cleanup_every)) {
            struct tc_class *nc = c->next;
            tc_class_free(d, c);
            c = nc;
        }
        else {
            c->updated = 0;
            c->name_updated = 0;

            c = c->next;
        }
    }
}

static inline void tc_device_commit(struct tc_device *d) {
    static int enable_new_interfaces = -1, enable_bytes = -1, enable_packets = -1, enable_dropped = -1, enable_tokens = -1, enable_ctokens = -1, enabled_all_classes_qdiscs = -1;

    if(unlikely(enable_new_interfaces == -1)) {
        enable_new_interfaces      = config_get_boolean_ondemand("plugin:tc", "enable new interfaces detected at runtime", CONFIG_BOOLEAN_YES);
        enable_bytes               = config_get_boolean_ondemand("plugin:tc", "enable traffic charts for all interfaces", CONFIG_BOOLEAN_AUTO);
        enable_packets             = config_get_boolean_ondemand("plugin:tc", "enable packets charts for all interfaces", CONFIG_BOOLEAN_AUTO);
        enable_dropped             = config_get_boolean_ondemand("plugin:tc", "enable dropped charts for all interfaces", CONFIG_BOOLEAN_AUTO);
        enable_tokens              = config_get_boolean_ondemand("plugin:tc", "enable tokens charts for all interfaces", CONFIG_BOOLEAN_NO);
        enable_ctokens             = config_get_boolean_ondemand("plugin:tc", "enable ctokens charts for all interfaces", CONFIG_BOOLEAN_NO);
        enabled_all_classes_qdiscs = config_get_boolean_ondemand("plugin:tc", "enable show all classes and qdiscs for all interfaces", CONFIG_BOOLEAN_NO);
    }

    if(unlikely(d->enabled == (char)-1)) {
        char var_name[CONFIG_MAX_NAME + 1];
        snprintfz(var_name, CONFIG_MAX_NAME, "qos for %s", d->id);

        d->enabled                    = (char)config_get_boolean_ondemand("plugin:tc", var_name, enable_new_interfaces);

        snprintfz(var_name, CONFIG_MAX_NAME, "traffic chart for %s", d->id);
        d->enabled_bytes              = (char)config_get_boolean_ondemand("plugin:tc", var_name, enable_bytes);

        snprintfz(var_name, CONFIG_MAX_NAME, "packets chart for %s", d->id);
        d->enabled_packets            = (char)config_get_boolean_ondemand("plugin:tc", var_name, enable_packets);

        snprintfz(var_name, CONFIG_MAX_NAME, "dropped packets chart for %s", d->id);
        d->enabled_dropped            = (char)config_get_boolean_ondemand("plugin:tc", var_name, enable_dropped);

        snprintfz(var_name, CONFIG_MAX_NAME, "tokens chart for %s", d->id);
        d->enabled_tokens             = (char)config_get_boolean_ondemand("plugin:tc", var_name, enable_tokens);

        snprintfz(var_name, CONFIG_MAX_NAME, "ctokens chart for %s", d->id);
        d->enabled_ctokens            = (char)config_get_boolean_ondemand("plugin:tc", var_name, enable_ctokens);

        snprintfz(var_name, CONFIG_MAX_NAME, "show all classes for %s", d->id);
        d->enabled_all_classes_qdiscs = (char)config_get_boolean_ondemand("plugin:tc", var_name, enabled_all_classes_qdiscs);
    }

    // we only need to add leaf classes
    struct tc_class *c, *x /*, *root = NULL */;
    unsigned long long bytes_sum = 0, packets_sum = 0, dropped_sum = 0, tokens_sum = 0, ctokens_sum = 0;
    int active_nodes = 0, updated_classes = 0, updated_qdiscs = 0;

    // prepare all classes
    // we set reasonable defaults for the rest of the code below

    for(c = d->classes ; c ; c = c->next) {
        c->render = 0;          // do not render this class

        c->isleaf = 1;          // this is a leaf class
        c->hasparent = 0;       // without a parent

        if(unlikely(!c->updated))
            c->unupdated++;     // increase its unupdated counter
        else {
            c->unupdated = 0;   // reset its unupdated counter

            // count how many of each kind
            if(c->isqdisc)
                updated_qdiscs++;
            else
                updated_classes++;
        }
    }

    if(unlikely(!d->enabled || (!updated_classes && !updated_qdiscs))) {
        debug(D_TC_LOOP, "TC: Ignoring TC device '%s'. It is not enabled/updated.", d->name?d->name:d->id);
        tc_device_classes_cleanup(d);
        return;
    }

    if(unlikely(updated_classes && updated_qdiscs)) {
        error("TC: device '%s' has active both classes (%d) and qdiscs (%d). Will render only qdiscs.", d->id, updated_classes, updated_qdiscs);

        // set all classes to !updated
        for(c = d->classes ; c ; c = c->next)
            if(unlikely(!c->isqdisc && c->updated))
                c->updated = 0;

        updated_classes = 0;
    }

    // mark the classes as leafs and parents
    //
    // TC is hierarchical:
    //  - classes can have other classes in them
    //  - the same is true for qdiscs (i.e. qdiscs have classes, that have other qdiscs)
    //
    // we need to present a chart with leaf nodes only, so that the sum
    // of all dimensions of the chart, will be the total utilization
    // of the interface.
    //
    // here we try to find the ones we need to report
    // by default all nodes are marked with: isleaf = 1 (see above)
    //
    // so, here we remove the isleaf flag from nodes in the middle
    // and we add the hasparent flag to leaf nodes we found their parent
    if(likely(!d->enabled_all_classes_qdiscs)) {
        for(c = d->classes; c; c = c->next) {
            if(unlikely(!c->updated)) continue;

            //debug(D_TC_LOOP, "TC: In device '%s', %s '%s'  has leafid: '%s' and parentid '%s'.",
            //    d->id,
            //    c->isqdisc?"qdisc":"class",
            //    c->id,
            //    c->leafid?c->leafid:"NULL",
            //    c->parentid?c->parentid:"NULL");

            // find if c is leaf or not
            for(x = d->classes; x; x = x->next) {
                if(unlikely(!x->updated || c == x || !x->parentid)) continue;

                // classes have both parentid and leafid
                // qdiscs have only parentid
                // the following works for both (it is an OR)

                if((c->hash == x->parent_hash && strcmp(c->id, x->parentid) == 0) ||
                   (c->leafid && c->leaf_hash == x->parent_hash && strcmp(c->leafid, x->parentid) == 0)) {
                    // debug(D_TC_LOOP, "TC: In device '%s', %s '%s' (leafid: '%s') has as leaf %s '%s' (parentid: '%s').", d->name?d->name:d->id, c->isqdisc?"qdisc":"class", c->name?c->name:c->id, c->leafid?c->leafid:c->id, x->isqdisc?"qdisc":"class", x->name?x->name:x->id, x->parentid?x->parentid:x->id);
                    c->isleaf = 0;
                    x->hasparent = 1;
                }
            }
        }
    }

    for(c = d->classes ; c ; c = c->next) {
        if(unlikely(!c->updated)) continue;

        // debug(D_TC_LOOP, "TC: device '%s', %s '%s' isleaf=%d, hasparent=%d", d->id, (c->isqdisc)?"qdisc":"class", c->id, c->isleaf, c->hasparent);

        if(unlikely((c->isleaf && c->hasparent) || d->enabled_all_classes_qdiscs)) {
            c->render = 1;
            active_nodes++;
            bytes_sum += c->bytes;
            packets_sum += c->packets;
            dropped_sum += c->dropped;
            tokens_sum += c->tokens;
            ctokens_sum += c->ctokens;
        }

        //if(unlikely(!c->hasparent)) {
        //    if(root) error("TC: multiple root class/qdisc for device '%s' (old: '%s', new: '%s')", d->id, root->id, c->id);
        //    root = c;
        //    debug(D_TC_LOOP, "TC: found root class/qdisc '%s'", root->id);
        //}
    }

#ifdef NETDATA_INTERNAL_CHECKS
    // dump all the list to see what we know

    if(unlikely(debug_flags & D_TC_LOOP)) {
        for(c = d->classes ; c ; c = c->next) {
            if(c->render) debug(D_TC_LOOP, "TC: final nodes dump for '%s': class %s, OK", d->name, c->id);
            else debug(D_TC_LOOP, "TC: final nodes dump for '%s': class %s, IGNORE (updated: %d, isleaf: %d, hasparent: %d, parent: %s)", d->name?d->name:d->id, c->id, c->updated, c->isleaf, c->hasparent, c->parentid?c->parentid:"(unset)");
        }
    }
#endif

    if(unlikely(!active_nodes)) {
        debug(D_TC_LOOP, "TC: Ignoring TC device '%s'. No useful classes/qdiscs.", d->name?d->name:d->id);
        tc_device_classes_cleanup(d);
        return;
    }

    debug(D_TC_LOOP, "TC: evaluating TC device '%s'. enabled = %d/%d (bytes: %d/%d, packets: %d/%d, dropped: %d/%d, tokens: %d/%d, ctokens: %d/%d, all_classes_qdiscs: %d/%d), classes: (bytes = %llu, packets = %llu, dropped = %llu, tokens = %llu, ctokens = %llu).",
        d->name?d->name:d->id,
        d->enabled, enable_new_interfaces,
        d->enabled_bytes, enable_bytes,
        d->enabled_packets, enable_packets,
        d->enabled_dropped, enable_dropped,
        d->enabled_tokens, enable_tokens,
        d->enabled_ctokens, enable_ctokens,
        d->enabled_all_classes_qdiscs, enabled_all_classes_qdiscs,
        bytes_sum,
        packets_sum,
        dropped_sum,
        tokens_sum,
        ctokens_sum
        );

    // --------------------------------------------------------------------
    // bytes

    if(d->enabled_bytes == CONFIG_BOOLEAN_YES || (d->enabled_bytes == CONFIG_BOOLEAN_AUTO && bytes_sum)) {
        d->enabled_bytes = CONFIG_BOOLEAN_YES;

        if(unlikely(!d->st_bytes))
            d->st_bytes = rrdset_create_localhost(
                    RRD_TYPE_TC
                    , d->id
                    , d->name ? d->name : d->id
                    , d->family ? d->family : d->id
                    , RRD_TYPE_TC ".qos"
                    , "Class Usage"
                    , "kilobits/s"
                    , "tc"
                    , NULL
                    , 7000
                    , localhost->rrd_update_every
                    , d->enabled_all_classes_qdiscs ? RRDSET_TYPE_LINE : RRDSET_TYPE_STACKED
            );

        else {
            rrdset_next(d->st_bytes);
            if(unlikely(d->name_updated)) rrdset_set_name(d->st_bytes, d->name);

            // FIXME
            // update the family
        }

        for(c = d->classes ; c ; c = c->next) {
            if(unlikely(!c->render)) continue;

            if(unlikely(!c->rd_bytes))
                c->rd_bytes = rrddim_add(d->st_bytes, c->id, c->name?c->name:c->id, 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            else if(unlikely(c->name_updated))
                rrddim_set_name(d->st_bytes, c->rd_bytes, c->name);

            rrddim_set_by_pointer(d->st_bytes, c->rd_bytes, c->bytes);
        }
        rrdset_done(d->st_bytes);
    }

    // --------------------------------------------------------------------
    // packets

    if(d->enabled_packets == CONFIG_BOOLEAN_YES || (d->enabled_packets == CONFIG_BOOLEAN_AUTO && packets_sum)) {
        d->enabled_packets = CONFIG_BOOLEAN_YES;

        if(unlikely(!d->st_packets)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_packets", d->id);
            snprintfz(name, RRD_ID_LENGTH_MAX, "%s_packets", d->name?d->name:d->id);

            d->st_packets = rrdset_create_localhost(
                    RRD_TYPE_TC
                    , id
                    , name
                    , d->family ? d->family : d->id
                    , RRD_TYPE_TC ".qos_packets"
                    , "Class Packets"
                    , "packets/s"
                    , "tc"
                    , NULL
                    , 7010
                    , localhost->rrd_update_every
                    , d->enabled_all_classes_qdiscs ? RRDSET_TYPE_LINE : RRDSET_TYPE_STACKED
            );
        }
        else {
            rrdset_next(d->st_packets);

            if(unlikely(d->name_updated)) {
                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "%s_packets", d->name?d->name:d->id);
                rrdset_set_name(d->st_packets, name);
            }

            // FIXME
            // update the family
        }

        for(c = d->classes ; c ; c = c->next) {
            if(unlikely(!c->render)) continue;

            if(unlikely(!c->rd_packets))
                c->rd_packets = rrddim_add(d->st_packets, c->id, c->name?c->name:c->id, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            else if(unlikely(c->name_updated))
                rrddim_set_name(d->st_packets, c->rd_packets, c->name);

            rrddim_set_by_pointer(d->st_packets, c->rd_packets, c->packets);
        }
        rrdset_done(d->st_packets);
    }

    // --------------------------------------------------------------------
    // dropped

    if(d->enabled_dropped == CONFIG_BOOLEAN_YES || (d->enabled_dropped == CONFIG_BOOLEAN_AUTO && dropped_sum)) {
        d->enabled_dropped = CONFIG_BOOLEAN_YES;

        if(unlikely(!d->st_dropped)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_dropped", d->id);
            snprintfz(name, RRD_ID_LENGTH_MAX, "%s_dropped", d->name?d->name:d->id);

            d->st_dropped = rrdset_create_localhost(
                    RRD_TYPE_TC
                    , id
                    , name
                    , d->family ? d->family : d->id
                    , RRD_TYPE_TC ".qos_dropped"
                    , "Class Dropped Packets"
                    , "packets/s"
                    , "tc"
                    , NULL
                    , 7020
                    , localhost->rrd_update_every
                    , d->enabled_all_classes_qdiscs ? RRDSET_TYPE_LINE : RRDSET_TYPE_STACKED
            );
        }
        else {
            rrdset_next(d->st_dropped);

            if(unlikely(d->name_updated)) {
                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "%s_dropped", d->name?d->name:d->id);
                rrdset_set_name(d->st_dropped, name);
            }

            // FIXME
            // update the family
        }

        for(c = d->classes ; c ; c = c->next) {
            if(unlikely(!c->render)) continue;

            if(unlikely(!c->rd_dropped))
                c->rd_dropped = rrddim_add(d->st_dropped, c->id, c->name?c->name:c->id, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            else if(unlikely(c->name_updated))
                rrddim_set_name(d->st_dropped, c->rd_dropped, c->name);

            rrddim_set_by_pointer(d->st_dropped, c->rd_dropped, c->dropped);
        }
        rrdset_done(d->st_dropped);
    }

    // --------------------------------------------------------------------
    // tokens

    if(d->enabled_tokens == CONFIG_BOOLEAN_YES || (d->enabled_tokens == CONFIG_BOOLEAN_AUTO && tokens_sum)) {
        d->enabled_tokens = CONFIG_BOOLEAN_YES;

        if(unlikely(!d->st_tokens)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_tokens", d->id);
            snprintfz(name, RRD_ID_LENGTH_MAX, "%s_tokens", d->name?d->name:d->id);

            d->st_tokens = rrdset_create_localhost(
                    RRD_TYPE_TC
                    , id
                    , name
                    , d->family ? d->family : d->id
                    , RRD_TYPE_TC ".qos_tokens"
                    , "Class Tokens"
                    , "tokens"
                    , "tc"
                    , NULL
                    , 7030
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
            );
        }
        else {
            rrdset_next(d->st_tokens);

            if(unlikely(d->name_updated)) {
                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "%s_tokens", d->name?d->name:d->id);
                rrdset_set_name(d->st_tokens, name);
            }

            // FIXME
            // update the family
        }

        for(c = d->classes ; c ; c = c->next) {
            if(unlikely(!c->render)) continue;

            if(unlikely(!c->rd_tokens)) {
                c->rd_tokens = rrddim_add(d->st_tokens, c->id, c->name?c->name:c->id, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else if(unlikely(c->name_updated))
                rrddim_set_name(d->st_tokens, c->rd_tokens, c->name);

            rrddim_set_by_pointer(d->st_tokens, c->rd_tokens, c->tokens);
        }
        rrdset_done(d->st_tokens);
    }

    // --------------------------------------------------------------------
    // ctokens

    if(d->enabled_ctokens == CONFIG_BOOLEAN_YES || (d->enabled_ctokens == CONFIG_BOOLEAN_AUTO && ctokens_sum)) {
        d->enabled_ctokens = CONFIG_BOOLEAN_YES;

        if(unlikely(!d->st_ctokens)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_ctokens", d->id);
            snprintfz(name, RRD_ID_LENGTH_MAX, "%s_ctokens", d->name?d->name:d->id);

            d->st_ctokens = rrdset_create_localhost(
                    RRD_TYPE_TC
                    , id
                    , name
                    , d->family ? d->family : d->id
                    , RRD_TYPE_TC ".qos_ctokens"
                    , "Class cTokens"
                    , "ctokens"
                    , "tc"
                    , NULL
                    , 7040
                    , localhost->rrd_update_every
                    , RRDSET_TYPE_LINE
            );
        }
        else {
            debug(D_TC_LOOP, "TC: Updating _ctokens chart for device '%s'", d->name?d->name:d->id);
            rrdset_next(d->st_ctokens);

            if(unlikely(d->name_updated)) {
                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "%s_ctokens", d->name?d->name:d->id);
                rrdset_set_name(d->st_ctokens, name);
            }

            // FIXME
            // update the family
        }

        for(c = d->classes ; c ; c = c->next) {
            if(unlikely(!c->render)) continue;

            if(unlikely(!c->rd_ctokens))
                c->rd_ctokens = rrddim_add(d->st_ctokens, c->id, c->name?c->name:c->id, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            else if(unlikely(c->name_updated))
                rrddim_set_name(d->st_ctokens, c->rd_ctokens, c->name);

            rrddim_set_by_pointer(d->st_ctokens, c->rd_ctokens, c->ctokens);
        }
        rrdset_done(d->st_ctokens);
    }

    tc_device_classes_cleanup(d);
}

static inline void tc_device_set_class_name(struct tc_device *d, char *id, char *name) {
    if(unlikely(!name || !*name)) return;

    struct tc_class *c = tc_class_index_find(d, id, 0);
    if(likely(c)) {
        if(likely(c->name)) {
            if(!strcmp(c->name, name)) return;
            freez(c->name);
            c->name = NULL;
        }

        if(likely(name && *name && strcmp(c->id, name) != 0)) {
            debug(D_TC_LOOP, "TC: Setting device '%s', class '%s' name to '%s'", d->id, id, name);
            c->name = strdupz(name);
            c->name_updated = 1;
        }
    }
}

static inline void tc_device_set_device_name(struct tc_device *d, char *name) {
    if(unlikely(!name || !*name)) return;

    if(d->name) {
        if(!strcmp(d->name, name)) return;
        freez(d->name);
        d->name = NULL;
    }

    if(likely(name && *name && strcmp(d->id, name) != 0)) {
        debug(D_TC_LOOP, "TC: Setting device '%s' name to '%s'", d->id, name);
        d->name = strdupz(name);
        d->name_updated = 1;
    }
}

static inline void tc_device_set_device_family(struct tc_device *d, char *family) {
    freez(d->family);
    d->family = NULL;

    if(likely(family && *family && strcmp(d->id, family) != 0)) {
        debug(D_TC_LOOP, "TC: Setting device '%s' family to '%s'", d->id, family);
        d->family = strdupz(family);
        d->family_updated = 1;
    }
    // no need for null termination - it is already null
}

static inline struct tc_device *tc_device_create(char *id)
{
    struct tc_device *d = tc_device_index_find(id, 0);

    if(!d) {
        debug(D_TC_LOOP, "TC: Creating device '%s'", id);

        d = callocz(1, sizeof(struct tc_device));

        d->id = strdupz(id);
        d->hash = simple_hash(d->id);
        d->enabled = (char)-1;

        avl_init(&d->classes_index, tc_class_compare);
        if(unlikely(tc_device_index_add(d) != d))
            error("plugin_tc: INTERNAL ERROR: removing device '%s' removed a different device.", d->id);

        if(!tc_device_root) {
            tc_device_root = d;
        }
        else {
            d->next = tc_device_root;
            tc_device_root->prev = d;
            tc_device_root = d;
        }
    }

    return(d);
}

static inline struct tc_class *tc_class_add(struct tc_device *n, char *id, char qdisc, char *parentid, char *leafid)
{
    struct tc_class *c = tc_class_index_find(n, id, 0);

    if(!c) {
        debug(D_TC_LOOP, "TC: Creating in device '%s', class id '%s', parentid '%s', leafid '%s'", n->id, id, parentid?parentid:"", leafid?leafid:"");

        c = callocz(1, sizeof(struct tc_class));

        if(unlikely(!n->classes))
            n->classes = c;

        else if(likely(n->last_class)) {
            n->last_class->next = c;
            c->prev = n->last_class;
        }

        n->last_class = c;

        c->id = strdupz(id);
        c->hash = simple_hash(c->id);

        c->isqdisc = qdisc;
        if(parentid && *parentid) {
            c->parentid = strdupz(parentid);
            c->parent_hash = simple_hash(c->parentid);
        }

        if(leafid && *leafid) {
            c->leafid = strdupz(leafid);
            c->leaf_hash = simple_hash(c->leafid);
        }

        if(unlikely(tc_class_index_add(n, c) != c))
            error("plugin_tc: INTERNAL ERROR: attempt index class '%s' on device '%s': already exists", c->id, n->id);
    }
    return(c);
}

static inline void tc_device_free(struct tc_device *n)
{
    if(n->next) n->next->prev = n->prev;
    if(n->prev) n->prev->next = n->next;
    if(tc_device_root == n) {
        if(n->next) tc_device_root = n->next;
        else tc_device_root = n->prev;
    }

    if(unlikely(tc_device_index_del(n) != n))
        error("plugin_tc: INTERNAL ERROR: removing device '%s' removed a different device.", n->id);

    while(n->classes) tc_class_free(n, n->classes);

    freez(n->id);
    freez(n->name);
    freez(n->family);
    freez(n);
}

static inline void tc_device_free_all()
{
    while(tc_device_root)
        tc_device_free(tc_device_root);
}

#define PLUGINSD_MAX_WORDS 20

static inline int tc_space(char c) {
    switch(c) {
    case ' ':
    case '\t':
    case '\r':
    case '\n':
        return 1;

    default:
        return 0;
    }
}

static inline void tc_split_words(char *str, char **words, int max_words) {
    char *s = str;
    int i = 0;

    // skip all white space
    while(tc_space(*s)) s++;

    // store the first word
    words[i++] = s;

    // while we have something
    while(*s) {
        // if it is a space
        if(unlikely(tc_space(*s))) {

            // terminate the word
            *s++ = '\0';

            // skip all white space
            while(tc_space(*s)) s++;

            // if we reached the end, stop
            if(!*s) break;

            // store the next word
            if(i < max_words) words[i++] = s;
            else break;
        }
        else s++;
    }

    // terminate the words
    while(i < max_words) words[i++] = NULL;
}

static pid_t tc_child_pid = 0;

static void tc_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    if(tc_child_pid) {
        info("TC: killing with SIGTERM tc-qos-helper process %d", tc_child_pid);
        if(killpid(tc_child_pid, SIGTERM) != -1) {
            siginfo_t info;

            info("TC: waiting for tc plugin child process pid %d to exit...", tc_child_pid);
            waitid(P_PID, (id_t) tc_child_pid, &info, WEXITED);
            // info("TC: finished tc plugin child process pid %d.", tc_child_pid);
        }

        tc_child_pid = 0;
    }

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

void *tc_main(void *ptr) {
    netdata_thread_cleanup_push(tc_main_cleanup, ptr);

    struct rusage thread;

    char command[FILENAME_MAX + 1];
    char *words[PLUGINSD_MAX_WORDS] = { NULL };

    uint32_t BEGIN_HASH = simple_hash("BEGIN");
    uint32_t END_HASH = simple_hash("END");
    uint32_t QDISC_HASH = simple_hash("qdisc");
    uint32_t CLASS_HASH = simple_hash("class");
    uint32_t SENT_HASH = simple_hash("Sent");
    uint32_t LENDED_HASH = simple_hash("lended:");
    uint32_t TOKENS_HASH = simple_hash("tokens:");
    uint32_t SETDEVICENAME_HASH = simple_hash("SETDEVICENAME");
    uint32_t SETDEVICEGROUP_HASH = simple_hash("SETDEVICEGROUP");
    uint32_t SETCLASSNAME_HASH = simple_hash("SETCLASSNAME");
    uint32_t WORKTIME_HASH = simple_hash("WORKTIME");
#ifdef DETACH_PLUGINS_FROM_NETDATA
    uint32_t MYPID_HASH = simple_hash("MYPID");
#endif
    uint32_t first_hash;

    snprintfz(command, TC_LINE_MAX, "%s/tc-qos-helper.sh", netdata_configured_plugins_dir);
    char *tc_script = config_get("plugin:tc", "script to run to get tc values", command);

    while(!netdata_exit) {
        FILE *fp;
        struct tc_device *device = NULL;
        struct tc_class *class = NULL;

        snprintfz(command, TC_LINE_MAX, "exec %s %d", tc_script, localhost->rrd_update_every);
        debug(D_TC_LOOP, "executing '%s'", command);

        fp = mypopen(command, (pid_t *)&tc_child_pid);
        if(unlikely(!fp)) {
            error("TC: Cannot popen(\"%s\", \"r\").", command);
            goto cleanup;
        }

        char buffer[TC_LINE_MAX+1] = "";
        while(fgets(buffer, TC_LINE_MAX, fp) != NULL) {
            if(unlikely(netdata_exit)) break;

            buffer[TC_LINE_MAX] = '\0';
            // debug(D_TC_LOOP, "TC: read '%s'", buffer);

            tc_split_words(buffer, words, PLUGINSD_MAX_WORDS);

            if(unlikely(!words[0] || !*words[0])) {
                // debug(D_TC_LOOP, "empty line");
                continue;
            }
            // else debug(D_TC_LOOP, "First word is '%s'", words[0]);

            first_hash = simple_hash(words[0]);

            if(unlikely(device && ((first_hash == CLASS_HASH && strcmp(words[0], "class") == 0) ||  (first_hash == QDISC_HASH && strcmp(words[0], "qdisc") == 0)))) {
                // debug(D_TC_LOOP, "CLASS line on class id='%s', parent='%s', parentid='%s', leaf='%s', leafid='%s'", words[2], words[3], words[4], words[5], words[6]);

                char *type     = words[1];  // the class/qdisc type: htb, fq_codel, etc
                char *id       = words[2];  // the class/qdisc major:minor
                char *parent   = words[3];  // the word 'parent' or 'root'
                char *parentid = words[4];  // parentid
                char *leaf     = words[5];  // the word 'leaf'
                char *leafid   = words[6];  // leafid

                int parent_is_root = 0;
                int parent_is_parent = 0;
                if(likely(parent)) {
                    parent_is_parent = !strcmp(parent, "parent");

                    if(!parent_is_parent)
                        parent_is_root = !strcmp(parent, "root");
                }

                if(likely(type && id && (parent_is_root || parent_is_parent))) {
                    char qdisc = 0;

                    if(first_hash == QDISC_HASH) {
                        qdisc = 1;

                        if(!strcmp(type, "ingress")) {
                            // we don't want to get the ingress qdisc
                            // there should be an IFB interface for this

                            class = NULL;
                            continue;
                        }

                        if(parent_is_parent && parentid) {
                            // eliminate the minor number from parentid
                            // why: parentid is the id of the parent class
                            // but major: is also the id of the parent qdisc

                            char *s = parentid;
                            while(*s && *s != ':') s++;
                            if(*s == ':') s[1] = '\0';
                        }
                    }

                    if(parent_is_root) {
                        parentid = NULL;
                        leafid = NULL;
                    }
                    else if(!leaf || strcmp(leaf, "leaf") != 0)
                        leafid = NULL;

                    char leafbuf[20 + 1] = "";
                    if(leafid && leafid[strlen(leafid) - 1] == ':') {
                        strncpyz(leafbuf, leafid, 20 - 1);
                        strcat(leafbuf, "1");
                        leafid = leafbuf;
                    }

                    class = tc_class_add(device, id, qdisc, parentid, leafid);
                }
                else {
                    // clear the last class
                    class = NULL;
                }
            }
            else if(unlikely(first_hash == END_HASH && strcmp(words[0], "END") == 0)) {
                // debug(D_TC_LOOP, "END line");

                if(likely(device)) {
                    netdata_thread_disable_cancelability();
                    tc_device_commit(device);
                    // tc_device_free(device);
                    netdata_thread_enable_cancelability();
                }

                device = NULL;
                class = NULL;
            }
            else if(unlikely(first_hash == BEGIN_HASH && strcmp(words[0], "BEGIN") == 0)) {
                // debug(D_TC_LOOP, "BEGIN line on device '%s'", words[1]);

                if(likely(words[1] && *words[1])) {
                    device = tc_device_create(words[1]);
                }
                else {
                    // tc_device_free(device);
                    device = NULL;
                }

                class = NULL;
            }
            else if(unlikely(device && class && first_hash == SENT_HASH && strcmp(words[0], "Sent") == 0)) {
                // debug(D_TC_LOOP, "SENT line '%s'", words[1]);
                if(likely(words[1] && *words[1])) {
                    class->bytes = str2ull(words[1]);
                    class->updated = 1;
                }
                else {
                    class->updated = 0;
                }

                if(likely(words[3] && *words[3]))
                    class->packets = str2ull(words[3]);

                if(likely(words[6] && *words[6]))
                    class->dropped = str2ull(words[6]);

                if(likely(words[8] && *words[8]))
                    class->overlimits = str2ull(words[8]);

                if(likely(words[10] && *words[10]))
                    class->requeues = str2ull(words[8]);
            }
            else if(unlikely(device && class && class->updated && first_hash == LENDED_HASH && strcmp(words[0], "lended:") == 0)) {
                // debug(D_TC_LOOP, "LENDED line '%s'", words[1]);
                if(likely(words[1] && *words[1]))
                    class->lended = str2ull(words[1]);

                if(likely(words[3] && *words[3]))
                    class->borrowed = str2ull(words[3]);

                if(likely(words[5] && *words[5]))
                    class->giants = str2ull(words[5]);
            }
            else if(unlikely(device && class && class->updated && first_hash == TOKENS_HASH && strcmp(words[0], "tokens:") == 0)) {
                // debug(D_TC_LOOP, "TOKENS line '%s'", words[1]);
                if(likely(words[1] && *words[1]))
                    class->tokens = str2ull(words[1]);

                if(likely(words[3] && *words[3]))
                    class->ctokens = str2ull(words[3]);
            }
            else if(unlikely(device && first_hash == SETDEVICENAME_HASH && strcmp(words[0], "SETDEVICENAME") == 0)) {
                // debug(D_TC_LOOP, "SETDEVICENAME line '%s'", words[1]);
                if(likely(words[1] && *words[1]))
                    tc_device_set_device_name(device, words[1]);
            }
            else if(unlikely(device && first_hash == SETDEVICEGROUP_HASH && strcmp(words[0], "SETDEVICEGROUP") == 0)) {
                // debug(D_TC_LOOP, "SETDEVICEGROUP line '%s'", words[1]);
                if(likely(words[1] && *words[1]))
                    tc_device_set_device_family(device, words[1]);
            }
            else if(unlikely(device && first_hash == SETCLASSNAME_HASH && strcmp(words[0], "SETCLASSNAME") == 0)) {
                // debug(D_TC_LOOP, "SETCLASSNAME line '%s' '%s'", words[1], words[2]);
                char *id    = words[1];
                char *path  = words[2];
                if(likely(id && *id && path && *path))
                    tc_device_set_class_name(device, id, path);
            }
            else if(unlikely(first_hash == WORKTIME_HASH && strcmp(words[0], "WORKTIME") == 0)) {
                // debug(D_TC_LOOP, "WORKTIME line '%s' '%s'", words[1], words[2]);
                getrusage(RUSAGE_THREAD, &thread);

                static RRDSET *stcpu = NULL;
                static RRDDIM *rd_user = NULL, *rd_system = NULL;

                if(unlikely(!stcpu)) {
                    stcpu = rrdset_create_localhost(
                            "netdata"
                            , "plugin_tc_cpu"
                            , NULL
                            , "tc.helper"
                            , NULL
                            , "NetData TC CPU usage"
                            , "milliseconds/s"
                            , "tc"
                            , NULL
                            , 135000
                            , localhost->rrd_update_every
                            , RRDSET_TYPE_STACKED
                    );
                    rd_user   = rrddim_add(stcpu, "user",  NULL,  1, 1000, RRD_ALGORITHM_INCREMENTAL);
                    rd_system = rrddim_add(stcpu, "system", NULL, 1, 1000, RRD_ALGORITHM_INCREMENTAL);
                }
                else rrdset_next(stcpu);

                rrddim_set_by_pointer(stcpu, rd_user  , thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
                rrddim_set_by_pointer(stcpu, rd_system, thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);
                rrdset_done(stcpu);

                static RRDSET *sttime = NULL;
                static RRDDIM *rd_run_time = NULL;

                if(unlikely(!sttime)) {
                    sttime = rrdset_create_localhost(
                            "netdata"
                            , "plugin_tc_time"
                            , NULL
                            , "tc.helper"
                            , NULL
                            , "NetData TC script execution"
                            , "milliseconds/run"
                            , "tc"
                            , NULL
                            , 135001
                            , localhost->rrd_update_every
                            , RRDSET_TYPE_AREA
                    );
                    rd_run_time = rrddim_add(sttime, "run_time",  "run time",  1, 1, RRD_ALGORITHM_ABSOLUTE);
                }
                else rrdset_next(sttime);

                rrddim_set_by_pointer(sttime, rd_run_time, str2ll(words[1], NULL));
                rrdset_done(sttime);

            }
#ifdef DETACH_PLUGINS_FROM_NETDATA
            else if(unlikely(first_hash == MYPID_HASH && (strcmp(words[0], "MYPID") == 0))) {
                // debug(D_TC_LOOP, "MYPID line '%s'", words[1]);
                char *id = words[1];
                pid_t pid = atol(id);

                if(likely(pid)) tc_child_pid = pid;

                debug(D_TC_LOOP, "TC: Child PID is %d.", tc_child_pid);
            }
#endif
            //else {
            //  debug(D_TC_LOOP, "IGNORED line");
            //}
        }

        // fgets() failed or loop broke
        int code = mypclose(fp, (pid_t)tc_child_pid);
        tc_child_pid = 0;

        if(unlikely(device)) {
            // tc_device_free(device);
            device = NULL;
            class = NULL;
        }

        if(unlikely(netdata_exit)) {
            tc_device_free_all();
            goto cleanup;
        }

        if(code == 1 || code == 127) {
            // 1 = DISABLE
            // 127 = cannot even run it
            error("TC: tc-qos-helper.sh exited with code %d. Disabling it.", code);

            tc_device_free_all();
            goto cleanup;
        }

        sleep((unsigned int) localhost->rrd_update_every);
    }

cleanup: ; // added semi-colon to prevent older gcc error: label at end of compound statement
    netdata_thread_cleanup_pop(1);
    return NULL;
}
