// SPDX-License-Identifier: GPL-3.0-or-later

#include <errno.h>
#include <pthread.h>
#include <sched.h>
#include <sys/stat.h>
#include <unistd.h>

#include "../http_auth.h"

RRDHOST *localhost = NULL;

enum bearer_test_io_mode {
    BEARER_TEST_IO_SUCCESS,
    BEARER_TEST_IO_SHORT_WRITE,
    BEARER_TEST_IO_FULL_WRITE_ERROR,
    BEARER_TEST_IO_CLOSE_ERROR,
    BEARER_TEST_IO_WRITE_AND_CLOSE_ERROR,
};

struct bearer_test_io_state {
    enum bearer_test_io_mode mode;
    FILE *stream;
    bool active;
    bool closed;
    size_t opens;
    size_t writes;
    size_t error_checks;
    size_t closes;
    size_t unlinks;
    size_t logs;
    size_t post_close_accesses;
    int logged_errno;
    size_t payload_size;
    char payload[4096];
};

static struct bearer_test_io_state bearer_test_io;

static FILE *bearer_test_fopen(const char *filename, const char *mode) {
    FILE *fp = fopen(filename, mode);
    if(bearer_test_io.active && strstr(filename, "/bearer_tokens/")) {
        bearer_test_io.opens++;
        bearer_test_io.stream = fp;
    }

    return fp;
}

static size_t bearer_test_fwrite(const void *ptr, size_t size, size_t count, FILE *stream) {
    if(!bearer_test_io.active || stream != bearer_test_io.stream)
        return fwrite(ptr, size, count, stream);

    if(bearer_test_io.closed) {
        bearer_test_io.post_close_accesses++;
        errno = EBADF;
        return 0;
    }

    bearer_test_io.writes++;
    size_t bytes = size * count;
    if(bytes > sizeof(bearer_test_io.payload)) {
        errno = EOVERFLOW;
        return 0;
    }

    memcpy(bearer_test_io.payload, ptr, bytes);
    bearer_test_io.payload_size = bytes;

    if(bearer_test_io.mode == BEARER_TEST_IO_SHORT_WRITE) {
        size_t short_count = count ? count - 1 : 0;
        size_t written = fwrite(ptr, size, short_count, stream);
        errno = EDQUOT;
        return written;
    }

    size_t written = fwrite(ptr, size, count, stream);
    if(bearer_test_io.mode == BEARER_TEST_IO_FULL_WRITE_ERROR ||
       bearer_test_io.mode == BEARER_TEST_IO_WRITE_AND_CLOSE_ERROR)
        errno = EDQUOT;

    return written;
}

static int bearer_test_ferror(FILE *stream) {
    if(!bearer_test_io.active || stream != bearer_test_io.stream)
        return ferror(stream);

    if(bearer_test_io.closed) {
        bearer_test_io.post_close_accesses++;
        return 1;
    }

    bearer_test_io.error_checks++;
    return ferror(stream) ||
           bearer_test_io.mode == BEARER_TEST_IO_SHORT_WRITE ||
           bearer_test_io.mode == BEARER_TEST_IO_FULL_WRITE_ERROR ||
           bearer_test_io.mode == BEARER_TEST_IO_WRITE_AND_CLOSE_ERROR;
}

static int bearer_test_fclose(FILE *stream) {
    if(!bearer_test_io.active || stream != bearer_test_io.stream)
        return fclose(stream);

    if(bearer_test_io.closed) {
        bearer_test_io.post_close_accesses++;
        errno = EBADF;
        return EOF;
    }

    bearer_test_io.closes++;
    int rc = fclose(stream);
    bearer_test_io.closed = true;
    if(bearer_test_io.mode == BEARER_TEST_IO_CLOSE_ERROR ||
       bearer_test_io.mode == BEARER_TEST_IO_WRITE_AND_CLOSE_ERROR) {
        errno = ENOSPC;
        return EOF;
    }

    return rc;
}

