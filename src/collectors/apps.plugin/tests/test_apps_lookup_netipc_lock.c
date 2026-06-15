// SPDX-License-Identifier: GPL-3.0-or-later

#include "../apps-lookup-netipc.h"
#include "../apps-cgroups-lookup-client.h"

#define TEST_PID_COUNT APPS_LOOKUP_MAX_PIDS_PER_REQUEST
#define TEST_PID_BASE 100000
#define TEST_RESPONSE_BUFFER_SIZE (16U * 1024U * 1024U)

netdata_mutex_t apps_pids_mutex;
uint64_t apps_collection_generation = 0;
int update_every = 1;

static struct pid_stat *test_pid_rows = NULL;
static struct cgroup_lookup_label test_labels[2];
static struct cgroup_lookup_entry test_cgroup_entry;

struct pid_stat *find_pid_entry(pid_t pid)
{
    if(pid < TEST_PID_BASE || pid >= TEST_PID_BASE + (pid_t)TEST_PID_COUNT)
        return NULL;

    return &test_pid_rows[pid - TEST_PID_BASE];
}

bool apps_cgroups_lookup_is_host_root_path(const char *path)
{
    return path && strcmp(path, "/") == 0;
}

#define NETDATA_APPS_LOOKUP_TEST_HOOKS 1
#include "../apps-lookup-netipc.c"

static bool expect_ok(bool condition, const char *message)
{
    if(condition)
        return true;

    fprintf(stderr, "%s\n", message);
    return false;
}

static bool setup_fixture(void)
{
    if(netdata_mutex_init(&apps_pids_mutex) != 0) {
        fprintf(stderr, "failed to initialize apps_pids_mutex\n");
        return false;
    }

    __atomic_store_n(&apps_collection_generation, 42, __ATOMIC_RELEASE);
    test_pid_rows = callocz(TEST_PID_COUNT, sizeof(*test_pid_rows));

    test_labels[0].key = string_strdupz("container_name");
    test_labels[0].value = string_strdupz("sow45-container");
    test_labels[1].key = string_strdupz("image");
    test_labels[1].value = string_strdupz("netdata/sow45");

    test_cgroup_entry = (struct cgroup_lookup_entry) {
        .cgroup_status = NIPC_CGROUP_LOOKUP_KNOWN,
        .orchestrator = NIPC_ORCHESTRATOR_DOCKER,
        .cgroup_name = string_strdupz("sow45-container"),
        .cgroup_labels = test_labels,
        .cgroup_label_count = _countof(test_labels),
    };

    for(uint32_t i = 0; i < TEST_PID_COUNT; i++) {
        struct pid_stat *p = &test_pid_rows[i];
        char comm[16];
        snprintfz(comm, sizeof(comm) - 1, "p%04u", i);

        p->pid = TEST_PID_BASE + (pid_t)i;
        p->ppid = 1;
        p->uid = 1000;
        p->starttime = 1000000 + i;
        p->comm = string_strdupz(comm);
        p->cgroup_path = string_strdupz("/docker/sow45");
        p->cgroup_cache = &test_cgroup_entry;
    }

    return true;
}

static void cleanup_fixture(void)
{
    if(test_pid_rows) {
        for(uint32_t i = 0; i < TEST_PID_COUNT; i++) {
            string_freez(test_pid_rows[i].comm);
            string_freez(test_pid_rows[i].cgroup_path);
        }
    }
    freez(test_pid_rows);
    test_pid_rows = NULL;

    string_freez(test_cgroup_entry.cgroup_name);
    for(size_t i = 0; i < _countof(test_labels); i++) {
        string_freez(test_labels[i].key);
        string_freez(test_labels[i].value);
    }
    memset(&test_cgroup_entry, 0, sizeof(test_cgroup_entry));
    memset(test_labels, 0, sizeof(test_labels));
    netdata_mutex_destroy(&apps_pids_mutex);
}

