// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>
#include <stdlib.h>

#include "ebpf.h"
#include "ebpf_disk.h"

struct config disk_config = APPCONFIG_INITIALIZER;

static ebpf_local_maps_t disk_maps[] = {
    {.name = "tbl_disk_iocall",
     .internal_input = NETDATA_DISK_HISTOGRAM_LENGTH,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = "tmp_disk_tp_stat",
     .internal_input = 8192,
     .user_input = 8192,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};
static avl_tree_lock disk_tree;
netdata_ebpf_disks_t *disk_list = NULL;

char *tracepoint_block_type = {"block"};
char *tracepoint_block_issue = {"block_rq_issue"};
char *tracepoint_block_rq_complete = {"block_rq_complete"};

static int was_block_issue_enabled = 0;
static int was_block_rq_complete_enabled = 0;

static char **dimensions = NULL;
static netdata_syscall_stat_t disk_aggregated_data[NETDATA_EBPF_HIST_MAX_BINS];
static netdata_publish_syscall_t disk_publish_aggregated[NETDATA_EBPF_HIST_MAX_BINS];

static netdata_idx_t *disk_hash_values = NULL;

ebpf_publish_disk_t *plot_disks = NULL;
pthread_mutex_t plot_mutex;

#ifdef LIBBPF_MAJOR_VERSION
/**
 * Set hash table
 *
 * Set the values for maps according the value given by kernel.
 *
 * @param obj is the main structure for bpf objects.
 */
static inline void ebpf_disk_set_hash_table(struct disk_bpf *obj)
{
    disk_maps[NETDATA_DISK_IO].map_fd = bpf_map__fd(obj->maps.tbl_disk_iocall);
}

/**
 * Load and attach
 *
 * Load and attach the eBPF code in kernel.
 *
 * @param obj is the main structure for bpf objects.
 *
 * @return it returns 0 on success and -1 otherwise
 */
static inline int ebpf_disk_load_and_attach(struct disk_bpf *obj)
{
    int ret = disk_bpf__load(obj);
    if (ret) {
        return ret;
    }

    return disk_bpf__attach(obj);
}
#endif

/*****************************************************************
 *
 *  FUNCTIONS TO MANIPULATE HARD DISKS
 *
 *****************************************************************/

/**
 * Parse start
 *
 * Parse start address of disk
 *
 * @param w          structure where data is stored
 * @param filename   variable used to store value
 *
 * @return It returns 0 on success and -1 otherwise
 */
static inline int ebpf_disk_parse_start(netdata_ebpf_disks_t *w, char *filename)
{
    char content[FILENAME_MAX + 1];
    int fd = open(filename, O_RDONLY, 0);
    if (fd < 0) {
        return -1;
    }

    ssize_t file_length = read(fd, content, 4095);
    if (file_length > 0) {
        if (file_length > FILENAME_MAX)
            file_length = FILENAME_MAX;

        content[file_length] = '\0';
        w->start = strtoul(content, NULL, 10);
    }
    close(fd);

    return 0;
}

/**
 * Parse uevent
 *
 * Parse uevent file
 *
 * @param w          structure where data is stored
 * @param filename   variable used to store value
 *
 * @return It returns 0 on success and -1 otherwise
 */
static inline int ebpf_parse_uevent(netdata_ebpf_disks_t *w, char *filename)
{
    char content[FILENAME_MAX + 1];
    int fd = open(filename, O_RDONLY, 0);
    if (fd < 0) {
        return -1;
    }

    ssize_t file_length = read(fd, content, FILENAME_MAX);
    if (file_length > 0) {
        if (file_length > FILENAME_MAX)
            file_length = FILENAME_MAX;

        content[file_length] = '\0';

        char *s = strstr(content, "PARTNAME=EFI");
        if (s) {
            w->main->boot_partition = w;
            w->flags |= NETDATA_DISK_HAS_EFI;
            w->boot_chart = strdupz("disk_bootsector");
        }
    }
    close(fd);

    return 0;
}

/**
 * Parse Size
 *
 * @param w          structure where data is stored
 * @param filename   variable used to store value
 *
 * @return It returns 0 on success and -1 otherwise
 */
static inline int ebpf_parse_size(netdata_ebpf_disks_t *w, char *filename)
{
    char content[FILENAME_MAX + 1];
    int fd = open(filename, O_RDONLY, 0);
    if (fd < 0) {
        return -1;
    }

    ssize_t file_length = read(fd, content, FILENAME_MAX);
    if (file_length > 0) {
        if (file_length > FILENAME_MAX)
            file_length = FILENAME_MAX;

        content[file_length] = '\0';
        w->end = w->start + strtoul(content, NULL, 10) - 1;
    }
    close(fd);

    return 0;
}

/**
 * Read Disk information
 *
 * Read disk information from /sys/block
 *
 * @param w    structure where data is stored
 * @param name disk name
 */
static void ebpf_read_disk_info(netdata_ebpf_disks_t *w, char *name)
{
    static netdata_ebpf_disks_t *main_disk = NULL;
    static uint32_t key = 0;
    char *path = {"/sys/block"};
    char disk[NETDATA_DISK_NAME_LEN + 1];
    char filename[FILENAME_MAX + 1];
    snprintfz(disk, NETDATA_DISK_NAME_LEN, "%s", name);
    size_t length = strlen(disk);
    if (!length) {
        return;
    }

    length--;
    size_t curr = length;
    while (isdigit((int)disk[length])) {
        disk[length--] = '\0';
    }

    // We are looking for partition information, if it is a device we will ignore it.
    if (curr == length) {
        main_disk = w;
        key = MKDEV(w->major, w->minor);
        w->bootsector_key = key;
        return;
    }
    w->bootsector_key = key;
    w->main = main_disk;

    snprintfz(filename, FILENAME_MAX, "%s/%s/%s/uevent", path, disk, name);
    if (ebpf_parse_uevent(w, filename))
        return;

    snprintfz(filename, FILENAME_MAX, "%s/%s/%s/start", path, disk, name);
    if (ebpf_disk_parse_start(w, filename))
        return;

    snprintfz(filename, FILENAME_MAX, "%s/%s/%s/size", path, disk, name);
    ebpf_parse_size(w, filename);
}

/**
 * New encode dev
 *
 * New encode algorithm extracted from https://elixir.bootlin.com/linux/v5.10.8/source/include/linux/kdev_t.h#L39
 *
 * @param major  driver major number
 * @param minor  driver minor number
 *
 * @return
 */
static inline uint32_t netdata_new_encode_dev(uint32_t major, uint32_t minor)
{
    return (minor & 0xff) | (major << 8) | ((minor & ~0xff) << 12);
}

/**
 * Compare disks
 *
 * Compare major and minor values to add disks to tree.
 *
 * @param a pointer to netdata_ebpf_disks
 * @param b pointer to netdata_ebpf_disks
 *
 * @return It returns 0 case the values are equal, 1 case a is bigger than b and -1 case a is smaller than b.
*/
static int ebpf_compare_disks(void *a, void *b)
{
    netdata_ebpf_disks_t *ptr1 = a;
    netdata_ebpf_disks_t *ptr2 = b;

    if (ptr1->dev > ptr2->dev)
        return 1;
    if (ptr1->dev < ptr2->dev)
        return -1;

    return 0;
}

/**
 * Update listen table
 *
 * Update link list when it is necessary.
 *
 * @param name         disk name
 * @param major        major disk identifier
 * @param minor        minor disk identifier
 * @param current_time current timestamp
 */
static void update_disk_table(char *name, int major, int minor, time_t current_time)
{
    netdata_ebpf_disks_t find;
    netdata_ebpf_disks_t *w;
    size_t length;

    uint32_t dev = netdata_new_encode_dev(major, minor);
    find.dev = dev;
    netdata_ebpf_disks_t *ret = (netdata_ebpf_disks_t *)avl_search_lock(&disk_tree, (avl_t *)&find);
    if (ret) { // Disk is already present
        ret->flags |= NETDATA_DISK_IS_HERE;
        ret->last_update = current_time;
        return;
    }

    netdata_ebpf_disks_t *update_next = disk_list;
    if (likely(disk_list)) {
        netdata_ebpf_disks_t *move = disk_list;
        while (move) {
            if (dev == move->dev)
                return;

            update_next = move;
            move = move->next;
        }

        w = callocz(1, sizeof(netdata_ebpf_disks_t));
        length = strlen(name);
        if (length >= NETDATA_DISK_NAME_LEN)
            length = NETDATA_DISK_NAME_LEN;

        memcpy(w->family, name, length);
        w->family[length] = '\0';
        w->major = major;
        w->minor = minor;
        w->dev = netdata_new_encode_dev(major, minor);
        update_next->next = w;
    } else {
        disk_list = callocz(1, sizeof(netdata_ebpf_disks_t));
        length = strlen(name);
        if (length >= NETDATA_DISK_NAME_LEN)
            length = NETDATA_DISK_NAME_LEN;

        memcpy(disk_list->family, name, length);
        disk_list->family[length] = '\0';
        disk_list->major = major;
        disk_list->minor = minor;
        disk_list->dev = netdata_new_encode_dev(major, minor);

        w = disk_list;
    }

    ebpf_read_disk_info(w, name);

    netdata_ebpf_disks_t *check;
    check = (netdata_ebpf_disks_t *)avl_insert_lock(&disk_tree, (avl_t *)w);
    if (check != w)
        netdata_log_error("Internal error, cannot insert the AVL tree.");

#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_info(
        "The Latency is monitoring the hard disk %s (Major = %d, Minor = %d, Device = %u)", name, major, minor, w->dev);
#endif

    w->flags |= NETDATA_DISK_IS_HERE;
}

