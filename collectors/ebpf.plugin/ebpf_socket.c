// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_socket.h"

// ----------------------------------------------------------------------------
// ARAL vectors used to speed up processing

/*****************************************************************
 *
 *  GLOBAL VARIABLES
 *
 *****************************************************************/

static char *socket_dimension_names[NETDATA_MAX_SOCKET_VECTOR] = { "received", "sent", "close",
                                                                   "received", "sent", "retransmitted",
                                                                   "connected_V4", "connected_V6", "connected_tcp",
                                                                   "connected_udp"};
static char *socket_id_names[NETDATA_MAX_SOCKET_VECTOR] = { "tcp_cleanup_rbuf", "tcp_sendmsg",  "tcp_close",
                                                            "udp_recvmsg", "udp_sendmsg", "tcp_retransmit_skb",
                                                            "tcp_connect_v4", "tcp_connect_v6", "inet_csk_accept_tcp",
                                                            "inet_csk_accept_udp" };

static ebpf_local_maps_t socket_maps[] = {{.name = "tbl_bandwidth",
                                           .internal_input = NETDATA_COMPILED_CONNECTIONS_ALLOWED,
                                           .user_input = NETDATA_MAXIMUM_CONNECTIONS_ALLOWED,
                                           .type = NETDATA_EBPF_MAP_RESIZABLE | NETDATA_EBPF_MAP_PID,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                           .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
                                          },
                                          {.name = "tbl_global_sock",
                                           .internal_input = NETDATA_SOCKET_COUNTER,
                                           .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                           .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                          },
                                          {.name = "tbl_lports",
                                           .internal_input = NETDATA_SOCKET_COUNTER,
                                           .user_input = 0, .type = NETDATA_EBPF_MAP_STATIC,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                           .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
                                           },
                                          {.name = "tbl_conn_ipv4",
                                           .internal_input = NETDATA_COMPILED_CONNECTIONS_ALLOWED,
                                           .user_input = NETDATA_MAXIMUM_CONNECTIONS_ALLOWED,
                                           .type = NETDATA_EBPF_MAP_STATIC,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                           .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
                                          },
                                          {.name = "tbl_conn_ipv6",
                                           .internal_input = NETDATA_COMPILED_CONNECTIONS_ALLOWED,
                                           .user_input = NETDATA_MAXIMUM_CONNECTIONS_ALLOWED,
                                           .type = NETDATA_EBPF_MAP_STATIC,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                           .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
                                          },
                                          {.name = "tbl_nv_udp",
                                           .internal_input = NETDATA_COMPILED_UDP_CONNECTIONS_ALLOWED,
                                           .user_input = NETDATA_MAXIMUM_UDP_CONNECTIONS_ALLOWED,
                                           .type = NETDATA_EBPF_MAP_STATIC,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                           .map_type = BPF_MAP_TYPE_PERCPU_HASH
#endif
                                          },
                                          {.name = "socket_ctrl", .internal_input = NETDATA_CONTROLLER_END,
                                           .user_input = 0,
                                           .type = NETDATA_EBPF_MAP_CONTROLLER,
                                           .map_fd = ND_EBPF_MAP_FD_NOT_INITIALIZED,
#ifdef LIBBPF_MAJOR_VERSION
                                           .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                          },
                                          {.name = NULL, .internal_input = 0, .user_input = 0,
#ifdef LIBBPF_MAJOR_VERSION
        .map_type = BPF_MAP_TYPE_PERCPU_ARRAY
#endif
                                          }};

static netdata_idx_t *socket_hash_values = NULL;
static netdata_syscall_stat_t socket_aggregated_data[NETDATA_MAX_SOCKET_VECTOR];
static netdata_publish_syscall_t socket_publish_aggregated[NETDATA_MAX_SOCKET_VECTOR];

static ebpf_bandwidth_t *bandwidth_vector = NULL;

pthread_mutex_t nv_mutex;
netdata_vector_plot_t inbound_vectors = { .plot = NULL, .next = 0, .last = 0 };
netdata_vector_plot_t outbound_vectors = { .plot = NULL, .next = 0, .last = 0 };
netdata_socket_t *socket_values;

ebpf_network_viewer_port_list_t *listen_ports = NULL;

struct config socket_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

netdata_ebpf_targets_t socket_targets[] = { {.name = "inet_csk_accept", .mode = EBPF_LOAD_TRAMPOLINE},
                                            {.name = "tcp_retransmit_skb", .mode = EBPF_LOAD_TRAMPOLINE},
                                            {.name = "tcp_cleanup_rbuf", .mode = EBPF_LOAD_TRAMPOLINE},
                                            {.name = "tcp_close", .mode = EBPF_LOAD_TRAMPOLINE},
                                            {.name = "udp_recvmsg", .mode = EBPF_LOAD_TRAMPOLINE},
                                            {.name = "tcp_sendmsg", .mode = EBPF_LOAD_TRAMPOLINE},
                                            {.name = "udp_sendmsg", .mode = EBPF_LOAD_TRAMPOLINE},
                                            {.name = "tcp_v4_connect", .mode = EBPF_LOAD_TRAMPOLINE},
                                            {.name = "tcp_v6_connect", .mode = EBPF_LOAD_TRAMPOLINE},
                                            {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

struct netdata_static_thread socket_threads = {
    .name = "EBPF SOCKET READ",
    .config_section = NULL,
    .config_name = NULL,
    .env_name = NULL,
    .enabled = 1,
    .thread = NULL,
    .init_routine = NULL,
    .start_routine = NULL
};

#ifdef LIBBPF_MAJOR_VERSION
/**
 * Disable Probe
 *
 * Disable probes to use trampoline.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_socket_disable_probes(struct socket_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_inet_csk_accept_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_v4_connect_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_v6_connect_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_retransmit_skb_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_cleanup_rbuf_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_close_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_udp_recvmsg_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_udp_recvmsg_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_sendmsg_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_sendmsg_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_udp_sendmsg_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_udp_sendmsg_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_socket_release_task_kprobe, false);
}

/**
 * Disable Trampoline
 *
 * Disable trampoline to use probes.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_socket_disable_trampoline(struct socket_bpf *obj)
{
    bpf_program__set_autoload(obj->progs.netdata_inet_csk_accept_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_v4_connect_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_v6_connect_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_retransmit_skb_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_cleanup_rbuf_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_close_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_udp_recvmsg_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_udp_recvmsg_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_sendmsg_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_sendmsg_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_udp_sendmsg_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_udp_sendmsg_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_socket_release_task_fentry, false);
}

/**
 *  Set trampoline target.
 *
 *  @param obj is the main structure for bpf objects.
 */
static void ebpf_set_trampoline_target(struct socket_bpf *obj)
{
    bpf_program__set_attach_target(obj->progs.netdata_inet_csk_accept_fentry, 0,
                                   socket_targets[NETDATA_FCNT_INET_CSK_ACCEPT].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_v4_connect_fexit, 0,
                                   socket_targets[NETDATA_FCNT_TCP_V4_CONNECT].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_v6_connect_fexit, 0,
                                   socket_targets[NETDATA_FCNT_TCP_V6_CONNECT].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_retransmit_skb_fentry, 0,
                                   socket_targets[NETDATA_FCNT_TCP_RETRANSMIT].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_cleanup_rbuf_fentry, 0,
                                   socket_targets[NETDATA_FCNT_CLEANUP_RBUF].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_close_fentry, 0, socket_targets[NETDATA_FCNT_TCP_CLOSE].name);

    bpf_program__set_attach_target(obj->progs.netdata_udp_recvmsg_fentry, 0,
                                   socket_targets[NETDATA_FCNT_UDP_RECEVMSG].name);

    bpf_program__set_attach_target(obj->progs.netdata_udp_recvmsg_fexit, 0,
                                   socket_targets[NETDATA_FCNT_UDP_RECEVMSG].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_sendmsg_fentry, 0,
                                   socket_targets[NETDATA_FCNT_TCP_SENDMSG].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_sendmsg_fexit, 0,
                                   socket_targets[NETDATA_FCNT_TCP_SENDMSG].name);

    bpf_program__set_attach_target(obj->progs.netdata_udp_sendmsg_fentry, 0,
                                   socket_targets[NETDATA_FCNT_UDP_SENDMSG].name);

    bpf_program__set_attach_target(obj->progs.netdata_udp_sendmsg_fexit, 0,
                                   socket_targets[NETDATA_FCNT_UDP_SENDMSG].name);

    bpf_program__set_attach_target(obj->progs.netdata_socket_release_task_fentry, 0, EBPF_COMMON_FNCT_CLEAN_UP);
}


/**
 * Disable specific trampoline
 *
 * Disable specific trampoline to match user selection.
 *
 * @param obj is the main structure for bpf objects.
 * @param sel option selected by user.
 */
static inline void ebpf_socket_disable_specific_trampoline(struct socket_bpf *obj, netdata_run_mode_t sel)
{
    if (sel == MODE_RETURN) {
        bpf_program__set_autoload(obj->progs.netdata_tcp_sendmsg_fentry, false);
        bpf_program__set_autoload(obj->progs.netdata_udp_sendmsg_fentry, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_tcp_sendmsg_fexit, false);
        bpf_program__set_autoload(obj->progs.netdata_udp_sendmsg_fexit, false);
    }
}

/**
 * Disable specific probe
 *
 * Disable specific probe to match user selection.
 *
 * @param obj is the main structure for bpf objects.
 * @param sel option selected by user.
 */
static inline void ebpf_socket_disable_specific_probe(struct socket_bpf *obj, netdata_run_mode_t sel)
{
    if (sel == MODE_RETURN) {
        bpf_program__set_autoload(obj->progs.netdata_tcp_sendmsg_kprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_udp_sendmsg_kprobe, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_tcp_sendmsg_kretprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_udp_sendmsg_kretprobe, false);
    }
}

/**
 * Attach probes
 *
 * Attach probes to targets.
 *
 * @param obj is the main structure for bpf objects.
 * @param sel option selected by user.
 */
static int ebpf_socket_attach_probes(struct socket_bpf *obj, netdata_run_mode_t sel)
{
    obj->links.netdata_inet_csk_accept_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_inet_csk_accept_kretprobe,
                                                                              true,
                                                                              socket_targets[NETDATA_FCNT_INET_CSK_ACCEPT].name);
    int ret = libbpf_get_error(obj->links.netdata_inet_csk_accept_kretprobe);
    if (ret)
            return -1;

    obj->links.netdata_tcp_v4_connect_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_tcp_v4_connect_kretprobe,
                                                                             true,
                                                                             socket_targets[NETDATA_FCNT_TCP_V4_CONNECT].name);
    ret = libbpf_get_error(obj->links.netdata_tcp_v4_connect_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_tcp_v6_connect_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_tcp_v6_connect_kretprobe,
                                                                             true,
                                                                             socket_targets[NETDATA_FCNT_TCP_V6_CONNECT].name);
    ret = libbpf_get_error(obj->links.netdata_tcp_v6_connect_kretprobe);
    if (ret)
        return -1;

    obj->links.netdata_tcp_retransmit_skb_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_tcp_retransmit_skb_kprobe,
                                                                              false,
                                                                              socket_targets[NETDATA_FCNT_TCP_RETRANSMIT].name);
    ret = libbpf_get_error(obj->links.netdata_tcp_retransmit_skb_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_tcp_cleanup_rbuf_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_tcp_cleanup_rbuf_kprobe,
                                                                            false,
                                                                            socket_targets[NETDATA_FCNT_CLEANUP_RBUF].name);
    ret = libbpf_get_error(obj->links.netdata_tcp_cleanup_rbuf_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_tcp_close_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_tcp_close_kprobe,
                                                                     false,
                                                                     socket_targets[NETDATA_FCNT_TCP_CLOSE].name);
    ret = libbpf_get_error(obj->links.netdata_tcp_close_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_udp_recvmsg_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_udp_recvmsg_kprobe,
                                                                       false,
                                                                       socket_targets[NETDATA_FCNT_UDP_RECEVMSG].name);
    ret = libbpf_get_error(obj->links.netdata_udp_recvmsg_kprobe);
    if (ret)
        return -1;

    obj->links.netdata_udp_recvmsg_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_udp_recvmsg_kretprobe,
                                                                          true,
                                                                          socket_targets[NETDATA_FCNT_UDP_RECEVMSG].name);
    ret = libbpf_get_error(obj->links.netdata_udp_recvmsg_kretprobe);
    if (ret)
        return -1;

    if (sel == MODE_RETURN) {
        obj->links.netdata_tcp_sendmsg_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_tcp_sendmsg_kretprobe,
                                                                              true,
                                                                              socket_targets[NETDATA_FCNT_TCP_SENDMSG].name);
        ret = libbpf_get_error(obj->links.netdata_tcp_sendmsg_kretprobe);
        if (ret)
            return -1;

        obj->links.netdata_udp_sendmsg_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_udp_sendmsg_kretprobe,
                                                                              true,
                                                                              socket_targets[NETDATA_FCNT_UDP_SENDMSG].name);
        ret = libbpf_get_error(obj->links.netdata_udp_sendmsg_kretprobe);
        if (ret)
            return -1;
    } else {
        obj->links.netdata_tcp_sendmsg_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_tcp_sendmsg_kprobe,
                                                                           false,
                                                                           socket_targets[NETDATA_FCNT_TCP_SENDMSG].name);
        ret = libbpf_get_error(obj->links.netdata_tcp_sendmsg_kprobe);
        if (ret)
            return -1;

        obj->links.netdata_udp_sendmsg_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_udp_sendmsg_kprobe,
                                                                           false,
                                                                           socket_targets[NETDATA_FCNT_UDP_SENDMSG].name);
        ret = libbpf_get_error(obj->links.netdata_udp_sendmsg_kprobe);
        if (ret)
            return -1;
    }

    obj->links.netdata_socket_release_task_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_socket_release_task_kprobe,
                                                                               false,  EBPF_COMMON_FNCT_CLEAN_UP);
    ret = libbpf_get_error(obj->links.netdata_socket_release_task_kprobe);
    if (ret)
        return -1;

    return 0;
}

/**
 * Set hash tables
 *
 * Set the values for maps according the value given by kernel.
 *
 * @param obj is the main structure for bpf objects.
 */
static void ebpf_socket_set_hash_tables(struct socket_bpf *obj)
{
    socket_maps[NETDATA_SOCKET_TABLE_BANDWIDTH].map_fd = bpf_map__fd(obj->maps.tbl_bandwidth);
    socket_maps[NETDATA_SOCKET_GLOBAL].map_fd = bpf_map__fd(obj->maps.tbl_global_sock);
    socket_maps[NETDATA_SOCKET_LPORTS].map_fd = bpf_map__fd(obj->maps.tbl_lports);
    socket_maps[NETDATA_SOCKET_TABLE_IPV4].map_fd = bpf_map__fd(obj->maps.tbl_conn_ipv4);
    socket_maps[NETDATA_SOCKET_TABLE_IPV6].map_fd = bpf_map__fd(obj->maps.tbl_conn_ipv6);
    socket_maps[NETDATA_SOCKET_TABLE_UDP].map_fd = bpf_map__fd(obj->maps.tbl_nv_udp);
    socket_maps[NETDATA_SOCKET_TABLE_CTRL].map_fd = bpf_map__fd(obj->maps.socket_ctrl);
}

