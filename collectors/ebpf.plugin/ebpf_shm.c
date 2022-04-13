// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_shm.h"

static char *shm_dimension_name[NETDATA_SHM_END] = { "get", "at", "dt", "ctl" };
static netdata_syscall_stat_t shm_aggregated_data[NETDATA_SHM_END];
static netdata_publish_syscall_t shm_publish_aggregated[NETDATA_SHM_END];

static int read_thread_closed = 1;
netdata_publish_shm_t *shm_vector = NULL;

static netdata_idx_t shm_hash_values[NETDATA_SHM_END];
static netdata_idx_t *shm_values = NULL;

netdata_publish_shm_t **shm_pid = NULL;

struct config shm_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

static ebpf_local_maps_t shm_maps[] = {{.name = "tbl_pid_shm", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                         .user_input = 0,
                                         .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = "shm_ctrl", .internal_input = NETDATA_CONTROLLER_END,
                                         .user_input = 0,
                                         .type = NETDATA_EBPF_MAP_CONTROLLER,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = "tbl_shm", .internal_input = NETDATA_SHM_END,
                                         .user_input = 0,
                                         .type = NETDATA_EBPF_MAP_STATIC,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = NULL, .internal_input = 0, .user_input = 0}};

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

struct netdata_static_thread shm_threads = {"SHM KERNEL", NULL, NULL, 1,
                                             NULL, NULL,  NULL};

