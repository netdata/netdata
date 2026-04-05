// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf-ipc.h"

netdata_ebpf_pid_stats_t *integration_shm = NULL;
int shm_fd_ebpf_integration = -1;
sem_t *shm_mutex_ebpf_integration = SEM_FAILED;

static Pvoid_t ebpf_ipc_JudyL = NULL;
ebpf_user_mem_stat_t ebpf_stat_values;

// Judy stores index+1 directly in the pointer slot to avoid heap allocation.
// 0 means "not found" (Judy returns NULL for missing keys),
// so we offset by 1: stored = index+1, retrieved = stored-1.
#define IDX_TO_JVALUE(idx) ((Pvoid_t)((Word_t)(idx) + 1))
#define JVALUE_TO_IDX(pv)  ((uint32_t)((Word_t)(pv) - 1))
#define JVALUE_IS_VALID(pv) ((pv) != NULL)

static bool ebpf_shm_find_index_unsafe(uint32_t pid, uint32_t *result)
{
    Pvoid_t *Pvalue = JudyLGet(ebpf_ipc_JudyL, (Word_t)pid, PJE0);
    if (Pvalue && JVALUE_IS_VALID(*Pvalue)) {
        *result = JVALUE_TO_IDX(*Pvalue);
        return true;
    }
    return false;
}

static bool ebpf_find_pid_shm_del_unsafe(uint32_t pid, enum ebpf_pids_index shm_idx)
{
    uint32_t idx;
    if (!ebpf_shm_find_index_unsafe(pid, &idx))
        return false;

    if (idx >= ebpf_stat_values.current)
        return false;

    netdata_ebpf_pid_stats_t *ptr = &integration_shm[idx];
    if (!ptr->threads)
        return false;

    ptr->threads &= ~(1UL << (shm_idx << 1));
    if (ptr->threads)
        return true;

    (void)JudyLDel(&ebpf_ipc_JudyL, (Word_t)pid, PJE0);
    ebpf_stat_values.current--;

    if (idx == ebpf_stat_values.current)
        return false;

    // Compact: move last entry into the freed slot
    uint32_t last_pid = integration_shm[ebpf_stat_values.current].pid;
    Pvoid_t *Pvalue = JudyLGet(ebpf_ipc_JudyL, (Word_t)last_pid, PJE0);
    if (Pvalue && JVALUE_IS_VALID(*Pvalue)) {
        *Pvalue = IDX_TO_JVALUE(idx);
        memcpy(ptr, &integration_shm[ebpf_stat_values.current], sizeof(*ptr));
    }

    return false;
}

static uint32_t ebpf_find_or_create_index_pid(uint32_t pid)
{
    uint32_t idx;
    if (ebpf_shm_find_index_unsafe(pid, &idx))
        return idx;

    if (ebpf_stat_values.current >= ebpf_stat_values.total)
        return UINT32_MAX;

    Pvoid_t *Pvalue = JudyLIns(&ebpf_ipc_JudyL, (Word_t)pid, PJE0);
    internal_fatal(!Pvalue || Pvalue == PJERR, "EBPF: pid judy index");

    uint32_t new_idx = ebpf_stat_values.current++;
    *Pvalue = IDX_TO_JVALUE(new_idx);

    return new_idx;
}

bool netdata_ebpf_reset_shm_pointer_unsafe(int fd, uint32_t pid, enum ebpf_pids_index idx)
{
    if (idx != NETDATA_EBPF_PIDS_SOCKET_IDX)
        bpf_map_delete_elem(fd, &pid);

    return ebpf_find_pid_shm_del_unsafe(pid, idx);
}

netdata_ebpf_pid_stats_t *netdata_ebpf_get_shm_pointer_unsafe(uint32_t pid, enum ebpf_pids_index idx)
{
    if (!integration_shm || ebpf_stat_values.current >= ebpf_stat_values.total)
        return NULL;

    uint32_t shm_idx = ebpf_find_or_create_index_pid(pid);
    if (shm_idx == UINT32_MAX || shm_idx >= ebpf_stat_values.total)
        return NULL;

    netdata_ebpf_pid_stats_t *ptr = &integration_shm[shm_idx];
    ptr->pid = pid;
    ptr->threads |= (1UL << (idx << 1));

    return ptr;
}

void netdata_integration_cleanup_shm()
{
    if (shm_mutex_ebpf_integration != SEM_FAILED) {
        sem_close(shm_mutex_ebpf_integration);
    }

    if (integration_shm) {
        size_t length = ebpf_stat_values.total * sizeof(netdata_ebpf_pid_stats_t);
        nd_munmap(integration_shm, length);
        integration_shm = NULL;
    }

    // Values are stored inline (no heap allocation), just free the Judy array
    (void)JudyLFreeArray(&ebpf_ipc_JudyL, PJE0);
    ebpf_ipc_JudyL = NULL;

    if (shm_fd_ebpf_integration > 0) {
        close(shm_fd_ebpf_integration);
        shm_fd_ebpf_integration = -1;
    }
}

int netdata_integration_initialize_shm(size_t pids)
{
    if (!pids)
        return -1;

    shm_fd_ebpf_integration = shm_open(NETDATA_EBPF_INTEGRATION_NAME, O_CREAT | O_RDWR, 0660);
    if (shm_fd_ebpf_integration < 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot initialize shared memory. Integration won't happen.");
        return -1;
    }

    ebpf_stat_values.current = 0;
    ebpf_stat_values.total = pids;
    size_t length = pids * sizeof(netdata_ebpf_pid_stats_t);
    if (ftruncate(shm_fd_ebpf_integration, (off_t)length)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot set size for shared memory.");
        goto end_shm;
    }

    integration_shm = nd_mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, shm_fd_ebpf_integration, 0);
    if (integration_shm == MAP_FAILED) {
        nd_log(
            NDLS_COLLECTORS,
            NDLP_ERR,
            "Cannot map shared memory used between cgroup and eBPF, integration won't happen");
        integration_shm = NULL;
        goto end_shm;
    }

    shm_mutex_ebpf_integration = sem_open(
        NETDATA_EBPF_SHM_INTEGRATION_NAME, O_CREAT, S_IRUSR | S_IWUSR | S_IRGRP | S_IWGRP | S_IROTH | S_IWOTH, 1);
    if (shm_mutex_ebpf_integration != SEM_FAILED) {
        return 0;
    }

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot create semaphore, integration between won't happen");

end_shm:
    if (integration_shm) {
        size_t unmap_len = ebpf_stat_values.total * sizeof(netdata_ebpf_pid_stats_t);
        nd_munmap(integration_shm, unmap_len);
        integration_shm = NULL;
    }
    if (shm_fd_ebpf_integration > 0) {
        close(shm_fd_ebpf_integration);
        shm_fd_ebpf_integration = -1;
    }
    return -1;
}

void netdata_integration_current_ipc_data(ebpf_user_mem_stat_t *values)
{
    memcpy(values, &ebpf_stat_values, sizeof(*values));
}
