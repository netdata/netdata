// SPDX-License-Identifier: GPL-3.0-or-later

#include "dictionary-internals.h"

// ----------------------------------------------------------------------------
// unit test

static void dictionary_unittest_free_char_pp(char **pp, size_t entries) {
    for(size_t i = 0; i < entries ;i++)
        freez(pp[i]);

    freez(pp);
}

static char **dictionary_unittest_generate_names(size_t entries) {
    char **names = mallocz(sizeof(char *) * entries);
    for(size_t i = 0; i < entries ;i++) {
        char buf[25 + 1] = "";
        snprintfz(buf, sizeof(buf), "name.%zu.0123456789.%zu!@#$%%^&*(),./[]{}\\|~`", i, entries / 2 + i);
        names[i] = strdupz(buf);
    }
    return names;
}

static char **dictionary_unittest_generate_values(size_t entries) {
    char **values = mallocz(sizeof(char *) * entries);
    for(size_t i = 0; i < entries ;i++) {
        char buf[25 + 1] = "";
        snprintfz(buf, sizeof(buf), "value-%zu-0987654321.%zu%%^&*(),. \t !@#$/[]{}\\|~`", i, entries / 2 + i);
        values[i] = strdupz(buf);
    }
    return values;
}

static size_t dictionary_unittest_set_clone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(values[i]);
        char *val = (char *)dictionary_set(dict, names[i], values[i], vallen);
        if(val == values[i]) { fprintf(stderr, ">>> %s() returns reference to value\n", __FUNCTION__); errors++; }
        if(!val || memcmp(val, values[i], vallen) != 0)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_set_null(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    size_t errors = 0;
    size_t i = 0;
    for(; i < entries ;i++) {
        void *val = dictionary_set(dict, names[i], NULL, 0);
        if(val != NULL) { fprintf(stderr, ">>> %s() returns a non NULL value\n", __FUNCTION__); errors++; }
    }
    if(dictionary_entries(dict) != i) {
        fprintf(stderr, ">>> %s() dictionary items do not match\n", __FUNCTION__);
        errors++;
    }
    return errors;
}