/**
 * Adjust Map Size
 *
 * Resize maps according input from users.
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 */
static void ebpf_socket_adjust_map(struct socket_bpf *obj, ebpf_module_t *em)
{
    ebpf_update_map_size(obj->maps.tbl_bandwidth, &socket_maps[NETDATA_SOCKET_TABLE_BANDWIDTH],
                         em, bpf_map__name(obj->maps.tbl_bandwidth));

    ebpf_update_map_size(obj->maps.tbl_conn_ipv4, &socket_maps[NETDATA_SOCKET_TABLE_IPV4],
                         em, bpf_map__name(obj->maps.tbl_conn_ipv4));

    ebpf_update_map_size(obj->maps.tbl_conn_ipv6, &socket_maps[NETDATA_SOCKET_TABLE_IPV6],
                         em, bpf_map__name(obj->maps.tbl_conn_ipv6));

    ebpf_update_map_size(obj->maps.tbl_nv_udp, &socket_maps[NETDATA_SOCKET_TABLE_UDP],
                         em, bpf_map__name(obj->maps.tbl_nv_udp));


    ebpf_update_map_type(obj->maps.tbl_bandwidth, &socket_maps[NETDATA_SOCKET_TABLE_BANDWIDTH]);
    ebpf_update_map_type(obj->maps.tbl_conn_ipv4, &socket_maps[NETDATA_SOCKET_TABLE_IPV4]);
    ebpf_update_map_type(obj->maps.tbl_conn_ipv6, &socket_maps[NETDATA_SOCKET_TABLE_IPV6]);
    ebpf_update_map_type(obj->maps.tbl_nv_udp, &socket_maps[NETDATA_SOCKET_TABLE_UDP]);
    ebpf_update_map_type(obj->maps.socket_ctrl, &socket_maps[NETDATA_SOCKET_TABLE_CTRL]);
    ebpf_update_map_type(obj->maps.tbl_global_sock, &socket_maps[NETDATA_SOCKET_GLOBAL]);
    ebpf_update_map_type(obj->maps.tbl_lports, &socket_maps[NETDATA_SOCKET_LPORTS]);
}

/**
 * Load and attach
 *
 * Load and attach the eBPF code in kernel.
 *
 * @param obj is the main structure for bpf objects.
 * @param em  structure with configuration
 *
 * @return it returns 0 on success and -1 otherwise
 */
static inline int ebpf_socket_load_and_attach(struct socket_bpf *obj, ebpf_module_t *em)
{
    netdata_ebpf_targets_t *mt = em->targets;
    netdata_ebpf_program_loaded_t test = mt[NETDATA_FCNT_INET_CSK_ACCEPT].mode;

    if (test == EBPF_LOAD_TRAMPOLINE) {
        ebpf_socket_disable_probes(obj);

        ebpf_set_trampoline_target(obj);
        ebpf_socket_disable_specific_trampoline(obj, em->mode);
    } else { // We are not using tracepoints for this thread.
        ebpf_socket_disable_trampoline(obj);

        ebpf_socket_disable_specific_probe(obj, em->mode);
    }

    ebpf_socket_adjust_map(obj, em);

    int ret = socket_bpf__load(obj);
    if (ret) {
        fprintf(stderr, "failed to load BPF object: %d\n", ret);
        return ret;
    }

    if (test == EBPF_LOAD_TRAMPOLINE) {
        ret = socket_bpf__attach(obj);
    } else {
        ret = ebpf_socket_attach_probes(obj, em->mode);
    }

    if (!ret) {
        ebpf_socket_set_hash_tables(obj);

        ebpf_update_controller(socket_maps[NETDATA_SOCKET_TABLE_CTRL].map_fd, em);
    }

    return ret;
}
#endif

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
    freez(ptr->dimension_retransmit);
}

/**
 * Clean socket plot
 *
 * Clean the allocated data for inbound and outbound vectors.
static void clean_allocated_socket_plot()
{
    if (!network_viewer_opt.enabled)
        return;

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
 */

/**
 * Clean network ports allocated during initialization.
 *
 * @param ptr a pointer to the link list.
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
 */

/**
 * Clean service names
 *
 * Clean the allocated link list that stores names.
 *
 * @param names the link list.
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
 */

/**
 * Clean hostnames
 *
 * @param hostnames the hostnames to clean
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
 */

/**
 * Clean port Structure
 *
 * Clean the allocated list.
 *
 * @param clean the list that will be cleaned
 */
void clean_port_structure(ebpf_network_viewer_port_list_t **clean)
{
    ebpf_network_viewer_port_list_t *move = *clean;
    while (move) {
        ebpf_network_viewer_port_list_t *next = move->next;
        freez(move->value);
        freez(move);

        move = next;
    }
    *clean = NULL;
}

/**
 * Clean IP structure
 *
 * Clean the allocated list.
 *
 * @param clean the list that will be cleaned
 */
static void clean_ip_structure(ebpf_network_viewer_ip_list_t **clean)
{
    ebpf_network_viewer_ip_list_t *move = *clean;
    while (move) {
        ebpf_network_viewer_ip_list_t *next = move->next;
        freez(move->value);
        freez(move);

        move = next;
    }
    *clean = NULL;
}

/**
 * Socket Free
 *
 * Cleanup variables after child threads to stop
 *
 * @param ptr thread data.
 */
static void ebpf_socket_free(ebpf_module_t *em )
{
    /* We can have thousands of sockets to clean, so we are transferring
     * for OS the responsibility while we do not use ARAL here
    freez(socket_hash_values);

    freez(bandwidth_vector);

    freez(socket_values);
    clean_allocated_socket_plot();
    freez(inbound_vectors.plot);
    freez(outbound_vectors.plot);

    clean_port_structure(&listen_ports);

    clean_network_ports(network_viewer_opt.included_port);
    clean_network_ports(network_viewer_opt.excluded_port);
    clean_service_names(network_viewer_opt.names);
    clean_hostnames(network_viewer_opt.included_hostnames);
    clean_hostnames(network_viewer_opt.excluded_hostnames);
     */

    pthread_mutex_destroy(&nv_mutex);

    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    pthread_mutex_unlock(&ebpf_exit_cleanup);
}

/**
 * Socket exit
 *
 * Clean up the main thread.
 *
 * @param ptr thread data.
 */
static void ebpf_socket_exit(void *ptr)
{
    ebpf_module_t *em = (ebpf_module_t *)ptr;
    pthread_mutex_lock(&nv_mutex);
    if (socket_threads.thread)
        netdata_thread_cancel(*socket_threads.thread);
    pthread_mutex_unlock(&nv_mutex);
    ebpf_socket_free(em);
}

/**
 * Socket cleanup
 *
 * Clean up allocated addresses.
 *
 * @param ptr thread data.
 */
void ebpf_socket_cleanup(void *ptr)
{
    UNUSED(ptr);
}

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

    tcp->write = -(long)publish[0].nbyte;
    tcp->read = (long)publish[1].nbyte;

    udp->write = -(long)publish[3].nbyte;
    udp->read = (long)publish[4].nbyte;
}

/**
 * Update Network Viewer plot data
 *
 * @param plot  the structure where the data will be stored
 * @param sock  the last update from the socket
 */
static inline void update_nv_plot_data(netdata_plot_values_t *plot, netdata_socket_t *sock)
{
    if (sock->ct != plot->last_time) {
        plot->last_time         = sock->ct;
        plot->plot_recv_packets = sock->recv_packets;
        plot->plot_sent_packets = sock->sent_packets;
        plot->plot_recv_bytes   = sock->recv_bytes;
        plot->plot_sent_bytes   = sock->sent_bytes;
        plot->plot_retransmit   = sock->retransmit;
    }

    sock->recv_packets = 0;
    sock->sent_packets = 0;
    sock->recv_bytes   = 0;
    sock->sent_bytes   = 0;
    sock->retransmit   = 0;
}

/**
 * Calculate Network Viewer Plot
 *
 * Do math with collected values before to plot data.
 */
static inline void calculate_nv_plot()
{
    pthread_mutex_lock(&nv_mutex);
    uint32_t i;
    uint32_t end = inbound_vectors.next;
    for (i = 0; i < end; i++) {
        update_nv_plot_data(&inbound_vectors.plot[i].plot, &inbound_vectors.plot[i].sock);
    }
    inbound_vectors.max_plot = end;

    // The 'Other' dimension is always calculated for the chart to have at least one dimension
    update_nv_plot_data(&inbound_vectors.plot[inbound_vectors.last].plot,
                        &inbound_vectors.plot[inbound_vectors.last].sock);

    end = outbound_vectors.next;
    for (i = 0; i < end; i++) {
        update_nv_plot_data(&outbound_vectors.plot[i].plot, &outbound_vectors.plot[i].sock);
    }
    outbound_vectors.max_plot = end;

    /*
    // The 'Other' dimension is always calculated for the chart to have at least one dimension
    update_nv_plot_data(&outbound_vectors.plot[outbound_vectors.last].plot,
                        &outbound_vectors.plot[outbound_vectors.last].sock);
                        */
    pthread_mutex_unlock(&nv_mutex);
}

/**
 * Network viewer send bytes
 *
 * @param ptr   the structure with values to plot
 * @param chart the chart name.
 */
static inline void ebpf_socket_nv_send_bytes(netdata_vector_plot_t *ptr, char *chart)
{
    uint32_t i;
    uint32_t end = ptr->last_plot;
    netdata_socket_plot_t *w = ptr->plot;
    collected_number value;

    write_begin_chart(NETDATA_EBPF_FAMILY, chart);
    for (i = 0; i < end; i++) {
        value = ((collected_number) w[i].plot.plot_sent_bytes);
        write_chart_dimension(w[i].dimension_sent, value);
        value = (collected_number) w[i].plot.plot_recv_bytes;
        write_chart_dimension(w[i].dimension_recv, value);
    }

    i = ptr->last;
    value = ((collected_number) w[i].plot.plot_sent_bytes);
    write_chart_dimension(w[i].dimension_sent, value);
    value = (collected_number) w[i].plot.plot_recv_bytes;
    write_chart_dimension(w[i].dimension_recv, value);
    write_end_chart();
}

/**
 * Network Viewer Send packets
 *
 * @param ptr   the structure with values to plot
 * @param chart the chart name.
 */
static inline void ebpf_socket_nv_send_packets(netdata_vector_plot_t *ptr, char *chart)
{
    uint32_t i;
    uint32_t end = ptr->last_plot;
    netdata_socket_plot_t *w = ptr->plot;
    collected_number value;

    write_begin_chart(NETDATA_EBPF_FAMILY, chart);
    for (i = 0; i < end; i++) {
        value = ((collected_number)w[i].plot.plot_sent_packets);
        write_chart_dimension(w[i].dimension_sent, value);
        value = (collected_number) w[i].plot.plot_recv_packets;
        write_chart_dimension(w[i].dimension_recv, value);
    }

    i = ptr->last;
    value = ((collected_number)w[i].plot.plot_sent_packets);
    write_chart_dimension(w[i].dimension_sent, value);
    value = (collected_number)w[i].plot.plot_recv_packets;
    write_chart_dimension(w[i].dimension_recv, value);
    write_end_chart();
}

/**
 * Network Viewer Send Retransmit
 *
 * @param ptr   the structure with values to plot
 * @param chart the chart name.
 */
static inline void ebpf_socket_nv_send_retransmit(netdata_vector_plot_t *ptr, char *chart)
{
    uint32_t i;
    uint32_t end = ptr->last_plot;
    netdata_socket_plot_t *w = ptr->plot;
    collected_number value;

    write_begin_chart(NETDATA_EBPF_FAMILY, chart);
    for (i = 0; i < end; i++) {
        value = (collected_number) w[i].plot.plot_retransmit;
        write_chart_dimension(w[i].dimension_retransmit, value);
    }

    i = ptr->last;
    value = (collected_number)w[i].plot.plot_retransmit;
    write_chart_dimension(w[i].dimension_retransmit, value);
    write_end_chart();
}

/**
 * Send network viewer data
 *
 * @param ptr the pointer to plot data
 */
static void ebpf_socket_send_nv_data(netdata_vector_plot_t *ptr)
{
    if (!ptr->flags)
        return;

    if (ptr == (netdata_vector_plot_t *)&outbound_vectors) {
        ebpf_socket_nv_send_bytes(ptr, NETDATA_NV_OUTBOUND_BYTES);
        fflush(stdout);

        ebpf_socket_nv_send_packets(ptr, NETDATA_NV_OUTBOUND_PACKETS);
        fflush(stdout);

        ebpf_socket_nv_send_retransmit(ptr,  NETDATA_NV_OUTBOUND_RETRANSMIT);
        fflush(stdout);
    } else {
        ebpf_socket_nv_send_bytes(ptr, NETDATA_NV_INBOUND_BYTES);
        fflush(stdout);

        ebpf_socket_nv_send_packets(ptr, NETDATA_NV_INBOUND_PACKETS);
        fflush(stdout);
    }
}

/**
 * Send Global Inbound connection
 *
 * Send number of connections read per protocol.
 */
static void ebpf_socket_send_global_inbound_conn()
{
    uint64_t udp_conn = 0;
    uint64_t tcp_conn = 0;
    ebpf_network_viewer_port_list_t *move = listen_ports;
    while (move) {
        if (move->protocol == IPPROTO_TCP)
            tcp_conn += move->connections;
        else
            udp_conn += move->connections;

        move = move->next;
    }

    write_begin_chart(NETDATA_EBPF_IP_FAMILY, NETDATA_INBOUND_CONNECTIONS);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_INCOMING_CONNECTION_TCP].name, (long long) tcp_conn);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_INCOMING_CONNECTION_UDP].name, (long long) udp_conn);
    write_end_chart();
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em the structure with thread information
 */
