// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/netipc/netipc_netdata.h"

#if defined(OS_LINUX) && defined(ENABLE_CGROUPS_LOOKUP_SERVER)

static const char *lookup_status_name(uint16_t status)
{
    switch (status) {
        case NIPC_CGROUP_LOOKUP_KNOWN:
            return "KNOWN";
        case NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER:
            return "UNKNOWN_RETRY_LATER";
        case NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT:
            return "UNKNOWN_PERMANENT";
        default:
            return "INVALID";
    }
}

static const char *orchestrator_name(uint16_t orchestrator)
{
    switch (orchestrator) {
        case NIPC_ORCHESTRATOR_UNKNOWN:
            return "UNKNOWN";
        case NIPC_ORCHESTRATOR_SYSTEMD:
            return "SYSTEMD";
        case NIPC_ORCHESTRATOR_DOCKER:
            return "DOCKER";
        case NIPC_ORCHESTRATOR_K8S:
            return "K8S";
        case NIPC_ORCHESTRATOR_KVM:
            return "KVM";
        case NIPC_ORCHESTRATOR_LXC:
            return "LXC";
        case NIPC_ORCHESTRATOR_PODMAN:
            return "PODMAN";
        case NIPC_ORCHESTRATOR_NSPAWN:
            return "NSPAWN";
        default:
            return "INVALID";
    }
}

static char *read_line_path(void)
{
    char *line = NULL;
    size_t size = 0;
    ssize_t len = getline(&line, &size, stdin);

    if (len < 0) {
        freez(line);
        return NULL;
    }

    while (len > 0 && (line[len - 1] == '\n' || line[len - 1] == '\r'))
        line[--len] = '\0';

    if (len == 0) {
        freez(line);
        return strdupz("");
    }

    return line;
}

int main(int argc, char **argv)
{
    if(nd_environment_init() != 0) {
        fprintf(stderr, "Cannot initialize the process environment: %s\n", strerror(errno));
        return 1;
    }

    const char *run_dir = argc > 1 ? argv[1] : os_run_dir(true);
    if(!run_dir) {
        fprintf(stderr, "Cannot resolve the runtime directory.\n");
        return 1;
    }

    if(nd_environment_freeze_process() != 0) {
        fprintf(stderr, "Cannot freeze the process environment: %s\n", strerror(errno));
        return 1;
    }

    const char *service_name = argc > 2 ? argv[2] : "cgroups-lookup";
    char **owned_paths = NULL;
    nipc_str_view_t *paths = NULL;
    uint32_t path_count = 0;
    uint32_t path_capacity = 0;

    while (true) {
        char *path = read_line_path();
        if (!path)
            break;

        if (!*path) {
            freez(path);
            continue;
        }

        if (path_count == path_capacity) {
            uint32_t new_capacity = path_capacity ? path_capacity * 2 : 16;
            owned_paths = reallocz(owned_paths, (size_t)new_capacity * sizeof(*owned_paths));
            paths = reallocz(paths, (size_t)new_capacity * sizeof(*paths));
            path_capacity = new_capacity;
        }

        owned_paths[path_count] = path;
        paths[path_count].ptr = path;
        paths[path_count].len = (uint32_t)strlen(path);
        path_count++;
    }

    if (!path_count) {
        fprintf(stderr, "usage: printf '/cgroup/path\\n' | %s [run_dir] [service_name]\n", argv[0]);
        freez(owned_paths);
        freez(paths);
        return 2;
    }

    nipc_client_config_t config = {
        .supported_profiles = NIPC_PROFILE_BASELINE | NIPC_PROFILE_SHM_HYBRID | NIPC_PROFILE_SHM_FUTEX,
        .preferred_profiles = NIPC_PROFILE_SHM_FUTEX,
        .auth_token = netipc_auth_token(),
    };

    nipc_client_ctx_t client;
    nipc_client_init(&client, run_dir, service_name, &config);
    (void)nipc_client_refresh(&client);

    nipc_cgroups_lookup_resp_view_t response;
    nipc_error_t err = nipc_client_call_cgroups_lookup(&client, paths, path_count, &response);
    if (err != NIPC_OK) {
        fprintf(stderr, "CGROUPS_LOOKUP failed: %u\n", (unsigned int)err);
        nipc_client_close(&client);
        for (uint32_t i = 0; i < path_count; i++)
            freez(owned_paths[i]);
        freez(owned_paths);
        freez(paths);
        return 1;
    }

    printf("generation=%llu items=%u\n", (unsigned long long)response.generation, response.item_count);
    for (uint32_t i = 0; i < response.item_count; i++) {
        nipc_cgroups_lookup_item_view_t item;
        err = nipc_cgroups_lookup_resp_item(&response, i, &item);
        if (err != NIPC_OK) {
            fprintf(stderr, "decode item %u failed: %u\n", i, (unsigned int)err);
            break;
        }

        printf(
            "%.*s\t%s\t%s\t%.*s\tlabels=%u",
            (int)item.path.len,
            item.path.ptr,
            lookup_status_name(item.status),
            orchestrator_name(item.orchestrator),
            (int)item.name.len,
            item.name.ptr,
            item.label_count);

        for (uint16_t j = 0; j < item.label_count; j++) {
            nipc_lookup_label_view_t label;
            err = nipc_cgroups_lookup_item_label(&item, j, &label);
            if (err != NIPC_OK) {
                fprintf(stderr, "decode label %u for item %u failed: %u\n", j, i, (unsigned int)err);
                break;
            }

            printf("\t%.*s=%.*s", (int)label.key.len, label.key.ptr, (int)label.value.len, label.value.ptr);
        }

        putchar('\n');
    }

    nipc_client_close(&client);
    for (uint32_t i = 0; i < path_count; i++)
        freez(owned_paths[i]);
    freez(owned_paths);
    freez(paths);

    return err == NIPC_OK ? 0 : 1;
}

#else

int main(void)
{
    fprintf(stderr, "cgroup-lookup-test is only available on Linux builds with ENABLE_CGROUPS_LOOKUP_SERVER\n");
    return 2;
}

#endif