static size_t dictionary_unittest_set_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(values[i]);
        char *val = (char *)dictionary_set(dict, names[i], values[i], vallen);
        if(val != values[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_get_clone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(values[i]);
        char *val = (char *)dictionary_get(dict, names[i]);
        if(val == values[i]) { fprintf(stderr, ">>> %s() returns reference to value\n", __FUNCTION__); errors++; }
        if(!val || memcmp(val, values[i], vallen) != 0)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_get_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        char *val = (char *)dictionary_get(dict, names[i]);
        if(val != values[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_get_nonexisting(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        char *val = (char *)dictionary_get(dict, values[i]);
        if(val) { fprintf(stderr, ">>> %s() returns non-existing item\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_del_nonexisting(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        bool ret = dictionary_del(dict, values[i]);
        if(ret) { fprintf(stderr, ">>> %s() deleted non-existing item\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_del_existing(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    size_t errors = 0;

    size_t forward_from = 0, forward_to = entries / 3;
    size_t middle_from = forward_to, middle_to = entries * 2 / 3;
    size_t backward_from = middle_to, backward_to = entries;

    for(size_t i = forward_from; i < forward_to ;i++) {
        bool ret = dictionary_del(dict, names[i]);
        if(!ret) { fprintf(stderr, ">>> %s() didn't delete (forward) existing item\n", __FUNCTION__); errors++; }
    }

    for(size_t i = middle_to - 1; i >= middle_from ;i--) {
        bool ret = dictionary_del(dict, names[i]);
        if(!ret) { fprintf(stderr, ">>> %s() didn't delete (middle) existing item\n", __FUNCTION__); errors++; }
    }

    for(size_t i = backward_to - 1; i >= backward_from ;i--) {
        bool ret = dictionary_del(dict, names[i]);
        if(!ret) { fprintf(stderr, ">>> %s() didn't delete (backward) existing item\n", __FUNCTION__); errors++; }
    }

    return errors;
}

static size_t dictionary_unittest_reset_clone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    // set the name as value too
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(names[i]);
        char *val = (char *)dictionary_set(dict, names[i], names[i], vallen);
        if(val == names[i]) { fprintf(stderr, ">>> %s() returns reference to value\n", __FUNCTION__); errors++; }
        if(!val || memcmp(val, names[i], vallen) != 0)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_reset_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)values;
    // set the name as value too
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(names[i]);
        char *val = (char *)dictionary_set(dict, names[i], names[i], vallen);
        if(val != names[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
        if(!val)  { fprintf(stderr, ">>> %s() returns invalid value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static size_t dictionary_unittest_reset_dont_overwrite_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries) {
    // set the name as value too
    size_t errors = 0;
    for(size_t i = 0; i < entries ;i++) {
        size_t vallen = strlen(names[i]);
        char *val = (char *)dictionary_set(dict, names[i], names[i], vallen);
        if(val != values[i]) { fprintf(stderr, ">>> %s() returns invalid pointer to value\n", __FUNCTION__); errors++; }
    }
    return errors;
}

static int dictionary_unittest_walkthrough_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *data __maybe_unused) {
    return 1;
}

static size_t dictionary_unittest_walkthrough(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    int sum = dictionary_walkthrough_read(dict, dictionary_unittest_walkthrough_callback, NULL);
    if(sum < (int)entries) return entries - sum;
    else return sum - entries;
}

static int dictionary_unittest_walkthrough_delete_this_callback(const DICTIONARY_ITEM *item, void *value __maybe_unused, void *data) {
    const char *name = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);

    if(!dictionary_del((DICTIONARY *)data, name))
        return 0;

    return 1;
}

static size_t dictionary_unittest_walkthrough_delete_this(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    int sum = dictionary_walkthrough_write(dict, dictionary_unittest_walkthrough_delete_this_callback, dict);
    if(sum < (int)entries) return entries - sum;
    else return sum - entries;
}

static int dictionary_unittest_walkthrough_stop_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *data __maybe_unused) {
    return -1;
}

static size_t dictionary_unittest_walkthrough_stop(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    int sum = dictionary_walkthrough_read(dict, dictionary_unittest_walkthrough_stop_callback, NULL);
    if(sum != -1) return 1;
    return 0;
}

static size_t dictionary_unittest_foreach(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    size_t count = 0;
    char *item;
    dfe_start_read(dict, item)
        count++;
    dfe_done(item);

    if(count > entries) return count - entries;
    return entries - count;
}

static size_t dictionary_unittest_foreach_delete_this(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    size_t count = 0;
    char *item;
    dfe_start_write(dict, item)
        if(dictionary_del(dict, item_dfe.name)) count++;
    dfe_done(item);

    if(count > entries) return count - entries;
    return entries - count;
}

static size_t dictionary_unittest_destroy(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    (void)entries;
    size_t bytes = dictionary_destroy(dict);
    fprintf(stderr, " %s() freed %zu bytes,", __FUNCTION__, bytes);
    return 0;
}

static usec_t dictionary_unittest_run_and_measure_time(DICTIONARY *dict, char *message, char **names, char **values, size_t entries, size_t *errors, size_t (*callback)(DICTIONARY *dict, char **names, char **values, size_t entries)) {
    fprintf(stderr, "%40s ... ", message);

    usec_t started = now_realtime_usec();
    size_t errs = callback(dict, names, values, entries);
    usec_t ended = now_realtime_usec();
    usec_t dt = ended - started;

    if(callback == dictionary_unittest_destroy) dict = NULL;

    long int found_ok = 0, found_deleted = 0, found_referenced = 0;
    if(dict) {
        DICTIONARY_ITEM *item;
        DOUBLE_LINKED_LIST_FOREACH_FORWARD(dict->items.list, item, prev, next) {
            if(item->refcount >= 0 && !(item ->flags & ITEM_FLAG_DELETED))
                found_ok++;
            else
                found_deleted++;

            if(item->refcount > 0)
                found_referenced++;
        }
    }

    fprintf(stderr, " %zu errors, %d (found %ld) items in dictionary, %d (found %ld) referenced, %d (found %ld) deleted, %"PRIu64" usec \n",
            errs, dict?dict->entries:0, found_ok, dict?dict->referenced_items:0, found_referenced, dict?dict->pending_deletion_items:0, found_deleted, dt);
    *errors += errs;
    return dt;
}

static void dictionary_unittest_clone(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, errors, dictionary_unittest_set_clone);
    dictionary_unittest_run_and_measure_time(dict, "getting entries", names, values, entries, errors, dictionary_unittest_get_clone);
    dictionary_unittest_run_and_measure_time(dict, "getting non-existing entries", names, values, entries, errors, dictionary_unittest_get_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "resetting entries", names, values, entries, errors, dictionary_unittest_reset_clone);
    dictionary_unittest_run_and_measure_time(dict, "deleting non-existing entries", names, values, entries, errors, dictionary_unittest_del_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback", names, values, entries, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback stop", names, values, entries, errors, dictionary_unittest_walkthrough_stop);
    dictionary_unittest_run_and_measure_time(dict, "deleting existing entries", names, values, entries, errors, dictionary_unittest_del_existing);
    dictionary_unittest_run_and_measure_time(dict, "walking through empty", names, values, 0, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach empty", names, values, 0, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, errors, dictionary_unittest_destroy);
}

static void dictionary_unittest_nonclone(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "getting entries", names, values, entries, errors, dictionary_unittest_get_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "getting non-existing entries", names, values, entries, errors, dictionary_unittest_get_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "resetting entries", names, values, entries, errors, dictionary_unittest_reset_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "deleting non-existing entries", names, values, entries, errors, dictionary_unittest_del_nonexisting);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback", names, values, entries, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback stop", names, values, entries, errors, dictionary_unittest_walkthrough_stop);
    dictionary_unittest_run_and_measure_time(dict, "deleting existing entries", names, values, entries, errors, dictionary_unittest_del_existing);
    dictionary_unittest_run_and_measure_time(dict, "walking through empty", names, values, 0, errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach empty", names, values, 0, errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, errors, dictionary_unittest_destroy);
}

struct dictionary_unittest_sorting {
    const char *old_name;
    const char *old_value;
    size_t count;
};

static int dictionary_unittest_sorting_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *name = dictionary_acquired_item_name((DICTIONARY_ITEM *)item);
    struct dictionary_unittest_sorting *t = (struct dictionary_unittest_sorting *)data;
    const char *v = (const char *)value;

    int ret = 0;
    if(t->old_name && strcmp(t->old_name, name) > 0) {
        fprintf(stderr, "name '%s' should be after '%s'\n", t->old_name, name);
        ret = 1;
    }
    t->count++;
    t->old_name = name;
    t->old_value = v;

    return ret;
}

static size_t dictionary_unittest_sorted_walkthrough(DICTIONARY *dict, char **names, char **values, size_t entries) {
    (void)names;
    (void)values;
    struct dictionary_unittest_sorting tmp = { .old_name = NULL, .old_value = NULL, .count = 0 };
    size_t errors;
    errors = dictionary_sorted_walkthrough_read(dict, dictionary_unittest_sorting_callback, &tmp);

    if(tmp.count != entries) {
        fprintf(stderr, "Expected %zu entries, counted %zu\n", entries, tmp.count);
        errors++;
    }
    return errors;
}

static void dictionary_unittest_sorting(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, errors, dictionary_unittest_set_clone);
    dictionary_unittest_run_and_measure_time(dict, "sorted walkthrough", names, values, entries, errors, dictionary_unittest_sorted_walkthrough);
}

static void dictionary_unittest_null_dfe(DICTIONARY *dict, char **names, char **values, size_t entries, size_t *errors) {
    dictionary_unittest_run_and_measure_time(dict, "adding null value entries", names, values, entries, errors, dictionary_unittest_set_null);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, errors, dictionary_unittest_foreach);
}


static int unittest_check_dictionary_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value __maybe_unused, void *data __maybe_unused) {
    return 1;
}

static size_t unittest_check_dictionary(const char *label, DICTIONARY *dict, size_t traversable, size_t active_items, size_t deleted_items, size_t referenced_items, size_t pending_deletion) {
    size_t errors = 0;

    size_t ll = 0;
    void *t;
    dfe_start_read(dict, t)
        ll++;
    dfe_done(t);

    fprintf(stderr, "DICT %-20s: dictionary foreach entries %zu, expected %zu...\t\t\t\t\t",
            label, ll, traversable);
    if(ll != traversable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    ll = dictionary_walkthrough_read(dict, unittest_check_dictionary_callback, NULL);
    fprintf(stderr, "DICT %-20s: dictionary walkthrough entries %zu, expected %zu...\t\t\t\t",
            label, ll, traversable);
    if(ll != traversable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    ll = dictionary_sorted_walkthrough_read(dict, unittest_check_dictionary_callback, NULL);
    fprintf(stderr, "DICT %-20s: dictionary sorted walkthrough entries %zu, expected %zu...\t\t\t",
            label, ll, traversable);
    if(ll != traversable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    DICTIONARY_ITEM *item;
    size_t active = 0, deleted = 0, referenced = 0, pending = 0;
    for(item = dict->items.list; item; item = item->next) {
        if(!(item->flags & ITEM_FLAG_DELETED) && !(item->shared->flags & ITEM_FLAG_DELETED))
            active++;
        else {
            deleted++;

            if(item->refcount == 0)
                pending++;
        }

        if(item->refcount > 0)
            referenced++;
    }

    fprintf(stderr, "DICT %-20s: dictionary active items reported %d, counted %zu, expected %zu...\t\t\t",
            label, dict->entries, active, active_items);
    if(active != active_items || active != (size_t)dict->entries) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "DICT %-20s: dictionary deleted items counted %zu, expected %zu...\t\t\t\t",
            label, deleted, deleted_items);
    if(deleted != deleted_items) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "DICT %-20s: dictionary referenced items reported %d, counted %zu, expected %zu...\t\t",
            label, dict->referenced_items, referenced, referenced_items);
    if(referenced != referenced_items || dict->referenced_items != (long int)referenced) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "DICT %-20s: dictionary pending deletion items reported %d, counted %zu, expected %zu...\t",
            label, dict->pending_deletion_items, pending, pending_deletion);
    if(pending != pending_deletion || pending != (size_t)dict->pending_deletion_items) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    return errors;
}

static int check_item_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data) {
    return value == data;
}

static size_t unittest_check_item(const char *label, DICTIONARY *dict,
                                  DICTIONARY_ITEM *item, const char *name, const char *value, int refcount,
                                  ITEM_FLAGS deleted_flags, bool searchable, bool browsable, bool linked) {
    size_t errors = 0;

    fprintf(stderr, "ITEM %-20s: name is '%s', expected '%s'...\t\t\t\t\t\t", label, item_get_name(item), name);
    if(strcmp(item_get_name(item), name) != 0) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "ITEM %-20s: value is '%s', expected '%s'...\t\t\t\t\t", label, (const char *)item->shared->value, value);
    if(strcmp((const char *)item->shared->value, value) != 0) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "ITEM %-20s: refcount is %d, expected %d...\t\t\t\t\t\t\t", label, item->refcount, refcount);
    if (item->refcount != refcount) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    fprintf(stderr, "ITEM %-20s: deleted flag is %s, expected %s...\t\t\t\t\t", label,
            (item->flags & ITEM_FLAG_DELETED || item->shared->flags & ITEM_FLAG_DELETED)?"true":"false",
            (deleted_flags & ITEM_FLAG_DELETED)?"true":"false");

    if ((item->flags & ITEM_FLAG_DELETED || item->shared->flags & ITEM_FLAG_DELETED) != (deleted_flags & ITEM_FLAG_DELETED)) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    void *v = dictionary_get(dict, name);
    bool found = v == item->shared->value;
    fprintf(stderr, "ITEM %-20s: searchable %5s, expected %5s...\t\t\t\t\t\t", label,
            found?"true":"false", searchable?"true":"false");
    if(found != searchable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = false;
    void *t;
    dfe_start_read(dict, t) {
        if(t == item->shared->value) found = true;
    }
    dfe_done(t);

    fprintf(stderr, "ITEM %-20s: dfe browsable %5s, expected %5s...\t\t\t\t\t", label,
            found?"true":"false", browsable?"true":"false");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = dictionary_walkthrough_read(dict, check_item_callback, item->shared->value);
    fprintf(stderr, "ITEM %-20s: walkthrough browsable %5s, expected %5s...\t\t\t\t", label,
            found?"true":"false", browsable?"true":"false");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = dictionary_sorted_walkthrough_read(dict, check_item_callback, item->shared->value);
    fprintf(stderr, "ITEM %-20s: sorted walkthrough browsable %5s, expected %5s...\t\t\t", label,
            found?"true":"false", browsable?"true":"false");
    if(found != browsable) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    found = false;
    DICTIONARY_ITEM *n;
    for(n = dict->items.list; n ;n = n->next)
        if(n == item) found = true;

    fprintf(stderr, "ITEM %-20s: linked %5s, expected %5s...\t\t\t\t\t\t", label,
            found?"true":"false", linked?"true":"false");
    if(found != linked) {
        fprintf(stderr, "FAILED\n");
        errors++;
    }
    else
        fprintf(stderr, "OK\n");

    return errors;
}

struct thread_unittest {
    int join;
    DICTIONARY *dict;
    int dups;

    ND_THREAD *thread;
    struct dictionary_stats stats;
};

