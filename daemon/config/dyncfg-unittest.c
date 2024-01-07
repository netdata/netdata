// SPDX-License-Identifier: GPL-3.0-or-later

#include "dyncfg-internals.h"
#include "dyncfg.h"

// ----------------------------------------------------------------------------
// unit test

#define LINE_FILE_STR TOSTRING(__LINE__) "@" __FILE__

struct dyncfg_unittest {
    bool enabled;
    int errors;
} dyncfg_unittest_data = { 0 };

static int dyncfg_unittest_execute_cb(uuid_t *transaction, BUFFER *result_body_wb, BUFFER *payload,
                                      usec_t *stop_monotonic_ut, const char *function,
                                      void *execute_cb_data,
                                      rrd_function_result_callback_t result_cb, void *result_cb_data,
                                      rrd_function_progress_cb_t progress_cb, void *progress_cb_data,
                                      rrd_function_is_cancelled_cb_t is_cancelled_cb,
                                      void *is_cancelled_cb_data,
                                      rrd_function_register_canceller_cb_t register_canceller_cb,
                                      void *register_canceller_cb_data,
                                      rrd_function_register_progresser_cb_t register_progresser_cb,
                                      void *register_progresser_cb_data) {
    if(strncmp(function, PLUGINSD_FUNCTION_CONFIG " ", sizeof(PLUGINSD_FUNCTION_CONFIG)) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: received function that is not config: %s", function);
        rrd_call_function_error(result_body_wb, "wrong function", 400);
        if(result_cb)
            result_cb(result_body_wb, 400, result_cb_data);
        return 400;
    }

    // extract the id
    const char *id = &function[sizeof(PLUGINSD_FUNCTION_CONFIG)];
    while(*id && isspace(*id)) id++;
    const char *space = id;
    while(*space && !isspace(*space)) space++;
    size_t id_len = space - id;

    char id_copy[id_len + 1];
    memcpy(id_copy, id, id_len);
    id_copy[id_len] = '\0';

    // extract the cmd
    const char *cmd = space;
    while(*cmd && isspace(*cmd)) cmd++;
    space = cmd;
    while(*space && !isspace(*space)) space++;
    size_t cmd_len = space - cmd;

    char cmd_copy[cmd_len + 1];
    memcpy(cmd_copy, cmd, cmd_len);
    cmd_copy[cmd_len] = '\0';
    DYNCFG_CMDS c = dyncfg_cmds2id(cmd_copy);

    int code = HTTP_RESP_OK;

    if(c == DYNCFG_CMD_ENABLE)
        dyncfg_unittest_data.enabled = true;
    else if(c == DYNCFG_CMD_DISABLE)
        dyncfg_unittest_data.enabled = false;
    else if(c == DYNCFG_CMD_UPDATE) {
        if(!payload || !buffer_tostring(payload))
            code = 32763;
        else
            nd_log(NDLS_DAEMON, NDLP_INFO,
                   "DYNCFG: received update for '%s' with payload: %s",
                   id_copy, buffer_tostring(payload));
    }
    else if(c == DYNCFG_CMD_ADD) {
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "DYNCFG: received add for '%s' with payload: %s",
               id_copy, buffer_tostring(payload));
    }
    else
        code = 32764;

    if(result_cb)
        result_cb(result_body_wb, HTTP_RESP_OK, result_cb_data);

    return code;
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
        dyncfg_unittest_data.errors++;
    }

    return rc;
}

int dyncfg_unittest(void) {
    rrd_functions_inflight_init();
    dyncfg_init(false);

    dyncfg_unittest_data.enabled = false;
    dyncfg_add_low_level(
        localhost,
        "unittest:sync:single",
        "/unittests",
        DYNCFG_STATUS_OK,
        DYNCFG_TYPE_SINGLE,
        DYNCFG_SOURCE_TYPE_INTERNAL,
        LINE_FILE_STR,
        DYNCFG_CMD_GET | DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE,
        0,
        0,
        true,
        dyncfg_unittest_execute_cb,
        &dyncfg_unittest_data);

    dyncfg_add_low_level(
        localhost,
        "unittest:sync:jobs",
        "/unittests",
        DYNCFG_STATUS_OK,
        DYNCFG_TYPE_TEMPLATE,
        DYNCFG_SOURCE_TYPE_INTERNAL,
        LINE_FILE_STR,
        DYNCFG_CMD_SCHEMA | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE | DYNCFG_CMD_ADD | DYNCFG_CMD_RESTART,
        0,
        0,
        true,
        dyncfg_unittest_execute_cb,
        &dyncfg_unittest_data);

    dyncfg_add_low_level(
        localhost,
        "unittest:sync:jobs:stock",
        "/unittests",
        DYNCFG_STATUS_OK,
        DYNCFG_TYPE_JOB,
        DYNCFG_SOURCE_TYPE_STOCK,
        LINE_FILE_STR,
        DYNCFG_CMD_GET | DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE |
            DYNCFG_CMD_RESTART | DYNCFG_CMD_TEST,
        0,
        0,
        true,
        dyncfg_unittest_execute_cb,
        &dyncfg_unittest_data);

    dyncfg_add_low_level(
        localhost,
        "unittest:sync:jobs:user",
        "/unittests",
        DYNCFG_STATUS_OK,
        DYNCFG_TYPE_JOB,
        DYNCFG_SOURCE_TYPE_USER,
        LINE_FILE_STR,
        DYNCFG_CMD_GET | DYNCFG_CMD_SCHEMA | DYNCFG_CMD_UPDATE | DYNCFG_CMD_ENABLE | DYNCFG_CMD_DISABLE |
            DYNCFG_CMD_RESTART | DYNCFG_CMD_TEST,
        0,
        0,
        true,
        dyncfg_unittest_execute_cb,
        &dyncfg_unittest_data);

    int rc;
    BUFFER *wb = buffer_create(0, NULL);

    // ------------------------------------------------------------------------

    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:single enable", wb, NULL);
    if(rc == HTTP_RESP_OK && !dyncfg_unittest_data.enabled) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: enabled flag is not set, code %d", rc);
        dyncfg_unittest_data.errors++;
    }

    // ------------------------------------------------------------------------

    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:single disable", wb, NULL);
    if(rc == HTTP_RESP_OK && dyncfg_unittest_data.enabled) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: enabled flag is set, code %d", rc);
        dyncfg_unittest_data.errors++;
    }

    // ------------------------------------------------------------------------
    DYNCFG *df;

    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:single update", wb, "hello world");
    df = dictionary_get(dyncfg_globals.nodes, "unittest:sync:single");
    if(rc == HTTP_RESP_OK && (!df || !df->payload || strcmp(buffer_tostring(df->payload), "hello world") != 0)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: failed to update a single job");
        dyncfg_unittest_data.errors++;
    }

    // ------------------------------------------------------------------------

    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " unittest:sync:jobs add master-db", wb, "master-db configuration instructions");
    df = dictionary_get(dyncfg_globals.nodes, "unittest:sync:jobs:master-db");
    if(rc == HTTP_RESP_OK && (!df || !df->payload || strcmp(buffer_tostring(df->payload), "master-db configuration instructions") != 0)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "DYNCFG UNITTEST: failed to test adding a job to an existing template");
        dyncfg_unittest_data.errors++;
    }

    // ------------------------------------------------------------------------

    rc = dyncfg_unittest_run(PLUGINSD_FUNCTION_CONFIG " tree", wb, NULL);
    if(rc == HTTP_RESP_OK)
        fprintf(stderr, "%s\n", buffer_tostring(wb));

    return dyncfg_unittest_data.errors;
}
