// SPDX-License-Identifier: GPL-3.0-or-later

// Drives the topology actor collection directly, with hand-built process actors, so the labels of
// an actor can be checked without live sockets or the apps lookup service.

#include "../network-viewer-topology.c"

netdata_mutex_t stdout_mutex;
#if defined(LOCAL_SOCKETS_USE_SETNS)
SPAWN_SERVER *spawn_srv = NULL;
#endif

typedef struct {
    pid_t pid;
    const char *process;
} TEST_PROCESS;

static void test_context_init(NV_TOPOLOGY_CONTEXT *ctx, NV_TOPOLOGY_GROUP_BY group_by)
{
    memset(ctx, 0, sizeof(*ctx));
    ctx->process_actors = topology_dict_create(sizeof(NV_PROCESS_ACTOR));
    ctx->remote_actors = topology_dict_create(sizeof(NV_REMOTE_ACTOR));
    ctx->local_ips = topology_dict_create(sizeof(bool));
    ctx->endpoint_owners = topology_dict_create(sizeof(NV_ENDPOINT_OWNER));
    ctx->links = topology_dict_create(sizeof(NV_TOPOLOGY_LINK));
    ctx->container_field_snapshot = topology_dict_create(sizeof(NV_TOPOLOGY_CONTAINER_FIELDS));
    ctx->pid_starttime_cache = topology_dict_create(sizeof(NV_PID_STARTTIME_CACHE_ENTRY));
    topology_strncpy(ctx->hostname, sizeof(ctx->hostname), "test-host");
    topology_strncpy(ctx->machine_guid, sizeof(ctx->machine_guid), "test-guid");
    ctx->options.group_by = group_by;
}

// A process with no start time: FreeBSD always, Linux when the process exits before its stat is read.
static void test_add_process(NV_TOPOLOGY_CONTEXT *ctx, const TEST_PROCESS *p)
{
    NV_PROCESS_ACTOR pa = {
        .pid = p->pid,
        .ppid = 1,
        .uid = 0,
        .starttime = 0,
        .sockets = 1,
    };
    topology_strncpy(pa.process, sizeof(pa.process), p->process);

    char key[32];
    snprintfz(key, sizeof(key), "pid=%d", (int)p->pid);
    dictionary_set(ctx->process_actors, key, &pa, sizeof(pa));
}

static const char *group_by_name(NV_TOPOLOGY_GROUP_BY group_by)
{
    switch(group_by) {
        case NV_TOPOLOGY_GROUP_BY_PID:
            return "pid";
        case NV_TOPOLOGY_GROUP_BY_CONTAINER:
            return "container";
        default:
            return "process_name";
    }
}

// Every process actor must carry exactly one container_name label, with the expected value and kind.
static bool test_container_name_labels(
    NV_TOPOLOGY_GROUP_BY group_by,
    const TEST_PROCESS *processes,
    size_t process_count,
    size_t expected_actors,
    const char *expected,
    const char *expected_kind)
{
    NV_TOPOLOGY_CONTEXT ctx;
    test_context_init(&ctx, group_by);
    for(size_t i = 0; i < process_count; i++)
        test_add_process(&ctx, &processes[i]);

    NV_TOPOLOGY_RENDER_STATE state;
    topology_render_state_init(&state, &ctx);
    NV_TOPOLOGY_V1_PAYLOAD payload;
    topology_v1_init(&payload);

    bool ok = true;
    const char *mode = group_by_name(group_by);
    if(topology_v1_collect_actors(&ctx, &state, &payload) != NV_TOPOLOGY_ABORT_NONE) {
        fprintf(stderr, "group_by=%s: actor collection aborted\n", mode);
        ok = false;
        goto cleanup;
    }

    size_t process_actors = 0;
    for(uint64_t a = 0; a < payload.actors.used; a++) {
        const NV_TOPOLOGY_V1_ACTOR *actor = &payload.actors.rows[a];
        if(strcmp(actor->type, "self") == 0)
            continue;

        process_actors++;
        size_t found = 0;
        for(size_t l = 0; l < payload.labels.used; l++) {
            const NV_TOPOLOGY_V1_ACTOR_LABEL *label = &payload.labels.rows[l];
            if(label->actor != a || strcmp(label->key, "container_name") != 0)
                continue;

            found++;
            if(strcmp(label->value, expected) != 0 || strcmp(label->kind, expected_kind) != 0) {
                fprintf(stderr, "group_by=%s actor '%s': container_name label '%s' (%s), expected '%s' (%s)\n",
                        mode, actor->id, label->value, label->kind, expected, expected_kind);
                ok = false;
            }
        }

        if(found != 1) {
            fprintf(stderr, "group_by=%s actor '%s': %zu container_name labels, expected 1\n", mode, actor->id, found);
            ok = false;
        }
    }

    if(process_actors != expected_actors) {
        fprintf(stderr, "group_by=%s: %zu process actors, expected %zu\n", mode, process_actors, expected_actors);
        ok = false;
    }

cleanup:
    topology_v1_free(&payload);
    topology_context_destroy(&ctx);
    return ok;
}

int main(void)
{
    const TEST_PROCESS one[] = {
        { .pid = 4242, .process = "postgres" },
    };
    const TEST_PROCESS two[] = {
        { .pid = 4242, .process = "postgres" },
        { .pid = 4243, .process = "postgres" },
    };

    bool ok = true;

    // the container actor is named after the process: its identity label is the only container_name
    ok = test_container_name_labels(NV_TOPOLOGY_GROUP_BY_CONTAINER, one, _countof(one), 1, "postgres", "identity") && ok;
    ok = test_container_name_labels(NV_TOPOLOGY_GROUP_BY_CONTAINER, two, _countof(two), 1, "postgres", "identity") && ok;

    // process actors carry the container name as an attribute, once
    ok = test_container_name_labels(NV_TOPOLOGY_GROUP_BY_PROCESS_NAME, two, _countof(two), 1, "postgres", "attribute") && ok;
    ok = test_container_name_labels(NV_TOPOLOGY_GROUP_BY_PID, two, _countof(two), 2, "postgres", "attribute") && ok;

    return ok ? 0 : 1;
}
