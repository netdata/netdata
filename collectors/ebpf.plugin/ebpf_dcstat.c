// SPDX-License-Identifier: GPL-3.0-or-later

#include "ebpf.h"
#include "ebpf_dcstat.h"

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
static void ebpf_dcstat_cleanup(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    if (!em->enabled)
        return;
}

/*****************************************************************
 *
 *  APPS
 *
 *****************************************************************/

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_dcstat_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    UNUSED(em);
    UNUSED(ptr);
}

/*****************************************************************
 *
 *  MAIN THREAD
 *
 *****************************************************************/

/**
 * Cachestat thread
 *
 * Thread used to make dcstat thread
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_dcstat_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_dcstat_cleanup, ptr);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
