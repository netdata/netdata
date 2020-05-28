// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_socket.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static ebpf_functions_t socket_functions;

static netdata_idx_t *socket_hash_values = NULL;
static netdata_syscall_stat_t *socket_aggregated_data = NULL;
static netdata_publish_syscall_t *socket_publish_aggregated = NULL;

static char *socket_dimension_names[NETDATA_MAX_SOCKET_VECTOR] = { "sent", "received", "close", "sent", "received" };
static char *socket_id_names[NETDATA_MAX_SOCKET_VECTOR] = { "tcp_sendmsg", "tcp_cleanup_rbuf", "tcp_close", "udp_sendmsg",
                                                     "udp_recvmsg" };

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
static void ebpf_update_publish(netdata_publish_syscall_t *publish,
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
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em the structure with thread information
 */
static void ebpf_process_send_data(ebpf_module_t *em) {
    netdata_publish_vfs_common_t common_tcp;
    netdata_publish_vfs_common_t common_udp;
    ebpf_update_publish(socket_publish_aggregated, &common_tcp, &common_udp, socket_aggregated_data);

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
        , "Calls"
        , NETDATA_SOCKET_GROUP
        , 950
        , ebpf_create_global_dimension
        , socket_publish_aggregated
        , 3);

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_TCP_FUNCTION_BYTES
        , "bytes/s"
        , NETDATA_SOCKET_GROUP
        , 951
        , ebpf_create_global_dimension
        , socket_publish_aggregated
        , 3);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_TCP_FUNCTION_ERROR
            , "Calls"
            , NETDATA_SOCKET_GROUP
            , 952
            , ebpf_create_global_dimension
            , socket_publish_aggregated
            , 2);
    }

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_UDP_FUNCTION_COUNT
        , "Calls"
        , NETDATA_SOCKET_GROUP
        , 953
        , ebpf_create_global_dimension
        , &socket_publish_aggregated[NETDATA_UDP_START]
        , 2);

    ebpf_create_chart(NETDATA_EBPF_FAMILY
        , NETDATA_UDP_FUNCTION_BYTES
        , "bytes/s"
        , NETDATA_SOCKET_GROUP
        , 954
        , ebpf_create_global_dimension
        , &socket_publish_aggregated[NETDATA_UDP_START]
        , 2);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY
            , NETDATA_UDP_FUNCTION_ERROR
            , "Calls"
            , NETDATA_SOCKET_GROUP
            , 955
            , ebpf_create_global_dimension
            , &socket_publish_aggregated[NETDATA_UDP_START]
            , 2);
    }
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
    for (idx = 0; idx < NETDATA_SOCKET_COUNTER ; idx++) {
        if (!bpf_map_lookup_elem(map_fd[4], &idx, val)) {
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

        read_hash_global_tables();

        pthread_mutex_lock(&lock);
        ebpf_process_send_data(em);
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

    if (socket_functions.libnetdata) {
        dlclose(socket_functions.libnetdata);
    }

    freez(socket_functions.map_fd);
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
