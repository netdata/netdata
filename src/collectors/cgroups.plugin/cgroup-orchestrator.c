// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"

#ifdef OS_LINUX

static int discovery_proxmox_pve_present_cache = -1;

#ifdef NETDATA_INTERNAL_CHECKS
static int discovery_proxmox_pve_present_for_testing = -1;

void discovery_orchestrator_set_proxmox_pve_present_for_testing(bool present)
{
    discovery_proxmox_pve_present_for_testing = present ? 1 : 0;
    discovery_proxmox_pve_present_cache = discovery_proxmox_pve_present_for_testing;
}
#endif

void discovery_orchestrator_begin_cycle(void)
{
#ifdef NETDATA_INTERNAL_CHECKS
    if (discovery_proxmox_pve_present_for_testing != -1) {
        discovery_proxmox_pve_present_cache = discovery_proxmox_pve_present_for_testing;
        return;
    }
#endif

    discovery_proxmox_pve_present_cache = -1;
}

static bool discovery_proxmox_pve_present(void)
{
    if (discovery_proxmox_pve_present_cache != -1)
        return discovery_proxmox_pve_present_cache == 1;

    char filename[FILENAME_MAX + 1];
    struct stat st;
    snprintfz(filename, FILENAME_MAX, "%s/etc/pve", netdata_configured_host_prefix ? netdata_configured_host_prefix : "");

    discovery_proxmox_pve_present_cache = (stat(filename, &st) == 0 && S_ISDIR(st.st_mode)) ? 1 : 0;
    return discovery_proxmox_pve_present_cache == 1;
}

static bool cgroup_string_ends_with(const char *s, const char *suffix)
{
    size_t s_len = strlen(s);
    size_t suffix_len = strlen(suffix);

    return s_len >= suffix_len && strcmp(&s[s_len - suffix_len], suffix) == 0;
}

static bool cgroup_hex_run_at_least(const char *s, size_t min_len)
{
    size_t len = 0;

    while (isxdigit((uint8_t)*s)) {
        len++;
        s++;
    }

    return len >= min_len;
}

static bool cgroup_digits_follow(const char *s)
{
    if (!isdigit((uint8_t)*s))
        return false;

    while (isdigit((uint8_t)*s))
        s++;

    return true;
}

static char cgroup_normalized_separator(char c)
{
    return c == '/' ? '_' : c;
}

static bool cgroup_normalized_find(const char *id, const char *needle, const char **after)
{
    if (!id || !needle || !*needle)
        return false;

    for (const char *s = id; *s; s++) {
        const char *p = s;
        const char *n = needle;

        while (*p && *n && cgroup_normalized_separator(*p) == *n) {
            p++;
            n++;
        }

        if (!*n) {
            if (after)
                *after = p;
            return true;
        }
    }

    return false;
}

static bool cgroup_normalized_contains(const char *id, const char *needle)
{
    return cgroup_normalized_find(id, needle, NULL);
}

static bool cgroup_normalized_has_hex_after(const char *id, const char *needle, size_t min_len)
{
    const char *p = id;
    const char *after = NULL;

    while (p && *p) {
        if (!cgroup_normalized_find(p, needle, &after))
            return false;

        if (cgroup_hex_run_at_least(after, min_len))
            return true;

        p = after;
    }

    return false;
}

static bool cgroup_has_hex_after(const char *id, const char *needle, size_t min_len)
{
    const char *p = id;

    while ((p = strstr(p, needle))) {
        p += strlen(needle);
        if (cgroup_hex_run_at_least(p, min_len))
            return true;
    }

    return false;
}

static bool cgroup_has_ecs_task_pattern(const char *id)
{
    const char *p = id;

    while ((p = strstr(p, "ecs"))) {
        char delimiter = p[3];
        if ((delimiter == '-' || delimiter == '_' || delimiter == '/' || delimiter == '.') &&
            cgroup_hex_run_at_least(&p[4], 8))
            return true;

        p += 3;
    }

    return false;
}

static bool cgroup_matches_proxmox_lxc_v1(const char *id)
{
    const char *p = id;

    while ((p = strstr(p, "lxc_"))) {
        p += sizeof("lxc_") - 1;
        if (cgroup_digits_follow(p))
            return true;
    }

    return false;
}

static bool cgroup_matches_proxmox_lxc_v2(const char *id)
{
    const char *p = id;

    if (strncmp(p, "/lxc/", sizeof("/lxc/") - 1) != 0)
        return false;

    p += sizeof("/lxc/") - 1;
    if (!isdigit((uint8_t)*p))
        return false;

    while (isdigit((uint8_t)*p))
        p++;

    return *p == '\0' || *p == '/';
}