netdata_ebpf_targets_t shm_targets[] = { {.name = "shmget", .mode = EBPF_LOAD_TRAMPOLINE},
                                         {.name = "shmat", .mode = EBPF_LOAD_TRAMPOLINE},
                                         {.name = "shmdt", .mode = EBPF_LOAD_TRAMPOLINE},
                                         {.name = "shmctl", .mode = EBPF_LOAD_TRAMPOLINE},
                                         {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

#ifdef LIBBPF_MAJOR_VERSION
#include "includes/shm.skel.h"

static struct shm_bpf *bpf_obj = NULL;

/*****************************************************************
 *
 *  BTF FUNCTIONS
 *
 *****************************************************************/

/*
 * Disable tracepoint
 *
 * Disable all tracepoints to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_shm_disable_tracepoint(struct shm_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_syscall_shmget, false);
    bpf_program__set_autoload(obj->progs.netdata_syscall_shmat, false);
    bpf_program__set_autoload(obj->progs.netdata_syscall_shmdt, false);
    bpf_program__set_autoload(obj->progs.netdata_syscall_shmctl, false);
}

/*
 * Disable probe
 *
 * Disable all probes to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_disable_probe(struct shm_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_shmget_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_shmat_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_shmdt_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_shmctl_probe, false);
}

/*
 * Disable trampoline
 *
 * Disable all trampoline to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_disable_trampoline(struct shm_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_shmget_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_shmat_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_shmdt_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_shmctl_fentry, false);
}

/**
 * Set trampoline target
 *
 * Set the targets we will monitor.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_set_trampoline_target(struct shm_bpf *obj)
{
    char syscall[NETDATA_EBPF_MAX_SYSCALL_LENGTH + 1];
    ebpf_select_host_prefix(syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH,
                            shm_targets[NETDATA_KEY_SHMGET_CALL].name, running_on_kernel);

    bpf_program__set_attach_target(obj->progs.netdata_shmget_fentry, 0,
                                   syscall);

    ebpf_select_host_prefix(syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH,
                            shm_targets[NETDATA_KEY_SHMAT_CALL].name, running_on_kernel);
    bpf_program__set_attach_target(obj->progs.netdata_shmat_fentry, 0,
                                   syscall);

    ebpf_select_host_prefix(syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH,
                            shm_targets[NETDATA_KEY_SHMDT_CALL].name, running_on_kernel);
    bpf_program__set_attach_target(obj->progs.netdata_shmdt_fentry, 0,
                                   syscall);

    ebpf_select_host_prefix(syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH,
                            shm_targets[NETDATA_KEY_SHMCTL_CALL].name, running_on_kernel);
    bpf_program__set_attach_target(obj->progs.netdata_shmctl_fentry, 0,
                                   syscall);
}

/**
 * SHM Attach Probe
 *
 * Attach probes to target
 *
 * @param obj is the main structure for bpf objects.
 *
 * @return It returns 0 on success and -1 otherwise.
 */
static int ebpf_shm_attach_probe(struct shm_bpf *obj)
{
    char syscall[NETDATA_EBPF_MAX_SYSCALL_LENGTH + 1];
    ebpf_select_host_prefix(syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH,
                            shm_targets[NETDATA_KEY_SHMGET_CALL].name, running_on_kernel);

    obj->links.netdata_shmget_probe = bpf_program__attach_kprobe(obj->progs.netdata_shmget_probe,
                                                                 false, syscall);
    int ret = (int)libbpf_get_error(obj->links.netdata_shmget_probe);
    if (ret)
        return -1;

    ebpf_select_host_prefix(syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH,
                            shm_targets[NETDATA_KEY_SHMAT_CALL].name, running_on_kernel);
    obj->links.netdata_shmat_probe = bpf_program__attach_kprobe(obj->progs.netdata_shmat_probe,
                                                                false, syscall);
    ret = (int)libbpf_get_error(obj->links.netdata_shmat_probe);
    if (ret)
        return -1;

    ebpf_select_host_prefix(syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH,
                            shm_targets[NETDATA_KEY_SHMDT_CALL].name, running_on_kernel);
    obj->links.netdata_shmdt_probe = bpf_program__attach_kprobe(obj->progs.netdata_shmdt_probe,
                                                                false, syscall);
    ret = (int)libbpf_get_error(obj->links.netdata_shmdt_probe);
    if (ret)
        return -1;

    ebpf_select_host_prefix(syscall, NETDATA_EBPF_MAX_SYSCALL_LENGTH,
                            shm_targets[NETDATA_KEY_SHMCTL_CALL].name, running_on_kernel);
    obj->links.netdata_shmctl_probe = bpf_program__attach_kprobe(obj->progs.netdata_shmctl_probe,
                                                                 false, syscall);
    ret = (int)libbpf_get_error(obj->links.netdata_shmctl_probe);
    if (ret)
        return -1;

    return 0;
}

/**
 * Set hash tables
 *
 * Set the values for maps according the value given by kernel.
 */
static void ebpf_shm_set_hash_tables(struct shm_bpf *obj)
{
    shm_maps[NETDATA_PID_SHM_TABLE].map_fd = bpf_map__fd(obj->maps.tbl_pid_shm);
    shm_maps[NETDATA_SHM_CONTROLLER].map_fd = bpf_map__fd(obj->maps.shm_ctrl);
    shm_maps[NETDATA_SHM_GLOBAL_TABLE].map_fd = bpf_map__fd(obj->maps.tbl_shm);
}

/**
 * Load and attach
 *
 * Load and attach the eBPF code in kernel.
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 *
 * @return it returns 0 on succes and -1 otherwise
 */
static inline int ebpf_shm_load_and_attach(struct shm_bpf *obj, ebpf_module_t *em)
{
    netdata_ebpf_targets_t *shmt = em->targets;
    netdata_ebpf_program_loaded_t test = shmt[NETDATA_KEY_SHMGET_CALL].mode;

    // We are testing only one, because all will have the same behavior
    if (test == EBPF_LOAD_TRAMPOLINE ) {
        ebpf_shm_disable_tracepoint(obj);
        ebpf_disable_probe(obj);

        ebpf_set_trampoline_target(obj);
    }  else if (test == EBPF_LOAD_PROBE || test == EBPF_LOAD_RETPROBE ) {
        ebpf_shm_disable_tracepoint(obj);
        ebpf_disable_trampoline(obj);
    } else  {
        ebpf_disable_probe(obj);
        ebpf_disable_trampoline(obj);
    }

    int ret = shm_bpf__load(obj);
    if (!ret) {
        if (test != EBPF_LOAD_PROBE && test != EBPF_LOAD_RETPROBE)
            shm_bpf__attach(obj);
        else
            ret = ebpf_shm_attach_probe(obj);

        if (!ret)
            ebpf_shm_set_hash_tables(obj);
    }

    return ret;
}
#endif
/*****************************************************************
 *  FUNCTIONS TO CLOSE THE THREAD
 *****************************************************************/

/**
 * Clean shm structure
 */
void clean_shm_pid_structures() {
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        freez(shm_pid[pids->pid]);

        pids = pids->next;
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_shm_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled) {
        return;
    }

    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t tick = 2 * USEC_PER_MS;
    while (!read_thread_closed) {
        usec_t dt = heartbeat_next(&hb, tick);
        UNUSED(dt);
    }

    ebpf_cleanup_publish_syscall(shm_publish_aggregated);

    freez(shm_vector);
    freez(shm_values);

    if (probe_links) {
        struct bpf_program *prog;
        size_t i = 0 ;
        bpf_object__for_each_program(prog, objects) {
            bpf_link__destroy(probe_links[i]);
            i++;
        }
        bpf_object__close(objects);
    }
#ifdef LIBBPF_MAJOR_VERSION
    else if (bpf_obj)
        shm_bpf__destroy(bpf_obj);
#endif
}

/*****************************************************************
 *  COLLECTOR THREAD
 *****************************************************************/

/**
 * Apps Accumulator
 *
 * Sum all values read from kernel and store in the first address.
 *
 * @param out the vector with read values.
 */
static void shm_apps_accumulator(netdata_publish_shm_t *out)
{
    int i, end = (running_on_kernel >= NETDATA_KERNEL_V4_15) ? ebpf_nprocs : 1;
    netdata_publish_shm_t *total = &out[0];
    for (i = 1; i < end; i++) {
        netdata_publish_shm_t *w = &out[i];
        total->get += w->get;
        total->at += w->at;
        total->dt += w->dt;
        total->ctl += w->ctl;
    }
}

/**
 * Fill PID
 *
 * Fill PID structures
 *
 * @param current_pid pid that we are collecting data
 * @param out         values read from hash tables;
 */
static void shm_fill_pid(uint32_t current_pid, netdata_publish_shm_t *publish)
{
    netdata_publish_shm_t *curr = shm_pid[current_pid];
    if (!curr) {
        curr = callocz(1, sizeof(netdata_publish_shm_t));
        shm_pid[current_pid] = curr;
    }

    memcpy(curr, publish, sizeof(netdata_publish_shm_t));
}

/**
 * Update cgroup
 *
 * Update cgroup data based in
 */
static void ebpf_update_shm_cgroup()
{
    netdata_publish_shm_t *cv = shm_vector;
    int fd = shm_maps[NETDATA_PID_SHM_TABLE].map_fd;
    size_t length = sizeof(netdata_publish_shm_t) * ebpf_nprocs;
    ebpf_cgroup_target_t *ect;

    memset(cv, 0, length);

    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            int pid = pids->pid;
            netdata_publish_shm_t *out = &pids->shm;
            if (likely(shm_pid) && shm_pid[pid]) {
                netdata_publish_shm_t *in = shm_pid[pid];

                memcpy(out, in, sizeof(netdata_publish_shm_t));
            } else {
                if (!bpf_map_lookup_elem(fd, &pid, cv)) {
                    shm_apps_accumulator(cv);

                    memcpy(out, cv, sizeof(netdata_publish_shm_t));

                    // now that we've consumed the value, zero it out in the map.
                    memset(cv, 0, length);
                    bpf_map_update_elem(fd, &pid, cv, BPF_EXIST);
                }
            }
        }
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Read APPS table
 *
 * Read the apps table and store data inside the structure.
 */
static void read_apps_table()
{
    netdata_publish_shm_t *cv = shm_vector;
    uint32_t key;
    struct pid_stat *pids = root_of_pids;
    int fd = shm_maps[NETDATA_PID_SHM_TABLE].map_fd;
    size_t length = sizeof(netdata_publish_shm_t)*ebpf_nprocs;
    while (pids) {
        key = pids->pid;

        if (bpf_map_lookup_elem(fd, &key, cv)) {
            pids = pids->next;
            continue;
        }

        shm_apps_accumulator(cv);

        shm_fill_pid(key, cv);

        // now that we've consumed the value, zero it out in the map.
        memset(cv, 0, length);
        bpf_map_update_elem(fd, &key, cv, BPF_EXIST);

        pids = pids->next;
    }
}

/**
* Send global charts to netdata agent.
*/
static void shm_send_global()
{
    write_begin_chart(NETDATA_EBPF_SYSTEM_GROUP, NETDATA_SHM_GLOBAL_CHART);
    write_chart_dimension(
        shm_publish_aggregated[NETDATA_KEY_SHMGET_CALL].dimension,
        (long long) shm_hash_values[NETDATA_KEY_SHMGET_CALL]
    );
    write_chart_dimension(
        shm_publish_aggregated[NETDATA_KEY_SHMAT_CALL].dimension,
        (long long) shm_hash_values[NETDATA_KEY_SHMAT_CALL]
    );
    write_chart_dimension(
        shm_publish_aggregated[NETDATA_KEY_SHMDT_CALL].dimension,
        (long long) shm_hash_values[NETDATA_KEY_SHMDT_CALL]
    );
    write_chart_dimension(
        shm_publish_aggregated[NETDATA_KEY_SHMCTL_CALL].dimension,
        (long long) shm_hash_values[NETDATA_KEY_SHMCTL_CALL]
    );
    write_end_chart();
}

/**
 * Read global counter
 *
 * Read the table with number of calls for all functions
 */
static void read_global_table()
{
    netdata_idx_t *stored = shm_values;
    netdata_idx_t *val = shm_hash_values;
    int fd = shm_maps[NETDATA_SHM_GLOBAL_TABLE].map_fd;

    uint32_t i, end = NETDATA_SHM_END;
    for (i = NETDATA_KEY_SHMGET_CALL; i < end; i++) {
        if (!bpf_map_lookup_elem(fd, &i, stored)) {
            int j;
            int last = ebpf_nprocs;
            netdata_idx_t total = 0;
            for (j = 0; j < last; j++)
                total += stored[j];

            val[i] = total;
        }
    }
}

/**
 * Shared memory reader thread.
 *
 * @param ptr It is a NULL value for this thread.
 * @return It always returns NULL.
 */
void *ebpf_shm_read_hash(void *ptr)
{
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    usec_t step = NETDATA_SHM_SLEEP_MS * em->update_every;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_global_table();
    }

    read_thread_closed = 1;
    return NULL;
}

