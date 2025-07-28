// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf_filesystem.h"

struct config fs_config = APPCONFIG_INITIALIZER;

ebpf_local_maps_t ext4_maps[] = {
    {.name = "tbl_ext4",
     .internal_input = NETDATA_KEY_CALLS_SYNC,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "tmp_ext4",
     .internal_input = 4192,
     .user_input = 4192,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
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
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    }};

ebpf_local_maps_t xfs_maps[] = {
    {.name = "tbl_xfs",
     .internal_input = NETDATA_KEY_CALLS_SYNC,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "tmp_xfs",
     .internal_input = 4192,
     .user_input = 4192,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
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
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    }};

ebpf_local_maps_t nfs_maps[] = {
    {.name = "tbl_nfs",
     .internal_input = NETDATA_KEY_CALLS_SYNC,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "tmp_nfs",
     .internal_input = 4192,
     .user_input = 4192,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
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
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    }};

ebpf_local_maps_t zfs_maps[] = {
    {.name = "tbl_zfs",
     .internal_input = NETDATA_KEY_CALLS_SYNC,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "tmp_zfs",
     .internal_input = 4192,
     .user_input = 4192,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
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
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    }};

ebpf_local_maps_t btrfs_maps[] = {
    {.name = "tbl_btrfs",
     .internal_input = NETDATA_KEY_CALLS_SYNC,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "tbl_ext_addr",
     .internal_input = 1,
     .user_input = 1,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = "tmp_btrfs",
     .internal_input = 4192,
     .user_input = 4192,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
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
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    }};

static netdata_syscall_stat_t filesystem_aggregated_data[NETDATA_EBPF_HIST_MAX_BINS];
static netdata_publish_syscall_t filesystem_publish_aggregated[NETDATA_EBPF_HIST_MAX_BINS];

char **dimensions = NULL;
static netdata_idx_t *filesystem_hash_values = NULL;

#ifdef LIBBPF_MAJOR_VERSION
/**
 *  FS disable kprobe
 *
 *  Disable kprobes, because system will use trampolines.
 *  We are not calling this function for while, because we are prioritizing kprobes. We opted by this road, because
 *  distribution are still not deliverying necessary btf files per FS.
 *
 *  @param obj FS object loaded.
 */
static void ebpf_fs_disable_kprobe(struct filesystem_bpf *obj)
{
    // kprobe
    bpf_program__set_autoload(obj->progs.netdata_fs_file_read_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_file_write_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_file_open_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_2nd_file_open_probe, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_getattr_probe, false);
    // kretprobe
    bpf_program__set_autoload(obj->progs.netdata_fs_file_read_retprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_file_write_retprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_file_open_retprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_2nd_file_open_retprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_getattr_retprobe, false);
}

/**
  * Disable trampoline
  *
  * Disable trampolines to use kprobes.
  *
  *  @param obj FS object loaded.
  */
static void ebpf_fs_disable_trampoline(struct filesystem_bpf *obj)
{
    // entry
    bpf_program__set_autoload(obj->progs.netdata_fs_file_read_entry, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_file_write_entry, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_file_open_entry, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_getattr_entry, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_2nd_file_open_entry, false);

    // exit
    bpf_program__set_autoload(obj->progs.netdata_fs_file_read_exit, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_file_write_exit, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_file_open_exit, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_getattr_exit, false);
    bpf_program__set_autoload(obj->progs.netdata_fs_2nd_file_open_exit, false);
}

/**
  * Set targets
  *
  * Set targets for each objects.
  *
  *  @param obj        FS object loaded.
  *  @param functions  array with function names.
  */
