// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_socket.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static char *socket_dimension_names[NETDATA_MAX_SOCKET_VECTOR] = { "sent", "received", "close", "sent", "received" };
static char *socket_id_names[NETDATA_MAX_SOCKET_VECTOR] = { "tcp_sendmsg", "tcp_cleanup_rbuf", "tcp_close", "udp_sendmsg",
                                                            "udp_recvmsg" };

static netdata_idx_t *socket_hash_values = NULL;
static netdata_syscall_stat_t *socket_aggregated_data = NULL;
static netdata_publish_syscall_t *socket_publish_aggregated = NULL;

static ebpf_functions_t socket_functions;

static ebpf_bandwidth_t **socket_bandwidth_stats = NULL;
static ebpf_socket_publish_apps_t **socket_bandwidth_curr = NULL;
static ebpf_socket_publish_apps_t **socket_bandwidth_prev = NULL;

#ifndef STATIC
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
#endif

/*****************************************************************
 *
 *  PROCESS DATA AND SEND TO NETDATA
 *
 *****************************************************************/

/**
 * Update publish structure before to send data to Netdata.
 *
 * @param publish  the first output structure with independent dimensions
 * @param tcp      structure to store IO from tcp sockets
 * @param udp      structure to store IO from udp sockets
 * @param input    the structure with the input data.
 */
static void ebpf_update_global_publish(netdata_publish_syscall_t *publish,
                                       netdata_publish_vfs_common_t *tcp,
                                       netdata_publish_vfs_common_t *udp,
                                       netdata_syscall_stat_t *input) {

    netdata_publish_syscall_t *move = publish;
    while(move) {
        if(input->call != move->pcall) {
            //This condition happens to avoid initial values with dimensions higher than normal values.
            if(move->pcall) {
                move->ncall = (input->call > move->pcall)?input->call - move->pcall: move->pcall - input->call;
                move->nbyte = (input->bytes > move->pbyte)?input->bytes - move->pbyte: move->pbyte - input->bytes;
                move->nerr = (input->ecall > move->nerr)?input->ecall - move->perr: move->perr - input->ecall;
            } else {
                move->ncall = 0;
                move->nbyte = 0;
                move->nerr = 0;
            }

            move->pcall = input->call;
            move->pbyte = input->bytes;
            move->perr = input->ecall;
        } else {
            move->ncall = 0;
            move->nbyte = 0;
            move->nerr = 0;
        }

        input = input->next;
        move = move->next;
    }

    tcp->write = -((long)publish[0].nbyte);
    tcp->read = (long)publish[1].nbyte;

    udp->write = -((long)publish[3].nbyte);
    udp->read = (long)publish[4].nbyte;
}

/**
 * Update the publish strctures to create the dimenssions
 *
 * @param curr   Last values read from memory.
 * @param prev   Previous values read from memory.
 * @param first   was it allocated now?
 */
static void ebpf_socket_update_apps_publish(ebpf_socket_publish_apps_t *curr,
                                            ebpf_socket_publish_apps_t *prev, int first)
{
    if(first)
        return;

    curr->publish_recv = curr->received - prev->received;
    curr->publish_sent = curr->sent - prev->sent;
}

/**
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em the structure with thread information
 */
static void ebpf_process_send_data(ebpf_module_t *em) {
    netdata_publish_vfs_common_t common_tcp;
    netdata_publish_vfs_common_t common_udp;
    ebpf_update_global_publish(socket_publish_aggregated, &common_tcp, &common_udp, socket_aggregated_data);

    write_count_chart(NETDATA_TCP_FUNCTION_COUNT, NETDATA_EBPF_FAMILY, socket_publish_aggregated, 3);
    write_io_chart(NETDATA_TCP_FUNCTION_BYTES, NETDATA_EBPF_FAMILY, socket_id_names[0], socket_id_names[1], &common_tcp);
    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_TCP_FUNCTION_ERROR, NETDATA_EBPF_FAMILY, socket_publish_aggregated, 2);
    }

    write_count_chart(NETDATA_UDP_FUNCTION_COUNT, NETDATA_EBPF_FAMILY,
                             &socket_publish_aggregated[NETDATA_UDP_START], 2);
    write_io_chart(NETDATA_UDP_FUNCTION_BYTES, NETDATA_EBPF_FAMILY, socket_id_names[3], socket_id_names[4], &common_udp);
    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_UDP_FUNCTION_ERROR, NETDATA_EBPF_FAMILY,
                               &socket_publish_aggregated[NETDATA_UDP_START], 2);
    }
}

