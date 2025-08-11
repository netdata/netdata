// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg-internals.h"
#include "dyncfg.h"

// ----------------------------------------------------------------------------
// unit test

#define LINE_FILE_STR TOSTRING(__LINE__) "@" __FILE__

struct dyncfg_unittest {
    bool enabled;
    size_t errors;

    DICTIONARY *nodes;

    SPINLOCK spinlock;
    struct dyncfg_unittest_action *queue;
} dyncfg_unittest_data = { 0 };

typedef struct {
    bool enabled;
    bool removed;
    struct {
        double dbl;
        bool bln;
    } value;
} TEST_CFG;

typedef struct {
    const char *id;
    const char *source;
    bool sync;
    DYNCFG_TYPE type;
    DYNCFG_CMDS cmds;
    DYNCFG_SOURCE_TYPE source_type;

    TEST_CFG current;
    TEST_CFG expected;

    bool received;
    bool finished;

    size_t last_saves;
    bool needs_save;
} TEST;

struct dyncfg_unittest_action {
    TEST *t;
    BUFFER *result;
    BUFFER *payload;
    DYNCFG_CMDS cmd;
    const char *add_name;
    const char *source;

    rrd_function_result_callback_t result_cb;
    void *result_cb_data;

    struct dyncfg_unittest_action *prev, *next;
};

static void dyncfg_unittest_register_error(const char *id, const char *msg) {
    if(msg)
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: error on id '%s': %s", id ? id : "", msg);

    __atomic_add_fetch(&dyncfg_unittest_data.errors, 1, __ATOMIC_RELAXED);
}

static int dyncfg_unittest_execute_cb(struct rrd_function_execute *rfe, void *data);

bool dyncfg_unittest_parse_payload(BUFFER *payload, TEST *t, DYNCFG_CMDS cmd, const char *add_name, const char *source) {
    CLEAN_JSON_OBJECT *jobj = json_tokener_parse(buffer_tostring(payload));
    if(!jobj) {
        dyncfg_unittest_register_error(t->id, "cannot parse json payload");
        return false;
    }

    struct json_object *json_double;
    struct json_object *json_boolean;

    json_object_object_get_ex(jobj, "double", &json_double);
    double value_double = json_object_get_double(json_double);

    json_object_object_get_ex(jobj, "boolean", &json_boolean);
    int value_boolean = json_object_get_boolean(json_boolean);

    if(cmd == DYNCFG_CMD_UPDATE) {
        t->current.value.dbl = value_double;
        t->current.value.bln = value_boolean;
    }
    else if(cmd == DYNCFG_CMD_ADD) {
        char buf[strlen(t->id) + strlen(add_name) + 20];
        snprintfz(buf, sizeof(buf), "%s:%s", t->id, add_name);
        TEST tmp = {
            .id = strdupz(buf),
            .source = strdupz(source),
            .cmds = (t->cmds & ~DYNCFG_CMD_ADD) | DYNCFG_CMD_GET | DYNCFG_CMD_REMOVE | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE | DYNCFG_CMD_TEST,
            .sync = t->sync,
            .type = DYNCFG_TYPE_JOB,
            .source_type = DYNCFG_SOURCE_TYPE_DYNCFG,
            .received = true,
            .finished = true,
            .current =
                {.enabled = true,
                 .removed = false,
                 .value =
                     {
                         .dbl = value_double,
                         .bln = value_boolean,
                     }},
            .expected = {
                .enabled = true,
                .removed = false,
                .value = {
                    .dbl = 3.14,
                    .bln = true,
                }
            },
            .needs_save = true,
        };
        const DICTIONARY_ITEM *item = dictionary_set_and_acquire_item(dyncfg_unittest_data.nodes, buf, &tmp, sizeof(tmp));
        TEST *t2 = dictionary_acquired_item_value(item);
        dictionary_acquired_item_release(dyncfg_unittest_data.nodes, item);

        dyncfg_add_low_level(localhost, t2->id, "/unittests",
                             DYNCFG_STATUS_RUNNING, t2->type, t2->source_type, t2->source,
                             t2->cmds, 0, 0, t2->sync,
                             HTTP_ACCESS_NONE, HTTP_ACCESS_NONE,
                             dyncfg_unittest_execute_cb, t2);
    }
    else {
        dyncfg_unittest_register_error(t->id, "invalid command received to parse payload");
        return false;
    }

    return true;
}

