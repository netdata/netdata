// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_vfs.h"

static char *vfs_dimension_names[NETDATA_KEY_PUBLISH_VFS_END] = {"delete", "read", "write", "fsync", "open", "create"};
static char *vfs_id_names[NETDATA_KEY_PUBLISH_VFS_END] =
    {"vfs_unlink", "vfs_read", "vfs_write", "vfs_fsync", "vfs_open", "vfs_create"};

static netdata_idx_t *vfs_hash_values = NULL;
static netdata_syscall_stat_t vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_END];
static netdata_publish_syscall_t vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_END];
netdata_ebpf_vfs_t *vfs_vector = NULL;

static ebpf_local_maps_t vfs_maps[] = {
    {.name = "tbl_vfs_pid",
     .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
    },
    {.name = "tbl_vfs_stats",
     .internal_input = NETDATA_VFS_COUNTER,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_STATIC,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = "vfs_ctrl",
     .internal_input = NETDATA_CONTROLLER_END,
     .user_input = 0,
     .type = NETDATA_EBPF_MAP_CONTROLLER,
     .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    },
    {.name = NULL,
     .internal_input = 0,
     .user_input = 0,
#ifdef LIBBPF_MAJOR_VERSION
     .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
    }};

struct netdata_static_thread ebpf_read_vfs = {
    .name = "EBPF_READ_VFS",
    .config_section = NULL,
    .config_name = NULL,
    .env_name = NULL,
    .enabled = 1,
    .thread = NULL,
    .init_routine = NULL,
    .start_routine = NULL};

struct config vfs_config = APPCONFIG_INITIALIZER;

netdata_ebpf_targets_t vfs_targets[] = {
    {.name = "vfs_write", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "vfs_writev", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "vfs_read", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "vfs_readv", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "vfs_unlink", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "vfs_fsync", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "vfs_open", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "vfs_create", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = "release_task", .mode = EBPF_LOAD_TRAMPOLINE},
    {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

#ifdef LIBBPF_MAJOR_VERSION
/**
 * Disable probe
 *
 * Disable all probes to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects
 */
static void ebpf_vfs_disable_probes(struct vfs_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_vfs_write_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_write_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_writev_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_writev_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_read_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_read_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_readv_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_readv_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_unlink_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_unlink_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_fsync_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_fsync_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_open_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_open_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_create_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_create_kretprobe, false);
}

/*
 * Disable trampoline
 *
 * Disable all trampoline to use exclusively another method.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_vfs_disable_trampoline(struct vfs_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_vfs_write_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_write_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_writev_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_writev_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_read_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_read_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_readv_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_readv_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_unlink_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_fsync_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_fsync_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_open_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_open_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_vfs_create_fentry, false);
}

/**
 * Set trampoline target
 *
 * Set the targets we will monitor.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_vfs_set_trampoline_target(struct vfs_bpf *obj)
{
    bpf_program__set_attach_target(obj->progs.netdata_vfs_write_fentry, 0, vfs_targets[NETDATA_EBPF_VFS_WRITE].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_write_fexit, 0, vfs_targets[NETDATA_EBPF_VFS_WRITE].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_writev_fentry, 0, vfs_targets[NETDATA_EBPF_VFS_WRITEV].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_writev_fexit, 0, vfs_targets[NETDATA_EBPF_VFS_WRITEV].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_read_fentry, 0, vfs_targets[NETDATA_EBPF_VFS_READ].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_read_fexit, 0, vfs_targets[NETDATA_EBPF_VFS_READ].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_readv_fentry, 0, vfs_targets[NETDATA_EBPF_VFS_READV].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_readv_fexit, 0, vfs_targets[NETDATA_EBPF_VFS_READV].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_unlink_fentry, 0, vfs_targets[NETDATA_EBPF_VFS_UNLINK].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_fsync_fentry, 0, vfs_targets[NETDATA_EBPF_VFS_FSYNC].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_fsync_fexit, 0, vfs_targets[NETDATA_EBPF_VFS_FSYNC].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_open_fentry, 0, vfs_targets[NETDATA_EBPF_VFS_OPEN].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_open_fexit, 0, vfs_targets[NETDATA_EBPF_VFS_OPEN].name);

    bpf_program__set_attach_target(obj->progs.netdata_vfs_create_fentry, 0, vfs_targets[NETDATA_EBPF_VFS_CREATE].name);
}

/**
 * Attach Probe
 *
 * Attach probes to target
 *
 * @param obj is the main structure for bpf objects.
 *
 * @return It returns 0 on success and -1 otherwise.
 */
static int ebpf_vfs_attach_probe(struct vfs_bpf *obj)
{
    obj->links.netdata_vfs_write_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_write_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_WRITE].name);
    long ret = libbpf_get_error(obj->links.netdata_vfs_write_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_write_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_write_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_WRITE].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_write_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_writev_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_writev_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_WRITEV].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_writev_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_writev_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_writev_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_WRITEV].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_writev_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_read_kprobe =
        bpf_program__attach_kprobe(obj->progs.netdata_vfs_read_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_READ].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_read_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_read_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_read_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_READ].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_read_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_readv_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_readv_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_READV].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_readv_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_readv_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_readv_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_READV].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_readv_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_unlink_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_unlink_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_UNLINK].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_unlink_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_unlink_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_unlink_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_UNLINK].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_unlink_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_fsync_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_fsync_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_FSYNC].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_fsync_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_fsync_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_fsync_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_FSYNC].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_fsync_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_open_kprobe =
        bpf_program__attach_kprobe(obj->progs.netdata_vfs_open_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_OPEN].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_open_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_open_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_open_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_OPEN].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_open_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_create_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_create_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_CREATE].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_create_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_create_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_create_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_CREATE].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_create_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_fsync_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_fsync_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_FSYNC].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_fsync_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_fsync_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_fsync_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_FSYNC].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_fsync_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_open_kprobe =
        bpf_program__attach_kprobe(obj->progs.netdata_vfs_open_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_OPEN].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_open_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_open_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_open_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_OPEN].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_open_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_create_kprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_create_kprobe, false, vfs_targets[NETDATA_EBPF_VFS_CREATE].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_create_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_vfs_create_kretprobe = bpf_program__attach_kprobe(
        obj->progs.netdata_vfs_create_kretprobe, true, vfs_targets[NETDATA_EBPF_VFS_CREATE].name);
    ret = libbpf_get_error(obj->links.netdata_vfs_create_kretprobe);
    if (ret)
        return -1;

    return 0;
}

/**
 * Adjust Size
 *
 * Resize maps according input from users.
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 */
static void ebpf_vfs_adjust_map(struct vfs_bpf *obj, ebpf_module_t *em)
{
    ebpf_update_map_size(obj->maps.tbl_vfs_pid, &vfs_maps[NETDATA_VFS_PID], em, bpf_map__name(obj->maps.tbl_vfs_pid));

    ebpf_update_map_type(obj->maps.tbl_vfs_pid, &vfs_maps[NETDATA_VFS_PID]);
    ebpf_update_map_type(obj->maps.tbl_vfs_stats, &vfs_maps[NETDATA_VFS_ALL]);
    ebpf_update_map_type(obj->maps.vfs_ctrl, &vfs_maps[NETDATA_VFS_CTRL]);
}

/**
 * Set hash tables
 *
 * Set the values for maps according the value given by kernel.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_vfs_set_hash_tables(struct vfs_bpf *obj)
{
    vfs_maps[NETDATA_VFS_ALL].map_fd = bpf_map__fd(obj->maps.tbl_vfs_stats);
    vfs_maps[NETDATA_VFS_PID].map_fd = bpf_map__fd(obj->maps.tbl_vfs_pid);
    vfs_maps[NETDATA_VFS_CTRL].map_fd = bpf_map__fd(obj->maps.vfs_ctrl);
}

/**
 * Load and attach
 *
 * Load and attach the eBPF code in kernel.
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 *
 * @return it returns 0 on success and -1 otherwise
 */