static void ebpf_fs_set_target(struct filesystem_bpf *obj, const char **functions)
{
    // entry
    bpf_program__set_attach_target(obj->progs.netdata_fs_file_read_entry, 0, functions[NETDATA_KEY_BTF_READ]);
    bpf_program__set_attach_target(obj->progs.netdata_fs_file_write_entry, 0, functions[NETDATA_KEY_BTF_WRITE]);
    bpf_program__set_attach_target(obj->progs.netdata_fs_file_open_entry, 0, functions[NETDATA_KEY_BTF_OPEN]);
    bpf_program__set_attach_target(obj->progs.netdata_fs_getattr_entry, 0, functions[NETDATA_KEY_BTF_SYNC_ATTR]);

    // exit
    bpf_program__set_attach_target(obj->progs.netdata_fs_file_read_exit, 0, functions[NETDATA_KEY_BTF_READ]);
    bpf_program__set_attach_target(obj->progs.netdata_fs_file_write_exit, 0, functions[NETDATA_KEY_BTF_WRITE]);
    bpf_program__set_attach_target(obj->progs.netdata_fs_file_open_exit, 0, functions[NETDATA_KEY_BTF_OPEN]);
    bpf_program__set_attach_target(obj->progs.netdata_fs_getattr_exit, 0, functions[NETDATA_KEY_BTF_SYNC_ATTR]);

    if (functions[NETDATA_KEY_BTF_OPEN2]) {
        bpf_program__set_attach_target(obj->progs.netdata_fs_2nd_file_open_entry, 0, functions[NETDATA_KEY_BTF_OPEN2]);
        bpf_program__set_attach_target(obj->progs.netdata_fs_2nd_file_open_exit, 0, functions[NETDATA_KEY_BTF_OPEN2]);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_fs_2nd_file_open_entry, false);
        bpf_program__set_autoload(obj->progs.netdata_fs_2nd_file_open_exit, false);
    }
}

/**
 * Attach Kprobe
 *
 * Attach kprobe on targets
 *
  *  @param obj        FS object loaded.
  *  @param functions  array with function names.
 */
static int ebpf_fs_attach_kprobe(struct filesystem_bpf *obj, const char **functions)
{
    // kprobe
    obj->links.netdata_fs_file_read_probe =
        bpf_program__attach_kprobe(obj->progs.netdata_fs_file_read_probe, false, functions[NETDATA_KEY_BTF_READ]);
    if (libbpf_get_error(obj->links.netdata_fs_file_read_probe))
        return -1;

    obj->links.netdata_fs_file_write_probe =
        bpf_program__attach_kprobe(obj->progs.netdata_fs_file_write_probe, false, functions[NETDATA_KEY_BTF_WRITE]);
    if (libbpf_get_error(obj->links.netdata_fs_file_write_probe))
        return -1;

    obj->links.netdata_fs_file_open_probe =
        bpf_program__attach_kprobe(obj->progs.netdata_fs_file_open_probe, false, functions[NETDATA_KEY_BTF_OPEN]);
    if (libbpf_get_error(obj->links.netdata_fs_file_open_probe))
        return -1;

    obj->links.netdata_fs_getattr_probe =
        bpf_program__attach_kprobe(obj->progs.netdata_fs_getattr_probe, false, functions[NETDATA_KEY_BTF_SYNC_ATTR]);
    if (libbpf_get_error(obj->links.netdata_fs_getattr_probe))
        return -1;

    // kretprobe
    obj->links.netdata_fs_file_read_retprobe =
        bpf_program__attach_kprobe(obj->progs.netdata_fs_file_read_retprobe, false, functions[NETDATA_KEY_BTF_READ]);
    if (libbpf_get_error(obj->links.netdata_fs_file_read_retprobe))
        return -1;

    obj->links.netdata_fs_file_write_retprobe =
        bpf_program__attach_kprobe(obj->progs.netdata_fs_file_write_retprobe, false, functions[NETDATA_KEY_BTF_WRITE]);
    if (libbpf_get_error(obj->links.netdata_fs_file_write_retprobe))
        return -1;

    obj->links.netdata_fs_file_open_retprobe =
        bpf_program__attach_kprobe(obj->progs.netdata_fs_file_open_retprobe, false, functions[NETDATA_KEY_BTF_OPEN]);
    if (libbpf_get_error(obj->links.netdata_fs_file_open_retprobe))
        return -1;

    obj->links.netdata_fs_getattr_retprobe =
        bpf_program__attach_kprobe(obj->progs.netdata_fs_getattr_retprobe, false, functions[NETDATA_KEY_BTF_SYNC_ATTR]);
    if (libbpf_get_error(obj->links.netdata_fs_getattr_retprobe))
        return -1;

    if (functions[NETDATA_KEY_BTF_OPEN2]) {
        obj->links.netdata_fs_2nd_file_open_probe = bpf_program__attach_kprobe(
            obj->progs.netdata_fs_2nd_file_open_probe, false, functions[NETDATA_KEY_BTF_OPEN2]);
        if (libbpf_get_error(obj->links.netdata_fs_2nd_file_open_probe))
            return -1;

        obj->links.netdata_fs_2nd_file_open_retprobe = bpf_program__attach_kprobe(
            obj->progs.netdata_fs_2nd_file_open_retprobe, false, functions[NETDATA_KEY_BTF_OPEN2]);
        if (libbpf_get_error(obj->links.netdata_fs_2nd_file_open_retprobe))
            return -1;
    }

    return 0;
}