static void unittest_dict_thread(void *arg) {
    struct thread_unittest *tu = arg;
    for(; 1 ;) {
        if(__atomic_load_n(&tu->join, __ATOMIC_RELAXED))
            break;

        DICT_ITEM_CONST DICTIONARY_ITEM *item =
            dictionary_set_and_acquire_item_advanced(tu->dict, "dict thread checking 1234567890",
                                                     -1, NULL, 0, NULL);
        tu->stats.ops.inserts++;

        dictionary_get(tu->dict, dictionary_acquired_item_name(item));
        tu->stats.ops.searches++;

        void *t1;
        dfe_start_write(tu->dict, t1) {

            // this should delete the referenced item
            dictionary_del(tu->dict, t1_dfe.name);
            tu->stats.ops.deletes++;

            void *t2;
            dfe_start_write(tu->dict, t2) {
                // this should add another
                dictionary_set(tu->dict, t2_dfe.name, NULL, 0);
                tu->stats.ops.inserts++;

                dictionary_get(tu->dict, dictionary_acquired_item_name(item));
                tu->stats.ops.searches++;

                // and this should delete it again
                dictionary_del(tu->dict, t2_dfe.name);
                tu->stats.ops.deletes++;
            }
            dfe_done(t2);
            tu->stats.ops.traversals++;

            // this should fail to add it
            dictionary_set(tu->dict, t1_dfe.name, NULL, 0);
            tu->stats.ops.inserts++;

            dictionary_del(tu->dict, t1_dfe.name);
            tu->stats.ops.deletes++;
        }
        dfe_done(t1);
        tu->stats.ops.traversals++;

        for(int i = 0; i < tu->dups ; i++) {
            dictionary_acquired_item_dup(tu->dict, item);
            dictionary_get(tu->dict, dictionary_acquired_item_name(item));
            tu->stats.ops.searches++;
        }

        for(int i = 0; i < tu->dups ; i++) {
            dictionary_acquired_item_release(tu->dict, item);
            dictionary_del(tu->dict, dictionary_acquired_item_name(item));
            tu->stats.ops.deletes++;
        }

        dictionary_acquired_item_release(tu->dict, item);
        dictionary_del(tu->dict, "dict thread checking 1234567890");
        tu->stats.ops.deletes++;

        // test concurrent deletions and flushes
        {
            if(gettid_cached() % 2) {
                char buf [256 + 1];

                for (int i = 0; i < 1000; i++) {
                    snprintfz(buf, sizeof(buf), "del/flush test %d", i);
                    dictionary_set(tu->dict, buf, NULL, 0);
                    tu->stats.ops.inserts++;
                }

                for (int i = 0; i < 1000; i++) {
                    snprintfz(buf, sizeof(buf), "del/flush test %d", i);
                    dictionary_del(tu->dict, buf);
                    tu->stats.ops.deletes++;
                }
            }
            else {
                for (int i = 0; i < 10; i++) {
                    dictionary_flush(tu->dict);
                    tu->stats.ops.flushes++;
                }
            }
        }
    }
}

static int dictionary_unittest_threads() {
    time_t seconds_to_run = 5;
    int threads_to_create = 2;

    struct thread_unittest tu[threads_to_create];
    memset(tu, 0, sizeof(struct thread_unittest) * threads_to_create);

    fprintf(
        stderr,
        "\nChecking dictionary concurrency with %d threads for %lld seconds...\n",
        threads_to_create,
        (long long)seconds_to_run);

    // threads testing of dictionary
    struct dictionary_stats stats = {};
    tu[0].join = 0;
    tu[0].dups = 1;
    tu[0].dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE, &stats, 0);

    for (int i = 0; i < threads_to_create; i++) {
        if(i)
            tu[i] = tu[0];

        char buf[100 + 1];
        snprintf(buf, 100, "dict%d", i);
        tu[i].thread = nd_thread_create(buf, NETDATA_THREAD_OPTION_DONT_LOG, unittest_dict_thread, &tu[i]);
    }

    sleep_usec(seconds_to_run * USEC_PER_SEC);

    for (int i = 0; i < threads_to_create; i++) {
        __atomic_store_n(&tu[i].join, 1, __ATOMIC_RELAXED);

        nd_thread_join(tu[i].thread);

        if(i) {
            tu[0].stats.ops.inserts += tu[i].stats.ops.inserts;
            tu[0].stats.ops.deletes += tu[i].stats.ops.deletes;
            tu[0].stats.ops.searches += tu[i].stats.ops.searches;
            tu[0].stats.ops.flushes += tu[i].stats.ops.flushes;
            tu[0].stats.ops.traversals += tu[i].stats.ops.traversals;
        }
    }

    fprintf(stderr,
            "CALLS : inserts %zu"
            ", deletes %zu"
            ", searches %zu"
            ", traversals %zu"
            ", flushes %zu"
            "\n",
            tu[0].stats.ops.inserts,
            tu[0].stats.ops.deletes,
            tu[0].stats.ops.searches,
            tu[0].stats.ops.traversals,
            tu[0].stats.ops.flushes
    );

#ifdef DICT_WITH_STATS
    fprintf(stderr,
            "ACTUAL: inserts %zu"
            ", deletes %zu"
            ", searches %zu"
            ", traversals %zu"
            ", resets %zu"
            ", flushes %zu"
            ", entries %d"
            ", referenced_items %d"
            ", pending deletions %d"
            ", check spins %zu"
            ", insert spins %zu"
            ", delete spins %zu"
            ", search ignores %zu"
            "\n",
            stats.ops.inserts,
            stats.ops.deletes,
            stats.ops.searches,
            stats.ops.traversals,
            stats.ops.resets,
            stats.ops.flushes,
            tu[0].dict->entries,
            tu[0].dict->referenced_items,
            tu[0].dict->pending_deletion_items,
            stats.spin_locks.use_spins,
            stats.spin_locks.insert_spins,
            stats.spin_locks.delete_spins,
            stats.spin_locks.search_spins
    );
#endif

    dictionary_destroy(tu[0].dict);
    return 0;
}

struct thread_view_unittest {
    int join;
    DICTIONARY *master;
    DICTIONARY *view;
    DICTIONARY_ITEM *item_master;
    int dups;
};

static void unittest_dict_master_thread(void *arg) {
    struct thread_view_unittest *tv = arg;

    DICTIONARY_ITEM *item = NULL;
    int loops = 0;
    while(!__atomic_load_n(&tv->join, __ATOMIC_RELAXED)) {

        if(!item)
            item = dictionary_set_and_acquire_item(tv->master, "ITEM1", "123", strlen("123"));

        if(__atomic_load_n(&tv->item_master, __ATOMIC_RELAXED) != NULL) {
            dictionary_acquired_item_release(tv->master, item);
            dictionary_del(tv->master, "ITEM1");
            item = NULL;
            loops++;
            continue;
        }

        dictionary_acquired_item_dup(tv->master, item); // for the view thread
        __atomic_store_n(&tv->item_master, item, __ATOMIC_RELAXED);
        dictionary_del(tv->master, "ITEM1");


        for(int i = 0; i < tv->dups + loops ; i++) {
            dictionary_acquired_item_dup(tv->master, item);
        }

        for(int i = 0; i < tv->dups + loops ; i++) {
            dictionary_acquired_item_release(tv->master, item);
        }

        dictionary_acquired_item_release(tv->master, item);

        item = NULL;
        loops = 0;
    }
}

static void unittest_dict_view_thread(void *arg) {
    struct thread_view_unittest *tv = arg;

    DICTIONARY_ITEM *m_item = NULL;

    while(!__atomic_load_n(&tv->join, __ATOMIC_RELAXED)) {
        if(!(m_item = __atomic_load_n(&tv->item_master, __ATOMIC_RELAXED)))
            continue;

        DICTIONARY_ITEM *v_item = dictionary_view_set_and_acquire_item(tv->view, "ITEM2", m_item);
        dictionary_acquired_item_release(tv->master, m_item);
        __atomic_store_n(&tv->item_master, NULL, __ATOMIC_RELAXED);

        for(int i = 0; i < tv->dups ; i++) {
            dictionary_acquired_item_dup(tv->view, v_item);
        }

        for(int i = 0; i < tv->dups ; i++) {
            dictionary_acquired_item_release(tv->view, v_item);
        }

        dictionary_del(tv->view, "ITEM2");

        while(!__atomic_load_n(&tv->join, __ATOMIC_RELAXED) && !(m_item = __atomic_load_n(&tv->item_master, __ATOMIC_RELAXED))) {
            dictionary_acquired_item_dup(tv->view, v_item);
            dictionary_acquired_item_release(tv->view, v_item);
        }

        dictionary_acquired_item_release(tv->view, v_item);
    }
}

static struct dictionary_stats stats_master = { 0 };
static struct dictionary_stats stats_view = { 0 };