/**
 * Sum values for all targets.
 */
static void ebpf_shm_sum_pids(netdata_publish_shm_t *shm, struct pid_on_target *root)
{
    while (root) {
        int32_t pid = root->pid;
        netdata_publish_shm_t *w = shm_pid[pid];
        if (w) {
            shm->get += w->get;
            shm->at += w->at;
            shm->dt += w->dt;
            shm->ctl += w->ctl;

            // reset for next collection.
            w->get = 0;
            w->at = 0;
            w->dt = 0;
            w->ctl = 0;
        }
        root = root->next;
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param root the target list.
*/
void ebpf_shm_send_apps_data(struct target *root)
{
    struct target *w;
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            ebpf_shm_sum_pids(&w->shm, w->root_pid);
        }
    }

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SHMGET_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, (long long) w->shm.get);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SHMAT_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, (long long) w->shm.at);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SHMDT_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, (long long) w->shm.dt);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_SHMCTL_CHART);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            write_chart_dimension(w->name, (long long) w->shm.ctl);
        }
    }
    write_end_chart();
}

/**
 * Sum values for all targets.
 */
static void ebpf_shm_sum_cgroup_pids(netdata_publish_shm_t *shm, struct pid_on_target2 *root)
{
    netdata_publish_shm_t shmv;
    memset(&shmv, 0, sizeof(shmv));
    while (root) {
        netdata_publish_shm_t *w = &root->shm;
        shmv.get += w->get;
        shmv.at += w->at;
        shmv.dt += w->dt;
        shmv.ctl += w->ctl;

        root = root->next;
    }

    memcpy(shm, &shmv, sizeof(shmv));
}

