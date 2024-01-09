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
    bool sync;
    DYNCFG_TYPE type;
    DYNCFG_CMDS cmds;
    DYNCFG_SOURCE_TYPE source_type;

    TEST_CFG current;
    TEST_CFG expected;

    bool received;
    bool finished;
} TEST;

struct dyncfg_unittest_action {
    TEST *t;
    BUFFER *result;
    BUFFER *payload;
    DYNCFG_CMDS cmd;
    const char *add_name;

    rrd_function_result_callback_t result_cb;
    void *result_cb_data;
};

static void dyncfg_unittest_register_error(const char *id, const char *msg) {
    if(msg)
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: error on id '%s': %s", id ? id : "", msg);

    __atomic_add_fetch(&dyncfg_unittest_data.errors, 1, __ATOMIC_RELAXED);
}

static int dyncfg_unittest_execute_cb(uuid_t *transaction __maybe_unused, BUFFER *result, BUFFER *payload,
                                      usec_t *stop_monotonic_ut __maybe_unused, const char *function,
                                      void *execute_cb_data,
                                      rrd_function_result_callback_t result_cb, void *result_cb_data,
                                      rrd_function_progress_cb_t progress_cb __maybe_unused, void *progress_cb_data __maybe_unused,
                                      rrd_function_is_cancelled_cb_t is_cancelled_cb __maybe_unused,
                                      void *is_cancelled_cb_data __maybe_unused,
                                      rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
                                      void *register_canceller_cb_data __maybe_unused,
                                      rrd_function_register_progresser_cb_t register_progresser_cb __maybe_unused,
                                      void *register_progresser_cb_data __maybe_unused);