/**
 * Load and Attach
 *
 * Load binary and attach to targets.
 *
 *  @param map        Structure with information about maps.
 *  @param obj        FS object loaded.
 *  @param functions  array with function names.
 *  @param bf         sttruct with btf file loaded.
 */
static inline int
ebpf_fs_load_and_attach(ebpf_local_maps_t *map, struct filesystem_bpf *obj, const char **functions, struct btf *bf)
{
    if (bf) {
        ebpf_fs_disable_kprobe(obj);
        ebpf_fs_set_target(obj, functions);
    } else {
        ebpf_fs_disable_trampoline(obj);
    }

    int ret = filesystem_bpf__load(obj);
    if (ret) {
        fprintf(stderr, "failed to load BPF object: %d\n", ret);
        return -1;
    }

    if (bf)
        ret = filesystem_bpf__attach(obj);
    else
        ret = ebpf_fs_attach_kprobe(obj, functions);

    if (!ret)
        map->map_fd = bpf_map__fd(obj->maps.tbl_fs);

    return ret;
}
#endif

/*****************************************************************
 *
 *  COMMON FUNCTIONS
 *
 *****************************************************************/

/**
 * Obsolete Cleanup Struct
 *
 * Clean allocatged data durinc obsolete steps
 *
 * @param efp
 */
static void ebpf_obsolete_cleanup_struct(ebpf_filesystem_partitions_t *efp)
{
    freez(efp->hread.name);
    efp->hread.name = NULL;
    freez(efp->hread.title);
    efp->hread.title = NULL;
    freez(efp->hread.ctx);
    efp->hread.ctx = NULL;

    freez(efp->hwrite.name);
    efp->hwrite.name = NULL;
    freez(efp->hwrite.title);
    efp->hwrite.title = NULL;
    freez(efp->hwrite.ctx);
    efp->hwrite.ctx = NULL;

    freez(efp->hopen.name);
    efp->hopen.name = NULL;
    freez(efp->hopen.title);
    efp->hopen.title = NULL;
    freez(efp->hopen.ctx);
    efp->hopen.ctx = NULL;

    freez(efp->hadditional.name);
    efp->hadditional.name = NULL;
    freez(efp->hadditional.title);
    efp->hadditional.title = NULL;
    freez(efp->hadditional.ctx);
    efp->hadditional.ctx = NULL;

    freez(efp->family_name);
    efp->family_name = NULL;
}