/**
 * Create specific shared memory charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_specific_shm_charts(char *type, int update_every)
{
    ebpf_create_chart(type, NETDATA_SHMGET_CHART,
                      "Calls to syscall <code>shmget(2)</code>.",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_APPS_IPC_SHM_GROUP,
                      NETDATA_CGROUP_SHM_GET_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5800,
                      ebpf_create_global_dimension,
                      &shm_publish_aggregated[NETDATA_KEY_SHMGET_CALL],
                      1,
                      update_every,
                      NETDATA_EBPF_MODULE_NAME_SHM);

    ebpf_create_chart(type, NETDATA_SHMAT_CHART,
                      "Calls to syscall <code>shmat(2)</code>.",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_APPS_IPC_SHM_GROUP,
                      NETDATA_CGROUP_SHM_AT_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5801,
                      ebpf_create_global_dimension,
                      &shm_publish_aggregated[NETDATA_KEY_SHMAT_CALL],
                      1,
                      update_every,
                      NETDATA_EBPF_MODULE_NAME_SHM);

    ebpf_create_chart(type, NETDATA_SHMDT_CHART,
                      "Calls to syscall <code>shmdt(2)</code>.",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_APPS_IPC_SHM_GROUP,
                      NETDATA_CGROUP_SHM_DT_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5802,
                      ebpf_create_global_dimension,
                      &shm_publish_aggregated[NETDATA_KEY_SHMDT_CALL],
                      1,
                      update_every,
                      NETDATA_EBPF_MODULE_NAME_SHM);

    ebpf_create_chart(type, NETDATA_SHMCTL_CHART,
                      "Calls to syscall <code>shmctl(2)</code>.",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_APPS_IPC_SHM_GROUP,
                      NETDATA_CGROUP_SHM_CTL_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5803,
                      ebpf_create_global_dimension,
                      &shm_publish_aggregated[NETDATA_KEY_SHMCTL_CALL],
                      1,
                      update_every,
                      NETDATA_EBPF_MODULE_NAME_SHM);
}

/**
 * Obsolete specific shared memory charts
 *
 * Obsolete charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_obsolete_specific_shm_charts(char *type, int update_every)
{
    ebpf_write_chart_obsolete(type, NETDATA_SHMGET_CHART,
                              "Calls to syscall <code>shmget(2)</code>.",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_IPC_SHM_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_SHM_GET_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5800, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_SHMAT_CHART,
                              "Calls to syscall <code>shmat(2)</code>.",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_IPC_SHM_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_SHM_AT_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5801, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_SHMDT_CHART,
                              "Calls to syscall <code>shmdt(2)</code>.",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_IPC_SHM_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_SHM_DT_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5802, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_SHMCTL_CHART,
                              "Calls to syscall <code>shmctl(2)</code>.",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_IPC_SHM_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_CGROUP_SHM_CTL_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5803, update_every);
}

/**
 *  Create Systemd Swap Charts
 *
 *  Create charts when systemd is enabled
 *
 *  @param update_every value to overwrite the update frequency set by the server.
 **/