static inline int ebpf_vfs_load_and_attach(struct vfs_bpf *obj, ebpf_module_t *em)
{
    netdata_ebpf_targets_t *mt = em->targets;
    netdata_ebpf_program_loaded_t test = mt[NETDATA_EBPF_VFS_WRITE].mode;

    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_vfs_disable_probes(obj);

        ebpf_vfs_set_trampoline_target(obj);
    } else {
        ebpf_vfs_disable_trampoline(obj);
    }

    ebpf_vfs_adjust_map(obj, em);

    int ret = vfs_bpf__load(obj);
    if (ret) {
        return ret;
    }

    ret = (test == EBPF_LOAD_TRAMPOLINE) ? vfs_bpf__attach(obj) : ebpf_vfs_attach_probe(obj);
    if (!ret) {
        ebpf_vfs_set_hash_tables(obj);

        ebpf_update_controller(vfs_maps[NETDATA_VFS_CTRL].map_fd, em);
    }

    return ret;
}
#endif

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

static void ebpf_obsolete_specific_vfs_charts(char *type, ebpf_module_t *em);

/**
 * Obsolete services
 *
 * Obsolete all service charts created
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_vfs_services(ebpf_module_t *em, char *id)
{
    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_FILE_DELETED,
        "",
        "Files deleted",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_VFS_UNLINK_CONTEXT,
        20065,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS,
        "",
        "Write to disk",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_VFS_WRITE_CONTEXT,
        20066,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            id,
            NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR,
            "",
            "Fails to write",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            NETDATA_SYSTEMD_VFS_WRITE_ERROR_CONTEXT,
            20067,
            em->update_every);
    }

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_VFS_READ_CALLS,
        "",
        "Read from disk",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_VFS_READ_CONTEXT,
        20068,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            id,
            NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR,
            "",
            "Fails to read",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            NETDATA_SYSTEMD_VFS_READ_ERROR_CONTEXT,
            20069,
            em->update_every);
    }

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES,
        "",
        "Bytes written on disk",
        EBPF_COMMON_UNITS_BYTES,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_VFS_WRITE_BYTES_CONTEXT,
        20070,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_VFS_READ_BYTES,
        "",
        "Bytes read from disk",
        EBPF_COMMON_UNITS_BYTES,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_VFS_READ_BYTES_CONTEXT,
        20071,
        em->update_every);

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_VFS_FSYNC,
        "",
        "Calls to vfs_fsync.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_VFS_FSYNC_CONTEXT,
        20072,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            id,
            NETDATA_SYSCALL_APPS_VFS_FSYNC_CALLS_ERROR,
            "",
            "Sync error",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            NETDATA_SYSTEMD_VFS_FSYNC_ERROR_CONTEXT,
            20073,
            em->update_every);
    }

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_VFS_OPEN,
        "",
        "Calls to vfs_open.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_VFS_OPEN_CONTEXT,
        20074,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            id,
            NETDATA_SYSCALL_APPS_VFS_OPEN_CALLS_ERROR,
            "",
            "Open error",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            NETDATA_SYSTEMD_VFS_OPEN_ERROR_CONTEXT,
            20075,
            em->update_every);
    }

    ebpf_write_chart_obsolete(
        id,
        NETDATA_SYSCALL_APPS_VFS_CREATE,
        "",
        "Calls to vfs_create.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_STACKED,
        NETDATA_SYSTEMD_VFS_OPEN_ERROR_CONTEXT,
        20076,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            id,
            NETDATA_SYSCALL_APPS_VFS_CREATE_CALLS_ERROR,
            "",
            "Create error",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            NETDATA_SYSTEMD_VFS_CREATE_ERROR_CONTEXT,
            20077,
            em->update_every);
    }
}

/**
 * Obsolete cgroup chart
 *
 * Send obsolete for all charts created before to close.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static inline void ebpf_obsolete_vfs_cgroup_charts(ebpf_module_t *em)
{
    netdata_mutex_lock(&mutex_cgroup_shm);

    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd) {
            ebpf_obsolete_vfs_services(em, ect->name);

            continue;
        }

        ebpf_obsolete_specific_vfs_charts(ect->name, em);
    }
    netdata_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Obsolette apps charts
 *
 * Obsolete apps charts.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_obsolete_vfs_apps_charts(struct ebpf_module *em)
{
    int order = 20275;
    struct ebpf_target *w;
    int update_every = em->update_every;
    netdata_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_VFS_IDX))))
            continue;

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_unlink",
            "Files deleted.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_unlink",
            order++,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_write",
            "Write to disk.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_write",
            order++,
            update_every);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_obsolete(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_call_vfs_write_error",
                "Fails to write.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_VFS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_call_vfs_write_error",
                order++,
                update_every);
        }

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_read",
            "Read from disk.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_read",
            order++,
            update_every);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_obsolete(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_call_vfs_read_error",
                "Fails to read.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_VFS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_call_vfs_read_error",
                order++,
                update_every);
        }

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_write_bytes",
            "Bytes written on disk.",
            EBPF_COMMON_UNITS_BYTES,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_write_bytes",
            order++,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_read_bytes",
            "Bytes read from disk.",
            EBPF_COMMON_UNITS_BYTES,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_read_bytes",
            order++,
            update_every);

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_fsync",
            "Calls to vfs_fsync.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_fsync",
            order++,
            update_every);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_obsolete(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_call_vfs_fsync_error",
                "Fails to sync.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_VFS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_call_vfs_fsync_error",
                order++,
                update_every);
        }

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_open",
            "Calls to vfs_open.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_open",
            order++,
            update_every);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_obsolete(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_call_vfs_open_error",
                "Fails to open.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_VFS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_call_vfs_open_error",
                order++,
                update_every);
        }

        ebpf_write_chart_obsolete(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_create",
            "Calls to vfs_create.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_create",
            order++,
            update_every);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_obsolete(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_call_vfs_create_error",
                "Fails to create.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_VFS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_call_vfs_create_error",
                order++,
                update_every);
        }
        w->charts_created &= ~(1 << EBPF_MODULE_VFS_IDX);
    }
    netdata_mutex_unlock(&collect_data_mutex);
}

/**
 * Obsolete global
 *
 * Obsolete global charts created by thread.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static void ebpf_obsolete_vfs_global(ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_FILE_CLEAN_COUNT,
        "",
        "Remove files",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "filesystem.vfs_deleted_objects",
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_CLEAN,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_FILE_IO_COUNT,
        "",
        "Calls to IO",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "filesystem.vfs_io",
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_COUNT,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_IO_FILE_BYTES,
        "",
        "Bytes written and read",
        EBPF_COMMON_UNITS_BYTES,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "filesystem.vfs_io_bytes",
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_BYTES,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_FSYNC,
        "",
        "Calls to vfs_fsync.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "filesystem.vfs_fsync",
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_FSYNC,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_OPEN,
        "",
        "Calls to vfs_open.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "filesystem.vfs_open",
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_OPEN,
        em->update_every);

    ebpf_write_chart_obsolete(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_CREATE,
        "",
        "Calls to vfs_create.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        "filesystem.vfs_create",
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_CREATE,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            NETDATA_FILESYSTEM_FAMILY,
            NETDATA_VFS_FILE_ERR_COUNT,
            "",
            "Fails to write or read",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "filesystem.vfs_io_error",
            NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_EBYTES,
            em->update_every);

        ebpf_write_chart_obsolete(
            NETDATA_FILESYSTEM_FAMILY,
            NETDATA_VFS_OPEN_ERR,
            "",
            "Fails to open a file",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "filesystem.vfs_open_error",
            NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_EOPEN,
            em->update_every);

        ebpf_write_chart_obsolete(
            NETDATA_FILESYSTEM_FAMILY,
            NETDATA_VFS_FSYNC_ERR,
            "",
            "Fails to synchronize",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "filesystem.vfs_fsync_error",
            NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_EFSYNC,
            em->update_every);

        ebpf_write_chart_obsolete(
            NETDATA_FILESYSTEM_FAMILY,
            NETDATA_VFS_CREATE_ERR,
            "",
            "Fails to create a file.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            "filesystem.vfs_create_error",
            NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_ECREATE,
            em->update_every);
    }
}

/**
 * Exit
 *
 * Cancel thread and exit.
 *
 * @param ptr thread data.
**/
static void ebpf_vfs_exit(void *pptr)
{
    ebpf_module_t *em = CLEANUP_FUNCTION_GET_PTR(pptr);
    if (!em)
        return;

    netdata_mutex_lock(&lock);
    collect_pids &= ~(1 << EBPF_MODULE_VFS_IDX);
    netdata_mutex_unlock(&lock);

    if (ebpf_read_vfs.thread)
        nd_thread_signal_cancel(ebpf_read_vfs.thread);

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        netdata_mutex_lock(&lock);
        if (em->cgroup_charts) {
            ebpf_obsolete_vfs_cgroup_charts(em);
            fflush(stdout);
        }

        if (em->apps_charts & NETDATA_EBPF_APPS_FLAG_CHART_CREATED) {
            ebpf_obsolete_vfs_apps_charts(em);
        }

        ebpf_obsolete_vfs_global(em);

        fflush(stdout);
        netdata_mutex_unlock(&lock);
    }

    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);