static void ebpf_socket_send_data(ebpf_module_t *em)
{
    netdata_publish_vfs_common_t common_tcp;
    netdata_publish_vfs_common_t common_udp;
    ebpf_update_global_publish(socket_publish_aggregated, &common_tcp, &common_udp, socket_aggregated_data);

    ebpf_socket_send_global_inbound_conn();
    write_count_chart(NETDATA_TCP_OUTBOUND_CONNECTIONS, NETDATA_EBPF_IP_FAMILY,
                      &socket_publish_aggregated[NETDATA_IDX_TCP_CONNECTION_V4], 2);

    // We read bytes from function arguments, but bandwidth is given in bits,
    // so we need to multiply by 8 to convert for the final value.
    write_count_chart(NETDATA_TCP_FUNCTION_COUNT, NETDATA_EBPF_IP_FAMILY, socket_publish_aggregated, 3);
    write_io_chart(NETDATA_TCP_FUNCTION_BITS, NETDATA_EBPF_IP_FAMILY, socket_id_names[0],
                   common_tcp.read * 8/BITS_IN_A_KILOBIT, socket_id_names[1],
                   common_tcp.write * 8/BITS_IN_A_KILOBIT);
    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_TCP_FUNCTION_ERROR, NETDATA_EBPF_IP_FAMILY, socket_publish_aggregated, 2);
    }
    write_count_chart(NETDATA_TCP_RETRANSMIT, NETDATA_EBPF_IP_FAMILY,
                      &socket_publish_aggregated[NETDATA_IDX_TCP_RETRANSMIT],1);

    write_count_chart(NETDATA_UDP_FUNCTION_COUNT, NETDATA_EBPF_IP_FAMILY,
                      &socket_publish_aggregated[NETDATA_IDX_UDP_RECVBUF],2);
    write_io_chart(NETDATA_UDP_FUNCTION_BITS, NETDATA_EBPF_IP_FAMILY,
                   socket_id_names[3], (long long)common_udp.read * 8/BITS_IN_A_KILOBIT,
                   socket_id_names[4], (long long)common_udp.write * 8/BITS_IN_A_KILOBIT);
    if (em->mode < MODE_ENTRY) {
        write_err_chart(NETDATA_UDP_FUNCTION_ERROR, NETDATA_EBPF_IP_FAMILY,
                        &socket_publish_aggregated[NETDATA_UDP_START], 2);
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
long long ebpf_socket_sum_values_for_pids(struct ebpf_pid_on_target *root, size_t offset)
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
 * Send data to Netdata calling auxiliary functions.
 *
 * @param em   the structure with thread information
 * @param root the target list.
 */
void ebpf_socket_send_apps_data(ebpf_module_t *em, struct ebpf_target *root)
{
    UNUSED(em);

    struct ebpf_target *w;
    collected_number value;

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_CONNECTION_TCP_V4);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t,
                                                                          call_tcp_v4_connection));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_CONNECTION_TCP_V6);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t,
                                                                          call_tcp_v6_connection));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_BANDWIDTH_SENT);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t,
                                                                          bytes_sent));
            // We multiply by 0.008, because we read bytes, but we display bits
            write_chart_dimension(w->name, ((value)*8)/1000);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_BANDWIDTH_RECV);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t,
                                                                          bytes_received));
            // We multiply by 0.008, because we read bytes, but we display bits
            write_chart_dimension(w->name, ((value)*8)/1000);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_BANDWIDTH_TCP_SEND_CALLS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t,
                                                                          call_tcp_sent));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_BANDWIDTH_TCP_RECV_CALLS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t,
                                                                          call_tcp_received));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_BANDWIDTH_TCP_RETRANSMIT);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t,
                                                                          retransmit));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_BANDWIDTH_UDP_SEND_CALLS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t,
                                                                          call_udp_sent));
            write_chart_dimension(w->name, value);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_APPS_FAMILY, NETDATA_NET_APPS_BANDWIDTH_UDP_RECV_CALLS);
    for (w = root; w; w = w->next) {
        if (unlikely(w->exposed && w->processes)) {
            value = ebpf_socket_sum_values_for_pids(w->root_pid, offsetof(ebpf_socket_publish_apps_t,
                                                                          call_udp_received));
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
    int order = 21070;
    ebpf_create_chart(NETDATA_EBPF_IP_FAMILY,
                      NETDATA_INBOUND_CONNECTIONS,
                      "Inbound connections.",
                      EBPF_COMMON_DIMENSION_CONNECTIONS,
                      NETDATA_SOCKET_KERNEL_FUNCTIONS,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      order++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_INCOMING_CONNECTION_TCP],
                      2, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(NETDATA_EBPF_IP_FAMILY,
                      NETDATA_TCP_OUTBOUND_CONNECTIONS,
                      "TCP outbound connections.",
                      EBPF_COMMON_DIMENSION_CONNECTIONS,
                      NETDATA_SOCKET_KERNEL_FUNCTIONS,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      order++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_TCP_CONNECTION_V4],
                      2, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);


    ebpf_create_chart(NETDATA_EBPF_IP_FAMILY,
                      NETDATA_TCP_FUNCTION_COUNT,
                      "Calls to internal functions",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_SOCKET_KERNEL_FUNCTIONS,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      order++,
                      ebpf_create_global_dimension,
                      socket_publish_aggregated,
                      3, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(NETDATA_EBPF_IP_FAMILY, NETDATA_TCP_FUNCTION_BITS,
                      "TCP bandwidth", EBPF_COMMON_DIMENSION_BITS,
                      NETDATA_SOCKET_KERNEL_FUNCTIONS,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      order++,
                      ebpf_create_global_dimension,
                      socket_publish_aggregated,
                      2, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_IP_FAMILY,
                          NETDATA_TCP_FUNCTION_ERROR,
                          "TCP errors",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_SOCKET_KERNEL_FUNCTIONS,
                          NULL,
                          NETDATA_EBPF_CHART_TYPE_LINE,
                          order++,
                          ebpf_create_global_dimension,
                          socket_publish_aggregated,
                          2, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);
    }

    ebpf_create_chart(NETDATA_EBPF_IP_FAMILY,
                      NETDATA_TCP_RETRANSMIT,
                      "Packages retransmitted",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_SOCKET_KERNEL_FUNCTIONS,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      order++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_TCP_RETRANSMIT],
                      1, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(NETDATA_EBPF_IP_FAMILY,
                      NETDATA_UDP_FUNCTION_COUNT,
                      "UDP calls",
                      EBPF_COMMON_DIMENSION_CALL,
                      NETDATA_SOCKET_KERNEL_FUNCTIONS,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      order++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_UDP_RECVBUF],
                      2, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(NETDATA_EBPF_IP_FAMILY, NETDATA_UDP_FUNCTION_BITS,
                      "UDP bandwidth", EBPF_COMMON_DIMENSION_BITS,
                      NETDATA_SOCKET_KERNEL_FUNCTIONS,
                      NULL,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      order++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_UDP_RECVBUF],
                      2, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    if (em->mode < MODE_ENTRY) {
        ebpf_create_chart(NETDATA_EBPF_IP_FAMILY,
                          NETDATA_UDP_FUNCTION_ERROR,
                          "UDP errors",
                          EBPF_COMMON_DIMENSION_CALL,
                          NETDATA_SOCKET_KERNEL_FUNCTIONS,
                          NULL,
                          NETDATA_EBPF_CHART_TYPE_LINE,
                          order++,
                          ebpf_create_global_dimension,
                          &socket_publish_aggregated[NETDATA_IDX_UDP_RECVBUF],
                          2, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);
    }
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em   a pointer to the structure with the default values.
 * @param ptr  a pointer for targets
 */
void ebpf_socket_create_apps_charts(struct ebpf_module *em, void *ptr)
{
    struct ebpf_target *root = ptr;
    int order = 20080;
    ebpf_create_charts_on_apps(NETDATA_NET_APPS_CONNECTION_TCP_V4,
                               "Calls to tcp_v4_connection", EBPF_COMMON_DIMENSION_CONNECTIONS,
                               NETDATA_APPS_NET_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               order++,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_charts_on_apps(NETDATA_NET_APPS_CONNECTION_TCP_V6,
                               "Calls to tcp_v6_connection", EBPF_COMMON_DIMENSION_CONNECTIONS,
                               NETDATA_APPS_NET_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               order++,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_SENT,
                               "Bytes sent", EBPF_COMMON_DIMENSION_BITS,
                               NETDATA_APPS_NET_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               order++,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_RECV,
                               "bytes received", EBPF_COMMON_DIMENSION_BITS,
                               NETDATA_APPS_NET_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               order++,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_TCP_SEND_CALLS,
                               "Calls for tcp_sendmsg",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_NET_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               order++,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_TCP_RECV_CALLS,
                               "Calls for tcp_cleanup_rbuf",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_NET_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               order++,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_TCP_RETRANSMIT,
                               "Calls for tcp_retransmit",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_NET_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               order++,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_UDP_SEND_CALLS,
                               "Calls for udp_sendmsg",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_NET_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               order++,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_charts_on_apps(NETDATA_NET_APPS_BANDWIDTH_UDP_RECV_CALLS,
                               "Calls for udp_recvmsg",
                               EBPF_COMMON_DIMENSION_CALL,
                               NETDATA_APPS_NET_GROUP,
                               NETDATA_EBPF_CHART_TYPE_STACKED,
                               order++,
                               ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                               root, em->update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    em->apps_charts |= NETDATA_EBPF_APPS_FLAG_CHART_CREATED;
}

/**
 *  Create network viewer chart
 *
 *  Create common charts.
 *
 * @param id            chart id
 * @param title         chart title
 * @param units         units label
 * @param family        group name used to attach the chart on dashboard
 * @param order         chart order
 * @param update_every value to overwrite the update frequency set by the server.
 * @param ptr          plot structure with values.
 */
static void ebpf_socket_create_nv_chart(char *id, char *title, char *units,
                                        char *family, int order, int update_every, netdata_vector_plot_t *ptr)
{
    ebpf_write_chart_cmd(NETDATA_EBPF_FAMILY,
                         id,
                         title,
                         units,
                         family,
                         NETDATA_EBPF_CHART_TYPE_STACKED,
                         NULL,
                         order,
                         update_every,
                         NETDATA_EBPF_MODULE_NAME_SOCKET);

    uint32_t i;
    uint32_t end = ptr->last_plot;
    netdata_socket_plot_t *w = ptr->plot;
    for (i = 0; i < end; i++) {
        fprintf(stdout, "DIMENSION %s '' incremental -1 1\n", w[i].dimension_sent);
        fprintf(stdout, "DIMENSION %s '' incremental 1 1\n", w[i].dimension_recv);
    }

    end = ptr->last;
    fprintf(stdout, "DIMENSION %s '' incremental -1 1\n", w[end].dimension_sent);
    fprintf(stdout, "DIMENSION %s '' incremental 1 1\n", w[end].dimension_recv);
}

/**
 *  Create network viewer retransmit
 *
 *  Create a specific chart.
 *
 * @param id        the chart id
 * @param title     the chart title
 * @param units     the units label
 * @param family    the group name used to attach the chart on dashboard
 * @param order     the chart order
 * @param update_every value to overwrite the update frequency set by the server.
 * @param ptr       the plot structure with values.
 */
static void ebpf_socket_create_nv_retransmit(char *id, char *title, char *units,
                                             char *family, int order, int update_every, netdata_vector_plot_t *ptr)
{
    ebpf_write_chart_cmd(NETDATA_EBPF_FAMILY,
                         id,
                         title,
                         units,
                         family,
                         NETDATA_EBPF_CHART_TYPE_STACKED,
                         NULL,
                         order,
                         update_every,
                         NETDATA_EBPF_MODULE_NAME_SOCKET);

    uint32_t i;
    uint32_t end = ptr->last_plot;
    netdata_socket_plot_t *w = ptr->plot;
    for (i = 0; i < end; i++) {
        fprintf(stdout, "DIMENSION %s '' incremental 1 1\n", w[i].dimension_retransmit);
    }

    end = ptr->last;
    fprintf(stdout, "DIMENSION %s '' incremental 1 1\n", w[end].dimension_retransmit);
}

/**
 * Create Network Viewer charts
 *
 * Recreate the charts when new sockets are created.
 *
 * @param ptr a pointer for inbound or outbound vectors.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_socket_create_nv_charts(netdata_vector_plot_t *ptr, int update_every)
{
    // We do not have new sockets, so we do not need move forward
    if (ptr->max_plot == ptr->last_plot)
        return;

    ptr->last_plot = ptr->max_plot;

    if (ptr == (netdata_vector_plot_t *)&outbound_vectors) {
        ebpf_socket_create_nv_chart(NETDATA_NV_OUTBOUND_BYTES,
                                    "Outbound connections (bytes).", EBPF_COMMON_DIMENSION_BYTES,
                                    NETDATA_NETWORK_CONNECTIONS_GROUP,
                                    21080,
                                    update_every, ptr);

        ebpf_socket_create_nv_chart(NETDATA_NV_OUTBOUND_PACKETS,
                                    "Outbound connections (packets)",
                                    EBPF_COMMON_DIMENSION_PACKETS,
                                    NETDATA_NETWORK_CONNECTIONS_GROUP,
                                    21082,
                                    update_every, ptr);

        ebpf_socket_create_nv_retransmit(NETDATA_NV_OUTBOUND_RETRANSMIT,
                                         "Retransmitted packets",
                                         EBPF_COMMON_DIMENSION_CALL,
                                         NETDATA_NETWORK_CONNECTIONS_GROUP,
                                         21083,
                                         update_every, ptr);
    } else {
        ebpf_socket_create_nv_chart(NETDATA_NV_INBOUND_BYTES,
                                    "Inbound connections (bytes)", EBPF_COMMON_DIMENSION_BYTES,
                                    NETDATA_NETWORK_CONNECTIONS_GROUP,
                                    21084,
                                    update_every, ptr);

        ebpf_socket_create_nv_chart(NETDATA_NV_INBOUND_PACKETS,
                                    "Inbound connections (packets)",
                                    EBPF_COMMON_DIMENSION_PACKETS,
                                    NETDATA_NETWORK_CONNECTIONS_GROUP,
                                    21085,
                                    update_every, ptr);
    }

    ptr->flags |= NETWORK_VIEWER_CHARTS_CREATED;
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
static int ebpf_is_specific_ip_inside_range(union netdata_ip_t *cmp, int family)
{
    if (!network_viewer_opt.excluded_ips && !network_viewer_opt.included_ips)
        return 1;

    uint32_t ipv4_test = htonl(cmp->addr32[0]);
    ebpf_network_viewer_ip_list_t *move = network_viewer_opt.excluded_ips;
    while (move) {
        if (family == AF_INET) {
            if (move->first.addr32[0] <= ipv4_test &&
                ipv4_test <= move->last.addr32[0])
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
        if (family == AF_INET && move->ver == AF_INET) {
            if (move->first.addr32[0] <= ipv4_test &&
                move->last.addr32[0] >= ipv4_test)
                return 1;
        } else {
            if (move->ver == AF_INET6 &&
                memcmp(move->first.addr8, cmp->addr8, sizeof(union netdata_ip_t)) <= 0 &&
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

    return ebpf_is_specific_ip_inside_range(&key->daddr, family);
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
static int ebpf_compare_sockets(void *a, void *b)
{
    struct netdata_socket_plot *val1 = a;
    struct netdata_socket_plot *val2 = b;
    int cmp = 0;

    // We do not need to compare val2 family, because data inside hash table is always from the same family
    if (val1->family == AF_INET) { //IPV4
        if (network_viewer_opt.included_port || network_viewer_opt.excluded_port)
            cmp = memcmp(&val1->index.dport, &val2->index.dport, sizeof(uint16_t));

        if (!cmp) {
            cmp = memcmp(&val1->index.daddr.addr32[0], &val2->index.daddr.addr32[0], sizeof(uint32_t));
        }
    } else {
        if (network_viewer_opt.included_port || network_viewer_opt.excluded_port)
            cmp = memcmp(&val1->index.dport, &val2->index.dport, sizeof(uint16_t));

        if (!cmp) {
            cmp = memcmp(&val1->index.daddr.addr32, &val2->index.daddr.addr32, 4*sizeof(uint32_t));
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
static inline int ebpf_build_outbound_dimension_name(char *dimname, char *hostname, char *service_name,
                                                     char *proto, int family)
{
    if (network_viewer_opt.included_port || network_viewer_opt.excluded_port)
        return snprintf(dimname, CONFIG_MAX_NAME - 7, (family == AF_INET)?"%s:%s:%s_":"%s:%s:[%s]_",
                        service_name, proto, hostname);

    return snprintf(dimname, CONFIG_MAX_NAME - 7, (family == AF_INET)?"%s:%s_":"%s:[%s]_",
                    proto, hostname);
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
        ptr->resolved_name = mallocz( NETDATA_MAX_NETWORK_COMBINED_LENGTH + 1);
        memcpy(ptr->resolved_name, hostname, length);
        ptr->resolved_name[length] = '\0';
    }

    char dimname[CONFIG_MAX_NAME];
    int size;
    char *protocol;
    if (ptr->sock.protocol == IPPROTO_UDP) {
        protocol = "UDP";
    } else if (ptr->sock.protocol == IPPROTO_TCP) {
        protocol = "TCP";
    } else {
        protocol = "ALL";
    }

    if (is_outbound)
        size = ebpf_build_outbound_dimension_name(dimname, hostname, service_name, protocol, ptr->family);
    else
        size = build_inbound_dimension_name(dimname,service_name, protocol);

    if (size > 0) {
        strcpy(&dimname[size], "sent");
        dimname[size + 4] = '\0';
        ptr->dimension_sent = strdupz(dimname);

        strcpy(&dimname[size], "recv");
        ptr->dimension_recv = strdupz(dimname);

        dimname[size - 1] = '\0';
        ptr->dimension_retransmit = strdupz(dimname);
    }
}

/**
 * Mount dimension names
 *
 * Fill the vector names after to resolve the addresses
 *
 * @param ptr a pointer to the structure where the values are stored.
 * @param is_outbound is a outbound ptr value?
 *
 * @return It returns 1 if the name is valid and 0 otherwise.
 */
int fill_names(netdata_socket_plot_t *ptr, int is_outbound)
{
    char hostname[NI_MAXHOST], service_name[NI_MAXSERV];
    if (ptr->resolved)
        return 1;

    int ret;
    static int resolve_name = -1;
    static int resolve_service = -1;
    if (resolve_name == -1)
        resolve_name = network_viewer_opt.hostname_resolution_enabled;

    if (resolve_service == -1)
        resolve_service = network_viewer_opt.service_resolution_enabled;

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

        if (!ret && !resolve_service) {
            snprintf(service_name, sizeof(service_name), "%u", ntohs(myaddr.sin_port));
        }

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

        if (!ret && !resolve_service) {
            snprintf(service_name, sizeof(service_name), "%u", ntohs(myaddr6.sin6_port));
        }

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

    if (resolve_name && !ret)
        ret = hostname_matches_pattern(hostname);

    ptr->resolved++;

    return ret;
}

/**
 * Fill last Network Viewer Dimension
 *
 * Fill the unique dimension that is always plotted.
 *
 * @param ptr           the pointer for the last dimension
 * @param is_outbound    is this an inbound structure?
 */
static void fill_last_nv_dimension(netdata_socket_plot_t *ptr, int is_outbound)
{
    char hostname[NI_MAXHOST], service_name[NI_MAXSERV];
    char *other = { "other" };
    // We are also copying the NULL bytes to avoid warnings in new compilers
    strncpy(hostname, other, 6);
    strncpy(service_name, other, 6);

    ptr->family = AF_INET;
    ptr->sock.protocol = 255;
    ptr->flags = (!is_outbound)?NETDATA_INBOUND_DIRECTION:NETDATA_OUTBOUND_DIRECTION;

    fill_resolved_name(ptr, hostname,  10 + NETDATA_DOTS_PROTOCOL_COMBINED_LENGTH, service_name, is_outbound);

#ifdef NETDATA_INTERNAL_CHECKS
    info("Last %s dimension added: ID = %u, IP = OTHER, NAME = %s, DIM1 = %s, DIM2 = %s, DIM3 = %s",
         (is_outbound)?"outbound":"inbound", network_viewer_opt.max_dim - 1, ptr->resolved_name,
         ptr->dimension_recv, ptr->dimension_sent, ptr->dimension_retransmit);
#endif
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
    sock->retransmit   = lvalues->retransmit;
    sock->ct = lvalues->ct;
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
 * @param flags   the connection flags
 */
static void store_socket_inside_avl(netdata_vector_plot_t *out, netdata_socket_t *lvalues,
                                    netdata_socket_idx_t *lindex, int family, uint32_t flags)
{
    netdata_socket_plot_t test, *ret ;

    memcpy(&test.index, lindex, sizeof(netdata_socket_idx_t));
    test.flags = flags;

    ret = (netdata_socket_plot_t *) avl_search_lock(&out->tree, (avl_t *)&test);
    if (ret) {
        if (lvalues->ct != ret->plot.last_time) {
            update_socket_data(&ret->sock, lvalues);
        }
    } else {
        uint32_t curr = out->next;
        uint32_t last = out->last;

        netdata_socket_plot_t *w = &out->plot[curr];

        int resolved;
        if (curr == last) {
            if (lvalues->ct != w->plot.last_time) {
                update_socket_data(&w->sock, lvalues);
            }
            return;
        } else {
            memcpy(&w->sock, lvalues, sizeof(netdata_socket_t));
            memcpy(&w->index, lindex, sizeof(netdata_socket_idx_t));
            w->family = family;

            resolved = fill_names(w, out != (netdata_vector_plot_t *)&inbound_vectors);
        }

        if (!resolved) {
            freez(w->resolved_name);
            freez(w->dimension_sent);
            freez(w->dimension_recv);
            freez(w->dimension_retransmit);

            memset(w, 0, sizeof(netdata_socket_plot_t));

            return;
        }

        w->flags = flags;
        netdata_socket_plot_t *check ;
        check = (netdata_socket_plot_t *) avl_insert_lock(&out->tree, (avl_t *)w);
        if (check != w)
            error("Internal error, cannot insert the AVL tree.");

#ifdef NETDATA_INTERNAL_CHECKS
        char iptext[INET6_ADDRSTRLEN];
        if (inet_ntop(family, &w->index.daddr.addr8, iptext, sizeof(iptext)))
            info("New %s dimension added: ID = %u, IP = %s, NAME = %s, DIM1 = %s, DIM2 = %s, DIM3 = %s",
                 (out == &inbound_vectors)?"inbound":"outbound", curr, iptext, w->resolved_name,
                 w->dimension_recv, w->dimension_sent, w->dimension_retransmit);
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
 * @param direction  store inbound and outbound direction.
 * @param cmp        index read from hash table.
 * @param proto      the protocol read.
 *
 * @return It returns the structure with address to compare.
 */
netdata_vector_plot_t * select_vector_to_store(uint32_t *direction, netdata_socket_idx_t *cmp, uint8_t proto)
{
    if (!listen_ports) {
        *direction = NETDATA_OUTBOUND_DIRECTION;
        return &outbound_vectors;
    }

    ebpf_network_viewer_port_list_t *move_ports = listen_ports;
    while (move_ports) {
        if (move_ports->protocol == proto && move_ports->first == cmp->sport) {
            *direction = NETDATA_INBOUND_DIRECTION;
            return &inbound_vectors;
        }

        move_ports = move_ports->next;
    }

    *direction = NETDATA_OUTBOUND_DIRECTION;
    return &outbound_vectors;
}

/**
 * Hash accumulator
 *
 * @param values        the values used to calculate the data.
 * @param key           the key to store  data.
 * @param family        the connection family
 * @param end           the values size.
 */
static void hash_accumulator(netdata_socket_t *values, netdata_socket_idx_t *key, int family, int end)
{
    if (!network_viewer_opt.enabled || !is_socket_allowed(key, family))
        return;

    uint64_t bsent = 0, brecv = 0, psent = 0, precv = 0;
    uint16_t retransmit = 0;
    int i;
    uint8_t protocol = values[0].protocol;
    uint64_t ct = values[0].ct;
    for (i = 1; i < end; i++) {
        netdata_socket_t *w = &values[i];

        precv += w->recv_packets;
        psent += w->sent_packets;
        brecv += w->recv_bytes;
        bsent += w->sent_bytes;
        retransmit += w->retransmit;

        if (!protocol)
            protocol = w->protocol;

        if (w->ct != ct)
            ct = w->ct;
    }

    values[0].recv_packets += precv;
    values[0].sent_packets += psent;
    values[0].recv_bytes   += brecv;
    values[0].sent_bytes   += bsent;
    values[0].retransmit   += retransmit;
    values[0].protocol     = (!protocol)?IPPROTO_TCP:protocol;
    values[0].ct           = ct;

    uint32_t dir;
    netdata_vector_plot_t *table = select_vector_to_store(&dir, key, protocol);
    store_socket_inside_avl(table, &values[0], key, family, dir);
}

/**
 * Read socket hash table
 *
 * Read data from hash tables created on kernel ring.
 *
 * @param fd                 the hash table with data.
 * @param family             the family associated to the hash table
 * @param maps_per_core      do I need to read all cores?
 *
 * @return it returns 0 on success and -1 otherwise.
 */
static void ebpf_read_socket_hash_table(int fd, int family, int maps_per_core)
{
    netdata_socket_idx_t key = {};
    netdata_socket_idx_t next_key = {};

    netdata_socket_t *values = socket_values;
    size_t length = sizeof(netdata_socket_t);
    int test, end;
    if (maps_per_core) {
        length *= ebpf_nprocs;
        end = ebpf_nprocs;
    } else
        end = 1;

    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        // We need to reset the values when we are working on kernel 4.15 or newer, because kernel does not create
        // values for specific processor unless it is used to store data. As result of this behavior one the next socket
        // can have values from the previous one.
        memset(values, 0, length);
        test = bpf_map_lookup_elem(fd, &key, values);
        if (test < 0) {
            key = next_key;
            continue;
        }

        hash_accumulator(values, &key, family, end);

        key = next_key;
    }
}

/**
 * Fill Network Viewer Port list
 *
 * Fill the structure with values read from /proc or hash table.
 *
 * @param out   the structure where we will store data.
 * @param value the ports we are listen to.
 * @param proto the protocol used for this connection.
 * @param in    the structure with values read form different sources.
 */
static inline void fill_nv_port_list(ebpf_network_viewer_port_list_t *out, uint16_t value, uint16_t proto,
                                     netdata_passive_connection_t *in)
{
    out->first = value;
    out->protocol = proto;
    out->pid = in->pid;
    out->tgid = in->tgid;
    out->connections = in->counter;
}

/**
 * Update listen table
 *
 * Update link list when it is necessary.
 *
 * @param value the ports we are listen to.
 * @param proto the protocol used with port connection.
 * @param in    the structure with values read form different sources.
 */
void update_listen_table(uint16_t value, uint16_t proto, netdata_passive_connection_t *in)
{
    ebpf_network_viewer_port_list_t *w;
    if (likely(listen_ports)) {
        ebpf_network_viewer_port_list_t *move = listen_ports, *store = listen_ports;
        while (move) {
            if (move->protocol == proto && move->first == value) {
                move->pid = in->pid;
                move->tgid = in->tgid;
                move->connections = in->counter;
                return;
            }

            store = move;
            move = move->next;
        }

        w = callocz(1, sizeof(ebpf_network_viewer_port_list_t));
        store->next = w;
    } else {
        w = callocz(1, sizeof(ebpf_network_viewer_port_list_t));

        listen_ports = w;
    }
    fill_nv_port_list(w, value, proto, in);

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
    netdata_passive_connection_idx_t key = {};
    netdata_passive_connection_idx_t next_key = {};

    int fd = socket_maps[NETDATA_SOCKET_LPORTS].map_fd;
    netdata_passive_connection_t value = {};
    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        int test = bpf_map_lookup_elem(fd, &key, &value);
        if (test < 0) {
            key = next_key;
            continue;
        }

        // The correct protocol must come from kernel
        update_listen_table(key.port, key.protocol, &value);

        key = next_key;
        memset(&value, 0, sizeof(value));
    }

    if (next_key.port && value.pid) {
        // The correct protocol must come from kernel
        update_listen_table(next_key.port, next_key.protocol, &value);
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
    netdata_thread_cleanup_push(ebpf_socket_cleanup, ptr);
    ebpf_module_t *em = (ebpf_module_t *)ptr;

    heartbeat_t hb;
    heartbeat_init(&hb);
    int fd_ipv4 = socket_maps[NETDATA_SOCKET_TABLE_IPV4].map_fd;
    int fd_ipv6 = socket_maps[NETDATA_SOCKET_TABLE_IPV6].map_fd;
    int maps_per_core = em->maps_per_core;
    // This thread is cancelled from another thread
    for (;;) {
        (void)heartbeat_next(&hb, USEC_PER_SEC);
        if (ebpf_exit_plugin)
           break;

        pthread_mutex_lock(&nv_mutex);
        ebpf_read_socket_hash_table(fd_ipv4, AF_INET, maps_per_core);
        ebpf_read_socket_hash_table(fd_ipv6, AF_INET6, maps_per_core);
        pthread_mutex_unlock(&nv_mutex);
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}

/**
 * Read the hash table and store data to allocated vectors.
 *
 * @param maps_per_core      do I need to read all cores?
 */
static void read_hash_global_tables(int maps_per_core)
{
    uint64_t idx;
    netdata_idx_t res[NETDATA_SOCKET_COUNTER];

    netdata_idx_t *val = socket_hash_values;
    size_t length = sizeof(netdata_idx_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    int fd = socket_maps[NETDATA_SOCKET_GLOBAL].map_fd;
    for (idx = 0; idx < NETDATA_SOCKET_COUNTER; idx++) {
        if (!bpf_map_lookup_elem(fd, &idx, val)) {
            uint64_t total = 0;
            int i;
            int end = (maps_per_core) ? ebpf_nprocs : 1;
            for (i = 0; i < end; i++)
                total += val[i];

            res[idx] = total;
            memset(socket_hash_values, 0, length);
        } else {
            res[idx] = 0;
        }
    }

    socket_aggregated_data[NETDATA_IDX_TCP_SENDMSG].call = res[NETDATA_KEY_CALLS_TCP_SENDMSG];
    socket_aggregated_data[NETDATA_IDX_TCP_CLEANUP_RBUF].call = res[NETDATA_KEY_CALLS_TCP_CLEANUP_RBUF];
    socket_aggregated_data[NETDATA_IDX_TCP_CLOSE].call = res[NETDATA_KEY_CALLS_TCP_CLOSE];
    socket_aggregated_data[NETDATA_IDX_UDP_RECVBUF].call = res[NETDATA_KEY_CALLS_UDP_RECVMSG];
    socket_aggregated_data[NETDATA_IDX_UDP_SENDMSG].call = res[NETDATA_KEY_CALLS_UDP_SENDMSG];
    socket_aggregated_data[NETDATA_IDX_TCP_RETRANSMIT].call = res[NETDATA_KEY_TCP_RETRANSMIT];
    socket_aggregated_data[NETDATA_IDX_TCP_CONNECTION_V4].call = res[NETDATA_KEY_CALLS_TCP_CONNECT_IPV4];
    socket_aggregated_data[NETDATA_IDX_TCP_CONNECTION_V6].call = res[NETDATA_KEY_CALLS_TCP_CONNECT_IPV6];

    socket_aggregated_data[NETDATA_IDX_TCP_SENDMSG].ecall = res[NETDATA_KEY_ERROR_TCP_SENDMSG];
    socket_aggregated_data[NETDATA_IDX_TCP_CLEANUP_RBUF].ecall = res[NETDATA_KEY_ERROR_TCP_CLEANUP_RBUF];
    socket_aggregated_data[NETDATA_IDX_UDP_RECVBUF].ecall = res[NETDATA_KEY_ERROR_UDP_RECVMSG];
    socket_aggregated_data[NETDATA_IDX_UDP_SENDMSG].ecall = res[NETDATA_KEY_ERROR_UDP_SENDMSG];
    socket_aggregated_data[NETDATA_IDX_TCP_CONNECTION_V4].ecall = res[NETDATA_KEY_ERROR_TCP_CONNECT_IPV4];
    socket_aggregated_data[NETDATA_IDX_TCP_CONNECTION_V6].ecall = res[NETDATA_KEY_ERROR_TCP_CONNECT_IPV6];

    socket_aggregated_data[NETDATA_IDX_TCP_SENDMSG].bytes = res[NETDATA_KEY_BYTES_TCP_SENDMSG];
    socket_aggregated_data[NETDATA_IDX_TCP_CLEANUP_RBUF].bytes = res[NETDATA_KEY_BYTES_TCP_CLEANUP_RBUF];
    socket_aggregated_data[NETDATA_IDX_UDP_RECVBUF].bytes = res[NETDATA_KEY_BYTES_UDP_RECVMSG];
    socket_aggregated_data[NETDATA_IDX_UDP_SENDMSG].bytes = res[NETDATA_KEY_BYTES_UDP_SENDMSG];
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
    if (!curr) {
        curr = ebpf_socket_stat_get();
        socket_bandwidth_curr[current_pid] = curr;
    }

    curr->bytes_sent = eb->bytes_sent;
    curr->bytes_received = eb->bytes_received;
    curr->call_tcp_sent = eb->call_tcp_sent;
    curr->call_tcp_received = eb->call_tcp_received;
    curr->retransmit = eb->retransmit;
    curr->call_udp_sent = eb->call_udp_sent;
    curr->call_udp_received = eb->call_udp_received;
    curr->call_close = eb->close;
    curr->call_tcp_v4_connection = eb->tcp_v4_connection;
    curr->call_tcp_v6_connection = eb->tcp_v6_connection;
}

/**
 * Bandwidth accumulator.
 *
 * @param out the vector with the values to sum
 */
void ebpf_socket_bandwidth_accumulator(ebpf_bandwidth_t *out, int maps_per_core)
{
    int i, end = (maps_per_core) ? ebpf_nprocs : 1;
    ebpf_bandwidth_t *total = &out[0];
    for (i = 1; i < end; i++) {
        ebpf_bandwidth_t *move = &out[i];
        total->bytes_sent += move->bytes_sent;
        total->bytes_received += move->bytes_received;
        total->call_tcp_sent += move->call_tcp_sent;
        total->call_tcp_received += move->call_tcp_received;
        total->retransmit += move->retransmit;
        total->call_udp_sent += move->call_udp_sent;
        total->call_udp_received += move->call_udp_received;
        total->close += move->close;
        total->tcp_v4_connection += move->tcp_v4_connection;
        total->tcp_v6_connection += move->tcp_v6_connection;
    }
}

/**
 *  Update the apps data reading information from the hash table
 *
 * @param maps_per_core      do I need to read all cores?
 */
static void ebpf_socket_update_apps_data(int maps_per_core)
{
    int fd = socket_maps[NETDATA_SOCKET_TABLE_BANDWIDTH].map_fd;
    ebpf_bandwidth_t *eb = bandwidth_vector;
    uint32_t key;
    struct ebpf_pid_stat *pids = ebpf_root_of_pids;
    size_t length = sizeof(ebpf_bandwidth_t);
    if (maps_per_core)
        length *= ebpf_nprocs;
    while (pids) {
        key = pids->pid;

        if (bpf_map_lookup_elem(fd, &key, eb)) {
            pids = pids->next;
            continue;
        }

        ebpf_socket_bandwidth_accumulator(eb, maps_per_core);

        ebpf_socket_fill_publish_apps(key, eb);

        memset(eb, 0, length);

        pids = pids->next;
    }
}

/**
 * Update cgroup
 *
 * Update cgroup data based in PIDs.
 *
 * @param maps_per_core      do I need to read all cores?
 */
static void ebpf_update_socket_cgroup(int maps_per_core)
{
    ebpf_cgroup_target_t *ect ;

    ebpf_bandwidth_t *eb = bandwidth_vector;
    int fd = socket_maps[NETDATA_SOCKET_TABLE_BANDWIDTH].map_fd;

    size_t length = sizeof(ebpf_bandwidth_t);
    if (maps_per_core)
        length *= ebpf_nprocs;

    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            int pid = pids->pid;
            ebpf_bandwidth_t *out = &pids->socket;
            ebpf_socket_publish_apps_t *publish = &ect->publish_socket;
            if (likely(socket_bandwidth_curr) && socket_bandwidth_curr[pid]) {
                ebpf_socket_publish_apps_t *in = socket_bandwidth_curr[pid];

                publish->bytes_sent = in->bytes_sent;
                publish->bytes_received = in->bytes_received;
                publish->call_tcp_sent = in->call_tcp_sent;
                publish->call_tcp_received = in->call_tcp_received;
                publish->retransmit = in->retransmit;
                publish->call_udp_sent = in->call_udp_sent;
                publish->call_udp_received = in->call_udp_received;
                publish->call_close = in->call_close;
                publish->call_tcp_v4_connection = in->call_tcp_v4_connection;
                publish->call_tcp_v6_connection = in->call_tcp_v6_connection;
            } else {
                if (!bpf_map_lookup_elem(fd, &pid, eb)) {
                    ebpf_socket_bandwidth_accumulator(eb, maps_per_core);

                    memcpy(out, eb, sizeof(ebpf_bandwidth_t));

                    publish->bytes_sent = out->bytes_sent;
                    publish->bytes_received = out->bytes_received;
                    publish->call_tcp_sent = out->call_tcp_sent;
                    publish->call_tcp_received = out->call_tcp_received;
                    publish->retransmit = out->retransmit;
                    publish->call_udp_sent = out->call_udp_sent;
                    publish->call_udp_received = out->call_udp_received;
                    publish->call_close = out->close;
                    publish->call_tcp_v4_connection = out->tcp_v4_connection;
                    publish->call_tcp_v6_connection = out->tcp_v6_connection;

                    memset(eb, 0, length);
                }
            }
        }
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Sum PIDs
 *
 * Sum values for all targets.
 *
 * @param fd  structure used to store data
 * @param pids input data
 */
static void ebpf_socket_sum_cgroup_pids(ebpf_socket_publish_apps_t *socket, struct pid_on_target2 *pids)
{
    ebpf_socket_publish_apps_t accumulator;
    memset(&accumulator, 0, sizeof(accumulator));

    while (pids) {
        ebpf_bandwidth_t *w = &pids->socket;

        accumulator.bytes_received += w->bytes_received;
        accumulator.bytes_sent += w->bytes_sent;
        accumulator.call_tcp_received += w->call_tcp_received;
        accumulator.call_tcp_sent += w->call_tcp_sent;
        accumulator.retransmit += w->retransmit;
        accumulator.call_udp_received += w->call_udp_received;
        accumulator.call_udp_sent += w->call_udp_sent;
        accumulator.call_close += w->close;
        accumulator.call_tcp_v4_connection += w->tcp_v4_connection;
        accumulator.call_tcp_v6_connection += w->tcp_v6_connection;

        pids = pids->next;
    }

    socket->bytes_sent = (accumulator.bytes_sent >= socket->bytes_sent) ? accumulator.bytes_sent : socket->bytes_sent;
    socket->bytes_received = (accumulator.bytes_received >= socket->bytes_received) ? accumulator.bytes_received : socket->bytes_received;
    socket->call_tcp_sent = (accumulator.call_tcp_sent >= socket->call_tcp_sent) ? accumulator.call_tcp_sent : socket->call_tcp_sent;
    socket->call_tcp_received = (accumulator.call_tcp_received >= socket->call_tcp_received) ? accumulator.call_tcp_received : socket->call_tcp_received;
    socket->retransmit = (accumulator.retransmit >= socket->retransmit) ? accumulator.retransmit : socket->retransmit;
    socket->call_udp_sent = (accumulator.call_udp_sent >= socket->call_udp_sent) ? accumulator.call_udp_sent : socket->call_udp_sent;
    socket->call_udp_received = (accumulator.call_udp_received >= socket->call_udp_received) ? accumulator.call_udp_received : socket->call_udp_received;
    socket->call_close = (accumulator.call_close >= socket->call_close) ? accumulator.call_close : socket->call_close;
    socket->call_tcp_v4_connection = (accumulator.call_tcp_v4_connection >= socket->call_tcp_v4_connection) ?
                                     accumulator.call_tcp_v4_connection : socket->call_tcp_v4_connection;
    socket->call_tcp_v6_connection = (accumulator.call_tcp_v6_connection >= socket->call_tcp_v6_connection) ?
                                     accumulator.call_tcp_v6_connection : socket->call_tcp_v6_connection;
}

/**
 * Create specific socket charts
 *
 * Create charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_create_specific_socket_charts(char *type, int update_every)
{
    int order_basis = 5300;
    ebpf_create_chart(type, NETDATA_NET_APPS_CONNECTION_TCP_V4,
                      "Calls to tcp_v4_connection",
                      EBPF_COMMON_DIMENSION_CONNECTIONS, NETDATA_CGROUP_NET_GROUP,
                      NETDATA_CGROUP_TCP_V4_CONN_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_TCP_CONNECTION_V4], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(type, NETDATA_NET_APPS_CONNECTION_TCP_V6,
                      "Calls to tcp_v6_connection",
                      EBPF_COMMON_DIMENSION_CONNECTIONS, NETDATA_CGROUP_NET_GROUP,
                      NETDATA_CGROUP_TCP_V6_CONN_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_TCP_CONNECTION_V6], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(type, NETDATA_NET_APPS_BANDWIDTH_RECV,
                      "Bytes received",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_CGROUP_NET_GROUP,
                      NETDATA_CGROUP_SOCKET_BYTES_RECV_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_TCP_CLEANUP_RBUF], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(type, NETDATA_NET_APPS_BANDWIDTH_SENT,
                      "Bytes sent",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_CGROUP_NET_GROUP,
                      NETDATA_CGROUP_SOCKET_BYTES_SEND_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++,
                      ebpf_create_global_dimension,
                      socket_publish_aggregated, 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(type, NETDATA_NET_APPS_BANDWIDTH_TCP_RECV_CALLS,
                      "Calls to tcp_cleanup_rbuf.",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_CGROUP_NET_GROUP,
                      NETDATA_CGROUP_SOCKET_TCP_RECV_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_TCP_CLEANUP_RBUF], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(type, NETDATA_NET_APPS_BANDWIDTH_TCP_SEND_CALLS,
                      "Calls to tcp_sendmsg.",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_CGROUP_NET_GROUP,
                      NETDATA_CGROUP_SOCKET_TCP_SEND_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++,
                      ebpf_create_global_dimension,
                      socket_publish_aggregated, 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(type, NETDATA_NET_APPS_BANDWIDTH_TCP_RETRANSMIT,
                      "Calls to tcp_retransmit.",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_CGROUP_NET_GROUP,
                      NETDATA_CGROUP_SOCKET_TCP_RETRANSMIT_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_TCP_RETRANSMIT], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(type, NETDATA_NET_APPS_BANDWIDTH_UDP_SEND_CALLS,
                      "Calls to udp_sendmsg",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_CGROUP_NET_GROUP,
                      NETDATA_CGROUP_SOCKET_UDP_SEND_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_UDP_SENDMSG], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);

    ebpf_create_chart(type, NETDATA_NET_APPS_BANDWIDTH_UDP_RECV_CALLS,
                      "Calls to udp_recvmsg",
                      EBPF_COMMON_DIMENSION_CALL, NETDATA_CGROUP_NET_GROUP,
                      NETDATA_CGROUP_SOCKET_UDP_RECV_CONTEXT,
                      NETDATA_EBPF_CHART_TYPE_LINE,
                      NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++,
                      ebpf_create_global_dimension,
                      &socket_publish_aggregated[NETDATA_IDX_UDP_RECVBUF], 1,
                      update_every, NETDATA_EBPF_MODULE_NAME_SOCKET);
}

/**
 * Obsolete specific socket charts
 *
 * Obsolete charts for cgroup/application.
 *
 * @param type the chart type.
 * @param update_every value to overwrite the update frequency set by the server.
 */
static void ebpf_obsolete_specific_socket_charts(char *type, int update_every)
{
    int order_basis = 5300;
    ebpf_write_chart_obsolete(type, NETDATA_NET_APPS_CONNECTION_TCP_V4, "Calls to tcp_v4_connection",
                              EBPF_COMMON_DIMENSION_CONNECTIONS, NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_SERVICES_SOCKET_TCP_V4_CONN_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_NET_APPS_CONNECTION_TCP_V6,"Calls to tcp_v6_connection",
                              EBPF_COMMON_DIMENSION_CONNECTIONS, NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_SERVICES_SOCKET_TCP_V6_CONN_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_NET_APPS_BANDWIDTH_RECV, "Bytes received",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_SERVICES_SOCKET_BYTES_RECV_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_NET_APPS_BANDWIDTH_SENT,"Bytes sent",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_SERVICES_SOCKET_BYTES_SEND_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_NET_APPS_BANDWIDTH_TCP_RECV_CALLS, "Calls to tcp_cleanup_rbuf.",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_SERVICES_SOCKET_TCP_RECV_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_NET_APPS_BANDWIDTH_TCP_SEND_CALLS, "Calls to tcp_sendmsg.",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_SERVICES_SOCKET_TCP_SEND_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_NET_APPS_BANDWIDTH_TCP_RETRANSMIT, "Calls to tcp_retransmit.",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_SERVICES_SOCKET_TCP_RETRANSMIT_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_NET_APPS_BANDWIDTH_UDP_SEND_CALLS, "Calls to udp_sendmsg",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_LINE, NETDATA_SERVICES_SOCKET_UDP_SEND_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++, update_every);

    ebpf_write_chart_obsolete(type, NETDATA_NET_APPS_BANDWIDTH_UDP_RECV_CALLS, "Calls to udp_recvmsg",
                              EBPF_COMMON_DIMENSION_CALL, NETDATA_APPS_NET_GROUP, NETDATA_EBPF_CHART_TYPE_LINE,
                              NETDATA_SERVICES_SOCKET_UDP_RECV_CONTEXT,
                              NETDATA_CHART_PRIO_CGROUPS_CONTAINERS + order_basis++, update_every);
}

/*
 * Send Specific Swap data
 *
 * Send data for specific cgroup/apps.
 *
 * @param type   chart type
 * @param values structure with values that will be sent to netdata
 */
static void ebpf_send_specific_socket_data(char *type, ebpf_socket_publish_apps_t *values)
{
    write_begin_chart(type, NETDATA_NET_APPS_CONNECTION_TCP_V4);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_TCP_CONNECTION_V4].name,
                          (long long) values->call_tcp_v4_connection);
    write_end_chart();

    write_begin_chart(type, NETDATA_NET_APPS_CONNECTION_TCP_V6);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_TCP_CONNECTION_V6].name,
                          (long long) values->call_tcp_v6_connection);
    write_end_chart();

    write_begin_chart(type, NETDATA_NET_APPS_BANDWIDTH_SENT);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_TCP_SENDMSG].name,
                          (long long) values->bytes_sent);
    write_end_chart();

    write_begin_chart(type, NETDATA_NET_APPS_BANDWIDTH_RECV);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_TCP_CLEANUP_RBUF].name,
                          (long long) values->bytes_received);
    write_end_chart();

    write_begin_chart(type, NETDATA_NET_APPS_BANDWIDTH_TCP_SEND_CALLS);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_TCP_SENDMSG].name,
                          (long long) values->call_tcp_sent);
    write_end_chart();

    write_begin_chart(type, NETDATA_NET_APPS_BANDWIDTH_TCP_RECV_CALLS);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_TCP_CLEANUP_RBUF].name,
                          (long long) values->call_tcp_received);
    write_end_chart();

    write_begin_chart(type, NETDATA_NET_APPS_BANDWIDTH_TCP_RETRANSMIT);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_TCP_RETRANSMIT].name,
                          (long long) values->retransmit);
    write_end_chart();

    write_begin_chart(type, NETDATA_NET_APPS_BANDWIDTH_UDP_SEND_CALLS);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_UDP_SENDMSG].name,
                          (long long) values->call_udp_sent);
    write_end_chart();

    write_begin_chart(type, NETDATA_NET_APPS_BANDWIDTH_UDP_RECV_CALLS);
    write_chart_dimension(socket_publish_aggregated[NETDATA_IDX_UDP_RECVBUF].name,
                          (long long) values->call_udp_received);
    write_end_chart();
}

