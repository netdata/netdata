// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-execute-function.h"
#include "mcp-tools-execute-function-registry.h"
#include "nrpc/nrpc.h"

// ----------------------------------------------------------------------------
// Regression test for GHSA-6628-vxm3-4g8g.
//
// An anonymous MCP execute_function caller could obtain protected function
// metadata (systemd-journal __logs_sources option names, file counts, sizes,
// coverage windows, timestamps) that the normal /api/v3/function path denies.
// The metadata is produced by mcp_functions_registry_get() -> "<fn> info", run
// with HTTP_ACCESS_ALL — so before the fix it ignored the caller's real access.
//
// The fix authorizes the caller (nrpc_method_authorize) in
// mcp_tool_execute_function_execute() BEFORE that metadata path runs.
//
// This test drives the real MCP handler with an anonymous caller and a
// protected function whose "info" callback:
//   (a) increments a call counter — reaching it means the elevated metadata
//       path executed, which must NOT happen for an unauthorized caller; and
//   (b) embeds a secret marker in its option metadata — which must never appear
//       in the response sent to an unauthorized caller.
// Remove the authorization call from the handler and this test fails: the
// handler falls through to the info path, the counter increments, and the
// marker leaks.

#define MCP_UT_FN "unittest-mcp-protected"
#define MCP_UT_SECRET "UNITTEST_SECRET_SOURCE_METADATA"

static int mcp_ut_info_calls = 0;

// Inline execute callback. Returns a systemd-journal-like "info" schema whose
// required parameter carries option metadata (the sensitive part). Reaching
// here means the function actually ran under elevated privileges.
static int mcp_ut_protected_cb(BUFFER *wb, const char *function __maybe_unused,
                               BUFFER *payload __maybe_unused, const char *source __maybe_unused) {
    mcp_ut_info_calls++;

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);
    buffer_json_member_add_uint64(wb, "v", 3);
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_string(wb, "help", "unittest protected function");

    buffer_json_member_add_array(wb, "required_params");
    {
        buffer_json_add_array_item_object(wb); // the __logs_sources selector
        buffer_json_member_add_string(wb, "id", "__logs_sources");
        buffer_json_member_add_string(wb, "name", "source");
        buffer_json_member_add_string(wb, "help", "select a log source");
        buffer_json_member_add_string(wb, "type", "select");

        buffer_json_member_add_array(wb, "options");
        {
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "id", "all");
            buffer_json_member_add_string(wb, "name", "all");
            // Sensitive metadata that /api/v3/function denies to anonymous callers.
            buffer_json_member_add_string(wb, "info", MCP_UT_SECRET " 4 files, 48MiB, covering 2d");
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb); // options

        buffer_json_object_close(wb);
    }
    buffer_json_array_close(wb); // required_params

    buffer_json_finalize(wb);
    return HTTP_RESP_OK;
}

static bool mcp_ut_output_contains(MCP_CLIENT *mcpc, const char *needle) {
    if (mcpc->error && buffer_strlen(mcpc->error) && strstr(buffer_tostring(mcpc->error), needle))
        return true;
    for (size_t i = 0; i < mcpc->response_chunks_used; i++) {
        BUFFER *b = mcpc->response_chunks[i].buffer;
        if (b && buffer_strlen(b) && strstr(buffer_tostring(b), needle))
            return true;
    }
    return false;
}

static struct json_object *mcp_ut_params(const char *node, const char *function) {
    struct json_object *p = json_object_new_object();
    json_object_object_add(p, "node", json_object_new_string(node));
    json_object_object_add(p, "function", json_object_new_string(function));
    return p;
}

