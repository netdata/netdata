// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf_filesystem.h"

struct config fs_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

ebpf_filesystem_partitions_t localfs[] =
    {{.filesystem = "ext4",
      .optional_filesystem = NULL,
      .family = "EXT4",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0}},
     {.filesystem = "xfs",
      .optional_filesystem = NULL,
      .family = "XFS",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0}},
     {.filesystem = "nfs",
      .optional_filesystem = "nfs4",
      .family = "NFS",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_ATTR_CHARTS,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0}},
     {.filesystem = "zfs",
      .optional_filesystem = NULL,
      .family = "ZFS",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0}},
     {.filesystem = "btrfs",
      .optional_filesystem = NULL,
      .family = "BTRFS",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FILL_ADDRESS_TABLE,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = "btrfs_file_operations", .addr = 0}},
     {.filesystem = NULL,
      .optional_filesystem = NULL,
      .family = NULL,
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0}}};

struct netdata_static_thread filesystem_threads = {"EBPF FS READ",
                                                   NULL, NULL, 1, NULL,
                                                   NULL, NULL };

static int read_thread_closed = 1;
static netdata_syscall_stat_t filesystem_aggregated_data[NETDATA_EBPF_HIST_MAX_BINS];
static netdata_publish_syscall_t filesystem_publish_aggregated[NETDATA_EBPF_HIST_MAX_BINS];

char **dimensions = NULL;
static netdata_idx_t *filesystem_hash_values = NULL;

/*****************************************************************
 *
 *  COMMON FUNCTIONS
 *
 *****************************************************************/

/**
 * Create Filesystem chart
 *
 * Create latency charts
 */
static void ebpf_obsolete_fs_charts()
{
    int i;
    uint32_t test = NETDATA_FILESYSTEM_FLAG_CHART_CREATED | NETDATA_FILESYSTEM_REMOVE_CHARTS;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        uint32_t flags = efp->flags;
        if ((flags & test) == test) {
            flags &= ~NETDATA_FILESYSTEM_FLAG_CHART_CREATED;

            ebpf_write_chart_obsolete(NETDATA_FILESYSTEM_FAMILY, efp->hread.name,
                                      efp->hread.title,
                                      EBPF_COMMON_DIMENSION_CALL, efp->family_name,
                                      NULL, NETDATA_EBPF_CHART_TYPE_STACKED, efp->hread.order);

            ebpf_write_chart_obsolete(NETDATA_FILESYSTEM_FAMILY, efp->hwrite.name,
                                      efp->hwrite.title,
                                      EBPF_COMMON_DIMENSION_CALL, efp->family_name,
                                      NULL, NETDATA_EBPF_CHART_TYPE_STACKED, efp->hwrite.order);

            ebpf_write_chart_obsolete(NETDATA_FILESYSTEM_FAMILY, efp->hopen.name, efp->hopen.title,
                                      EBPF_COMMON_DIMENSION_CALL, efp->family_name,
                                      NULL, NETDATA_EBPF_CHART_TYPE_STACKED, efp->hopen.order);

            ebpf_write_chart_obsolete(NETDATA_FILESYSTEM_FAMILY, efp->hadditional.name, efp->hadditional.title,
                                      EBPF_COMMON_DIMENSION_CALL, efp->family_name,
                                      NULL, NETDATA_EBPF_CHART_TYPE_STACKED, efp->hadditional.order);
        }
        efp->flags = flags;
    }
}

/**
 * Create Filesystem chart
 *
 * Create latency charts
 */