static void ebpf_create_systemd_shm_charts(int update_every)
{
    ebpf_create_charts_on_systemd(NETDATA_SHMGET_CHART,
                                  "Calls to syscall <code>shmget(2)</code>.",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_IPC_SHM_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  20191,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SYSTEMD_SHM_GET_CONTEXT, NETDATA_EBPF_MODULE_NAME_SHM, update_every);

    ebpf_create_charts_on_systemd(NETDATA_SHMAT_CHART,
                                  "Calls to syscall <code>shmat(2)</code>.",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_IPC_SHM_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  20192,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SYSTEMD_SHM_AT_CONTEXT, NETDATA_EBPF_MODULE_NAME_SHM, update_every);

    ebpf_create_charts_on_systemd(NETDATA_SHMDT_CHART,
                                  "Calls to syscall <code>shmdt(2)</code>.",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_IPC_SHM_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  20193,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SYSTEMD_SHM_DT_CONTEXT, NETDATA_EBPF_MODULE_NAME_SHM, update_every);

    ebpf_create_charts_on_systemd(NETDATA_SHMCTL_CHART,
                                  "Calls to syscall <code>shmctl(2)</code>.",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_IPC_SHM_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  20193,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SYSTEMD_SHM_CTL_CONTEXT, NETDATA_EBPF_MODULE_NAME_SHM, update_every);
}