bool dyncfg_unittest_parse_payload(BUFFER *payload, TEST *t, DYNCFG_CMDS cmd, const char *add_name) {
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
            .id = NULL,
            .cmds = t->cmds | DYNCFG_CMD_GET | DYNCFG_CMD_REMOVE | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
            .sync = t->sync,
            .type = DYNCFG_TYPE_JOB,
            .source_type = DYNCFG_SOURCE_TYPE_DYNCFG,
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
                }}};
        const DICTIONARY_ITEM *item = dictionary_set_and_acquire_item(dyncfg_unittest_data.nodes, buf, &tmp, sizeof(tmp));
        TEST *t2 = dictionary_acquired_item_value(item);
        t2->id = dictionary_acquired_item_name(item);
        dictionary_acquired_item_release(dyncfg_unittest_data.nodes, item);

        dyncfg_add_low_level(localhost, t2->id, "/unittest",
                             DYNCFG_STATUS_RUNNING, t2->type, t2->source_type, LINE_FILE_STR,
                             t2->cmds, 0, 0, t2->sync,
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
        rc = dyncfg_unittest_parse_payload(a->payload, a->t, a->cmd, a->add_name) ? HTTP_RESP_OK : HTTP_RESP_BAD_REQUEST;
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

static void *dyncfg_unittest_thread_action(void *ptr) {
    dyncfg_unittest_action(ptr);
    return ptr;
}

static int dyncfg_unittest_execute_cb(uuid_t *transaction __maybe_unused, BUFFER *result, BUFFER *payload,
                                      usec_t *stop_monotonic_ut __maybe_unused, const char *function,
                                      void *execute_cb_data,
                                      rrd_function_result_callback_t result_cb, void *result_cb_data,
                                      rrd_function_progress_cb_t progress_cb __maybe_unused, void *progress_cb_data __maybe_unused,
                                      rrd_function_is_cancelled_cb_t is_cancelled_cb __maybe_unused,
                                      void *is_cancelled_cb_data __maybe_unused,
                                      rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
                                      void *register_canceller_cb_data __maybe_unused,
                                      rrd_function_register_progresser_cb_t register_progresser_cb __maybe_unused,
                                      void *register_progresser_cb_data __maybe_unused) {

    int rc;
    bool run_the_callback = true;
    TEST *t = execute_cb_data;

    t->received = true;

    char buf[strlen(function) + 1];
    memcpy(buf, function, sizeof(buf));

    char *words[MAX_FUNCTION_PARAMETERS];    // an array of pointers for the words in this line
    size_t num_words = quoted_strings_splitter_pluginsd(buf, words, MAX_FUNCTION_PARAMETERS);

    const char *config = get_word(words, num_words, 0);
    const char *id = get_word(words, num_words, 1);
    const char *action = get_word(words, num_words, 2);
    const char *add_name = get_word(words, num_words, 3);

    if(!config || !*config || strcmp(config, PLUGINSD_FUNCTION_CONFIG) != 0) {
        char *msg = "did not receive a config call";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    if(!id || !*id) {
        char *msg = "did not receive an id";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    if(strcmp(t->id, id) != 0) {
        char *msg = "id received is not the expected";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    if(!action || !*action) {
        char *msg = "did not receive an action";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    DYNCFG_CMDS cmd = dyncfg_cmds2id(action);
    if(cmd == DYNCFG_CMD_NONE) {
        char *msg = "action received is not known";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    if(!(t->cmds & cmd)) {
        char *msg = "received a command that is not supported";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    if(t->current.removed && cmd != DYNCFG_CMD_ADD) {
        char *msg = "received a command for a removed entry";
        dyncfg_unittest_register_error(id, msg);
        rc = dyncfg_default_response(result, HTTP_RESP_BAD_REQUEST, msg);
        goto cleanup;
    }

    struct dyncfg_unittest_action *a = callocz(1, sizeof(*a));
    a->t = t;
    a->add_name = add_name ? strdupz(add_name) : NULL;
    a->result = result;
    a->payload = buffer_dup(payload);
    a->cmd = cmd;
    a->result_cb = result_cb;
    a->result_cb_data = result_cb_data;

    run_the_callback = false;

    if(t->sync)
        rc = dyncfg_unittest_action(a);
    else {
        netdata_thread_t thread;
        netdata_thread_create(&thread, "unittest", NETDATA_THREAD_OPTION_DEFAULT, dyncfg_unittest_thread_action, a);
    }

cleanup:
    if(run_the_callback) {
        __atomic_store_n(&t->finished, true, __ATOMIC_RELAXED);

        if (result_cb)
            result_cb(result, rc, result_cb_data);
    }

    return rc;
}

static int dyncfg_unittest_run(const char *cmd, BUFFER *wb, const char *payload) {
    buffer_flush(wb);

    CLEAN_BUFFER *pld = NULL;

    if(payload) {
        pld = buffer_create(1024, NULL);
        buffer_strcat(pld, payload);
    }

    int rc = rrd_function_run(localhost, wb, 10, HTTP_ACCESS_ADMIN, cmd,
                              true, NULL,
                              NULL, NULL,
                              NULL, NULL,
                              NULL, NULL,
                              pld);
    if(rc != HTTP_RESP_OK) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: failed to run: %s; returned code %d", cmd, rc);
        dyncfg_unittest_register_error(NULL, NULL);
    }

    return rc;
}

static void dyncfg_unittest_cleanup_files(void) {
    char path[PATH_MAX];
    snprintfz(path, sizeof(path), "%s/%s", netdata_configured_varlib_dir, "config");

    DIR *dir = opendir(path);
    if (!dir) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: cannot open directory '%s'", path);
        return;
    }

    struct dirent *entry;
    char filename[PATH_MAX];
    while ((entry = readdir(dir)) != NULL) {
        if ((entry->d_type == DT_REG || entry->d_type == DT_LNK) && strstartswith(entry->d_name, "unittest:") && strendswith(entry->d_name, ".dyncfg")) {
            snprintf(filename, sizeof(filename), "%s/%s", path, entry->d_name);
            nd_log(NDLS_DAEMON, NDLP_INFO, "DYNCFG UNITTEST: deleting file '%s'", filename);
            unlink(filename);
        }
    }

    closedir(dir);
}

static void dyncfg_unittest_add(TEST *t) {
    dictionary_set(dyncfg_unittest_data.nodes, t->id, &t, sizeof(*t));

    if(!dyncfg_add_low_level(localhost, t->id, "/unittests", DYNCFG_STATUS_RUNNING, t->type,
        t->source_type, LINE_FILE_STR,
        t->cmds, 0, 0, t->sync, dyncfg_unittest_execute_cb, t)) {
        dyncfg_unittest_register_error(t->id, "addition of job failed");
    }
}

static void dyncfg_unittest_check(TEST *t, const char *name, bool received) {
    size_t errors = 0;

    fprintf(stderr, "TEST '%s' on '%s'...", name, t->id);

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
        static const struct timespec ns = { .tv_sec = 0, .tv_nsec = 1 };
        nanosleep(&ns, NULL);

        if(now_monotonic_usec() > give_up_ut) {
            fprintf(stderr, "\n  - gave up waiting for the plugin to process this!");
            errors++;
            goto cleanup;
        }
    }

    if(t->current.enabled != t->expected.enabled) {
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

cleanup:
    if(errors) {
        fprintf(stderr, "\n  >>> FAILED\n\n");
        dyncfg_unittest_register_error(NULL, NULL);
    }
    else
        fprintf(stderr, " OK\n");

}

int dyncfg_unittest(void) {
    dyncfg_unittest_data.nodes = dictionary_create(DICT_OPTION_NONE);

    dyncfg_unittest_cleanup_files();
    rrd_functions_inflight_init();
    dyncfg_init(false);

    TEST tss1 = {
        .id = "unittest:sync:single1",
        .type = DYNCFG_TYPE_SINGLE,
        .cmds = DYNCFG_CMD_GET | DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
        .sync = true,
        .expected = {
            .enabled = true,
        }
    };
    dyncfg_unittest_add(&tss1);
    dyncfg_unittest_check(&tss1, "tss1-1", true);

    TEST tas1 = {
        .id = "unittest:async:single1",
        .type = DYNCFG_TYPE_SINGLE,
        .cmds = DYNCFG_CMD_GET | DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
        .sync = false,
        .expected = {
            .enabled = true,
        }
    };
    dyncfg_unittest_add(&tas1);
    dyncfg_unittest_check(&tas1, "tas1-1", true);

    TEST tst1 = {
        .id = "unittest:sync:template1",
        .type = DYNCFG_TYPE_TEMPLATE,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_ADD | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
        .sync = true,
        .expected = {
            .enabled = true,
        }
    };
    dyncfg_unittest_add(&tst1);
    dyncfg_unittest_check(&tst1, "tst1-1", false);

    TEST tat1 = {
        .id = "unittest:async:template1",
        .type = DYNCFG_TYPE_TEMPLATE,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_ADD | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
        .sync = false,
        .expected = {
            .enabled = true,
        }
    };
    dyncfg_unittest_add(&tat1);
    dyncfg_unittest_check(&tat1, "tat1-1", false);

    TEST taj1 = {
        .id = "unittest:async:job1",
        .type = DYNCFG_TYPE_JOB,
        .cmds = DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        .source_type = DYNCFG_SOURCE_TYPE_INTERNAL,
        .sync = false,
        .expected = {
            .enabled = true,
        }
    };
    dyncfg_unittest_add(&taj1);
    dyncfg_unittest_check(&taj1, "taj1-1", true);



    

//    int rc;
//    BUFFER *wb = buffer_create(0, NULL);
//
//    // ------------------------------------------------------------------------
//
//    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:single enable", wb, NULL);
//    if(rc == HTTP_RESP_OK && !dyncfg_unittest_data.enabled) {
//        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: enabled flag is not set, code %d", rc);
//        dyncfg_unittest_data.errors++;
//    }
//
//    // ------------------------------------------------------------------------
//
//    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:single disable", wb, NULL);
//    if(rc == HTTP_RESP_OK && dyncfg_unittest_data.enabled) {
//        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: enabled flag is set, code %d", rc);
//        dyncfg_unittest_data.errors++;
//    }
//
//    // ------------------------------------------------------------------------
//    DYNCFG *df;
//
//    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:single update", wb, "hello world");
//    df = dictionary_get(dyncfg_globals.nodes, "unittest:sync:single");
//    if(rc == HTTP_RESP_OK && (!df || !df->payload || strcmp(buffer_tostring(df->payload), "hello world") != 0)) {
//        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: failed to update a single job");
//        dyncfg_unittest_data.errors++;
//    }
//
//    // ------------------------------------------------------------------------
//
//    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:jobs add master-db", wb, "master-db configuration instructions");
//    df = dictionary_get(dyncfg_globals.nodes, "unittest:sync:jobs:master-db");
//    if(rc == HTTP_RESP_OK && (!df || !df->payload || strcmp(buffer_tostring(df->payload), "master-db configuration instructions") != 0)) {
//        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: failed to test adding a job to an existing template");
//        dyncfg_unittest_data.errors++;
//    }
//
//    // ------------------------------------------------------------------------
//
//    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " tree", wb, NULL);
//    if(rc == HTTP_RESP_OK)
//        fprintf(stderr, "%s\n", buffer_tostring(wb));

    dyncfg_unittest_cleanup_files();
    dictionary_destroy(dyncfg_unittest_data.nodes);
    return __atomic_load_n(&dyncfg_unittest_data.errors, __ATOMIC_RELAXED) > 0 ? 1 : 0;
}