int mcp_execute_function_access_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    RRDHOST *host = localhost;
    if (!host) {
        fprintf(stderr, "  FAILED: localhost is NULL (rrd_init not prepared)\n");
        return 1;
    }

    mcp_functions_registry_init();

    // The info path executes the method, which registers an in-flight
    // call. Normal startup (and dyncfg_unittest) create the in-flight calls table; the
    // standalone -W mcpfunctionaccesstest path does not, so ensure it here
    // (create is idempotent).
    nrpc_inflight_calls_create();

    // A protected function mirroring systemd-journal's access requirements.
    nrpc_method_register_builtin(&(struct nrpc_builtin_desc) {
        .owner = rrdhost_nrpc_owner(host),
        .name = MCP_UT_FN,
        .help = "unittest protected function",
        .tags = "logs",
        .timeout_s = 10,
        .priority = 0,
        .version = 1,
        .access = HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
        .handler = mcp_ut_protected_cb,
    });

    const char *node = rrdhost_hostname(host);
    int errors = 0;

    // ---- CASE 1: anonymous caller MUST be denied before any metadata is generated.
    // Runs first, while the registry cache is empty: if the authorization gate is
    // removed, the handler reaches mcp_functions_registry_get() -> "<fn> info",
    // the callback fires, and the option metadata leaks.
    {
        MCP_CLIENT *mcpc = mcp_create_client(MCP_TRANSPORT_HTTP, NULL);
        USER_AUTH anon;
        memset(&anon, 0, sizeof(anon));
        anon.access = HTTP_ACCESS_ANONYMOUS_DATA;
        mcpc->user_auth = &anon;

        struct json_object *params = mcp_ut_params(node, MCP_UT_FN);

        mcp_ut_info_calls = 0;
        MCP_RETURN_CODE rc = mcp_tool_execute_function_execute(mcpc, params, 0);

        if (rc == MCP_RC_OK) {
            fprintf(stderr, "  FAILED: anonymous execute_function returned MCP_RC_OK (expected denial)\n");
            errors++;
        }
        if (mcp_ut_info_calls != 0) {
            fprintf(stderr, "  FAILED: metadata (info) path executed for anonymous caller "
                            "(callback fired %d time(s)) — GHSA-6628-vxm3-4g8g regression\n",
                    mcp_ut_info_calls);
            errors++;
        }
        if (mcp_ut_output_contains(mcpc, MCP_UT_SECRET)) {
            fprintf(stderr, "  FAILED: protected option metadata leaked to anonymous caller — "
                            "GHSA-6628-vxm3-4g8g regression\n");
            errors++;
        }
        if (rc != MCP_RC_OK && mcp_ut_info_calls == 0 && !mcp_ut_output_contains(mcpc, MCP_UT_SECRET))
            fprintf(stderr, "  OK: anonymous caller denied before metadata generation\n");

        json_object_put(params);
        mcp_free_client(mcpc);
    }

    // ---- CASE 2 (positive control): an authorized caller reaches the metadata path.
    // Without this, CASE 1's zero would be meaningless — proves the callback CAN
    // fire through this handler and the gate does not over-block. The authorized
    // caller supplies no selections, so the handler stops at the required-params
    // prompt (no data query), keeping the control robust.
    {
        MCP_CLIENT *mcpc = mcp_create_client(MCP_TRANSPORT_HTTP, NULL);
        USER_AUTH full;
        memset(&full, 0, sizeof(full));
        full.access = HTTP_ACCESS_ALL;
        mcpc->user_auth = &full;

        struct json_object *params = mcp_ut_params(node, MCP_UT_FN);

        mcp_ut_info_calls = 0;
        (void) mcp_tool_execute_function_execute(mcpc, params, 0);

        if (mcp_ut_info_calls == 0) {
            fprintf(stderr, "  FAILED: authorized caller never reached the metadata path "
                            "(gate over-blocks, or the positive control is broken)\n");
            errors++;
        }
        else
            fprintf(stderr, "  OK: authorized caller reached the metadata path "
                            "(callback fired %d time(s))\n", mcp_ut_info_calls);

        json_object_put(params);
        mcp_free_client(mcpc);
    }

    nrpc_method_unregister(rrdhost_nrpc_owner(host), MCP_UT_FN, NRPC_SOURCE_DAEMON);
    mcp_functions_registry_cleanup();

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");
    return errors;
}

// ----------------------------------------------------------------------------
// The POST body sent to a v3 has_history Function carries "selections" only
// when it holds an entry. A free-text "q" becomes a wildcard condition that
// never turns into a selection, and Functions with a strict wire (otel-traces)
// reject an empty "selections" object as an unknown field. The control
// Function has a required parameter, so a caller's selection must still reach
// its body.

#define MCP_UT_PAYLOAD_FN "unittest-mcp-payload"
#define MCP_UT_PAYLOAD_REQUIRED_FN "unittest-mcp-payload-required"

static char mcp_ut_payload_seen[4096];
static int mcp_ut_payload_calls = 0;