/**
 *  Create Systemd Socket Charts
 *
 *  Create charts when systemd is enabled
 *
 *  @param update_every value to overwrite the update frequency set by the server.
 **/
static void ebpf_create_systemd_socket_charts(int update_every)
{
    int order = 20080;
    ebpf_create_charts_on_systemd(NETDATA_NET_APPS_CONNECTION_TCP_V4,
                                  "Calls to tcp_v4_connection", EBPF_COMMON_DIMENSION_CONNECTIONS,
                                  NETDATA_APPS_NET_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  order++,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SERVICES_SOCKET_TCP_V4_CONN_CONTEXT, NETDATA_EBPF_MODULE_NAME_SOCKET,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_NET_APPS_CONNECTION_TCP_V6,
                                  "Calls to tcp_v6_connection", EBPF_COMMON_DIMENSION_CONNECTIONS,
                                  NETDATA_APPS_NET_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  order++,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SERVICES_SOCKET_TCP_V6_CONN_CONTEXT, NETDATA_EBPF_MODULE_NAME_SOCKET,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_NET_APPS_BANDWIDTH_RECV,
                                  "Bytes received", EBPF_COMMON_DIMENSION_BITS,
                                  NETDATA_APPS_NET_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  order++,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SERVICES_SOCKET_BYTES_RECV_CONTEXT, NETDATA_EBPF_MODULE_NAME_SOCKET,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_NET_APPS_BANDWIDTH_SENT,
                                  "Bytes sent", EBPF_COMMON_DIMENSION_BITS,
                                  NETDATA_APPS_NET_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  order++,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SERVICES_SOCKET_BYTES_SEND_CONTEXT, NETDATA_EBPF_MODULE_NAME_SOCKET,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_NET_APPS_BANDWIDTH_TCP_RECV_CALLS,
                                  "Calls to tcp_cleanup_rbuf.",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_NET_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  order++,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SERVICES_SOCKET_TCP_RECV_CONTEXT, NETDATA_EBPF_MODULE_NAME_SOCKET,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_NET_APPS_BANDWIDTH_TCP_SEND_CALLS,
                                  "Calls to tcp_sendmsg.",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_NET_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  order++,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SERVICES_SOCKET_TCP_SEND_CONTEXT, NETDATA_EBPF_MODULE_NAME_SOCKET,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_NET_APPS_BANDWIDTH_TCP_RETRANSMIT,
                                  "Calls to tcp_retransmit",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_NET_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  order++,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SERVICES_SOCKET_TCP_RETRANSMIT_CONTEXT, NETDATA_EBPF_MODULE_NAME_SOCKET,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_NET_APPS_BANDWIDTH_UDP_SEND_CALLS,
                                  "Calls to udp_sendmsg",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_NET_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  order++,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SERVICES_SOCKET_UDP_SEND_CONTEXT, NETDATA_EBPF_MODULE_NAME_SOCKET,
                                  update_every);

    ebpf_create_charts_on_systemd(NETDATA_NET_APPS_BANDWIDTH_UDP_RECV_CALLS,
                                  "Calls to udp_recvmsg",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_APPS_NET_GROUP,
                                  NETDATA_EBPF_CHART_TYPE_STACKED,
                                  order++,
                                  ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX],
                                  NETDATA_SERVICES_SOCKET_UDP_RECV_CONTEXT, NETDATA_EBPF_MODULE_NAME_SOCKET,
                                  update_every);
}

