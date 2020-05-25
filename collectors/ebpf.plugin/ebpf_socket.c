// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static ebpf_functions_t functions;

/**
 * Pointers used when collector is dynamically linked
 */

//Libbpf (It is necessary to have at least kernel 4.10)
static int (*bpf_map_lookup_elem)(int, const void *, void *);
static int (*bpf_map_delete_elem)(int fd, const void *key);

static int *map_fd = NULL;
/**
 * End of the pointers
 */

/*****************************************************************
 *
 *  FUNCTIONS WITH THE MAIN LOOP
 *
 *****************************************************************/


/**
 * Main loop for this collector.
 *
 * @param step the number of microseconds used with heart beat
 * @param em   the structure with thread information
 */
static void socket_collector(usec_t step, ebpf_module_t *em)
{
    (void)em;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        fflush(stdout);
    }
}

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
static void ebpf_socket_cleanup(void *ptr)
{
    (void)ptr;

    if (functions.libnetdata) {
        dlclose(functions.libnetdata);
    }
}

/*****************************************************************
 *
 *  FUNCTIONS TO START THREAD
 *
 *****************************************************************/

/**
 * Set local function pointers, this function will never be compiled with static libraries
 */
static void set_local_pointers(ebpf_module_t *em) {
    bpf_map_lookup_elem = functions.bpf_map_lookup_elem;
    (void) bpf_map_lookup_elem;

    bpf_map_delete_elem = functions.bpf_map_delete_elem;
    (void) bpf_map_delete_elem;

    map_fd = functions.map_fd;
    (void)map_fd;
}

/*****************************************************************
 *
 *  EBPF SOCKET THREAD
 *
 *****************************************************************/

/**
 * Socket thread
 *
 * Thread used to generate socket charts.
 *
 * @param ptr a pointer to `struct ebpf_module`
 *
 * @return It always return NULL
 */
void *ebpf_socket_thread(void *ptr)
{
    netdata_thread_cleanup_push(ebpf_socket_cleanup, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;

    pthread_mutex_lock(&lock);
    fill_ebpf_functions(&functions);
    if (ebpf_load_libraries(&functions, "libnetdata_ebpf.so", ebpf_plugin_dir)) {
        pthread_mutex_unlock(&lock);
        return NULL;
    }
    pthread_mutex_unlock(&lock);

    set_local_pointers(em);
    if (ebpf_load_program(ebpf_plugin_dir, em->thread_id, em->mode, kernel_string,
                      em->thread_name, functions.load_bpf_file) )
        goto endsocket;

    socket_collector((usec_t)(em->update_time*USEC_PER_SEC), em);

endsocket:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