static int bearer_test_unlink(const char *filename) {
    if(bearer_test_io.active && strstr(filename, "/bearer_tokens/")) {
        bearer_test_io.unlinks++;
        if(bearer_test_io.stream && !bearer_test_io.closed)
            bearer_test_io.post_close_accesses++;
    }

    int rc = unlink(filename);
    if(bearer_test_io.active)
        errno = EBUSY;
    return rc;
}

static void bearer_test_netdata_logger(
    ND_LOG_SOURCES source __maybe_unused, ND_LOG_FIELD_PRIORITY priority __maybe_unused,
    const char *file __maybe_unused, const char *function __maybe_unused,
    unsigned long line __maybe_unused, const char *fmt __maybe_unused, ...) {
    if(bearer_test_io.active) {
        bearer_test_io.logs++;
        bearer_test_io.logged_errno = errno;
    }
}

#define fopen bearer_test_fopen
#define fwrite bearer_test_fwrite
#define ferror bearer_test_ferror
#define fclose bearer_test_fclose
#define unlink bearer_test_unlink
#define netdata_logger bearer_test_netdata_logger
// Include the implementation to exercise its private helpers directly.
#include "../http_auth.c"
#undef netdata_logger
#undef unlink
#undef fclose
#undef ferror
#undef fwrite
#undef fopen

void web_client_set_permissions(
    struct web_client *w, HTTP_ACCESS access, HTTP_USER_ROLE role, USER_AUTH_METHOD type) {
    w->user_auth.access = access;
    w->user_auth.user_role = role;
    w->user_auth.method = type;
}

bool mcp_api_key_verify(const char *key __maybe_unused, bool silent __maybe_unused) {
    return false;
}

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

static int bearer_test_expect(bool condition, const char *message) {
    if(condition)
        return 0;

    fprintf(stderr, "bearer persistence test failed: %s\n", message);
    return 1;
}

static void bearer_test_io_reset(enum bearer_test_io_mode mode) {
    memset(&bearer_test_io, 0, sizeof(bearer_test_io));
    bearer_test_io.mode = mode;
    bearer_test_io.active = true;
}

static int bearer_test_file_matches_payload(const char *filename) {
    char contents[sizeof(bearer_test_io.payload)];
    FILE *fp = fopen(filename, "rb");
    if(!fp)
        return 1;

    size_t bytes = fread(contents, 1, sizeof(contents), fp);
    bool failed = ferror(fp);
    if(fclose(fp) != 0)
        failed = true;

    return failed || bytes != bearer_test_io.payload_size ||
           memcmp(contents, bearer_test_io.payload, bytes) != 0;
}

static int bearer_test_save_case(
    const char *name, enum bearer_test_io_mode mode, nd_uuid_t token,
    const struct bearer_token *bt, bool expected_success, int expected_errno) {
    char filename[FILENAME_MAX];
    bearer_token_filename(filename, token);
    (void)unlink(filename);

    bearer_test_io_reset(mode);
    errno = 0;
    bool result = bearer_token_save_to_file(token, (struct bearer_token *)bt);
    bearer_test_io.active = false;

    int errors = 0;
    errors += bearer_test_expect(result == expected_success, name);
    errors += bearer_test_expect(bearer_test_io.opens == 1, "save must open one stream");
    errors += bearer_test_expect(bearer_test_io.writes == 1, "save must issue one fwrite");
    errors += bearer_test_expect(bearer_test_io.error_checks == 1, "save must inspect stream state before close");
    errors += bearer_test_expect(bearer_test_io.closes == 1, "save must close exactly once");
    errors += bearer_test_expect(bearer_test_io.post_close_accesses == 0, "save must not access FILE after close");
    errors += bearer_test_expect(
        bearer_test_io.unlinks == (expected_success ? 0U : 1U),
        "save must unlink only failed direct-final output");
    errors += bearer_test_expect(
        bearer_test_io.logs == (expected_success ? 0U : 1U),
        "save must log only failed persistence");

    if(expected_success) {
        struct stat st;
        errors += bearer_test_expect(stat(filename, &st) == 0, "successful save must retain the final file");
        errors += bearer_test_expect(S_ISREG(st.st_mode), "successful save must produce a regular file");
        errors += bearer_test_expect((st.st_mode & 0777) == 0600, "save must retain fopen plus umask permissions");
        errors += bearer_test_expect(!bearer_test_file_matches_payload(filename), "successful file bytes must match fwrite input");
    }
    else {
        errors += bearer_test_expect(bearer_test_io.logged_errno == expected_errno,
                                     "save must log the first persistence errno");
        errors += bearer_test_expect(access(filename, F_OK) != 0 && errno == ENOENT,
                                     "failed direct-final save must remove the file");
    }

    return errors;
}