#ifdef LIBBPF_MAJOR_VERSION
    if (vfs_bpf_obj) {
        vfs_bpf__destroy(vfs_bpf_obj);
        vfs_bpf_obj = NULL;
    }
#endif
    if (em->objects) {
        ebpf_unload_legacy_code(em->objects, em->probe_links);
        em->objects = NULL;
        em->probe_links = NULL;
    }

    netdata_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    ebpf_update_stats(&plugin_statistics, em);
    netdata_mutex_unlock(&ebpf_exit_cleanup);
}

/*****************************************************************
 *
 *  FUNCTIONS WITH THE MAIN LOOP
 *
 *****************************************************************/

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em the structure with thread information
*/
static void ebpf_vfs_send_data(ebpf_module_t *em)
{
    netdata_publish_vfs_common_t pvc;

    pvc.write = (long)vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_WRITE].bytes;
    pvc.read = (long)vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_READ].bytes;

    write_count_chart(
        NETDATA_VFS_FILE_CLEAN_COUNT,
        NETDATA_FILESYSTEM_FAMILY,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK],
        1);

    write_count_chart(
        NETDATA_VFS_FILE_IO_COUNT, NETDATA_FILESYSTEM_FAMILY, &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ], 2);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(
            NETDATA_VFS_FILE_ERR_COUNT,
            NETDATA_FILESYSTEM_FAMILY,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ],
            2);
    }

    write_io_chart(
        NETDATA_VFS_IO_FILE_BYTES,
        NETDATA_FILESYSTEM_FAMILY,
        vfs_id_names[NETDATA_KEY_PUBLISH_VFS_WRITE],
        (long long)pvc.write,
        vfs_id_names[NETDATA_KEY_PUBLISH_VFS_READ],
        (long long)pvc.read);

    write_count_chart(
        NETDATA_VFS_FSYNC, NETDATA_FILESYSTEM_FAMILY, &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC], 1);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(
            NETDATA_VFS_FSYNC_ERR, NETDATA_FILESYSTEM_FAMILY, &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC], 1);
    }

    write_count_chart(
        NETDATA_VFS_OPEN, NETDATA_FILESYSTEM_FAMILY, &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN], 1);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(
            NETDATA_VFS_OPEN_ERR, NETDATA_FILESYSTEM_FAMILY, &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN], 1);
    }

    write_count_chart(
        NETDATA_VFS_CREATE, NETDATA_FILESYSTEM_FAMILY, &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE], 1);

    if (em->mode < MODE_ENTRY) {
        write_err_chart(
            NETDATA_VFS_CREATE_ERR,
            NETDATA_FILESYSTEM_FAMILY,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE],
            1);
    }
}

/**
 * Read the hash table and store data to allocated vectors.
 *
 * @param stats         vector used to read data from control table.
 * @param maps_per_core do I need to read all cores?
 */
static void ebpf_vfs_read_global_table(netdata_idx_t *stats, int maps_per_core)
{
    netdata_idx_t res[NETDATA_VFS_COUNTER];
    ebpf_read_global_table_stats(
        res,
        vfs_hash_values,
        vfs_maps[NETDATA_VFS_ALL].map_fd,
        maps_per_core,
        NETDATA_KEY_CALLS_VFS_WRITE,
        NETDATA_VFS_COUNTER);

    ebpf_read_global_table_stats(
        stats,
        vfs_hash_values,
        vfs_maps[NETDATA_VFS_CTRL].map_fd,
        maps_per_core,
        NETDATA_CONTROLLER_PID_TABLE_ADD,
        NETDATA_CONTROLLER_END);

    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK].ncall = res[NETDATA_KEY_CALLS_VFS_UNLINK];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ].ncall =
        res[NETDATA_KEY_CALLS_VFS_READ] + res[NETDATA_KEY_CALLS_VFS_READV];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE].ncall =
        res[NETDATA_KEY_CALLS_VFS_WRITE] + res[NETDATA_KEY_CALLS_VFS_WRITEV];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC].ncall = res[NETDATA_KEY_CALLS_VFS_FSYNC];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN].ncall = res[NETDATA_KEY_CALLS_VFS_OPEN];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE].ncall = res[NETDATA_KEY_CALLS_VFS_CREATE];

    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK].nerr = res[NETDATA_KEY_ERROR_VFS_UNLINK];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ].nerr =
        res[NETDATA_KEY_ERROR_VFS_READ] + res[NETDATA_KEY_ERROR_VFS_READV];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE].nerr =
        res[NETDATA_KEY_ERROR_VFS_WRITE] + res[NETDATA_KEY_ERROR_VFS_WRITEV];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC].nerr = res[NETDATA_KEY_ERROR_VFS_FSYNC];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN].nerr = res[NETDATA_KEY_ERROR_VFS_OPEN];
    vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE].nerr = res[NETDATA_KEY_ERROR_VFS_CREATE];

    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_WRITE].bytes =
        (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITE] + (uint64_t)res[NETDATA_KEY_BYTES_VFS_WRITEV];
    vfs_aggregated_data[NETDATA_KEY_PUBLISH_VFS_READ].bytes =
        (uint64_t)res[NETDATA_KEY_BYTES_VFS_READ] + (uint64_t)res[NETDATA_KEY_BYTES_VFS_READV];
}

/**
 * Set VFS
 *
 * Set vfs structure with values from ebpf structure.
 *
 * @param vfs the output structure.
 * @param w   the input data.
 */
static inline void vfs_aggregate_set_vfs(netdata_publish_vfs_t *vfs, netdata_ebpf_vfs_t *w)
{
    vfs->write_call = w->write_call;
    vfs->writev_call = w->writev_call;
    vfs->read_call = w->read_call;
    vfs->readv_call = w->readv_call;
    vfs->unlink_call = w->unlink_call;
    vfs->fsync_call = w->fsync_call;
    vfs->open_call = w->open_call;
    vfs->create_call = w->create_call;

    vfs->write_bytes = w->write_bytes;
    vfs->writev_bytes = w->writev_bytes;
    vfs->read_bytes = w->read_bytes;
    vfs->readv_bytes = w->readv_bytes;

    vfs->write_err = w->write_err;
    vfs->writev_err = w->writev_err;
    vfs->read_err = w->read_err;
    vfs->readv_err = w->readv_err;
    vfs->unlink_err = w->unlink_err;
    vfs->fsync_err = w->fsync_err;
    vfs->open_err = w->open_err;
    vfs->create_err = w->create_err;
}

/**
 * Aggregate Publish VFS
 *
 * Aggregate data from w source.
 *
 * @param vfs the output structure.
 * @param w   the input data.
 */
