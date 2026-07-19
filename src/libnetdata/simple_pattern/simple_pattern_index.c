// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "simple_pattern_internals.h"

struct simple_pattern_index {
    RW_SPINLOCK rwlock;
    Pvoid_t keys;
};

_Static_assert(sizeof(void *) <= sizeof(Word_t), "SIMPLE_PATTERN_INDEX user pointers must fit in Word_t");

struct simple_pattern_index_matches {
    Pvoid_t users;
    size_t count;
};

typedef enum {
    SIMPLE_PATTERN_INDEX_UNSEEN = 0,
    SIMPLE_PATTERN_INDEX_POSITIVE = 1,
    SIMPLE_PATTERN_INDEX_NEGATIVE = 2,
} SIMPLE_PATTERN_INDEX_STATE;

typedef bool (*simple_pattern_exact_literal_cb_t)(
    const char *literal, size_t length, SIMPLE_PATTERN_RESULT result, void *data);

static bool simple_pattern_is_exact_case_sensitive(SIMPLE_PATTERN *pattern) {
    if(!pattern)
        return false;

    for(struct simple_pattern *rule = (struct simple_pattern *)pattern; rule; rule = rule->next) {
        if(rule->mode != SIMPLE_PATTERN_EXACT || rule->child || !rule->case_sensitive || !rule->match)
            return false;
    }

    return true;
}

static bool simple_pattern_foreach_exact_literal(
    SIMPLE_PATTERN *pattern, simple_pattern_exact_literal_cb_t callback, void *data)
{
    if(!callback || !simple_pattern_is_exact_case_sensitive(pattern))
        return false;

    for(struct simple_pattern *rule = (struct simple_pattern *)pattern; rule; rule = rule->next) {
        SIMPLE_PATTERN_RESULT result = rule->negative ? SP_MATCHED_NEGATIVE : SP_MATCHED_POSITIVE;
        if(!callback(rule->match, rule->len, result, data))
            return false;
    }

    return true;
}

// ----------------------------------------------------------------------------
// SIMPLE_PATTERN_INDEX

SIMPLE_PATTERN_INDEX *simple_pattern_index_create(void) {
    SIMPLE_PATTERN_INDEX *index = callocz(1, sizeof(*index));
    rw_spinlock_init(&index->rwlock);
    return index;
}

static inline bool simple_pattern_index_add_user_to_result(
    SIMPLE_PATTERN_INDEX_MATCHES *matches, Pvoid_t users, bool negative)
{
    Word_t user = 0;
    bool first = true;
    Pvoid_t *user_slot;

    while((user_slot = JudyLFirstThenNext(users, &user, &first))) {
        Pvoid_t *result_slot = JudyLIns(&matches->users, user, PJE0);
        if(unlikely(!result_slot || result_slot == PJERR))
            return false;

        SIMPLE_PATTERN_INDEX_STATE state = (SIMPLE_PATTERN_INDEX_STATE)(uintptr_t)*result_slot;
        if(negative)
            *result_slot = (Pvoid_t)(uintptr_t)SIMPLE_PATTERN_INDEX_NEGATIVE;
        else if(state != SIMPLE_PATTERN_INDEX_NEGATIVE)
            *result_slot = (Pvoid_t)(uintptr_t)SIMPLE_PATTERN_INDEX_POSITIVE;
    }

    return true;
}

static bool simple_pattern_index_add_unsafe(SIMPLE_PATTERN_INDEX *index, STRING *key, void *user) {
    bool added = false;
    Pvoid_t *key_slot = JudyLIns(&index->keys, (Word_t)key, PJE0);
    if(likely(key_slot && key_slot != PJERR)) {
        if(!*key_slot)
            string_dup(key);

        Pvoid_t *user_slot = JudyLIns(key_slot, (Word_t)user, PJE0);
        if(likely(user_slot && user_slot != PJERR)) {
            *user_slot = NULL;
            added = true;
        }

        if(unlikely(!*key_slot)) {
            string_freez(key);
            (void)JudyLDel(&index->keys, (Word_t)key, PJE0);
        }
    }

    return added;
}

bool simple_pattern_index_add(SIMPLE_PATTERN_INDEX *index, STRING *key, void *user) {
    if(unlikely(!index || !key || !user))
        return false;

    rw_spinlock_write_lock(&index->rwlock);
    bool added = simple_pattern_index_add_unsafe(index, key, user);
    rw_spinlock_write_unlock(&index->rwlock);
    return added;
}