static int dyncfg_unittest_action(struct dyncfg_unittest_action *a) {
    TEST *t = a->t;

    int rc = HTTP_RESP_OK;

    if(a->cmd == DYNCFG_CMD_ENABLE)
        t->current.enabled = true;
    else if(a->cmd == DYNCFG_CMD_DISABLE)
        t->current.enabled = false;
    else if(a->cmd == DYNCFG_CMD_ADD || a->cmd == DYNCFG_CMD_UPDATE)
        rc = dyncfg_unittest_parse_payload(a->payload, a->t, a->cmd, a->add_name, a->source) ? HTTP_RESP_OK : HTTP_RESP_BAD_REQUEST;
    else if(a->cmd == DYNCFG_CMD_REMOVE)
        t->current.removed = true;
    else
        rc = HTTP_RESP_BAD_REQUEST;

    dyncfg_default_response(a->result, rc, NULL);

    a->result_cb(a->result, rc, a->result_cb_data);

    buffer_free(a->payload);
    freez((void *)a->add_name);
    freez(a);

    __atomic_store_n(&t->finished, true, __ATOMIC_RELAXED);

    return rc;
}

static void dyncfg_unittest_thread_action(void *ptr __maybe_unused) {
    while(!nd_thread_signaled_to_cancel()) {
        struct dyncfg_unittest_action *a = NULL;
        spinlock_lock(&dyncfg_unittest_data.spinlock);
        a = dyncfg_unittest_data.queue;
        if(a)
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(dyncfg_unittest_data.queue, a, prev, next);
        spinlock_unlock(&dyncfg_unittest_data.spinlock);

        if(a)
            dyncfg_unittest_action(a);
        else
            sleep_usec(10 * USEC_PER_MS);
    }
}