/**
 *  Read Local Disks
 *
 *  Parse /proc/partitions to get block disks used to measure latency.
 *
 *  @return It returns 0 on success and -1 otherwise
 */
static int read_local_disks()
{
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, NETDATA_EBPF_PROC_PARTITIONS);
    procfile *ff = procfile_open(filename, " \t:", PROCFILE_FLAG_DEFAULT);
    if (!ff)
        return -1;

    ff = procfile_readall(ff);
    if (!ff)
        return -1;

    size_t lines = procfile_lines(ff), l;
    time_t current_time = now_realtime_sec();
    for (l = 2; l < lines; l++) {
        size_t words = procfile_linewords(ff, l);
        // This is header or end of file
        if (unlikely(words < 4))
            continue;

        int major = (int)strtol(procfile_lineword(ff, l, 0), NULL, 10);
        // The main goal of this thread is to measure block devices, so any block device with major number
        // smaller than 7 according /proc/devices is not "important".
        if (major > 7) {
            int minor = (int)strtol(procfile_lineword(ff, l, 1), NULL, 10);
            update_disk_table(procfile_lineword(ff, l, 3), major, minor, current_time);
        }
    }

    procfile_close(ff);

    return 0;
}

/**
 * Update disks
 *
 * @param em main thread structure
 */