/**
 * Create Filesystem chart
 *
 * Create latency charts
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_obsolete_fs_charts(int update_every)
{
    int i;
    uint32_t test = NETDATA_FILESYSTEM_FLAG_CHART_CREATED | NETDATA_FILESYSTEM_REMOVE_CHARTS;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        uint32_t flags = efp->flags;
        if ((flags & test) == test) {
            flags &= ~test;

            ebpf_write_chart_obsolete(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hread.name,
                "",
                efp->hread.title,
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                efp->family_name,
                NULL,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                efp->hread.order,
                update_every);

            ebpf_write_chart_obsolete(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hwrite.name,
                "",
                efp->hwrite.title,
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                efp->family_name,
                NULL,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                efp->hwrite.order,
                update_every);

            ebpf_write_chart_obsolete(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hopen.name,
                "",
                efp->hopen.title,
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                efp->family_name,
                NULL,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                efp->hopen.order,
                update_every);

            ebpf_write_chart_obsolete(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hadditional.name,
                "",
                efp->hadditional.title,
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                efp->family_name,
                NULL,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                efp->hadditional.order,
                update_every);

            ebpf_obsolete_cleanup_struct(efp);
        }
        efp->flags = flags;
    }
}

/**
 * Create Filesystem chart
 *
 * Create latency charts
 *
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_fs_charts(int update_every)
{
    static int order = NETDATA_CHART_PRIO_EBPF_FILESYSTEM_CHARTS;
    char chart_name[64], title[256], family[64], ctx[64];
    int i;
    uint32_t test = NETDATA_FILESYSTEM_FLAG_CHART_CREATED | NETDATA_FILESYSTEM_REMOVE_CHARTS;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        uint32_t flags = efp->flags;
        if (flags & NETDATA_FILESYSTEM_FLAG_HAS_PARTITION && !(flags & test)) {
            snprintfz(title, sizeof(title) - 1, "%s latency for each read request.", efp->filesystem);
            snprintfz(family, sizeof(family) - 1, "%s_latency", efp->family);
            snprintfz(chart_name, sizeof(chart_name) - 1, "%s_read_latency", efp->filesystem);
            snprintfz(ctx, sizeof(ctx) - 1, "filesystem.read_latency");
            efp->hread.name = strdupz(chart_name);
            efp->hread.title = strdupz(title);
            efp->hread.ctx = strdupz(ctx);
            efp->hread.order = order;
            efp->family_name = strdupz(family);

            ebpf_create_chart(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hread.name,
                efp->hread.title,
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                efp->family_name,
                "filesystem.read_latency",
                NETDATA_EBPF_CHART_TYPE_STACKED,
                order,
                ebpf_create_global_dimension,
                filesystem_publish_aggregated,
                NETDATA_EBPF_HIST_MAX_BINS,
                update_every,
                NETDATA_EBPF_MODULE_NAME_FILESYSTEM);
            order++;

            snprintfz(title, sizeof(title) - 1, "%s latency for each write request.", efp->filesystem);
            snprintfz(chart_name, sizeof(chart_name) - 1, "%s_write_latency", efp->filesystem);
            snprintfz(ctx, sizeof(ctx) - 1, "filesystem.write_latency");
            efp->hwrite.name = strdupz(chart_name);
            efp->hwrite.title = strdupz(title);
            efp->hwrite.ctx = strdupz(ctx);
            efp->hwrite.order = order;
            ebpf_create_chart(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hwrite.name,
                efp->hwrite.title,
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                efp->family_name,
                "filesystem.write_latency",
                NETDATA_EBPF_CHART_TYPE_STACKED,
                order,
                ebpf_create_global_dimension,
                filesystem_publish_aggregated,
                NETDATA_EBPF_HIST_MAX_BINS,
                update_every,
                NETDATA_EBPF_MODULE_NAME_FILESYSTEM);
            order++;

            snprintfz(title, sizeof(title) - 1, "%s latency for each open request.", efp->filesystem);
            snprintfz(chart_name, sizeof(chart_name) - 1, "%s_open_latency", efp->filesystem);
            snprintfz(ctx, sizeof(ctx) - 1, "filesystem.open_latency");
            efp->hopen.name = strdupz(chart_name);
            efp->hopen.title = strdupz(title);
            efp->hopen.ctx = strdupz(ctx);
            efp->hopen.order = order;
            ebpf_create_chart(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hopen.name,
                efp->hopen.title,
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                efp->family_name,
                "filesystem.open_latency",
                NETDATA_EBPF_CHART_TYPE_STACKED,
                order,
                ebpf_create_global_dimension,
                filesystem_publish_aggregated,
                NETDATA_EBPF_HIST_MAX_BINS,
                update_every,
                NETDATA_EBPF_MODULE_NAME_FILESYSTEM);
            order++;

            char *type = (efp->flags & NETDATA_FILESYSTEM_ATTR_CHARTS) ? "attribute" : "sync";
            snprintfz(title, sizeof(title) - 1, "%s latency for each %s request.", efp->filesystem, type);
            snprintfz(chart_name, sizeof(chart_name) - 1, "%s_%s_latency", efp->filesystem, type);
            snprintfz(ctx, sizeof(ctx) - 1, "filesystem.%s_latency", efp->filesystem);
            efp->hadditional.name = strdupz(chart_name);
            efp->hadditional.title = strdupz(title);
            efp->hadditional.ctx = strdupz(ctx);
            efp->hadditional.order = order;
            ebpf_create_chart(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hadditional.name,
                efp->hadditional.title,
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                efp->family_name,
                ctx,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                order,
                ebpf_create_global_dimension,
                filesystem_publish_aggregated,
                NETDATA_EBPF_HIST_MAX_BINS,
                update_every,
                NETDATA_EBPF_MODULE_NAME_FILESYSTEM);
            order++;
            efp->flags |= NETDATA_FILESYSTEM_FLAG_CHART_CREATED;
        }
    }

    fflush(stdout);
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
    pthread_mutex_lock(&lock);
    int i;
    const char *saved_name = em->info.thread_name;
    uint64_t kernels = em->kernels;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        if (!efp->probe_links && efp->flags & NETDATA_FILESYSTEM_LOAD_EBPF_PROGRAM) {
            em->info.thread_name = efp->filesystem;
            em->kernels = efp->kernels;
            em->maps = efp->fs_maps;
#ifdef LIBBPF_MAJOR_VERSION
            ebpf_define_map_type(em->maps, em->maps_per_core, running_on_kernel);
#endif
            if (em->load & EBPF_LOAD_LEGACY) {
                efp->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &efp->objects);
                if (!efp->probe_links) {
                    em->info.thread_name = saved_name;
                    em->kernels = kernels;
                    em->maps = NULL;
                    pthread_mutex_unlock(&lock);
                    return -1;
                }
            }
#ifdef LIBBPF_MAJOR_VERSION
            else {
                efp->fs_obj = filesystem_bpf__open();
                if (!efp->fs_obj) {
                    em->info.thread_name = saved_name;
                    em->kernels = kernels;
                    pthread_mutex_unlock(&lock);
                    return -1;
                } else if (ebpf_fs_load_and_attach(em->maps, efp->fs_obj, efp->functions, NULL)) {
                    em->info.thread_name = saved_name;
                    em->kernels = kernels;
                    pthread_mutex_unlock(&lock);
                    return -1;
                }
            }
#endif
            efp->flags |= NETDATA_FILESYSTEM_FLAG_HAS_PARTITION;
            ebpf_update_kernel_memory(&plugin_statistics, efp->fs_maps, EBPF_ACTION_STAT_ADD);

            // Nedeed for filesystems like btrfs
            if ((efp->flags & NETDATA_FILESYSTEM_FILL_ADDRESS_TABLE) && (efp->addresses.function)) {
                ebpf_load_addresses(&efp->addresses, efp->fs_maps[NETDATA_ADDR_FS_TABLE].map_fd);
            }
        }
        efp->flags &= ~NETDATA_FILESYSTEM_LOAD_EBPF_PROGRAM;
    }
    em->info.thread_name = saved_name;
    pthread_mutex_unlock(&lock);
    em->kernels = kernels;
    em->maps = NULL;

    if (!dimensions) {
        dimensions = ebpf_fill_histogram_dimension(NETDATA_EBPF_HIST_MAX_BINS);

        memset(filesystem_aggregated_data, 0, NETDATA_EBPF_HIST_MAX_BINS * sizeof(netdata_syscall_stat_t));
        memset(filesystem_publish_aggregated, 0, NETDATA_EBPF_HIST_MAX_BINS * sizeof(netdata_publish_syscall_t));

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
    if (unlikely(!ff)) {
        snprintfz(filename, FILENAME_MAX, "%s/proc/1/mountinfo", netdata_configured_host_prefix);
        ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
        if (unlikely(!ff))
            return 0;
    }

    ff = procfile_readall(ff);
    if (unlikely(!ff))
        return 0;

    int count = 0;
    unsigned long l, i, lines = procfile_lines(ff);
    for (i = 0; localfs[i].filesystem; i++) {
        localfs[i].flags |= NETDATA_FILESYSTEM_REMOVE_CHARTS;
    }

    for (l = 0; l < lines; l++) {
        // In "normal" situation the expected value is at column 7
        // When `shared` options is added to mount information, the filesystem is at column 8
        // Finally when we have systemd starting netdata, it will be at column 9
        unsigned long index = procfile_linewords(ff, l) - 3;

        char *fs = procfile_lineword(ff, l, index);

        for (i = 0; localfs[i].filesystem; i++) {
            ebpf_filesystem_partitions_t *w = &localfs[i];
            if (w->enabled &&
                (!strcmp(fs, w->filesystem) || (w->optional_filesystem && !strcmp(fs, w->optional_filesystem)))) {
                if (!(localfs[i].flags & NETDATA_FILESYSTEM_FLAG_CHART_CREATED))
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
    static time_t update_every = 0;
    time_t curr = now_realtime_sec();
    if (curr < update_every)
        return 0;

    update_every = curr + 5 * em->update_every;
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
            freez(efp->family_name);
            efp->family_name = NULL;

            freez(efp->hread.name);
            efp->hread.name = NULL;
            freez(efp->hread.title);
            efp->hread.title = NULL;

            freez(efp->hwrite.name);
            efp->hwrite.name = NULL;
            freez(efp->hwrite.title);
            efp->hwrite.title = NULL;

            freez(efp->hopen.name);
            efp->hopen.name = NULL;
            freez(efp->hopen.title);
            efp->hopen.title = NULL;

            freez(efp->hadditional.name);
            efp->hadditional.name = NULL;
            freez(efp->hadditional.title);
            efp->hadditional.title = NULL;
            freez(efp->hadditional.ctx);
            efp->hadditional.ctx = NULL;
        }
    }
}

/**
 * Cleanup FS Histograms
 *
 * @param ptr pointer to structure to be cleaned
 */