static bool response_has_expected_shape(const uint8_t *response_buffer, size_t response_len)
{
    nipc_apps_lookup_resp_view_t response;
    nipc_error_t err = nipc_apps_lookup_resp_decode(response_buffer, response_len, &response);
    if(!expect_ok(err == NIPC_OK, "APPS_LOOKUP response decode failed"))
        return false;

    if(!expect_ok(response.generation == 42, "APPS_LOOKUP generation mismatch"))
        return false;
    if(!expect_ok(response.item_count == TEST_PID_COUNT, "APPS_LOOKUP response item count mismatch"))
        return false;

    uint32_t sample_indexes[] = { 0, TEST_PID_COUNT / 2, TEST_PID_COUNT - 1 };
    for(size_t i = 0; i < _countof(sample_indexes); i++) {
        nipc_apps_lookup_item_view_t item;
        err = nipc_apps_lookup_resp_item(&response, sample_indexes[i], &item);
        if(!expect_ok(err == NIPC_OK, "APPS_LOOKUP response item decode failed"))
            return false;

        if(!expect_ok(item.status == NIPC_PID_LOOKUP_KNOWN, "APPS_LOOKUP item PID status mismatch") ||
           !expect_ok(item.cgroup_status == NIPC_APPS_CGROUP_KNOWN, "APPS_LOOKUP item cgroup status mismatch") ||
           !expect_ok(item.orchestrator == NIPC_ORCHESTRATOR_DOCKER, "APPS_LOOKUP item orchestrator mismatch") ||
           !expect_ok(item.label_count == _countof(test_labels), "APPS_LOOKUP item label count mismatch"))
            return false;
    }

    return true;
}

static bool test_8192_pid_lookup_encodes_outside_apps_pids_mutex(void)
{
    uint32_t *pids = callocz(TEST_PID_COUNT, sizeof(*pids));
    for(uint32_t i = 0; i < TEST_PID_COUNT; i++)
        pids[i] = TEST_PID_BASE + i;

    size_t request_buffer_size =
        NIPC_APPS_LOOKUP_REQ_HDR_SIZE +
        (size_t)TEST_PID_COUNT * (NIPC_LOOKUP_DIR_ENTRY_SIZE + NIPC_APPS_LOOKUP_KEY_SIZE);
    uint8_t *request_buffer = mallocz(request_buffer_size);
    size_t request_len = nipc_apps_lookup_req_encode(pids, TEST_PID_COUNT, request_buffer, request_buffer_size);
    if(!expect_ok(request_len > 0, "APPS_LOOKUP request encode failed"))
        goto fail;

    nipc_apps_lookup_req_view_t request;
    nipc_error_t err = nipc_apps_lookup_req_decode(request_buffer, request_len, &request);
    if(!expect_ok(err == NIPC_OK, "APPS_LOOKUP request decode failed"))
        goto fail;

    uint8_t *response_buffer = mallocz(TEST_RESPONSE_BUFFER_SIZE);
    nipc_apps_lookup_builder_t builder;
    nipc_apps_lookup_builder_init(&builder, response_buffer, TEST_RESPONSE_BUFFER_SIZE, TEST_PID_COUNT, 0);

    usec_t started_ut = now_monotonic_usec();
    bool ok = apps_lookup_handler(NULL, &request, &builder);
    usec_t total_ut = now_monotonic_usec() - started_ut;
    size_t response_len = nipc_apps_lookup_builder_finish(&builder);

    fprintf(
        stderr,
        "APPS_LOOKUP %u-PID handler: total=%" PRIu64 "us max_apps_pids_mutex_hold=%" PRIu64 "us emit_calls=%" PRIu64 "\n",
        (unsigned)TEST_PID_COUNT,
        (uint64_t)total_ut,
        (uint64_t)apps_lookup_test_max_lock_hold_ut,
        apps_lookup_test_emit_calls);

    bool result =
        expect_ok(ok, "APPS_LOOKUP handler failed") &&
        expect_ok(builder.error == NIPC_OK, "APPS_LOOKUP builder recorded an error") &&
        expect_ok(response_len > 0, "APPS_LOOKUP response is empty") &&
        expect_ok(apps_lookup_test_lock_acquisitions == 1, "APPS_LOOKUP did not take exactly one apps_pids_mutex critical section") &&
        expect_ok(apps_lookup_test_emit_calls == TEST_PID_COUNT, "APPS_LOOKUP did not emit exactly one row per requested PID") &&
        expect_ok(apps_lookup_test_emit_calls_while_locked == 0, "APPS_LOOKUP emitted response rows while apps_pids_mutex was held") &&
        expect_ok(apps_lookup_test_max_lock_hold_ut > 0, "APPS_LOOKUP lock-hold measurement did not record a duration") &&
        expect_ok(apps_lookup_test_max_lock_hold_ut <= total_ut, "APPS_LOOKUP lock-hold duration exceeded total handler duration") &&
        response_has_expected_shape(response_buffer, response_len);

    freez(response_buffer);
    freez(request_buffer);
    freez(pids);
    return result;

fail:
    freez(request_buffer);
    freez(pids);
    return false;
}

int main(void)
{
    if(!setup_fixture())
        return 1;

    bool ok = test_8192_pid_lookup_encodes_outside_apps_pids_mutex();
    cleanup_fixture();
    return ok ? 0 : 1;
}
