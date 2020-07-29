// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_socket.h"

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static char *socket_dimension_names[NETDATA_MAX_SOCKET_VECTOR] = { "sent", "received", "close", "sent",
                                                                   "received", "retransmitted" };
static char *socket_id_names[NETDATA_MAX_SOCKET_VECTOR] = { "tcp_sendmsg", "tcp_cleanup_rbuf", "tcp_close",
                                                            "udp_sendmsg", "udp_recvmsg", "tcp_retransmit_skb" };

static netdata_idx_t *socket_hash_values = NULL;
static netdata_syscall_stat_t *socket_aggregated_data = NULL;
static netdata_publish_syscall_t *socket_publish_aggregated = NULL;

static ebpf_data_t socket_data;

static ebpf_socket_publish_apps_t **socket_bandwidth_curr = NULL;
static ebpf_socket_publish_apps_t **socket_bandwidth_prev = NULL;
static ebpf_bandwidth_t *bandwidth_vector = NULL;

static int socket_apps_created = 0;

netdata_vector_plot_t inbound_vectors = { .plot = NULL, .next = 0, .last = 0 };
netdata_vector_plot_t outbound_vectors = { .plot = NULL, .next = 0, .last = 0 };
netdata_socket_t *socket_values;

ebpf_network_viewer_port_list_t *listen_ports = NULL;

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
            // This condition happens to avoid initial values with dimensions higher than normal values.
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
        NETDATA_TCP_RETRANSMIT, NETDATA_EBPF_FAMILY, &socket_publish_aggregated[NETDATA_RETRANSMIT_START], 1);

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
                      NETDATA_TCP_RETRANSMIT,
                      "Packages retransmitted",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_SOCKET_GROUP,
                      21073,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_RETRANSMIT_START],
                      1);

    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_UDP_FUNCTION_COUNT,
                      "UDP calls",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_SOCKET_GROUP,
                      21074,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_UDP_START],
                      2);

    ebpf_create_chart(NETDATA_EBPF_FAMILY,
                      NETDATA_UDP_FUNCTION_BYTES,
                      "UDP bandwidth",
                      EBPF_COMMON_DIMENSION_BYTESS,
                      NETDATA_SOCKET_GROUP,
                      21075,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_UDP_START],
                      2);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_FAMILY,
                          NETDATA_UDP_FUNCTION_ERROR,
                          "UDP errors",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_SOCKET_GROUP,
                          21076,
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
 * Is specific ip inside the range
 *
 * Check if the ip is inside a IP range previously defined
 *
 * @param cmp       the IP to compare
 * @param family    the IP family
 *
 * @return It returns 1 if the IP is inside the range and 0 otherwise
 */
static int is_specific_ip_inside_range(union netdata_ip_t *cmp, int family)
{
    if (!network_viewer_opt.excluded_ips && !network_viewer_opt.included_ips)
        return 1;

    uint32_t ipv4_test = ntohl(cmp->addr32[0]);
    ebpf_network_viewer_ip_list_t *move = network_viewer_opt.excluded_ips;
    while (move) {
        if (family == AF_INET) {
            if (ntohl(move->first.addr32[0]) <= ipv4_test &&
                ipv4_test <= ntohl(move->last.addr32[0]) )
                return 0;
        } else {
            if (memcmp(move->first.addr8, cmp->addr8, sizeof(union netdata_ip_t)) <= 0 &&
                memcmp(move->last.addr8, cmp->addr8, sizeof(union netdata_ip_t)) >= 0) {
                return 0;
            }
        }
        move = move->next;
    }

    move = network_viewer_opt.included_ips;
    while (move) {
        if (family == AF_INET) {
            if (ntohl(move->first.addr32[0]) <= ipv4_test &&
                ntohl(move->last.addr32[0]) >= ipv4_test)
                return 1;
        } else {
            if (memcmp(move->first.addr8, cmp->addr8, sizeof(union netdata_ip_t)) <= 0 &&
                memcmp(move->last.addr8, cmp->addr8, sizeof(union netdata_ip_t)) >= 0) {
                return 1;
            }
        }
        move = move->next;
    }

    return 0;
}

/**
 * Is port inside range
 *
 * Verify if the cmp port is inside the range [first, last].
 * This function expects only the last parameter as big endian.
 *
 * @param cmp    the value to compare
 *
 * @return It returns 1 when cmp is inside and 0 otherwise.
 */
