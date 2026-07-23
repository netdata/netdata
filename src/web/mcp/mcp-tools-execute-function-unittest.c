// SPDX-License-Identifier: GPL-3.0-or-later

#include "mcp-tools-execute-function.h"
#include "mcp-tools-execute-function-registry.h"
#include "database/rrdfunctions-inline.h"

// ----------------------------------------------------------------------------
// Regression test for GHSA-6628-vxm3-4g8g.
//
// An anonymous MCP execute_function caller could obtain protected function
// metadata (systemd-journal __logs_sources option names, file counts, sizes,
// coverage windows, timestamps) that the normal /api/v3/function path denies.
// The metadata is produced by mcp_functions_registry_get() -> "<fn> info", run
// with HTTP_ACCESS_ALL — so before the fix it ignored the caller's real access.
//
// The fix authorizes the caller (rrd_function_verify_access) in
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

    // The info path executes the function, which registers an inflight request.
    // Normal startup (and dyncfg_unittest) initialize this global; the standalone
    // -W mcpfunctionaccesstest path does not, so ensure it here (init is idempotent).
    rrd_functions_inflight_init();

    // A protected function mirroring systemd-journal's access requirements.
    rrd_function_add_inline(host, NULL, MCP_UT_FN, 10, 0, 1,
                            "unittest protected function", "logs",
                            HTTP_ACCESS_SIGNED_ID | HTTP_ACCESS_SAME_SPACE | HTTP_ACCESS_SENSITIVE_DATA,
                            mcp_ut_protected_cb);

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

    rrd_function_del(host, NULL, MCP_UT_FN, false, true);
    mcp_functions_registry_cleanup();

    fprintf(stderr, "%s() %s (%d error%s)\n\n",
            __FUNCTION__, errors ? "FAILED" : "passed", errors, errors == 1 ? "" : "s");
    return errors;
}
