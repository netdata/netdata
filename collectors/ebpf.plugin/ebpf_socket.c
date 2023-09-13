// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/resource.h>

#include "ebpf.h"
#include "ebpf_socket.h"

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

static ebpf_local_maps_t socket_maps[] = {{.name = "tbl_global_sock",
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
                                           {.name = "tbl_nd_socket",
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

netdata_socket_t *socket_values;

ebpf_network_viewer_port_list_t *listen_ports = NULL;

struct config socket_config = { .first_section = NULL,
    .last_section = NULL,
    .mutex = NETDATA_MUTEX_INITIALIZER,
    .index = { .avl_tree = { .root = NULL, .compar = appconfig_section_compare },
        .rwlock = AVL_LOCK_INITIALIZER } };

netdata_ebpf_targets_t socket_targets[] = { {.name = "inet_csk_accept", .mode = EBPF_LOAD_PROBE},
                                            {.name = "tcp_retransmit_skb", .mode = EBPF_LOAD_PROBE},
                                            {.name = "tcp_cleanup_rbuf", .mode = EBPF_LOAD_PROBE},
                                            {.name = "tcp_close", .mode = EBPF_LOAD_PROBE},
                                            {.name = "udp_recvmsg", .mode = EBPF_LOAD_PROBE},
                                            {.name = "tcp_sendmsg", .mode = EBPF_LOAD_PROBE},
                                            {.name = "udp_sendmsg", .mode = EBPF_LOAD_PROBE},
                                            {.name = "tcp_v4_connect", .mode = EBPF_LOAD_PROBE},
                                            {.name = "tcp_v6_connect", .mode = EBPF_LOAD_PROBE},
                                            {.name = NULL, .mode = EBPF_LOAD_TRAMPOLINE}};

struct netdata_static_thread ebpf_read_socket = {
        .name = "EBPF_READ_SOCKET",
        .config_section = NULL,
        .config_name = NULL,
        .env_name = NULL,
        .enabled = 1,
        .thread = NULL,
        .init_routine = NULL,
        .start_routine = NULL
};

ARAL *aral_socket_table = NULL;

#ifdef NETDATA_DEV_MODE
int socket_disable_priority;
#endif

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
    bpf_program__set_autoload(obj->progs.netdata_tcp_v4_connect_kprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_v4_connect_kretprobe, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_v6_connect_kprobe, false);
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
    bpf_program__set_autoload(obj->progs.netdata_inet_csk_accept_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_v4_connect_fentry, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_v4_connect_fexit, false);
    bpf_program__set_autoload(obj->progs.netdata_tcp_v6_connect_fentry, false);
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
}

/**
 *  Set trampoline target.
 *
 *  @param obj is the main structure for bpf objects.
 */
static void ebpf_set_trampoline_target(struct socket_bpf *obj)
{
    bpf_program__set_attach_target(obj->progs.netdata_inet_csk_accept_fexit, 0,
                                   socket_targets[NETDATA_FCNT_INET_CSK_ACCEPT].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_v4_connect_fentry, 0,
                                   socket_targets[NETDATA_FCNT_TCP_V4_CONNECT].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_v4_connect_fexit, 0,
                                   socket_targets[NETDATA_FCNT_TCP_V4_CONNECT].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_v6_connect_fentry, 0,
                                   socket_targets[NETDATA_FCNT_TCP_V6_CONNECT].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_v6_connect_fexit, 0,
                                   socket_targets[NETDATA_FCNT_TCP_V6_CONNECT].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_retransmit_skb_fentry, 0,
                                   socket_targets[NETDATA_FCNT_TCP_RETRANSMIT].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_cleanup_rbuf_fentry, 0,
                                   socket_targets[NETDATA_FCNT_CLEANUP_RBUF].name);

    bpf_program__set_attach_target(obj->progs.netdata_tcp_close_fentry, 0,
                                   socket_targets[NETDATA_FCNT_TCP_CLOSE].name);

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
        bpf_program__set_autoload(obj->progs.netdata_tcp_v4_connect_fentry, false);
        bpf_program__set_autoload(obj->progs.netdata_tcp_v6_connect_fentry, false);
        bpf_program__set_autoload(obj->progs.netdata_udp_sendmsg_fentry, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_tcp_sendmsg_fexit, false);
        bpf_program__set_autoload(obj->progs.netdata_tcp_v4_connect_fexit, false);
        bpf_program__set_autoload(obj->progs.netdata_tcp_v6_connect_fexit, false);
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
        bpf_program__set_autoload(obj->progs.netdata_tcp_v4_connect_kprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_tcp_v6_connect_kprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_udp_sendmsg_kprobe, false);
    } else {
        bpf_program__set_autoload(obj->progs.netdata_tcp_sendmsg_kretprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_tcp_v4_connect_kretprobe, false);
        bpf_program__set_autoload(obj->progs.netdata_tcp_v6_connect_kretprobe, false);
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
static long ebpf_socket_attach_probes(struct socket_bpf *obj, netdata_run_mode_t sel)
{
    obj->links.netdata_inet_csk_accept_kretprobe = bpf_program__attach_kprobe(obj->progs.netdata_inet_csk_accept_kretprobe,
                                                                              true,
                                                                              socket_targets[NETDATA_FCNT_INET_CSK_ACCEPT].name);
    long ret = libbpf_get_error(obj->links.netdata_inet_csk_accept_kretprobe);
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

        obj->links.netdata_tcp_v4_connect_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_tcp_v4_connect_kprobe,
                                                                              false,
                                                                              socket_targets[NETDATA_FCNT_TCP_V4_CONNECT].name);
        ret = libbpf_get_error(obj->links.netdata_tcp_v4_connect_kprobe);
        if (ret)
            return -1;

        obj->links.netdata_tcp_v6_connect_kprobe = bpf_program__attach_kprobe(obj->progs.netdata_tcp_v6_connect_kprobe,
                                                                              false,
                                                                              socket_targets[NETDATA_FCNT_TCP_V6_CONNECT].name);
        ret = libbpf_get_error(obj->links.netdata_tcp_v6_connect_kprobe);
        if (ret)
            return -1;
    }

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
    socket_maps[NETDATA_SOCKET_GLOBAL].map_fd = bpf_map__fd(obj->maps.tbl_global_sock);
    socket_maps[NETDATA_SOCKET_LPORTS].map_fd = bpf_map__fd(obj->maps.tbl_lports);
    socket_maps[NETDATA_SOCKET_OPEN_SOCKET].map_fd = bpf_map__fd(obj->maps.tbl_nd_socket);
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
    ebpf_update_map_size(obj->maps.tbl_nd_socket, &socket_maps[NETDATA_SOCKET_OPEN_SOCKET],
                         em, bpf_map__name(obj->maps.tbl_nd_socket));

    ebpf_update_map_size(obj->maps.tbl_nv_udp, &socket_maps[NETDATA_SOCKET_TABLE_UDP],
                         em, bpf_map__name(obj->maps.tbl_nv_udp));

    ebpf_update_map_type(obj->maps.tbl_nd_socket, &socket_maps[NETDATA_SOCKET_OPEN_SOCKET]);
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
        ret = (int)ebpf_socket_attach_probes(obj, em->mode);
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
 * Socket Free
 *
 * Cleanup variables after child threads to stop
 *
 * @param ptr thread data.
 */
static void ebpf_socket_free(ebpf_module_t *em )
{
    pthread_mutex_lock(&ebpf_exit_cleanup);
    em->enabled = NETDATA_THREAD_EBPF_STOPPED;
    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_REMOVE);
    pthread_mutex_unlock(&ebpf_exit_cleanup);
}

/**
 *  Obsolete Systemd Socket Charts
 *
 *  Obsolete charts when systemd is enabled
 *
 *  @param update_every value to overwrite the update frequency set by the server.
 **/
static void ebpf_obsolete_systemd_socket_charts(int update_every)
{
    int order = 20080;
    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_NET_APPS_CONNECTION_TCP_V4,
                              "Calls to tcp_v4_connection",
                              EBPF_COMMON_DIMENSION_CONNECTIONS,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_SERVICES_SOCKET_TCP_V4_CONN_CONTEXT,
                              order++,
                              update_every);

    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_NET_APPS_CONNECTION_TCP_V6,
                              "Calls to tcp_v6_connection",
                              EBPF_COMMON_DIMENSION_CONNECTIONS,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_SERVICES_SOCKET_TCP_V6_CONN_CONTEXT,
                              order++,
                              update_every);

    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_RECV,
                              "Bytes received",
                              EBPF_COMMON_DIMENSION_BITS,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_SERVICES_SOCKET_BYTES_RECV_CONTEXT,
                              order++,
                              update_every);

    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_SENT,
                              "Bytes sent",
                              EBPF_COMMON_DIMENSION_BITS,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_SERVICES_SOCKET_BYTES_SEND_CONTEXT,
                              order++,
                              update_every);

    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_TCP_RECV_CALLS,
                              "Calls to tcp_cleanup_rbuf.",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_SERVICES_SOCKET_TCP_RECV_CONTEXT,
                              order++,
                              update_every);

    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_TCP_SEND_CALLS,
                              "Calls to tcp_sendmsg.",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_SERVICES_SOCKET_TCP_SEND_CONTEXT,
                              order++,
                              update_every);

    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_TCP_RETRANSMIT,
                              "Calls to tcp_retransmit",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_SERVICES_SOCKET_TCP_RETRANSMIT_CONTEXT,
                              order++,
                              update_every);

    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_UDP_SEND_CALLS,
                              "Calls to udp_sendmsg",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_SERVICES_SOCKET_UDP_SEND_CONTEXT,
                              order++,
                              update_every);

    ebpf_write_chart_obsolete(NETDATA_SERVICE_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_UDP_RECV_CALLS,
                              "Calls to udp_recvmsg",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NETDATA_SERVICES_SOCKET_UDP_RECV_CONTEXT,
                              order++,
                              update_every);
}

