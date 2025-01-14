// SPDX-License-Identifier: GPL-3.0-or-later

#define PULSE_INTERNALS 1
#include "pulse-dictionary.h"

struct dictionary_stats dictionary_stats_category_collectors = { .name = "collectors" };
struct dictionary_stats dictionary_stats_category_rrdhost = { .name = "rrdhost" };
struct dictionary_stats dictionary_stats_category_rrdset = { .name = "rrdset" };
struct dictionary_stats dictionary_stats_category_rrddim = { .name = "rrddim" };
struct dictionary_stats dictionary_stats_category_rrdcontext = { .name = "context" };
struct dictionary_stats dictionary_stats_category_rrdlabels = { .name = "labels" };
struct dictionary_stats dictionary_stats_category_rrdhealth = { .name = "health" };
struct dictionary_stats dictionary_stats_category_functions = { .name = "functions" };
struct dictionary_stats dictionary_stats_category_replication = { .name = "replication" };
struct dictionary_stats dictionary_stats_category_dyncfg = { .name = "dyncfg" };

#ifdef DICT_WITH_STATS
struct dictionary_categories {
    struct dictionary_stats *stats;

    RRDSET *st_dicts;
    RRDDIM *rd_dicts_active;
    RRDDIM *rd_dicts_deleted;

    RRDSET *st_items;
    RRDDIM *rd_items_entries;
    RRDDIM *rd_items_referenced;
    RRDDIM *rd_items_pending_deletion;

    RRDSET *st_ops;
    RRDDIM *rd_ops_creations;
    RRDDIM *rd_ops_destructions;
    RRDDIM *rd_ops_flushes;
    RRDDIM *rd_ops_traversals;
    RRDDIM *rd_ops_walkthroughs;
    RRDDIM *rd_ops_garbage_collections;
    RRDDIM *rd_ops_searches;
    RRDDIM *rd_ops_inserts;
    RRDDIM *rd_ops_resets;
    RRDDIM *rd_ops_deletes;

    RRDSET *st_callbacks;
    RRDDIM *rd_callbacks_inserts;
    RRDDIM *rd_callbacks_conflicts;
    RRDDIM *rd_callbacks_reacts;
    RRDDIM *rd_callbacks_deletes;

    RRDSET *st_memory;
    RRDDIM *rd_memory_indexed;
    RRDDIM *rd_memory_values;
    RRDDIM *rd_memory_dict;

    RRDSET *st_spins;
    RRDDIM *rd_spins_use;
    RRDDIM *rd_spins_search;
    RRDDIM *rd_spins_insert;
    RRDDIM *rd_spins_delete;

} dictionary_categories[] = {
    { .stats = &dictionary_stats_category_collectors, },
    { .stats = &dictionary_stats_category_rrdhost, },
    { .stats = &dictionary_stats_category_rrdset, },
    { .stats = &dictionary_stats_category_rrdcontext, },
    { .stats = &dictionary_stats_category_rrdlabels, },
    { .stats = &dictionary_stats_category_rrdhealth, },
    { .stats = &dictionary_stats_category_functions, },
    { .stats = &dictionary_stats_category_replication, },
    { .stats = &dictionary_stats_category_dyncfg, },
    { .stats = &dictionary_stats_category_other, },

    // terminator
    { .stats = NULL, NULL, NULL, 0 },
};

#define load_dictionary_stats_entry(x) total += (size_t)(stats.x = __atomic_load_n(&c->stats->x, __ATOMIC_RELAXED))

