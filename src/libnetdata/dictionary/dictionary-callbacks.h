// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_DICTIONARY_CALLBACKS_H
#define NETDATA_DICTIONARY_CALLBACKS_H

#include "dictionary-internals.h"

// ----------------------------------------------------------------------------
// callbacks execution

static inline void dictionary_execute_insert_callback(DICTIONARY *dict, DICTIONARY_ITEM *item, void *constructor_data) {
    if(likely(!dict->hooks || !dict->hooks->insert_callback))
        return;

    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    internal_error(false,
                   "DICTIONARY: Running insert callback on item '%s' of dictionary created from %s() %zu@%s.",
                   item_get_name(item),
                   dict->creation_function,
                   dict->creation_line,
                   dict->creation_file);

    dict->hooks->insert_callback(item, item->shared->value, constructor_data?constructor_data:dict->hooks->insert_callback_data);
    DICTIONARY_STATS_CALLBACK_INSERTS_PLUS1(dict);
}

static inline bool dictionary_execute_conflict_callback(DICTIONARY *dict, DICTIONARY_ITEM *item, void *new_value, void *constructor_data) {
    if(likely(!dict->hooks || !dict->hooks->conflict_callback))
        return false;

    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    internal_error(false,
                   "DICTIONARY: Running conflict callback on item '%s' of dictionary created from %s() %zu@%s.",
                   item_get_name(item),
                   dict->creation_function,
                   dict->creation_line,
                   dict->creation_file);

    bool ret = dict->hooks->conflict_callback(
        item, item->shared->value, new_value,
        constructor_data ? constructor_data : dict->hooks->conflict_callback_data);

    DICTIONARY_STATS_CALLBACK_CONFLICTS_PLUS1(dict);

    return ret;
}

static inline void dictionary_execute_react_callback(DICTIONARY *dict, DICTIONARY_ITEM *item, void *constructor_data) {
    if(likely(!dict->hooks || !dict->hooks->react_callback))
        return;

    if(unlikely(is_view_dictionary(dict)))
        fatal("DICTIONARY: called %s() on a view.", __FUNCTION__ );

    internal_error(false,
                   "DICTIONARY: Running react callback on item '%s' of dictionary created from %s() %zu@%s.",
                   item_get_name(item),
                   dict->creation_function,
                   dict->creation_line,
                   dict->creation_file);

    dict->hooks->react_callback(item, item->shared->value,
                                constructor_data?constructor_data:dict->hooks->react_callback_data);

    DICTIONARY_STATS_CALLBACK_REACTS_PLUS1(dict);
}

static inline void dictionary_execute_delete_callback(DICTIONARY *dict, DICTIONARY_ITEM *item) {
    if(likely(!dict->hooks || !dict->hooks->delete_callback))
        return;

    // We may execute delete callback on items deleted from a view,
    // because we may have references to it, after the master is gone
    // so, the shared structure will remain until the last reference is released.

    internal_error(false,
                   "DICTIONARY: Running delete callback on item '%s' of dictionary created from %s() %zu@%s.",
                   item_get_name(item),
                   dict->creation_function,
                   dict->creation_line,
                   dict->creation_file);

    dict->hooks->delete_callback(item, item->shared->value, dict->hooks->delelte_callback_data);

    DICTIONARY_STATS_CALLBACK_DELETES_PLUS1(dict);
}


#endif //NETDATA_DICTIONARY_CALLBACKS_H