/*****************************************************************
 *
 *  FUNCTIONS TO CREATE CHARTS
 *
 *****************************************************************/

/**
 * Create global charts
 *
 * Call ebpf_create_chart to create the charts for the collector.
 *
 * @param em a pointer to the structure with the default values.
 */
static void ebpf_create_global_charts(ebpf_module_t *em) {
    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_TCP_FUNCTION_COUNT
        , EBPF_COMMON_DIMENSION_CALL
        , NETDATA_SOCKET_GROUP
        , 21070
        , ebpf_create_global_dimension
        , socket_publish_aggregated
        , 3);

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_TCP_FUNCTION_BYTES
        , EBPF_COMMON_DIMENSION_BYTESS
        , NETDATA_SOCKET_GROUP
        , 21071
        , ebpf_create_global_dimension
        , socket_publish_aggregated
        , 3);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_TCP_FUNCTION_ERROR
            , EBPF_COMMON_DIMENSION_CALL
            , NETDATA_SOCKET_GROUP
            , 21072
            , ebpf_create_global_dimension
            , socket_publish_aggregated
            , 2);
    }

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_UDP_FUNCTION_COUNT
        , EBPF_COMMON_DIMENSION_CALL
        , NETDATA_SOCKET_GROUP
        , 21073
        , ebpf_create_global_dimension
        , &socket_publish_aggregated[NETDATA_UDP_START]
        , 2);

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_UDP_FUNCTION_BYTES
        , EBPF_COMMON_DIMENSION_BYTESS
        , NETDATA_SOCKET_GROUP
        , 21074
        , ebpf_create_global_dimension
        , &socket_publish_aggregated[NETDATA_UDP_START]
        , 2);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_UDP_FUNCTION_ERROR
            , EBPF_COMMON_DIMENSION_CALL
            , NETDATA_SOCKET_GROUP
            , 21075
            , ebpf_create_global_dimension
            , &socket_publish_aggregated[NETDATA_UDP_START]
            , 2);
    }
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
static void ebpf_socket_create_apps_charts(ebpf_module_t *em)
{
    (void)em;
    return;
    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_SENT,
                               EBPF_COMMON_DIMENSION_BYTESS,
                               NETDATA_APPS_NET_GROUP,
                               20080,
                               apps_groups_root_target);

    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_RECV,
                               EBPF_COMMON_DIMENSION_BYTESS,
                               NETDATA_APPS_NET_GROUP,
                               20081,
                               apps_groups_root_target);

}

/*****************************************************************
 *
 *  READ INFORMATION FROM KERNEL RING
 *
 *****************************************************************/

/**
 * Read the hash table and store data to allocated vectors.
 */