static void ebpf_obsolete_specific_socket_charts(char *type, int update_every);
/**
 * Obsolete cgroup chart
 *
 * Send obsolete for all charts created before to close.
 *
 * @param em a pointer to `struct ebpf_module`
 */
static inline void ebpf_obsolete_socket_cgroup_charts(ebpf_module_t *em) {
    pthread_mutex_lock(&mutex_cgroup_shm);

    ebpf_obsolete_systemd_socket_charts(em->update_every);

    ebpf_cgroup_target_t *ect;
    for (ect = ebpf_cgroup_pids; ect ; ect = ect->next) {
        if (ect->systemd)
            continue;

        ebpf_obsolete_specific_socket_charts(ect->name, em->update_every);
    }
    pthread_mutex_unlock(&mutex_cgroup_shm);
}

/**
 * Create apps charts
 *
 * Call ebpf_create_chart to create the charts on apps submenu.
 *
 * @param em   a pointer to the structure with the default values.
 */
void ebpf_socket_obsolete_apps_charts(struct ebpf_module *em)
{
    int order = 20080;
    ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                              NETDATA_NET_APPS_CONNECTION_TCP_V4,
                              "Calls to tcp_v4_connection",
                              EBPF_COMMON_DIMENSION_CONNECTIONS,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                              NETDATA_NET_APPS_CONNECTION_TCP_V6,
                              "Calls to tcp_v6_connection",
                              EBPF_COMMON_DIMENSION_CONNECTIONS,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_SENT,
                              "Bytes sent",
                              EBPF_COMMON_DIMENSION_BITS,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_RECV,
                               "bytes received",
                              EBPF_COMMON_DIMENSION_BITS,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_TCP_SEND_CALLS,
                              "Calls for tcp_sendmsg",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_TCP_RECV_CALLS,
                              "Calls for tcp_cleanup_rbuf",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_TCP_RETRANSMIT,
                              "Calls for tcp_retransmit",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_UDP_SEND_CALLS,
                              "Calls for udp_sendmsg",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_APPS_FAMILY,
                              NETDATA_NET_APPS_BANDWIDTH_UDP_RECV_CALLS,
                              "Calls for udp_recvmsg",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_APPS_NET_GROUP,
                              NETDATA_EBPF_CHART_TYPE_STACKED,
                              NULL,
                              order++,
                              em->update_every);
}

