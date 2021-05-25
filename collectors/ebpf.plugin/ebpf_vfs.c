// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_vfs.h"

static char *vfs_dimension_names[NETDATA_KEY_PUBLISH_VFS_END] = { "delete",  "read",  "write",
                                                                  "fsync", "open", "create" };
static char *vfs_id_names[NETDATA_KEY_PUBLISH_VFS_END] = { "vfs_unlink", "vfs_read", "vfs_write",
                                                           "vfs_fsync", "vfs_open", "vfs_create"};

static netdata_idx_t *vfs_hash_values = NULL;
static netdata_syscall_stat_t vfs_aggregated_data[NETDATA_KEY_PUBLISH_PROCESS_END];
static netdata_publish_syscall_t vfs_publish_aggregated[NETDATA_KEY_PUBLISH_PROCESS_END];
netdata_publish_vfs_t **vfs_pid = NULL;
netdata_publish_vfs_t *vfs_vector = NULL;

static ebpf_data_t vfs_data;

static ebpf_local_maps_t vfs_maps[] = {{.name = "tbl_vfs_pid", .internal_input = ND_EBPF_DEFAULT_PID_SIZE,
                                        .user_input = 0},
                                       {.name = NULL, .internal_input = 0, .user_input = 0}};

struct config vfs_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
    .rwlock = AVL_LOCK_INITIALIZER } };

static struct bpf_object *objects = NULL;
static struct bpf_link **probe_links = NULL;

/*****************************************************************
 *
 *  FUNCTIONS TO CLOSE THE THREAD
 *
 *****************************************************************/

/**
* Clean up the main thread.
*
* @param ptr thread data.
**/
static void ebpf_vfs_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;

    freez(vfs_data.map_fd);
    freez(vfs_hash_values);
    freez(vfs_vector);

    struct bpf_program *prog;
    size_t i = 0 ;
    bpf_object__for_each_program(prog, objects) {
        bpf_link__destroy(probe_links[i]);
        i++;
    }
    bpf_object__close(objects);
}

/*****************************************************************
 *
 *  FUNCTIONS TO CREATE CHARTS
 *
 *****************************************************************/

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
 *  @param length is the length for the vectors used inside the collector.
 */
static void ebpf_vfs_allocate_global_vectors()
{
    memset(vfs_aggregated_data, 0, sizeof(vfs_aggregated_data));
    memset(vfs_publish_aggregated, 0, sizeof(vfs_publish_aggregated));

    vfs_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));
    vfs_vector = callocz(ebpf_nprocs, sizeof(netdata_publish_vfs_t));
    vfs_pid = callocz((size_t)pid_max, sizeof(netdata_publish_vfs_t *));
}

/*****************************************************************
 *
 *  EBPF PROCESS THREAD
 *
 *****************************************************************/

/**
 * Process thread
 *
 * Thread used to generate process charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_vfs_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_vfs_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = vfs_maps;
    fill_ebpf_data(&vfs_data);

    ebpf_update_pid_table(&vfs_maps[0], em);

    ebpf_vfs_allocate_global_vectors();

    if (!em->enabled)
        goto endvfs;

    probe_links = ebpf_load_program(ebpf_plugin_dir, em, kernel_string, &objects, vfs_data.map_fd);
    if (!probe_links) {
        goto endvfs;
    }

    int algorithms[NETDATA_KEY_PUBLISH_PROCESS_END] = {
        NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX,NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX, NETDATA_EBPF_INCREMENTAL_IDX,NETDATA_EBPF_INCREMENTAL_IDX
    };

    ebpf_global_labels(vfs_aggregated_data, vfs_publish_aggregated, vfs_dimension_names,
                       vfs_id_names, algorithms, NETDATA_KEY_PUBLISH_VFS_END);

endvfs:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