static int dictionary_unittest_view_threads() {
    struct thread_view_unittest tv = {
        .join = 0,
        .master = NULL,
        .view = NULL,
        .item_master = NULL,
        .dups = 1,
    };

    // threads testing of dictionary
    tv.master = dictionary_create_advanced(DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE, &stats_master, 0);
    tv.view = dictionary_create_view(tv.master);
    tv.view->stats = &stats_view;

    time_t seconds_to_run = 5;
    fprintf(
        stderr,
        "\nChecking dictionary concurrency with 1 master and 1 view threads for %lld seconds...\n",
        (long long)seconds_to_run);

    ND_THREAD *master_thread, *view_thread;
    tv.join = 0;

    master_thread = nd_thread_create("master", NETDATA_THREAD_OPTION_DONT_LOG, unittest_dict_master_thread, &tv);

    view_thread = nd_thread_create("view", NETDATA_THREAD_OPTION_DONT_LOG, unittest_dict_view_thread, &tv);

    sleep_usec(seconds_to_run * USEC_PER_SEC);

    __atomic_store_n(&tv.join, 1, __ATOMIC_RELAXED);
    nd_thread_join(view_thread);
    nd_thread_join(master_thread);

#ifdef DICT_WITH_STATS
    fprintf(stderr,
            "MASTER: inserts %zu"
            ", deletes %zu"
            ", searches %zu"
            ", resets %zu"
            ", entries %d"
            ", referenced_items %d"
            ", pending deletions %d"
            ", check spins %zu"
            ", insert spins %zu"
            ", delete spins %zu"
            ", search ignores %zu"
            "\n",
            stats_master.ops.inserts,
            stats_master.ops.deletes,
            stats_master.ops.searches,
            stats_master.ops.resets,
            tv.master->entries,
            tv.master->referenced_items,
            tv.master->pending_deletion_items,
            stats_master.spin_locks.use_spins,
            stats_master.spin_locks.insert_spins,
            stats_master.spin_locks.delete_spins,
            stats_master.spin_locks.search_spins
    );
    fprintf(stderr,
            "VIEW  : inserts %zu"
            ", deletes %zu"
            ", searches %zu"
            ", resets %zu"
            ", entries %d"
            ", referenced_items %d"
            ", pending deletions %d"
            ", check spins %zu"
            ", insert spins %zu"
            ", delete spins %zu"
            ", search ignores %zu"
            "\n",
            stats_view.ops.inserts,
            stats_view.ops.deletes,
            stats_view.ops.searches,
            stats_view.ops.resets,
            tv.view->entries,
            tv.view->referenced_items,
            tv.view->pending_deletion_items,
            stats_view.spin_locks.use_spins,
            stats_view.spin_locks.insert_spins,
            stats_view.spin_locks.delete_spins,
            stats_view.spin_locks.search_spins
    );
#endif

    dictionary_destroy(tv.master);
    dictionary_destroy(tv.view);

    return 0;
}

size_t dictionary_unittest_views(void) {
    size_t errors = 0;
    struct dictionary_stats stats = {};
    DICTIONARY *master = dictionary_create_advanced(DICT_OPTION_NONE, &stats, 0);
    DICTIONARY *view = dictionary_create_view(master);
    DICTIONARY_ITEM *master_item2 = NULL;
    DICTIONARY_ITEM *view_item2 = NULL;
    DICTIONARY_ITEM *lookup = NULL;

    fprintf(stderr, "\n\nChecking dictionary views...\n");

    // Add an item to both master and view, then remove the view first and the master second
    fprintf(stderr, "\nPASS 1: Adding 1 item to master:\n");
    DICTIONARY_ITEM *item1_on_master = dictionary_set_and_acquire_item(master, "KEY 1", "VALUE1", strlen("VALUE1") + 1);
    errors += unittest_check_dictionary("master", master, 1, 1, 0, 1, 0);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 1: Adding master item to view:\n");
    DICTIONARY_ITEM *item1_on_view = dictionary_view_set_and_acquire_item(view, "KEY 1 ON VIEW", item1_on_master);
    errors += unittest_check_dictionary("view", view, 1, 1, 0, 1, 0);
    errors += unittest_check_item("view", view, item1_on_view, "KEY 1 ON VIEW", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 1: Deleting view item:\n");
    dictionary_del(view, "KEY 1 ON VIEW");
    errors += unittest_check_dictionary("master", master, 1, 1, 0, 1, 0);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 1, 0);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);
    errors += unittest_check_item("view", view, item1_on_view, "KEY 1 ON VIEW", item1_on_master->shared->value, 1, ITEM_FLAG_DELETED, false, false, true);

    fprintf(stderr, "\nPASS 1: Releasing the deleted view item:\n");
    dictionary_acquired_item_release(view, item1_on_view);
    errors += unittest_check_dictionary("master", master, 1, 1, 0, 1, 0);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 0, 1);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 1: Releasing the acquired master item:\n");
    dictionary_acquired_item_release(master, item1_on_master);
    errors += unittest_check_dictionary("master", master, 1, 1, 0, 0, 0);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 0, 1);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 0, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 1: Deleting the released master item:\n");
    dictionary_del(master, "KEY 1");
    errors += unittest_check_dictionary("master", master, 0, 0, 0, 0, 0);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 0, 1);

    // The other way now:
    // Add an item to both master and view, then remove the master first and verify it is deleted on the view also
    fprintf(stderr, "\nPASS 2: Adding 1 item to master:\n");
    item1_on_master = dictionary_set_and_acquire_item(master, "KEY 1", "VALUE1", strlen("VALUE1") + 1);
    errors += unittest_check_dictionary("master", master, 1, 1, 0, 1, 0);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 2: Adding master item to view:\n");
    item1_on_view = dictionary_view_set_and_acquire_item(view, "KEY 1 ON VIEW", item1_on_master);
    errors += unittest_check_dictionary("view", view, 1, 1, 0, 1, 0);
    errors += unittest_check_item("view", view, item1_on_view, "KEY 1 ON VIEW", item1_on_master->shared->value, 1, ITEM_FLAG_NONE, true, true, true);

    fprintf(stderr, "\nPASS 2: Deleting master item:\n");
    dictionary_del(master, "KEY 1");
    garbage_collect_pending_deletes(view);
    errors += unittest_check_dictionary("master", master, 0, 0, 1, 1, 0);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 1, 0);
    errors += unittest_check_item("master", master, item1_on_master, "KEY 1", item1_on_master->shared->value, 1, ITEM_FLAG_DELETED, false, false, true);
    errors += unittest_check_item("view", view, item1_on_view, "KEY 1 ON VIEW", item1_on_master->shared->value, 1, ITEM_FLAG_DELETED, false, false, true);

    fprintf(stderr, "\nPASS 2: Releasing the acquired master item:\n");
    dictionary_acquired_item_release(master, item1_on_master);
    errors += unittest_check_dictionary("master", master, 0, 0, 1, 0, 1);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 1, 0);
    errors += unittest_check_item("view", view, item1_on_view, "KEY 1 ON VIEW", item1_on_master->shared->value, 1, ITEM_FLAG_DELETED, false, false, true);

    fprintf(stderr, "\nPASS 2: Releasing the deleted view item:\n");
    dictionary_acquired_item_release(view, item1_on_view);
    errors += unittest_check_dictionary("master", master, 0, 0, 1, 0, 1);
    errors += unittest_check_dictionary("view", view, 0, 0, 1, 0, 1);

    fprintf(stderr, "\nPASS 3: Replacing a stale view item after master deletion:\n");
    item1_on_master = dictionary_set_and_acquire_item(master, "KEY 1", "VALUE1", strlen("VALUE1") + 1);
    item1_on_view = dictionary_view_set_and_acquire_item(view, "KEY 1 ON VIEW", item1_on_master);
    dictionary_acquired_item_release(view, item1_on_view);
    dictionary_del(master, "KEY 1");

    // Suppress the preflight garbage collection once so view_set() exercises
    // the stale-entry cleanup path inside dict_item_add_or_reset_value_and_acquire().
    __atomic_store_n(&view->last_gc_run_us, now_realtime_usec(), __ATOMIC_RELAXED);

    master_item2 = dictionary_set_and_acquire_item(master, "KEY 1", "VALUE2", strlen("VALUE2") + 1);
    view_item2 = dictionary_view_set_and_acquire_item(view, "KEY 1 ON VIEW", master_item2);

    if(!view_item2) {
        fprintf(stderr, "View replacement returned NULL\n");
        errors++;
    }
    else if(item_flag_check(view_item2, ITEM_FLAG_DELETED) || view_item2->shared != master_item2->shared) {
        fprintf(stderr, "View replacement returned stale/deleted item\n");
        dictionary_acquired_item_release(view, view_item2);
        errors++;
    }
    else
        dictionary_acquired_item_release(view, view_item2);

    lookup = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(view, "KEY 1 ON VIEW");
    if(!lookup || item_flag_check(lookup, ITEM_FLAG_DELETED) || lookup->shared != master_item2->shared) {
        fprintf(stderr, "View lookup did not resolve to the replacement item\n");
        errors++;
    }
    if(lookup)
        dictionary_acquired_item_release(view, lookup);

    dictionary_acquired_item_release(master, master_item2);
    dictionary_acquired_item_release(master, item1_on_master);

    dictionary_destroy(master);
    dictionary_destroy(view);
    return errors;
}