static inline void vfs_aggregate_publish_vfs(netdata_publish_vfs_t *vfs, netdata_publish_vfs_t *w)
{
    vfs->write_call += w->write_call;
    vfs->writev_call += w->writev_call;
    vfs->read_call += w->read_call;
    vfs->readv_call += w->readv_call;
    vfs->unlink_call += w->unlink_call;
    vfs->fsync_call += w->fsync_call;
    vfs->open_call += w->open_call;
    vfs->create_call += w->create_call;

    vfs->write_bytes += w->write_bytes;
    vfs->writev_bytes += w->writev_bytes;
    vfs->read_bytes += w->read_bytes;
    vfs->readv_bytes += w->readv_bytes;

    vfs->write_err += w->write_err;
    vfs->writev_err += w->writev_err;
    vfs->read_err += w->read_err;
    vfs->readv_err += w->readv_err;
    vfs->unlink_err += w->unlink_err;
    vfs->fsync_err += w->fsync_err;
    vfs->open_err += w->open_err;
    vfs->create_err += w->create_err;
}

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param swap output structure
 * @param root link list with structure to be used
 */
static void ebpf_vfs_sum_pids(netdata_publish_vfs_t *vfs, struct ebpf_pid_on_target *root)
{
    memset(vfs, 0, sizeof(netdata_publish_vfs_t));

    for (; root; root = root->next) {
        uint32_t pid = root->pid;
        netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(pid, NETDATA_EBPF_PIDS_VFS_IDX);
        if (!local_pid)
            continue;

        netdata_publish_vfs_t *w = &local_pid->vfs;

        vfs_aggregate_publish_vfs(vfs, w);
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em   the structure with thread information
 * @param root the target list.
 */
void ebpf_vfs_send_apps_data(ebpf_module_t *em, struct ebpf_target *root)
{
    struct ebpf_target *w;
    netdata_mutex_lock(&collect_data_mutex);
    for (w = root; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_VFS_IDX))))
            continue;

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_unlink");
        write_chart_dimension("calls", w->vfs.unlink_call);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_write");
        write_chart_dimension("calls", w->vfs.write_call + w->vfs.writev_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_write_error");
            write_chart_dimension("calls", w->vfs.write_err + w->vfs.writev_err);
            ebpf_write_end_chart();
        }

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_read");
        write_chart_dimension("calls", w->vfs.read_call + w->vfs.readv_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_read_error");
            write_chart_dimension("calls", w->vfs.read_err + w->vfs.readv_err);
            ebpf_write_end_chart();
        }

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_write_bytes");
        write_chart_dimension("writes", w->vfs.write_bytes + w->vfs.writev_bytes);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_read_bytes");
        write_chart_dimension("reads", w->vfs.read_bytes + w->vfs.readv_bytes);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_fsync");
        write_chart_dimension("calls", w->vfs.fsync_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_fsync_error");
            write_chart_dimension("calls", w->vfs.fsync_err);
            ebpf_write_end_chart();
        }

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_open");
        write_chart_dimension("calls", w->vfs.open_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_open_error");
            write_chart_dimension("calls", w->vfs.open_err);
            ebpf_write_end_chart();
        }

        ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_create");
        write_chart_dimension("calls", w->vfs.create_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(NETDATA_APP_FAMILY, w->clean_name, "_ebpf_call_vfs_create_error");
            write_chart_dimension("calls", w->vfs.create_err);
            ebpf_write_end_chart();
        }
    }
    netdata_mutex_unlock(&collect_data_mutex);
}

/**
 * Apps Accumulator
 *
 * Sum all values read from kernel and store in the first address.
 *
 * @param out the vector with read values.
 */
static void vfs_apps_accumulator(netdata_ebpf_vfs_t *out, int maps_per_core)
{
    int i, end = (maps_per_core) ? ebpf_nprocs : 1;
    netdata_ebpf_vfs_t *total = &out[0];
    uint64_t ct = total->ct;
    for (i = 1; i < end; i++) {
        netdata_ebpf_vfs_t *w = &out[i];

        total->write_call += w->write_call;
        total->writev_call += w->writev_call;
        total->read_call += w->read_call;
        total->readv_call += w->readv_call;
        total->unlink_call += w->unlink_call;

        total->write_bytes += w->write_bytes;
        total->writev_bytes += w->writev_bytes;
        total->read_bytes += w->read_bytes;
        total->readv_bytes += w->readv_bytes;

        total->write_err += w->write_err;
        total->writev_err += w->writev_err;
        total->read_err += w->read_err;
        total->readv_err += w->readv_err;
        total->unlink_err += w->unlink_err;

        if (w->ct > ct)
            ct = w->ct;

        if (!total->name[0] && w->name[0])
            strncpyz(total->name, w->name, sizeof(total->name) - 1);
    }
}

/**
 * Read the hash table and store data to allocated vectors.
 */
static void ebpf_vfs_read_apps(int maps_per_core)
{
    netdata_ebpf_vfs_t *vv = vfs_vector;
    int fd = vfs_maps[NETDATA_VFS_PID].map_fd;
    size_t length = sizeof(netdata_ebpf_vfs_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    uint32_t key = 0, next_key = 0;
    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        if (bpf_map_lookup_elem(fd, &key, vv)) {
            goto end_vfs_loop;
        }

        vfs_apps_accumulator(vv, maps_per_core);

        netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(key, NETDATA_EBPF_PIDS_VFS_IDX);
        if (!local_pid)
            continue;
        netdata_publish_vfs_t *publish = &local_pid->vfs;

        if (!publish->ct || publish->ct != vv->ct) {
            vfs_aggregate_set_vfs(publish, vv);
        } else {
            if (kill((pid_t)key, 0)) { // No PID found
                if (netdata_ebpf_reset_shm_pointer_unsafe(fd, key, NETDATA_EBPF_PIDS_VFS_IDX))
                    memset(publish, 0, sizeof(*publish));
            }
        }

    end_vfs_loop:
        // We are cleaning to avoid passing data read from one process to other.
        memset(vv, 0, length);
        key = next_key;
    }
}

/**
 * Update cgroup
 *
 * Update cgroup data based in PID.
 *
 * @param maps_per_core do I need to read all cores?
 */
static void read_update_vfs_cgroup()
{
    ebpf_cgroup_target_t *ect;
    netdata_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            uint32_t pid = pids->pid;
            netdata_publish_vfs_t *out = &pids->vfs;
            memset(out, 0, sizeof(netdata_publish_vfs_t));

            netdata_ebpf_pid_stats_t *local_pid = netdata_ebpf_get_shm_pointer_unsafe(pid, NETDATA_EBPF_PIDS_VFS_IDX);
            if (!local_pid)
                continue;
            netdata_publish_vfs_t *in = &local_pid->vfs;

            vfs_aggregate_publish_vfs(out, in);
        }
    }
    netdata_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param vfs  structure used to store data
 * @param pids input data
 */
