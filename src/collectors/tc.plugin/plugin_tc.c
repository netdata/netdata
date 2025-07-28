// SPDX-License-Identifier: GPL-3.0-or-later

#include "database/rrd.h"

#define RRD_TYPE_TC "tc"
#define PLUGIN_TC_NAME "tc.plugin"

// ----------------------------------------------------------------------------
// /sbin/tc processor
// this requires the script plugins.d/tc-qos-helper.sh

#define TC_LINE_MAX 1024

struct tc_class {
    STRING *id;
    STRING *name;
    STRING *leafid;
    STRING *parentid;

    bool hasparent;
    bool isleaf;
    bool isqdisc;
    bool render;
    bool name_updated;
    bool updated;

    int  unupdated; // the number of times, this has been found un-updated

    unsigned long long bytes;
    unsigned long long packets;
    unsigned long long dropped;
    unsigned long long tokens;
    unsigned long long ctokens;

    //unsigned long long overlimits;
    //unsigned long long requeues;
    //unsigned long long lended;
    //unsigned long long borrowed;
    //unsigned long long giants;

    RRDDIM *rd_bytes;
    RRDDIM *rd_packets;
    RRDDIM *rd_dropped;
    RRDDIM *rd_tokens;
    RRDDIM *rd_ctokens;
};

struct tc_device {
    STRING *id;
    STRING *name;
    STRING *family;

    bool name_updated;
    bool family_updated;

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

    DICTIONARY *classes;
};


// ----------------------------------------------------------------------------
// tc_class index

static void tc_class_free_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    // struct tc_device *d = data;
    struct tc_class *c = value;

    string_freez(c->id);
    string_freez(c->name);
    string_freez(c->leafid);
    string_freez(c->parentid);
}

static bool tc_class_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *old_value, void *new_value, void *data __maybe_unused) {
    struct tc_device *d = data; (void)d;
    struct tc_class *c = old_value; (void)c;
    struct tc_class *new_c = new_value; (void)new_c;

    collector_error("TC: class '%s' is already in device '%s'. Ignoring duplicate.", dictionary_acquired_item_name(item), string2str(d->id));

    tc_class_free_callback(item, new_value, data);

    return true;
}

static void tc_class_index_init(struct tc_device *d) {
    if(!d->classes) {
        d->classes = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_SINGLE_THREADED | DICT_OPTION_FIXED_SIZE, &dictionary_stats_category_collectors, sizeof(struct tc_class));

        dictionary_register_delete_callback(d->classes, tc_class_free_callback, d);
        dictionary_register_conflict_callback(d->classes, tc_class_conflict_callback, d);
    }
}

static void tc_class_index_destroy(struct tc_device *d) {
    dictionary_destroy(d->classes);
    d->classes = NULL;
}

static struct tc_class *tc_class_index_add(struct tc_device *d, struct tc_class *c) {
    return dictionary_set(d->classes, string2str(c->id), c, sizeof(struct tc_class));
}

static void tc_class_index_del(struct tc_device *d, struct tc_class *c) {
    dictionary_del(d->classes, string2str(c->id));
}

static inline struct tc_class *tc_class_index_find(struct tc_device *d, const char *id) {
    return dictionary_get(d->classes, id);
}

// ----------------------------------------------------------------------------
// tc_device index

static DICTIONARY *tc_device_root_index = NULL;

static void tc_device_add_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct tc_device *d = value;
    tc_class_index_init(d);
}

static void tc_device_free_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    struct tc_device *d = value;

    tc_class_index_destroy(d);

    string_freez(d->id);
    string_freez(d->name);
    string_freez(d->family);
}

static void tc_device_index_init() {
    if(!tc_device_root_index) {
        tc_device_root_index = dictionary_create_advanced(
            DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_SINGLE_THREADED | DICT_OPTION_ADD_IN_FRONT,
            &dictionary_stats_category_collectors, 0);

        dictionary_register_insert_callback(tc_device_root_index, tc_device_add_callback, NULL);
        dictionary_register_delete_callback(tc_device_root_index, tc_device_free_callback, NULL);
    }
}

static void tc_device_index_destroy() {
    dictionary_destroy(tc_device_root_index);
    tc_device_root_index = NULL;
}

static struct tc_device *tc_device_index_add(struct tc_device *d) {
    return dictionary_set(tc_device_root_index, string2str(d->id), d, sizeof(*d));
}

//static struct tc_device *tc_device_index_del(struct tc_device *d) {
//    dictionary_del(tc_device_root_index, string2str(d->id));
//    return d;
//}

static inline struct tc_device *tc_device_index_find(const char *id) {
    return dictionary_get(tc_device_root_index, id);
}

// ----------------------------------------------------------------------------

static inline void tc_class_free(struct tc_device *n, struct tc_class *c) {
    netdata_log_debug(D_TC_LOOP, "Removing from device '%s' class '%s', parentid '%s', leafid '%s', unused=%d",
          string2str(n->id), string2str(c->id), string2str(c->parentid), string2str(c->leafid),
          c->unupdated);

    tc_class_index_del(n, c);
}