// ----------------------------------------------------------------------------
// Test: dictionary_destroy() TOCTOU race
//
// Stress test for the race where dictionary_destroy() could start force-freeing
// a dictionary while concurrent get/set/traversal operations were still able
// to enter through stale pre-lock destroyed-state checks.
//
// The test runs the racy workload in a forked child process so that a crash
// is detected as a child signal instead of bringing down the test harness.

#ifndef OS_WINDOWS
#include <sys/wait.h>
#endif

#ifndef OS_WINDOWS

struct dict_destroy_race_data {
    DICTIONARY *dict;
    int ready;          // atomic: worker signals it is looping
    int stop;           // atomic: main tells worker to stop
};

// Worker that continuously acquires and releases an item.
// Keep the reference briefly so destroy() can observe an in-flight access in
// the post-index-teardown recheck without forcing the old "already referenced"
// path up front.
static void dict_destroy_race_getter_thread(void *arg) {
    struct dict_destroy_race_data *d = arg;

    __atomic_store_n(&d->ready, 1, __ATOMIC_RELEASE);

    while(!__atomic_load_n(&d->stop, __ATOMIC_RELAXED)) {
        DICTIONARY_ITEM *item = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(d->dict, "key");
        if(item) {
            const char *val = dictionary_acquired_item_value(item);
            if(val) {
                volatile char c __attribute__((unused)) = val[0];
            }
            tinysleep();
            dictionary_acquired_item_release(d->dict, item);
        }
    }
}

// Worker that continuously sets (inserts/updates) items.
static void dict_destroy_race_setter_thread(void *arg) {
    struct dict_destroy_race_data *d = arg;

    __atomic_store_n(&d->ready, 1, __ATOMIC_RELEASE);

    int counter = 0;
    while(!__atomic_load_n(&d->stop, __ATOMIC_RELAXED)) {
        char key[32], val[32];
        // Use unique keys so the test exercises concurrent inserts during
        // destruction without racing on value replacement semantics.
        snprintfz(key, sizeof(key), "key-%d", counter);
        snprintfz(val, sizeof(val), "val-%d", counter);
        dictionary_set(d->dict, key, val, strlen(val) + 1);
        counter++;
    }
}

// Worker that continuously traverses (dfe_start_read / dfe_done).
static void dict_destroy_race_traverser_thread(void *arg) {
    struct dict_destroy_race_data *d = arg;

    __atomic_store_n(&d->ready, 1, __ATOMIC_RELEASE);

    while(!__atomic_load_n(&d->stop, __ATOMIC_RELAXED)) {
        void *val;
        dfe_start_read(d->dict, val) {
            if(val) {
                volatile char c __attribute__((unused)) = ((const char *)val)[0];
            }
        }
        dfe_done(val);
    }
}

// Run the racy workload in a child process: concurrent get/set/traverse
// while the main thread destroys the dictionary. Without the fix this may
// crash or trip internal consistency checks, depending on timing.
static void dict_destroy_race_child(int iterations) {
    for(int i = 0; i < iterations; i++) {
        DICTIONARY *dict = dictionary_create(DICT_OPTION_NONE);
        dictionary_set(dict, "key", "value", 6);

        struct dict_destroy_race_data getter_data = { .dict = dict, .ready = 0, .stop = 0 };
        struct dict_destroy_race_data setter_data = { .dict = dict, .ready = 0, .stop = 0 };
        struct dict_destroy_race_data traverser_data = { .dict = dict, .ready = 0, .stop = 0 };

        ND_THREAD *getter = nd_thread_create(
            "race-getter", NETDATA_THREAD_OPTION_DONT_LOG,
            dict_destroy_race_getter_thread, &getter_data);

        ND_THREAD *setter = nd_thread_create(
            "race-setter", NETDATA_THREAD_OPTION_DONT_LOG,
            dict_destroy_race_setter_thread, &setter_data);

        ND_THREAD *traverser = nd_thread_create(
            "race-trav", NETDATA_THREAD_OPTION_DONT_LOG,
            dict_destroy_race_traverser_thread, &traverser_data);

        if(!getter || !setter || !traverser) {
            // Thread creation failed — stop any that did start and clean up.
            __atomic_store_n(&getter_data.stop, 1, __ATOMIC_RELEASE);
            __atomic_store_n(&setter_data.stop, 1, __ATOMIC_RELEASE);
            __atomic_store_n(&traverser_data.stop, 1, __ATOMIC_RELEASE);
            if(getter) nd_thread_join(getter);
            if(setter) nd_thread_join(setter);
            if(traverser) nd_thread_join(traverser);
            dictionary_destroy(dict);
            cleanup_destroyed_dictionaries(false);
            _exit(2);
        }

        // wait for all workers to be running
        while(!__atomic_load_n(&getter_data.ready, __ATOMIC_ACQUIRE) ||
              !__atomic_load_n(&setter_data.ready, __ATOMIC_ACQUIRE) ||
              !__atomic_load_n(&traverser_data.ready, __ATOMIC_ACQUIRE))
            tinysleep();

        tinysleep();

        // Do not hold a permanent acquired item here: that would force the old
        // "already referenced" delayed-destroy path before destroy() reaches
        // the new destroyed-flag + index-teardown synchronization. Instead,
        // rely on the active workers to create transient in-flight accesses
        // while destroy() races with get/set/traversal.
        dictionary_destroy(dict);

        __atomic_store_n(&getter_data.stop, 1, __ATOMIC_RELEASE);
        __atomic_store_n(&setter_data.stop, 1, __ATOMIC_RELEASE);
        __atomic_store_n(&traverser_data.stop, 1, __ATOMIC_RELEASE);
        nd_thread_join(getter);
        nd_thread_join(setter);
        nd_thread_join(traverser);

        cleanup_destroyed_dictionaries(false);
    }
}

static int dictionary_destroy_race_unittest(void) {
    const int iterations = nd_is_running_under_ci() ? 10 : 100;

    fprintf(stderr,
            "\nTesting dictionary_destroy() TOCTOU race (%d iterations in child process)...\n",
            iterations);

    fflush(stderr);
    fflush(stdout);

    pid_t pid = fork();
    if(pid == 0) {
        // child — run the racy workload
        dict_destroy_race_child(iterations);
        _exit(0);
    }

    if(pid < 0) {
        fprintf(stderr, "dictionary_destroy() TOCTOU race test: fork() failed: %s\n",
                strerror(errno));
        return 1;
    }

    // Give the child a generous timeout so a hang doesn't stall the suite.
    int timeout_sec = 120;
    int status = 0;
    bool reaped = false;
    for(int elapsed = 0; elapsed < timeout_sec; elapsed++) {
        pid_t rc = waitpid(pid, &status, WNOHANG);
        if(rc > 0) { reaped = true; break; }
        if(rc < 0) {
            if(errno == EINTR)
                continue;
            fprintf(stderr, "dictionary_destroy() TOCTOU race test: waitpid() failed: %s\n",
                    strerror(errno));
            return 1;
        }
        sleep_usec(USEC_PER_SEC);
    }
    if(!reaped) {
        kill(pid, SIGKILL);
        while(waitpid(pid, &status, 0) < 0) {
            if(errno != EINTR) {
                fprintf(stderr, "dictionary_destroy() TOCTOU race test: waitpid() failed after SIGKILL: %s\n",
                        strerror(errno));
                return 1;
            }
        }
        fprintf(stderr, "dictionary_destroy() TOCTOU race test: FAILED — "
                "child hung (killed after %d seconds)\n", timeout_sec);
        return 1;
    }

    if(WIFSIGNALED(status)) {
        int sig = WTERMSIG(status);
        fprintf(stderr,
                "dictionary_destroy() TOCTOU race test: FAILED — "
                "child killed by signal %d (%s) — "
                "dictionary_destroy() still has a TOCTOU in its destroy/access "
                "synchronization path\n",
                sig, strsignal(sig));
        return 1;
    }

    if(WIFEXITED(status) && WEXITSTATUS(status) != 0) {
        fprintf(stderr,
                "dictionary_destroy() TOCTOU race test: FAILED — "
                "child exited with status %d\n",
                WEXITSTATUS(status));
        return 1;
    }

    fprintf(stderr, "dictionary_destroy() TOCTOU race test: OK\n");
    return 0;
}

#else

static int dictionary_destroy_race_unittest(void) {
    fprintf(stderr,
            "\nTesting dictionary_destroy() TOCTOU race: SKIPPED "
            "(fork-based test is unsupported on this platform)\n");
    return 0;
}

#endif

bool dictionary_traverse_or_destroy_unittest(void) {
    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_set(dict, "KEY 1", "VALUE1", strlen("VALUE1") + 1);
    dictionary_set(dict, "KEY 2", "VALUE2", strlen("VALUE2") + 1);
    dictionary_set(dict, "KEY 3", "VALUE3", strlen("VALUE3") + 1);

    size_t counted = 0;
    const char *s;
    dfe_start_read(dict, s) {
        if(!counted)
            dictionary_destroy(dict);

        counted++;
    }
    dfe_done(s);

    return counted == 1;
}