static void ebpf_vfs_sum_cgroup_pids(netdata_publish_vfs_t *vfs, struct pid_on_target2 *pids)
{
    netdata_publish_vfs_t accumulator;
    memset(&accumulator, 0, sizeof(accumulator));

    while (pids) {
        netdata_publish_vfs_t *w = &pids->vfs;

        accumulator.write_call += w->write_call;
        accumulator.writev_call += w->writev_call;
        accumulator.read_call += w->read_call;
        accumulator.readv_call += w->readv_call;
        accumulator.unlink_call += w->unlink_call;
        accumulator.fsync_call += w->fsync_call;
        accumulator.open_call += w->open_call;
        accumulator.create_call += w->create_call;

        accumulator.write_bytes += w->write_bytes;
        accumulator.writev_bytes += w->writev_bytes;
        accumulator.read_bytes += w->read_bytes;
        accumulator.readv_bytes += w->readv_bytes;

        accumulator.write_err += w->write_err;
        accumulator.writev_err += w->writev_err;
        accumulator.read_err += w->read_err;
        accumulator.readv_err += w->readv_err;
        accumulator.unlink_err += w->unlink_err;
        accumulator.fsync_err += w->fsync_err;
        accumulator.open_err += w->open_err;
        accumulator.create_err += w->create_err;

        pids = pids->next;
    }

    // These conditions were added, because we are using incremental algorithm
    vfs->write_call = (accumulator.write_call >= vfs->write_call) ? accumulator.write_call : vfs->write_call;
    vfs->writev_call = (accumulator.writev_call >= vfs->writev_call) ? accumulator.writev_call : vfs->writev_call;
    vfs->read_call = (accumulator.read_call >= vfs->read_call) ? accumulator.read_call : vfs->read_call;
    vfs->readv_call = (accumulator.readv_call >= vfs->readv_call) ? accumulator.readv_call : vfs->readv_call;
    vfs->unlink_call = (accumulator.unlink_call >= vfs->unlink_call) ? accumulator.unlink_call : vfs->unlink_call;
    vfs->fsync_call = (accumulator.fsync_call >= vfs->fsync_call) ? accumulator.fsync_call : vfs->fsync_call;
    vfs->open_call = (accumulator.open_call >= vfs->open_call) ? accumulator.open_call : vfs->open_call;
    vfs->create_call = (accumulator.create_call >= vfs->create_call) ? accumulator.create_call : vfs->create_call;

    vfs->write_bytes = (accumulator.write_bytes >= vfs->write_bytes) ? accumulator.write_bytes : vfs->write_bytes;
    vfs->writev_bytes = (accumulator.writev_bytes >= vfs->writev_bytes) ? accumulator.writev_bytes : vfs->writev_bytes;
    vfs->read_bytes = (accumulator.read_bytes >= vfs->read_bytes) ? accumulator.read_bytes : vfs->read_bytes;
    vfs->readv_bytes = (accumulator.readv_bytes >= vfs->readv_bytes) ? accumulator.readv_bytes : vfs->readv_bytes;

    vfs->write_err = (accumulator.write_err >= vfs->write_err) ? accumulator.write_err : vfs->write_err;
    vfs->writev_err = (accumulator.writev_err >= vfs->writev_err) ? accumulator.writev_err : vfs->writev_err;
    vfs->read_err = (accumulator.read_err >= vfs->read_err) ? accumulator.read_err : vfs->read_err;
    vfs->readv_err = (accumulator.readv_err >= vfs->readv_err) ? accumulator.readv_err : vfs->readv_err;
    vfs->unlink_err = (accumulator.unlink_err >= vfs->unlink_err) ? accumulator.unlink_err : vfs->unlink_err;
    vfs->fsync_err = (accumulator.fsync_err >= vfs->fsync_err) ? accumulator.fsync_err : vfs->fsync_err;
    vfs->open_err = (accumulator.open_err >= vfs->open_err) ? accumulator.open_err : vfs->open_err;
    vfs->create_err = (accumulator.create_err >= vfs->create_err) ? accumulator.create_err : vfs->create_err;
}

/**
 * Create specific VFS charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param em   the main thread structure.
 */
static void ebpf_create_specific_vfs_charts(char *type, ebpf_module_t *em)
{
    char *label = (!strncmp(type, "cgroup_", 7)) ? &type[7] : type;
    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_FILE_DELETED,
        "Files deleted",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_CGROUP_VFS_UNLINK_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5500,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS,
        "Write to disk",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_CGROUP_VFS_WRITE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5501,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            type,
            NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR,
            "Fails to write",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_CGROUP_VFS_WRITE_ERROR_CONTEXT,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5502,
            ebpf_create_global_dimension,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
    }

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_VFS_READ_CALLS,
        "Read from disk",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_CGROUP_VFS_READ_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5503,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            type,
            NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR,
            "Fails to read",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_CGROUP_VFS_READ_ERROR_CONTEXT,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5504,
            ebpf_create_global_dimension,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
    }

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES,
        "Bytes written on disk",
        EBPF_COMMON_UNITS_BYTES,
        NETDATA_VFS_GROUP,
        NETDATA_CGROUP_VFS_WRITE_BYTES_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5505,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_VFS_READ_BYTES,
        "Bytes read from disk",
        EBPF_COMMON_UNITS_BYTES,
        NETDATA_VFS_GROUP,
        NETDATA_CGROUP_VFS_READ_BYTES_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5506,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_VFS_FSYNC,
        "Calls to vfs_fsync.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_CGROUP_VFS_FSYNC_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5507,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            type,
            NETDATA_SYSCALL_APPS_VFS_FSYNC_CALLS_ERROR,
            "Sync error",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_CGROUP_VFS_FSYNC_ERROR_CONTEXT,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5508,
            ebpf_create_global_dimension,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
    }

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_VFS_OPEN,
        "Calls to vfs_open.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_CGROUP_VFS_OPEN_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5509,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            type,
            NETDATA_SYSCALL_APPS_VFS_OPEN_CALLS_ERROR,
            "Open error",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_CGROUP_VFS_OPEN_ERROR_CONTEXT,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5510,
            ebpf_create_global_dimension,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
    }

    ebpf_create_chart(
        type,
        NETDATA_SYSCALL_APPS_VFS_CREATE,
        "Calls to vfs_create.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_CGROUP_VFS_CREATE_CONTEXT,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5511,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);
    ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
    ebpf_commit_label();

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            type,
            NETDATA_SYSCALL_APPS_VFS_CREATE_CALLS_ERROR,
            "Create error",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_CGROUP_VFS_CREATE_ERROR_CONTEXT,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5512,
            ebpf_create_global_dimension,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("cgroup_name", label, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
    }
}

/**
 * Obsolete specific VFS charts
 *
 * Obsolete charts for cgroup/application.
 *
 * @param type the chart type.
 * @param em   the main thread structure.
 */
static void ebpf_obsolete_specific_vfs_charts(char *type, ebpf_module_t *em)
{
    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_FILE_DELETED,
        "",
        "Files deleted",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_VFS_UNLINK_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5500,
        em->update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS,
        "",
        "Write to disk",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_VFS_WRITE_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5501,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            type,
            NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR,
            "",
            "Fails to write",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CGROUP_VFS_WRITE_ERROR_CONTEXT,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5502,
            em->update_every);
    }

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_VFS_READ_CALLS,
        "",
        "Read from disk",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_VFS_READ_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5503,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            type,
            NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR,
            "",
            "Fails to read",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CGROUP_VFS_READ_ERROR_CONTEXT,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5504,
            em->update_every);
    }

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES,
        "",
        "Bytes written on disk",
        EBPF_COMMON_UNITS_BYTES,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_VFS_WRITE_BYTES_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5505,
        em->update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_VFS_READ_BYTES,
        "",
        "Bytes read from disk",
        EBPF_COMMON_UNITS_BYTES,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_VFS_READ_BYTES_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5506,
        em->update_every);

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_VFS_FSYNC,
        "",
        "Calls to vfs_fsync.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_VFS_FSYNC_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5507,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            type,
            NETDATA_SYSCALL_APPS_VFS_FSYNC_CALLS_ERROR,
            "",
            "Sync error",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CGROUP_VFS_FSYNC_ERROR_CONTEXT,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5508,
            em->update_every);
    }

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_VFS_OPEN,
        "",
        "Calls to vfs_open.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_VFS_OPEN_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5509,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            type,
            NETDATA_SYSCALL_APPS_VFS_OPEN_CALLS_ERROR,
            "",
            "Open error",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CGROUP_VFS_OPEN_ERROR_CONTEXT,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5510,
            em->update_every);
    }

    ebpf_write_chart_obsolete(
        type,
        NETDATA_SYSCALL_APPS_VFS_CREATE,
        "",
        "Calls to vfs_create.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CGROUP_VFS_CREATE_CONTEXT,
        NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5511,
        em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(
            type,
            NETDATA_SYSCALL_APPS_VFS_CREATE_CALLS_ERROR,
            "",
            "Create error",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CGROUP_VFS_CREATE_ERROR_CONTEXT,
            NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + 5512,
            em->update_every);
    }
}

/*
 * Send specific VFS data
 *
 * Send data for specific cgroup/apps.
 *
 * @param type   chart type
 * @param values structure with values that will be sent to netdata
 */