void ebpf_update_disks(ebpf_module_t *em)
{
    static time_t update_every = 0;
    time_t curr = now_realtime_sec();
    if (curr < update_every)
        return;

    update_every = curr + 5 * em->update_every;

    (void)read_local_disks();
}

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
 * Disk disable tracepoints
 *
 * Disable tracepoints when the plugin was responsible to enable it.
 */
static void ebpf_disk_disable_tracepoints()
{
    char *default_message = {"Cannot disable the tracepoint"};
    if (!was_block_issue_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_block_type, tracepoint_block_issue))
            netdata_log_error("%s %s/%s.", default_message, tracepoint_block_type, tracepoint_block_issue);
    }

    if (!was_block_rq_complete_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_block_type, tracepoint_block_rq_complete))
            netdata_log_error("%s %s/%s.", default_message, tracepoint_block_type, tracepoint_block_rq_complete);
    }
}

/**
 * Cleanup plot disks
 *
 * Clean disk list
 */
static void ebpf_cleanup_plot_disks()
{
    ebpf_publish_disk_t *move = plot_disks, *next;
    while (move) {
        next = move->next;

        freez(move);

        move = next;
    }
    plot_disks = NULL;
}

/**
 * Cleanup Disk List
 */
static void ebpf_cleanup_disk_list()
{
    netdata_ebpf_disks_t *move = disk_list;
    while (move) {
        netdata_ebpf_disks_t *next = move->next;

        freez(move->histogram.name);
        freez(move->boot_chart);
        freez(move);

        move = next;
    }
    disk_list = NULL;
}