/*
 * FIXME: a dictionary-related leak is reported when running the address
 * sanitizer. Need to investigate if it's introduced by the unit-test itself,
 * or the dictionary implementation.
*/
// ============================================================================
// Dictionary benchmark harness for before/after concurrency work (for example
// RCU internals). Keep the workloads stable and the output compact.
// ============================================================================

#define DICT_BENCH_MAX_SAMPLES 4096
#define DICT_BENCH_SAMPLE_EVERY 64

typedef enum {
    DICT_BENCH_READ_NONE = 0,
    DICT_BENCH_READ_TRAVERSAL,
    DICT_BENCH_READ_LOOKUP_HOT,
    DICT_BENCH_READ_LOOKUP_RANDOM,
} dict_bench_read_mode_t;

typedef enum {
    DICT_BENCH_WRITE_NONE = 0,
    DICT_BENCH_WRITE_CHURN,
    DICT_BENCH_WRITE_UPDATE,
} dict_bench_write_mode_t;

struct dict_bench_config {
    const char *workload;
    size_t entries;
    int readers;
    int writers;
    time_t seconds_to_run;
    dict_bench_read_mode_t read_mode;
    dict_bench_write_mode_t write_mode;
};

struct dict_bench_stats {
    uint64_t ops;
    uint64_t items_seen;
    uint64_t latency_total_ut;
    size_t latency_samples_used;
    uint64_t latency_samples_seen;
    usec_t latency_samples[DICT_BENCH_MAX_SAMPLES];
};

struct dict_bench_thread {
    int id;
    int *join;
    DICTIONARY *dict;
    const struct dict_bench_config *cfg;
    struct dict_bench_stats stats;
    bool is_writer;
    uint32_t rng_state;
    ND_THREAD *thread;
};

struct dict_bench_summary {
    uint64_t read_ops;
    uint64_t read_items_seen;
    uint64_t write_ops;
    double read_avg_ut;
    double read_p99_ut;
    double write_avg_ut;
    double write_p99_ut;
};

static int dict_bench_usec_cmp(const void *a, const void *b) {
    const usec_t ua = *(const usec_t *)a;
    const usec_t ub = *(const usec_t *)b;
    return (ua > ub) - (ua < ub);
}

static inline uint32_t dict_bench_rand(uint32_t *state) {
    if(!*state) *state = 1;
    *state ^= *state << 13;
    *state ^= *state >> 17;
    *state ^= *state << 5;
    return *state;
}

static inline void dict_bench_record_latency(struct dict_bench_stats *stats, usec_t latency_ut, uint32_t *rng) {
    stats->latency_total_ut += latency_ut;
    stats->latency_samples_seen++;

    if(stats->latency_samples_used < DICT_BENCH_MAX_SAMPLES)
        stats->latency_samples[stats->latency_samples_used++] = latency_ut;
    else {
        // reservoir sampling: replace a random slot with probability N/total_seen
        uint32_t idx = dict_bench_rand(rng) % stats->latency_samples_seen;
        if(idx < DICT_BENCH_MAX_SAMPLES)
            stats->latency_samples[idx] = latency_ut;
    }
}

static void dict_bench_lookup_key(char *buf, size_t len, size_t key_idx) {
    snprintfz(buf, len, "bench-item-%zu", key_idx);
}

static void dict_bench_reader_thread(void *arg) {
    struct dict_bench_thread *ctx = arg;
    const size_t hot_keys = MIN(ctx->cfg->entries, (size_t)64);

    while(!__atomic_load_n(ctx->join, __ATOMIC_RELAXED)) {
        usec_t started_ut = 0;
        bool sample_latency = ((ctx->stats.ops & (DICT_BENCH_SAMPLE_EVERY - 1)) == 0);

        if(sample_latency)
            started_ut = now_monotonic_usec();

        if(ctx->cfg->read_mode == DICT_BENCH_READ_TRAVERSAL) {
            void *v;
            dfe_start_read(ctx->dict, v) {
                (void)v;
                ctx->stats.items_seen++;
            }
            dfe_done(v);
        }
        else {
            char name[64];
            size_t key_idx;

            if(ctx->cfg->read_mode == DICT_BENCH_READ_LOOKUP_HOT)
                key_idx = dict_bench_rand(&ctx->rng_state) % hot_keys;
            else
                key_idx = dict_bench_rand(&ctx->rng_state) % ctx->cfg->entries;

            dict_bench_lookup_key(name, sizeof(name), key_idx);
            (void)dictionary_get(ctx->dict, name);
        }

        ctx->stats.ops++;

        if(sample_latency)
            dict_bench_record_latency(&ctx->stats, now_monotonic_usec() - started_ut, &ctx->rng_state);
    }
}

static void dict_bench_writer_thread(void *arg) {
    struct dict_bench_thread *ctx = arg;
    uint64_t counter = 0;

    while(!__atomic_load_n(ctx->join, __ATOMIC_RELAXED)) {
        usec_t started_ut = 0;
        bool sample_latency = ((ctx->stats.ops & (DICT_BENCH_SAMPLE_EVERY - 1)) == 0);

        if(sample_latency)
            started_ut = now_monotonic_usec();

        if(ctx->cfg->write_mode == DICT_BENCH_WRITE_CHURN) {
            char buf[64];
            snprintfz(buf, sizeof(buf), "writer-key-%d-%"PRIu64, ctx->id, counter);
            dictionary_set(ctx->dict, buf, NULL, 0);
            dictionary_del(ctx->dict, buf);
        }
        else {
            char buf[64];
            size_t key_idx = dict_bench_rand(&ctx->rng_state) % ctx->cfg->entries;
            uint64_t value = counter;

            dict_bench_lookup_key(buf, sizeof(buf), key_idx);
            dictionary_set(ctx->dict, buf, &value, sizeof(value));
        }

        counter++;
        ctx->stats.ops++;

        if(sample_latency)
            dict_bench_record_latency(&ctx->stats, now_monotonic_usec() - started_ut, &ctx->rng_state);
    }
}

static void dict_bench_prepopulate(DICTIONARY *dict, size_t entries) {
    char name[64];

    for(size_t i = 0; i < entries; i++) {
        uint64_t value = i;
        dict_bench_lookup_key(name, sizeof(name), i);
        dictionary_set(dict, name, &value, sizeof(value));
    }
}

static double dict_bench_percentile_ut(usec_t *samples, size_t samples_used, size_t percentile) {
    if(!samples_used)
        return 0.0;

    qsort(samples, samples_used, sizeof(*samples), dict_bench_usec_cmp);

    size_t idx = ((samples_used - 1) * percentile) / 100;
    return (double)samples[idx];
}

static void dict_bench_aggregate_latency(
    struct dict_bench_thread *threads,
    int count,
    bool writers,
    double *avg_ut,
    double *p99_ut
) {
    size_t max_samples = (size_t)DICT_BENCH_MAX_SAMPLES * count;
    usec_t *samples = callocz(max_samples, sizeof(usec_t));
    size_t samples_used = 0;
    uint64_t total_latency_ut = 0;
    uint64_t total_ops = 0;

    for(int i = 0; i < count; i++) {
        if(threads[i].is_writer != writers)
            continue;

        total_latency_ut += threads[i].stats.latency_total_ut;
        total_ops += threads[i].stats.latency_samples_seen;

        size_t available = max_samples - samples_used;
        size_t copy = MIN(available, threads[i].stats.latency_samples_used);
        if(copy) {
            memcpy(&samples[samples_used], threads[i].stats.latency_samples, copy * sizeof(usec_t));
            samples_used += copy;
        }
    }

    *avg_ut = total_ops ? (double)total_latency_ut / (double)total_ops : 0.0;
    *p99_ut = dict_bench_percentile_ut(samples, samples_used, 99);
    freez(samples);
}

