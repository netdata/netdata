// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_mount.h"

static ebpf_local_maps_t mount_maps[] = {{.name = "tbl_mount", .internal_input = NETDATA_MOUNT_END,
                                          .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                          .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED},
                                         {.name = NULL, .internal_input = 0, .user_input = 0,
                                          .type = NETDATA_EBPF_MAP_CONTROLLER,
                                          .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED}};

static ebpf_data_t mount_data;
static char *mount_dimension_name[NETDATA_EBPF_MOUNT_SYSCALL] = { "mount", "umount" };
static netdata_syscall_stat_t mount_aggregated_data[NETDATA_EBPF_MOUNT_SYSCALL];
static netdata_publish_syscall_t mount_publish_aggregated[NETDATA_EBPF_MOUNT_SYSCALL];

struct config mount_config = { .first_section = NULL, .last_section = NULL, .mutex = NETDATA_MUTEX_INITIALIZER,
                               .index = {.avl_tree = { .root = NULL, .compar = appconfig_section_compare },
                                         .rwlock = AVL_LOCK_INITIALIZER } };

static struct bpf_link **probe_links = NULL;
static struct bpf_object *objects = NULL;

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_mount_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

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
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Directory Cache thread
 *
 * Thread used to make dcstat thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always returns NULL
 */
void *ebpf_mount_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_mount_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = mount_maps;
    fill_ebpf_data(&mount_data);

    if (!em->enabled)
        goto endmount;

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, mount_data.map_fd);
    if (!probe_links) {
        goto endmount;
    }

    int algorithms[NETDATA_EBPF_MOUNT_SYSCALL] = { NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX };

    ebpf_global_labels(mount_aggregated_data, mount_publish_aggregated, mount_dimension_name, mount_dimension_name,
                       algorithms, NETDATA_EBPF_MOUNT_SYSCALL);


endmount:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