bool simple_pattern_index_del(SIMPLE_PATTERN_INDEX *index, STRING *key, void *user) {
    if(unlikely(!index || !key || !user))
        return false;

    bool deleted = false;
    rw_spinlock_write_lock(&index->rwlock);

    Pvoid_t *key_slot = JudyLGet(index->keys, (Word_t)key, PJE0);
    if(likely(key_slot && key_slot != PJERR)) {
        deleted = JudyLDel(key_slot, (Word_t)user, PJE0) == 1;
        if(!*key_slot) {
            (void)JudyLDel(&index->keys, (Word_t)key, PJE0);
            string_freez(key);
        }
    }

    rw_spinlock_write_unlock(&index->rwlock);
    return deleted;
}

static size_t simple_pattern_index_del_user_unsafe(SIMPLE_PATTERN_INDEX *index, void *user) {
    size_t deleted = 0;
    Word_t key = 0;
    Pvoid_t *key_slot = JudyLFirst(index->keys, &key, PJE0);
    while(key_slot && key_slot != PJERR) {
        if(JudyLDel(key_slot, (Word_t)user, PJE0) == 1)
            deleted++;

        if(!*key_slot) {
            STRING *string = (STRING *)key;
            (void)JudyLDel(&index->keys, key, PJE0);
            string_freez(string);
        }

        key_slot = JudyLNext(index->keys, &key, PJE0);
    }

    return deleted;
}

size_t simple_pattern_index_del_user(SIMPLE_PATTERN_INDEX *index, void *user) {
    if(unlikely(!index || !user))
        return 0;

    rw_spinlock_write_lock(&index->rwlock);
    size_t deleted = simple_pattern_index_del_user_unsafe(index, user);
    rw_spinlock_write_unlock(&index->rwlock);
    return deleted;
}

bool simple_pattern_index_replace_user(
    SIMPLE_PATTERN_INDEX *index, STRING *const *keys, size_t keys_count, void *user)
{
    if(unlikely(!index || !user || (keys_count && !keys)))
        return false;

    for(size_t i = 0; i < keys_count; i++)
        if(unlikely(!keys[i]))
            return false;

    rw_spinlock_write_lock(&index->rwlock);
    (void)simple_pattern_index_del_user_unsafe(index, user);

    bool ok = true;
    for(size_t i = 0; i < keys_count; i++)
        if(unlikely(!simple_pattern_index_add_unsafe(index, keys[i], user))) {
            ok = false;
            break;
        }

    if(unlikely(!ok))
        (void)simple_pattern_index_del_user_unsafe(index, user);

    rw_spinlock_write_unlock(&index->rwlock);
    return ok;
}

void simple_pattern_index_destroy(SIMPLE_PATTERN_INDEX *index) {
    if(!index)
        return;

    rw_spinlock_write_lock(&index->rwlock);

    Word_t key = 0;
    bool first = true;
    Pvoid_t *key_slot;
    while((key_slot = JudyLFirstThenNext(index->keys, &key, &first))) {
        JudyLFreeArray(key_slot, PJE0);
        string_freez((STRING *)key);
    }
    JudyLFreeArray(&index->keys, PJE0);

    rw_spinlock_write_unlock(&index->rwlock);
    freez(index);
}

struct simple_pattern_index_exact_search {
    SIMPLE_PATTERN_INDEX *index;
    SIMPLE_PATTERN_INDEX_MATCHES *matches;
    Pvoid_t visited;
};

static bool simple_pattern_index_search_exact_literal(
    const char *literal, size_t length, SIMPLE_PATTERN_RESULT result, void *data)
{
    struct simple_pattern_index_exact_search *search = data;
    STRING *key = string_acquire_existing(literal, length);
    if(!key)
        return true;

    Pvoid_t *visited_slot = JudyLGet(search->visited, (Word_t)key, PJE0);
    if(visited_slot && visited_slot != PJERR) {
        string_freez(key);
        return true;
    }

    visited_slot = JudyLIns(&search->visited, (Word_t)key, PJE0);
    if(unlikely(!visited_slot || visited_slot == PJERR)) {
        string_freez(key);
        return false;
    }
    *visited_slot = (Pvoid_t)1;

    Pvoid_t *key_slot = JudyLGet(search->index->keys, (Word_t)key, PJE0);
    bool ok = true;
    if(key_slot && key_slot != PJERR)
        ok = simple_pattern_index_add_user_to_result(
            search->matches, *key_slot, result == SP_MATCHED_NEGATIVE);

    string_freez(key);
    return ok;
}