/**
 * Obsolete global
 *
 * Obsolete global charts created by thread.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_disk_global(ebpf_module_t *em)
{
    ebpf_publish_disk_t *move = plot_disks;
    while (move) {
        netdata_ebpf_disks_t *ned = move->plot;
        uint32_t flags = ned->flags;
        if (flags & NETDATA_DISK_CHART_CREATED) {
            ebpf_write_chart_obsolete(
                ned->histogram.name,
                ned->family,
                "",
                "Disk latency",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                ned->family,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                NETDATA_EBPF_DISK_LATENCY_CONTEXT,
                ned->histogram.order,
                em->update_every);
        }

        move = move->next;
    }
}

/**
 * Disk exit.
 *
 * Cancel child and exit.
 *
 * @param ptr thread data.
 */
static void ebpf_disk_exit(void *pptr)
{
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        pthread_mutex_lock(&lock);

        ebpf_obsolete_disk_global(em);

        pthread_mutex_unlock(&lock);
        fflush(stdout);
    }
    ebpf_disk_disable_tracepoints();

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, disk_maps, EBPF_ACTION_STAT_REMOVE);

    if (em->objects) {
        ebpf_unload_legacy_code(em->objects, em->probe_links);
        em->objects = NULL;
        em->probe_links = NULL;
    }

    if (dimensions)
        ebpf_histogram_dimension_cleanup(dimensions, NETDATA_EBPF_HIST_MAX_BINS);

    freez(disk_hash_values);
    disk_hash_values = NULL;
    pthread_mutex_destroy(&plot_mutex);

    ebpf_cleanup_plot_disks();
    ebpf_cleanup_disk_list();

    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&ebpf_exit_cleanup);
}

/*****************************************************************
 *
 *  MAIN LOOP
 *
 *****************************************************************/

/**
 * Fill Plot list
 *
 * @param ptr a pointer for current disk
 */
static void ebpf_fill_plot_disks(netdata_ebpf_disks_t *ptr)
{
    pthread_mutex_lock(&plot_mutex);
    ebpf_publish_disk_t *w;
    if (likely(plot_disks)) {
        ebpf_publish_disk_t *move = plot_disks, *store = plot_disks;
        while (move) {
            if (move->plot == ptr) {
                pthread_mutex_unlock(&plot_mutex);
                return;
            }

            store = move;
            move = move->next;
        }

        w = callocz(1, sizeof(ebpf_publish_disk_t));
        w->plot = ptr;
        store->next = w;
    } else {
        plot_disks = callocz(1, sizeof(ebpf_publish_disk_t));
        plot_disks->plot = ptr;
    }
    pthread_mutex_unlock(&plot_mutex);

    ptr->flags |= NETDATA_DISK_ADDED_TO_PLOT_LIST;
}

/**
 * Read hard disk table
 *
 * Read the table with number of calls for all functions
 *
 * @param table file descriptor for table
 * @param maps_per_core do I need to read all cores?
 */
static void read_hard_disk_tables(int table, int maps_per_core)
{
    netdata_idx_t *values = disk_hash_values;
    block_key_t key = {};
    block_key_t next_key = {};

    netdata_ebpf_disks_t *ret = NULL;

    while (bpf_map_get_next_key(table, &key, &next_key) == 0) {
        int test = bpf_map_lookup_elem(table, &key, values);
        if (test < 0) {
            key = next_key;
            continue;
        }

        netdata_ebpf_disks_t find;
        find.dev = key.dev;

        if (likely(ret)) {
            if (find.dev != ret->dev)
                ret = (netdata_ebpf_disks_t *)avl_search_lock(&disk_tree, (avl_t *)&find);
        } else
            ret = (netdata_ebpf_disks_t *)avl_search_lock(&disk_tree, (avl_t *)&find);

        // Disk was inserted after we parse /proc/partitions
        if (!ret) {
            if (read_local_disks()) {
                key = next_key;
                continue;
            }

            ret = (netdata_ebpf_disks_t *)avl_search_lock(&disk_tree, (avl_t *)&find);
            if (!ret) {
                // We should never reach this point, but we are adding it to keep a safe code
                key = next_key;
                continue;
            }
        }

        uint64_t total = 0;
        int i;
        int end = (maps_per_core) ? 1 : ebpf_nprocs;
        for (i = 0; i < end; i++) {
            total += values[i];
        }

        ret->histogram.histogram[key.bin] = total;

        if (!(ret->flags & NETDATA_DISK_ADDED_TO_PLOT_LIST))
            ebpf_fill_plot_disks(ret);

        key = next_key;
    }
}

