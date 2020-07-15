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
static char *socket_id_names[NETDATA_MAX_SOCKET_VECTOR] = { "tcp_sendmsg", "tcp_cleanup_rbuf", "tcp_close",
                                                            "udp_sendmsg", "udp_recvmsg" };

static netdata_idx_t *socket_hash_values = NULL;
static netdata_syscall_stat_t *socket_aggregated_data = NULL;
static netdata_publish_syscall_t *socket_publish_aggregated = NULL;

static ebpf_data_t socket_data;

static ebpf_socket_publish_apps_t **socket_bandwidth_curr = NULL;
static ebpf_socket_publish_apps_t **socket_bandwidth_prev = NULL;
static ebpf_bandwidth_t *bandwidth_vector = NULL;

static int socket_apps_created = 0;

<<<<<<< HEAD
=======
netdata_vector_plot_t inbound_vectors = { .plot = NULL, .next = 0, .last = (REMOVE_THIS_DEFAULT - 1) };
netdata_vector_plot_t outbound_vectors = { .plot = NULL, .next = 0, .last = (REMOVE_THIS_DEFAULT -1 ) };
netdata_socket_t *socket_values;


#ifndef STATIC
/**
 * Pointers used when collector is dynamically linked
 */

//Libbpf (It is necessary to have at least kernel 4.10)
static int (*bpf_map_lookup_elem)(int, const void *, void *);
static int (*bpf_map_delete_elem)(int fd, const void *key);
static int (*bpf_map_get_next_key)(int fd, const void *key, void *next_key);