static void ebpf_send_specific_vfs_data(char *type, netdata_publish_vfs_t *values, ebpf_module_t *em)
{
    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_FILE_DELETED, "");
    write_chart_dimension(vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK].name, (long long)values->unlink_call);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS, "");
    write_chart_dimension(
        vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE].name,
        (long long)values->write_call + (long long)values->writev_call);
    ebpf_write_end_chart();

    if (em->mode < MODE_ENTRY) {
        ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR, "");
        write_chart_dimension(
            vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE].name,
            (long long)values->write_err + (long long)values->writev_err);
        ebpf_write_end_chart();
    }

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_READ_CALLS, "");
    write_chart_dimension(
        vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ].name,
        (long long)values->read_call + (long long)values->readv_call);
    ebpf_write_end_chart();

    if (em->mode < MODE_ENTRY) {
        ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR, "");
        write_chart_dimension(
            vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ].name,
            (long long)values->read_err + (long long)values->readv_err);
        ebpf_write_end_chart();
    }

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES, "");
    write_chart_dimension(
        vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_WRITE].name,
        (long long)values->write_bytes + (long long)values->writev_bytes);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_READ_BYTES, "");
    write_chart_dimension(
        vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ].name,
        (long long)values->read_bytes + (long long)values->readv_bytes);
    ebpf_write_end_chart();

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_FSYNC, "");
    write_chart_dimension(vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC].name, (long long)values->fsync_call);
    ebpf_write_end_chart();

    if (em->mode < MODE_ENTRY) {
        ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_FSYNC_CALLS_ERROR, "");
        write_chart_dimension(vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC].name, (long long)values->fsync_err);
        ebpf_write_end_chart();
    }

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_OPEN, "");
    write_chart_dimension(vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN].name, (long long)values->open_call);
    ebpf_write_end_chart();

    if (em->mode < MODE_ENTRY) {
        ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_OPEN_CALLS_ERROR, "");
        write_chart_dimension(vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN].name, (long long)values->open_err);
        ebpf_write_end_chart();
    }

    ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_CREATE, "");
    write_chart_dimension(vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE].name, (long long)values->create_call);
    ebpf_write_end_chart();

    if (em->mode < MODE_ENTRY) {
        ebpf_write_begin_chart(type, NETDATA_SYSCALL_APPS_VFS_CREATE_CALLS_ERROR, "");
        write_chart_dimension(
            vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE].name, (long long)values->create_err);
        ebpf_write_end_chart();
    }
}

/**
 *  Create Systemd Socket Charts
 *
 *  Create charts when systemd is enabled
 *
 *  @param em the main collector structure
 **/
static void ebpf_create_systemd_vfs_charts(ebpf_module_t *em)
{
    static ebpf_systemd_args_t data_vfs_unlink = {
        .title = "Files deleted",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20065,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_UNLINK_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_FILE_DELETED,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_vfs_write = {
        .title = "Write to disk",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20066,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_WRITE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_vfs_write_err = {
        .title = "Fails to write",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20067,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_WRITE_ERROR_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_vfs_read = {
        .title = "Read from disk",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20068,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_READ_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_READ_CALLS,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_vfs_read_err = {
        .title = "Fails to read",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20069,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_READ_ERROR_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_vfs_write_bytes = {
        .title = "Bytes written on disk",
        .units = EBPF_COMMON_UNITS_BYTES,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20070,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_WRITE_BYTES_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES,
        .dimension = "bytes"};

    static ebpf_systemd_args_t data_vfs_read_bytes = {
        .title = "Bytes read from disk",
        .units = EBPF_COMMON_UNITS_BYTES,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20071,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_READ_BYTES_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_READ_BYTES,
        .dimension = "bytes"};

    static ebpf_systemd_args_t data_vfs_fsync = {
        .title = "Calls to vfs_fsync.",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20072,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_FSYNC_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_FSYNC,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_vfs_fsync_err = {
        .title = "Sync error",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20073,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_FSYNC_ERROR_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_FSYNC_CALLS_ERROR,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_vfs_open = {
        .title = "Calls to vfs_open.",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20074,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_OPEN_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_OPEN,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_vfs_open_err = {
        .title = "Open error",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20075,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_OPEN_ERROR_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_OPEN_CALLS_ERROR,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_vfs_create = {
        .title = "Calls to vfs_create.",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20076,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_CREATE_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_CREATE,
        .dimension = "calls"};

    static ebpf_systemd_args_t data_vfs_create_err = {
        .title = "Create error",
        .units = EBPF_COMMON_UNITS_CALLS_PER_SEC,
        .family = NETDATA_VFS_GROUP,
        .charttype = NETDATA_EBPF_CHART_TYPE_STACKED,
        .order = 20077,
        .algorithm = EBPF_CHART_ALGORITHM_INCREMENTAL,
        .context = NETDATA_SYSTEMD_VFS_CREATE_ERROR_CONTEXT,
        .module = NETDATA_EBPF_MODULE_NAME_VFS,
        .update_every = 0,
        .suffix = NETDATA_SYSCALL_APPS_VFS_CREATE_CALLS_ERROR,
        .dimension = "calls"};

    if (!data_vfs_create.update_every)
        data_vfs_unlink.update_every = data_vfs_write.update_every = data_vfs_write_err.update_every =
            data_vfs_read.update_every = data_vfs_read_err.update_every = data_vfs_write_bytes.update_every =
                data_vfs_read_bytes.update_every = data_vfs_fsync.update_every = data_vfs_fsync_err.update_every =
                    data_vfs_open.update_every = data_vfs_open_err.update_every = data_vfs_create.update_every =
                        data_vfs_create_err.update_every = em->update_every;

    ebpf_cgroup_target_t *w;
    for (w = ebpf_cgroup_pids; w; w = w->next) {
        if (unlikely(!w->systemd || w->flags & NETDATA_EBPF_SERVICES_HAS_VFS_CHART))
            continue;

        data_vfs_unlink.id = data_vfs_write.id = data_vfs_write_err.id = data_vfs_read.id = data_vfs_read_err.id =
            data_vfs_write_bytes.id = data_vfs_read_bytes.id = data_vfs_fsync.id = data_vfs_fsync_err.id =
                data_vfs_open.id = data_vfs_open_err.id = data_vfs_create.id = data_vfs_create_err.id = w->name;
        ebpf_create_charts_on_systemd(&data_vfs_unlink);

        ebpf_create_charts_on_systemd(&data_vfs_write);

        ebpf_create_charts_on_systemd(&data_vfs_read);

        ebpf_create_charts_on_systemd(&data_vfs_write_bytes);

        ebpf_create_charts_on_systemd(&data_vfs_read_bytes);

        ebpf_create_charts_on_systemd(&data_vfs_fsync);

        ebpf_create_charts_on_systemd(&data_vfs_open);

        ebpf_create_charts_on_systemd(&data_vfs_create);
        if (em->mode < MODE_ENTRY) {
            ebpf_create_charts_on_systemd(&data_vfs_write_err);
            ebpf_create_charts_on_systemd(&data_vfs_read_err);
            ebpf_create_charts_on_systemd(&data_vfs_fsync_err);
            ebpf_create_charts_on_systemd(&data_vfs_open_err);
            ebpf_create_charts_on_systemd(&data_vfs_create_err);
        }

        w->flags |= NETDATA_EBPF_SERVICES_HAS_VFS_CHART;
    }
}

/**
 * Send Systemd charts
 *
 * Send collected data to Netdata.
 *
 *  @param em the main collector structure
 */
static void ebpf_send_systemd_vfs_charts(ebpf_module_t *em)
{
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (unlikely(!(ect->flags & NETDATA_EBPF_SERVICES_HAS_VFS_CHART))) {
            continue;
        }

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_FILE_DELETED, "");
        write_chart_dimension("calls", ect->publish_systemd_vfs.unlink_call);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS, "");
        write_chart_dimension("calls", ect->publish_systemd_vfs.write_call + ect->publish_systemd_vfs.writev_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_WRITE_CALLS_ERROR, "");
            write_chart_dimension("calls", ect->publish_systemd_vfs.write_err + ect->publish_systemd_vfs.writev_err);
            ebpf_write_end_chart();
        }

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_READ_CALLS, "");
        write_chart_dimension("calls", ect->publish_systemd_vfs.read_call + ect->publish_systemd_vfs.readv_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_READ_CALLS_ERROR, "");
            write_chart_dimension("calls", ect->publish_systemd_vfs.read_err + ect->publish_systemd_vfs.readv_err);
            ebpf_write_end_chart();
        }

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_WRITE_BYTES, "");
        write_chart_dimension("bytes", ect->publish_systemd_vfs.write_bytes + ect->publish_systemd_vfs.writev_bytes);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_READ_BYTES, "");
        write_chart_dimension("bytes", ect->publish_systemd_vfs.read_bytes + ect->publish_systemd_vfs.readv_bytes);
        ebpf_write_end_chart();

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_FSYNC, "");
        write_chart_dimension("calls", ect->publish_systemd_vfs.fsync_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_FSYNC_CALLS_ERROR, "");
            write_chart_dimension("calls", ect->publish_systemd_vfs.fsync_err);
            ebpf_write_end_chart();
        }

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_OPEN, "");
        write_chart_dimension("calls", ect->publish_systemd_vfs.open_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_OPEN_CALLS_ERROR, "");
            write_chart_dimension("calls", ect->publish_systemd_vfs.open_err);
            ebpf_write_end_chart();
        }

        ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_CREATE, "");
        write_chart_dimension("calls", ect->publish_systemd_vfs.create_call);
        ebpf_write_end_chart();

        if (em->mode < MODE_ENTRY) {
            ebpf_write_begin_chart(ect->name, NETDATA_SYSCALL_APPS_VFS_CREATE_CALLS_ERROR, "");
            write_chart_dimension("calls", ect->publish_systemd_vfs.create_err);
            ebpf_write_end_chart();
        }
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em the main collector structure
*/
static void ebpf_vfs_send_cgroup_data(ebpf_module_t *em)
{
    netdata_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        ebpf_vfs_sum_cgroup_pids(&ect->publish_systemd_vfs, ect->pids);
    }

    if (shm_ebpf_cgroup.header->systemd_enabled) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_vfs_charts(em);
        }
        ebpf_send_systemd_vfs_charts(em);
    }

    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_VFS_CHART) && ect->updated) {
            ebpf_create_specific_vfs_charts(ect->name, em);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_VFS_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_VFS_CHART) {
            if (ect->updated) {
                ebpf_send_specific_vfs_data(ect->name, &ect->publish_systemd_vfs, em);
            } else {
                ebpf_obsolete_specific_vfs_charts(ect->name, em);
                ect->flags &= ~NETDATA_EBPF_CGROUP_HAS_VFS_CHART;
            }
        }
    }

    netdata_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Resume apps data
 */