/**
 * Obsolete Hard Disk charts
 *
 * Make Hard disk charts and fill chart name
 *
 * @param w the structure with necessary information to create the chart
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_obsolete_hd_charts(netdata_ebpf_disks_t *w, int update_every)
{
    ebpf_write_chart_obsolete(
        w->histogram.name,
        w->family,
        "",
        "Disk latency",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        w->family,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_EBPF_DISK_LATENCY_CONTEXT,
        w->histogram.order,
        update_every);

    w->flags = NETDATA_DISK_NONE;
}

/**
 * Create Hard Disk charts
 *
 * Make Hard disk charts and fill chart name
 *
 * @param w the structure with necessary information to create the chart
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_hd_charts(netdata_ebpf_disks_t *w, int update_every)
{
    int order = NETDATA_CHART_PRIO_DISK_LATENCY;
    char *family = w->family;

    w->histogram.name = strdupz("disk_latency_io");
    w->histogram.title = NULL;
    w->histogram.order = order;

    ebpf_create_chart(
        w->histogram.name,
        family,
        "Disk latency",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        family,
        NETDATA_EBPF_DISK_LATENCY_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        order,
        ebpf_create_global_dimension,
        disk_publish_aggregated,
        NETDATA_EBPF_HIST_MAX_BINS,
        update_every,
        NETDATA_EBPF_MODULE_NAME_DISK);
    order++;

    w->flags |= NETDATA_DISK_CHART_CREATED;

    fflush(stdout);
}

/**
 * Remove pointer from plot
 *
 * Remove pointer from plot list when the disk is not present.
 */
static void ebpf_remove_pointer_from_plot_disk(ebpf_module_t *em)
{
    time_t current_time = now_realtime_sec();
    time_t limit = 10 * em->update_every;
    pthread_mutex_lock(&plot_mutex);
    ebpf_publish_disk_t *move = plot_disks, *prev = plot_disks;
    int update_every = em->update_every;
    while (move) {
        netdata_ebpf_disks_t *ned = move->plot;
        uint32_t flags = ned->flags;

        if (!(flags & NETDATA_DISK_IS_HERE) && ((current_time - ned->last_update) > limit)) {
            ebpf_obsolete_hd_charts(ned, update_every);
            avl_t *ret = (avl_t *)avl_remove_lock(&disk_tree, (avl_t *)ned);
            UNUSED(ret);
            if (move == plot_disks) {
                freez(move);
                plot_disks = NULL;
                break;
            } else {
                prev->next = move->next;
                ebpf_publish_disk_t *clean = move;
                move = move->next;
                freez(clean);
                continue;
            }
        }

        prev = move;
        move = move->next;
    }
    pthread_mutex_unlock(&plot_mutex);
}

