// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_vfs.h"

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

    netdata_thread_cleanup_pop(1);
    return NULL;
}