static void ebpf_create_fs_charts()
{
    static int order = NETDATA_CHART_PRIO_EBPF_FILESYSTEM_CHARTS;
    char chart_name[64], title[256], family[64];
    int i;
    uint32_t test = NETDATA_FILESYSTEM_FLAG_CHART_CREATED|NETDATA_FILESYSTEM_REMOVE_CHARTS;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        uint32_t flags = efp->flags;
        if (flags & NETDATA_FILESYSTEM_FLAG_HAS_PARTITION && !(flags & test)) {
            snprintfz(title, 255, "%s latency for each read request.", efp->filesystem);
            snprintfz(family, 63, "%s_latency", efp->family);
            snprintfz(chart_name, 63, "%s_read_latency", efp->filesystem);
            efp->hread.name = strdupz(chart_name);
            efp->hread.title = strdupz(title);
            efp->hread.order = order;
            efp->family_name = strdupz(family);

            ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY, efp->hread.name,
                              title,
                              EBPF_COMMON_DIMENSION_CALL, family,
                              NULL, NETDATA_EBPF_CHART_TYPE_STACKED, order, ebpf_create_global_dimension,
                              filesystem_publish_aggregated, NETDATA_EBPF_HIST_MAX_BINS);
            order++;

            snprintfz(title, 255, "%s latency for each write request.", efp->filesystem);
            snprintfz(chart_name, 63, "%s_write_latency", efp->filesystem);
            efp->hwrite.name = strdupz(chart_name);
            efp->hwrite.title = strdupz(title);
            efp->hwrite.order = order;
            ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY, efp->hwrite.name,
                              title,
                              EBPF_COMMON_DIMENSION_CALL, family,
                              NULL, NETDATA_EBPF_CHART_TYPE_STACKED, order, ebpf_create_global_dimension,
                              filesystem_publish_aggregated, NETDATA_EBPF_HIST_MAX_BINS);
            order++;

            snprintfz(title, 255, "%s latency for each open request.", efp->filesystem);
            snprintfz(chart_name, 63, "%s_open_latency", efp->filesystem);
            efp->hopen.name = strdupz(chart_name);
            efp->hopen.title = strdupz(title);
            efp->hopen.order = order;
            ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY, efp->hopen.name,
                              title,
                              EBPF_COMMON_DIMENSION_CALL, family,
                              NULL, NETDATA_EBPF_CHART_TYPE_STACKED, order, ebpf_create_global_dimension,
                              filesystem_publish_aggregated, NETDATA_EBPF_HIST_MAX_BINS);
            order++;

            char *type = (efp->flags & NETDATA_FILESYSTEM_ATTR_CHARTS) ? "attribute" : "sync";
            snprintfz(title, 255, "%s latency for each %s request.", efp->filesystem, type);
            snprintfz(chart_name, 63, "%s_%s_latency", efp->filesystem, type);
            efp->hadditional.name = strdupz(chart_name);
            efp->hadditional.title = strdupz(title);
            efp->hadditional.order = order;
            ebpf_create_chart(NETDATA_FILESYSTEM_FAMILY, efp->hadditional.name, title,
                              EBPF_COMMON_DIMENSION_CALL, family,
                              NULL, NETDATA_EBPF_CHART_TYPE_STACKED, order, ebpf_create_global_dimension,
                              filesystem_publish_aggregated, NETDATA_EBPF_HIST_MAX_BINS);
            order++;
            efp->flags |= NETDATA_FILESYSTEM_FLAG_CHART_CREATED;
        }
    }
}

/**
 * Initialize eBPF data
 *
 * @param em  main thread structure.
 *
 * @return it returns 0 on success and -1 otherwise.
 */