static int is_port_inside_range(uint16_t cmp)
{
    // We do not have restrictions for ports.
    if (!network_viewer_opt.excluded_port && !network_viewer_opt.included_port)
        return 1;

    // Test if port is excluded
    ebpf_network_viewer_port_list_t *move = network_viewer_opt.excluded_port;
    cmp = htons(cmp);
    while (move) {
        if (move->cmp_first <= cmp && cmp <= move->cmp_last)
            return 0;

        move = move->next;
    }

    // Test if the port is inside allowed range
    move = network_viewer_opt.included_port;
    while (move) {
        if (move->cmp_first <= cmp && cmp <= move->cmp_last)
            return 1;

        move = move->next;
    }

    return 0;
}

/**
 * Hostname matches pattern
 *
 * @param cmp  the value to compare
 *
 * @return It returns 1 when the value matches and zero otherwise.
 */
int hostname_matches_pattern(char *cmp)
{
    if (!network_viewer_opt.included_hostnames && !network_viewer_opt.excluded_hostnames)
        return 1;

    ebpf_network_viewer_hostname_list_t *move = network_viewer_opt.excluded_hostnames;
    while (move) {
        if (simple_pattern_matches(move->value_pattern, cmp))
            return 0;

        move = move->next;
    }

    move = network_viewer_opt.included_hostnames;
    while (move) {
        if (simple_pattern_matches(move->value_pattern, cmp))
            return 1;

        move = move->next;
    }


    return 0;
}

/**
 * Is socket allowed?
 *
 * Compare destination addresses and destination ports to define next steps
 *
 * @param key     the socket read from kernel ring
 * @param family  the family used to compare IPs (AF_INET and AF_INET6)
 *
 * @return It returns 1 if this socket is inside the ranges and 0 otherwise.
 */
int is_socket_allowed(netdata_socket_idx_t *key, int family)
{
    if (!is_port_inside_range(key->dport))
        return 0;

    return is_specific_ip_inside_range(&key->daddr, family);
}

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

    // We do not need to compare val2 family, because data inside hash table is always from the same family
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
 * @param proto         the protocol used in this connection
 * @param family        is this IPV4(AF_INET) or IPV6(AF_INET6)
 *
 * @return  it returns the size of the data copied on success and -1 otherwise.
 */
static inline int build_outbound_dimension_name(char *dimname, char *hostname, char *service_name,
                                               char *proto, int family)
{
    return snprintf(dimname, CONFIG_MAX_NAME - 7, (family == AF_INET)?"%s:%s:%s_":"%s:%s:[%s]_",
                    service_name, proto,
                    hostname);
}

/**
 * Fill inbound dimension name
 *
 * Mount the dimension name with the input given
 *
 * @param dimname       the output vector
 * @param service_name  the service used to connect.
 * @param proto         the protocol used in this connection
 *
 * @return  it returns the size of the data copied on success and -1 otherwise.
 */