/**
 * Send Systemd charts
 *
 * Send collected data to Netdata.
 *
 * @return It returns the status for chart creation, if it is necessary to remove a specific dimension, zero is returned
 *         otherwise function returns 1 to avoid chart recreation
 */
static int ebpf_send_systemd_shm_charts()
{
    int ret = 1;
    ebpf_cgroup_target_t *ect;
    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SHMGET_CHART);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_shm.get);
        } else
            ret = 0;
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SHMAT_CHART);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_shm.at);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SHMDT_CHART);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_shm.dt);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_SHMCTL_CHART);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_shm.ctl);
        }
    }
    write_end_chart();

    return ret;
}

/*
 * Send Specific Shared memory data
 *
 * Send data for specific cgroup/apps.
 *
 * @param type   chart type
 * @param values structure with values that will be sent to netdata
 */
static void ebpf_send_specific_shm_data(char *type, netdata_publish_shm_t *values)
{
    write_begin_chart(type, NETDATA_SHMGET_CHART);
    write_chart_dimension(shm_publish_aggregated[NETDATA_KEY_SHMGET_CALL].name, (long long)values->get);
    write_end_chart();

    write_begin_chart(type, NETDATA_SHMAT_CHART);
    write_chart_dimension(shm_publish_aggregated[NETDATA_KEY_SHMAT_CALL].name, (long long)values->at);
    write_end_chart();

    write_begin_chart(type, NETDATA_SHMDT_CHART);
    write_chart_dimension(shm_publish_aggregated[NETDATA_KEY_SHMDT_CALL].name, (long long)values->dt);
    write_end_chart();

    write_begin_chart(type, NETDATA_SHMCTL_CHART);
    write_chart_dimension(shm_publish_aggregated[NETDATA_KEY_SHMCTL_CALL].name, (long long)values->ctl);
    write_end_chart();
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param update_every value to overwrite the update frequency set by the server.
*/
void ebpf_shm_send_cgroup_data(int update_every)
{
    if (!ebpf_cgroup_pids)
        return;

    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        ebpf_shm_sum_cgroup_pids(&ect->publish_shm, ect->pids);
    }

    int has_systemd = shm_ebpf_cgroup.header->systemd_enabled;
    if (has_systemd) {
        static int systemd_charts = 0;
        if (!systemd_charts) {
            ebpf_create_systemd_shm_charts(update_every);
            systemd_charts = 1;
        }

        systemd_charts = ebpf_send_systemd_shm_charts();
    }

    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_SHM_CHART) && ect->updated) {
            ebpf_create_specific_shm_charts(ect->name, update_every);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_SHM_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_SHM_CHART) {
            if (ect->updated) {
                ebpf_send_specific_shm_data(ect->name, &ect->publish_shm);
            } else {
                ebpf_obsolete_specific_shm_charts(ect->name, update_every);
                ect->flags &= ~NETDATA_EBPF_CGROUP_HAS_SWAP_CHART;
            }
        }
    }

    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
* Main loop for this collector.
*/
static void shm_collector(ebpf_module_t *em)
{
    shm_threads.thread = mallocz(sizeof(netdata_thread_t));
    shm_threads.start_routine = ebpf_shm_read_hash;

    netdata_thread_create(
        shm_threads.thread,
        shm_threads.name,
        NETDATA_THREAD_OPTION_JOINABLE,
        ebpf_shm_read_hash,
        em
    );

    int apps = em->apps_charts;
    int cgroups = em->cgroup_charts;
    int update_every = em->update_every;
    int counter = update_every - 1;
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        if (++counter == update_every) {
            counter = 0;
            if (apps) {
                read_apps_table();
            }

            if (cgroups) {
                ebpf_update_shm_cgroup();
            }

            pthread_mutex_lock(&lock);

            shm_send_global();

            if (apps) {
                ebpf_shm_send_apps_data(apps_groups_root_target);
            }

            if (cgroups) {
                ebpf_shm_send_cgroup_data(update_every);
            }

            pthread_mutex_unlock(&lock);
        }

        pthread_mutex_unlock(&collect_data_mutex);
    }
}

