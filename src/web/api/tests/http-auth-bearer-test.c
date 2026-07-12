// SPDX-License-Identifier: GPL-3.0-or-later

#include <pthread.h>
#include <sched.h>

// Include the implementation to exercise its private publication helper directly.
#include "../http_auth.c"

#define BEARER_TEST_CREATORS 16
#define BEARER_TEST_READERS 16

struct bearer_test_shared {
    bool start;
    size_t creators;
    size_t creators_done;
    nd_uuid_t token;
};

struct bearer_test_creator {
    struct bearer_test_shared *shared;
    struct bearer_token expected;
    struct bearer_token observed;
    bool inserted;
    bool failed;
};

struct bearer_test_reader {
    struct bearer_test_shared *shared;
    struct bearer_token observed;
    bool failed;
};

static void bearer_test_wait_for_start(struct bearer_test_shared *shared) {
    while(!__atomic_load_n(&shared->start, __ATOMIC_ACQUIRE))
        sched_yield();
}

static bool bearer_test_tokens_equal(const struct bearer_token *a, const struct bearer_token *b) {
    return uuid_eq(a->cloud_account_id, b->cloud_account_id) &&
           strcmp(a->client_name, b->client_name) == 0 &&
           a->access == b->access &&
           a->user_role == b->user_role &&
           a->created_s == b->created_s &&
           a->expires_s == b->expires_s;
}

static void *bearer_test_creator_main(void *arg) {
    struct bearer_test_creator *ctx = arg;
    bearer_test_wait_for_start(ctx->shared);

    const DICTIONARY_ITEM *item = bearer_token_set_and_acquire(
        ctx->shared->token,
        ctx->expected.user_role,
        ctx->expected.access,
        ctx->expected.cloud_account_id,
        ctx->expected.client_name,
        ctx->expected.created_s,
        ctx->expected.expires_s,
        &ctx->inserted);
    if(!item)
        ctx->failed = true;
    else {
        ctx->observed = *(struct bearer_token *)dictionary_acquired_item_value(item);
        dictionary_acquired_item_release(netdata_authorized_bearers, item);
    }

    __atomic_add_fetch(&ctx->shared->creators_done, 1, __ATOMIC_RELEASE);
    return NULL;
}

static void *bearer_test_reader_main(void *arg) {
    struct bearer_test_reader *ctx = arg;
    bearer_test_wait_for_start(ctx->shared);

    char uuid_str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(ctx->shared->token, uuid_str);

    const DICTIONARY_ITEM *item;
    while(!(item = dictionary_get_and_acquire_item(netdata_authorized_bearers, uuid_str))) {
        if(__atomic_load_n(&ctx->shared->creators_done, __ATOMIC_ACQUIRE) == ctx->shared->creators) {
            ctx->failed = true;
            return NULL;
        }
        sched_yield();
    }

    struct bearer_token *token = dictionary_acquired_item_value(item);
    if(!token)
        ctx->failed = true;
    else
        ctx->observed = *token;

    dictionary_acquired_item_release(netdata_authorized_bearers, item);
    return NULL;
}

static bool bearer_test_token_matches(
    const struct bearer_token *token,
    const struct bearer_test_creator creators[BEARER_TEST_CREATORS],
    size_t creators_count) {
    for(size_t i = 0; i < creators_count; i++) {
        if(bearer_test_tokens_equal(token, &creators[i].expected))
            return true;
    }

    return false;
}

int main(void) {
    struct bearer_test_shared shared = { 0 };
    memset(shared.token, 0xa5, sizeof(shared.token));

    netdata_authorized_bearers = bearer_tokens_dictionary_create();

    struct bearer_test_creator creators[BEARER_TEST_CREATORS] = { 0 };
    struct bearer_test_reader readers[BEARER_TEST_READERS] = { 0 };
    pthread_t creator_threads[BEARER_TEST_CREATORS];
    pthread_t reader_threads[BEARER_TEST_READERS];
    size_t creators_started = 0, readers_started = 0;
    int errors = 0;

    for(size_t i = 0; i < BEARER_TEST_CREATORS; i++) {
        creators[i].shared = &shared;
        creators[i].expected.user_role = HTTP_USER_ROLE_ADMIN;
        creators[i].expected.access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE;
        creators[i].expected.created_s = (time_t)(1000 + i);
        creators[i].expected.expires_s = (time_t)(100000 + i);
        memset(creators[i].expected.cloud_account_id, (int)i + 1,
               sizeof(creators[i].expected.cloud_account_id));
        snprintfz(creators[i].expected.client_name,
                  sizeof(creators[i].expected.client_name),
                  "synthetic-client-%zu-abcdefghijklmnopqrstuvwxyz", i);

        if(pthread_create(&creator_threads[i], NULL, bearer_test_creator_main, &creators[i]) != 0) {
            fprintf(stderr, "failed to create bearer test creator %zu\n", i);
            errors++;
            break;
        }
        creators_started++;
    }

    shared.creators = creators_started;
    if(creators_started) {
        for(size_t i = 0; i < BEARER_TEST_READERS; i++) {
            readers[i].shared = &shared;
            if(pthread_create(&reader_threads[i], NULL, bearer_test_reader_main, &readers[i]) != 0) {
                fprintf(stderr, "failed to create bearer test reader %zu\n", i);
                errors++;
                break;
            }
            readers_started++;
        }
    }

    __atomic_store_n(&shared.start, true, __ATOMIC_RELEASE);

    for(size_t i = 0; i < creators_started; i++)
        pthread_join(creator_threads[i], NULL);
    for(size_t i = 0; i < readers_started; i++)
        pthread_join(reader_threads[i], NULL);

    char uuid_str[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(shared.token, uuid_str);
    const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(netdata_authorized_bearers, uuid_str);
    if(!item) {
        fprintf(stderr, "bearer test token was not published\n");
        errors++;
    }
    else {
        struct bearer_token winner = *(struct bearer_token *)dictionary_acquired_item_value(item);
        dictionary_acquired_item_release(netdata_authorized_bearers, item);

        if(!bearer_test_token_matches(&winner, creators, creators_started)) {
            fprintf(stderr, "published bearer is not one complete constructor payload\n");
            errors++;
        }

        size_t inserted = 0;
        for(size_t i = 0; i < creators_started; i++) {
            inserted += creators[i].inserted ? 1 : 0;
            if(creators[i].failed || !bearer_test_tokens_equal(&creators[i].observed, &winner)) {
                fprintf(stderr, "creator %zu did not acquire the complete winning bearer\n", i);
                errors++;
            }
        }

        if(inserted != 1) {
            fprintf(stderr, "expected one bearer insertion, got %zu\n", inserted);
            errors++;
        }

        for(size_t i = 0; i < readers_started; i++) {
            if(readers[i].failed || !bearer_test_tokens_equal(&readers[i].observed, &winner)) {
                fprintf(stderr, "reader %zu observed a partial or different bearer\n", i);
                errors++;
            }
        }
    }

    dictionary_destroy(netdata_authorized_bearers);
    netdata_authorized_bearers = NULL;

    if(errors) {
        fprintf(stderr, "bearer publication test failed with %d error(s)\n", errors);
        return 1;
    }

    fprintf(stderr, "bearer publication test passed\n");
    return 0;
}