/**
 * Send Systemd charts
 *
 * Send collected data to Netdata.
 */
static void ebpf_send_systemd_socket_charts()
{
    ebpf_cgroup_target_t *ect;
    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_NET_APPS_CONNECTION_TCP_V4);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_socket.call_tcp_v4_connection);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_NET_APPS_CONNECTION_TCP_V6);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_socket.call_tcp_v6_connection);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_NET_APPS_BANDWIDTH_SENT);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_socket.bytes_sent);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_NET_APPS_BANDWIDTH_RECV);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_socket.bytes_received);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_NET_APPS_BANDWIDTH_TCP_SEND_CALLS);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_socket.call_tcp_sent);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_NET_APPS_BANDWIDTH_TCP_RECV_CALLS);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_socket.call_tcp_received);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_NET_APPS_BANDWIDTH_TCP_RETRANSMIT);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_socket.retransmit);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_NET_APPS_BANDWIDTH_UDP_SEND_CALLS);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_socket.call_udp_sent);
        }
    }
    write_end_chart();

    write_begin_chart(NETDATA_SERVICE_FAMILY, NETDATA_NET_APPS_BANDWIDTH_UDP_RECV_CALLS);
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (unlikely(ect->systemd) && unlikely(ect->updated)) {
            write_chart_dimension(ect->name, (long long)ect->publish_socket.call_udp_received);
        }
    }
    write_end_chart();
}