static void ebpf_cleanup_fs_histograms(netdata_ebpf_histogram_t *ptr)
{
    freez(ptr->name);
    ptr->name = NULL;

    freez(ptr->title);
    ptr->title = NULL;

    freez(ptr->ctx);
    ptr->ctx = NULL;
}

/**
 * Obsolete global
 *
 * Obsolete global charts created by thread.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_filesystem_global(ebpf_module_t *em)
{
    int i;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        if (!efp->objects)
            continue;

        ebpf_write_chart_obsolete(
            NETDATA_FILESYSTEM_FAMILY,
            efp->hread.name,
            "",
            efp->hread.title,
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            efp->family_name,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "filesystem.read_latency",
            efp->hread.order,
            em->update_every);
        ebpf_cleanup_fs_histograms(&efp->hread);

        ebpf_write_chart_obsolete(
            NETDATA_FILESYSTEM_FAMILY,
            efp->hwrite.name,
            "",
            efp->hwrite.title,
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            efp->family_name,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "filesystem.write_latency",
            efp->hwrite.order,
            em->update_every);
        ebpf_cleanup_fs_histograms(&efp->hwrite);

        ebpf_write_chart_obsolete(
            NETDATA_FILESYSTEM_FAMILY,
            efp->hopen.name,
            "",
            efp->hopen.title,
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            efp->family_name,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "filesystem.open_latency",
            efp->hopen.order,
            em->update_every);
        ebpf_cleanup_fs_histograms(&efp->hopen);

        ebpf_write_chart_obsolete(
            NETDATA_FILESYSTEM_FAMILY,
            efp->hadditional.name,
            "",
            efp->hadditional.title,
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            efp->family_name,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            efp->hadditional.ctx,
            efp->hadditional.order,
            em->update_every);
        ebpf_cleanup_fs_histograms(&efp->hadditional);

        efp->flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION;
    }
}

/**
 * Filesystem exit
 *
 * Cancel child thread.
 *
 * @param ptr thread data.
 */
