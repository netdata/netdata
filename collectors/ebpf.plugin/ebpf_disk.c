// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_disk.h"

struct config disk_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

static ebpf_local_maps_t disk_maps[] = {{.name = "tbl_disk_rcall", .internal_input = NETDATA_DISK_HISTOGRAM_LENGTH,
                                         .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = "tbl_disk_wcall", .internal_input = NETDATA_DISK_HISTOGRAM_LENGTH,
                                         .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                        {.name = NULL, .internal_input = 0, .user_input = 0,
                                         .type = NETDATA_EBPF_MAP_CONTROLLER,
                                         .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED}};
static ebpf_data_t disk_data;

static avl_tree_lock disk_tree;
netdata_ebpf_disks_t *disk_list = NULL;

char *tracepoint_block_type = { "block"} ;
char *tracepoint_block_issue = { "block_rq_issue" };
char *tracepoint_block_rq_complete = { "block_rq_complete" };

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

static int was_block_issue_enabled = 0;
static int was_block_rq_complete_enabled = 0;

static char **dimensions = NULL;

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
 * @param w    structure where data is stored
 * @param text variable used to store value
 *
 * @return It returns 0 on success and -1 otherwise
 */
static inline int ebpf_disk_parse_start(netdata_ebpf_disks_t *w, char *text)
{
    int fd = open(text, O_RDONLY, 0);
    if (fd < 0) {
        return -1;
    }

    ssize_t file_length = read(fd, text, 4095);
    if (file_length > 0) {
        text[file_length] = '\0';
        w->start = str2uint64_t(text);
    }
    close(fd);

    return 0;
}

/**
 * Parse uevent
 *
 * Parse uevent file
 *
 * @param w    structure where data is stored
 * @param text variable used to store value
 *
 * @return It returns 0 on success and -1 otherwise
 */
static inline int ebpf_parse_uevent(netdata_ebpf_disks_t *w, char *text)
{
    int fd = open(text, O_RDONLY, 0);
    if (fd < 0) {
        return -1;
    }

    ssize_t file_length = read(fd, text, FILENAME_MAX);
    if (file_length > 0) {
        text[file_length] = '\0';

        char *s = strstr(text, "PARTNAME=EFI");
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
 * @param w    structure where data is stored
 * @param text variable used to store value
 *
 * @return It returns 0 on success and -1 otherwise
 */
static inline int ebpf_parse_size(netdata_ebpf_disks_t *w, char *text)
{
    int fd = open(text, O_RDONLY, 0);
    if (fd < 0) {
        return -1;
    }

    ssize_t file_length = read(fd, text, FILENAME_MAX);
    if (file_length > 0) {
        text[file_length] = '\0';
        w->end = w->start + str2uint64_t(text) -1;
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
    char *path = { "/sys/block" };
    char disk[NETDATA_DISK_NAME_LEN + 1];
    char text[FILENAME_MAX + 1];
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

    snprintfz(text, FILENAME_MAX, "%s/%s/%s/uevent", path, disk, name);
    if (ebpf_parse_uevent(w, text))
        return;

    snprintfz(text, FILENAME_MAX, "%s/%s/%s/start", path, disk, name);
    if (ebpf_disk_parse_start(w, text))
        return;

    snprintfz(text, FILENAME_MAX, "%s/%s/%s/size", path, disk, name);
    ebpf_parse_size(w, text);
}

// Decode function extracted from: https://elixir.bootlin.com/linux/v5.10.8/source/include/linux/kdev_t.h#L46
/**
 * New encode dev
 *
 * New enconde algorithm
 *
 * @param major  driver major number
 * @param minor  driver minor number
 *
 * @return
 */
static inline uint32_t netdata_new_encode_dev(uint32_t major, uint32_t minor) {
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
 * @param name the disk name
 */
static void update_disk_table(char *name, int major, int minor)
{
    netdata_ebpf_disks_t find;
    netdata_ebpf_disks_t *w;

    uint32_t dev = netdata_new_encode_dev(major, minor);
    find.dev = dev;
    netdata_ebpf_disks_t *ret = (netdata_ebpf_disks_t *) avl_search_lock(&disk_tree, (avl_t *)&find);
    if (ret) { // Disk is already present
        ret->flags |= NETDATA_DISK_IS_HERE;
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
        strcpy(w->family, name);
        w->major = major;
        w->minor = minor;
        w->dev = netdata_new_encode_dev(major, minor);
        update_next->next = w;
    } else {
        disk_list = callocz(1, sizeof(netdata_ebpf_disks_t));
        strcpy(disk_list->family, name);
        disk_list->major = major;
        disk_list->minor = minor;
        disk_list->dev = netdata_new_encode_dev(major, minor);

        w = disk_list;
    }

    ebpf_read_disk_info(w, name);

    netdata_ebpf_disks_t *check;
    check = (netdata_ebpf_disks_t *) avl_insert_lock(&disk_tree, (avl_t *)w);
    if (check != w)
        error("Internal error, cannot insert the AVL tree.");

#ifdef NETDATA_INTERNAL_CHECKS
    info("The Latency is monitoring the hard disk %s (Major = %d, Minor = %d, Device = %u)", name, major, minor,w->dev);
#endif

    w->flags |= NETDATA_DISK_IS_HERE;
}

/**
 *  Read Local Ports
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
    for(l = 2; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        // This is header or end of file
        if (unlikely(words < 4))
            continue;

        int major = (int)strtol(procfile_lineword(ff, l, 0), NULL, 10);
        // The main goal of this thread is to measure block devices, so any block device with major number
        // smaller than 7 according /proc/devices is not "important".
        if (major > 7) {
            int minor = (int)strtol(procfile_lineword(ff, l, 1), NULL, 10);
            update_disk_table(procfile_lineword(ff, l, 3), major, minor);
        }
    }

    procfile_close(ff);

    return 0;
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
    char *default_message = { "Cannot disable the tracepoint" };
    if (!was_block_issue_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_block_type, tracepoint_block_issue))
            error("%s %s/%s.", default_message, tracepoint_block_type, tracepoint_block_issue);
    }

    if (!was_block_rq_complete_enabled) {
        if (ebpf_disable_tracing_values(tracepoint_block_type, tracepoint_block_rq_complete))
            error("%s %s/%s.", default_message, tracepoint_block_type, tracepoint_block_rq_complete);
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_disk_cleanup(void *ptr)
{
    ebpf_disk_disable_tracepoints();

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    if (dimensions)
        ebpf_histogram_dimension_cleanup(dimensions, NETDATA_EBPF_HIST_MAX_BINS);

    if (probe_links) {
        struct bpf_program *prog;
        size_t i = 0 ;
        bpf_object__for_each_program(prog, objects) {
            bpf_link__destroy(probe_links[i]);
            i++;
        }
        bpf_object__close(objects);
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

/**
 * Disk thread
 *
 * Thread used to generate disk charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_disk_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_disk_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = disk_maps;

    fill_ebpf_data(&disk_data);

    if (!em->enabled)
        goto enddisk;

    if (ebpf_update_kernel(&disk_data)) {
        goto enddisk;
    }

    if (ebpf_disk_enable_tracepoints()) {
        em->enabled = CONFIG_BOOLEAN_NO;
        goto enddisk;
    }

    avl_init_lock(&disk_tree, ebpf_compare_disks);
    if (read_local_disks()) {
        em->enabled = CONFIG_BOOLEAN_NO;
        goto enddisk;
    }

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, disk_data.map_fd);
    if (!probe_links) {
        goto enddisk;
    }

    int algorithms[NETDATA_EBPF_HIST_MAX_BINS];
    ebpf_fill_algorithms(algorithms, NETDATA_EBPF_HIST_MAX_BINS, NETDATA_EBPF_INCREMENTAL_IDX);
    dimensions = ebpf_fill_histogram_dimension(NETDATA_EBPF_HIST_MAX_BINS);

enddisk:
    netdata_thread_cleanup_pop(1);

    return NULL;
}