/**
 * Update Cgroup algorithm
 *
 * Change algorithm from absolute to incremental
 */
void ebpf_socket_update_cgroup_algorithm()
{
    int i;
    for (i = 0; i < NETDATA_MAX_SOCKET_VECTOR; i++) {
        netdata_publish_syscall_t *ptr = &socket_publish_aggregated[i];
        ptr->algorithm = ebpf_algorithms[NETDATA_EBPF_INCREMENTAL_IDX];
    }
}

/**
 * Send data to Netdata calling auxiliary functions.
 *
 * @param update_every value to overwrite the update frequency set by the server.
*/
static void ebpf_socket_send_cgroup_data(int update_every)
{
    if (!ebpf_cgroup_pids)
        return;

    pthread_mutex_lock(&mutex_cgroup_shm);
    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        ebpf_socket_sum_cgroup_pids(&ect->publish_socket, ect->pids);
    }

    int has_systemd = shm_ebpf_cgroup.header->systemd_enabled;
    if (has_systemd) {
        if (send_cgroup_chart) {
            ebpf_create_systemd_socket_charts(update_every);
        }
        ebpf_send_systemd_socket_charts();
    }

    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (ect->systemd)
            continue;

        if (!(ect->flags & NETDATA_EBPF_CGROUP_HAS_SOCKET_CHART)) {
            ebpf_create_specific_socket_charts(ect->name, update_every);
            ect->flags |= NETDATA_EBPF_CGROUP_HAS_SOCKET_CHART;
        }

        if (ect->flags & NETDATA_EBPF_CGROUP_HAS_SOCKET_CHART && ect->updated) {
            ebpf_send_specific_socket_data(ect->name, &ect->publish_socket);
        } else {
            ebpf_obsolete_specific_socket_charts(ect->name, update_every);
            ect->flags &= ~NETDATA_EBPF_CGROUP_HAS_SOCKET_CHART;
        }
    }

    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/*****************************************************************
 *
 *  FUNCTIONS WITH THE MAIN LOOP
 *
 *****************************************************************/

/**
 * Main loop for this collector.
 *
 * @param em   the structure with thread information
 */
static void socket_collector(ebpf_module_t *em)
{
    heartbeat_t hb;
    heartbeat_init(&hb);
    uint32_t network_connection = network_viewer_opt.enabled;

    if (network_connection) {
        socket_threads.thread = mallocz(sizeof(netdata_thread_t));
        socket_threads.start_routine = ebpf_socket_read_hash;

        netdata_thread_create(socket_threads.thread, socket_threads.name,
                              NETDATA_THREAD_OPTION_DEFAULT, ebpf_socket_read_hash, em);
    }

    int cgroups = em->cgroup_charts;
    if (cgroups)
        ebpf_socket_update_cgroup_algorithm();

    int socket_global_enabled = em->global_charts;
    int update_every = em->update_every;
    int maps_per_core = em->maps_per_core;
    int counter = update_every - 1;
    while (!ebpf_exit_plugin) {
        (void)heartbeat_next(&hb, USEC_PER_SEC);
        if (ebpf_exit_plugin || ++counter != update_every)
            continue;

        counter = 0;
        netdata_apps_integration_flags_t socket_apps_enabled = em->apps_charts;
        if (socket_global_enabled) {
            read_listen_table();
            read_hash_global_tables(maps_per_core);
        }

        pthread_mutex_lock(&collect_data_mutex);
        if (socket_apps_enabled)
            ebpf_socket_update_apps_data(maps_per_core);

        if (cgroups)
            ebpf_update_socket_cgroup(maps_per_core);

        if (network_connection)
            calculate_nv_plot();

        pthread_mutex_lock(&lock);
        if (socket_global_enabled)
            ebpf_socket_send_data(em);

        if (socket_apps_enabled & NETDATA_EBPF_APPS_FLAG_CHART_CREATED)
            ebpf_socket_send_apps_data(em, apps_groups_root_target);

#ifdef NETDATA_DEV_MODE
        if (ebpf_aral_socket_pid)
            ebpf_send_data_aral_chart(ebpf_aral_socket_pid, em);
#endif

        if (cgroups)
            ebpf_socket_send_cgroup_data(update_every);

        fflush(stdout);

        if (network_connection) {
            // We are calling fflush many times, because when we have a lot of dimensions
            // we began to have not expected outputs and Netdata closed the plugin.
            pthread_mutex_lock(&nv_mutex);
            ebpf_socket_create_nv_charts(&inbound_vectors, update_every);
            fflush(stdout);
            ebpf_socket_send_nv_data(&inbound_vectors);

            ebpf_socket_create_nv_charts(&outbound_vectors, update_every);
            fflush(stdout);
            ebpf_socket_send_nv_data(&outbound_vectors);
            pthread_mutex_unlock(&nv_mutex);

        }
        pthread_mutex_unlock(&lock);
        pthread_mutex_unlock(&collect_data_mutex);
    }
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
 * @param apps is apps enabled?
 */
static void ebpf_socket_allocate_global_vectors(int apps)
{
    memset(socket_aggregated_data, 0 ,NETDATA_MAX_SOCKET_VECTOR * sizeof(netdata_syscall_stat_t));
    memset(socket_publish_aggregated, 0 ,NETDATA_MAX_SOCKET_VECTOR * sizeof(netdata_publish_syscall_t));
    socket_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));

    if (apps) {
        ebpf_socket_aral_init();
        socket_bandwidth_curr = callocz((size_t)pid_max, sizeof(ebpf_socket_publish_apps_t *));
        bandwidth_vector = callocz((size_t)ebpf_nprocs, sizeof(ebpf_bandwidth_t));
    }

    socket_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_socket_t));
    if (network_viewer_opt.enabled) {
        inbound_vectors.plot = callocz(network_viewer_opt.max_dim, sizeof(netdata_socket_plot_t));
        outbound_vectors.plot = callocz(network_viewer_opt.max_dim, sizeof(netdata_socket_plot_t));
    }
}

/**
 * Initialize Inbound and Outbound
 *
 * Initialize the common outbound and inbound sockets.
 */
static void initialize_inbound_outbound()
{
    inbound_vectors.last = network_viewer_opt.max_dim - 1;
    outbound_vectors.last = inbound_vectors.last;
    fill_last_nv_dimension(&inbound_vectors.plot[inbound_vectors.last], 0);
    fill_last_nv_dimension(&outbound_vectors.plot[outbound_vectors.last], 1);
}

/*****************************************************************
 *
 *  EBPF SOCKET THREAD
 *
 *****************************************************************/

/**
 * Fill Port list
 *
 * @param out a pointer to the link list.
 * @param in the structure that will be linked.
 */