int ebpf_filesystem_initialize_ebpf_data(ebpf_module_t *em)
{
    int i;
    const char *saved_name = em->thread_name;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        if (!efp->probe_links && efp->flags & NETDATA_FILESYSTEM_LOAD_EBPF_PROGRAM) {
            ebpf_data_t *ed = &efp->kernel_info;
            fill_ebpf_data(ed);

            if (ebpf_update_kernel(ed)) {
                em->thread_name = saved_name;
                return -1;
            }

            em->thread_name = efp->filesystem;
            efp->probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string,
                                                 &efp->objects, ed->map_fd);
            if (!efp->probe_links) {
                em->thread_name = saved_name;
                return -1;
            }
            efp->flags |= NETDATA_FILESYSTEM_FLAG_HAS_PARTITION;

            // Nedeed for filesystems like btrfs
            if ((efp->flags & NETDATA_FILESYSTEM_FILL_ADDRESS_TABLE) && (efp->addresses.function))
                ebpf_load_addresses(&efp->addresses, efp->kernel_info.map_fd[NETDATA_ADDR_FS_TABLE]);
        }
        efp->flags &= ~NETDATA_FILESYSTEM_LOAD_EBPF_PROGRAM;
    }
    em->thread_name = saved_name;

    if (!dimensions) {
        dimensions = ebpf_fill_histogram_dimension(NETDATA_EBPF_HIST_MAX_BINS);

        memset(filesystem_aggregated_data, 0 , NETDATA_EBPF_HIST_MAX_BINS * sizeof(netdata_syscall_stat_t));
        memset(filesystem_publish_aggregated, 0 , NETDATA_EBPF_HIST_MAX_BINS * sizeof(netdata_publish_syscall_t));

        filesystem_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));
    }

    return 0;
}

/**
 * Read Local partitions
 *
 * @return  the total of partitions that will be monitored
 */
static int ebpf_read_local_partitions()
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/self/mountinfo", netdata_configured_host_prefix);
    procfile *ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) {
        snprintfz(filename, FILENAME_MAX, "%s/proc/1/mountinfo", netdata_configured_host_prefix);
        ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 0;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0;

    int count = 0;
    unsigned long l, i, lines = procfile_lines(ff);
    for (i = 0; localfs[i].filesystem; i++) {
        localfs[i].flags |= NETDATA_FILESYSTEM_REMOVE_CHARTS;
    }

    for(l = 0; l < lines ; l++) {
        // In "normal" situation the expected value is at column 7
        // When `shared` options is added to mount information, the filesystem is at column 8
        // Finally when we have systemd starting netdata, it will be at column 9
        unsigned long index = procfile_linewords(ff, l) - 3;

        char *fs = procfile_lineword(ff, l, index);

        for (i = 0; localfs[i].filesystem; i++) {
            ebpf_filesystem_partitions_t *w = &localfs[i];
            if (w->enabled && (!strcmp(fs, w->filesystem) ||
                              (w->optional_filesystem && !strcmp(fs, w->optional_filesystem)))) {
                localfs[i].flags |= NETDATA_FILESYSTEM_LOAD_EBPF_PROGRAM;
                localfs[i].flags &= ~NETDATA_FILESYSTEM_REMOVE_CHARTS;
                count++;
                break;
            }
        }
    }
    procfile_close(ff);

    return count;
}

/**
 *  Update partition
 *
 *  Update the partition structures before to plot
 *
 * @param em main thread structure
 *
 * @return 0 on success and -1 otherwise.
 */
static int ebpf_update_partitions(ebpf_module_t *em)
{
    static time_t update_time = 0;
    time_t curr = now_realtime_sec();
    if (curr < update_time)
        return 0;

    update_time = curr + 5 * em->update_time;
    if (!ebpf_read_local_partitions()) {
        em->optional = -1;
        return -1;
    }

    if (ebpf_filesystem_initialize_ebpf_data(em)) {
        return -1;
    }

    return 0;
}

/*****************************************************************
 *
 *  CLEANUP FUNCTIONS
 *
 *****************************************************************/

/*
 * Cleanup eBPF data
 */