SIMPLE_PATTERN_INDEX_MATCHES *simple_pattern_index_search(SIMPLE_PATTERN_INDEX *index, SIMPLE_PATTERN *pattern) {
    SIMPLE_PATTERN_INDEX_MATCHES *matches = callocz(1, sizeof(*matches));

    if(unlikely(!index))
        return matches;

    rw_spinlock_read_lock(&index->rwlock);

    bool ok = true;
    if(pattern && simple_pattern_is_exact_case_sensitive(pattern)) {
        struct simple_pattern_index_exact_search search = {
            .index = index,
            .matches = matches,
        };
        ok = simple_pattern_foreach_exact_literal(pattern, simple_pattern_index_search_exact_literal, &search);
        JudyLFreeArray(&search.visited, PJE0);
    }
    else if(pattern) {
        Word_t key = 0;
        bool first = true;
        Pvoid_t *key_slot;
        while((key_slot = JudyLFirstThenNext(index->keys, &key, &first))) {
            STRING *string = (STRING *)key;
            SIMPLE_PATTERN_RESULT result = simple_pattern_matches_string_extract(pattern, string, NULL, 0);
            if(result != SP_NOT_MATCHED &&
               !simple_pattern_index_add_user_to_result(matches, *key_slot, result == SP_MATCHED_NEGATIVE)) {
                ok = false;
                break;
            }
        }
    }
    else {
        Word_t key = 0;
        bool first = true;
        Pvoid_t *key_slot;
        while((key_slot = JudyLFirstThenNext(index->keys, &key, &first))) {
            if(unlikely(!simple_pattern_index_add_user_to_result(matches, *key_slot, false))) {
                ok = false;
                break;
            }
        }
    }

    rw_spinlock_read_unlock(&index->rwlock);

    if(unlikely(!ok)) {
        simple_pattern_index_matches_free(matches);
        return NULL;
    }

    Word_t user = 0;
    bool first = true;
    Pvoid_t *user_slot;
    while((user_slot = JudyLFirstThenNext(matches->users, &user, &first))) {
        if((SIMPLE_PATTERN_INDEX_STATE)(uintptr_t)*user_slot == SIMPLE_PATTERN_INDEX_POSITIVE)
            matches->count++;
    }

    return matches;
}

bool simple_pattern_index_matches_contains(SIMPLE_PATTERN_INDEX_MATCHES *matches, const void *user) {
    if(unlikely(!matches || !user))
        return false;

    Pvoid_t *slot = JudyLGet(matches->users, (Word_t)user, PJE0);
    return slot && slot != PJERR &&
           (SIMPLE_PATTERN_INDEX_STATE)(uintptr_t)*slot == SIMPLE_PATTERN_INDEX_POSITIVE;
}

static void *simple_pattern_index_matches_seek(
    SIMPLE_PATTERN_INDEX_MATCHES *matches, Word_t *cursor, bool first)
{
    if(unlikely(!matches || !cursor))
        return NULL;

    Pvoid_t *slot = first ? JudyLFirst(matches->users, cursor, PJE0) : JudyLNext(matches->users, cursor, PJE0);
    while(slot && slot != PJERR) {
        if((SIMPLE_PATTERN_INDEX_STATE)(uintptr_t)*slot == SIMPLE_PATTERN_INDEX_POSITIVE)
            return (void *)*cursor;

        slot = JudyLNext(matches->users, cursor, PJE0);
    }

    return NULL;
}

void *simple_pattern_index_matches_first(SIMPLE_PATTERN_INDEX_MATCHES *matches, Word_t *cursor) {
    return simple_pattern_index_matches_seek(matches, cursor, true);
}

void *simple_pattern_index_matches_next(SIMPLE_PATTERN_INDEX_MATCHES *matches, Word_t *cursor) {
    return simple_pattern_index_matches_seek(matches, cursor, false);
}

size_t simple_pattern_index_matches_count(SIMPLE_PATTERN_INDEX_MATCHES *matches) {
    return matches ? matches->count : 0;
}

void simple_pattern_index_matches_free(SIMPLE_PATTERN_INDEX_MATCHES *matches) {
    if(!matches)
        return;

    JudyLFreeArray(&matches->users, PJE0);
    freez(matches);
}

// ----------------------------------------------------------------------------
// SIMPLE_PATTERN_INDEX tests

static bool simple_pattern_index_test_add(SIMPLE_PATTERN_INDEX *index, const char *key, void *user) {
    STRING *string = string_strdupz(key);
    bool ok = simple_pattern_index_add(index, string, user);
    string_freez(string);
    return ok;
}