static void read_hash_global_tables()
{
    uint64_t idx;
    netdata_idx_t res[NETDATA_SOCKET_COUNTER];

    netdata_idx_t *val = socket_hash_values;
    int fd = map_fd[4];
    for (idx = 0; idx < NETDATA_SOCKET_COUNTER ; idx++) {
        if (!bpf_map_lookup_elem(fd, &idx, val)) {
            uint64_t total = 0;
            int i;
            int end = (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs;
            for (i = 0; i < end; i++)
                total += val[i];

            res[idx] = total;
        } else {
            res[idx] = 0;
        }
    }

    socket_aggregated_data[0].call = res[NETDATA_KEY_CALLS_TCP_SENDMSG];
    socket_aggregated_data[1].call = res[NETDATA_KEY_CALLS_TCP_CLEANUP_RBUF];
    socket_aggregated_data[2].call = res[NETDATA_KEY_CALLS_TCP_CLOSE];
    socket_aggregated_data[3].call = res[NETDATA_KEY_CALLS_UDP_RECVMSG];
    socket_aggregated_data[4].call = res[NETDATA_KEY_CALLS_UDP_SENDMSG];

    socket_aggregated_data[0].ecall = res[NETDATA_KEY_ERROR_TCP_SENDMSG];
    socket_aggregated_data[1].ecall = res[NETDATA_KEY_ERROR_TCP_CLEANUP_RBUF];
    socket_aggregated_data[3].ecall = res[NETDATA_KEY_ERROR_UDP_RECVMSG];
    socket_aggregated_data[4].ecall = res[NETDATA_KEY_ERROR_UDP_SENDMSG];

    socket_aggregated_data[0].bytes = res[NETDATA_KEY_BYTES_TCP_SENDMSG];
    socket_aggregated_data[1].bytes = res[NETDATA_KEY_BYTES_TCP_CLEANUP_RBUF];
    socket_aggregated_data[3].bytes = res[NETDATA_KEY_BYTES_UDP_RECVMSG];
    socket_aggregated_data[4].bytes = res[NETDATA_KEY_BYTES_UDP_SENDMSG];
}

/**
 *  Update the apps data reading information from the hash table
 */
static void ebpf_socket_update_apps_data()
{
    int i;
    int fd = map_fd[0];
    for (i = 0 ; i < pids_running ; i++) {
        pid_t current_pid = pid_index[i];
        ebpf_bandwidth_t eb;

        if (bpf_map_lookup_elem(fd, &current_pid, &eb))
            continue;

        ebpf_socket_publish_apps_t *curr= socket_bandwidth_curr[current_pid];
        ebpf_socket_publish_apps_t *prev = socket_bandwidth_prev[current_pid];
        int status;
        if (!curr) {
            curr = callocz(2, sizeof(ebpf_socket_publish_apps_t));
            socket_bandwidth_curr[current_pid] = &curr[0];
            socket_bandwidth_prev[current_pid] = &curr[1];
            status = 1;
        } else {
            memcpy(prev, curr, sizeof(ebpf_socket_publish_apps_t));
            status = 0;
        }

        curr->sent = eb.sent;
        curr->received = eb.received;

        ebpf_socket_update_apps_publish(curr, prev, status);
    }
}

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

    int apps_enabled = em->apps_charts;
    while(!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_hash_global_tables();
        if (apps_enabled)
            ebpf_socket_update_apps_data();

        pthread_mutex_lock(&lock);
        ebpf_process_send_data(em);
        if (apps_enabled)
            ebpf_socket_create_apps_charts(em);
        pthread_mutex_unlock(&lock);

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

    freez(socket_aggregated_data);
    freez(socket_publish_aggregated);
    freez(socket_hash_values);

    freez(socket_bandwidth_stats);

    if (socket_functions.libnetdata) {
        dlclose(socket_functions.libnetdata);
    }

    freez(socket_functions.map_fd);
    freez(socket_bandwidth_curr);
    freez(socket_bandwidth_prev);
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
 * @param length is the length for the vectors used inside the collector.
 */
static void ebpf_socket_allocate_global_vectors(size_t length) {
    socket_aggregated_data = callocz(length, sizeof(netdata_syscall_stat_t));
    socket_publish_aggregated = callocz(length, sizeof(netdata_publish_syscall_t));
    socket_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));

    socket_bandwidth_stats = callocz((size_t)pid_max, sizeof(ebpf_bandwidth_t *));
    socket_bandwidth_curr = callocz((size_t)pid_max, sizeof(ebpf_socket_publish_apps_t *));
    socket_bandwidth_prev = callocz((size_t)pid_max, sizeof(ebpf_socket_publish_apps_t *));
}

static void change_collector_event() {
    socket_probes[0].type = 'p';
    socket_probes[5].type = 'p';
}

/**
 * Set local function pointers, this function will never be compiled with static libraries
 */
static void set_local_pointers(ebpf_module_t *em) {
#ifndef STATIC
    bpf_map_lookup_elem = socket_functions.bpf_map_lookup_elem;
    (void) bpf_map_lookup_elem;
    bpf_map_delete_elem = socket_functions.bpf_map_delete_elem;
    (void) bpf_map_delete_elem;
#endif
    map_fd = socket_functions.map_fd;

    if (em->mode == MODE_ENTRY) {
        change_collector_event();
    }
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
    fill_ebpf_functions(&socket_functions);

    if (!em->enabled)
        goto endsocket;

    pthread_mutex_lock(&lock);

    ebpf_socket_allocate_global_vectors(NETDATA_MAX_SOCKET_VECTOR);

    if (ebpf_load_libraries(&socket_functions, "libnetdata_ebpf.so", ebpf_plugin_dir)) {
        pthread_mutex_unlock(&lock);
        goto endsocket;
    }

    set_local_pointers(em);
    if (ebpf_load_program(ebpf_plugin_dir, em->thread_id, em->mode, kernel_string,
                          em->thread_name, socket_functions.map_fd, socket_functions.load_bpf_file) ) {
        pthread_mutex_unlock(&lock);
        goto endsocket;
    }

    ebpf_global_labels(socket_aggregated_data, socket_publish_aggregated, socket_dimension_names,
                       socket_id_names, NETDATA_MAX_SOCKET_VECTOR);

    ebpf_create_global_charts(em);

    pthread_mutex_unlock(&lock);

    socket_collector((usec_t)(em->update_time*USEC_PER_SEC), em);

endsocket:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