/**
 * Send Hard disk data
 *
 * Send hard disk information to Netdata.
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_latency_send_hd_data(int update_every)
{
    pthread_mutex_lock(&plot_mutex);
    if (!plot_disks) {
        pthread_mutex_unlock(&plot_mutex);
        return;
    }

    ebpf_publish_disk_t *move = plot_disks;
    while (move) {
        netdata_ebpf_disks_t *ned = move->plot;
        uint32_t flags = ned->flags;
        if (!(flags & NETDATA_DISK_CHART_CREATED)) {
            ebpf_create_hd_charts(ned, update_every);
        }

        if ((flags & NETDATA_DISK_CHART_CREATED)) {
            write_histogram_chart(
                ned->histogram.name, ned->family, ned->histogram.histogram, dimensions, NETDATA_EBPF_HIST_MAX_BINS);
        }

        ned->flags &= ~NETDATA_DISK_IS_HERE;

        move = move->next;
    }
    pthread_mutex_unlock(&plot_mutex);
}

/**
* Main loop for this collector.
*/
static void disk_collector(ebpf_module_t *em)
{
    disk_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));

    int update_every = em->update_every;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    int counter = update_every - 1;
    int maps_per_core = em->maps_per_core;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);

        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        read_hard_disk_tables(disk_maps[NETDATA_DISK_IO].map_fd, maps_per_core);
        pthread_mutex_lock(&lock);
        ebpf_remove_pointer_from_plot_disk(em);
        ebpf_latency_send_hd_data(update_every);

        pthread_mutex_unlock(&lock);

        ebpf_update_disks(em);

        pthread_mutex_lock(&ebpf_exit_cleanup);
        if (running_time && !em->running_time)
            running_time = update_every;
        else
            running_time += update_every;

        em->running_time = running_time;
        pthread_mutex_unlock(&ebpf_exit_cleanup);
    }
}

/*****************************************************************
 *
 *  EBPF DISK THREAD
 *
 *****************************************************************/

/**
 * Enable tracepoints
 *
 * Enable necessary tracepoints for thread.
 *
 * @return  It returns 0 on success and -1 otherwise
 */
static int ebpf_disk_enable_tracepoints()
{
    int test = ebpf_is_tracepoint_enabled(tracepoint_block_type, tracepoint_block_issue);
    if (test == -1)
        return -1;
    else if (!test) {
        if (ebpf_enable_tracing_values(tracepoint_block_type, tracepoint_block_issue))
            return -1;
    }
    was_block_issue_enabled = test;

    test = ebpf_is_tracepoint_enabled(tracepoint_block_type, tracepoint_block_rq_complete);
    if (test == -1)
        return -1;
    else if (!test) {
        if (ebpf_enable_tracing_values(tracepoint_block_type, tracepoint_block_rq_complete))
            return -1;
    }
    was_block_rq_complete_enabled = test;

    return 0;
}

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 *
 * @return It returns 0 on success and -1 otherwise.
 */
static int ebpf_disk_load_bpf(ebpf_module_t *em)
{
    int ret = 0;
    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
        if (!em->probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        disk_bpf_obj = disk_bpf__open();
        if (!disk_bpf_obj)
            ret = -1;
        else {
            ret = ebpf_disk_load_and_attach(disk_bpf_obj);
            if (!ret)
                ebpf_disk_set_hash_table(disk_bpf_obj);
        }
    }
#endif

    if (ret)
        netdata_log_error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->info.thread_name);

    return ret;
}

/**
 * Disk thread
 *
 * Thread used to generate disk charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void ebpf_disk_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(ebpf_disk_exit) cleanup_ptr = em;

    em->maps = disk_maps;

    if (ebpf_disk_enable_tracepoints()) {
        goto enddisk;
    }

    avl_init_lock(&disk_tree, ebpf_compare_disks);
    if (read_local_disks()) {
        goto enddisk;
    }

    if (pthread_mutex_init(&plot_mutex, NULL)) {
        netdata_log_error("Cannot initialize local mutex");
        goto enddisk;
    }

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(disk_maps, em->maps_per_core, running_on_kernel);
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_disk_load_bpf(em)) {
        goto enddisk;
    }

    int algorithms[NETDATA_EBPF_HIST_MAX_BINS];
    ebpf_fill_algorithms(algorithms, NETDATA_EBPF_HIST_MAX_BINS, NETDATA_EBPF_INCREMENTAL_IDX);
    dimensions = ebpf_fill_histogram_dimension(NETDATA_EBPF_HIST_MAX_BINS);

    ebpf_global_labels(
        disk_aggregated_data, disk_publish_aggregated, dimensions, dimensions, algorithms, NETDATA_EBPF_HIST_MAX_BINS);

    pthread_mutex_lock(&lock);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, disk_maps, EBPF_ACTION_STAT_ADD);
    pthread_mutex_unlock(&lock);

    disk_collector(em);

enddisk:
    ebpf_update_disabled_plugin_stats(em);
}
