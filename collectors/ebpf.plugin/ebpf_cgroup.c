// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_cgroup.h"

ebpf_cgroup_target_t *ebpf_cgroup_pids = NULL;

// --------------------------------------------------------------------------------------------------------------------
// Map shared memory

/**
 * Map Shared Memory locally
 *
 * Map the shared memory for current process
 *
 * @param fd       file descriptor returned after shm_open was called.
 * @param length   length of the shared memory
 *
 * @return It returns a pointer to the region mapped.
 */
static inline void *ebpf_cgroup_map_shm_locally(int fd, size_t length)
{
    void *value;

    value =  mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
    if (!value) {
        error("Cannot map shared memory used between eBPF and cgroup, integration between processes won't happen");
        close(shm_fd_ebpf_cgroup);
        shm_fd_ebpf_cgroup = -1;
        shm_unlink(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME);
    }

    return value;
}

/**
 * Map cgroup shared memory
 *
 * Map cgroup shared memory from cgroup to plugin
 */
void ebpf_map_cgroup_shared_memory()
{
    static int limit_try = 0;
    static time_t next_try = 0;

    if (shm_ebpf_cgroup.header || limit_try > NETDATA_EBPF_CGROUP_MAX_TRIES)
        return;

    time_t curr_time = time(NULL);
    if (curr_time < next_try)
        return;

    limit_try++;
    next_try = curr_time + NETDATA_EBPF_CGROUP_NEXT_TRY_SEC;

    shm_fd_ebpf_cgroup = shm_open(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME, O_RDWR, 0660);
    if (shm_fd_ebpf_cgroup < 0) {
        if (limit_try == NETDATA_EBPF_CGROUP_MAX_TRIES)
            error("Shared memory was not initialized, integration between processes won't happen.");

        return;
    }

    // Map only header
    shm_ebpf_cgroup.header = (netdata_ebpf_cgroup_shm_header_t *) ebpf_cgroup_map_shm_locally(shm_fd_ebpf_cgroup,
                                                                                             sizeof(netdata_ebpf_cgroup_shm_header_t));
    if (!shm_ebpf_cgroup.header) {
        limit_try = NETDATA_EBPF_CGROUP_MAX_TRIES + 1;
        return;
    }

    size_t length =  shm_ebpf_cgroup.header->body_length;

    munmap(shm_ebpf_cgroup.header, sizeof(netdata_ebpf_cgroup_shm_header_t));

    shm_ebpf_cgroup.header = (netdata_ebpf_cgroup_shm_header_t *)ebpf_cgroup_map_shm_locally(shm_fd_ebpf_cgroup, length);
    if (!shm_ebpf_cgroup.header) {
        limit_try = NETDATA_EBPF_CGROUP_MAX_TRIES + 1;
        return;
    }
    shm_ebpf_cgroup.body = (netdata_ebpf_cgroup_shm_body_t *) ((char *)shm_ebpf_cgroup.header +
                                                              sizeof(netdata_ebpf_cgroup_shm_header_t));

    shm_sem_ebpf_cgroup = sem_open(NETDATA_NAMED_SEMAPHORE_EBPF_CGROUP_NAME, O_CREAT, 0660, 1);

    if (shm_sem_ebpf_cgroup == SEM_FAILED) {
        error("Cannot create semaphore, integration between eBPF and cgroup won't happen");
        munmap(shm_ebpf_cgroup.header, length);
        shm_ebpf_cgroup.header = NULL;
        close(shm_fd_ebpf_cgroup);
        shm_fd_ebpf_cgroup = -1;
        shm_unlink(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME);
    }
}

// --------------------------------------------------------------------------------------------------------------------
// Close and Cleanup

/**
 * Close shared memory
 */
void ebpf_close_cgroup_shm()
{
    if (shm_sem_ebpf_cgroup != SEM_FAILED) {
        sem_close(shm_sem_ebpf_cgroup);
        sem_unlink(NETDATA_NAMED_SEMAPHORE_EBPF_CGROUP_NAME);
        shm_sem_ebpf_cgroup = SEM_FAILED;
    }

    if (shm_fd_ebpf_cgroup > 0) {
        close(shm_fd_ebpf_cgroup);
        shm_unlink(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME);
        shm_fd_ebpf_cgroup = -1;
    }
}

/**
 * Clean Specific cgroup pid
 *
 * Clean all PIDs associated with cgroup.
 *
 * @param pt structure pid on target that will have your PRs removed
 */
static inline void ebpf_clean_specific_cgroup_pids(struct pid_on_target2 *pt)
{
    while (pt) {
        struct pid_on_target2 *next_pid = pt->next;

        freez(pt);
        pt = next_pid;
    }
}

/**
 * Cleanup link list
 */
void ebpf_clean_cgroup_pids()
{
    if (!ebpf_cgroup_pids)
        return;

    ebpf_cgroup_target_t *ect = ebpf_cgroup_pids;
    while (ect) {
        ebpf_cgroup_target_t *next_cgroup = ect->next;

        ebpf_clean_specific_cgroup_pids(ect->pids);
        freez(ect);

        ect = next_cgroup;
    }
    ebpf_cgroup_pids = NULL;
}

/**
 * Remove Cgroup Update Target Update List
 *
 * Remove from cgroup target and update the link list
 */
static void ebpf_remove_cgroup_target_update_list()
{
    ebpf_cgroup_target_t *next, *ect = ebpf_cgroup_pids;
    ebpf_cgroup_target_t *prev = ebpf_cgroup_pids;
    while (ect) {
        next = ect->next;
        if (!ect->updated) {
            if (ect == ebpf_cgroup_pids) {
                ebpf_cgroup_pids = next;
                prev = next;
            } else {
                prev->next = next;
            }

            ebpf_clean_specific_cgroup_pids(ect->pids);
            freez(ect);
        } else {
            prev = ect;
        }

        ect = next;
    }
}