>>>>>>> 561d2baf... ebpf_read_socket: Auxiliar functions to read the information from socket
static int *map_fd = NULL;

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
static void ebpf_update_global_publish(
    netdata_publish_syscall_t *publish, netdata_publish_vfs_common_t *tcp, netdata_publish_vfs_common_t *udp,
    netdata_syscall_stat_t *input)
{
    netdata_publish_syscall_t *move = publish;
    while (move) {
        if (input->call != move->pcall) {
            //This condition happens to avoid initial values with dimensions higher than normal values.
            if (move->pcall) {
                move->ncall = (input->call > move->pcall) ? input->call - move->pcall : move->pcall - input->call;
                move->nbyte = (input->bytes > move->pbyte) ? input->bytes - move->pbyte : move->pbyte - input->bytes;
                move->nerr = (input->ecall > move->nerr) ? input->ecall - move->perr : move->perr - input->ecall;
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
 */
static void ebpf_socket_update_apps_publish(ebpf_socket_publish_apps_t *curr, ebpf_socket_publish_apps_t *prev)
{
    curr->publish_recv = curr->received - prev->received;
    curr->publish_sent = curr->sent - prev->sent;
}

/**
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em the structure with thread information
 */
static void ebpf_socket_send_data(ebpf_module_t *em)
{
    netdata_publish_vfs_common_t common_tcp;
    netdata_publish_vfs_common_t common_udp;
    ebpf_update_global_publish(socket_publish_aggregated, &common_tcp, &common_udp, socket_aggregated_data);

    write_count_chart(
      NETDATA_TCP_FUNCTION_COUNT, NETDATA_EBPF_FAMILY, socket_publish_aggregated, 3);
    write_io_chart(
        NETDATA_TCP_FUNCTION_BYTES, NETDATA_EBPF_FAMILY, socket_id_names[0], socket_id_names[1], &common_tcp);
    if (em->mode < MODE_ENTRY) {
        write_err_chart(
          NETDATA_TCP_FUNCTION_ERROR, NETDATA_EBPF_FAMILY, socket_publish_aggregated, 2);
    }

    write_count_chart(
        NETDATA_UDP_FUNCTION_COUNT, NETDATA_EBPF_FAMILY, &socket_publish_aggregated[NETDATA_UDP_START], 2);
    write_io_chart(
        NETDATA_UDP_FUNCTION_BYTES, NETDATA_EBPF_FAMILY, socket_id_names[3], socket_id_names[4], &common_udp);
    if (em->mode < MODE_ENTRY) {
        write_err_chart(
            NETDATA_UDP_FUNCTION_ERROR, NETDATA_EBPF_FAMILY, &socket_publish_aggregated[NETDATA_UDP_START], 2);
    }
}

/**
 * Sum values for pid
 *
 * @param root the structure with all available PIDs
 *
 * @param offset the address that we are reading
 *
 * @return it returns the sum of all PIDs
 */
long long ebpf_socket_sum_values_for_pids(struct pid_on_target *root, size_t offset)
{
    long long ret = 0;
    while (root) {
        int32_t pid = root->pid;
        ebpf_socket_publish_apps_t *w = socket_bandwidth_curr[pid];
        if (w) {
            ret += get_value_from_structure((char *)w, offset);
        }

        root = root->next;
    }

    return ret;
}

/**
 * Send data to Netdata calling auxiliar functions.
 *
 * @param em   the structure with thread information
 * @param root the target list.
 */
void ebpf_socket_send_apps_data(ebpf_module_t *em, struct target *root)
{
    UNUSED(em);
    if (!socket_apps_created)
        return;

    struct target *w;
    collected_number value;

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_BANDWIDTH_SENT);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t, publish_sent));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_BANDWIDTH_RECV);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t, publish_recv));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();
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
static void ebpf_create_global_charts(ebpf_module_t *em)
{
    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_TCP_FUNCTION_COUNT,
                      "Calls to internal functions",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_SOCKET_GROUP,
                      21070,
                      ebpf_create_global_dimension,
                      socket_publish_aggregated,
                      3);

    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_TCP_FUNCTION_BYTES,
                      "TCP bandwidth",
                      EBPF_COMMON_DIMENSION_BYTESS,
                      NETDATA_SOCKET_GROUP,
                      21071,
                      ebpf_create_global_dimension,
                      socket_publish_aggregated,
                      3);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY,
                          NETDATA_TCP_FUNCTION_ERROR,
                          "TCP errors",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_SOCKET_GROUP,
                          21072,
                          ebpf_create_global_dimension,
                          socket_publish_aggregated,
                          2);
    }

    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_UDP_FUNCTION_COUNT,
                      "UDP calls",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_SOCKET_GROUP,
                      21073,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_UDP_START],
                      2);

    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_UDP_FUNCTION_BYTES,
                      "UDP bandwidth",
                      EBPF_COMMON_DIMENSION_BYTESS,
                      NETDATA_SOCKET_GROUP,
                      21074,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_UDP_START],
                      2);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY,
                          NETDATA_UDP_FUNCTION_ERROR,
                          "UDP errors",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_SOCKET_GROUP,
                          21075,
                          ebpf_create_global_dimension,
                          &socket_publish_aggregated[NETDATA_UDP_START],
                          2);
    }
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em a pointer to the structure with the default values.
 */
void ebpf_socket_create_apps_charts(ebpf_module_t *em, struct target *root)
{
    UNUSED(em);
    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_SENT,
                               "Bytes sent",
                               EBPF_COMMON_DIMENSION_BYTESS,
                               NETDATA_APPS_NET_GROUP,
                               20080,
                               root);

    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_RECV,
                               "bytes received",
                               EBPF_COMMON_DIMENSION_BYTESS,
                               NETDATA_APPS_NET_GROUP,
                               20081,
                               root);

    socket_apps_created = 1;
}

/*****************************************************************
 *
 *  READ INFORMATION FROM KERNEL RING
 *
 *****************************************************************/

/**
 * Compare sockets
 *
 * Compare destination address and destination port.
 * We do not compare source port, because it is random.
 * We also do not compare source address, because inbound and outbound connections are stored in separated AVL trees.
 *
 * @param a pointer to netdata_socket_plot
 * @param b pointer  to netdata_socket_plot
 *
 * @return It returns 0 case the values are equal, 1 case a is bigger than b and -1 case a is smaller than b.
 */
