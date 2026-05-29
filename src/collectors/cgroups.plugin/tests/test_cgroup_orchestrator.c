// SPDX-License-Identifier: GPL-3.0-or-later

#include "../cgroup-internals.h"

SIMPLE_PATTERN *systemd_services_cgroups = NULL;

struct orchestrator_case {
    const char *name;
    const char *path;
    enum cgroups_container_orchestrator expected;
    bool proxmox_pve_present;
};

static const char *orchestrator_name(enum cgroups_container_orchestrator orchestrator)
{
    switch (orchestrator) {
        case CGROUPS_ORCHESTRATOR_UNKNOWN:
            return "UNKNOWN";
        case CGROUPS_ORCHESTRATOR_SYSTEMD:
            return "SYSTEMD";
        case CGROUPS_ORCHESTRATOR_DOCKER:
            return "DOCKER";
        case CGROUPS_ORCHESTRATOR_K8S:
            return "K8S";
        case CGROUPS_ORCHESTRATOR_KVM:
            return "KVM";
        case CGROUPS_ORCHESTRATOR_LXC:
            return "LXC";
        case CGROUPS_ORCHESTRATOR_PODMAN:
            return "PODMAN";
        case CGROUPS_ORCHESTRATOR_NSPAWN:
            return "NSPAWN";
        default:
            return "INVALID";
    }
}

static bool run_case(const struct orchestrator_case *tc)
{
    struct cgroup cg = {
        .id = (char *)tc->path,
        .container_orchestrator = CGROUPS_ORCHESTRATOR_UNKNOWN,
        .container_orchestrator_resolved = false,
    };

    discovery_orchestrator_set_proxmox_pve_present_for_testing(tc->proxmox_pve_present);
    discovery_classify_orchestrator(&cg);

    if (!cg.container_orchestrator_resolved) {
        fprintf(stderr, "%s: orchestrator was not marked resolved\n", tc->name);
        return false;
    }

    if (cg.container_orchestrator != tc->expected) {
        fprintf(
            stderr,
            "%s: expected %s, got %s for path %s\n",
            tc->name,
            orchestrator_name(tc->expected),
            orchestrator_name(cg.container_orchestrator),
            tc->path);
        return false;
    }

    return true;
}