static void ebpf_filesystem_exit(void *pptr)
{
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        pthread_mutex_lock(&lock);
        ebpf_obsolete_filesystem_global(em);

        fflush(stdout);
        pthread_mutex_unlock(&lock);
    }

    ebpf_filesystem_cleanup_ebpf_data();
    if (dimensions) {
        ebpf_histogram_dimension_cleanup(dimensions, NETDATA_EBPF_HIST_MAX_BINS);
        dimensions = NULL;
    }

    freez(filesystem_hash_values);

    int i;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        if (!efp->probe_links)
            continue;

        ebpf_unload_legacy_code(efp->objects, efp->probe_links);
        efp->objects = NULL;
        efp->probe_links = NULL;
        efp->flags = NETDATA_FILESYSTEM_FLAG_NO_PARTITION;
    }

    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&ebpf_exit_cleanup);
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
    } else if (id < NETDATA_KEY_CALLS_SYNC) {
        *idx = id - NETDATA_KEY_CALLS_OPEN;
        return &efp->hadditional;
    }

    return NULL;
}

/**
 * Read hard disk table
 *
 * @param efp           structure with filesystem monitored
 * @param fd            file descriptor to get data.
 * @param maps_per_core do I need to read all cores?
 *
 * Read the table with number of calls for all functions
 */