static size_t simple_pattern_index_test_search(
    SIMPLE_PATTERN_INDEX *index, SIMPLE_PATTERN *pattern, void *expected, bool expected_present)
{
    SIMPLE_PATTERN_INDEX_MATCHES *matches = simple_pattern_index_search(index, pattern);
    if(!matches)
        return SIZE_MAX;

    size_t count = simple_pattern_index_matches_count(matches);
    if(expected && simple_pattern_index_matches_contains(matches, expected) != expected_present)
        count = SIZE_MAX;

    simple_pattern_index_matches_free(matches);
    return count;
}

struct simple_pattern_index_concurrency_test {
    SIMPLE_PATTERN_INDEX *index;
    SIMPLE_PATTERN *pattern;
    STRING *changing_key;
    STRING *alternate_key;
    void *stable_user;
    void *changing_user;
    size_t iterations;
    int errors;
};

static void simple_pattern_index_concurrency_reader(void *data) {
    struct simple_pattern_index_concurrency_test *test = data;

    for(size_t i = 0; i < test->iterations; i++) {
        SIMPLE_PATTERN_INDEX_MATCHES *matches = simple_pattern_index_search(test->index, test->pattern);
        size_t count = simple_pattern_index_matches_count(matches);
        if(unlikely(!matches || !simple_pattern_index_matches_contains(matches, test->stable_user) ||
                    count != 2))
            __atomic_fetch_add(&test->errors, 1, __ATOMIC_RELAXED);

        simple_pattern_index_matches_free(matches);
    }
}

static void simple_pattern_index_concurrency_writer(void *data) {
    struct simple_pattern_index_concurrency_test *test = data;

    for(size_t i = 0; i < test->iterations; i++) {
        STRING *key = (i & 1) ? test->changing_key : test->alternate_key;
        if(unlikely(!simple_pattern_index_replace_user(test->index, &key, 1, test->changing_user)))
            __atomic_fetch_add(&test->errors, 1, __ATOMIC_RELAXED);
    }
}

static int simple_pattern_index_concurrency_unittest(void) {
    int stable_user, changing_user;
    SIMPLE_PATTERN_INDEX *index = simple_pattern_index_create();
    STRING *stable_key = string_strdupz("stable");
    STRING *changing_key = string_strdupz("changeable");
    STRING *alternate_key = string_strdupz("switchable");
    SIMPLE_PATTERN *exact_pattern = string_to_simple_pattern("stable|changeable|switchable");
    SIMPLE_PATTERN *wildcard_pattern = string_to_simple_pattern("*able");

    STRING *initial_keys[] = { changing_key };
    int errors = simple_pattern_index_add(index, stable_key, &stable_user) &&
                 simple_pattern_index_replace_user(index, initial_keys, _countof(initial_keys), &changing_user) ? 0 : 1;
    struct simple_pattern_index_concurrency_test exact = {
        .index = index,
        .pattern = exact_pattern,
        .changing_key = changing_key,
        .alternate_key = alternate_key,
        .stable_user = &stable_user,
        .changing_user = &changing_user,
        .iterations = 2000,
    };
    struct simple_pattern_index_concurrency_test wildcard = exact;
    wildcard.pattern = wildcard_pattern;

    ND_THREAD *exact_reader = nd_thread_create(
        "SP-INDEX-EXACT", NETDATA_THREAD_OPTION_DONT_LOG, simple_pattern_index_concurrency_reader, &exact);
    ND_THREAD *wildcard_reader = nd_thread_create(
        "SP-INDEX-SCAN", NETDATA_THREAD_OPTION_DONT_LOG, simple_pattern_index_concurrency_reader, &wildcard);
    ND_THREAD *writer = nd_thread_create(
        "SP-INDEX-WRITER", NETDATA_THREAD_OPTION_DONT_LOG, simple_pattern_index_concurrency_writer, &exact);

    if(unlikely(!exact_reader || !wildcard_reader || !writer))
        fatal("SIMPLE_PATTERN_INDEX: cannot create concurrency test threads");

    nd_thread_join(exact_reader);
    nd_thread_join(wildcard_reader);
    nd_thread_join(writer);
    errors += exact.errors + wildcard.errors;

    simple_pattern_free(wildcard_pattern);
    simple_pattern_free(exact_pattern);
    string_freez(changing_key);
    string_freez(alternate_key);
    string_freez(stable_key);
    simple_pattern_index_destroy(index);

    return errors;
}