void ebpf_vfs_resume_apps_data()
{
    struct ebpf_target *w;
    netdata_mutex_lock(&collect_data_mutex);
    for (w = apps_groups_root_target; w; w = w->next) {
        if (unlikely(!(w->charts_created & (1 << EBPF_MODULE_VFS_IDX))))
            continue;

        ebpf_vfs_sum_pids(&w->vfs, w->root_pid);
    }
    netdata_mutex_unlock(&collect_data_mutex);
}

/**
 * VFS thread
 *
 * Thread used to generate  charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void ebpf_read_vfs_thread(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    int maps_per_core = em->maps_per_core;
    int update_every = em->update_every;
    int collect_pid = (em->apps_charts || em->cgroup_charts);
    if (!collect_pid)
        return;

    int counter = update_every - 1;

    uint32_t lifetime = em->lifetime;
    int cgroups = em->cgroup_charts;
    uint32_t running_time = 0;
    pids_fd[NETDATA_EBPF_PIDS_VFS_IDX] = vfs_maps[NETDATA_VFS_PID].map_fd;
    heartbeat_t hb;
    heartbeat_init(&hb, update_every * USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        sem_wait(shm_mutex_ebpf_integration);
        ebpf_vfs_read_apps(maps_per_core);
        ebpf_vfs_resume_apps_data();
        if (cgroups && shm_ebpf_cgroup.header)
            read_update_vfs_cgroup();

        sem_post(shm_mutex_ebpf_integration);

        counter = 0;

        netdata_mutex_lock(&ebpf_exit_cleanup);
        if (running_time && !em->running_time)
            running_time = update_every;
        else
            running_time += update_every;

        em->running_time = running_time;
        netdata_mutex_unlock(&ebpf_exit_cleanup);
    }
}

/**
 * Main loop for this collector.
 *
 * @param step the number of microseconds used with heart beat
 * @param em   the structure with thread information
 */
static void vfs_collector(ebpf_module_t *em)
{
    int cgroups = em->cgroup_charts;
    int update_every = em->update_every;
    int counter = update_every - 1;
    int maps_per_core = em->maps_per_core;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    netdata_idx_t *stats = em->hash_table_stats;
    memset(stats, 0, sizeof(em->hash_table_stats));
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while (!ebpf_plugin_stop() && running_time < lifetime) {
        heartbeat_next(&hb);
        if (ebpf_plugin_stop() || ++counter != update_every)
            continue;

        counter = 0;
        netdata_apps_integration_flags_t apps = em->apps_charts;
        ebpf_vfs_read_global_table(stats, maps_per_core);

        netdata_mutex_lock(&lock);

        ebpf_vfs_send_data(em);
        fflush(stdout);

        if (apps & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
            ebpf_vfs_send_apps_data(em, apps_groups_root_target);

        if (cgroups && shm_ebpf_cgroup.header)
            ebpf_vfs_send_cgroup_data(em);

        netdata_mutex_unlock(&lock);

        netdata_mutex_lock(&ebpf_exit_cleanup);
        if (running_time && !em->running_time)
            running_time = update_every;
        else
            running_time += update_every;

        em->running_time = running_time;
        netdata_mutex_unlock(&ebpf_exit_cleanup);
    }
}

/*****************************************************************
 *
 *  FUNCTIONS TO CREATE CHARTS
 *
 *****************************************************************/

/**
 * Create IO chart
 *
 * @param family the chart family
 * @param name   the chart name
 * @param axis   the axis label
 * @param web    the group name used to attach the chart on dashboard
 * @param order  the order number of the specified chart
 * @param algorithm the algorithm used to make the charts.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void
ebpf_create_io_chart(char *family, char *name, char *axis, char *web, int order, int algorithm, int update_every)
{
    printf(
        "CHART %s.%s '' 'Bytes written and read' '%s' '%s' 'filesystem.vfs_io_bytes' line %d %d '' 'ebpf.plugin' 'filesystem'\n",
        family,
        name,
        axis,
        web,
        order,
        update_every);

    printf(
        "DIMENSION %s %s %s 1 1\n",
        vfs_id_names[NETDATA_KEY_PUBLISH_VFS_READ],
        vfs_dimension_names[NETDATA_KEY_PUBLISH_VFS_READ],
        ebpf_algorithms[algorithm]);
    printf(
        "DIMENSION %s %s %s -1 1\n",
        vfs_id_names[NETDATA_KEY_PUBLISH_VFS_WRITE],
        vfs_dimension_names[NETDATA_KEY_PUBLISH_VFS_WRITE],
        ebpf_algorithms[algorithm]);
}

/**
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param em a pointer to the structure with the default values.
 */
static void ebpf_create_global_charts(ebpf_module_t *em)
{
    ebpf_create_chart(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_FILE_CLEAN_COUNT,
        "Remove files",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        "filesystem.vfs_deleted_objects",
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_CLEAN,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_UNLINK],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);

    ebpf_create_chart(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_FILE_IO_COUNT,
        "Calls to IO",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        "filesystem.vfs_io",
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_COUNT,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ],
        2,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);

    ebpf_create_io_chart(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_IO_FILE_BYTES,
        EBPF_COMMON_UNITS_BYTES,
        NETDATA_VFS_GROUP,
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_BYTES,
        NETDATA_EBPF_INCREMENTAL_IDX,
        em->update_every);

    ebpf_create_chart(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_FSYNC,
        "Calls to vfs_fsync.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        "filesystem.vfs_fsync",
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_FSYNC,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);

    ebpf_create_chart(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_OPEN,
        "Calls to vfs_open.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        "filesystem.vfs_open",
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_OPEN,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);

    ebpf_create_chart(
        NETDATA_FILESYSTEM_FAMILY,
        NETDATA_VFS_CREATE,
        "Calls to vfs_create.",
        EBPF_COMMON_UNITS_CALLS_PER_SEC,
        NETDATA_VFS_GROUP,
        "filesystem.vfs_create",
        NETDATA_EBPF_CHART_TYPE_LINE,
        NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_CREATE,
        ebpf_create_global_dimension,
        &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE],
        1,
        em->update_every,
        NETDATA_EBPF_MODULE_NAME_VFS);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(
            NETDATA_FILESYSTEM_FAMILY,
            NETDATA_VFS_FILE_ERR_COUNT,
            "Fails to write or read",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            "filesystem.vfs_io_error",
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_EBYTES,
            ebpf_create_global_dimension,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_READ],
            2,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);

        ebpf_create_chart(
            NETDATA_FILESYSTEM_FAMILY,
            NETDATA_VFS_FSYNC_ERR,
            "Fails to synchronize",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            "filesystem.vfs_fsync_error",
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_EFSYNC,
            ebpf_create_global_dimension,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_FSYNC],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);

        ebpf_create_chart(
            NETDATA_FILESYSTEM_FAMILY,
            NETDATA_VFS_OPEN_ERR,
            "Fails to open a file",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            "filesystem.vfs_open_error",
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_EOPEN,
            ebpf_create_global_dimension,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_OPEN],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);

        ebpf_create_chart(
            NETDATA_FILESYSTEM_FAMILY,
            NETDATA_VFS_CREATE_ERR,
            "Fails to create a file.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            "filesystem.vfs_create_error",
            NETDATA_EBPF_CHART_TYPE_LINE,
            NETDATA_CHART_PRIO_FILESYSTEM_VFS_IO_ECREATE,
            ebpf_create_global_dimension,
            &vfs_publish_aggregated[NETDATA_KEY_PUBLISH_VFS_CREATE],
            1,
            em->update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
    }

    fflush(stdout);
}