static void read_filesystem_table(ebpf_filesystem_partitions_t *efp, int fd, int maps_per_core)
{
    netdata_idx_t *values = filesystem_hash_values;
    uint32_t key;
    uint32_t idx;
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
        int end = (maps_per_core) ? ebpf_nprocs : 1;
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
 * Read the table with number of calls for all functions
 *
 * @param maps_per_core do I need to read all cores?
 */
static void read_filesystem_tables(int maps_per_core)
{
    int i;
    for (i = 0; localfs[i].filesystem; i++) {
        ebpf_filesystem_partitions_t *efp = &localfs[i];
        if (efp->flags & NETDATA_FILESYSTEM_FLAG_HAS_PARTITION) {
            read_filesystem_table(efp, efp->fs_maps[NETDATA_MAIN_FS_TABLE].map_fd, maps_per_core);
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
void ebpf_filesystem_read_hash(ebpf_module_t *em)
{
    (void)ebpf_update_partitions(em);

    ebpf_obsolete_fs_charts(em->update_every);

    if (em->optional)
        return;

    read_filesystem_tables(em->maps_per_core);
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
            write_histogram_chart(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hread.name,
                efp->hread.histogram,
                dimensions,
                NETDATA_EBPF_HIST_MAX_BINS);

            write_histogram_chart(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hwrite.name,
                efp->hwrite.histogram,
                dimensions,
                NETDATA_EBPF_HIST_MAX_BINS);

            write_histogram_chart(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hopen.name,
                efp->hopen.histogram,
                dimensions,
                NETDATA_EBPF_HIST_MAX_BINS);

            write_histogram_chart(
                NETDATA_FILESYSTEM_FAMILY,
                efp->hadditional.name,
                efp->hadditional.histogram,
                dimensions,
                NETDATA_EBPF_HIST_MAX_BINS);
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
    int update_every = em->update_every;
    int counter = update_every - 1;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);

        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        ebpf_filesystem_read_hash(em);
        pthread_mutex_lock(&lock);

        ebpf_create_fs_charts(update_every);
        ebpf_histogram_send_data();

        pthread_mutex_unlock(&lock);

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

        localfs[i].enabled = inicfg_get_boolean(&fs_config, NETDATA_FILESYSTEM_CONFIG_NAME, dist,
                                                   CONFIG_BOOLEAN_YES);
    }
}

/**
 * Set maps
 *
 * When thread is initialized the variable fs_maps is set as null,
 * this function fills the variable before to use.
 */
static void ebpf_set_maps()
{
    localfs[NETDATA_FS_LOCALFS_EXT4].fs_maps = ext4_maps;
    localfs[NETDATA_FS_LOCALFS_XFS].fs_maps = xfs_maps;
    localfs[NETDATA_FS_LOCALFS_NFS].fs_maps = nfs_maps;
    localfs[NETDATA_FS_LOCALFS_ZFS].fs_maps = zfs_maps;
    localfs[NETDATA_FS_LOCALFS_BTRFS].fs_maps = btrfs_maps;
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
void ebpf_filesystem_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(ebpf_filesystem_exit) cleanup_ptr = em;

    ebpf_set_maps();
    ebpf_update_filesystem();

    // Initialize optional as zero, to identify when there are not partitions to monitor
    em->optional = 0;

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_update_partitions(em)) {
        if (em->optional)
            netdata_log_info("Netdata cannot monitor the filesystems used on this host.");

        goto endfilesystem;
    }

    int algorithms[NETDATA_EBPF_HIST_MAX_BINS];
    ebpf_fill_algorithms(algorithms, NETDATA_EBPF_HIST_MAX_BINS, NETDATA_EBPF_INCREMENTAL_IDX);
    ebpf_global_labels(
        filesystem_aggregated_data,
        filesystem_publish_aggregated,
        dimensions,
        dimensions,
        algorithms,
        NETDATA_EBPF_HIST_MAX_BINS);

    pthread_mutex_lock(&lock);
    ebpf_create_fs_charts(em->update_every);
    ebpf_update_stats(&plugin_statistics, em);
    pthread_mutex_unlock(&lock);

    filesystem_collector(em);

endfilesystem:
    ebpf_update_disabled_plugin_stats(em);
}