static int compare_sockets(void *a, void *b)
{
    struct netdata_socket_plot *val1 = a;
    struct netdata_socket_plot *val2 = b;
    int cmp;

    //We do not need to compare val2 family, because data inside hash table is always from the same family
    if (val1->family == AF_INET) { //IPV4
        cmp = memcmp(&val1->index.daddr.addr32[0], &val2->index.daddr.addr32[0], sizeof(uint32_t));
        if (!cmp) {
            cmp = memcmp(&val1->index.dport, &val2->index.dport, sizeof(uint16_t));
        }
    } else {
        cmp = memcmp(&val1->index.daddr.addr32, &val2->index.daddr.addr32, 4*sizeof(uint32_t));
        if (!cmp) {
            cmp = memcmp(&val1->index.dport, &val2->index.dport, sizeof(uint16_t));
        }
    }

    return cmp;
}

/**
 * Build dimension name
 *
 * Fill dimension name vector with values given
 *
 * @param dimname       the output vector
 * @param hostname      the hostname for the socket.
 * @param service_name  the service used to connect.
 * @param is_inbound    is this an inbound connection.
 *
 * @return  it returns the size of the data copied on success and -1 otherwise.
 */
static inline int build_dimension_name(char *dimname, char *hostname, char *service_name,
                                       int is_inbound, netdata_socket_t *sock)
{
    int size;
    if (!is_inbound) {
        size = snprintf(dimname, CONFIG_MAX_NAME - 1, "%s:%s:%s_",
                        service_name, (sock->protocol == IPPROTO_UDP) ? "UDP" : "TCP",
                        hostname);

    } else {
        size = snprintf(dimname, CONFIG_MAX_NAME -1, "%s:%s_", service_name,
                        (sock->protocol == IPPROTO_UDP)?"UDP":"TCP");

    }

    return size;
}

/**
 * Fill Resolved Name
 *
 * Fill the resolved name structure with the value given.
 * The hostname is the largest value possible, if it is necessary to cut some value, it must be cut.
 *
 * @param ptr          the output vector
 * @param hostname     the hostname resolved or IP.
 * @param length       the length for the hostname.
 * @param service_name the service name associated to the connection
 * @param is_inboud    the is this an ibound connection
 */
static inline void fill_resolved_name(netdata_socket_plot_t *ptr, char *hostname, size_t length,
                                      char *service_name, int is_inbound)
{
    if (length < NETDATA_MAX_NETWORK_COMBINED_LENGTH)
        ptr->resolved_name = strdupz(hostname);
    else {
        length = NETDATA_MAX_NETWORK_COMBINED_LENGTH;
        ptr->resolved_name = mallocz(NETDATA_DIM_LENGTH_WITHOUT_SERVICE_PROTOCOL + 1);
        memcpy(ptr->resolved_name, hostname, NETDATA_DIM_LENGTH_WITHOUT_SERVICE_PROTOCOL);
        ptr->resolved_name[length] = '\0';
    }

    char dimname[CONFIG_MAX_NAME];
    int size = build_dimension_name(dimname, hostname, service_name, is_inbound, &ptr->sock);
    if (size > 0) {
        strcpy(&dimname[size], "sent");
        dimname[size + 4] = '\0';
        ptr->dimension_sent = strdupz(dimname);

        strcpy(&dimname[size], "recv");
        ptr->dimension_recv = strdupz(dimname);
    }
}

/**
 * Mount dimension names
 *
 * Fill the vector names after to resolve the addresses
 *
 * @param ptr a pointer to the structure where the values are stored.
 * @param is_inbound is a inbound ptr value?
 * @param is_last is this the last value possible?
 */