void ebpf_filesystem_cleanup_ebpf_data()
{
    int i;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        if (efp->probe_links) {
            freez(efp->kernel_info.map_fd);
            freez(efp->family_name);

            freez(efp->hread.name);
            freez(efp->hread.title);

            freez(efp->hwrite.name);
            freez(efp->hwrite.title);

            freez(efp->hopen.name);
            freez(efp->hopen.title);

            freez(efp->hadditional.name);
            freez(efp->hadditional.title);

            struct bpf_link **probe_links = efp->probe_links;
            size_t j = 0 ;
            struct bpf_program *prog;
            bpf_object__for_each_program(prog, efp->objects) {
                bpf_link__destroy(probe_links[j]);
                j++;
            }
            bpf_object__close(efp->objects);
        }
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_filesystem_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t tick = 2*USEC_PER_MS;
    while (!read_thread_closed) {
        usec_t dt = heartbeat_next(&hb, tick);
        UNUSED(dt);
    }

    freez(filesystem_threads.thread);
    ebpf_cleanup_publish_syscall(filesystem_publish_aggregated);

    ebpf_filesystem_cleanup_ebpf_data();
    if (dimensions)
        ebpf_histogram_dimension_cleanup(dimensions, NETDATA_EBPF_HIST_MAX_BINS);
    freez(filesystem_hash_values);
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Select hist
 *
 * Select a histogram to store data.
 *
 * @param efp pointer for the structure with pointers.
 * @param id  histogram selector
 *
 * @return It returns a pointer for the histogram
 */
static inline netdata_ebpf_histogram_t *select_hist(ebpf_filesystem_partitions_t *efp, uint32_t *idx, uint32_t id)
{
    if (id < NETDATA_KEY_CALLS_READ) {
        *idx = id;
        return &efp->hread;
    } else if (id < NETDATA_KEY_CALLS_WRITE) {
        *idx = id - NETDATA_KEY_CALLS_READ;
        return &efp->hwrite;
    } else if (id < NETDATA_KEY_CALLS_OPEN) {
        *idx = id - NETDATA_KEY_CALLS_WRITE;
        return &efp->hopen;
    } else if (id < NETDATA_KEY_CALLS_SYNC ){
        *idx = id - NETDATA_KEY_CALLS_OPEN;
        return &efp->hadditional;
    }

    return NULL;
}

/**
 * Read hard disk table
 *
 * @param table index for the hash table
 *
 * Read the table with number of calls for all functions
 */
static void read_filesystem_table(ebpf_filesystem_partitions_t *efp)
{
    netdata_idx_t *values = filesystem_hash_values;
    uint32_t key;
    uint32_t idx;
    int fd = efp->kernel_info.map_fd[NETDATA_MAIN_FS_TABLE];
    for (key = 0; key < NETDATA_KEY_CALLS_SYNC; key++) {
        netdata_ebpf_histogram_t *w = select_hist(efp, &idx, key);
        if (!w) {
            continue;
        }

        int test = bpf_map_lookup_elem(fd, &key, values);
        if (test < 0) {
            continue;
        }

        uint64_t total = 0;
        int i;
        int end = ebpf_nprocs;
        for (i = 0; i < end; i++) {
            total += values[i];
        }

        if (idx >= NETDATA_EBPF_HIST_MAX_BINS)
            idx = NETDATA_EBPF_HIST_MAX_BINS - 1;
        w->histogram[idx] = total;
    }
}

/**
 * Read hard disk table
 *
 * @param table index for the hash table
 *
 * Read the table with number of calls for all functions
 */
static void read_filesystem_tables()
{
    int i;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        if (efp->flags & NETDATA_FILESYSTEM_FLAG_HAS_PARTITION) {
            read_filesystem_table(efp);
        }
    }
}

/**
 * Socket read hash
 *
 * This is the thread callback.
 * This thread is necessary, because we cannot freeze the whole plugin to read the data on very busy socket.
 *
 * @param ptr It is a NULL value for this thread.
 *
 * @return It always returns NULL.
 */
void *ebpf_filesystem_read_hash(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    read_thread_closed = 0;

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = NETDATA_FILESYSTEM_READ_SLEEP_MS * em->update_time;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        (void) ebpf_update_partitions(em);
        ebpf_obsolete_fs_charts();

        // No more partitions, it is not necessary to read tables
        if (em->optional)
            continue;

        read_filesystem_tables();
    }

    read_thread_closed = 1;
    return NULL;
}

