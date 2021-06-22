// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_disk.h"

struct config disk_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

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
static void ebpf_disk_cleanup(void *ptr)
{
}

/*****************************************************************
 *
 *  EBPF DISK THREAD
 *
 *****************************************************************/

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

    if (!em->enabled)
        goto enddisk;

enddisk:
    netdata_thread_cleanup_pop(1);

    return NULL;
}
