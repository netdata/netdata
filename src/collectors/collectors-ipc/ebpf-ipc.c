// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf-ipc.h"

netdata_ebpf_pid_stats_t *integration_shm = NULL;
int shm_fd_ebpf_integration = -1;
sem_t *shm_mutex_ebpf_integration = SEM_FAILED;

static Pvoid_t ebpf_ipc_JudyL = NULL;
static uint32_t last_idx = 0;
static uint32_t max_idx = 0;

bool using_vector = false;

static uint32_t *ebpf_shm_find_index_unsafe(uint32_t pid)
{
    Pvoid_t *Pvalue = JudyLGet(ebpf_ipc_JudyL, (Word_t)pid, PJE0);
    return (uint32_t *)*Pvalue;
}

static bool ebpf_find_pid_shm_del_unsafe(uint32_t pid, enum ebpf_pids_index idx)
{
    uint32_t *lpid = ebpf_shm_find_index_unsafe(pid);
    if (!lpid)
        return false;

    netdata_ebpf_pid_stats_t *ptr = &integration_shm[*lpid];
    ptr->threads &= ~(idx << 1);
    if (ptr->threads) {
        return true;
    }

    (void)JudyLDel(&ebpf_ipc_JudyL, (Word_t)pid, PJE0);

    last_idx--;
    if (!last_idx)
        return false;

    netdata_ebpf_pid_stats_t *newValue = &integration_shm[last_idx];
    uint32_t *move = ebpf_shm_find_index_unsafe(newValue->pid);
    *move = *lpid;

    memcpy(ptr, newValue, sizeof(*ptr));
    return false;
}

static uint32_t *ebpf_find_or_create_index_pid(uint32_t pid)
{
    uint32_t *idx = ebpf_shm_find_index_unsafe(pid);
    if (!idx) {
        Pvoid_t *Pvalue = JudyLIns(&ebpf_ipc_JudyL, (Word_t)pid, PJE0);
        internal_fatal(!Pvalue || Pvalue == PJERR, "EBPF: pid judy index");
        if (likely(!*Pvalue))
            *Pvalue = idx = callocz(1, sizeof(*idx));
        else
            idx = *Pvalue;
    }
    return idx;
}

bool netdata_ebpf_reset_shm_pointer_unsafe(int fd, uint32_t pid, enum ebpf_pids_index idx)
{
    if (idx != NETDATA_EBPF_PIDS_SOCKET_IDX)
        bpf_map_delete_elem(fd, &pid);

    if (using_vector && integration_shm) {
        netdata_ebpf_pid_stats_t *ptr = &integration_shm[pid];
        ptr->threads &= ~(idx << 1);
        if (!ptr->threads) {
            memset(ptr, 0, sizeof(*ptr));
            return false;
        }
    } else {
        return ebpf_find_pid_shm_del_unsafe(pid, idx);
    }

    return true;
}

netdata_ebpf_pid_stats_t *netdata_ebpf_get_shm_pointer_unsafe(uint32_t pid, enum ebpf_pids_index idx)
{
    if (!integration_shm || (last_idx + 1) == max_idx)
        return NULL;

    if (!using_vector) {
        uint32_t *pidx = ebpf_find_or_create_index_pid(pid);
        if (*pidx == 0) {
            *pidx = last_idx++;
        }
        pid = *pidx;
    }
    netdata_ebpf_pid_stats_t *ptr = &integration_shm[pid];
    ptr->pid = pid;
    ptr->threads |= idx << 1;

    return ptr;
}

void netdata_integration_cleanup_shm()
{
    if (shm_mutex_ebpf_integration != SEM_FAILED) {
        sem_close(shm_mutex_ebpf_integration);
    }

    if (integration_shm) {
        size_t length = last_idx * sizeof(netdata_ebpf_pid_stats_t);
        munmap(integration_shm, length);
    }

    if (shm_fd_ebpf_integration > 0) {
        close(shm_fd_ebpf_integration);
    }
}

static void netdata_ebpf_select_access_mode(size_t pids)
{
    size_t local_max = (size_t)os_get_system_pid_max();
    using_vector = (pids == local_max);
}

int netdata_integration_initialize_shm(size_t pids)
{
    if (!pids)
        return -1;

    netdata_ebpf_select_access_mode(pids);

    shm_fd_ebpf_integration = shm_open(NETDATA_EBPF_INTEGRATION_NAME, O_CREAT | O_RDWR, 0660);
    if (shm_fd_ebpf_integration < 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot initialize shared memory. Integration won't happen.");
        return -1;
    }

    last_idx = pids;
    size_t length = pids * sizeof(netdata_ebpf_pid_stats_t);
    if (ftruncate(shm_fd_ebpf_integration, (off_t)length)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot set size for shared memory.");
        goto end_shm;
    }

    integration_shm = nd_mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, shm_fd_ebpf_integration, 0);
    if (!integration_shm) {
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