static int dyncfg_unittest_execute_cb(struct rrd_function_execute *rfe, void *data) {

    int rc;
    bool run_the_callback = true;
    TEST *t = data;

    t->received = true;

    char buf[strlen(rfe->function) + 1];
    memcpy(buf, rfe->function, sizeof(buf));

    char *words[MAX_FUNCTION_PARAMETERS];    // an array of pointers for the words in this line
    size_t num_words = quoted_strings_splitter_whitespace(buf, words, MAX_FUNCTION_PARAMETERS);

    const char *config = get_word(words, num_words, 0);
    const char *id = get_word(words, num_words, 1);
    const char *action = get_word(words, num_words, 2);
    const char *add_name = get_word(words, num_words, 3);

    if(!config || !*config || strcmp(config, PLUGINSD_FUNCTION_CONFIG) != 0) {
        char *msg = "did not receive a config call";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(rfe->result.wb, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    if(!id || !*id) {
        char *msg = "did not receive an id";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(rfe->result.wb, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    if(t->type != DYNCFG_TYPE_TEMPLATE && strcmp(t->id, id) != 0) {
        char *msg = "id received is not the expected";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(rfe->result.wb, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    if(!action || !*action) {
        char *msg = "did not receive an action";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(rfe->result.wb, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    DYNCFG_CMDS cmd = dyncfg_cmds2id(action);
    if(cmd == DYNCFG_CMD_NONE) {
        char *msg = "action received is not known";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(rfe->result.wb, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    if(!(t->cmds & cmd)) {
        char *msg = "received a command that is not supported";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(rfe->result.wb, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    if(t->current.removed && cmd != DYNCFG_CMD_ADD) {
        char *msg = "received a command for a removed entry";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(rfe->result.wb, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    struct dyncfg_unittest_action *a = callocz(1, sizeof(*a));
    a->t = t;
    a->add_name = add_name ? strdupz(add_name) : NULL;
    a->source = rfe->source,
    a->result = rfe->result.wb;
    a->payload = buffer_dup(rfe->payload);
    a->cmd = cmd;
    a->result_cb = rfe->result.cb;
    a->result_cb_data = rfe->result.data;

    run_the_callback = false;

    if(t->sync)
        rc = dyncfg_unittest_action(a);
    else {
        spinlock_lock(&dyncfg_unittest_data.spinlock);
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(dyncfg_unittest_data.queue, a, prev, next);
        spinlock_unlock(&dyncfg_unittest_data.spinlock);
        rc = HTTP_RESP_OK;
    }

cleanup:
    if(run_the_callback) {
        __atomic_store_n(&t->finished, true, __ATOMIC_RELAXED);

        if (rfe->result.cb)
            rfe->result.cb(rfe->result.wb, rc, rfe->result.data);
    }

    return rc;
}

static bool dyncfg_unittest_check(TEST *t, DYNCFG_CMDS c, const char *cmd, bool received) {
    size_t errors = 0;

    fprintf(stderr, "CHECK '%s' after cmd '%s'...", t->id, cmd);

    if(t->received != received) {
        fprintf(stderr, "\n  - received flag found '%s', expected '%s'",
                t->received?"true":"false",
                received?"true":"false");
        errors++;
        goto cleanup;
    }

    if(!received)
        goto cleanup;

    usec_t give_up_ut = now_monotonic_usec() + 2 * USEC_PER_SEC;
    while(!__atomic_load_n(&t->finished, __ATOMIC_RELAXED)) {
        tinysleep();

        if(now_monotonic_usec() > give_up_ut) {
            fprintf(stderr, "\n  - gave up waiting for the plugin to process this!");
            errors++;
            goto cleanup;
        }
    }

    if(t->type != DYNCFG_TYPE_TEMPLATE && t->current.enabled != t->expected.enabled) {
        fprintf(stderr, "\n  - enabled flag found '%s', expected '%s'",
                t->current.enabled?"true":"false",
                t->expected.enabled?"true":"false");
        errors++;
    }
    if(t->current.removed != t->expected.removed) {
        fprintf(stderr, "\n  - removed flag found '%s', expected '%s'",
                t->current.removed?"true":"false",
                t->expected.removed?"true":"false");
        errors++;
    }
    if(t->current.value.bln != t->expected.value.bln) {
        fprintf(stderr, "\n  - boolean value found '%s', expected '%s'",
                t->current.value.bln?"true":"false",
                t->expected.value.bln?"true":"false");
        errors++;
    }
    if(t->current.value.dbl != t->expected.value.dbl) {
        fprintf(stderr, "\n  - double value found '%f', expected '%f'",
                t->current.value.dbl, t->expected.value.dbl);
        errors++;
    }

    DYNCFG *df = dictionary_get(dyncfg_globals.nodes, t->id);
    if(!df) {
        fprintf(stderr, "\n  - not found in DYNCFG nodes dictionary!");
        errors++;
    }
    else if(df->cmds != t->cmds) {
        fprintf(stderr, "\n  - has different cmds in DYNCFG nodes dictionary; found: ");
        dyncfg_cmds2fp(df->cmds, stderr);
        fprintf(stderr, ", expected: ");
        dyncfg_cmds2fp(t->cmds, stderr);
        fprintf(stderr, "\n");
        errors++;
    }
    else if(df->type == DYNCFG_TYPE_JOB && df->current.source_type == DYNCFG_SOURCE_TYPE_DYNCFG && !df->dyncfg.saves) {
        fprintf(stderr, "\n  - DYNCFG job has no saves!");
        errors++;
    }
    else if(df->type == DYNCFG_TYPE_JOB && df->current.source_type == DYNCFG_SOURCE_TYPE_DYNCFG && (!df->dyncfg.payload || !buffer_strlen(df->dyncfg.payload))) {
        fprintf(stderr, "\n  - DYNCFG job has no payload!");
        errors++;
    }
    else if(df->dyncfg.user_disabled && !df->dyncfg.saves) {
        fprintf(stderr, "\n  - DYNCFG disabled config has no saves!");
        errors++;
    }
    else if((c & (DYNCFG_CMD_ADD | DYNCFG_CMD_UPDATE)) && t->source && string_strcmp(df->current.source, t->source) != 0) {
        fprintf(stderr, "\n  - source does not match!");
        errors++;
    }
    else if((c & (DYNCFG_CMD_ADD | DYNCFG_CMD_UPDATE)) && df->current.source && !t->source) {
        fprintf(stderr, "\n  - there is a source but it shouldn't be any!");
        errors++;
    }
    else if(t->needs_save && df->dyncfg.saves <= t->last_saves) {
        fprintf(stderr, "\n  - should be saved, but it is not saved!");
        errors++;
    }
    else if(!t->needs_save && df->dyncfg.saves > t->last_saves) {
        fprintf(stderr, "\n  - should be not be saved, but it saved!");
        errors++;
    }

cleanup:
    if(errors) {
        fprintf(stderr, "\n  >>> FAILED\n\n");
        dyncfg_unittest_register_error(NULL, NULL);
        return false;
    }

    fprintf(stderr, " OK\n");
    return true;
}

static void dyncfg_unittest_reset(void) {
    TEST *t;
    dfe_start_read(dyncfg_unittest_data.nodes, t) {
        t->received = t->finished = false;
        t->needs_save = false;

        DYNCFG *df = dictionary_get(dyncfg_globals.nodes, t->id);
        if(!df) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: cannot find id '%s'", t->id);
            dyncfg_unittest_register_error(NULL, NULL);
        }
        else
            t->last_saves = df->dyncfg.saves;
    }
    dfe_done(t);
}

void should_be_saved(TEST *t, DYNCFG_CMDS c) {
    DYNCFG *df;

    if(t->type == DYNCFG_TYPE_TEMPLATE) {
        df = dictionary_get(dyncfg_globals.nodes, t->id);
        t->current.enabled = !df->dyncfg.user_disabled;
    }

    t->needs_save =
        c == DYNCFG_CMD_UPDATE ||
        (t->current.enabled && c == DYNCFG_CMD_DISABLE) ||
        (!t->current.enabled && c == DYNCFG_CMD_ENABLE);
}

static int dyncfg_unittest_run(const char *cmd, BUFFER *wb, const char *payload, const char *source) {
    dyncfg_unittest_reset();

    char buf[strlen(cmd) + 1];
    memcpy(buf, cmd, sizeof(buf));

    char *words[MAX_FUNCTION_PARAMETERS];    // an array of pointers for the words in this line
    size_t num_words = quoted_strings_splitter_whitespace(buf, words, MAX_FUNCTION_PARAMETERS);

    // const char *config = get_word(words, num_words, 0);
    const char *id = get_word(words, num_words, 1);
    char *action = get_word(words, num_words, 2);
    const char *add_name = get_word(words, num_words, 3);

    DYNCFG_CMDS c = dyncfg_cmds2id(action);

    TEST *t = dictionary_get(dyncfg_unittest_data.nodes, id);
    if(!t) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: cannot find id '%s' from cmd: %s", id, cmd);
        dyncfg_unittest_register_error(NULL, NULL);
        return HTTP_RESP_NOT_FOUND;
    }

    if(t->type == DYNCFG_TYPE_TEMPLATE)
        t->received = t->finished = true;

    if(c == DYNCFG_CMD_DISABLE)
        t->expected.enabled = false;
    if(c == DYNCFG_CMD_ENABLE)
        t->expected.enabled = true;
    if(c == DYNCFG_CMD_UPDATE)
        memset(&t->current.value, 0, sizeof(t->current.value));

    if(c & (DYNCFG_CMD_UPDATE) || (c & (DYNCFG_CMD_DISABLE|DYNCFG_CMD_ENABLE) && t->type != DYNCFG_TYPE_TEMPLATE)) {
        freez((void *)t->source);
        t->source = strdupz(source);
    }

    buffer_flush(wb);

    CLEAN_BUFFER *pld = NULL;

    if(payload) {
        pld = buffer_create(1024, NULL);
        buffer_strcat(pld, payload);
    }

    should_be_saved(t, c);

    int rc = rrd_function_run(localhost, wb, 10, HTTP_ACCESS_ALL, cmd,
                              true, NULL,
                              NULL, NULL,
                              NULL, NULL,
                              NULL, NULL,
                              pld, source, false);
    if(!DYNCFG_RESP_SUCCESS(rc)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: failed to run: %s; returned code %d", cmd, rc);
        dyncfg_unittest_register_error(NULL, NULL);
    }

    dyncfg_unittest_check(t, c, cmd, true);

    if(rc == HTTP_RESP_OK && t->type == DYNCFG_TYPE_TEMPLATE) {
        if(c == DYNCFG_CMD_ADD) {
            char buf2[strlen(id) + strlen(add_name) + 2];
            snprintfz(buf2, sizeof(buf2), "%s:%s", id, add_name);
            TEST *tt = dictionary_get(dyncfg_unittest_data.nodes, buf2);
            if (!tt) {
                nd_log(NDLS_DAEMON, NDLP_ERR,
                       "DYNCFG UNITTEST: failed to find newly added id '%s' of command: %s",
                       id, cmd);
                dyncfg_unittest_register_error(NULL, NULL);
            }
            dyncfg_unittest_check(tt, c, cmd, true);
        }
        else {
            STRING *template = string_strdupz(t->id);
            DYNCFG *df;
            dfe_start_read(dyncfg_globals.nodes, df) {
                if(df->type == DYNCFG_TYPE_JOB && df->template == template) {
                    TEST *tt = dictionary_get(dyncfg_unittest_data.nodes, df_dfe.name);
                    if (!tt) {
                        nd_log(NDLS_DAEMON, NDLP_ERR,
                               "DYNCFG UNITTEST: failed to find id '%s' while running command: %s", df_dfe.name, cmd);
                        dyncfg_unittest_register_error(NULL, NULL);
                    }
                    else {
                        if(c == DYNCFG_CMD_DISABLE)
                            tt->expected.enabled = false;
                        if(c == DYNCFG_CMD_ENABLE)
                            tt->expected.enabled = true;
                        dyncfg_unittest_check(tt, c, cmd, true);
                    }
                }
            }
            dfe_done(df);
            string_freez(template);
        }
    }

    return rc;
}

static void dyncfg_unittest_cleanup_files(void) {
    char path[FILENAME_MAX];
    snprintfz(path, sizeof(path) - 1, "%s/%s", netdata_configured_varlib_dir, "config");

    DIR *dir = opendir(path);
    if (!dir) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: cannot open directory '%s'", path);
        return;
    }

    struct dirent *entry;
    char filename[FILENAME_MAX + sizeof(entry->d_name)];
    while ((entry = readdir(dir)) != NULL) {
        if ((entry->d_type == DT_REG || entry->d_type == DT_LNK) && strstartswith(entry->d_name, "unittest:") && strendswith(entry->d_name, ".dyncfg")) {
            snprintf(filename, sizeof(filename), "%s/%s", path, entry->d_name);
            nd_log(NDLS_DAEMON, NDLP_INFO, "DYNCFG UNITTEST: deleting file '%s'", filename);
            unlink(filename);
        }
    }

    closedir(dir);
}

static TEST *dyncfg_unittest_add(TEST t) {
    dyncfg_unittest_reset();

    TEST *ret = dictionary_set(dyncfg_unittest_data.nodes, t.id, &t, sizeof(t));

    if(!dyncfg_add_low_level(localhost, t.id, "/unittests", DYNCFG_STATUS_RUNNING, t.type,
                              t.source_type, t.source,
                              t.cmds, 0, 0, t.sync,
                              HTTP_ACCESS_NONE, HTTP_ACCESS_NONE,
                              dyncfg_unittest_execute_cb, ret)) {
        dyncfg_unittest_register_error(t.id, "addition of job failed");
    }

    dyncfg_unittest_check(ret, DYNCFG_CMD_NONE, "plugin create", t.type != DYNCFG_TYPE_TEMPLATE);

    return ret;
}

void dyncfg_unittest_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    TEST *v = value;
    freez((void *)v->id);
    freez((void *)v->source);
}

int dyncfg_unittest(void) {
    dyncfg_unittest_data.nodes = dictionary_create(DICT_OPTION_NONE);
    dictionary_register_delete_callback(dyncfg_unittest_data.nodes, dyncfg_unittest_delete_cb, NULL);

    dyncfg_unittest_cleanup_files();
    rrd_functions_inflight_init();
    dyncfg_init(false);

    // ------------------------------------------------------------------------
    // create the thread for testing async communication

    ND_THREAD *thread = nd_thread_create("unittest", NETDATA_THREAD_OPTION_DEFAULT, dyncfg_unittest_thread_action, NULL);

    // ------------------------------------------------------------------------
    // single

    TEST *single1 = dyncfg_unittest_add((TEST){
        .id = strdupz("unittest:sync:single1"),
        .source = strdupz(LINE_FILE_STR),
        .type = DYNCFG_TYPE_SINGLE,
        .cmds = DYNCFG_CMD_GET | DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
        .sync = true,
        .current = {
            .enabled = true,
        },
        .expected = {
            .enabled = true,
        }
    }); (void)single1;

    TEST *single2 = dyncfg_unittest_add((TEST){
        .id = strdupz("unittest:async:single2"),
        .source = strdupz(LINE_FILE_STR),
        .type = DYNCFG_TYPE_SINGLE,
        .cmds = DYNCFG_CMD_GET | DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
        .sync = false,
        .current = {
            .enabled = true,
        },
        .expected = {
            .enabled = true,
        }
    }); (void)single2;

    // ------------------------------------------------------------------------
    // template

    TEST *template1 = dyncfg_unittest_add((TEST){
        .id = strdupz("unittest:sync:template1"),
        .source = strdupz(LINE_FILE_STR),
        .type = DYNCFG_TYPE_TEMPLATE,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_ADD | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
        .sync = true,
    }); (void)template1;

    TEST *template2 = dyncfg_unittest_add((TEST){
        .id = strdupz("unittest:async:template2"),
        .source = strdupz(LINE_FILE_STR),
        .type = DYNCFG_TYPE_TEMPLATE,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_ADD | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
        .sync = false,
    }); (void)template2;

    // ------------------------------------------------------------------------
    // job

    TEST *user1 = dyncfg_unittest_add((TEST){
        .id = strdupz("unittest:sync:template1:user1"),
        .source = strdupz(LINE_FILE_STR),
        .type = DYNCFG_TYPE_JOB,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_USER,
        .sync = true,
        .current = {
            .enabled = true,
        },
        .expected = {
            .enabled = true,
        }
    }); (void)user1;

    TEST *user2 = dyncfg_unittest_add((TEST){
        .id = strdupz("unittest:async:template2:user2"),
        .source = strdupz(LINE_FILE_STR),
        .type = DYNCFG_TYPE_JOB,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_USER,
        .sync = false,
        .expected = {
            .enabled = true,
        }
    }); (void)user2;

    // ------------------------------------------------------------------------

    int rc; (void)rc;
    BUFFER *wb = buffer_create(0, NULL);

    // ------------------------------------------------------------------------
    // dynamic job

    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1 add dyn1", wb, "{\"double\":3.14,\"boolean\":true}", LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1 add dyn2", wb, "{\"double\":3.14,\"boolean\":true}", LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2 add dyn3", wb, "{\"double\":3.14,\"boolean\":true}", LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2 add dyn4", wb, "{\"double\":3.14,\"boolean\":true}", LINE_FILE_STR);

    // ------------------------------------------------------------------------
    // saving of user_disabled

    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:single1 disable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:single2 disable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1:user1 disable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2:user2 disable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1:dyn1 disable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1:dyn2 disable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2:dyn3 disable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2:dyn4 disable", wb, NULL, LINE_FILE_STR);

    // ------------------------------------------------------------------------
    // enabling

    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:single1 enable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:single2 enable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1:user1 enable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2:user2 enable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1:dyn1 enable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1:dyn2 enable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2:dyn3 enable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2:dyn4 enable", wb, NULL, LINE_FILE_STR);

    // ------------------------------------------------------------------------
    // disabling template

    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1 disable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2 disable", wb, NULL, LINE_FILE_STR);

    // ------------------------------------------------------------------------
    // enabling template

    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1 enable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2 enable", wb, NULL, LINE_FILE_STR);

    // ------------------------------------------------------------------------
    // adding job on disabled template

    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1 disable", wb, NULL, LINE_FILE_STR);
    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2 disable", wb, NULL, LINE_FILE_STR);

    TEST *user3 = dyncfg_unittest_add((TEST){
        .id = strdupz("unittest:sync:template1:user3"),
        .source = strdupz(LINE_FILE_STR),
        .type = DYNCFG_TYPE_JOB,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_USER,
        .sync = true,
        .expected = {
            .enabled = false,
        }
    }); (void)user3;

    TEST *user4 = dyncfg_unittest_add((TEST){
        .id = strdupz("unittest:async:template2:user4"),
        .source = strdupz(LINE_FILE_STR),
        .type = DYNCFG_TYPE_JOB,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_USER,
        .sync = false,
        .expected = {
            .enabled = false,
        }
    }); (void)user4;

    TEST *user5 = dyncfg_unittest_add((TEST){
        .id = strdupz("unittest:sync:template1:user5"),
        .source = strdupz(LINE_FILE_STR),
        .type = DYNCFG_TYPE_JOB,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_USER,
        .sync = true,
        .expected = {
            .enabled = false,
        }
    }); (void)user5;

    TEST *user6 = dyncfg_unittest_add((TEST){
        .id = strdupz("unittest:async:template2:user6"),
        .source = strdupz(LINE_FILE_STR),
        .type = DYNCFG_TYPE_JOB,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_USER,
        .sync = false,
        .expected = {
            .enabled = false,
        }
    }); (void)user6;

//    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1:user5 disable", wb, NULL, LINE_FILE_STR);
//    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2:user6 disable", wb, NULL, LINE_FILE_STR);

//    // ------------------------------------------------------------------------
//    // enable template with disabled jobs
//
//    user3->expected.enabled = true;
//    user5->expected.enabled = false;
//    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:template1 enable", wb, NULL, LINE_FILE_STR);
//
//    user4->expected.enabled = true;
//    user6->expected.enabled = false;
//    dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:async:template2 enable", wb, NULL, LINE_FILE_STR);


//    // ------------------------------------------------------------------------
//
//    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " tree", wb, NULL);
//    if(rc == HTTP_RESP_OK)
//        fprintf(stderr, "%s\n", buffer_tostring(wb));

    nd_thread_signal_cancel(thread);
    nd_thread_join(thread);
    dyncfg_unittest_cleanup_files();
    dictionary_destroy(dyncfg_unittest_data.nodes);
    buffer_free(wb);
    return __atomic_load_n(&dyncfg_unittest_data.errors, __ATOMIC_RELAXED) > 0 ? 1 : 0;
}