void fill_names(netdata_socket_plot_t *ptr, int is_inbound, uint32_t is_last)
{
    char hostname[NI_MAXHOST], service_name[NI_MAXSERV];
    if (ptr->resolved)
        return;

    if (is_last) {
        char *other = { "Other" };
        strncpy(hostname, other, 4);
        hostname[4] = '\0';
        strncpy(service_name, other, 4);
        service_name[4] = '\0';

        ptr->family = AF_INET;

        fill_resolved_name(ptr, hostname,  8 + NETDATA_DOTS_PROTOCOL_COMBINED_LENGTH, service_name, is_inbound);
        goto laststep;
    }

    netdata_socket_idx_t *idx = &ptr->index;

    char *errname = { "Not resolved" };
    // Resolve Name
    if (ptr->family == AF_INET) { //IPV4
        struct sockaddr_in myaddr;
        memset(&myaddr, 0 , sizeof(myaddr));

        myaddr.sin_family = ptr->family;
        myaddr.sin_port = idx->dport;
        myaddr.sin_addr.s_addr = (!is_inbound)?idx->daddr.addr32[0]:idx->saddr.addr32[0];

        if (getnameinfo((struct sockaddr *)&myaddr, sizeof(myaddr), hostname,
                         sizeof(hostname), service_name, sizeof(service_name), NI_NAMEREQD)) {
            //I cannot resolve the name, I will use the IP
            if (!inet_ntop(AF_INET, &myaddr.sin_addr, hostname, NI_MAXHOST)) {
                memcpy(hostname, errname, 12);
                hostname[12] = '\0';
            }
        }
    } else { //IPV6
        struct sockaddr_in6 myaddr6;
        memset(&myaddr6, 0 , sizeof(myaddr6));

        myaddr6.sin6_family = AF_INET6;
        myaddr6.sin6_port =  idx->dport;
        memcpy(myaddr6.sin6_addr.s6_addr, (!is_inbound)?idx->daddr.addr32:idx->saddr.addr32, sizeof(union netdata_ip));
        if (getnameinfo((struct sockaddr *)&myaddr6, sizeof(myaddr6), hostname,
                        sizeof(hostname), service_name, sizeof(service_name), NI_NAMEREQD)) {
            //I cannot resolve the name, I will use the IP
            if (!inet_ntop(AF_INET6, myaddr6.sin6_addr.s6_addr, hostname, NI_MAXHOST)) {
                memcpy(hostname, errname, 12);
                hostname[12] = '\0';
            }
        }
    }

    fill_resolved_name(ptr, hostname,
                       strlen(hostname) + strlen(service_name)+ NETDATA_DOTS_PROTOCOL_COMBINED_LENGTH,
                       service_name, is_inbound);

laststep:

    ptr->resolved++;
#ifdef NETDATA_INTERNAL_CHECKS
    info("The dimensions will be added Sent(%s) Recv(%s)", ptr->dimension_sent, ptr->dimension_recv);
#endif
}

/**
 * Store socket inside avl
 *
 * Store the socket values inside the avl tree.
 *
 * @param out     the structure with information used to plot charts.
 * @param lvalues Values read from socket ring.
 * @param lindex  the index information, the real socket.
 * @param family  the family associated to the socket
 */
static void store_socket_inside_avl(netdata_vector_plot_t *out, netdata_socket_t *lvalues,
                                    netdata_socket_idx_t *lindex, int family)
{
    netdata_socket_plot_t test, *ret ;

    memcpy(&test.index, lindex, sizeof(*lindex));

    char removeme_src[INET6_ADDRSTRLEN],  removeme_dst[INET6_ADDRSTRLEN];
    if (inet_ntop(family, lindex->daddr.addr8, removeme_dst, sizeof(removeme_dst)))
        inet_ntop(family, lindex->saddr.addr8, removeme_src, sizeof(removeme_src));

    ret = (netdata_socket_plot_t *) avl_search_lock(&out->tree, (avl *)&test);
    if (ret) {
        netdata_socket_t *sock = &ret->sock;

        sock->sent += lvalues->sent;
        sock->recv += lvalues->recv;
    } else {
        uint32_t curr = out->next;
        uint32_t last = out->last;

        netdata_socket_plot_t *w = &out->plot[curr];

        if (curr == last) {
            if (!w->resolved) {
                fill_names(w, out != (netdata_vector_plot_t *)&inbound_vectors, 1);
            }

            netdata_socket_t *sock = &w->sock;
            sock->sent += lvalues->sent;
            sock->recv += lvalues->recv;
            return;
        } else {
            memcpy(&w->sock, lvalues, sizeof(*lvalues));
            memcpy(&w->index, lindex, sizeof(*lindex));
            w->family = family;

            fill_names(w, out != (netdata_vector_plot_t *)&inbound_vectors, 0);
        }

        netdata_socket_plot_t *check ;
        check = (netdata_socket_plot_t *) avl_insert_lock(&out->tree, (avl *)w);
        if (check != w)
            error("Internal error, cannot insert the AVL tree.");

#ifdef NETDATA_INTERNAL_CHECKS
        char iptext[INET6_ADDRSTRLEN];
        if (inet_ntop(family, &w->index.daddr.addr8, iptext, sizeof(iptext)))
            info("New dimension added: ID = %u, IP = %s, NAME = %s, DIM1 = %s, DIM2 = %s, SENT = %lu, RECEIVED = %lu",
                 curr, iptext, w->resolved_name, w->dimension_recv, w->dimension_sent, w->sock.sent, w->sock.recv);
#endif
        curr++;
        if (curr > last)
            curr = last;
        out->next = curr;
    }
}

