// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_cgroup.h"

ebpf_cgroup_target_t *ebpf_cgroup_pids = NULL;
static void *ebpf_mapped_memory = NULL;
int send_cgroup_chart = 0;

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
 * @return It returns a pointer to the region mapped on success and MAP_FAILED otherwise.
 */
static inline void *ebpf_cgroup_map_shm_locally(int fd, size_t length)
{
    void *value;

    value = nd_mmap(NULL, length, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
    if (!value) {
        netdata_log_error(
            "Cannot map shared memory used between eBPF and cgroup, integration between processes won't happen");
        close(shm_fd_ebpf_cgroup);
        shm_fd_ebpf_cgroup = -1;
        shm_unlink(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME);
    }

    return value;
}

/**
 * Unmap Shared Memory
 *
 * Unmap shared memory used to integrate eBPF and cgroup plugin
 */
void ebpf_unmap_cgroup_shared_memory()
{
    nd_munmap(ebpf_mapped_memory, shm_ebpf_cgroup.header->body_length);
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

    if (shm_fd_ebpf_cgroup < 0) {
        shm_fd_ebpf_cgroup = shm_open(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME, O_RDWR, 0660);
        if (shm_fd_ebpf_cgroup < 0) {
            if (limit_try == NETDATA_EBPF_CGROUP_MAX_TRIES)
                netdata_log_error("Shared memory was not initialized, integration between processes won't happen.");

            return;
        }
    }

    // Map only header
    void *mapped = (netdata_ebpf_cgroup_shm_header_t *)ebpf_cgroup_map_shm_locally(
        shm_fd_ebpf_cgroup, sizeof(netdata_ebpf_cgroup_shm_header_t));
    if (unlikely(mapped == SEM_FAILED)) {
        return;
    }
    netdata_ebpf_cgroup_shm_header_t *header = mapped;

    size_t length = header->body_length;

    nd_munmap(header, sizeof(netdata_ebpf_cgroup_shm_header_t));

    if (length <= ((sizeof(netdata_ebpf_cgroup_shm_header_t) + sizeof(netdata_ebpf_cgroup_shm_body_t)))) {
        return;
    }

    ebpf_mapped_memory = (void *)ebpf_cgroup_map_shm_locally(shm_fd_ebpf_cgroup, length);
    if (unlikely(ebpf_mapped_memory == MAP_FAILED)) {
        return;
    }
    shm_ebpf_cgroup.header = ebpf_mapped_memory;
    shm_ebpf_cgroup.body = ebpf_mapped_memory + sizeof(netdata_ebpf_cgroup_shm_header_t);

    shm_sem_ebpf_cgroup = sem_open(NETDATA_NAMED_SEMAPHORE_EBPF_CGROUP_NAME, O_CREAT, 0660, 1);

    if (shm_sem_ebpf_cgroup == SEM_FAILED) {
        netdata_log_error("Cannot create semaphore, integration between eBPF and cgroup won't happen");
        limit_try = NETDATA_EBPF_CGROUP_MAX_TRIES + 1;
        nd_munmap(ebpf_mapped_memory, length);
        shm_ebpf_cgroup.header = NULL;
        shm_ebpf_cgroup.body = NULL;
        close(shm_fd_ebpf_cgroup);
        shm_fd_ebpf_cgroup = -1;
        shm_unlink(NETDATA_SHARED_MEMORY_EBPF_CGROUP_NAME);
    }
}

// --------------------------------------------------------------------------------------------------------------------
// Close and Cleanup

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
static ebpf_cgroup_target_t *ebpf_cgroup_find_or_create(netdata_ebpf_cgroup_shm_body_t *ptr)
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
    for (l = 0; l < lines; l++) {
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
    static int previous = 0;
    if (!shm_ebpf_cgroup.header || shm_sem_ebpf_cgroup == SEM_FAILED)
        return;

    sem_wait(shm_sem_ebpf_cgroup);
    int i, end = shm_ebpf_cgroup.header->cgroup_root_count;
    if (end <= 0) {
        sem_post(shm_sem_ebpf_cgroup);
        return;
    }

    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_remove_cgroup_target_update_list();

    ebpf_reset_updated_var();

    for (i = 0; i < end; i++) {
        netdata_ebpf_cgroup_shm_body_t *ptr = &shm_ebpf_cgroup.body[i];
        if (ptr->enabled) {
            ebpf_cgroup_target_t *ect = ebpf_cgroup_find_or_create(ptr);
            ebpf_update_pid_link_list(ect, ptr->path);
        }
    }
    send_cgroup_chart = previous != shm_ebpf_cgroup.header->cgroup_root_count;
    previous = shm_ebpf_cgroup.header->cgroup_root_count;
    sem_post(shm_sem_ebpf_cgroup);
    pthread_mutex_unlock(&mutex_cgroup_shm);
#ifdef NETDATA_DEV_MODE
    netdata_log_info(
        "Updating cgroup %d (Previous: %d, Current: %d)",
        send_cgroup_chart,
        previous,
        shm_ebpf_cgroup.header->cgroup_root_count);
#endif

    sem_post(shm_sem_ebpf_cgroup);
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
void ebpf_create_charts_on_systemd(ebpf_systemd_args_t *chart)
{
    ebpf_write_chart_cmd(
        chart->id,
        chart->suffix,
        "",
        chart->title,
        chart->units,
        chart->family,
        chart->charttype,
        chart->context,
        chart->order,
        chart->update_every,
        chart->module);
    char service_name[512];
    snprintfz(service_name, 511, "%s", (!strstr(chart->id, "systemd_")) ? chart->id : (chart->id + 8));
    ebpf_create_chart_labels("service_name", service_name, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();
    // Let us keep original string that can be used in another place. Chart creation does not happen frequently.
    char *move = strdupz(chart->dimension);
    while (move) {
        char *next_dim = strchr(move, ',');
        if (next_dim) {
            *next_dim = '\0';
            next_dim++;
        }

        fprintf(stdout, "DIMENSION %s '' %s 1 1\n", move, chart->algorithm);
        move = next_dim;
    }
    freez(move);
}

// --------------------------------------------------------------------------------------------------------------------
// Cgroup main thread

/**
 * Cgroup integratin
 *
 * Thread responsible to call functions responsible to sync data between plugins.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void *ebpf_cgroup_integration(void *ptr __maybe_unused)
{
    int counter = NETDATA_EBPF_CGROUP_UPDATE - 1;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    //Plugin will be killed when it receives a signal
    while (!ebpf_plugin_stop()) {
        heartbeat_next(&hb);

        // We are using a small heartbeat time to wake up thread,
        // but we should not update so frequently the shared memory data
        if (++counter >= NETDATA_EBPF_CGROUP_UPDATE) {
            counter = 0;
            if (!shm_ebpf_cgroup.header)
                ebpf_map_cgroup_shared_memory();
            else
                ebpf_parse_cgroup_shm_data();
        }
    }

    return NULL;
}