/**
 * Create process apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em   a pointer to the structure with the default values.
 * @param ptr  a pointer for the targets.
 **/
void ebpf_vfs_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    struct ebpf_target *root = ptr;
    struct ebpf_target *w;
    int order = 20275;
    int update_every = em->update_every;
    for (w = root; w; w = w->next) {
        if (unlikely(!w->exposed))
            continue;

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_unlink",
            "Files deleted.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_unlink",
            order++,
            update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_write",
            "Write to disk.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_write",
            order++,
            update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_cmd(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_call_vfs_write_error",
                "Fails to write.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_VFS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_call_vfs_write_error",
                order++,
                update_every,
                NETDATA_EBPF_MODULE_NAME_VFS);
            ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
            ebpf_commit_label();
            fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        }

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_read",
            "Read from disk.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_read",
            order++,
            update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_cmd(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_call_vfs_read_error",
                "Fails to read.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_VFS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_call_vfs_read_error",
                order++,
                update_every,
                NETDATA_EBPF_MODULE_NAME_VFS);
            ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
            ebpf_commit_label();
            fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        }

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_write_bytes",
            "Bytes written on disk.",
            EBPF_COMMON_UNITS_BYTES,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_write_bytes",
            order++,
            update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION writes '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_read_bytes",
            "Bytes read from disk.",
            EBPF_COMMON_UNITS_BYTES,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_read_bytes",
            order++,
            update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION reads '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_fsync",
            "Calls to vfs_fsync.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_fsync",
            order++,
            update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_cmd(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_call_vfs_fsync_error",
                "Fails to sync.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_VFS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_call_vfs_fsync_error",
                order++,
                update_every,
                NETDATA_EBPF_MODULE_NAME_VFS);
            ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
            ebpf_commit_label();
            fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        }

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_open",
            "Calls to vfs_open.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_open",
            order++,
            update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_cmd(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_call_vfs_open_error",
                "Fails to open.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_VFS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_call_vfs_open_error",
                order++,
                update_every,
                NETDATA_EBPF_MODULE_NAME_VFS);
            ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
            ebpf_commit_label();
            fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        }

        ebpf_write_chart_cmd(
            NETDATA_APP_FAMILY,
            w->clean_name,
            "_ebpf_call_vfs_create",
            "Calls to vfs_create.",
            EBPF_COMMON_UNITS_CALLS_PER_SEC,
            NETDATA_VFS_GROUP,
            NETDATA_EBPF_CHART_TYPE_STACKED,
            "app.ebpf_call_vfs_create",
            order++,
            update_every,
            NETDATA_EBPF_MODULE_NAME_VFS);
        ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
        ebpf_commit_label();
        fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);

        if (em->mode < MODE_ENTRY) {
            ebpf_write_chart_cmd(
                NETDATA_APP_FAMILY,
                w->clean_name,
                "_ebpf_call_vfs_create_error",
                "Fails to create a file.",
                EBPF_COMMON_UNITS_CALLS_PER_SEC,
                NETDATA_VFS_GROUP,
                NETDATA_EBPF_CHART_TYPE_STACKED,
                "app.ebpf_call_vfs_create_error",
                order++,
                update_every,
                NETDATA_EBPF_MODULE_NAME_VFS);
            ebpf_create_chart_labels("app_group", w->name, RRDLABEL_SRC_AUTO);
            ebpf_commit_label();
            fprintf(stdout, "DIMENSION calls '' %s 1 1\n", ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX]);
        }

        w->charts_created |= 1 << EBPF_MODULE_VFS_IDX;
    }

    em->apps_charts |= NETDATA_EBPF_APPS_FLAG_CHART_CREATED;
}

/*****************************************************************
 *
 *  FUNCTIONS TO START THREAD
 *
 *****************************************************************/

/**
 * Allocate vectors used with this thread.
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 *
 *  @param apps is apps enabled?
 */
static void ebpf_vfs_allocate_global_vectors()
{
    vfs_vector = callocz(ebpf_nprocs, sizeof(netdata_ebpf_vfs_t));

    memset(vfs_aggregated_data, 0, sizeof(vfs_aggregated_data));
    memset(vfs_publish_aggregated, 0, sizeof(vfs_publish_aggregated));

    vfs_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));
}

/*****************************************************************
 *
 *  EBPF VFS THREAD
 *
 *****************************************************************/

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 */
static int ebpf_vfs_load_bpf(ebpf_module_t *em)
{
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(em->maps, em->maps_per_core, running_on_kernel);
#endif

    int ret = 0;
    ebpf_adjust_apps_cgroup(em, em->targets[NETDATA_EBPF_VFS_WRITE].mode);
    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
        if (!em->probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        vfs_bpf_obj = vfs_bpf__open();
        if (!vfs_bpf_obj)
            ret = -1;
        else
            ret = ebpf_vfs_load_and_attach(vfs_bpf_obj, em);
    }
#endif

    return ret;
}

/**
 * Process thread
 *
 * Thread used to generate process charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void ebpf_vfs_thread(void *ptr)
{
    pids_fd[NETDATA_EBPF_PIDS_VFS_IDX] = -1;
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    CLEANUP_FUNCTION_REGISTER(ebpf_vfs_exit) cleanup_ptr = em;

    em->maps = vfs_maps;

    ebpf_update_pid_table(&vfs_maps[NETDATA_VFS_PID], em);

    ebpf_vfs_allocate_global_vectors();

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_vfs_load_bpf(em)) {
        goto endvfs;
    }

    int algorithms[NETDATA_KEY_PUBLISH_VFS_END] = {
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX};

    ebpf_global_labels(
        vfs_aggregated_data,
        vfs_publish_aggregated,
        vfs_dimension_names,
        vfs_id_names,
        algorithms,
        NETDATA_KEY_PUBLISH_VFS_END);

    netdata_mutex_lock(&lock);
    ebpf_create_global_charts(em);
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);

    netdata_mutex_unlock(&lock);

    ebpf_read_vfs.thread =
        nd_thread_create(ebpf_read_vfs.name, NETDATA_THREAD_OPTION_DEFAULT, ebpf_read_vfs_thread, em);

    vfs_collector(em);

endvfs:
    ebpf_update_disabled_plugin_stats(em);
}