/**
 * Obsolete global charts
 *
 * Obsolete charts created.
 *
 * @param em a pointer to the structure with the default values.
 */
static void ebpf_socket_obsolete_global_charts(ebpf_module_t *em)
{
    int order = 21070;
    ebpf_write_chart_obsolete(NETDATA_EBPF_IP_FAMILY,
                              NETDATA_INBOUND_CONNECTIONS,
                              "Inbound connections.",
                              EBPF_COMMON_DIMENSION_CONNECTIONS,
                              NETDATA_SOCKET_KERNEL_FUNCTIONS,
                              NETDATA_EBPF_CHART_TYPE_LINE,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_EBPF_IP_FAMILY,
                              NETDATA_TCP_OUTBOUND_CONNECTIONS,
                              "TCP outbound connections.",
                              EBPF_COMMON_DIMENSION_CONNECTIONS,
                              NETDATA_SOCKET_KERNEL_FUNCTIONS,
                              NETDATA_EBPF_CHART_TYPE_LINE,
                              NULL,
                              order++,
                              em->update_every);


    ebpf_write_chart_obsolete(NETDATA_EBPF_IP_FAMILY,
                              NETDATA_TCP_FUNCTION_COUNT,
                              "Calls to internal functions",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_SOCKET_KERNEL_FUNCTIONS,
                              NETDATA_EBPF_CHART_TYPE_LINE,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_EBPF_IP_FAMILY,
                              NETDATA_TCP_FUNCTION_BITS,
                              "TCP bandwidth",
                              EBPF_COMMON_DIMENSION_BITS,
                              NETDATA_SOCKET_KERNEL_FUNCTIONS,
                              NETDATA_EBPF_CHART_TYPE_LINE,
                              NULL,
                              order++,
                              em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(NETDATA_EBPF_IP_FAMILY,
                                  NETDATA_TCP_FUNCTION_ERROR,
                                  "TCP errors",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_SOCKET_KERNEL_FUNCTIONS,
                                  NETDATA_EBPF_CHART_TYPE_LINE,
                                  NULL,
                                  order++,
                                  em->update_every);
    }

    ebpf_write_chart_obsolete(NETDATA_EBPF_IP_FAMILY,
                              NETDATA_TCP_RETRANSMIT,
                              "Packages retransmitted",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_SOCKET_KERNEL_FUNCTIONS,
                              NETDATA_EBPF_CHART_TYPE_LINE,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_EBPF_IP_FAMILY,
                              NETDATA_UDP_FUNCTION_COUNT,
                              "UDP calls",
                              EBPF_COMMON_DIMENSION_CALL,
                              NETDATA_SOCKET_KERNEL_FUNCTIONS,
                              NETDATA_EBPF_CHART_TYPE_LINE,
                              NULL,
                              order++,
                              em->update_every);

    ebpf_write_chart_obsolete(NETDATA_EBPF_IP_FAMILY,
                              NETDATA_UDP_FUNCTION_BITS,
                              "UDP bandwidth",
                              EBPF_COMMON_DIMENSION_BITS,
                              NETDATA_SOCKET_KERNEL_FUNCTIONS,
                              NETDATA_EBPF_CHART_TYPE_LINE,
                              NULL,
                              order++,
                              em->update_every);

    if (em->mode < MODE_ENTRY) {
        ebpf_write_chart_obsolete(NETDATA_EBPF_IP_FAMILY,
                                  NETDATA_UDP_FUNCTION_ERROR,
                                  "UDP errors",
                                  EBPF_COMMON_DIMENSION_CALL,
                                  NETDATA_SOCKET_KERNEL_FUNCTIONS,
                                  NETDATA_EBPF_CHART_TYPE_LINE,
                                  NULL,
                                  order++,
                                  em->update_every);
    }

    fflush(stdout);
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

    if (ebpf_read_socket.thread)
        netdata_thread_cancel(*ebpf_read_socket.thread);

    if (em->enabled == NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        pthread_mutex_lock(&lock);

        if (em->cgroup_charts) {
            ebpf_obsolete_socket_cgroup_charts(em);
            fflush(stdout);
        }

        if (em->apps_charts & NETDATA_EBPF_APPS_FLAG_CHART_CREATED) {
            ebpf_socket_obsolete_apps_charts(em);
            fflush(stdout);
        }

        ebpf_socket_obsolete_global_charts(em);

#ifdef NETDATA_DEV_MODE
        if (ebpf_aral_socket_pid)
            ebpf_statistic_obsolete_aral_chart(em, socket_disable_priority);
#endif
        pthread_mutex_unlock(&lock);
    }

    ebpf_socket_free(em);
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
static void ebpf_socket_create_global_charts(ebpf_module_t *em)
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

    fflush(stdout);
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
static int ebpf_is_port_inside_range(uint16_t cmp)
{
    // We do not have restrictions for ports.
    if (!network_viewer_opt.excluded_port && !network_viewer_opt.included_port)
        return 1;

    // Test if port is excluded
    ebpf_network_viewer_port_list_t *move = network_viewer_opt.excluded_port;
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
 * @param data    the socket data used also used to refuse some sockets.
 *
 * @return It returns 1 if this socket is inside the ranges and 0 otherwise.
 */
int ebpf_is_socket_allowed(netdata_socket_idx_t *key, netdata_socket_t *data)
{
    int ret = 0;
    // If family is not AF_UNSPEC and it is different of specified
    if (network_viewer_opt.family && network_viewer_opt.family != data->family)
        goto endsocketallowed;

    if (!ebpf_is_port_inside_range(key->dport))
        goto endsocketallowed;

    ret = ebpf_is_specific_ip_inside_range(&key->daddr, data->family);

endsocketallowed:
    return ret;
}

/**
 * Hash accumulator
 *
 * @param values        the values used to calculate the data.
 * @param family        the connection family
 * @param end           the values size.
 */
static void ebpf_hash_socket_accumulator(netdata_socket_t *values, int end)
{
    int i;
    uint8_t protocol = values[0].protocol;
    uint64_t ct = values[0].current_timestamp;
    uint64_t ft = values[0].first_timestamp;
    uint16_t family = AF_UNSPEC;
    uint32_t external_origin = values[0].external_origin;
    for (i = 1; i < end; i++) {
        netdata_socket_t *w = &values[i];

        values[0].tcp.call_tcp_sent         += w->tcp.call_tcp_sent;
        values[0].tcp.call_tcp_received     += w->tcp.call_tcp_received;
        values[0].tcp.tcp_bytes_received    += w->tcp.tcp_bytes_received;
        values[0].tcp.tcp_bytes_sent        += w->tcp.tcp_bytes_sent;
        values[0].tcp.close                 += w->tcp.close;
        values[0].tcp.retransmit            += w->tcp.retransmit;
        values[0].tcp.ipv4_connect          += w->tcp.ipv4_connect;
        values[0].tcp.ipv6_connect          += w->tcp.ipv6_connect;

        if (!protocol)
            protocol = w->protocol;

        if (family == AF_UNSPEC)
            family = w->family;

        if (w->current_timestamp > ct)
            ct = w->current_timestamp;

        if (!ft)
            ft = w->first_timestamp;

        if (w->external_origin)
            external_origin = NETDATA_EBPF_SRC_IP_ORIGIN_EXTERNAL;
    }

    values[0].protocol          = (!protocol)?IPPROTO_TCP:protocol;
    values[0].current_timestamp = ct;
    values[0].first_timestamp = ft;
    values[0].external_origin = external_origin;
}

/**
 * Translate socket
 *
 * Convert socket address to string
 *
 * @param dst structure where we will store
 * @param key the socket address
 */
static void ebpf_socket_translate(netdata_socket_plus_t *dst, netdata_socket_idx_t *key)
{
    uint32_t resolve = network_viewer_opt.service_resolution_enabled;
    char service[NI_MAXSERV];
    int ret;
    if (dst->data.family == AF_INET) {
        struct sockaddr_in ipv4_addr = { };
        ipv4_addr.sin_port = 0;
        ipv4_addr.sin_addr.s_addr = key->saddr.addr32[0];
        ipv4_addr.sin_family = AF_INET;
        if (resolve) {
            // NI_NAMEREQD : It is too slow
            ret = getnameinfo((struct sockaddr *) &ipv4_addr, sizeof(ipv4_addr), dst->socket_string.src_ip,
                              INET6_ADDRSTRLEN, service, NI_MAXSERV, NI_NUMERICHOST | NI_NUMERICSERV);
            if (ret) {
                collector_error("Cannot resolve name: %s", gai_strerror(ret));
                resolve = 0;
            } else {
                ipv4_addr.sin_addr.s_addr = key->daddr.addr32[0];

                ipv4_addr.sin_port = key->dport;
                ret = getnameinfo((struct sockaddr *) &ipv4_addr, sizeof(ipv4_addr), dst->socket_string.dst_ip,
                                  INET6_ADDRSTRLEN, dst->socket_string.dst_port, NI_MAXSERV,
                                  NI_NUMERICHOST);
                if (ret) {
                    collector_error("Cannot resolve name: %s", gai_strerror(ret));
                    resolve = 0;
                }
            }
        }

        // When resolution fail, we should use addresses
        if (!resolve) {
            ipv4_addr.sin_addr.s_addr = key->saddr.addr32[0];

            if(!inet_ntop(AF_INET, &ipv4_addr.sin_addr, dst->socket_string.src_ip, INET6_ADDRSTRLEN))
                netdata_log_info("Cannot convert IP %u .", ipv4_addr.sin_addr.s_addr);

            ipv4_addr.sin_addr.s_addr = key->daddr.addr32[0];

            if(!inet_ntop(AF_INET, &ipv4_addr.sin_addr, dst->socket_string.dst_ip, INET6_ADDRSTRLEN))
                netdata_log_info("Cannot convert IP %u .", ipv4_addr.sin_addr.s_addr);
            snprintfz(dst->socket_string.dst_port, NI_MAXSERV, "%u",  ntohs(key->dport));
        }
    } else {
        struct sockaddr_in6 ipv6_addr = { };
        memcpy(&ipv6_addr.sin6_addr, key->saddr.addr8, sizeof(key->saddr.addr8));
        ipv6_addr.sin6_family = AF_INET6;
        if (resolve) {
            ret = getnameinfo((struct sockaddr *) &ipv6_addr, sizeof(ipv6_addr), dst->socket_string.src_ip,
                              INET6_ADDRSTRLEN, service, NI_MAXSERV, NI_NUMERICHOST | NI_NUMERICSERV);
            if (ret) {
                collector_error("Cannot resolve name: %s", gai_strerror(ret));
                resolve = 0;
            } else {
                memcpy(&ipv6_addr.sin6_addr, key->daddr.addr8, sizeof(key->daddr.addr8));
                ret = getnameinfo((struct sockaddr *) &ipv6_addr, sizeof(ipv6_addr), dst->socket_string.dst_ip,
                                  INET6_ADDRSTRLEN, dst->socket_string.dst_port, NI_MAXSERV,
                                  NI_NUMERICHOST);
                if (ret) {
                    collector_error("Cannot resolve name: %s", gai_strerror(ret));
                    resolve = 0;
                }
            }
        }

        if (!resolve) {
            memcpy(&ipv6_addr.sin6_addr, key->saddr.addr8, sizeof(key->saddr.addr8));
            if(!inet_ntop(AF_INET6, &ipv6_addr.sin6_addr, dst->socket_string.src_ip, INET6_ADDRSTRLEN))
                netdata_log_info("Cannot convert IPv6 Address.");

            memcpy(&ipv6_addr.sin6_addr, key->daddr.addr8, sizeof(key->daddr.addr8));
            if(!inet_ntop(AF_INET6, &ipv6_addr.sin6_addr, dst->socket_string.dst_ip, INET6_ADDRSTRLEN))
                netdata_log_info("Cannot convert IPv6 Address.");
            snprintfz(dst->socket_string.dst_port, NI_MAXSERV, "%u",  ntohs(key->dport));
        }
    }
    dst->pid = key->pid;

    if (!strcmp(dst->socket_string.dst_port, "0"))
        snprintfz(dst->socket_string.dst_port, NI_MAXSERV, "%u",  ntohs(key->dport));
#ifdef NETDATA_DEV_MODE
    collector_info("New socket: { ORIGIN IP: %s, ORIGIN : %u, DST IP:%s, DST PORT: %s, PID: %u, PROTO: %d, FAMILY: %d}",
                   dst->socket_string.src_ip,
                   dst->data.external_origin,
                   dst->socket_string.dst_ip,
                   dst->socket_string.dst_port,
                   dst->pid,
                   dst->data.protocol,
                   dst->data.family
                   );
#endif
}

/**
 * Update array vectors
 *
 * Read data from hash table and update vectors.
 *
 * @param em the structure with configuration
 */
static void ebpf_update_array_vectors(ebpf_module_t *em)
{
    netdata_thread_disable_cancelability();
    netdata_socket_idx_t key = {};
    netdata_socket_idx_t next_key = {};

    int maps_per_core = em->maps_per_core;
    int fd = em->maps[NETDATA_SOCKET_OPEN_SOCKET].map_fd;

    netdata_socket_t *values = socket_values;
    size_t length = sizeof(netdata_socket_t);
    int test, end;
    if (maps_per_core) {
        length *= ebpf_nprocs;
        end = ebpf_nprocs;
    } else
        end = 1;

    // We need to reset the values when we are working on kernel 4.15 or newer, because kernel does not create
    // values for specific processor unless it is used to store data. As result of this behavior one the next socket
    // can have values from the previous one.
    memset(values, 0, length);
    time_t update_time = time(NULL);
    while (bpf_map_get_next_key(fd, &key, &next_key) == 0) {
        test = bpf_map_lookup_elem(fd, &key, values);
        if (test < 0) {
            goto end_socket_loop;
        }

        if (key.pid > (uint32_t)pid_max) {
            goto end_socket_loop;
        }

        ebpf_hash_socket_accumulator(values, end);
        ebpf_socket_fill_publish_apps(key.pid, values);

        // We update UDP to show info with charts, but we do not show them with functions
        /*
        if (key.dport == NETDATA_EBPF_UDP_PORT && values[0].protocol == IPPROTO_UDP) {
            bpf_map_delete_elem(fd, &key);
            goto end_socket_loop;
        }
         */

        // Discard non-bind sockets
        if (!key.daddr.addr64[0] && !key.daddr.addr64[1] && !key.saddr.addr64[0] && !key.saddr.addr64[1]) {
            bpf_map_delete_elem(fd, &key);
            goto end_socket_loop;
        }

        // When socket is not allowed, we do not append it to table, but we are still keeping it to accumulate data.
        if (!ebpf_is_socket_allowed(&key, values)) {
            goto end_socket_loop;
        }

        // Get PID structure
        rw_spinlock_write_lock(&ebpf_judy_pid.index.rw_spinlock);
        PPvoid_t judy_array = &ebpf_judy_pid.index.JudyLArray;
        netdata_ebpf_judy_pid_stats_t *pid_ptr = ebpf_get_pid_from_judy_unsafe(judy_array, key.pid);
        if (!pid_ptr) {
            goto end_socket_loop;
        }

        // Get Socket structure
        rw_spinlock_write_lock(&pid_ptr->socket_stats.rw_spinlock);
        netdata_socket_plus_t **socket_pptr = (netdata_socket_plus_t **)ebpf_judy_insert_unsafe(
            &pid_ptr->socket_stats.JudyLArray, values[0].first_timestamp);
        netdata_socket_plus_t *socket_ptr = *socket_pptr;
        bool translate = false;
        if (likely(*socket_pptr == NULL)) {
            *socket_pptr = aral_mallocz(aral_socket_table);

            socket_ptr = *socket_pptr;

            translate = true;
        }
        uint64_t prev_period = socket_ptr->data.current_timestamp;
        memcpy(&socket_ptr->data, &values[0], sizeof(netdata_socket_t));
        if (translate)
            ebpf_socket_translate(socket_ptr, &key);
        else { // Check socket was updated
            if (prev_period) {
                if (values[0].current_timestamp > prev_period) // Socket updated
                    socket_ptr->last_update = update_time;
                else if ((update_time - socket_ptr->last_update) > em->update_every) {
                    // Socket was not updated since last read
                    JudyLDel(&pid_ptr->socket_stats.JudyLArray, values[0].first_timestamp, PJE0);
                    aral_freez(aral_socket_table, socket_ptr);
                }
            } else // First time
                socket_ptr->last_update = update_time;
        }

        rw_spinlock_write_unlock(&pid_ptr->socket_stats.rw_spinlock);
        rw_spinlock_write_unlock(&ebpf_judy_pid.index.rw_spinlock);

end_socket_loop:
        memset(values, 0, length);
        memcpy(&key, &next_key, sizeof(key));
    }
    netdata_thread_enable_cancelability();
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
void *ebpf_read_socket_thread(void *ptr)
{
    heartbeat_t hb;
    heartbeat_init(&hb);

    ebpf_module_t *em = (ebpf_module_t *)ptr;

    ebpf_update_array_vectors(em);

    int update_every = em->update_every;
    int counter = update_every - 1;

    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    usec_t period = update_every * USEC_PER_SEC;
    while (!ebpf_exit_plugin && running_time < lifetime) {
        (void)heartbeat_next(&hb, period);
        if (ebpf_exit_plugin || ++counter != update_every)
            continue;

        ebpf_update_array_vectors(em);

        counter = 0;
    }

    return NULL;
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
    netdata_log_info("The network viewer is monitoring inbound connections for port %u", ntohs(value));
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
 * Read the hash table and store data to allocated vectors.
 *
 * @param stats         vector used to read data from control table.
 * @param maps_per_core      do I need to read all cores?
 */
static void ebpf_socket_read_hash_global_tables(netdata_idx_t *stats, int maps_per_core)
{
    netdata_idx_t res[NETDATA_SOCKET_COUNTER];
    ebpf_read_global_table_stats(res,
                                 socket_hash_values,
                                 socket_maps[NETDATA_SOCKET_GLOBAL].map_fd,
                                 maps_per_core,
                                 NETDATA_KEY_CALLS_TCP_SENDMSG,
                                 NETDATA_SOCKET_COUNTER);

    ebpf_read_global_table_stats(stats,
                                 socket_hash_values,
                                 socket_maps[NETDATA_SOCKET_TABLE_CTRL].map_fd,
                                 maps_per_core,
                                 NETDATA_CONTROLLER_PID_TABLE_ADD,
                                 NETDATA_CONTROLLER_END);

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
 * @param ns           the structure with data read from memory.
 */
void ebpf_socket_fill_publish_apps(uint32_t current_pid, netdata_socket_t *ns)
{
    ebpf_socket_publish_apps_t *curr = socket_bandwidth_curr[current_pid];
    if (!curr) {
        curr = ebpf_socket_stat_get();
        socket_bandwidth_curr[current_pid] = curr;
    }

    curr->bytes_sent += ns->tcp.tcp_bytes_sent;
    curr->bytes_received += ns->tcp.tcp_bytes_received;
    curr->call_tcp_sent += ns->tcp.call_tcp_sent;
    curr->call_tcp_received += ns->tcp.call_tcp_received;
    curr->retransmit += ns->tcp.retransmit;
    curr->call_close += ns->tcp.close;
    curr->call_tcp_v4_connection += ns->tcp.ipv4_connect;
    curr->call_tcp_v6_connection += ns->tcp.ipv6_connect;

    curr->call_udp_sent += ns->udp.call_udp_sent;
    curr->call_udp_received += ns->udp.call_udp_received;
}

/**
 * Update cgroup
 *
 * Update cgroup data based in PIDs.
 */
static void ebpf_update_socket_cgroup()
{
    ebpf_cgroup_target_t *ect ;

    pthread_mutex_lock(&mutex_cgroup_shm);
    for (ect = ebpf_cgroup_pids; ect; ect = ect->next) {
        struct pid_on_target2 *pids;
        for (pids = ect->pids; pids; pids = pids->next) {
            int pid = pids->pid;
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
        netdata_socket_t *w = &pids->socket;

        accumulator.bytes_received += w->tcp.tcp_bytes_received;
        accumulator.bytes_sent += w->tcp.tcp_bytes_sent;
        accumulator.call_tcp_received += w->tcp.call_tcp_received;
        accumulator.call_tcp_sent += w->tcp.call_tcp_sent;
        accumulator.retransmit += w->tcp.retransmit;
        accumulator.call_close += w->tcp.close;
        accumulator.call_tcp_v4_connection += w->tcp.ipv4_connect;
        accumulator.call_tcp_v6_connection += w->tcp.ipv6_connect;
        accumulator.call_udp_received += w->udp.call_udp_received;
        accumulator.call_udp_sent += w->udp.call_udp_sent;

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

    int cgroups = em->cgroup_charts;
    if (cgroups)
        ebpf_socket_update_cgroup_algorithm();

    int socket_global_enabled = em->global_charts;
    int update_every = em->update_every;
    int maps_per_core = em->maps_per_core;
    int counter = update_every - 1;
    uint32_t running_time = 0;
    uint32_t lifetime = em->lifetime;
    netdata_idx_t *stats = em->hash_table_stats;
    memset(stats, 0, sizeof(em->hash_table_stats));
    while (!ebpf_exit_plugin && running_time < lifetime) {
        (void)heartbeat_next(&hb, USEC_PER_SEC);
        if (ebpf_exit_plugin || ++counter != update_every)
            continue;

        counter = 0;
        netdata_apps_integration_flags_t socket_apps_enabled = em->apps_charts;
        if (socket_global_enabled) {
            read_listen_table();
            ebpf_socket_read_hash_global_tables(stats, maps_per_core);
        }

        pthread_mutex_lock(&collect_data_mutex);
        if (cgroups)
            ebpf_update_socket_cgroup();

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

        pthread_mutex_unlock(&lock);
        pthread_mutex_unlock(&collect_data_mutex);

        pthread_mutex_lock(&ebpf_exit_cleanup);
        if (running_time && !em->running_time)
            running_time = update_every;
        else
            running_time += update_every;

        em->running_time = running_time;
        pthread_mutex_unlock(&ebpf_exit_cleanup);
    }
}

/*****************************************************************
 *
 *  FUNCTIONS TO START THREAD
 *
 *****************************************************************/

/**
 * Initialize vectors used with this thread.
 *
 * We are not testing the return, because callocz does this and shutdown the software
 * case it was not possible to allocate.
 */
static void ebpf_socket_initialize_global_vectors()
{
    memset(socket_aggregated_data, 0 ,NETDATA_MAX_SOCKET_VECTOR * sizeof(netdata_syscall_stat_t));
    memset(socket_publish_aggregated, 0 ,NETDATA_MAX_SOCKET_VECTOR * sizeof(netdata_publish_syscall_t));
    socket_hash_values = callocz(ebpf_nprocs, sizeof(netdata_idx_t));

    ebpf_socket_aral_init();
    socket_bandwidth_curr = callocz((size_t)pid_max, sizeof(ebpf_socket_publish_apps_t *));

    aral_socket_table = ebpf_allocate_pid_aral(NETDATA_EBPF_SOCKET_ARAL_TABLE_NAME,
                                               sizeof(netdata_socket_plus_t));

    socket_values = callocz((size_t)ebpf_nprocs, sizeof(netdata_socket_t));
}

/*****************************************************************
 *
 *  EBPF SOCKET THREAD
 *
 *****************************************************************/

/**
 * Link dimension name
 *
 * Link user specified names inside a link list.
 *
 * @param port the port number associated to the dimension name.
 * @param hash the calculated hash for the dimension name.
 * @param name the dimension name.
 */
static void ebpf_link_dimension_name(char *port, uint32_t hash, char *value)
{
    int test = str2i(port);
    if (test < NETDATA_MINIMUM_PORT_VALUE || test > NETDATA_MAXIMUM_PORT_VALUE){
        netdata_log_error("The dimension given (%s = %s) has an invalid value and it will be ignored.", port, value);
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
                netdata_log_info("Duplicated definition for a service, the name %s will be ignored. ", names->name);
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
    netdata_log_info("Adding values %s( %u) to dimension name list used on network viewer", w->name, htons(w->port));
#endif
}

/**
 * Parse service Name section.
 *
 * This function gets the values that will be used to overwrite dimensions.
 *
 * @param cfg the configuration structure
 */
void ebpf_parse_service_name_section(struct config *cfg)
{
    struct section *co = appconfig_get_section(cfg, EBPF_SERVICE_NAME_SECTION);
    if (co) {
        struct config_option *cv;
        for (cv = co->values; cv ; cv = cv->next) {
            ebpf_link_dimension_name(cv->name, cv->hash, cv->value);
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
            ebpf_link_dimension_name(port_string, simple_hash(port_string), "Netdata");
    }
}

/**
 * Parse table size options
 *
 * @param cfg configuration options read from user file.
 */
void parse_table_size_options(struct config *cfg)
{
    socket_maps[NETDATA_SOCKET_OPEN_SOCKET].user_input = (uint32_t) appconfig_get_number(cfg,
                                                                                        EBPF_GLOBAL_SECTION,
                                                                                        EBPF_CONFIG_SOCKET_MONITORING_SIZE,
                                                                                        NETDATA_MAXIMUM_CONNECTIONS_ALLOWED);

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
        netdata_log_error("%s %s", EBPF_DEFAULT_ERROR_MSG, em->info.thread_name);
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
    if (em->enabled > NETDATA_THREAD_EBPF_FUNCTION_RUNNING) {
        collector_error("There is already a thread %s running", em->info.thread_name);
        return NULL;
    }

    em->maps = socket_maps;

    rw_spinlock_write_lock(&network_viewer_opt.rw_spinlock);
    // It was not enabled from main config file (ebpf.d.conf)
    if (!network_viewer_opt.enabled)
        network_viewer_opt.enabled = appconfig_get_boolean(&socket_config, EBPF_NETWORK_VIEWER_SECTION, "enabled",
                                                           CONFIG_BOOLEAN_YES);
    rw_spinlock_write_unlock(&network_viewer_opt.rw_spinlock);

    parse_table_size_options(&socket_config);

    ebpf_socket_initialize_global_vectors();

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

    ebpf_read_socket.thread = mallocz(sizeof(netdata_thread_t));
    netdata_thread_create(ebpf_read_socket.thread,
                          ebpf_read_socket.name,
                          NETDATA_THREAD_OPTION_DEFAULT,
                          ebpf_read_socket_thread,
                          em);

    pthread_mutex_lock(&lock);
    ebpf_socket_create_global_charts(em);

    ebpf_update_stats(&plugin_statistics, em);
    ebpf_update_kernel_memory_with_vector(&plugin_statistics, em->maps, EBPF_ACTION_STAT_ADD);

#ifdef NETDATA_DEV_MODE
    if (ebpf_aral_socket_pid)
        socket_disable_priority = ebpf_statistic_create_aral_chart(NETDATA_EBPF_SOCKET_ARAL_NAME, em);
#endif

    pthread_mutex_unlock(&lock);

    socket_collector(em);

endsocket:
    ebpf_update_disabled_plugin_stats(em);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