/**
 * Read socket hash table
 *
 * Read data from hash tables created on kernel ring.
 *
 * @param fd  the hash table with data.
 * @param family the family associated to the hash table
 *
 * @return it returns 0 on success and -1 otherwise.
 */
static void read_socket_hash_table(int fd, int family)
{
    netdata_socket_idx_t key = { };
    netdata_socket_idx_t next_key;
    netdata_socket_idx_t removeme;
    int removesock = 0;

    netdata_socket_t *values = socket_values;
    int end = (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs;

    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        int test = bpf_map_lookup_elem(fd, &key, values);
        if (test < 0) {
            key = next_key;
            continue;
        }

        if (removesock)
            bpf_map_delete_elem(fd, &removeme);

        uint64_t sent,recv;
        sent = recv = 0;
        removesock = 0;
        int i;
        for (i = 1; i < end; i++) {
            netdata_socket_t *w = &values[i];

            sent += w->sent;
            recv += w->recv;

            removesock += (int)w->removeme;
        }

        values[0].recv += recv;
        values[0].sent += sent;
        values[0].removeme += removesock;

        store_socket_inside_avl(&inbound_vectors, values, &key, family);

        if (removesock)
            removeme = key;

        key = next_key;
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
void *ebpf_socket_read_hash(void *ptr)
{
    UNUSED(ptr);

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = NETDATA_SOCKET_READ_SLEEP_MS;
    int fd_ipv4 = map_fd[NETDATA_SOCKET_IPV4_HASH_TABLE];
    int fd_ipv6 = map_fd[NETDATA_SOCKET_IPV6_HASH_TABLE];
    while (!close_ebpf_plugin) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        read_socket_hash_table(fd_ipv4, AF_INET);
        read_socket_hash_table(fd_ipv6, AF_INET6);
    }

    return NULL;
}

/**
 * Read the hash table and store data to allocated vectors.
 */
static void read_hash_global_tables()
{
    uint64_t idx;
    netdata_idx_t res[NETDATA_SOCKET_COUNTER];

    netdata_idx_t *val = socket_hash_values;
<<<<<<< HEAD
    int fd = map_fd[4];
    for (idx = 0; idx < NETDATA_SOCKET_COUNTER; idx++) {
=======
    int fd = map_fd[NETDATA_SOCKET_GLOBAL_HASH_TABLE];
    for (idx = 0; idx < NETDATA_SOCKET_COUNTER ; idx++) {
>>>>>>> 561d2baf... ebpf_read_socket: Auxiliar functions to read the information from socket
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
 * Fill publish apps when necessary.
 *
 * @param current_pid  the PID that I am updating
 * @param eb           the structure with data read from memory.
 */
void ebpf_socket_fill_publish_apps(uint32_t current_pid, ebpf_bandwidth_t *eb)
{
    ebpf_socket_publish_apps_t *curr = socket_bandwidth_curr[current_pid];
    ebpf_socket_publish_apps_t *prev = socket_bandwidth_prev[current_pid];
    if (!curr) {
        ebpf_socket_publish_apps_t *ptr = callocz(2, sizeof(ebpf_socket_publish_apps_t));
        curr = &ptr[0];
        socket_bandwidth_curr[current_pid] = curr;
        prev = &ptr[1];
        socket_bandwidth_prev[current_pid] = prev;
    } else {
        memcpy(prev, curr, sizeof(ebpf_socket_publish_apps_t));
    }

    curr->sent = eb->sent;
    curr->received = eb->received;

    ebpf_socket_update_apps_publish(curr, prev);
}

/**
 * Bandwidth accumulator.
 *
 * @param out the vector with the values to sum
 */
void ebpf_socket_bandwidth_accumulator(ebpf_bandwidth_t *out)
{
    int i, end = (running_on_kernel >= NETDATA_KERNEL_V4_15) ? ebpf_nprocs : 1;
    ebpf_bandwidth_t *total = &out[0];
    for (i = 1; i < end; i++) {
        ebpf_bandwidth_t *move = &out[i];
        total->sent += move->sent;
        total->received += move->received;
    }
}

/**
 *  Update the apps data reading information from the hash table
 */
static void ebpf_socket_update_apps_data()
{
    int fd = map_fd[NETDATA_SOCKET_APPS_HASH_TABLE];
    ebpf_bandwidth_t *eb = bandwidth_vector;
    uint32_t key;
    struct pid_stat *pids = root_of_pids;
    while (pids) {
        key = pids->pid;

        if (bpf_map_lookup_elem(fd, &key, eb)) {
            pids = pids->next;
            continue;
        }

        ebpf_socket_bandwidth_accumulator(eb);

        ebpf_socket_fill_publish_apps(key, eb);

        if (eb[0].removed)
            bpf_map_delete_elem(fd, &key);

        pids = pids->next;
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
    UNUSED(em);
    UNUSED(step);
    heartbeat_t hb;
    heartbeat_init(&hb);

    struct netdata_static_thread socket_threads = {"EBPF SOCKET READ",
                                                    NULL, NULL, 1, NULL,
                                                    NULL, ebpf_socket_read_hash };
    socket_threads.thread = mallocz(sizeof(netdata_thread_t));;

    netdata_thread_create(socket_threads.thread, socket_threads.name,
                          NETDATA_THREAD_OPTION_JOINABLE, ebpf_socket_read_hash, em);

    int socket_apps_enabled = ebpf_modules[EBPF_MODULE_SOCKET_IDX].apps_charts;
    int socket_global_enabled = ebpf_modules[EBPF_MODULE_SOCKET_IDX].global_charts;
    while (!close_ebpf_plugin) {
        pthread_mutex_lock(&collect_data_mutex);
        pthread_cond_wait(&collect_data_cond_var, &collect_data_mutex);

        if (socket_global_enabled)
            read_hash_global_tables();

        if (socket_apps_enabled)
            ebpf_socket_update_apps_data();

        pthread_mutex_lock(&lock);
        if (socket_global_enabled)
            ebpf_socket_send_data(em);

        if (socket_apps_enabled)
            ebpf_socket_send_apps_data(em, apps_groups_root_target);

        pthread_mutex_unlock(&collect_data_mutex);

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
 * Clean netowrk ports allocated during initializaion.
 *
 * @param ptr a pointer to the link list.
 */
static void clean_network_ports(ebpf_network_viewer_port_list_t *ptr)
{
    if (unlikely(!ptr))
        return;

    while (ptr) {
        ebpf_network_viewer_port_list_t *next = ptr->next;
        freez(ptr->value);
        freez(ptr);
        ptr = next;
    }
}

/**
 * Clean service names
 *
 * Clean the allocated link list that stores names.
 *
 * @param names the link list.
 */
static void clean_service_names(ebpf_network_viewer_dim_name_t *names)
{
    if (unlikely(!names))
        return;

    while (names) {
        ebpf_network_viewer_dim_name_t *next = names->next;
        freez(names->name);
        freez(names);
        names = next;
    }
}

/**
 * Clean hostnames
 *
 * @param hostnames the hostnames to clean
 */
static void clean_hostnames(ebpf_network_viewer_hostname_list_t *hostnames)
{
    if (unlikely(!hostnames))
        return;

    while (hostnames) {
        ebpf_network_viewer_hostname_list_t *next = hostnames->next;
        freez(hostnames->value);
        simple_pattern_free(hostnames->value_pattern);
        freez(hostnames);
        hostnames = next;
    }
}

/**
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_socket_cleanup(void *ptr)
{
    UNUSED(ptr);
    freez(socket_aggregated_data);
    freez(socket_publish_aggregated);
    freez(socket_hash_values);

    freez(socket_data.map_fd);
    freez(socket_bandwidth_curr);
    freez(socket_bandwidth_prev);
    freez(bandwidth_vector);

    freez(socket_values);
    freez(inbound_vectors.plot);
    freez(outbound_vectors.plot);

    ebpf_modules[EBPF_MODULE_SOCKET_IDX].enabled = 0;

    clean_network_ports(network_viewer_opt.included_port);
    clean_network_ports(network_viewer_opt.excluded_port);
    clean_service_names(network_viewer_opt.names);
    clean_hostnames(network_viewer_opt.included_hostnames);
    clean_hostnames(network_viewer_opt.excluded_hostnames);
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
static void ebpf_socket_allocate_global_vectors(size_t length)
{
    socket_aggregated_data = callocz(length, sizeof(netdata_syscall_stat_t));
    socket_publish_aggregated = callocz(length, sizeof(netdata_publish_syscall_t));
    socket_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));

    socket_bandwidth_curr = callocz((size_t)pid_max, sizeof(ebpf_socket_publish_apps_t *));
    socket_bandwidth_prev = callocz((size_t)pid_max, sizeof(ebpf_socket_publish_apps_t *));
<<<<<<< HEAD
    bandwidth_vector = callocz((size_t)ebpf_nprocs, sizeof(ebpf_bandwidth_t));
=======
    bandwidth_vector = callocz((size_t) ebpf_nprocs, sizeof(ebpf_bandwidth_t));

    socket_values = callocz((size_t) ebpf_nprocs, sizeof(netdata_socket_t));
    inbound_vectors.plot = callocz(REMOVE_THIS_DEFAULT, sizeof(netdata_socket_plot_t));
    outbound_vectors.plot = callocz(REMOVE_THIS_DEFAULT, sizeof(netdata_socket_plot_t));
>>>>>>> 561d2baf... ebpf_read_socket: Auxiliar functions to read the information from socket
}

void change_socket_event()
{
    socket_probes[0].type = 'p';
    socket_probes[4].type = 'p';
    socket_probes[5].type = 'p';
    socket_probes[7].name = NULL;
}

/**
 * Set local function pointers, this function will never be compiled with static libraries
 */
<<<<<<< HEAD
static void set_local_pointers(ebpf_module_t *em)
{
    map_fd = socket_data.map_fd;
=======
static void set_local_pointers(ebpf_module_t *em) {
#ifndef STATIC
    bpf_map_lookup_elem = socket_functions.bpf_map_lookup_elem;
    (void) bpf_map_lookup_elem;
    bpf_map_delete_elem = socket_functions.bpf_map_delete_elem;
    (void) bpf_map_delete_elem;
    bpf_map_get_next_key = socket_functions.bpf_map_get_next_key;
    (void) bpf_map_get_next_key;
#endif
    map_fd = socket_functions.map_fd;
>>>>>>> 561d2baf... ebpf_read_socket: Auxiliar functions to read the information from socket

    if (em->mode == MODE_ENTRY) {
        change_socket_event();
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

    avl_init_lock(&inbound_vectors.tree, compare_sockets);
    avl_init_lock(&outbound_vectors.tree, compare_sockets);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    fill_ebpf_data(&socket_data);

    if (!em->enabled)
        goto endsocket;

    pthread_mutex_lock(&lock);

    ebpf_socket_allocate_global_vectors(NETDATA_MAX_SOCKET_VECTOR);

    if (ebpf_update_kernel(&socket_data)) {
        pthread_mutex_unlock(&lock);
        goto endsocket;
    }

    set_local_pointers(em);
    if (ebpf_load_program(
            ebpf_plugin_dir, em->thread_id, em->mode, kernel_string, em->thread_name, socket_data.map_fd)) {
        pthread_mutex_unlock(&lock);
        goto endsocket;
    }

    ebpf_global_labels(
        socket_aggregated_data, socket_publish_aggregated, socket_dimension_names, socket_id_names,
        NETDATA_MAX_SOCKET_VECTOR);

    ebpf_create_global_charts(em);

    pthread_mutex_unlock(&lock);

    socket_collector((usec_t)(em->update_time * USEC_PER_SEC), em);

endsocket:
    netdata_thread_cleanup_pop(1);
    return NULL;
}