static int bearer_test_successful_reload(nd_uuid_t token, const struct bearer_token *expected) {
    int errors = 0;
    netdata_authorized_bearers = bearer_tokens_dictionary_create();
    errors += bearer_test_expect(bearer_token_load_token(token), "successful file must reload");

    char token_string[UUID_STR_LEN];
    uuid_unparse_lower(token, token_string);
    struct web_client w = { 0 };
    errors += bearer_test_expect(web_client_bearer_token_auth(&w, token_string),
                                 "reloaded successful token must authenticate");
    errors += bearer_test_expect(w.user_auth.access == expected->access,
                                 "reloaded token must retain access");
    errors += bearer_test_expect(w.user_auth.user_role == expected->user_role,
                                 "reloaded token must retain role");
    errors += bearer_test_expect(w.user_auth.method == USER_AUTH_METHOD_BEARER,
                                 "reloaded token must retain bearer auth method");

    dictionary_destroy(netdata_authorized_bearers);
    netdata_authorized_bearers = NULL;
    return errors;
}

static int bearer_test_failed_save_caller(nd_uuid_t token, struct bearer_token *expected) {
    int errors = 0;
    netdata_authorized_bearers = bearer_tokens_dictionary_create();

    bearer_test_io_reset(BEARER_TEST_IO_FULL_WRITE_ERROR);
    errno = 0;
    time_t expiration = bearer_create_token_internal(
        token, expected->user_role, expected->access, expected->cloud_account_id,
        expected->client_name, expected->created_s, expected->expires_s, true);
    bearer_test_io.active = false;

    errors += bearer_test_expect(expiration == expected->expires_s,
                                 "caller must preserve expiration after ignored save failure");
    errors += bearer_test_expect(bearer_test_io.logged_errno == EDQUOT,
                                 "caller must log the persistence errno");
    errors += bearer_test_expect(bearer_test_io.opens == 1 && bearer_test_io.writes == 1 &&
                                     bearer_test_io.error_checks == 1 && bearer_test_io.closes == 1,
                                 "caller must execute one complete failed save attempt");

    char token_string[UUID_STR_LEN];
    uuid_unparse_lower(token, token_string);
    struct web_client w = { 0 };
    errors += bearer_test_expect(web_client_bearer_token_auth(&w, token_string),
                                 "failed persistence must leave the token in memory");

    bearer_test_io_reset(BEARER_TEST_IO_SUCCESS);
    time_t second_expiration = bearer_create_token_internal(
        token, HTTP_USER_ROLE_MEMBER, HTTP_ACCESS_NONE, expected->cloud_account_id,
        "different-client", expected->created_s + 1, expected->expires_s + 1, true);
    bearer_test_io.active = false;
    errors += bearer_test_expect(second_expiration == expected->expires_s,
                                 "duplicate caller must retain the first in-memory token");
    errors += bearer_test_expect(bearer_test_io.opens == 0 && bearer_test_io.writes == 0,
                                 "duplicate caller must not retry ignored persistence failure");

    dictionary_destroy(netdata_authorized_bearers);
    netdata_authorized_bearers = bearer_tokens_dictionary_create();
    memset(&w, 0, sizeof(w));
    errors += bearer_test_expect(!web_client_bearer_token_auth(&w, token_string),
                                 "failed persistence token must disappear after restart");
    dictionary_destroy(netdata_authorized_bearers);
    netdata_authorized_bearers = NULL;
    return errors;
}

