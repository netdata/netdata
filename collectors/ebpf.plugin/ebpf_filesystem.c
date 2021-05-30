// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf_filesystem.h"

struct config fs_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

ebpf_filesystem_partitions_t localfs[] =
    {{.filesystem = "ext4",
      .family = "EXT4",
      .objects = NULL,
      .probe_links = NULL,
      .flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION,
      .enabled = CONFIG_BOOLEAN_YES,
      .addresses = {.function = NULL, .addr = 0}},
     {.filesystem = NULL,
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

/*****************************************************************
 *
 *  COMMON FUNCTIONS
 *
 *****************************************************************/

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

    /*
    if (!dimensions) {
        dimensions = ebpf_fill_histogram_dimension(NETDATA_FILESYSTEM_MAX_BINS);

        memset(filesystem_aggregated_data, 0 , NETDATA_FILESYSTEM_MAX_BINS*sizeof(netdata_syscall_stat_t));
        memset(filesystem_publish_aggregated, 0 , NETDATA_FILESYSTEM_MAX_BINS*sizeof(netdata_publish_syscall_t));

        filesystem_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));
    }
     */

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
    for(l = 0; l < lines ; l++) {
        char *fs = procfile_lineword(ff, l, 7);
        // When `shared` options is added to mount information, the filesystem shifts one position
        if (*fs == '-')
            fs = procfile_lineword(ff, l,8);

        for (i = 0; localfs[i].filesystem; i++) {
            ebpf_filesystem_partitions_t *w = &localfs[i];
            if (w->enabled && !strcmp(fs, w->filesystem)) {
                error("KILLME INSIDE");
                localfs[i].flags |= NETDATA_FILESYSTEM_LOAD_EBPF_PROGRAM;
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

    update_time = curr + 5*em->update_time;
    if (!ebpf_read_local_partitions()) {
        em->optional = -1;
        info("Netdata cannot monitor the filesystems used on this host.");
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

            /*
            freez(efp->hread.name);
            freez(efp->hwrite.name);
            freez(efp->hopen.name);
            freez(efp->hsync.name);
             */

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

    ebpf_filesystem_cleanup_ebpf_data();
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

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
    usec_t step = NETDATA_FILESYSTEM_READ_SLEEP_MS*em->update_time;
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        (void) ebpf_update_partitions(em);
        // No more partitions, stopping everything
        if (em->optional)
            break;
    }

    read_thread_closed = 1;
    return NULL;
}

/**
 * Main loop for this collector.
 *
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

    // Initialize optional as zero, for we identify when there is not partitions to monitor
    em->optional = 0;

    if (ebpf_update_partitions(em)) {
        em->enabled = 0;
        goto endfilesystem;
    }

    filesystem_collector(em);

endfilesystem:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