int main(void)
{
    netdata_configured_host_prefix = "";

    static const struct orchestrator_case cases[] = {
        {
            .name = "k8s kubepods",
            .path = "/kubepods.slice/kubepods-burstable.slice/pod123/docker-0123456789abcdef.scope",
            .expected = CGROUPS_ORCHESTRATOR_K8S,
        },
        {
            .name = "docker slash path",
            .path = "/docker/0123456789abcdef",
            .expected = CGROUPS_ORCHESTRATOR_DOCKER,
        },
        {
            .name = "docker systemd scope",
            .path = "/system.slice/docker-0123456789abcdef.scope",
            .expected = CGROUPS_ORCHESTRATOR_DOCKER,
        },
        {
            .name = "containerd docker scope",
            .path = "/system.slice/containerd.service/docker-0123456789abcdef.scope",
            .expected = CGROUPS_ORCHESTRATOR_DOCKER,
        },
        {
            .name = "aws ecs task",
            .path = "/ecs/0123456789abcdef",
            .expected = CGROUPS_ORCHESTRATOR_DOCKER,
        },
        {
            .name = "podman scope",
            .path = "/machine.slice/libpod-0123456789abcdef.scope",
            .expected = CGROUPS_ORCHESTRATOR_PODMAN,
        },
        {
            .name = "lxc payload",
            .path = "/lxc.payload.demo",
            .expected = CGROUPS_ORCHESTRATOR_LXC,
        },
        {
            .name = "libvirt lxc",
            .path = "/machine.slice/machine-lxc\\x2ddemo.scope",
            .expected = CGROUPS_ORCHESTRATOR_LXC,
        },
        {
            .name = "proxmox lxc v1",
            .path = "/lxc_100",
            .expected = CGROUPS_ORCHESTRATOR_LXC,
            .proxmox_pve_present = true,
        },
        {
            .name = "proxmox lxc v2",
            .path = "/lxc/100",
            .expected = CGROUPS_ORCHESTRATOR_LXC,
            .proxmox_pve_present = true,
        },
        {
            .name = "libvirt kvm",
            .path = "/machine.slice/machine-qemu\\x2dvm.scope",
            .expected = CGROUPS_ORCHESTRATOR_KVM,
        },
        {
            .name = "older libvirt kvm",
            .path = "/machine_100.libvirt-qemu",
            .expected = CGROUPS_ORCHESTRATOR_KVM,
        },
        {
            .name = "qemu scope",
            .path = "/qemu-100.scope",
            .expected = CGROUPS_ORCHESTRATOR_KVM,
        },
        {
            .name = "proxmox qemu",
            .path = "/qemu.slice_100.scope",
            .expected = CGROUPS_ORCHESTRATOR_KVM,
            .proxmox_pve_present = true,
        },
        {
            .name = "proxmox qemu without pve directory",
            .path = "/qemu.slice_100.scope",
            .expected = CGROUPS_ORCHESTRATOR_UNKNOWN,
        },
        {
            .name = "systemd nspawn",
            .path = "/machine.slice/machine-demo.service",
            .expected = CGROUPS_ORCHESTRATOR_NSPAWN,
        },
        {
            .name = "systemd service",
            .path = "/system.slice/ssh.service",
            .expected = CGROUPS_ORCHESTRATOR_SYSTEMD,
        },
        {
            .name = "systemd service under slice",
            .path = "/system.slice/system-cups.slice/cups.service",
            .expected = CGROUPS_ORCHESTRATOR_SYSTEMD,
        },
        {
            .name = "systemd service under service remains unknown",
            .path = "/system.slice/foo.service/bar.service",
            .expected = CGROUPS_ORCHESTRATOR_UNKNOWN,
        },
        {
            .name = "systemd slice remains unknown",
            .path = "/system.slice/system-cups.slice",
            .expected = CGROUPS_ORCHESTRATOR_UNKNOWN,
        },
        {
            .name = "standalone crio remains unknown",
            .path = "/system.slice/crio-0123456789abcdef.scope",
            .expected = CGROUPS_ORCHESTRATOR_UNKNOWN,
        },
        {
            .name = "unknown user slice",
            .path = "/user.slice/user-1000.slice/session-3.scope",
            .expected = CGROUPS_ORCHESTRATOR_UNKNOWN,
        },
    };

    size_t failures = 0;
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        if (!run_case(&cases[i]))
            failures++;
    }

    systemd_services_cgroups = simple_pattern_create(
        " !/system.slice/*.service/*.service "
        " /system.slice/*.service ",
        NULL,
        SIMPLE_PATTERN_EXACT,
        true);

    static const struct orchestrator_case configured_systemd_cases[] = {
        {
            .name = "configured systemd service",
            .path = "/system.slice/ssh.service",
            .expected = CGROUPS_ORCHESTRATOR_SYSTEMD,
        },
        {
            .name = "configured systemd service under slice",
            .path = "/system.slice/system-cups.slice/cups.service",
            .expected = CGROUPS_ORCHESTRATOR_SYSTEMD,
        },
        {
            .name = "configured systemd service under service remains unknown",
            .path = "/system.slice/foo.service/bar.service",
            .expected = CGROUPS_ORCHESTRATOR_UNKNOWN,
        },
        {
            .name = "configured systemd slice remains unknown",
            .path = "/system.slice/system-cups.slice",
            .expected = CGROUPS_ORCHESTRATOR_UNKNOWN,
        },
    };

    for (size_t i = 0; i < sizeof(configured_systemd_cases) / sizeof(configured_systemd_cases[0]); i++) {
        if (!run_case(&configured_systemd_cases[i]))
            failures++;
    }

    simple_pattern_free(systemd_services_cgroups);

    if (failures) {
        fprintf(stderr, "%zu cgroup orchestrator classifier tests failed\n", failures);
        return 1;
    }

    return 0;
}