static inline int build_inbound_dimension_name(char *dimname, char *service_name, char *proto)
{
    return snprintf(dimname, CONFIG_MAX_NAME - 7, "%s:%s_", service_name,
                    proto);
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
 * @param is_outbound    the is this an outbound connection
 */
static inline void fill_resolved_name(netdata_socket_plot_t *ptr, char *hostname, size_t length,
                                      char *service_name, int is_outbound)
{
    if (length < NETDATA_MAX_NETWORK_COMBINED_LENGTH)
        ptr->resolved_name = strdupz(hostname);
    else {
        length = NETDATA_MAX_NETWORK_COMBINED_LENGTH;
        ptr->resolved_name = mallocz(NETDATA_DIM_LENGTH_WITHOUT_SERVICE_PROTOCOL + 1);
        memcpy(ptr->resolved_name, hostname, length);
        ptr->resolved_name[length] = '\0';
    }

    char dimname[CONFIG_MAX_NAME];
    int size;
    char *protocol = (ptr->sock.protocol == IPPROTO_UDP) ? "UDP" : "TCP";
    if (is_outbound)
        size = build_outbound_dimension_name(dimname, hostname, service_name, protocol, ptr->family);
    else
        size = build_inbound_dimension_name(dimname,service_name, protocol);

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
 * @param is_outbound is a outbound ptr value?
 * @param is_last is this the last value possible?
 *
 * @return It returns 1 if the name is valid and 0 otherwise.
 */
int fill_names(netdata_socket_plot_t *ptr, int is_outbound, uint32_t is_last)
{
    char hostname[NI_MAXHOST], service_name[NI_MAXSERV];
    if (ptr->resolved)
        return 1;

    int ret;
    static int resolve_name = -1;
    if (resolve_name == -1)
        resolve_name = network_viewer_opt.name_resolution_enabled;

    if (is_last) {
        char *other = { "Other" };
        // We are also copying the NULL bytes to avoid warnings in new compilers
        strncpy(hostname, other, 6);
        strncpy(service_name, other, 6);

        ptr->family = AF_INET;

        fill_resolved_name(ptr, hostname,  10 + NETDATA_DOTS_PROTOCOL_COMBINED_LENGTH, service_name, is_outbound);
        ret = 1;
        goto laststep;
    }

    netdata_socket_idx_t *idx = &ptr->index;

    char *errname = { "Not resolved" };
    // Resolve Name
    if (ptr->family == AF_INET) { //IPV4
        struct sockaddr_in myaddr;
        memset(&myaddr, 0 , sizeof(myaddr));

        myaddr.sin_family = ptr->family;
        if (is_outbound) {
            myaddr.sin_port = idx->dport;
            myaddr.sin_addr.s_addr = idx->daddr.addr32[0];
        } else {
            myaddr.sin_port = idx->sport;
            myaddr.sin_addr.s_addr = idx->saddr.addr32[0];
        }

        ret = (!resolve_name)?-1:getnameinfo((struct sockaddr *)&myaddr, sizeof(myaddr), hostname,
                                              sizeof(hostname), service_name, sizeof(service_name), NI_NAMEREQD);
        if (ret) {
            // I cannot resolve the name, I will use the IP
            if (!inet_ntop(AF_INET, &myaddr.sin_addr.s_addr, hostname, NI_MAXHOST)) {
                strncpy(hostname, errname, 13);
            }

            snprintf(service_name, sizeof(service_name), "%u", ntohs(myaddr.sin_port));
            ret = 1;
        }
    } else { // IPV6
        struct sockaddr_in6 myaddr6;
        memset(&myaddr6, 0 , sizeof(myaddr6));

        myaddr6.sin6_family = AF_INET6;
        if (is_outbound) {
            myaddr6.sin6_port =  idx->dport;
            memcpy(myaddr6.sin6_addr.s6_addr, idx->daddr.addr8, sizeof(union netdata_ip_t));
        } else {
            myaddr6.sin6_port =  idx->sport;
            memcpy(myaddr6.sin6_addr.s6_addr, idx->saddr.addr8, sizeof(union netdata_ip_t));
        }

        ret = (!resolve_name)?-1:getnameinfo((struct sockaddr *)&myaddr6, sizeof(myaddr6), hostname,
                                              sizeof(hostname), service_name, sizeof(service_name), NI_NAMEREQD);
        if (ret) {
            // I cannot resolve the name, I will use the IP
            if (!inet_ntop(AF_INET6, myaddr6.sin6_addr.s6_addr, hostname, NI_MAXHOST)) {
                strncpy(hostname, errname, 13);
            }
            snprintf(service_name, sizeof(service_name), "%u", ntohs(myaddr6.sin6_port));
            ret = 1;
        }
    }

    fill_resolved_name(ptr, hostname,
                       strlen(hostname) + strlen(service_name)+ NETDATA_DOTS_PROTOCOL_COMBINED_LENGTH,
                       service_name, is_outbound);

laststep:

    if (resolve_name && !ret)
        ret = hostname_matches_pattern(hostname);

    ptr->resolved++;

    return ret;
}

/**
 * Update Socket Data
 *
 * Update the socket information with last collected data
 *
 * @param sock
 * @param lvalues
 */
static inline void update_socket_data(netdata_socket_t *sock, netdata_socket_t *lvalues)
{
    sock->recv_packets = lvalues->recv_packets;
    sock->sent_packets = lvalues->sent_packets;
    sock->recv_bytes   = lvalues->recv_bytes;
    sock->sent_bytes   = lvalues->sent_bytes;
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

    ret = (netdata_socket_plot_t *) avl_search_lock(&out->tree, (avl *)&test);
    if (ret) {
        update_socket_data(&ret->sock, lvalues);
    } else {
        uint32_t curr = out->next;
        uint32_t last = out->last;

        netdata_socket_plot_t *w = &out->plot[curr];

        int resolved;
        if (curr == last) {
            if (!w->resolved) {
                resolved = fill_names(w, out != (netdata_vector_plot_t *)&inbound_vectors, 1);
                UNUSED(resolved);
#ifdef NETDATA_INTERNAL_CHECKS
                info("Last %s dimension added: ID = %u, IP = OTHER, NAME = %s, DIM1 = %s, DIM2 = %s",
                     (out == &inbound_vectors)?"inbound":"outbound", curr, w->resolved_name,
                     w->dimension_recv, w->dimension_sent);
#endif
            }

            update_socket_data(&w->sock, lvalues);
            return;
        } else {
            memcpy(&w->sock, lvalues, sizeof(*lvalues));
            memcpy(&w->index, lindex, sizeof(*lindex));
            w->family = family;

            resolved = fill_names(w, out != (netdata_vector_plot_t *)&inbound_vectors, 0);
        }

        if (!resolved) {
            freez(w->resolved_name);
            freez(w->dimension_sent);
            freez(w->dimension_recv);

            memset(w, 0, sizeof(netdata_socket_plot_t));

            return;
        }

        netdata_socket_plot_t *check ;
        check = (netdata_socket_plot_t *) avl_insert_lock(&out->tree, (avl *)w);
        if (check != w)
            error("Internal error, cannot insert the AVL tree.");

#ifdef NETDATA_INTERNAL_CHECKS
        char iptext[INET6_ADDRSTRLEN];
        if (inet_ntop(family, &w->index.daddr.addr8, iptext, sizeof(iptext)))
            info("New %s dimension added: ID = %u, IP = %s, NAME = %s, DIM1 = %s, DIM2 = %s, SENT = %lu(%lu), RECV = %lu(%lu)",
                 (out == &inbound_vectors)?"inbound":"outbound", curr, iptext, w->resolved_name,
                 w->dimension_recv, w->dimension_sent, w->sock.sent_bytes, w->sock.sent_packets,
                 w->sock.recv_bytes, w->sock.recv_packets);
#endif
        curr++;
        if (curr > last)
            curr = last;
        out->next = curr;
    }
}

/**
 * Compare Vector to store
 *
 * Compare input values with local address to select table to store.
 *
 * @param cmp     index read from hash table.
 *
 * @return It returns the structure with address to compare.
 */
netdata_vector_plot_t * select_vector_to_store(netdata_socket_idx_t *cmp)
{
    if (!listen_ports)
        return &outbound_vectors;

    ebpf_network_viewer_port_list_t *move_ports = listen_ports;
    while (move_ports) {
        if (move_ports->first == cmp->sport) {
            return &inbound_vectors;
        }

        move_ports = move_ports->next;
    }

    return &outbound_vectors;
}

static void hash_accumulator(netdata_socket_t *values, netdata_socket_idx_t *key, int *removesock, int family, int end)
{
    uint64_t bsent = 0, brecv = 0, psent = 0, precv = 0;
    int i;
    uint8_t protocol = values[0].protocol;
    for (i = 1; i < end; i++) {
        netdata_socket_t *w = &values[i];

        precv += w->recv_packets;
        psent += w->sent_packets;
        brecv += w->recv_bytes;
        bsent += w->sent_bytes;

        if (!protocol)
            protocol = w->protocol;

        *removesock += (int)w->removeme;
    }

    values[0].recv_packets += precv;
    values[0].sent_packets += psent;
    values[0].recv_bytes   += brecv;
    values[0].sent_bytes   += bsent;
    values[0].removeme     += *removesock;
    values[0].protocol = protocol;

    if (is_socket_allowed(key, family)) {
        netdata_vector_plot_t *table = select_vector_to_store(key);
        store_socket_inside_avl(table, values, key, family);
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
    netdata_socket_idx_t key = {};
    netdata_socket_idx_t next_key;
    netdata_socket_idx_t removeme;
    int removesock = 0;

    netdata_socket_t *values = socket_values;
    int test, end = (running_on_kernel < NETDATA_KERNEL_V4_15) ? 1 : ebpf_nprocs;

    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        test = bpf_map_lookup_elem(fd, &key, values);
        if (test < 0) {
            key = next_key;
            continue;
        }

        if (removesock)
            bpf_map_delete_elem(fd, &removeme);

        removesock = 0;
        hash_accumulator(values, &key, &removesock, family, end);

        if (removesock)
            removeme = key;

        key = next_key;
    }

    if (removesock)
        bpf_map_delete_elem(fd, &removeme);

    test = bpf_map_lookup_elem(fd, &next_key, values);
    if (test < 0) {
        return;
    }

    removesock = 0;
    hash_accumulator(values, &next_key, &removesock, family, end);

    if (removesock)
        bpf_map_delete_elem(fd, &next_key);
}

/**
 * Update listen table
 *
 * Update link list when it is necessary.
 *
 * @param value the ports we are listen to.
 */
void update_listen_table(uint16_t value)
{
    ebpf_network_viewer_port_list_t *w;
    if (likely(listen_ports)) {
        ebpf_network_viewer_port_list_t *move = listen_ports, *store = listen_ports;
        while (move) {
            if (move->first == value)
                return;

            store = move;
            move = move->next;
        }

        w = callocz(1, sizeof(ebpf_network_viewer_port_list_t));
        w->first = value;
        store->next = w;
    } else {
        w = callocz(1, sizeof(ebpf_network_viewer_port_list_t));
        w->first = value;

        listen_ports = w;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    info("The network viewer is monitoring inbound connections for port %u", ntohs(value));
#endif
}

/**
 * Read listen table
 *
 * Read the table with all ports that we are listen on host.
 */
static void read_listen_table()
{
    uint16_t key = 0;
    uint16_t next_key;

    int fd = map_fd[NETDATA_SOCKET_LISTEN_TABLE];
    uint8_t value;
    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        int test = bpf_map_lookup_elem(fd, &key, &value);
        if (test < 0) {
            key = next_key;
            continue;
        }

        update_listen_table(htons(key));

        key = next_key;
    }

    if (next_key)
        update_listen_table(htons(next_key));
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

        read_listen_table();
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
    int fd = map_fd[NETDATA_SOCKET_GLOBAL_HASH_TABLE];
    for (idx = 0; idx < NETDATA_SOCKET_COUNTER; idx++) {
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
    socket_aggregated_data[5].call = res[NETDATA_KEY_TCP_RETRANSMIT];

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
 * Clean internal socket plot
 *
 * Clean all structures allocated with strdupz.
 *
 * @param ptr the pointer with addresses to clean.
 */
static inline void clean_internal_socket_plot(netdata_socket_plot_t *ptr)
{
    freez(ptr->dimension_recv);
    freez(ptr->dimension_sent);
    freez(ptr->resolved_name);
}

/**
 * Clean socket plot
 *
 * Clean the allocated data for inbound and outbound vectors.
 */
static void clean_allocated_socket_plot()
{
    uint32_t i;
    uint32_t end = inbound_vectors.last;
    netdata_socket_plot_t *plot = inbound_vectors.plot;
    for (i = 0; i < end; i++) {
        clean_internal_socket_plot(&plot[i]);
    }

    clean_internal_socket_plot(&plot[inbound_vectors.last]);

    end = outbound_vectors.last;
    plot = outbound_vectors.plot;
    for (i = 0; i < end; i++) {
        clean_internal_socket_plot(&plot[i]);
    }
    clean_internal_socket_plot(&plot[outbound_vectors.last]);
}

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
    clean_allocated_socket_plot();
    freez(inbound_vectors.plot);
    freez(outbound_vectors.plot);

    clean_port_structure(&listen_ports);

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
    bandwidth_vector = callocz((size_t)ebpf_nprocs, sizeof(ebpf_bandwidth_t));

    socket_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_socket_t));
    inbound_vectors.plot = callocz(network_viewer_opt.max_dim, sizeof(netdata_socket_plot_t));
    outbound_vectors.plot = callocz(network_viewer_opt.max_dim, sizeof(netdata_socket_plot_t));
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
static void set_local_pointers(ebpf_module_t *em)
{
    map_fd = socket_data.map_fd;

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

    inbound_vectors.last = network_viewer_opt.max_dim - 1;
    outbound_vectors.last = inbound_vectors.last;

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