static bool cgroup_matches_proxmox_qemu(const char *id)
{
    const char *after = NULL;

    if (!cgroup_normalized_find(id, "qemu.slice_", &after) || !isdigit((uint8_t)*after))
        return false;

    while (isdigit((uint8_t)*after))
        after++;

    return strcmp(after, ".scope") == 0;
}

static bool cgroup_matches_qemu_scope(const char *id)
{
    const char *base = strrchr(id, '/');
    base = base ? base + 1 : id;

    return strncmp(base, "qemu-", sizeof("qemu-") - 1) == 0 && cgroup_string_ends_with(base, ".scope");
}

static bool cgroup_matches_machine_slice_service(const char *id)
{
    const char prefix[] = "/machine.slice/";
    const char *name;

    if (strncmp(id, prefix, sizeof(prefix) - 1) != 0)
        return false;

    name = &id[sizeof(prefix) - 1];
    return *name && !strchr(name, '/') && cgroup_string_ends_with(name, ".service");
}

static bool cgroup_matches_systemd_service_path(struct cgroup *cg)
{
    if (systemd_services_cgroups)
        return matches_systemd_services_cgroups(cg->id);

    const char prefix[] = "/system.slice/";
    const char *name;

    if (strncmp(cg->id, prefix, sizeof(prefix) - 1) != 0)
        return false;

    name = &cg->id[sizeof(prefix) - 1];
    if (!*name || !cgroup_string_ends_with(name, ".service"))
        return false;

    for (const char *slash = strchr(name, '/'); slash; slash = strchr(slash + 1, '/')) {
        const char *service_suffix = ".service";
        size_t suffix_len = strlen(service_suffix);

        if ((size_t)(slash - name) >= suffix_len && strncmp(slash - suffix_len, service_suffix, suffix_len) == 0)
            return false;
    }

    return true;
}

static enum cgroups_container_orchestrator discovery_detect_orchestrator(const char *id)
{
    bool proxmox_pve_present;

    if (strstr(id, "kubepods"))
        return CGROUPS_ORCHESTRATOR_K8S;

    proxmox_pve_present = discovery_proxmox_pve_present();

    if (strstr(id, "lxc.payload.") || strstr(id, "lxc.monitor.") ||
        (cgroup_normalized_contains(id, "machine.slice_machine") && strstr(id, "-lxc")) ||
        (proxmox_pve_present && (cgroup_matches_proxmox_lxc_v1(id) || cgroup_matches_proxmox_lxc_v2(id))))
        return CGROUPS_ORCHESTRATOR_LXC;

    if ((cgroup_normalized_contains(id, "machine.slice_machine") && strstr(id, "-qemu")) ||
        strstr(id, ".libvirt-qemu") ||
        (proxmox_pve_present && cgroup_matches_proxmox_qemu(id)) ||
        cgroup_matches_qemu_scope(id) ||
        strstr(id, "/qemu-"))
        return CGROUPS_ORCHESTRATOR_KVM;

    if (strstr(id, "/docker/") ||
        cgroup_has_hex_after(id, "docker-", 8) ||
        cgroup_normalized_has_hex_after(id, "system.slice_containerd.service_cpuset_", 8) ||
        cgroup_normalized_has_hex_after(id, "system.slice_containerd.service_cpuset-", 8) ||
        cgroup_normalized_has_hex_after(id, "system.slice_containerd.service_docker-", 8) ||
        cgroup_has_ecs_task_pattern(id))
        return CGROUPS_ORCHESTRATOR_DOCKER;

    if (strstr(id, "/podman/") ||
        cgroup_has_hex_after(id, "podman-", 8) ||
        cgroup_has_hex_after(id, "libpod-", 8) ||
        cgroup_has_hex_after(id, "libpod-conmon-", 8))
        return CGROUPS_ORCHESTRATOR_PODMAN;

    if (cgroup_matches_machine_slice_service(id))
        return CGROUPS_ORCHESTRATOR_NSPAWN;

    return CGROUPS_ORCHESTRATOR_UNKNOWN;
}

void discovery_classify_orchestrator(struct cgroup *cg)
{
    if (!cg || cg->container_orchestrator_resolved)
        return;

    if (!cg->id) {
        cg->container_orchestrator = CGROUPS_ORCHESTRATOR_UNKNOWN;
        cg->container_orchestrator_resolved = true;
        return;
    }

    cg->container_orchestrator = discovery_detect_orchestrator(cg->id);

    if (cg->container_orchestrator == CGROUPS_ORCHESTRATOR_UNKNOWN && cgroup_matches_systemd_service_path(cg))
        cg->container_orchestrator = CGROUPS_ORCHESTRATOR_SYSTEMD;

    cg->container_orchestrator_resolved = true;
}

#endif // OS_LINUX
