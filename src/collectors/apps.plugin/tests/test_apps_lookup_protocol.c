// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/netipc/netipc_netdata.h"

static bool expect_ok(bool condition, const char *message)
{
    if (condition)
        return true;

    fprintf(stderr, "%s\n", message);
    return false;
}

static bool test_known_retry_later_allows_empty_cgroup_path(void)
{
    uint8_t buffer[4096];
    nipc_apps_lookup_builder_t builder;

    nipc_apps_lookup_builder_init(&builder, buffer, sizeof(buffer), 1, 42);

    nipc_error_t err = nipc_apps_lookup_builder_add(
        &builder,
        NIPC_PID_LOOKUP_KNOWN,
        NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER,
        NIPC_ORCHESTRATOR_UNKNOWN,
        1234,
        1,
        1000,
        987654321,
        "netdata",
        7,
        NULL,
        0,
        NULL,
        0,
        NULL,
        0);
    if (!expect_ok(err == NIPC_OK, "builder rejected KNOWN + RETRY_LATER with empty cgroup_path"))
        return false;

    size_t len = nipc_apps_lookup_builder_finish(&builder);
    nipc_apps_lookup_resp_view_t response;
    err = nipc_apps_lookup_resp_decode(buffer, len, &response);
    if (!expect_ok(err == NIPC_OK, "decoder rejected KNOWN + RETRY_LATER with empty cgroup_path"))
        return false;

    if (!expect_ok(response.generation == 42, "decoded response generation mismatch"))
        return false;
    if (!expect_ok(response.item_count == 1, "decoded response item count mismatch"))
        return false;

    nipc_apps_lookup_item_view_t item;
    err = nipc_apps_lookup_resp_item(&response, 0, &item);
    if (!expect_ok(err == NIPC_OK, "failed to decode APPS_LOOKUP item"))
        return false;

    return expect_ok(item.status == NIPC_PID_LOOKUP_KNOWN, "decoded PID status mismatch") &&
           expect_ok(item.cgroup_status == NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER, "decoded cgroup status mismatch") &&
           expect_ok(item.cgroup_path.len == 0, "decoded cgroup path should be empty");
}

static bool test_known_permanent_rejects_empty_cgroup_path(void)
{
    uint8_t buffer[4096];
    nipc_apps_lookup_builder_t builder;

    nipc_apps_lookup_builder_init(&builder, buffer, sizeof(buffer), 1, 42);

    nipc_error_t err = nipc_apps_lookup_builder_add(
        &builder,
        NIPC_PID_LOOKUP_KNOWN,
        NIPC_APPS_CGROUP_UNKNOWN_PERMANENT,
        NIPC_ORCHESTRATOR_UNKNOWN,
        1234,
        1,
        1000,
        987654321,
        "netdata",
        7,
        NULL,
        0,
        NULL,
        0,
        NULL,
        0);

    return expect_ok(err == NIPC_ERR_BAD_LAYOUT, "builder allowed PERMANENT with empty cgroup_path");
}

int main(void)
{
    if (!test_known_retry_later_allows_empty_cgroup_path())
        return 1;

    if (!test_known_permanent_rejects_empty_cgroup_path())
        return 1;

    return 0;
}