static int bearer_test_persistence(void) {
    static RRDHOST test_host;
    const char *original_varlib_dir = netdata_configured_varlib_dir;
    memset(&test_host, 0, sizeof(test_host));
    memset(test_host.host_id.uuid, 0x11, sizeof(test_host.host_id.uuid));
    localhost = &test_host;

    char root_template[] = "/tmp/netdata-http-auth-test-XXXXXX";
    char *root = mkdtemp(root_template);
    if(!root) {
        fprintf(stderr, "bearer persistence test failed: cannot create temporary directory\n");
        return 1;
    }

    netdata_configured_varlib_dir = root;
    int errors = bearer_test_expect(bearer_tokens_ensure_path_exists(),
                                    "cannot create bearer token directory");

    nd_uuid_t token;
    memset(token, 0x22, sizeof(token));
    struct bearer_token bt = {
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE,
        .user_role = HTTP_USER_ROLE_ADMIN,
        .created_s = now_realtime_sec(),
        .expires_s = now_realtime_sec() + 86400,
    };
    memset(bt.cloud_account_id, 0x33, sizeof(bt.cloud_account_id));
    strncpyz(bt.client_name, "persistence-test-client", sizeof(bt.client_name) - 1);

    errors += bearer_test_save_case(
        "successful save must return true", BEARER_TEST_IO_SUCCESS,
        token, &bt, true, 0);
    errors += bearer_test_successful_reload(token, &bt);
    errors += bearer_test_save_case(
        "short fwrite must fail", BEARER_TEST_IO_SHORT_WRITE,
        token, &bt, false, EDQUOT);
    errors += bearer_test_save_case(
        "full fwrite plus ferror must fail", BEARER_TEST_IO_FULL_WRITE_ERROR,
        token, &bt, false, EDQUOT);
    errors += bearer_test_save_case(
        "close-only error must fail", BEARER_TEST_IO_CLOSE_ERROR,
        token, &bt, false, ENOSPC);
    errors += bearer_test_save_case(
        "combined write and close errors must fail", BEARER_TEST_IO_WRITE_AND_CLOSE_ERROR,
        token, &bt, false, EDQUOT);

    nd_uuid_t failed_token;
    memset(failed_token, 0x44, sizeof(failed_token));
    errors += bearer_test_failed_save_caller(failed_token, &bt);

    char token_path[FILENAME_MAX];
    bearer_token_filename(token_path, token);
    (void)unlink(token_path);
    bearer_token_filename(token_path, failed_token);
    (void)unlink(token_path);
    char token_dir[FILENAME_MAX];
    bearer_tokens_path(token_dir);
    (void)rmdir(token_dir);
    (void)rmdir(root);
    netdata_configured_varlib_dir = original_varlib_dir;
    localhost = NULL;
    return errors;
}

static int bearer_test_publication(void) {
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

    return errors;
}

int main(void) {
    mode_t old_umask = umask(0077); // Flawfinder: ignore - restrictive test-only creation mask.
    int persistence_errors = bearer_test_persistence();
    int publication_errors = bearer_test_publication();
    umask(old_umask); // Flawfinder: ignore - restore the caller's mask.

    int errors = persistence_errors + publication_errors;
    if(errors) {
        fprintf(stderr, "bearer tests failed with %d error(s)\n", errors);
        return 1;
    }

    fprintf(stderr, "bearer persistence and publication tests passed\n");
    return 0;
}