static inline void fill_port_list(ebpf_network_viewer_port_list_t **out, ebpf_network_viewer_port_list_t *in)
{
    if (likely(*out)) {
        ebpf_network_viewer_port_list_t *move = *out, *store = *out;
        uint16_t first = ntohs(in->first);
        uint16_t last = ntohs(in->last);
        while (move) {
            uint16_t cmp_first = ntohs(move->first);
            uint16_t cmp_last = ntohs(move->last);
            if (cmp_first <= first && first <= cmp_last  &&
                cmp_first <= last && last <= cmp_last ) {
                info("The range/value (%u, %u) is inside the range/value (%u, %u) already inserted, it will be ignored.",
                     first, last, cmp_first, cmp_last);
                freez(in->value);
                freez(in);
                return;
            } else if (first <= cmp_first && cmp_first <= last  &&
                       first <= cmp_last && cmp_last <= last) {
                info("The range (%u, %u) is bigger than previous range (%u, %u) already inserted, the previous will be ignored.",
                     first, last, cmp_first, cmp_last);
                freez(move->value);
                move->value = in->value;
                move->first = in->first;
                move->last = in->last;
                freez(in);
                return;
            }

            store = move;
            move = move->next;
        }

        store->next = in;
    } else {
        *out = in;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    info("Adding values %s( %u, %u) to %s port list used on network viewer",
         in->value, ntohs(in->first), ntohs(in->last),
         (*out == network_viewer_opt.included_port)?"included":"excluded");
#endif
}

/**
 * Parse Service List
 *
 * @param out a pointer to store the link list
 * @param service the service used to create the structure that will be linked.
 */
static void parse_service_list(void **out, char *service)
{
    ebpf_network_viewer_port_list_t **list = (ebpf_network_viewer_port_list_t **)out;
    struct servent *serv = getservbyname((const char *)service, "tcp");
    if (!serv)
        serv = getservbyname((const char *)service, "udp");

    if (!serv) {
        info("Cannot resolv the service '%s' with protocols TCP and UDP, it will be ignored", service);
        return;
    }

    ebpf_network_viewer_port_list_t *w = callocz(1, sizeof(ebpf_network_viewer_port_list_t));
    w->value = strdupz(service);
    w->hash = simple_hash(service);

    w->first = w->last = (uint16_t)serv->s_port;

    fill_port_list(list, w);
}

/**
 * Netmask
 *
 * Copied from iprange (https://github.com/firehol/iprange/blob/master/iprange.h)
 *
 * @param prefix create the netmask based in the CIDR value.
 *
 * @return
 */
static inline in_addr_t netmask(int prefix) {

    if (prefix == 0)
        return (~((in_addr_t) - 1));
    else
        return (in_addr_t)(~((1 << (32 - prefix)) - 1));

}

/**
 * Broadcast
 *
 * Copied from iprange (https://github.com/firehol/iprange/blob/master/iprange.h)
 *
 * @param addr is the ip address
 * @param prefix is the CIDR value.
 *
 * @return It returns the last address of the range
 */
static inline in_addr_t broadcast(in_addr_t addr, int prefix)
{
    return (addr | ~netmask(prefix));
}

/**
 * Network
 *
 * Copied from iprange (https://github.com/firehol/iprange/blob/master/iprange.h)
 *
 * @param addr is the ip address
 * @param prefix is the CIDR value.
 *
 * @return It returns the first address of the range.
 */
static inline in_addr_t ipv4_network(in_addr_t addr, int prefix)
{
    return (addr & netmask(prefix));
}

/**
 * IP to network long
 *
 * @param dst the vector to store the result
 * @param ip the source ip given by our users.
 * @param domain the ip domain (IPV4 or IPV6)
 * @param source the original string
 *
 * @return it returns 0 on success and -1 otherwise.
 */
static inline int ip2nl(uint8_t *dst, char *ip, int domain, char *source)
{
    if (inet_pton(domain, ip, dst) <= 0) {
        error("The address specified (%s) is invalid ", source);
        return -1;
    }

    return 0;
}

/**
 * Get IPV6 Last Address
 *
 * @param out the address to store the last address.
 * @param in the address used to do the math.
 * @param prefix number of bits used to calculate the address
 */
static void get_ipv6_last_addr(union netdata_ip_t *out, union netdata_ip_t *in, uint64_t prefix)
{
    uint64_t mask,tmp;
    uint64_t ret[2];
    memcpy(ret, in->addr32, sizeof(union netdata_ip_t));

    if (prefix == 128) {
        memcpy(out->addr32, in->addr32, sizeof(union netdata_ip_t));
        return;
    } else if (!prefix) {
        ret[0] = ret[1] = 0xFFFFFFFFFFFFFFFF;
        memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
        return;
    } else if (prefix <= 64) {
        ret[1] = 0xFFFFFFFFFFFFFFFFULL;

        tmp = be64toh(ret[0]);
        if (prefix > 0) {
            mask = 0xFFFFFFFFFFFFFFFFULL << (64 - prefix);
            tmp |= ~mask;
        }
        ret[0] = htobe64(tmp);
    } else {
        mask = 0xFFFFFFFFFFFFFFFFULL << (128 - prefix);
        tmp = be64toh(ret[1]);
        tmp |= ~mask;
        ret[1] = htobe64(tmp);
    }

    memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
}

/**
 * Calculate ipv6 first address
 *
 * @param out the address to store the first address.
 * @param in the address used to do the math.
 * @param prefix number of bits used to calculate the address
 */
static void get_ipv6_first_addr(union netdata_ip_t *out, union netdata_ip_t *in, uint64_t prefix)
{
    uint64_t mask,tmp;
    uint64_t ret[2];

    memcpy(ret, in->addr32, sizeof(union netdata_ip_t));

    if (prefix == 128) {
        memcpy(out->addr32, in->addr32, sizeof(union netdata_ip_t));
        return;
    } else if (!prefix) {
        ret[0] = ret[1] = 0;
        memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
        return;
    } else if (prefix <= 64) {
        ret[1] = 0ULL;

        tmp = be64toh(ret[0]);
        if (prefix > 0) {
            mask = 0xFFFFFFFFFFFFFFFFULL << (64 - prefix);
            tmp &= mask;
        }
        ret[0] = htobe64(tmp);
    } else {
        mask = 0xFFFFFFFFFFFFFFFFULL << (128 - prefix);
        tmp = be64toh(ret[1]);
        tmp &= mask;
        ret[1] = htobe64(tmp);
    }

    memcpy(out->addr32, ret, sizeof(union netdata_ip_t));
}

/**
 * Is ip inside the range
 *
 * Check if the ip is inside a IP range
 *
 * @param rfirst    the first ip address of the range
 * @param rlast     the last ip address of the range
 * @param cmpfirst  the first ip to compare
 * @param cmplast   the last ip to compare
 * @param family    the IP family
 *
 * @return It returns 1 if the IP is inside the range and 0 otherwise
 */
static int ebpf_is_ip_inside_range(union netdata_ip_t *rfirst, union netdata_ip_t *rlast,
                                   union netdata_ip_t *cmpfirst, union netdata_ip_t *cmplast, int family)
{
    if (family == AF_INET) {
        if ((rfirst->addr32[0] <= cmpfirst->addr32[0]) && (rlast->addr32[0] >= cmplast->addr32[0]))
            return 1;
    } else {
        if (memcmp(rfirst->addr8, cmpfirst->addr8, sizeof(union netdata_ip_t)) <= 0 &&
            memcmp(rlast->addr8, cmplast->addr8, sizeof(union netdata_ip_t)) >= 0) {
            return 1;
        }

    }
    return 0;
}

/**
 * Fill IP list
 *
 * @param out a pointer to the link list.
 * @param in the structure that will be linked.
 * @param table the modified table.
 */
void ebpf_fill_ip_list(ebpf_network_viewer_ip_list_t **out, ebpf_network_viewer_ip_list_t *in, char *table)
{
#ifndef NETDATA_INTERNAL_CHECKS
    UNUSED(table);
#endif
    if (in->ver == AF_INET) { // It is simpler to compare using host order
        in->first.addr32[0] = ntohl(in->first.addr32[0]);
        in->last.addr32[0] = ntohl(in->last.addr32[0]);
    }
    if (likely(*out)) {
        ebpf_network_viewer_ip_list_t *move = *out, *store = *out;
        while (move) {
            if (in->ver == move->ver &&
                ebpf_is_ip_inside_range(&move->first, &move->last, &in->first, &in->last, in->ver)) {
                info("The range/value (%s) is inside the range/value (%s) already inserted, it will be ignored.",
                     in->value, move->value);
                freez(in->value);
                freez(in);
                return;
            }
            store = move;
            move = move->next;
        }

        store->next = in;
    } else {
        *out = in;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    char first[256], last[512];
    if (in->ver == AF_INET) {
        info("Adding values %s: (%u - %u) to %s IP list \"%s\" used on network viewer",
             in->value, in->first.addr32[0], in->last.addr32[0],
             (*out == network_viewer_opt.included_ips)?"included":"excluded",
             table);
    } else {
        if (inet_ntop(AF_INET6, in->first.addr8, first, INET6_ADDRSTRLEN) &&
            inet_ntop(AF_INET6, in->last.addr8, last, INET6_ADDRSTRLEN))
            info("Adding values %s - %s to %s IP list \"%s\" used on network viewer",
                 first, last,
                 (*out == network_viewer_opt.included_ips)?"included":"excluded",
                 table);
    }
#endif
}

/**
 * Parse IP List
 *
 * Parse IP list and link it.
 *
 * @param out a pointer to store the link list
 * @param ip the value given as parameter
 */
static void ebpf_parse_ip_list(void **out, char *ip)
{
    ebpf_network_viewer_ip_list_t **list = (ebpf_network_viewer_ip_list_t **)out;

    char *ipdup = strdupz(ip);
    union netdata_ip_t first = { };
    union netdata_ip_t last = { };
    char *is_ipv6;
    if (*ip == '*' && *(ip+1) == '\0') {
        memset(first.addr8, 0, sizeof(first.addr8));
        memset(last.addr8, 0xFF, sizeof(last.addr8));

        is_ipv6 = ip;

        clean_ip_structure(list);
        goto storethisip;
    }

    char *end = ip;
    // Move while I cannot find a separator
    while (*end && *end != '/' && *end != '-') end++;

    // We will use only the classic IPV6 for while, but we could consider the base 85 in a near future
    // https://tools.ietf.org/html/rfc1924
    is_ipv6 = strchr(ip, ':');

    int select;
    if (*end && !is_ipv6) { // IPV4 range
        select = (*end == '/') ? 0 : 1;
        *end++ = '\0';
        if (*end == '!') {
            info("The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
            goto cleanipdup;
        }

        if (!select) { // CIDR
            select = ip2nl(first.addr8, ip, AF_INET, ipdup);
            if (select)
                goto cleanipdup;

            select = (int) str2i(end);
            if (select < NETDATA_MINIMUM_IPV4_CIDR || select > NETDATA_MAXIMUM_IPV4_CIDR) {
                info("The specified CIDR %s is not valid, the IP %s will be ignored.", end, ip);
                goto cleanipdup;
            }

            last.addr32[0] = htonl(broadcast(ntohl(first.addr32[0]), select));
            // This was added to remove
            // https://app.codacy.com/manual/netdata/netdata/pullRequest?prid=5810941&bid=19021977
            UNUSED(last.addr32[0]);

            uint32_t ipv4_test = htonl(ipv4_network(ntohl(first.addr32[0]), select));
            if (first.addr32[0] != ipv4_test) {
                first.addr32[0] = ipv4_test;
                struct in_addr ipv4_convert;
                ipv4_convert.s_addr = ipv4_test;
                char ipv4_msg[INET_ADDRSTRLEN];
                if(inet_ntop(AF_INET, &ipv4_convert, ipv4_msg, INET_ADDRSTRLEN))
                    info("The network value of CIDR %s was updated for %s .", ipdup, ipv4_msg);
            }
        } else { // Range
            select = ip2nl(first.addr8, ip, AF_INET, ipdup);
            if (select)
                goto cleanipdup;

            select = ip2nl(last.addr8, end, AF_INET, ipdup);
            if (select)
                goto cleanipdup;
        }

        if (htonl(first.addr32[0]) > htonl(last.addr32[0])) {
            info("The specified range %s is invalid, the second address is smallest than the first, it will be ignored.",
                 ipdup);
            goto cleanipdup;
        }
    } else if (is_ipv6) { // IPV6
        if (!*end) { // Unique
            select = ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            memcpy(last.addr8, first.addr8, sizeof(first.addr8));
        } else if (*end == '-') {
            *end++ = 0x00;
            if (*end == '!') {
                info("The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
                goto cleanipdup;
            }

            select = ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            select = ip2nl(last.addr8, end, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;
        } else { // CIDR
            *end++ = 0x00;
            if (*end == '!') {
                info("The exclusion cannot be in the second part of the range %s, it will be ignored.", ipdup);
                goto cleanipdup;
            }

            select = str2i(end);
            if (select < 0 || select > 128) {
                info("The CIDR %s is not valid, the address %s will be ignored.", end, ip);
                goto cleanipdup;
            }

            uint64_t prefix = (uint64_t)select;
            select = ip2nl(first.addr8, ip, AF_INET6, ipdup);
            if (select)
                goto cleanipdup;

            get_ipv6_last_addr(&last, &first, prefix);

            union netdata_ip_t ipv6_test;
            get_ipv6_first_addr(&ipv6_test, &first, prefix);

            if (memcmp(first.addr8, ipv6_test.addr8, sizeof(union netdata_ip_t)) != 0) {
                memcpy(first.addr8, ipv6_test.addr8, sizeof(union netdata_ip_t));

                struct in6_addr ipv6_convert;
                memcpy(ipv6_convert.s6_addr,  ipv6_test.addr8, sizeof(union netdata_ip_t));

                char ipv6_msg[INET6_ADDRSTRLEN];
                if(inet_ntop(AF_INET6, &ipv6_convert, ipv6_msg, INET6_ADDRSTRLEN))
                    info("The network value of CIDR %s was updated for %s .", ipdup, ipv6_msg);
            }
        }

        if ((be64toh(*(uint64_t *)&first.addr32[2]) > be64toh(*(uint64_t *)&last.addr32[2]) &&
             !memcmp(first.addr32, last.addr32, 2*sizeof(uint32_t))) ||
            (be64toh(*(uint64_t *)&first.addr32) > be64toh(*(uint64_t *)&last.addr32)) ) {
            info("The specified range %s is invalid, the second address is smallest than the first, it will be ignored.",
                 ipdup);
            goto cleanipdup;
        }
    } else { // Unique ip
        select = ip2nl(first.addr8, ip, AF_INET, ipdup);
        if (select)
            goto cleanipdup;

        memcpy(last.addr8, first.addr8, sizeof(first.addr8));
    }

    ebpf_network_viewer_ip_list_t *store;

storethisip:
    store = callocz(1, sizeof(ebpf_network_viewer_ip_list_t));
    store->value = ipdup;
    store->hash = simple_hash(ipdup);
    store->ver = (uint8_t)(!is_ipv6)?AF_INET:AF_INET6;
    memcpy(store->first.addr8, first.addr8, sizeof(first.addr8));
    memcpy(store->last.addr8, last.addr8, sizeof(last.addr8));

    ebpf_fill_ip_list(list, store, "socket");
    return;

cleanipdup:
    freez(ipdup);
}

/**
 * Parse IP Range
 *
 * Parse the IP ranges given and create Network Viewer IP Structure
 *
 * @param ptr  is a pointer with the text to parse.
 */
static void ebpf_parse_ips(char *ptr)
{
    // No value
    if (unlikely(!ptr))
        return;

    while (likely(ptr)) {
        // Move forward until next valid character
        while (isspace(*ptr)) ptr++;

        // No valid value found
        if (unlikely(!*ptr))
            return;

        // Find space that ends the list
        char *end = strchr(ptr, ' ');
        if (end) {
            *end++ = '\0';
        }

        int neg = 0;
        if (*ptr == '!') {
            neg++;
            ptr++;
        }

        if (isascii(*ptr)) { // Parse port
            ebpf_parse_ip_list((!neg)?(void **)&network_viewer_opt.included_ips:
                                      (void **)&network_viewer_opt.excluded_ips,
                                ptr);
        }

        ptr = end;
    }
}



/**
 * Parse port list
 *
 * Parse an allocated port list with the range given
 *
 * @param out a pointer to store the link list
 * @param range the informed range for the user.
 */
static void parse_port_list(void **out, char *range)
{
    int first, last;
    ebpf_network_viewer_port_list_t **list = (ebpf_network_viewer_port_list_t **)out;

    char *copied = strdupz(range);
    if (*range == '*' && *(range+1) == '\0') {
        first = 1;
        last = 65535;

        clean_port_structure(list);
        goto fillenvpl;
    }

    char *end = range;
    //Move while I cannot find a separator
    while (*end && *end != ':' && *end != '-') end++;

    //It has a range
    if (likely(*end)) {
        *end++ = '\0';
        if (*end == '!') {
            info("The exclusion cannot be in the second part of the range, the range %s will be ignored.", copied);
            freez(copied);
            return;
        }
        last = str2i((const char *)end);
    } else {
        last = 0;
    }

    first = str2i((const char *)range);
    if (first < NETDATA_MINIMUM_PORT_VALUE || first > NETDATA_MAXIMUM_PORT_VALUE) {
        info("The first port %d of the range \"%s\" is invalid and it will be ignored!", first, copied);
        freez(copied);
        return;
    }

    if (!last)
        last = first;

    if (last < NETDATA_MINIMUM_PORT_VALUE || last > NETDATA_MAXIMUM_PORT_VALUE) {
        info("The second port %d of the range \"%s\" is invalid and the whole range will be ignored!", last, copied);
        freez(copied);
        return;
    }

    if (first > last) {
        info("The specified order %s is wrong, the smallest value is always the first, it will be ignored!", copied);
        freez(copied);
        return;
    }

    ebpf_network_viewer_port_list_t *w;
fillenvpl:
    w = callocz(1, sizeof(ebpf_network_viewer_port_list_t));
    w->value = copied;
    w->hash = simple_hash(copied);
    w->first = (uint16_t)htons((uint16_t)first);
    w->last = (uint16_t)htons((uint16_t)last);
    w->cmp_first = (uint16_t)first;
    w->cmp_last = (uint16_t)last;

    fill_port_list(list, w);
}

/**
 * Read max dimension.
 *
 * Netdata plot two dimensions per connection, so it is necessary to adjust the values.
 *
 * @param cfg the configuration structure
 */
static void read_max_dimension(struct config *cfg)
{
    int maxdim ;
    maxdim = (int) appconfig_get_number(cfg,
                                        EBPF_NETWORK_VIEWER_SECTION,
                                        EBPF_MAXIMUM_DIMENSIONS,
                                        NETDATA_NV_CAP_VALUE);
    if (maxdim < 0) {
        error("'maximum dimensions = %d' must be a positive number, Netdata will change for default value %ld.",
              maxdim, NETDATA_NV_CAP_VALUE);
        maxdim = NETDATA_NV_CAP_VALUE;
    }

    maxdim /= 2;
    if (!maxdim) {
        info("The number of dimensions is too small (%u), we are setting it to minimum 2", network_viewer_opt.max_dim);
        network_viewer_opt.max_dim = 1;
        return;
    }

    network_viewer_opt.max_dim = (uint32_t)maxdim;
}

/**
 * Parse Port Range
 *
 * Parse the port ranges given and create Network Viewer Port Structure
 *
 * @param ptr  is a pointer with the text to parse.
 */
static void parse_ports(char *ptr)
{
    // No value
    if (unlikely(!ptr))
        return;

    while (likely(ptr)) {
        // Move forward until next valid character
        while (isspace(*ptr)) ptr++;

        // No valid value found
        if (unlikely(!*ptr))
            return;

        // Find space that ends the list
        char *end = strchr(ptr, ' ');
        if (end) {
            *end++ = '\0';
        }

        int neg = 0;
        if (*ptr == '!') {
            neg++;
            ptr++;
        }

        if (isdigit(*ptr)) { // Parse port
            parse_port_list((!neg)?(void **)&network_viewer_opt.included_port:(void **)&network_viewer_opt.excluded_port,
                            ptr);
        } else if (isalpha(*ptr)) { // Parse service
            parse_service_list((!neg)?(void **)&network_viewer_opt.included_port:(void **)&network_viewer_opt.excluded_port,
                               ptr);
        } else if (*ptr == '*') { // All
            parse_port_list((!neg)?(void **)&network_viewer_opt.included_port:(void **)&network_viewer_opt.excluded_port,
                            ptr);
        }

        ptr = end;
    }
}

/**
 * Link hostname
 *
 * @param out is the output link list
 * @param in the hostname to add to list.
 */
static void link_hostname(ebpf_network_viewer_hostname_list_t **out, ebpf_network_viewer_hostname_list_t *in)
{
    if (likely(*out)) {
        ebpf_network_viewer_hostname_list_t *move = *out;
        for (; move->next ; move = move->next ) {
            if (move->hash == in->hash && !strcmp(move->value, in->value)) {
                info("The hostname %s was already inserted, it will be ignored.", in->value);
                freez(in->value);
                simple_pattern_free(in->value_pattern);
                freez(in);
                return;
            }
        }

        move->next = in;
    } else {
        *out = in;
    }
#ifdef NETDATA_INTERNAL_CHECKS
    info("Adding value %s to %s hostname list used on network viewer",
         in->value,
         (*out == network_viewer_opt.included_hostnames)?"included":"excluded");
#endif
}

/**
 * Link Hostnames
 *
 * Parse the list of hostnames to create the link list.
 * This is not associated with the IP, because simple patterns like *example* cannot be resolved to IP.
 *
 * @param out is the output link list
 * @param parse is a pointer with the text to parser.
 */
static void link_hostnames(char *parse)
{
    // No value
    if (unlikely(!parse))
        return;

    while (likely(parse)) {
        // Find the first valid value
        while (isspace(*parse)) parse++;

        // No valid value found
        if (unlikely(!*parse))
            return;

        // Find space that ends the list
        char *end = strchr(parse, ' ');
        if (end) {
            *end++ = '\0';
        }

        int neg = 0;
        if (*parse == '!') {
            neg++;
            parse++;
        }

        ebpf_network_viewer_hostname_list_t *hostname = callocz(1 , sizeof(ebpf_network_viewer_hostname_list_t));
        hostname->value = strdupz(parse);
        hostname->hash = simple_hash(parse);
        hostname->value_pattern = simple_pattern_create(parse, NULL, SIMPLE_PATTERN_EXACT, true);

        link_hostname((!neg)?&network_viewer_opt.included_hostnames:&network_viewer_opt.excluded_hostnames,
                      hostname);

        parse = end;
    }
}

/**
 * Parse network viewer section
 *
 * @param cfg the configuration structure
 */
void parse_network_viewer_section(struct config *cfg)
{
    read_max_dimension(cfg);

    network_viewer_opt.hostname_resolution_enabled = appconfig_get_boolean(cfg,
                                                                           EBPF_NETWORK_VIEWER_SECTION,
                                                                           EBPF_CONFIG_RESOLVE_HOSTNAME,
                                                                           CONFIG_BOOLEAN_NO);

    network_viewer_opt.service_resolution_enabled = appconfig_get_boolean(cfg,
                                                                          EBPF_NETWORK_VIEWER_SECTION,
                                                                          EBPF_CONFIG_RESOLVE_SERVICE,
                                                                          CONFIG_BOOLEAN_NO);

    char *value = appconfig_get(cfg, EBPF_NETWORK_VIEWER_SECTION, EBPF_CONFIG_PORTS, NULL);
    parse_ports(value);

    if (network_viewer_opt.hostname_resolution_enabled) {
        value = appconfig_get(cfg, EBPF_NETWORK_VIEWER_SECTION, EBPF_CONFIG_HOSTNAMES, NULL);
        link_hostnames(value);
    } else {
        info("Name resolution is disabled, collector will not parser \"hostnames\" list.");
    }

    value = appconfig_get(cfg, EBPF_NETWORK_VIEWER_SECTION,
                          "ips", "!127.0.0.1/8 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fc00::/7 !::1/128");
    ebpf_parse_ips(value);
}

/**
 * Link dimension name
 *
 * Link user specified names inside a link list.
 *
 * @param port the port number associated to the dimension name.
 * @param hash the calculated hash for the dimension name.
 * @param name the dimension name.
 */
static void link_dimension_name(char *port, uint32_t hash, char *value)
{
    int test = str2i(port);
    if (test < NETDATA_MINIMUM_PORT_VALUE || test > NETDATA_MAXIMUM_PORT_VALUE){
        error("The dimension given (%s = %s) has an invalid value and it will be ignored.", port, value);
        return;
    }

    ebpf_network_viewer_dim_name_t *w;
    w = callocz(1, sizeof(ebpf_network_viewer_dim_name_t));

    w->name = strdupz(value);
    w->hash = hash;

    w->port = (uint16_t) htons(test);

    ebpf_network_viewer_dim_name_t *names = network_viewer_opt.names;
    if (unlikely(!names)) {
        network_viewer_opt.names = w;
    } else {
        for (; names->next; names = names->next) {
            if (names->port == w->port) {
                info("Duplicated definition for a service, the name %s will be ignored. ", names->name);
                freez(names->name);
                names->name = w->name;
                names->hash = w->hash;
                freez(w);
                return;
            }
        }
        names->next = w;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    info("Adding values %s( %u) to dimension name list used on network viewer", w->name, htons(w->port));
#endif
}

/**
 * Parse service Name section.
 *
 * This function gets the values that will be used to overwrite dimensions.
 *
 * @param cfg the configuration structure
 */
void parse_service_name_section(struct config *cfg)
{
    struct section *co = appconfig_get_section(cfg, EBPF_SERVICE_NAME_SECTION);
    if (co) {
        struct config_option *cv;
        for (cv = co->values; cv ; cv = cv->next) {
            link_dimension_name(cv->name, cv->hash, cv->value);
        }
    }

    // Always associated the default port to Netdata
    ebpf_network_viewer_dim_name_t *names = network_viewer_opt.names;
    if (names) {
        uint16_t default_port = htons(19999);
        while (names) {
            if (names->port == default_port)
                return;

            names = names->next;
        }
    }

    char *port_string = getenv("NETDATA_LISTEN_PORT");
    if (port_string) {
        // if variable has an invalid value, we assume netdata is using 19999
        int default_port = str2i(port_string);
        if (default_port > 0 && default_port < 65536)
            link_dimension_name(port_string, simple_hash(port_string), "Netdata");
    }
}

void parse_table_size_options(struct config *cfg)
{
    socket_maps[NETDATA_SOCKET_TABLE_BANDWIDTH].user_input = (uint32_t) appconfig_get_number(cfg,
                                                                                            EBPF_GLOBAL_SECTION,
                                                                                            EBPF_CONFIG_BANDWIDTH_SIZE, NETDATA_MAXIMUM_CONNECTIONS_ALLOWED);

    socket_maps[NETDATA_SOCKET_TABLE_IPV4].user_input = (uint32_t) appconfig_get_number(cfg,
                                                                                       EBPF_GLOBAL_SECTION,
                                                                                       EBPF_CONFIG_IPV4_SIZE, NETDATA_MAXIMUM_CONNECTIONS_ALLOWED);

    socket_maps[NETDATA_SOCKET_TABLE_IPV6].user_input = (uint32_t) appconfig_get_number(cfg,
                                                                                       EBPF_GLOBAL_SECTION,
                                                                                       EBPF_CONFIG_IPV6_SIZE, NETDATA_MAXIMUM_CONNECTIONS_ALLOWED);

    socket_maps[NETDATA_SOCKET_TABLE_UDP].user_input = (uint32_t) appconfig_get_number(cfg,
                                                                                      EBPF_GLOBAL_SECTION,
                                                                                      EBPF_CONFIG_UDP_SIZE, NETDATA_MAXIMUM_UDP_CONNECTIONS_ALLOWED);
}

/*
 * Load BPF
 *
 * Load BPF files.
 *
 * @param em the structure with configuration
 */
static int ebpf_socket_load_bpf(ebpf_module_t *em)
{
#ifdef LIBBPF_MAJOR_VERSION
    ebpf_define_map_type(em->maps, em->maps_per_core, running_on_kernel);
#endif

    int ret = 0;

    if (em->load & EBPF_LOAD_LEGACY) {
        em->probe_links = ebpf_load_program(ebpf_plugin_dir, em, running_on_kernel, isrh, &em->objects);
        if (!em->probe_links) {
            ret = -1;
        }
    }
#ifdef LIBBPF_MAJOR_VERSION
    else {
        socket_bpf_obj = socket_bpf__open();
        if (!socket_bpf_obj)
            ret = -1;
        else
            ret = ebpf_socket_load_and_attach(socket_bpf_obj, em);
    }
#endif

    if (ret) {
        error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->thread_name);
    }

    return ret;
}

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
    netdata_thread_cleanup_push(ebpf_socket_exit, ptr);

    ebpf_module_t *em = (ebpf_module_t *)ptr;
    em->maps = socket_maps;

    parse_table_size_options(&socket_config);

    if (pthread_mutex_init(&nv_mutex, NULL)) {
        error("Cannot initialize local mutex");
        goto endsocket;
    }

    ebpf_socket_allocate_global_vectors(em->apps_charts);

    if (network_viewer_opt.enabled) {
        memset(&inbound_vectors.tree, 0, sizeof(avl_tree_lock));
        memset(&outbound_vectors.tree, 0, sizeof(avl_tree_lock));
        avl_init_lock(&inbound_vectors.tree, ebpf_compare_sockets);
        avl_init_lock(&outbound_vectors.tree, ebpf_compare_sockets);

        initialize_inbound_outbound();
    }

    if (running_on_kernel < NETDATA_EBPF_KERNEL_5_0)
        em->mode = MODE_ENTRY;

#ifdef LIBBPF_MAJOR_VERSION
    ebpf_adjust_thread_load(em, default_btf);
#endif
    if (ebpf_socket_load_bpf(em)) {
        pthread_mutex_unlock(&lock);
        goto endsocket;
    }

    int algorithms[NETDATA_MAX_SOCKET_VECTOR] = {
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX,
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX,
        NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_ABSOLUTE_IDX, NETDATA_EBPF_INCREMENTAL_IDX,
        NETDATA_EBPF_INCREMENTAL_IDX
    };
    ebpf_global_labels(
        socket_aggregated_data, socket_publish_aggregated, socket_dimension_names, socket_id_names,
        algorithms, NETDATA_MAX_SOCKET_VECTOR);

    pthread_mutex_lock(&lock);
    ebpf_create_global_charts(em);

    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps);

#ifdef NETDATA_DEV_MODE
    if (ebpf_aral_socket_pid)
        ebpf_statistic_create_aral_chart(NETDATA_EBPF_SOCKET_ARAL_NAME, em);
#endif

    pthread_mutex_unlock(&lock);

    socket_collector(em);

endsocket:
    ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