static inline void tc_device_classes_cleanup(struct tc_device *d) {
    static int cleanup_every = 999;

    if(unlikely(cleanup_every > 0)) {
        cleanup_every = (int) inicfg_get_number(&netdata_config, "plugin:tc", "cleanup unused classes every", 120);
        if(cleanup_every < 0) cleanup_every = -cleanup_every;
    }

    d->name_updated = false;
    d->family_updated = false;

    struct tc_class *c;
    dfe_start_write(d->classes, c) {
        if(unlikely(cleanup_every && c->unupdated >= cleanup_every))
            tc_class_free(d, c);

        else {
            c->updated = false;
            c->name_updated = false;
        }
    }
    dfe_done(c);
}

static inline void tc_device_commit(struct tc_device *d) {
    static int enable_tokens = -1, enable_ctokens = -1, enabled_all_classes_qdiscs = -1;

    if(unlikely(enabled_all_classes_qdiscs == -1)) {
        enable_tokens              = inicfg_get_boolean_ondemand(&netdata_config, "plugin:tc", "enable tokens charts for all interfaces", CONFIG_BOOLEAN_NO);
        enable_ctokens             = inicfg_get_boolean_ondemand(&netdata_config, "plugin:tc", "enable ctokens charts for all interfaces", CONFIG_BOOLEAN_NO);
        enabled_all_classes_qdiscs = inicfg_get_boolean_ondemand(&netdata_config, "plugin:tc", "enable show all classes and qdiscs for all interfaces", CONFIG_BOOLEAN_NO);
    }

    if(unlikely(d->enabled == (char)-1)) {
        d->enabled = CONFIG_BOOLEAN_YES;
        d->enabled_bytes = CONFIG_BOOLEAN_YES;
        d->enabled_packets = CONFIG_BOOLEAN_YES;
        d->enabled_dropped = CONFIG_BOOLEAN_YES;
        d->enabled_tokens = enable_tokens;
        d->enabled_ctokens = enable_ctokens;
        d->enabled_all_classes_qdiscs = enabled_all_classes_qdiscs;


        char var_name[CONFIG_MAX_NAME + 1];

        snprintfz(var_name, CONFIG_MAX_NAME, "qos for %s", string2str(d->id));
        if (inicfg_exists(&netdata_config, "plugin:tc", var_name))
            d->enabled = (char)inicfg_get_boolean_ondemand(&netdata_config, "plugin:tc", var_name, d->enabled);

        snprintfz(var_name, CONFIG_MAX_NAME, "traffic chart for %s", string2str(d->id));
        if (inicfg_exists(&netdata_config, "plugin:tc", var_name))
            d->enabled_bytes = (char)inicfg_get_boolean_ondemand(&netdata_config, "plugin:tc", var_name, d->enabled_bytes);

        snprintfz(var_name, CONFIG_MAX_NAME, "packets chart for %s", string2str(d->id));
        if (inicfg_exists(&netdata_config, "plugin:tc", var_name))
            d->enabled_packets = (char)inicfg_get_boolean_ondemand(&netdata_config, "plugin:tc", var_name, d->enabled_packets);

        snprintfz(var_name, CONFIG_MAX_NAME, "dropped packets chart for %s", string2str(d->id));
        if (inicfg_exists(&netdata_config, "plugin:tc", var_name))
            d->enabled_dropped = (char)inicfg_get_boolean_ondemand(&netdata_config, "plugin:tc", var_name, d->enabled_dropped);

        snprintfz(var_name, CONFIG_MAX_NAME, "tokens chart for %s", string2str(d->id));
        if (inicfg_exists(&netdata_config, "plugin:tc", var_name))
            d->enabled_tokens = (char)inicfg_get_boolean_ondemand(&netdata_config, "plugin:tc", var_name, d->enabled_tokens);

        snprintfz(var_name, CONFIG_MAX_NAME, "ctokens chart for %s", string2str(d->id));
        if (inicfg_exists(&netdata_config, "plugin:tc", var_name))
            d->enabled_ctokens = (char)inicfg_get_boolean_ondemand(&netdata_config, "plugin:tc", var_name, d->enabled_ctokens);

        snprintfz(var_name, CONFIG_MAX_NAME, "show all classes for %s", string2str(d->id));
        if (inicfg_exists(&netdata_config, "plugin:tc", var_name))
            d->enabled_all_classes_qdiscs =
                (char)inicfg_get_boolean_ondemand(&netdata_config, "plugin:tc", var_name, d->enabled_all_classes_qdiscs);
    }

    // we only need to add leaf classes
    struct tc_class *c, *x /*, *root = NULL */;
    unsigned long long bytes_sum = 0, packets_sum = 0, dropped_sum = 0, tokens_sum = 0, ctokens_sum = 0;
    int active_nodes = 0, updated_classes = 0, updated_qdiscs = 0;

    // prepare all classes
    // we set reasonable defaults for the rest of the code below

    dfe_start_read(d->classes, c) {
        c->render = false;      // do not render this class
        c->isleaf = true;       // this is a leaf class
        c->hasparent = false;   // without a parent

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
    dfe_done(c);

    if(unlikely(!d->enabled || (!updated_classes && !updated_qdiscs))) {
        netdata_log_debug(D_TC_LOOP, "TC: Ignoring TC device '%s'. It is not enabled/updated.", string2str(d->name?d->name:d->id));
        tc_device_classes_cleanup(d);
        return;
    }

    if(unlikely(updated_classes && updated_qdiscs)) {
        collector_error("TC: device '%s' has active both classes (%d) and qdiscs (%d). Will render only qdiscs.", string2str(d->id), updated_classes, updated_qdiscs);

        // set all classes to !updated
        dfe_start_read(d->classes, c) {
            if (unlikely(!c->isqdisc && c->updated))
                c->updated = false;
        }
        dfe_done(c);
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
        dfe_start_read(d->classes, c) {
            if(unlikely(!c->updated))
                continue;

            //netdata_log_debug(D_TC_LOOP, "TC: In device '%s', %s '%s'  has leafid: '%s' and parentid '%s'.",
            //    d->id,
            //    c->isqdisc?"qdisc":"class",
            //    c->id,
            //    c->leafid?c->leafid:"NULL",
            //    c->parentid?c->parentid:"NULL");

            // find if c is leaf or not
            dfe_start_read(d->classes, x) {
                if(unlikely(!x->updated || c == x || !x->parentid))
                    continue;

                // classes have both parentid and leafid
                // qdiscs have only parentid
                // the following works for both (it is an OR)

                if((x->parentid && c->id == x->parentid) ||
                   (c->leafid && x->parentid && c->leafid == x->parentid)) {
                    // netdata_log_debug(D_TC_LOOP, "TC: In device '%s', %s '%s' (leafid: '%s') has as leaf %s '%s' (parentid: '%s').", d->name?d->name:d->id, c->isqdisc?"qdisc":"class", c->name?c->name:c->id, c->leafid?c->leafid:c->id, x->isqdisc?"qdisc":"class", x->name?x->name:x->id, x->parentid?x->parentid:x->id);
                    c->isleaf = false;
                    x->hasparent = true;
                }
            }
            dfe_done(x);
        }
        dfe_done(c);
    }

    dfe_start_read(d->classes, c) {
        if(unlikely(!c->updated))
            continue;

        // netdata_log_debug(D_TC_LOOP, "TC: device '%s', %s '%s' isleaf=%d, hasparent=%d", d->id, (c->isqdisc)?"qdisc":"class", c->id, c->isleaf, c->hasparent);

        if(unlikely((c->isleaf && c->hasparent) || d->enabled_all_classes_qdiscs)) {
            c->render = true;
            active_nodes++;
            bytes_sum += c->bytes;
            packets_sum += c->packets;
            dropped_sum += c->dropped;
            tokens_sum += c->tokens;
            ctokens_sum += c->ctokens;
        }

        //if(unlikely(!c->hasparent)) {
        //    if(root) collector_error("TC: multiple root class/qdisc for device '%s' (old: '%s', new: '%s')", d->id, root->id, c->id);
        //    root = c;
        //    netdata_log_debug(D_TC_LOOP, "TC: found root class/qdisc '%s'", root->id);
        //}
    }
    dfe_done(c);

#ifdef NETDATA_INTERNAL_CHECKS
    // dump all the list to see what we know

    if(unlikely(debug_flags & D_TC_LOOP)) {
        dfe_start_read(d->classes, c) {
            if(c->render) netdata_log_debug(D_TC_LOOP, "TC: final nodes dump for '%s': class %s, OK", string2str(d->name), string2str(c->id));
            else netdata_log_debug(D_TC_LOOP, "TC: final nodes dump for '%s': class '%s', IGNORE (updated: %d, isleaf: %d, hasparent: %d, parent: '%s')",
                      string2str(d->name?d->name:d->id), string2str(c->id), c->updated, c->isleaf, c->hasparent, string2str(c->parentid));
        }
        dfe_done(c);
    }
#endif

    if(unlikely(!active_nodes)) {
        netdata_log_debug(D_TC_LOOP, "TC: Ignoring TC device '%s'. No useful classes/qdiscs.", string2str(d->name?d->name:d->id));
        tc_device_classes_cleanup(d);
        return;
    }

    // --------------------------------------------------------------------
    // bytes

    if (d->enabled_bytes == CONFIG_BOOLEAN_YES || d->enabled_bytes == CONFIG_BOOLEAN_AUTO) {
        d->enabled_bytes = CONFIG_BOOLEAN_YES;

        if(unlikely(!d->st_bytes)) {
            d->st_bytes = rrdset_create_localhost(
                RRD_TYPE_TC,
                string2str(d->id),
                string2str(d->name ? d->name : d->id),
                string2str(d->family ? d->family : d->id),
                RRD_TYPE_TC ".qos",
                "Class Usage",
                "kilobits/s",
                PLUGIN_TC_NAME,
                NULL,
                NETDATA_CHART_PRIO_TC_QOS,
                localhost->rrd_update_every,
                d->enabled_all_classes_qdiscs ? RRDSET_TYPE_LINE : RRDSET_TYPE_STACKED);

            rrdlabels_add(d->st_bytes->rrdlabels, "device", string2str(d->id), RRDLABEL_SRC_AUTO);
            rrdlabels_add(d->st_bytes->rrdlabels, "device_name", string2str(d->name?d->name:d->id), RRDLABEL_SRC_AUTO);
            rrdlabels_add(d->st_bytes->rrdlabels, "device_group", string2str(d->family?d->family:d->id), RRDLABEL_SRC_AUTO);
        }
        else {
            if(unlikely(d->name_updated))
                rrdset_reset_name(d->st_bytes, string2str(d->name));

            if(d->name && d->name_updated)
                rrdlabels_add(d->st_bytes->rrdlabels, "device_name", string2str(d->name), RRDLABEL_SRC_AUTO);

            if(d->family && d->family_updated)
                rrdlabels_add(d->st_bytes->rrdlabels, "device_group", string2str(d->family), RRDLABEL_SRC_AUTO);

            // TODO
            // update the family
        }

        dfe_start_read(d->classes, c) {
            if(unlikely(!c->render)) continue;

            if(unlikely(!c->rd_bytes))
                c->rd_bytes = rrddim_add(d->st_bytes, string2str(c->id), string2str(c->name?c->name:c->id), 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            else if(unlikely(c->name_updated))
                rrddim_reset_name(d->st_bytes, c->rd_bytes, string2str(c->name));

            rrddim_set_by_pointer(d->st_bytes, c->rd_bytes, c->bytes);
        }
        dfe_done(c);

        rrdset_done(d->st_bytes);
    }

    // --------------------------------------------------------------------
    // packets

    if (d->enabled_packets == CONFIG_BOOLEAN_YES || d->enabled_packets == CONFIG_BOOLEAN_AUTO) {
        d->enabled_packets = CONFIG_BOOLEAN_YES;

        if(unlikely(!d->st_packets)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_packets", string2str(d->id));
            snprintfz(name, RRD_ID_LENGTH_MAX, "%s_packets", string2str(d->name ? d->name : d->id));

            d->st_packets = rrdset_create_localhost(
                RRD_TYPE_TC,
                id,
                name,
                string2str(d->family ? d->family : d->id),
                RRD_TYPE_TC ".qos_packets",
                "Class Packets",
                "packets/s",
                PLUGIN_TC_NAME,
                NULL,
                NETDATA_CHART_PRIO_TC_QOS_PACKETS,
                localhost->rrd_update_every,
                d->enabled_all_classes_qdiscs ? RRDSET_TYPE_LINE : RRDSET_TYPE_STACKED);

            rrdlabels_add(d->st_packets->rrdlabels, "device", string2str(d->id), RRDLABEL_SRC_AUTO);
            rrdlabels_add(d->st_packets->rrdlabels, "device_name", string2str(d->name?d->name:d->id), RRDLABEL_SRC_AUTO);
            rrdlabels_add(d->st_packets->rrdlabels, "device_group", string2str(d->family?d->family:d->id), RRDLABEL_SRC_AUTO);
        }
        else {
            if(unlikely(d->name_updated)) {
                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "%s_packets", string2str(d->name?d->name:d->id));
                rrdset_reset_name(d->st_packets, name);
            }

            if(d->name && d->name_updated)
                rrdlabels_add(d->st_packets->rrdlabels, "device_name", string2str(d->name), RRDLABEL_SRC_AUTO);

            if(d->family && d->family_updated)
                rrdlabels_add(d->st_packets->rrdlabels, "device_group", string2str(d->family), RRDLABEL_SRC_AUTO);

            // TODO
            // update the family
        }

        dfe_start_read(d->classes, c) {
            if(unlikely(!c->render)) continue;

            if(unlikely(!c->rd_packets))
                c->rd_packets = rrddim_add(d->st_packets, string2str(c->id), string2str(c->name?c->name:c->id), 1, 1, RRD_ALGORITHM_INCREMENTAL);
            else if(unlikely(c->name_updated))
                rrddim_reset_name(d->st_packets, c->rd_packets, string2str(c->name));

            rrddim_set_by_pointer(d->st_packets, c->rd_packets, c->packets);
        }
        dfe_done(c);

        rrdset_done(d->st_packets);
    }

    // --------------------------------------------------------------------
    // dropped

    if (d->enabled_dropped == CONFIG_BOOLEAN_YES || d->enabled_dropped == CONFIG_BOOLEAN_AUTO) {
        d->enabled_dropped = CONFIG_BOOLEAN_YES;

        if(unlikely(!d->st_dropped)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_dropped", string2str(d->id));
            snprintfz(name, RRD_ID_LENGTH_MAX, "%s_dropped", string2str(d->name ? d->name : d->id));

            d->st_dropped = rrdset_create_localhost(
                RRD_TYPE_TC,
                id,
                name,
                string2str(d->family ? d->family : d->id),
                RRD_TYPE_TC ".qos_dropped",
                "Class Dropped Packets",
                "packets/s",
                PLUGIN_TC_NAME,
                NULL,
                NETDATA_CHART_PRIO_TC_QOS_DROPPED,
                localhost->rrd_update_every,
                d->enabled_all_classes_qdiscs ? RRDSET_TYPE_LINE : RRDSET_TYPE_STACKED);

            rrdlabels_add(d->st_dropped->rrdlabels, "device", string2str(d->id), RRDLABEL_SRC_AUTO);
            rrdlabels_add(d->st_dropped->rrdlabels, "device_name", string2str(d->name?d->name:d->id), RRDLABEL_SRC_AUTO);
            rrdlabels_add(d->st_dropped->rrdlabels, "device_group", string2str(d->family?d->family:d->id), RRDLABEL_SRC_AUTO);
        }
        else {
            if(unlikely(d->name_updated)) {
                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "%s_dropped", string2str(d->name?d->name:d->id));
                rrdset_reset_name(d->st_dropped, name);
            }

            if(d->name && d->name_updated)
                rrdlabels_add(d->st_dropped->rrdlabels, "device_name", string2str(d->name), RRDLABEL_SRC_AUTO);

            if(d->family && d->family_updated)
                rrdlabels_add(d->st_dropped->rrdlabels, "device_group", string2str(d->family), RRDLABEL_SRC_AUTO);

            // TODO
            // update the family
        }

        dfe_start_read(d->classes, c) {
            if(unlikely(!c->render)) continue;

            if(unlikely(!c->rd_dropped))
                c->rd_dropped = rrddim_add(d->st_dropped, string2str(c->id), string2str(c->name?c->name:c->id), 1, 1, RRD_ALGORITHM_INCREMENTAL);
            else if(unlikely(c->name_updated))
                rrddim_reset_name(d->st_dropped, c->rd_dropped, string2str(c->name));

            rrddim_set_by_pointer(d->st_dropped, c->rd_dropped, c->dropped);
        }
        dfe_done(c);

        rrdset_done(d->st_dropped);
    }

    // --------------------------------------------------------------------
    // tokens

    if (d->enabled_tokens == CONFIG_BOOLEAN_YES || d->enabled_tokens == CONFIG_BOOLEAN_AUTO) {
        d->enabled_tokens = CONFIG_BOOLEAN_YES;

        if(unlikely(!d->st_tokens)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_tokens", string2str(d->id));
            snprintfz(name, RRD_ID_LENGTH_MAX, "%s_tokens", string2str(d->name ? d->name : d->id));

            d->st_tokens = rrdset_create_localhost(
                RRD_TYPE_TC,
                id,
                name,
                string2str(d->family ? d->family : d->id),
                RRD_TYPE_TC ".qos_tokens",
                "Class Tokens",
                "tokens",
                PLUGIN_TC_NAME,
                NULL,
                NETDATA_CHART_PRIO_TC_QOS_TOKENS,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rrdlabels_add(d->st_tokens->rrdlabels, "device", string2str(d->id), RRDLABEL_SRC_AUTO);
            rrdlabels_add(d->st_tokens->rrdlabels, "device_name", string2str(d->name?d->name:d->id), RRDLABEL_SRC_AUTO);
            rrdlabels_add(d->st_tokens->rrdlabels, "device_group", string2str(d->family?d->family:d->id), RRDLABEL_SRC_AUTO);
        }
        else {
            if(unlikely(d->name_updated)) {
                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "%s_tokens", string2str(d->name?d->name:d->id));
                rrdset_reset_name(d->st_tokens, name);
            }

            if(d->name && d->name_updated)
                rrdlabels_add(d->st_tokens->rrdlabels, "device_name", string2str(d->name), RRDLABEL_SRC_AUTO);

            if(d->family && d->family_updated)
                rrdlabels_add(d->st_tokens->rrdlabels, "device_group", string2str(d->family), RRDLABEL_SRC_AUTO);

            // TODO
            // update the family
        }

        dfe_start_read(d->classes, c) {
            if(unlikely(!c->render)) continue;

            if(unlikely(!c->rd_tokens)) {
                c->rd_tokens = rrddim_add(d->st_tokens, string2str(c->id), string2str(c->name?c->name:c->id), 1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else if(unlikely(c->name_updated))
                rrddim_reset_name(d->st_tokens, c->rd_tokens, string2str(c->name));

            rrddim_set_by_pointer(d->st_tokens, c->rd_tokens, c->tokens);
        }
        dfe_done(c);

        rrdset_done(d->st_tokens);
    }

    // --------------------------------------------------------------------
    // ctokens

    if (d->enabled_ctokens == CONFIG_BOOLEAN_YES || d->enabled_ctokens == CONFIG_BOOLEAN_AUTO) {
        d->enabled_ctokens = CONFIG_BOOLEAN_YES;

        if(unlikely(!d->st_ctokens)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            char name[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s_ctokens", string2str(d->id));
            snprintfz(name, RRD_ID_LENGTH_MAX, "%s_ctokens", string2str(d->name ? d->name : d->id));

            d->st_ctokens = rrdset_create_localhost(
                RRD_TYPE_TC,
                id,
                name,
                string2str(d->family ? d->family : d->id),
                RRD_TYPE_TC ".qos_ctokens",
                "Class cTokens",
                "ctokens",
                PLUGIN_TC_NAME,
                NULL,
                NETDATA_CHART_PRIO_TC_QOS_CTOKENS,
                localhost->rrd_update_every,
                RRDSET_TYPE_LINE);

            rrdlabels_add(d->st_ctokens->rrdlabels, "device", string2str(d->id), RRDLABEL_SRC_AUTO);
            rrdlabels_add(d->st_ctokens->rrdlabels, "device_name", string2str(d->name?d->name:d->id), RRDLABEL_SRC_AUTO);
            rrdlabels_add(d->st_ctokens->rrdlabels, "device_group", string2str(d->family?d->family:d->id), RRDLABEL_SRC_AUTO);
        }
        else {
            netdata_log_debug(D_TC_LOOP, "TC: Updating _ctokens chart for device '%s'", string2str(d->name?d->name:d->id));

            if(unlikely(d->name_updated)) {
                char name[RRD_ID_LENGTH_MAX + 1];
                snprintfz(name, RRD_ID_LENGTH_MAX, "%s_ctokens", string2str(d->name?d->name:d->id));
                rrdset_reset_name(d->st_ctokens, name);
            }

            if(d->name && d->name_updated)
                rrdlabels_add(d->st_ctokens->rrdlabels, "device_name", string2str(d->name), RRDLABEL_SRC_AUTO);

            if(d->family && d->family_updated)
                rrdlabels_add(d->st_ctokens->rrdlabels, "device_group", string2str(d->family), RRDLABEL_SRC_AUTO);

            // TODO
            // update the family
        }

        dfe_start_read(d->classes, c) {
            if(unlikely(!c->render)) continue;

            if(unlikely(!c->rd_ctokens))
                c->rd_ctokens = rrddim_add(d->st_ctokens, string2str(c->id), string2str(c->name?c->name:c->id), 1, 1, RRD_ALGORITHM_ABSOLUTE);
            else if(unlikely(c->name_updated))
                rrddim_reset_name(d->st_ctokens, c->rd_ctokens, string2str(c->name));

            rrddim_set_by_pointer(d->st_ctokens, c->rd_ctokens, c->ctokens);
        }
        dfe_done(c);

        rrdset_done(d->st_ctokens);
    }

    tc_device_classes_cleanup(d);
}

static inline void tc_device_set_class_name(struct tc_device *d, char *id, char *name) {
    if(unlikely(!name || !*name)) return;

    struct tc_class *c = tc_class_index_find(d, id);
    if(likely(c)) {
        if(likely(c->name)) {
            if(!strcmp(string2str(c->name), name)) return;
            string_freez(c->name);
            c->name = NULL;
        }

        if(likely(name && *name && strcmp(string2str(c->id), name) != 0)) {
            netdata_log_debug(D_TC_LOOP, "TC: Setting device '%s', class '%s' name to '%s'", string2str(d->id), id, name);
            c->name = string_strdupz(name);
            c->name_updated = true;
        }
    }
}

static inline void tc_device_set_device_name(struct tc_device *d, char *name) {
    if(unlikely(!name || !*name)) return;

    if(d->name) {
        if(!strcmp(string2str(d->name), name)) return;
        string_freez(d->name);
        d->name = NULL;
    }

    if(likely(name && *name && strcmp(string2str(d->id), name) != 0)) {
        netdata_log_debug(D_TC_LOOP, "TC: Setting device '%s' name to '%s'", string2str(d->id), name);
        d->name = string_strdupz(name);
        d->name_updated = true;
    }
}

static inline void tc_device_set_device_family(struct tc_device *d, char *family) {
    string_freez(d->family);
    d->family = NULL;

    if(likely(family && *family && strcmp(string2str(d->id), family) != 0)) {
        netdata_log_debug(D_TC_LOOP, "TC: Setting device '%s' family to '%s'", string2str(d->id), family);
        d->family = string_strdupz(family);
        d->family_updated = true;
    }
    // no need for null termination - it is already null
}

static inline struct tc_device *tc_device_create(char *id) {
    struct tc_device *d = tc_device_index_find(id);

    if(!d) {
        netdata_log_debug(D_TC_LOOP, "TC: Creating device '%s'", id);

        struct tc_device tmp = {
            .id = string_strdupz(id),
            .enabled = (char)-1,
        };
        d = tc_device_index_add(&tmp);
    }

    return(d);
}

static inline struct tc_class *tc_class_add(struct tc_device *n, char *id, bool qdisc, char *parentid, char *leafid) {
    struct tc_class *c = tc_class_index_find(n, id);

    if(!c) {
        netdata_log_debug(D_TC_LOOP, "TC: Creating in device '%s', class id '%s', parentid '%s', leafid '%s'",
              string2str(n->id), id, parentid?parentid:"", leafid?leafid:"");

        struct tc_class tmp = {
            .id = string_strdupz(id),
            .isqdisc = qdisc,
            .parentid = string_strdupz(parentid),
            .leafid = string_strdupz(leafid),
        };

        tc_class_index_add(n, &tmp);
    }
    return(c);
}

//static inline void tc_device_free(struct tc_device *d) {
//    tc_device_index_del(d);
//}

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

static POPEN_INSTANCE *tc_child_instance = NULL;

static void tc_main_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    worker_unregister();
    tc_device_index_destroy();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    if(tc_child_instance) {
        collector_info("TC: stopping the running tc-qos-helper script");
        int code = spawn_popen_wait(tc_child_instance); (void)code;
        tc_child_instance = NULL;
    }

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

#define WORKER_TC_CLASS          0
#define WORKER_TC_BEGIN          1
#define WORKER_TC_END            2
#define WORKER_TC_SENT           3
#define WORKER_TC_LENDED         4
#define WORKER_TC_TOKENS         5
#define WORKER_TC_SETDEVICENAME  6
#define WORKER_TC_SETDEVICEGROUP 7
#define WORKER_TC_SETCLASSNAME   8
#define WORKER_TC_WORKTIME       9
#define WORKER_TC_PLUGIN_TIME   10
#define WORKER_TC_DEVICES       11
#define WORKER_TC_CLASSES       12

#if WORKER_UTILIZATION_MAX_JOB_TYPES < 13
#error WORKER_UTILIZATION_MAX_JOB_TYPES has to be at least 10
#endif

void tc_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(tc_main_cleanup) cleanup_ptr = ptr;

    worker_register("TC");
    worker_register_job_name(WORKER_TC_CLASS, "class");
    worker_register_job_name(WORKER_TC_BEGIN, "begin");
    worker_register_job_name(WORKER_TC_END, "end");
    worker_register_job_name(WORKER_TC_SENT, "sent");
    worker_register_job_name(WORKER_TC_LENDED, "lended");
    worker_register_job_name(WORKER_TC_TOKENS, "tokens");
    worker_register_job_name(WORKER_TC_SETDEVICENAME, "devicename");
    worker_register_job_name(WORKER_TC_SETDEVICEGROUP, "devicegroup");
    worker_register_job_name(WORKER_TC_SETCLASSNAME, "classname");
    worker_register_job_name(WORKER_TC_WORKTIME, "worktime");

    worker_register_job_custom_metric(WORKER_TC_PLUGIN_TIME, "tc script execution time", "milliseconds/run", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_TC_DEVICES, "number of devices", "devices", WORKER_METRIC_ABSOLUTE);
    worker_register_job_custom_metric(WORKER_TC_CLASSES, "number of classes", "classes", WORKER_METRIC_ABSOLUTE);

    tc_device_index_init();

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
    uint32_t first_hash;

    snprintfz(command, TC_LINE_MAX, "%s/tc-qos-helper.sh", netdata_configured_primary_plugins_dir);
    const char *tc_script = inicfg_get(&netdata_config, "plugin:tc", "script to run to get tc values", command);

    while(service_running(SERVICE_COLLECTORS)) {
        struct tc_device *device = NULL;
        struct tc_class *class = NULL;

        snprintfz(command, TC_LINE_MAX, "exec %s %d", tc_script, localhost->rrd_update_every);
        netdata_log_debug(D_TC_LOOP, "executing '%s'", command);

        tc_child_instance = spawn_popen_run(command);
        if(!tc_child_instance) {
            collector_error("TC: Cannot popen(\"%s\", \"r\").", command);
            return;
        }

        char buffer[TC_LINE_MAX+1] = "";
        while(fgets(buffer, TC_LINE_MAX, spawn_popen_stdout(tc_child_instance)) != NULL) {
            if(unlikely(!service_running(SERVICE_COLLECTORS))) break;

            buffer[TC_LINE_MAX] = '\0';
            // netdata_log_debug(D_TC_LOOP, "TC: read '%s'", buffer);

            tc_split_words(buffer, words, PLUGINSD_MAX_WORDS);

            if(unlikely(!words[0] || !*words[0])) {
                // netdata_log_debug(D_TC_LOOP, "empty line");
                worker_is_idle();
                continue;
            }
            // else netdata_log_debug(D_TC_LOOP, "First word is '%s'", words[0]);

            first_hash = simple_hash(words[0]);

            if(unlikely(device && ((first_hash == CLASS_HASH && strcmp(words[0], "class") == 0) ||  (first_hash == QDISC_HASH && strcmp(words[0], "qdisc") == 0)))) {
                worker_is_busy(WORKER_TC_CLASS);

                // netdata_log_debug(D_TC_LOOP, "CLASS line on class id='%s', parent='%s', parentid='%s', leaf='%s', leafid='%s'", words[2], words[3], words[4], words[5], words[6]);

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
                    bool qdisc = false;

                    if(first_hash == QDISC_HASH) {
                        qdisc = true;

                        if(!strcmp(type, "ingress")) {
                            // we don't want to get the ingress qdisc
                            // there should be an IFB interface for this

                            class = NULL;
                            worker_is_idle();
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
                worker_is_busy(WORKER_TC_END);

                // netdata_log_debug(D_TC_LOOP, "END line");

                if(likely(device)) {
                    tc_device_commit(device);
                    // tc_device_free(device);
                }

                device = NULL;
                class = NULL;
            }
            else if(unlikely(first_hash == BEGIN_HASH && strcmp(words[0], "BEGIN") == 0)) {
                worker_is_busy(WORKER_TC_BEGIN);

                // netdata_log_debug(D_TC_LOOP, "BEGIN line on device '%s'", words[1]);

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
                worker_is_busy(WORKER_TC_SENT);

                // netdata_log_debug(D_TC_LOOP, "SENT line '%s'", words[1]);
                if(likely(words[1] && *words[1])) {
                    class->bytes = str2ull(words[1], NULL);
                    class->updated = true;
                }
                else {
                    class->updated = false;
                }

                if(likely(words[3] && *words[3]))
                    class->packets = str2ull(words[3], NULL);

                if(likely(words[6] && *words[6]))
                    class->dropped = str2ull(words[6], NULL);

                //if(likely(words[8] && *words[8]))
                //    class->overlimits = str2ull(words[8]);

                //if(likely(words[10] && *words[10]))
                //    class->requeues = str2ull(words[8]);
            }
            else if(unlikely(device && class && class->updated && first_hash == LENDED_HASH && strcmp(words[0], "lended:") == 0)) {
                worker_is_busy(WORKER_TC_LENDED);

                // netdata_log_debug(D_TC_LOOP, "LENDED line '%s'", words[1]);
                //if(likely(words[1] && *words[1]))
                //    class->lended = str2ull(words[1]);

                //if(likely(words[3] && *words[3]))
                //    class->borrowed = str2ull(words[3]);

                //if(likely(words[5] && *words[5]))
                //    class->giants = str2ull(words[5]);
            }
            else if(unlikely(device && class && class->updated && first_hash == TOKENS_HASH && strcmp(words[0], "tokens:") == 0)) {
                worker_is_busy(WORKER_TC_TOKENS);

                // netdata_log_debug(D_TC_LOOP, "TOKENS line '%s'", words[1]);
                if(likely(words[1] && *words[1]))
                    class->tokens = str2ull(words[1], NULL);

                if(likely(words[3] && *words[3]))
                    class->ctokens = str2ull(words[3], NULL);
            }
            else if(unlikely(device && first_hash == SETDEVICENAME_HASH && strcmp(words[0], "SETDEVICENAME") == 0)) {
                worker_is_busy(WORKER_TC_SETDEVICENAME);

                // netdata_log_debug(D_TC_LOOP, "SETDEVICENAME line '%s'", words[1]);
                if(likely(words[1] && *words[1]))
                    tc_device_set_device_name(device, words[1]);
            }
            else if(unlikely(device && first_hash == SETDEVICEGROUP_HASH && strcmp(words[0], "SETDEVICEGROUP") == 0)) {
                worker_is_busy(WORKER_TC_SETDEVICEGROUP);

                // netdata_log_debug(D_TC_LOOP, "SETDEVICEGROUP line '%s'", words[1]);
                if(likely(words[1] && *words[1]))
                    tc_device_set_device_family(device, words[1]);
            }
            else if(unlikely(device && first_hash == SETCLASSNAME_HASH && strcmp(words[0], "SETCLASSNAME") == 0)) {
                worker_is_busy(WORKER_TC_SETCLASSNAME);

                // netdata_log_debug(D_TC_LOOP, "SETCLASSNAME line '%s' '%s'", words[1], words[2]);
                char *id    = words[1];
                char *path  = words[2];
                if(likely(id && *id && path && *path))
                    tc_device_set_class_name(device, id, path);
            }
            else if(unlikely(first_hash == WORKTIME_HASH && strcmp(words[0], "WORKTIME") == 0)) {
                worker_is_busy(WORKER_TC_WORKTIME);
                worker_set_metric(WORKER_TC_PLUGIN_TIME, str2ll(words[1], NULL));

                size_t number_of_devices = dictionary_entries(tc_device_root_index);
                size_t number_of_classes = 0;

                struct tc_device *d;
                dfe_start_read(tc_device_root_index, d) {
                    number_of_classes += dictionary_entries(d->classes);
                }
                dfe_done(d);

                worker_set_metric(WORKER_TC_DEVICES, number_of_devices);
                worker_set_metric(WORKER_TC_CLASSES, number_of_classes);
            }
            //else {
            //  netdata_log_debug(D_TC_LOOP, "IGNORED line");
            //}

            worker_is_idle();
        }

        // fgets() failed or loop broke
        int code = spawn_popen_kill(tc_child_instance, 0);
        tc_child_instance = NULL;

        if(unlikely(device)) {
            // tc_device_free(device);
            device = NULL;
            class = NULL;
        }

        if(unlikely(!service_running(SERVICE_COLLECTORS)))
            return;

        if(code == 1 || code == 127) {
            // 1 = DISABLE
            // 127 = cannot even run it
            collector_error("TC: tc-qos-helper.sh exited with code %d. Disabling it.", code);
            return;
        }

        sleep((unsigned int) localhost->rrd_update_every);
    };
}