int simple_pattern_index_unittest(void) {
    int errors = 0;
    SIMPLE_PATTERN_INDEX *index = simple_pattern_index_create();
    int user1, user2, user3;

    if(!simple_pattern_index_test_add(index, "host1", &user1) ||
       !simple_pattern_index_test_add(index, "host1", &user1) ||
       !simple_pattern_index_test_add(index, "uuid1", &user1) ||
       !simple_pattern_index_test_add(index, "host2", &user2) ||
       !simple_pattern_index_test_add(index, "uuid1", &user2) ||
       !simple_pattern_index_test_add(index, "Host3", &user3) ||
       !simple_pattern_index_test_add(index, "uuid3", &user3) ||
       !simple_pattern_index_test_add(index, "host|pipe", &user3))
        errors++;

    if(simple_pattern_index_test_search(index, NULL, &user1, true) != 3)
        errors++;

    STRING *host1 = string_strdupz("host1");
    STRING *host1_acquired = string_acquire_existing("host1", 5);
    if(host1_acquired != host1)
        errors++;
    string_freez(host1_acquired);
    string_freez(host1);

    SIMPLE_PATTERN *pattern = string_to_simple_pattern("uuid1");
    if(simple_pattern_index_test_search(index, pattern, &user2, true) != 2)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern("host*");
    if(simple_pattern_index_test_search(index, pattern, &user1, true) != 3)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern("!host1|uuid1");
    if(simple_pattern_index_test_search(index, pattern, &user1, false) != 1 ||
       simple_pattern_index_test_search(index, pattern, &user2, true) != 1)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern("uuid1|!host1");
    if(simple_pattern_index_test_search(index, pattern, &user1, false) != 1)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern("host1|!host1");
    if(simple_pattern_index_test_search(index, pattern, &user1, true) != 1)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern("!host1|host1");
    if(simple_pattern_index_test_search(index, pattern, NULL, false) != 0)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern("!host*|uuid1");
    if(simple_pattern_index_test_search(index, pattern, NULL, false) != 0)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern("!host1");
    if(simple_pattern_index_test_search(index, pattern, NULL, false) != 0)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern("HOST3");
    if(simple_pattern_index_test_search(index, pattern, NULL, false) != 0)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern_nocase("HOST3");
    if(simple_pattern_index_test_search(index, pattern, &user3, true) != 1)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern("*1");
    if(simple_pattern_index_test_search(index, pattern, &user2, true) != 2)
        errors++;
    simple_pattern_free(pattern);

    pattern = string_to_simple_pattern("host\\|pipe");
    if(simple_pattern_index_test_search(index, pattern, &user3, true) != 1)
        errors++;
    simple_pattern_free(pattern);

    static const char absent_literal[] = "simple-pattern-index-definitely-absent";
    const char *absent_literal_ptr = absent_literal;
    STRING *absent = string_acquire_existing(absent_literal, sizeof(absent_literal) - 1);
    if(absent) {
        errors++;
        string_freez(absent);
    }
    pattern = string_to_simple_pattern(absent_literal_ptr);
    if(simple_pattern_index_test_search(index, pattern, NULL, false) != 0)
        errors++;
    simple_pattern_free(pattern);
    absent = string_acquire_existing(absent_literal, sizeof(absent_literal) - 1);
    if(absent) {
        errors++;
        string_freez(absent);
    }

    STRING *uuid1 = string_strdupz("uuid1");
    if(!simple_pattern_index_del(index, uuid1, &user2))
        errors++;
    pattern = string_to_simple_pattern("uuid1");
    if(simple_pattern_index_test_search(index, pattern, &user2, false) != 1)
        errors++;
    simple_pattern_free(pattern);
    string_freez(uuid1);

    if(simple_pattern_index_del_user(index, &user1) != 2)
        errors++;
    pattern = string_to_simple_pattern("host1|uuid1");
    if(simple_pattern_index_test_search(index, pattern, NULL, false) != 0)
        errors++;
    simple_pattern_free(pattern);

    host1_acquired = string_acquire_existing("host1", 5);
    uuid1 = string_acquire_existing("uuid1", 5);
    if(host1_acquired || uuid1)
        errors++;
    string_freez(host1_acquired);
    string_freez(uuid1);

    simple_pattern_index_destroy(index);
    simple_pattern_index_destroy(NULL);

    errors += simple_pattern_index_concurrency_unittest();

    if(errors)
        fprintf(stderr, "SIMPLE_PATTERN_INDEX: %d test(s) failed\n", errors);

    return errors;
}