// --------------------------------------------------------------------------------------------------------------------
// Fill variables

/**
 * Set Target Data
 *
 * Set local variable values according shared memory information.
 *
 * @param out local output variable.
 * @param ptr input from shared memory.
 */
static inline void ebpf_cgroup_set_target_data(ebpf_cgroup_target_t *out, netdata_ebpf_cgroup_shm_body_t *ptr)
{
    out->hash = ptr->hash;
    snprintfz(out->name, 255, "%s", ptr->name);
    out->systemd = ptr->options & CGROUP_OPTIONS_SYSTEM_SLICE_SERVICE;
    out->updated = 1;
}

/**
 * Find or create
 *
 * Find the structure inside the link list or allocate and link when it is not present.
 *
 * @param ptr Input from shared memory.
 *
 * @return It returns a pointer for the structure associated with the input.
 */
static ebpf_cgroup_target_t * ebpf_cgroup_find_or_create(netdata_ebpf_cgroup_shm_body_t *ptr)
{
    ebpf_cgroup_target_t *ect, *prev;
    for (ect = ebpf_cgroup_pids, prev = ebpf_cgroup_pids; ect; prev = ect, ect = ect->next) {
        if (ect->hash == ptr->hash && !strcmp(ect->name, ptr->name)) {
            ect->updated = 1;
            return ect;
        }
    }

    ebpf_cgroup_target_t *new_ect = callocz(1, sizeof(ebpf_cgroup_target_t));

    ebpf_cgroup_set_target_data(new_ect, ptr);
    if (!ebpf_cgroup_pids) {
        ebpf_cgroup_pids = new_ect;
    } else {
        prev->next = new_ect;
    }

    return new_ect;
}

/**
 * Update pid link list
 *
 * Update PIDs list associated with specific cgroup.
 *
 * @param ect  cgroup structure where pids will be stored
 * @param path file with PIDs associated to cgroup.
 */
static void ebpf_update_pid_link_list(ebpf_cgroup_target_t *ect, char *path)
{
    procfile *ff = procfile_open_no_log(path, " \t:", PROCFILE_FLAG_DEFAULT);
    if (!ff)
        return;

    ff = procfile_readall(ff);
    if (!ff)
        return;

    size_t lines = procfile_lines(ff), l;
    for (l = 0; l < lines ;l++) {
        int pid = (int)str2l(procfile_lineword(ff, l, 0));
        if (pid) {
            struct pid_on_target2 *pt, *prev;
            for (pt = ect->pids, prev = ect->pids; pt; prev = pt, pt = pt->next) {
                if (pt->pid == pid)
                    break;
            }

            if (!pt) {
                struct pid_on_target2 *w = callocz(1, sizeof(struct pid_on_target2));
                w->pid = pid;
                if (!ect->pids)
                    ect->pids = w;
                else
                    prev->next = w;
            }
        }
    }

    procfile_close(ff);
}

/**
 * Set remove var
 *
 * Set variable remove. If this variable is not reset, the structure will be removed from link list.
 */
 void ebpf_reset_updated_var()
 {
     ebpf_cgroup_target_t *ect;
     for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
         ect->updated = 0;
     }
 }

/**
 * Parse cgroup shared memory
 *
 * This function is responsible to copy necessary data from shared memory to local memory.
 */
void ebpf_parse_cgroup_shm_data()
{
    if (shm_ebpf_cgroup.header) {
        sem_wait(shm_sem_ebpf_cgroup);
        int i, end = shm_ebpf_cgroup.header->cgroup_root_count;

        pthread_mutex_lock(&mutex_cgroup_shm);

        ebpf_remove_cgroup_target_update_list();

        ebpf_reset_updated_var();

        for (i = 0; i < end; i++) {
            netdata_ebpf_cgroup_shm_body_t *ptr = &shm_ebpf_cgroup.body[i];
            if (ptr->enabled) {
                ebpf_cgroup_target_t *ect =  ebpf_cgroup_find_or_create(ptr);
                ebpf_update_pid_link_list(ect, ptr->path);
            }
        }
        pthread_mutex_unlock(&mutex_cgroup_shm);

        sem_post(shm_sem_ebpf_cgroup);
    }
}

// --------------------------------------------------------------------------------------------------------------------
// Create charts

/**
 * Create charts on systemd submenu
 *
 * @param id   the chart id
 * @param title  the value displayed on vertical axis.
 * @param units  the value displayed on vertical axis.
 * @param family Submenu that the chart will be attached on dashboard.
 * @param charttype chart type
 * @param order  the chart order
 * @param algorithm the algorithm used by dimension
 * @param context   add context for chart
 * @param module    chart module name, this is the eBPF thread.
 * @param update_every value to overwrite the update frequency set by the server.
 */
void ebpf_create_charts_on_systemd(char *id, char *title, char *units, char *family, char *charttype, int order,
                                   char *algorithm, char *context, char *module, int update_every)
{
    ebpf_cgroup_target_t *w;
    ebpf_write_chart_cmd(NETDATA_SERVICE_FAMILY, id, title, units, family, charttype, context,
                         order, update_every, module);

    for (w = ebpf_cgroup_pids; w; w = w->next) {
        if (unlikely(w->systemd) && unlikely(w->updated))
            fprintf(stdout, "DIMENSION %s '' %s 1 1\n", w->name, algorithm);
    }
}
