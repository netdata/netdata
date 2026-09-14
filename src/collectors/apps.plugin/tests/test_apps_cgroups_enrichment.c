// SPDX-License-Identifier: GPL-3.0-or-later

#include "../apps-cgroups-enrichment.h"

bool apps_cgroups_lookup_is_host_root_path(const char *path)
{
    return path && strcmp(path, "/") == 0;
}

#include "../apps-cgroups-enrichment.c"

static bool expect_ok(bool condition, const char *message)
{
    if(condition)
        return true;

    fprintf(stderr, "%s\n", message);
    return false;
}

static bool expect_str_eq(const char *actual, const char *expected, const char *message)
{
    if((actual == expected) || (actual && expected && strcmp(actual, expected) == 0))
        return true;

    fprintf(stderr, "%s: expected '%s', got '%s'\n",
            message,
            expected ? expected : "(null)",
            actual ? actual : "(null)");
    return false;
}

static void pid_fixture_clear(struct pid_stat *p)
{
    if(!p)
        return;

    string_freez(p->comm);
    p->comm = NULL;
    string_freez(p->cgroup_path);
    p->cgroup_path = NULL;
}

static void label_clear(struct cgroup_lookup_label *labels, size_t count)
{
    for(size_t i = 0; i < count; i++) {
        string_freez(labels[i].key);
        labels[i].key = NULL;
        string_freez(labels[i].value);
        labels[i].value = NULL;
    }
}

static bool test_process_without_cgroup_path(void)
{
    struct pid_stat p = {
        .pid = 1001,
        .uid = 1001,
        .comm = string_strdupz("opencode"),
    };
    APPS_PROCESS_ENRICHMENT out;

    apps_process_enrichment_fill(&p, &out);

    bool ok =
        expect_str_eq(out.container_name, "opencode", "no cgroup path container fallback mismatch") &&
        expect_str_eq(out.actor_type, "process_group", "no cgroup path actor type mismatch") &&
        expect_str_eq(out.actor_kind, "process", "no cgroup path actor kind mismatch") &&
        expect_str_eq(out.cgroup_status, "unknown", "no cgroup path cgroup status mismatch");

    pid_fixture_clear(&p);
    return ok;
}

static bool test_systemd_user_scope_without_cache(void)
{
    struct pid_stat p = {
        .pid = 1002,
        .uid = 1001,
        .comm = string_strdupz("opencode"),
        .cgroup_path = string_strdupz("/user.slice/user-1001.slice/user@1001.service/app.slice/opencode.scope"),
    };
    APPS_PROCESS_ENRICHMENT out;

    apps_process_enrichment_fill(&p, &out);

    bool ok =
        expect_str_eq(out.container_name, "opencode.scope", "systemd scope container name mismatch") &&
        expect_str_eq(out.actor_type, "systemd_scope", "systemd scope actor type mismatch") &&
        expect_str_eq(out.actor_kind, "scope", "systemd scope actor kind mismatch") &&
        expect_str_eq(out.cgroup_status, "retry_later", "systemd scope cgroup status mismatch");

    pid_fixture_clear(&p);
    return ok;
}

static bool test_pending_container_path_without_cache(void)
{
    struct pid_stat p = {
        .pid = 1003,
        .uid = 1001,
        .comm = string_strdupz("worker"),
        .cgroup_path = string_strdupz("/kubepods/burstable/podabc/container123"),
    };
    APPS_PROCESS_ENRICHMENT out;

    apps_process_enrichment_fill(&p, &out);

    bool ok =
        expect_str_eq(out.container_name, "[pending]", "pending container name mismatch") &&
        expect_str_eq(out.actor_type, "container", "pending container actor type mismatch") &&
        expect_str_eq(out.actor_kind, "pending", "pending container actor kind mismatch") &&
        expect_str_eq(out.cgroup_status, "retry_later", "pending cgroup status mismatch");

    pid_fixture_clear(&p);
    return ok;
}

static bool test_namespace_relative_container_scope_without_cache(void)
{
    struct pid_stat p = {
        .pid = 1008,
        .uid = 1001,
        .comm = string_strdupz("worker"),
        .cgroup_path = string_strdupz(
            "/../../kubepods.slice/kubepods-besteffort.slice/podabc/"
            "cri-containerd-0123456789abcdef.scope"),
    };
    APPS_PROCESS_ENRICHMENT out;

    apps_process_enrichment_fill(&p, &out);

    bool ok =
        expect_str_eq(out.container_name, "[pending]", "namespace-relative pending container name mismatch") &&
        expect_str_eq(out.actor_type, "container", "namespace-relative pending actor type mismatch") &&
        expect_str_eq(out.actor_kind, "pending", "namespace-relative pending actor kind mismatch") &&
        expect_str_eq(out.cgroup_status, "retry_later", "namespace-relative pending cgroup status mismatch");

    pid_fixture_clear(&p);
    return ok;
}

static bool test_known_k8s_labels(void)
{
    struct cgroup_lookup_label labels[5] = {
        { .key = string_strdupz("k8s_pod_name"), .value = string_strdupz("api-7d4f9c84d9-abc12") },
        { .key = string_strdupz("k8s_namespace"), .value = string_strdupz("production") },
        { .key = string_strdupz("k8s_container_name"), .value = string_strdupz("api") },
        { .key = string_strdupz("k8s_controller_name"), .value = string_strdupz("api-deployment") },
        { .key = string_strdupz("image"), .value = string_strdupz("registry.example/api:v1") },
    };
    struct cgroup_lookup_entry entry = {
        .cgroup_status = NIPC_CGROUP_LOOKUP_KNOWN,
        .orchestrator = NIPC_ORCHESTRATOR_K8S,
        .cgroup_name = string_strdupz("cri-container-id"),
        .cgroup_labels = labels,
        .cgroup_label_count = _countof(labels),
    };
    struct pid_stat p = {
        .pid = 1004,
        .uid = 1001,
        .comm = string_strdupz("api"),
        .cgroup_path = string_strdupz("/kubepods.slice/podabc/cri-container-id"),
        .cgroup_cache = &entry,
    };
    APPS_PROCESS_ENRICHMENT out;

    apps_process_enrichment_fill(&p, &out);

    bool ok =
        expect_str_eq(out.container_name, "api", "known k8s container name mismatch") &&
        expect_str_eq(out.actor_type, "k8s_container", "known k8s actor type mismatch") &&
        expect_str_eq(out.actor_kind, "k8s_container", "known k8s actor kind mismatch") &&
        expect_str_eq(out.orchestrator, "k8s", "known k8s orchestrator mismatch") &&
        expect_str_eq(out.k8s_pod_name, "api-7d4f9c84d9-abc12", "known k8s pod name mismatch") &&
        expect_str_eq(out.k8s_namespace, "production", "known k8s namespace mismatch") &&
        expect_str_eq(out.k8s_workload, "api-deployment", "known k8s workload mismatch") &&
        expect_str_eq(out.docker_image, "registry.example/api:v1", "known k8s image mismatch");

    pid_fixture_clear(&p);
    string_freez(entry.cgroup_name);
    label_clear(labels, _countof(labels));
    return ok;
}

int main(void)
{
    bool ok =
        test_process_without_cgroup_path() &&
        test_systemd_user_scope_without_cache() &&
        test_pending_container_path_without_cache() &&
        test_namespace_relative_container_scope_without_cache() &&
        test_known_k8s_labels();

    return ok ? 0 : 1;
}