static void update_dictionary_category_charts(struct dictionary_categories *c) {
    struct dictionary_stats stats;
    stats.name = c->stats->name;
    int priority = 900000;
    const char *family = "dictionaries";
    const char *context_prefix = "dictionaries";

    // ------------------------------------------------------------------------

    size_t total = 0;
    load_dictionary_stats_entry(dictionaries.active);
    load_dictionary_stats_entry(dictionaries.deleted);

    if(c->st_dicts || total != 0) {
        if (unlikely(!c->st_dicts)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.dictionaries", context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.dictionaries", context_prefix);

            c->st_dicts = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , family
                , context
                , "Dictionaries"
                , "dictionaries"
                , "netdata"
                , "pulse"
                , priority + 0
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            c->rd_dicts_active  = rrddim_add(c->st_dicts, "active",   NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            c->rd_dicts_deleted = rrddim_add(c->st_dicts, "deleted",   NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(c->st_dicts->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_dicts, c->rd_dicts_active,  (collected_number)stats.dictionaries.active);
        rrddim_set_by_pointer(c->st_dicts, c->rd_dicts_deleted, (collected_number)stats.dictionaries.deleted);
        rrdset_done(c->st_dicts);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(items.entries);
    load_dictionary_stats_entry(items.referenced);
    load_dictionary_stats_entry(items.pending_deletion);

    if(c->st_items || total != 0) {
        if (unlikely(!c->st_items)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.items", context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.items", context_prefix);

            c->st_items = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , family
                , context
                , "Dictionary Items"
                , "items"
                , "netdata"
                , "pulse"
                , priority + 1
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            c->rd_items_entries             = rrddim_add(c->st_items, "active",   NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            c->rd_items_pending_deletion    = rrddim_add(c->st_items, "deleted",   NULL, -1, 1, RRD_ALGORITHM_ABSOLUTE);
            c->rd_items_referenced          = rrddim_add(c->st_items, "referenced",   NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(c->st_items->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_items, c->rd_items_entries,             stats.items.entries);
        rrddim_set_by_pointer(c->st_items, c->rd_items_pending_deletion,    stats.items.pending_deletion);
        rrddim_set_by_pointer(c->st_items, c->rd_items_referenced,          stats.items.referenced);
        rrdset_done(c->st_items);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(ops.creations);
    load_dictionary_stats_entry(ops.destructions);
    load_dictionary_stats_entry(ops.flushes);
    load_dictionary_stats_entry(ops.traversals);
    load_dictionary_stats_entry(ops.walkthroughs);
    load_dictionary_stats_entry(ops.garbage_collections);
    load_dictionary_stats_entry(ops.searches);
    load_dictionary_stats_entry(ops.inserts);
    load_dictionary_stats_entry(ops.resets);
    load_dictionary_stats_entry(ops.deletes);

    if(c->st_ops || total != 0) {
        if (unlikely(!c->st_ops)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.ops", context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.ops", context_prefix);

            c->st_ops = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , family
                , context
                , "Dictionary Operations"
                , "ops/s"
                , "netdata"
                , "pulse"
                , priority + 2
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            c->rd_ops_creations = rrddim_add(c->st_ops, "creations", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_destructions = rrddim_add(c->st_ops, "destructions", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_flushes = rrddim_add(c->st_ops, "flushes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_traversals = rrddim_add(c->st_ops, "traversals", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_walkthroughs = rrddim_add(c->st_ops, "walkthroughs", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_garbage_collections = rrddim_add(c->st_ops, "garbage_collections", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_searches = rrddim_add(c->st_ops, "searches", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_inserts = rrddim_add(c->st_ops, "inserts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_resets = rrddim_add(c->st_ops, "resets", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_ops_deletes = rrddim_add(c->st_ops, "deletes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(c->st_ops->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_ops, c->rd_ops_creations,           (collected_number)stats.ops.creations);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_destructions,        (collected_number)stats.ops.destructions);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_flushes,             (collected_number)stats.ops.flushes);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_traversals,          (collected_number)stats.ops.traversals);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_walkthroughs,        (collected_number)stats.ops.walkthroughs);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_garbage_collections, (collected_number)stats.ops.garbage_collections);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_searches,            (collected_number)stats.ops.searches);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_inserts,             (collected_number)stats.ops.inserts);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_resets,              (collected_number)stats.ops.resets);
        rrddim_set_by_pointer(c->st_ops, c->rd_ops_deletes,             (collected_number)stats.ops.deletes);

        rrdset_done(c->st_ops);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(callbacks.inserts);
    load_dictionary_stats_entry(callbacks.conflicts);
    load_dictionary_stats_entry(callbacks.reacts);
    load_dictionary_stats_entry(callbacks.deletes);

    if(c->st_callbacks || total != 0) {
        if (unlikely(!c->st_callbacks)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.callbacks", context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.callbacks", context_prefix);

            c->st_callbacks = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , family
                , context
                , "Dictionary Callbacks"
                , "callbacks/s"
                , "netdata"
                , "pulse"
                , priority + 3
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            c->rd_callbacks_inserts = rrddim_add(c->st_callbacks, "inserts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_callbacks_deletes = rrddim_add(c->st_callbacks, "deletes", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_callbacks_conflicts = rrddim_add(c->st_callbacks, "conflicts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_callbacks_reacts = rrddim_add(c->st_callbacks, "reacts", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(c->st_callbacks->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_inserts, (collected_number)stats.callbacks.inserts);
        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_conflicts, (collected_number)stats.callbacks.conflicts);
        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_reacts, (collected_number)stats.callbacks.reacts);
        rrddim_set_by_pointer(c->st_callbacks, c->rd_callbacks_deletes, (collected_number)stats.callbacks.deletes);

        rrdset_done(c->st_callbacks);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(memory.index);
    load_dictionary_stats_entry(memory.values);
    load_dictionary_stats_entry(memory.dict);

    if(c->st_memory || total != 0) {
        if (unlikely(!c->st_memory)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.memory", context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.memory", context_prefix);

            c->st_memory = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , family
                , context
                , "Dictionary Memory"
                , "bytes"
                , "netdata"
                , "pulse"
                , priority + 4
                , localhost->rrd_update_every
                , RRDSET_TYPE_STACKED
            );

            c->rd_memory_indexed = rrddim_add(c->st_memory, "index", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            c->rd_memory_values = rrddim_add(c->st_memory, "data", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            c->rd_memory_dict = rrddim_add(c->st_memory, "structures", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

            rrdlabels_add(c->st_memory->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_memory, c->rd_memory_indexed, (collected_number)stats.memory.index);
        rrddim_set_by_pointer(c->st_memory, c->rd_memory_values, (collected_number)stats.memory.values);
        rrddim_set_by_pointer(c->st_memory, c->rd_memory_dict, (collected_number)stats.memory.dict);

        rrdset_done(c->st_memory);
    }

    // ------------------------------------------------------------------------

    total = 0;
    load_dictionary_stats_entry(spin_locks.use_spins);
    load_dictionary_stats_entry(spin_locks.search_spins);
    load_dictionary_stats_entry(spin_locks.insert_spins);
    load_dictionary_stats_entry(spin_locks.delete_spins);

    if(c->st_spins || total != 0) {
        if (unlikely(!c->st_spins)) {
            char id[RRD_ID_LENGTH_MAX + 1];
            snprintfz(id, RRD_ID_LENGTH_MAX, "%s.%s.spins", context_prefix, stats.name);

            char context[RRD_ID_LENGTH_MAX + 1];
            snprintfz(context, RRD_ID_LENGTH_MAX, "netdata.%s.category.spins", context_prefix);

            c->st_spins = rrdset_create_localhost(
                "netdata"
                , id
                , NULL
                , family
                , context
                , "Dictionary Spins"
                , "count"
                , "netdata"
                , "pulse"
                , priority + 5
                , localhost->rrd_update_every
                , RRDSET_TYPE_LINE
            );

            c->rd_spins_use = rrddim_add(c->st_spins, "use", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_spins_search = rrddim_add(c->st_spins, "search", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_spins_insert = rrddim_add(c->st_spins, "insert", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            c->rd_spins_delete = rrddim_add(c->st_spins, "delete", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrdlabels_add(c->st_spins->rrdlabels, "category", stats.name, RRDLABEL_SRC_AUTO);
        }

        rrddim_set_by_pointer(c->st_spins, c->rd_spins_use, (collected_number)stats.spin_locks.use_spins);
        rrddim_set_by_pointer(c->st_spins, c->rd_spins_search, (collected_number)stats.spin_locks.search_spins);
        rrddim_set_by_pointer(c->st_spins, c->rd_spins_insert, (collected_number)stats.spin_locks.insert_spins);
        rrddim_set_by_pointer(c->st_spins, c->rd_spins_delete, (collected_number)stats.spin_locks.delete_spins);

        rrdset_done(c->st_spins);
    }
}

void pulse_dictionary_do(bool extended) {
    if(!extended) return;

    for(int i = 0; dictionary_categories[i].stats ;i++) {
        update_dictionary_category_charts(&dictionary_categories[i]);
    }
}
#endif // DICT_WITH_STATS