/*****************************************************************
 *  INITIALIZE THREAD
 *****************************************************************/

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_shm_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    struct target *root = ptr;
    ebpf_create_charts_on_apps(NETDATA_SHMGET_CHART,
                               "Calls to syscall <code>shmget(2)</code>.",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_IPC_SHM_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20191,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SHM);

    ebpf_create_charts_on_apps(NETDATA_SHMAT_CHART,
                               "Calls to syscall <code>shmat(2)</code>.",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_IPC_SHM_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20192,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SHM);

    ebpf_create_charts_on_apps(NETDATA_SHMDT_CHART,
                               "Calls to syscall <code>shmdt(2)</code>.",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_IPC_SHM_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20193,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SHM);

    ebpf_create_charts_on_apps(NETDATA_SHMCTL_CHART,
                               "Calls to syscall <code>shmctl(2)</code>.",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_IPC_SHM_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               20194,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SHM);
}

/**
 * Allocate vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 *
 * @param apps is apps enabled?
 */
static void ebpf_shm_allocate_global_vectors(int apps)
{
    if (apps)
        shm_pid = callocz((size_t)pid_max, sizeof(netdata_publish_shm_t *));

    shm_vector = callocz((size_t)ebpf_nprocs, sizeof(netdata_publish_shm_t));

    shm_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_idx_t));

    memset(shm_hash_values, 0, sizeof(shm_hash_values));
}

/*****************************************************************
 *  MAIN THREAD
 *****************************************************************/

/**
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_shm_charts(int update_every)
{
    ebpf_create_chart(
        NETDATA_EBPF_SYSTEM_GROUP,
        NETDATA_SHM_GLOBAL_CHART,
        "Calls to shared memory system calls",
        EBPF_COMMON_DIMENSION_CALL,
        NETDATA_SYSTEM_IPC_SHM_SUBMENU,
        NULL,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_SYSTEM_IPC_SHARED_MEM_CALLS,
        ebpf_create_global_dimension,
        shm_publish_aggregated,
        NETDATA_SHM_END,
        update_every, NETDATA_EBPF_MODULE_NAME_SHM
    );

    fflush(stdout);
}

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 */
static int ebpf_shm_load_bpf(ebpf_module_t *em)
{
    int ret = 0;
    if (em->load == EBPF_LOAD_LEGACY) {
        probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &objects);
        if (!probe_links) {
            em->enabled = CONFIG_BOOLEAN_NO;
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        bpf_obj = shm_bpf__open();
        if (!bpf_obj)
            ret = -1;
        else
            ret = ebpf_shm_load_and_attach(bpf_obj, em);
    }
#endif


    if (ret)
        error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->thread_name);

    return ret;
}

/**
 * Shared memory thread.
 *
 * @param ptr a pointer to `struct ebpf_module`
 * @return It always return NULL
 */
void *ebpf_shm_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_shm_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = shm_maps;

    ebpf_update_pid_table(&shm_maps[NETDATA_PID_SHM_TABLE], em);

    if (!em->enabled) {
        goto endshm;
    }

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_shm_load_bpf(em)) {
        em->enabled = CONFIG_BOOLEAN_NO;
        goto endshm;
    }

    ebpf_shm_allocate_global_vectors(em->apps_charts);

    int algorithms[NETDATA_SHM_END] = {
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX
    };
    ebpf_global_labels(
        shm_aggregated_data,
        shm_publish_aggregated,
        shm_dimension_name,
        shm_dimension_name,
        algorithms,
        NETDATA_SHM_END
    );

    pthread_mutex_lock(&lock);
    ebpf_create_shm_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&lock);

    shm_collector(em);

endshm:
    if (!em->enabled)
        ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