/**
 * Send Hard disk data
 *
 * Send hard disk information to Netdata.
 */
static void ebpf_histogram_send_data()
{
    uint32_t i;
    uint32_t test = NETDATA_FILESYSTEM_FLAG_HAS_PARTITION | NETDATA_FILESYSTEM_REMOVE_CHARTS;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        if ((efp->flags & test) == NETDATA_FILESYSTEM_FLAG_HAS_PARTITION) {
            write_histogram_chart(NETDATA_FILESYSTEM_FAMILY, efp->hread.name,
                                  efp->hread.histogram, dimensions, NETDATA_EBPF_HIST_MAX_BINS);

            write_histogram_chart(NETDATA_FILESYSTEM_FAMILY, efp->hwrite.name,
                                  efp->hwrite.histogram, dimensions, NETDATA_EBPF_HIST_MAX_BINS);

            write_histogram_chart(NETDATA_FILESYSTEM_FAMILY, efp->hopen.name,
                                  efp->hopen.histogram, dimensions, NETDATA_EBPF_HIST_MAX_BINS);

            write_histogram_chart(NETDATA_FILESYSTEM_FAMILY, efp->hadditional.name,
                                  efp->hadditional.histogram, dimensions, NETDATA_EBPF_HIST_MAX_BINS);
        }
    }
}

/**
 * Main loop for this collector.
 *
 * @param em main structure for this thread
 */
static void filesystem_collector(ebpf_module_t *em)
{
    filesystem_threads.thread = mallocz(sizeof(netdata_thread_t));
    filesystem_threads.start_routine = ebpf_filesystem_read_hash;

    netdata_thread_create(filesystem_threads.thread, filesystem_threads.name,
                          NETDATA_THREAD_OPTION_JOINABLE, ebpf_filesystem_read_hash, em);

    while (!close_ebpf_plugin || em->optional) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        pthread_mutex_lock(&lock);

        ebpf_create_fs_charts();
        ebpf_histogram_send_data();

        pthread_mutex_unlock(&collect_data_mutex);
        pthread_mutex_unlock(&lock);
    }
}

/*****************************************************************
 *
 *  ENTRY THREAD
 *
 *****************************************************************/

/**
 * Update Filesystem
 *
 * Update file system structure using values read from configuration file.
 */
static void ebpf_update_filesystem()
{
    char dist[NETDATA_FS_MAX_DIST_NAME + 1];
    int i;
    for (i = 0; localfs[i].filesystem; i++) {
        snprintfz(dist, NETDATA_FS_MAX_DIST_NAME, "%sdist", localfs[i].filesystem);

        localfs[i].enabled = appconfig_get_boolean(&fs_config, NETDATA_FILESYSTEM_CONFIG_NAME, dist,
                                                   CONFIG_BOOLEAN_YES);
    }
}

/**
 * Filesystem thread
 *
 * Thread used to generate socket charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_filesystem_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_filesystem_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    ebpf_update_filesystem();

    if (!em->enabled)
        goto endfilesystem;

    // Initialize optional as zero, to identify when there are not partitions to monitor
    em->optional = 0;

    if (ebpf_update_partitions(em)) {
        if (em->optional)
            info("Netdata cannot monitor the filesystems used on this host.");

        em->enabled = 0;
        goto endfilesystem;
    }

    int algorithms[NETDATA_EBPF_HIST_MAX_BINS];
    ebpf_fill_algorithms(algorithms, NETDATA_EBPF_HIST_MAX_BINS, NETDATA_EBPF_INCREMENTAL_IDX);
    ebpf_global_labels(filesystem_aggregated_data, filesystem_publish_aggregated, dimensions, dimensions,
                       algorithms, NETDATA_EBPF_HIST_MAX_BINS);

    pthread_mutex_lock(&lock);
    ebpf_create_fs_charts();
    pthread_mutex_unlock(&lock);

    filesystem_collector(em);

endfilesystem:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