static int mcp_ut_payload_cb(BUFFER *wb, const char *function,
                             BUFFER *payload, const char *source __maybe_unused) {
    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    if (strstr(function, " info")) {
        buffer_json_member_add_uint64(wb, "v", 3);
        buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
        buffer_json_member_add_string(wb, "type", "traces");
        buffer_json_member_add_boolean(wb, "has_history", true);
        buffer_json_member_add_string(wb, "help", "unittest payload function");
        buffer_json_member_add_array(wb, "accepted_params");
        buffer_json_add_array_item_string(wb, "after");
        buffer_json_add_array_item_string(wb, "before");
        buffer_json_add_array_item_string(wb, "last");
        buffer_json_array_close(wb);
        buffer_json_member_add_array(wb, "required_params");
        if (strstr(function, MCP_UT_PAYLOAD_REQUIRED_FN)) {
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "id", "__source");
            buffer_json_member_add_string(wb, "name", "source");
            buffer_json_member_add_string(wb, "type", "select");
            buffer_json_member_add_array(wb, "options");
            buffer_json_add_array_item_object(wb);
            buffer_json_member_add_string(wb, "id", "all");
            buffer_json_member_add_string(wb, "name", "all");
            buffer_json_object_close(wb);
            buffer_json_array_close(wb);
            buffer_json_object_close(wb);
        }
        buffer_json_array_close(wb);
    }
    else {
        mcp_ut_payload_calls++;
        strncpyz(mcp_ut_payload_seen, payload ? buffer_tostring(payload) : "",
                 sizeof(mcp_ut_payload_seen) - 1);
        buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
        buffer_json_member_add_string(wb, "type", "traces");
    }

    buffer_json_finalize(wb);
    return HTTP_RESP_OK;
}

int mcp_execute_function_payload_unittest(void) {
    fprintf(stderr, "\n%s() running...\n", __FUNCTION__);

    RRDHOST *host = localhost;
    if (!host) {
        fprintf(stderr, "  FAILED: localhost is NULL (rrd_init not prepared)\n");
        return 1;
    }

    mcp_functions_registry_init();
    nrpc_inflight_calls_create();

    const char *names[] = { MCP_UT_PAYLOAD_FN, MCP_UT_PAYLOAD_REQUIRED_FN };
    for (size_t i = 0; i < _countof(names); i++)
        nrpc_method_register_builtin(&(struct nrpc_builtin_desc) {
            .owner = rrdhost_nrpc_owner(host),
            .name = names[i],
            .help = "unittest payload function",
            .tags = "traces",
            .timeout_s = 10,
            .priority = 0,
            .version = 3,
            .access = HTTP_ACCESS_NONE,
            .handler = mcp_ut_payload_cb,
        });

    struct {
        const char *function;
        const char *selections;
        const char *expected;
    } cases[] = {
        { MCP_UT_PAYLOAD_FN, NULL, NULL },
        { MCP_UT_PAYLOAD_REQUIRED_FN, "{\"__source\":\"all\"}", "\"selections\":{\"__source\":[\"all\"]}" },
    };

    int errors = 0;
    for (size_t i = 0; i < _countof(cases); i++) {
        MCP_CLIENT *mcpc = mcp_create_client(MCP_TRANSPORT_HTTP, NULL);
        USER_AUTH full;
        memset(&full, 0, sizeof(full));
        full.access = HTTP_ACCESS_ALL;
        mcpc->user_auth = &full;

        struct json_object *params = mcp_ut_params(rrdhost_hostname(host), cases[i].function);
        json_object_object_add(params, "after", json_object_new_int(-3600));
        json_object_object_add(params, "before", json_object_new_int(0));
        json_object_object_add(params, "q", json_object_new_string("checkout"));
        if (cases[i].selections)
            json_object_object_add(params, "selections", json_tokener_parse(cases[i].selections));

        mcp_ut_payload_calls = 0;
        mcp_ut_payload_seen[0] = '\0';
        (void) mcp_tool_execute_function_execute(mcpc, params, 0);

        const char *found = strstr(mcp_ut_payload_seen, "\"selections\"");
        struct json_object *parsed = json_tokener_parse(mcp_ut_payload_seen);
        if (mcp_ut_payload_calls != 1) {
            fprintf(stderr, "  FAILED: %s: the data call ran %d time(s), expected 1\n",
                    cases[i].function, mcp_ut_payload_calls);
            errors++;
        }
        else if (!parsed) {
            fprintf(stderr, "  FAILED: %s: the body is not valid JSON: %s\n",
                    cases[i].function, mcp_ut_payload_seen);
            errors++;
        }
        else if (cases[i].expected ? !strstr(mcp_ut_payload_seen, cases[i].expected) : found != NULL) {
            fprintf(stderr, "  FAILED: %s: unexpected body %s\n", cases[i].function, mcp_ut_payload_seen);
            errors++;
        }
        else
            fprintf(stderr, "  OK: %s: %s\n", cases[i].function, mcp_ut_payload_seen);

        if (parsed)
            json_object_put(parsed);
        json_object_put(params);
        mcp_free_client(mcpc);
    }

    for (size_t i = 0; i < _countof(names); i++)
        nrpc_method_unregister(rrdhost_nrpc_owner(host), names[i], NRPC_SOURCE_DAEMON);
    mcp_functions_registry_cleanup();

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");
    return errors;
}