static void dict_bench_run_case(const struct dict_bench_config *cfg) {
    int total_threads = cfg->readers + cfg->writers;
    int join = 0;
    struct dict_bench_thread *threads = callocz(total_threads, sizeof(*threads));
    struct dict_bench_summary summary = {0};
    DICTIONARY *dict = dictionary_create(DICT_OPTION_NONE);
    dict_bench_prepopulate(dict, cfg->entries);

    for(int i = 0; i < cfg->readers; i++) {
        char tname[32];
        threads[i] = (struct dict_bench_thread){
            .id = i,
            .join = &join,
            .dict = dict,
            .cfg = cfg,
            .is_writer = false,
            .rng_state = (uint32_t)(i + 1) * 2654435761U,
        };
        snprintfz(tname, sizeof(tname), "dbread%d", i);
        threads[i].thread = nd_thread_create(tname, NETDATA_THREAD_OPTION_DONT_LOG,
                                             dict_bench_reader_thread, &threads[i]);
    }

    for(int i = 0; i < cfg->writers; i++) {
        int idx = cfg->readers + i;
        char tname[32];
        threads[idx] = (struct dict_bench_thread){
            .id = idx,
            .join = &join,
            .dict = dict,
            .cfg = cfg,
            .is_writer = true,
            .rng_state = (uint32_t)(idx + 1) * 2246822519U,
        };
        snprintfz(tname, sizeof(tname), "dbwrite%d", i);
        threads[idx].thread = nd_thread_create(tname, NETDATA_THREAD_OPTION_DONT_LOG,
                                               dict_bench_writer_thread, &threads[idx]);
    }

    sleep_usec(cfg->seconds_to_run * USEC_PER_SEC);
    __atomic_store_n(&join, 1, __ATOMIC_RELAXED);

    for(int i = 0; i < total_threads; i++) {
        nd_thread_join(threads[i].thread);
        if(threads[i].is_writer)
            summary.write_ops += threads[i].stats.ops;
        else {
            summary.read_ops += threads[i].stats.ops;
            summary.read_items_seen += threads[i].stats.items_seen;
        }
    }

    dict_bench_aggregate_latency(threads, total_threads, false, &summary.read_avg_ut, &summary.read_p99_ut);
    dict_bench_aggregate_latency(threads, total_threads, true, &summary.write_avg_ut, &summary.write_p99_ut);
    dictionary_destroy(dict);
    cleanup_destroyed_dictionaries(false);
    freez(threads);

    if(cfg->read_mode == DICT_BENCH_READ_TRAVERSAL) {
        fprintf(stderr, "%-14s %8zu %8d %8d %14.0f %14.0f %14.0f %14.2f %14.2f %14.2f %14.2f\n",
                cfg->workload,
                cfg->entries,
                cfg->readers,
                cfg->writers,
                cfg->seconds_to_run ? (double)summary.read_ops / cfg->seconds_to_run : 0.0,
                cfg->seconds_to_run ? (double)summary.read_items_seen / cfg->seconds_to_run : 0.0,
                cfg->seconds_to_run ? (double)summary.write_ops / cfg->seconds_to_run : 0.0,
                summary.read_avg_ut,
                summary.read_p99_ut,
                summary.write_avg_ut,
                summary.write_p99_ut);
    }
    else {
        fprintf(stderr, "%-14s %8zu %8d %8d %14.0f %14.0f %14.2f %14.2f %14.2f %14.2f\n",
                cfg->workload,
                cfg->entries,
                cfg->readers,
                cfg->writers,
                cfg->seconds_to_run ? (double)summary.read_ops / cfg->seconds_to_run : 0.0,
                cfg->seconds_to_run ? (double)summary.write_ops / cfg->seconds_to_run : 0.0,
                summary.read_avg_ut,
                summary.read_p99_ut,
                summary.write_avg_ut,
                summary.write_p99_ut);
    }
}

static void dict_bench_print_separator(size_t width) {
    for(size_t i = 0; i < width; i++)
        fputc('-', stderr);
    fputc('\n', stderr);
}

static void dict_bench_print_header_line(const char *line) {
    fprintf(stderr, "%s\n", line);
    dict_bench_print_separator(strlen(line));
}

static void dict_bench_print_suite_header(
    const char *suite,
    const char *read_ops_label,
    const char *read_avg_label,
    const char *read_slow_label,
    const char *writer_desc
) {
    char header[512];

    fprintf(stderr, "\n=== %s ===\n", suite);
    fprintf(stderr, "%s\n", writer_desc);
    snprintfz(header, sizeof(header),
              "%-14s %8s %8s %8s %14s %14s %14s %14s %14s %14s",
              "workload", "entries", "readers", "writers",
              read_ops_label, "write ops/s",
              read_avg_label, read_slow_label, "w avg us", "w slow us");
    dict_bench_print_header_line(header);
}

static void dict_bench_print_traversal_header(void) {
    char header[512];

    fprintf(stderr, "\n=== Dictionary Traversal Benchmark ===\n");
    fprintf(stderr, "Reader workload: full dictionary scan. Writer workload: temporary-key insert followed by delete.\n");
    snprintfz(header, sizeof(header),
              "%-14s %8s %8s %8s %14s %14s %14s %14s %14s %14s %14s",
              "workload", "entries", "readers", "writers",
              "full scans/s", "items visited/s", "write ops/s",
              "avg scan us", "slow scan us", "w avg us", "w slow us");
    dict_bench_print_header_line(header);
}

int dictionary_unittest_benchmark(void) {
    const time_t seconds_to_run = 2;
    const size_t sizes[] = {100, 10000};
    const int readers[] = {1, 4, 8};
    const int writers[] = {0, 1, 2};

    dict_bench_print_traversal_header();
    for(size_t i = 0; i < sizeof(readers) / sizeof(readers[0]); i++) {
        for(size_t j = 0; j < sizeof(writers) / sizeof(writers[0]); j++) {
            struct dict_bench_config cfg = {
                .workload = "traversal",
                .entries = 10000,
                .readers = readers[i],
                .writers = writers[j],
                .seconds_to_run = seconds_to_run,
                .read_mode = DICT_BENCH_READ_TRAVERSAL,
                .write_mode = DICT_BENCH_WRITE_CHURN,
            };
            dict_bench_run_case(&cfg);
        }
    }

    dict_bench_print_suite_header(
        "Dictionary Lookup Benchmark",
        "lookups/s",
        "avg lookup us",
        "slow lookup us",
        "Reader workload: dictionary_get(). Writer workload: overwrite an existing dictionary entry."
    );
    for(size_t s = 0; s < sizeof(sizes) / sizeof(sizes[0]); s++) {
        for(size_t i = 0; i < sizeof(readers) / sizeof(readers[0]); i++) {
            for(size_t j = 0; j < sizeof(writers) / sizeof(writers[0]); j++) {
                struct dict_bench_config hot_cfg = {
                    .workload = "lookup-hot",
                    .entries = sizes[s],
                    .readers = readers[i],
                    .writers = writers[j],
                    .seconds_to_run = seconds_to_run,
                    .read_mode = DICT_BENCH_READ_LOOKUP_HOT,
                    .write_mode = DICT_BENCH_WRITE_UPDATE,
                };
                struct dict_bench_config random_cfg = hot_cfg;
                random_cfg.workload = "lookup-random";
                random_cfg.read_mode = DICT_BENCH_READ_LOOKUP_RANDOM;

                dict_bench_run_case(&hot_cfg);
                dict_bench_run_case(&random_cfg);
            }
        }
    }

    dict_bench_print_suite_header(
        "Dictionary Mixed RW Benchmark",
        "read ops/s",
        "read avg us",
        "slow read us",
        "Reader workload: random lookups. Writer workload depends on the row: update rewrites existing keys, churn inserts then deletes temporary keys."
    );
    {
        const struct dict_bench_config configs[] = {
            {.workload = "mixed-8r1w", .entries = 10000, .readers = 8, .writers = 1, .seconds_to_run = seconds_to_run, .read_mode = DICT_BENCH_READ_LOOKUP_RANDOM, .write_mode = DICT_BENCH_WRITE_UPDATE},
            {.workload = "mixed-8r2w", .entries = 10000, .readers = 8, .writers = 2, .seconds_to_run = seconds_to_run, .read_mode = DICT_BENCH_READ_LOOKUP_RANDOM, .write_mode = DICT_BENCH_WRITE_UPDATE},
            {.workload = "mixed-4r1w", .entries = 10000, .readers = 4, .writers = 1, .seconds_to_run = seconds_to_run, .read_mode = DICT_BENCH_READ_LOOKUP_RANDOM, .write_mode = DICT_BENCH_WRITE_CHURN},
            {.workload = "mixed-4r2w", .entries = 10000, .readers = 4, .writers = 2, .seconds_to_run = seconds_to_run, .read_mode = DICT_BENCH_READ_LOOKUP_RANDOM, .write_mode = DICT_BENCH_WRITE_CHURN},
        };

        for(size_t i = 0; i < sizeof(configs) / sizeof(configs[0]); i++)
            dict_bench_run_case(&configs[i]);
    }

    dict_bench_print_suite_header(
        "Dictionary Writer Cost Benchmark",
        "read ops/s",
        "read avg us",
        "slow read us",
        "Writer workload: 'update' overwrites existing keys, 'churn' inserts then deletes temporary keys; '+r' rows include background readers."
    );
    {
        const struct dict_bench_config configs[] = {
            {.workload = "update-1w", .entries = 10000, .readers = 0, .writers = 1, .seconds_to_run = seconds_to_run, .read_mode = DICT_BENCH_READ_NONE, .write_mode = DICT_BENCH_WRITE_UPDATE},
            {.workload = "update-2w", .entries = 10000, .readers = 0, .writers = 2, .seconds_to_run = seconds_to_run, .read_mode = DICT_BENCH_READ_NONE, .write_mode = DICT_BENCH_WRITE_UPDATE},
            {.workload = "churn-1w",  .entries = 10000, .readers = 0, .writers = 1, .seconds_to_run = seconds_to_run, .read_mode = DICT_BENCH_READ_NONE, .write_mode = DICT_BENCH_WRITE_CHURN},
            {.workload = "churn-2w",  .entries = 10000, .readers = 0, .writers = 2, .seconds_to_run = seconds_to_run, .read_mode = DICT_BENCH_READ_NONE, .write_mode = DICT_BENCH_WRITE_CHURN},
            {.workload = "update+r",  .entries = 10000, .readers = 8, .writers = 1, .seconds_to_run = seconds_to_run, .read_mode = DICT_BENCH_READ_LOOKUP_RANDOM, .write_mode = DICT_BENCH_WRITE_UPDATE},
            {.workload = "churn+r",   .entries = 10000, .readers = 8, .writers = 2, .seconds_to_run = seconds_to_run, .read_mode = DICT_BENCH_READ_LOOKUP_RANDOM, .write_mode = DICT_BENCH_WRITE_CHURN},
        };

        for(size_t i = 0; i < sizeof(configs) / sizeof(configs[0]); i++)
            dict_bench_run_case(&configs[i]);
    }

    fprintf(stderr, "\n");
    return 0;
}

