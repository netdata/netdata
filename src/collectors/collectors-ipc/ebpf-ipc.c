// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf-ipc.h"

netdata_ebpf_pid_stats_t *integration_shm;
int shm_fd_ebpf_integration = -1;
sem_t *shm_mutex_ebpf_integration = SEM_FAILED;

void netdata_integration_cleanup_shm()
{
    if (shm_mutex_ebpf_integration != SEM_FAILED) {
        sem_close(shm_mutex_ebpf_integration);
    }

    if (integration_shm) {
        size_t length = os_get_system_pid_max() * sizeof(netdata_ebpf_pid_stats_t);
        munmap(integration_shm, length);
    }

    if (shm_fd_ebpf_integration > 0) {
        close(shm_fd_ebpf_integration);
    }
}

int netdata_integration_initialize_shm(size_t pids)
{
    shm_fd_ebpf_integration = shm_open(NETDATA_EBPF_INTEGRATION_NAME, O_CREAT | O_RDWR, 0660);
    if (shm_fd_ebpf_integration < 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot initialize shared memory. Integration won't happen.");
        return -1;
    }

    size_t length = pids * sizeof(netdata_ebpf_pid_stats_t);
    if (ftruncate(shm_fd_ebpf_integration, (off_t)length)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot set size for shared memory.");
        goto end_shm;
    }

    integration_shm = mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, shm_fd_ebpf_integration, 0);
    if (unlikely(MAP_FAILED == integration_shm)) {
        integration_shm = NULL;
        nd_log(
            NDLS_COLLECTORS,
            NDLP_ERR,
            "Cannot map shared memory used between cgroup and eBPF, integration won't happen");
        goto end_shm;
    }

    shm_mutex_ebpf_integration = sem_open(
        NETDATA_EBPF_SHM_INTEGRATION_NAME, O_CREAT, S_IRUSR | S_IWUSR | S_IRGRP | S_IWGRP | S_IROTH | S_IWOTH, 1);
    if (shm_mutex_ebpf_integration != SEM_FAILED) {
        return 0;
    }

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot create semaphore, integration between won't happen");
    munmap(integration_shm, length);
    integration_shm = NULL;

end_shm:
    return -1;
}