int dictionary_unittest(size_t entries) {
    if(entries < 10) entries = 10;

    DICTIONARY *dict;
    size_t errors = 0;

    fprintf(stderr, "Generating %zu names and values...\n", entries);
    char **names = dictionary_unittest_generate_names(entries);
    char **values = dictionary_unittest_generate_values(entries);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_unittest_clone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary multi threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_NONE);
    dictionary_unittest_clone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary single threaded, non-clone, add-in-front options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE |
        DICT_OPTION_ADD_IN_FRONT);
    dictionary_unittest_nonclone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary multi threaded, non-clone, add-in-front options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_ADD_IN_FRONT);
    dictionary_unittest_nonclone(dict, names, values, entries, &errors);

    fprintf(stderr, "\nCreating dictionary single-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_SINGLE_THREADED | DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE |
        DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "resetting non-overwrite entries", names, values, entries, &errors, dictionary_unittest_reset_dont_overwrite_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop", names, values, entries, &errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback", names, values, entries, &errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback stop", names, values, entries, &errors, dictionary_unittest_walkthrough_stop);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary multi-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough write delete this", names, values, entries, &errors, dictionary_unittest_walkthrough_delete_this);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary multi-threaded, non-clone, don't overwrite options, %zu items\n", entries);
    dict = dictionary_create(
        DICT_OPTION_NAME_LINK_DONT_CLONE | DICT_OPTION_VALUE_LINK_DONT_CLONE | DICT_OPTION_DONT_OVERWRITE_VALUE);
    dictionary_unittest_run_and_measure_time(dict, "adding entries", names, values, entries, &errors, dictionary_unittest_set_nonclone);
    dictionary_unittest_run_and_measure_time(dict, "foreach write delete this", names, values, entries, &errors, dictionary_unittest_foreach_delete_this);
    dictionary_unittest_run_and_measure_time(dict, "traverse foreach read loop empty", names, values, 0, &errors, dictionary_unittest_foreach);
    dictionary_unittest_run_and_measure_time(dict, "walkthrough read callback empty", names, values, 0, &errors, dictionary_unittest_walkthrough);
    dictionary_unittest_run_and_measure_time(dict, "destroying empty dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_unittest_sorting(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, clone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    dictionary_unittest_null_dfe(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    fprintf(stderr, "\nCreating dictionary single threaded, noclone, %zu items\n", entries);
    dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_VALUE_LINK_DONT_CLONE);
    dictionary_unittest_null_dfe(dict, names, values, entries, &errors);
    dictionary_unittest_run_and_measure_time(dict, "destroying full dictionary", names, values, entries, &errors, dictionary_unittest_destroy);

    // check reference counters
    {
        fprintf(stderr, "\nTesting reference counters:\n");
        dict = dictionary_create(DICT_OPTION_NONE | DICT_OPTION_NAME_LINK_DONT_CLONE);
        errors += unittest_check_dictionary("", dict, 0, 0, 0, 0, 0);

        fprintf(stderr, "\nAdding test item to dictionary and acquiring it\n");
        dictionary_set(dict, "test", "ITEM1", 6);
        DICTIONARY_ITEM *item = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(dict, "test");

        errors += unittest_check_dictionary("", dict, 1, 1, 0, 1, 0);
        errors += unittest_check_item("ACQUIRED", dict, item, "test", "ITEM1", 1, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nChecking that reference counters are increased:\n");
        void *t;
        dfe_start_read(dict, t) {
            errors += unittest_check_dictionary("", dict, 1, 1, 0, 1, 0);
            errors += unittest_check_item("ACQUIRED TRAVERSAL", dict, item, "test", "ITEM1", 2, ITEM_FLAG_NONE, true, true, true);
        }
        dfe_done(t);

        fprintf(stderr, "\nChecking that reference counters are decreased:\n");
        errors += unittest_check_dictionary("", dict, 1, 1, 0, 1, 0);
        errors += unittest_check_item("ACQUIRED TRAVERSAL 2", dict, item, "test", "ITEM1", 1, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nDeleting the item we have acquired:\n");
        dictionary_del(dict, "test");

        errors += unittest_check_dictionary("", dict, 0, 0, 1, 1, 0);
        errors += unittest_check_item("DELETED", dict, item, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);

        fprintf(stderr, "\nAdding another item with the same name of the item we deleted, while being acquired:\n");
        dictionary_set(dict, "test", "ITEM2", 6);
        errors += unittest_check_dictionary("", dict, 1, 1, 1, 1, 0);

        fprintf(stderr, "\nAcquiring the second item:\n");
        DICTIONARY_ITEM *item2 = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(dict, "test");
        errors += unittest_check_item("FIRST", dict, item, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);
        errors += unittest_check_item("SECOND", dict, item2, "test", "ITEM2", 1, ITEM_FLAG_NONE, true, true, true);
        errors += unittest_check_dictionary("", dict, 1, 1, 1, 2, 0);

        fprintf(stderr, "\nReleasing the second item (the first is still acquired):\n");
        dictionary_acquired_item_release(dict, (DICTIONARY_ITEM *)item2);
        errors += unittest_check_dictionary("", dict, 1, 1, 1, 1, 0);
        errors += unittest_check_item("FIRST", dict, item, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);
        errors += unittest_check_item("SECOND RELEASED", dict, item2, "test", "ITEM2", 0, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nDeleting the second item (the first is still acquired):\n");
        dictionary_del(dict, "test");
        errors += unittest_check_dictionary("", dict, 0, 0, 1, 1, 0);
        errors += unittest_check_item("ACQUIRED DELETED", dict, item, "test", "ITEM1", 1, ITEM_FLAG_DELETED, false, false, true);

        fprintf(stderr, "\nReleasing the first item (which we have already deleted):\n");
        dictionary_acquired_item_release(dict, (DICTIONARY_ITEM *)item);
        dfe_start_write(dict, item) ; dfe_done(item);
        errors += unittest_check_dictionary("", dict, 0, 0, 1, 0, 1);

        fprintf(stderr, "\nAdding again the test item to dictionary and acquiring it\n");
        dictionary_set(dict, "test", "ITEM1", 6);
        item = (DICTIONARY_ITEM *)dictionary_get_and_acquire_item(dict, "test");

        errors += unittest_check_dictionary("", dict, 1, 1, 0, 1, 0);
        errors += unittest_check_item("RE-ADDITION", dict, item, "test", "ITEM1", 1, ITEM_FLAG_NONE, true, true, true);

        fprintf(stderr, "\nDestroying the dictionary while we have acquired an item\n");
        dictionary_destroy(dict);

        fprintf(stderr, "Releasing the item (on a destroyed dictionary)\n");
        dictionary_acquired_item_release(dict, (DICTIONARY_ITEM *)item);
        item = NULL;
        dict = NULL;
    }

    dictionary_unittest_free_char_pp(names, entries);
    dictionary_unittest_free_char_pp(values, entries);

    errors += dictionary_unittest_views();
    errors += dictionary_unittest_threads();
    errors += dictionary_unittest_view_threads();

    if(!dictionary_traverse_or_destroy_unittest()) {
        fprintf(stderr, "Destroy on traversal test failed\n");
        errors++;
    }
    else
        fprintf(stderr, "Destroy on traversal test OK\n");

    errors += dictionary_destroy_race_unittest();

    cleanup_destroyed_dictionaries(false);

    size_t delayed = dictionary_destroy_delayed_count();
    if(delayed != 0) {
        fprintf(stderr, "WARNING: There are %zu dictionaries that cannot be destroyed\n", delayed);
    }
    else
        fprintf(stderr, "All dictionaries have been freed: OK\n");

    fprintf(stderr, "\n%zu errors found\n", errors);
    return  errors ? 1 : 0;
}
