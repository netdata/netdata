// SPDX-License-Identifier: GPL-3.0-or-later

//#include <daemon/main.h>
//#include "../../libnetdata/libnetdata.h"

#include <sys/time.h>
#include <sys/resource.h>

#include "network_viewer_plugin.h"

// callback required by eval()
int health_variable_lookup(const char *variable, uint32_t hash, struct rrdcalc *rc, calculated_number *result) {
    (void)variable;
    (void)hash;
    (void)rc;
    (void)result;
    return 0;
};

void send_statistics( const char *action, const char *action_result, const char *action_data) {
    (void) action;
    (void) action_result;
    (void) action_data;
    return;
}

// callbacks required by popen()
void signals_block(void) {};
void signals_unblock(void) {};
void signals_reset(void) {};

// required by get_system_cpus()
char *netdata_configured_host_prefix = "";

// callback required by fatal()
void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}
// ----------------------------------------------------------------------

void *libnetdatanv = NULL;
int (*load_bpf_file)(char *) = NULL;
int (*test_bpf_perf_event)(int);
int (*perf_event_mmap)(int);
int (*perf_event_mmap_header)(int, struct perf_event_mmap_page **);
void (*netdata_perf_loop_multi)(int *, struct perf_event_mmap_page **, int, int *, int (*nsb)(void *, int));

static int pmu_fd[NETDATA_MAX_PROCESSOR];
static struct perf_event_mmap_page *headers[NETDATA_MAX_PROCESSOR];

netdata_network_t *outgoing_table = NULL;
netdata_network_t *ingoing_table = NULL;
netdata_port_list_t *port_list = NULL;
netdata_control_connection_t connection_controller;

static char *user_config_dir = NULL;
static char *stock_config_dir = NULL;
static char *plugin_dir = NULL;

uint32_t *econn_udp = NULL;
uint64_t *ibytes_udp = NULL;
uint64_t *ebytes_udp = NULL;
uint32_t *econn_tcp = NULL;
uint64_t *ibytes_tcp = NULL;
uint64_t *ebytes_tcp = NULL;

//protocols used with this collector
static char *protocols[] = { "tcp", "udp" };

static int update_every = 1;

// ----------------------------------------------------------------------

static int is_ip_inside_this_range(in_addr_t val, netdata_network_t *lnn) {
    while (lnn) {
        in_addr_t ip = lnn->first;
        in_addr_t mask = lnn->netmask;

        if ((ip & mask) == (val & mask)) {
            return 1;
        }

        lnn = lnn->next;
    }

    return 0;
}

static inline int is_ip_inside_table(in_addr_t src, in_addr_t dst) {
    return (is_ip_inside_this_range(src, outgoing_table) &&  is_ip_inside_this_range(dst, ingoing_table));
}

void netdata_set_port_stats(netdata_port_stats_t *p, uint16_t dport, uint8_t protocol) {
    char port[8];
    struct servent *sname;

    p->port = dport;
    p->protocol = protocol;

    char *proto = ( protocol == 6 )?protocols[0]:protocols[1];
    sname = getservbyport(dport,proto);
    if (sname) {
        p->dimension = strdup(sname->s_name);
    } else {
        snprintf(port, 8, "%d", ntohs(p->port) );
        p->dimension = strdup(port);
    }
    p->next = NULL ;
}


// ----------------------------------------------------------------------
static void netdata_create_chart(char *family, char *name, char *msg, char *axis, char *group, int priority) {
    printf("CHART %s.%s '' '%s' '%s' '%s' '' line %d 1 ''\n"
            , family
            , name
            , msg
            , axis
            , group
            , priority);

    netdata_port_list_t *move = port_list;
    while (move) {
        printf("DIMENSION %s '' absolute 1 1\n", move->dimension);

        move = move->next;
    }
}

static void netdata_create_charts() {
    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_TCP_OUTBOUND_IPV6, "TCP request size to specific port.", "kilobits/s", "TCP", 1000);
    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_TCP_INBOUND_IPV6, "TCP receive size from specific port.", "kilobits/s", "TCP", 999);
    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_TCP_CONNECTION_OUTBOUND_IPV6, "TCP active connections per port.", "active connections", "TCP", 998);

    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_UDP_OUTBOUND_IPV6, "UDP request size to specific port.", "kilobits/s", "UDP", 997);
    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_UDP_INBOUND_IPV6, "UDP receive size from specific port.", "kilobits/s", "UDP", 996);
    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_UDP_CONNECTION_OUTBOUND_IPV6, "UDP active connections per port.", "active connections", "UDP", 995);

    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_TCP_OUTBOUND_IPV4, "TCP request size to specific port.", "kilobits/s", "TCP", 994);
    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_TCP_INBOUND_IPV4, "TCP receive size from specific port.", "kilobits/s", "TCP", 993);
    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_TCP_CONNECTION_OUTBOUND_IPV4, "TCP active connections per port.", "active connections", "TCP", 992);

    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_UDP_OUTBOUND_IPV4, "UDP request size to specific port.", "kilobits/s","UDP", 991);
    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_UDP_INBOUND_IPV4, "UDP receive size from specific port.", "kilobits/s","UDP", 990);
    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_UDP_CONNECTION_OUTBOUND_IPV4, "UDP active connections per port.", "active connections","UDP", 989);

}

static void write_connection(char *name, uint32_t *bytes) {
    printf( "BEGIN %s.%s\n"
            , NETWORK_VIEWER_FAMILY
            , name);

    uint16_t i = 0;
    netdata_port_list_t *move = port_list;
    while (move) {
        printf("SET %s = %lld\n", move->dimension, (long long) bytes[i]);
        i++;
        move = move->next;
    }

    printf("END\n");
}

static void write_traffic(char *name, uint64_t *bytes) {
    printf( "BEGIN %s.%s\n"
            , NETWORK_VIEWER_FAMILY
            , name);

    uint16_t i = 0;
    netdata_port_list_t *move = port_list;
    while (move) {
        printf("SET %s = %lld\n", move->dimension, (long long) bytes[i]);
        i++;

        move = move->next;
    }

    printf("END\n");
}

static void netdata_publish_data(netdata_port_stats_t *move, int version) {
    //fill content
    uint16_t tcp = 0;
    uint16_t udp = 0;

    uint64_t *ibytes;
    uint64_t *ebytes;
    uint32_t *econn;
    while (move) {
        if (move->protocol == 6) {//tcp
            ibytes = &ibytes_tcp[tcp];
            ebytes = &ebytes_tcp[tcp];
            econn = &econn_tcp[tcp];
            tcp++;
        } else {
            ibytes = &ibytes_udp[udp];
            ebytes = &ebytes_udp[udp];
            econn = &econn_udp[udp];
            udp++;
        }

        if(move->inow != move->iprev) {
            *ibytes = (move->inow - move->iprev)/1000;
            move->iprev = move->inow;

            *ebytes = (move->enow - move->eprev)/1000;
            move->eprev = move->enow;

            *econn = move->etot;
        } else {
            *ibytes = 0;
            *ebytes = 0;

            *econn = move->etot;
        }

        move = move->next;
    }

    if (version == AF_INET) {
        write_traffic(NETWORK_VIEWER_TCP_INBOUND_IPV4, ibytes_tcp);
        write_traffic(NETWORK_VIEWER_UDP_INBOUND_IPV4, ibytes_udp);

        write_traffic(NETWORK_VIEWER_TCP_OUTBOUND_IPV4, ebytes_tcp);
        write_traffic(NETWORK_VIEWER_UDP_OUTBOUND_IPV4, ebytes_udp);

        write_connection(NETWORK_VIEWER_TCP_CONNECTION_OUTBOUND_IPV4, econn_tcp);
        write_connection(NETWORK_VIEWER_UDP_CONNECTION_OUTBOUND_IPV4, econn_udp);
    } else {
        write_traffic(NETWORK_VIEWER_TCP_INBOUND_IPV6, ibytes_tcp);
        write_traffic(NETWORK_VIEWER_UDP_INBOUND_IPV6, ibytes_udp);

        write_traffic(NETWORK_VIEWER_TCP_OUTBOUND_IPV6, ebytes_tcp);
        write_traffic(NETWORK_VIEWER_UDP_OUTBOUND_IPV6, ebytes_udp);

        write_connection(NETWORK_VIEWER_TCP_CONNECTION_OUTBOUND_IPV6, econn_tcp);
        write_connection(NETWORK_VIEWER_UDP_CONNECTION_OUTBOUND_IPV6, econn_udp);
    }
}

void *network_viewer_publisher(void *ptr) {
    (void)ptr;
    netdata_create_charts();

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!netdata_exit) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        netdata_publish_data(connection_controller.ports_ipv4, AF_INET);
        netdata_publish_data(connection_controller.ports_ipv6, AF_INET6);
        fflush(stdout);
    }

    return NULL;
}

// ----------------------------------------------------------------------
void netdata_set_conn_stats(netdata_conn_stats_t *ncs, netdata_kern_stats_t *e) {
    uint16_t family = e->family;
    ncs->first = e->first;
    ncs->ct = e->ct;

    ncs->saddr.addr32[0] = e->saddr.addr32[0];
    ncs->daddr.addr32[0] = e->daddr.addr32[0];
    if (family == AF_INET6) {
        ncs->saddr.addr32[1] = e->saddr.addr32[1];
        ncs->saddr.addr32[2] = e->saddr.addr32[2];
        ncs->saddr.addr32[3] = e->saddr.addr32[3];

        ncs->daddr.addr32[1] = e->daddr.addr32[1];
        ncs->daddr.addr32[2] = e->daddr.addr32[2];
        ncs->daddr.addr32[3] = e->daddr.addr32[3];
    }

    ncs->dport = e->dport;
    ncs->retransmit = e->retransmit;
    ncs->sent = e->sent;
    ncs->recv = e->recv;

    ncs->protocol = e->protocol;
    ncs->family = family;

    ncs->remove_time = (!e->removeme)?0:time(NULL) + 5;

    ncs->next = NULL;
}

void netdata_update_conn_stats(netdata_conn_stats_t *ncs, netdata_kern_stats_t *e) {
    ncs->ct = e->ct;

    ncs->retransmit += e->retransmit;
    ncs->sent += e->sent;
    ncs->recv += e->recv;

    ncs->remove_time = (!e->removeme)?0:time(NULL) + 5;
}

netdata_conn_stats_t *store_new_connection_stat(netdata_kern_stats_t *e) {
    netdata_conn_stats_t *ncs = callocz(1, sizeof(netdata_conn_stats_t));
    if(ncs) {
        netdata_set_conn_stats(ncs, e);
    }

    return ncs;
}

void netdata_update_port_stats(netdata_port_stats_t *p, netdata_kern_stats_t *e) {
    p->inow += e->recv;
    p->enow += e->sent;

    netdata_conn_stats_t *ret;
    netdata_conn_stats_t search = { .daddr.addr32[0] = e->daddr.addr32[0], .saddr.addr32[0] = e->saddr.addr32[0], .dport = e->dport };
    netdata_conn_stats_t *ncs = (netdata_conn_stats_t *)avl_search_lock(&p->destination_port, (avl *)&search);
    if (!ncs) {
        ncs = store_new_connection_stat(e);
        if(ncs) {
            p->etot += 1;

            if (!connection_controller.tree) {
                connection_controller.last_connection = ncs;
                ncs->prev = NULL;
                connection_controller.tree = ncs;
            } else {
                connection_controller.last_connection->next = ncs;
                ncs->prev = connection_controller.last_connection;
                connection_controller.last_connection = ncs;
            }

            ret = (netdata_conn_stats_t *)avl_insert_lock(&p->destination_port, (avl *)ncs);
            if(ret != ncs) {
                error("[NETWORK VIEWER] Cannot insert a new connection to index.");
            } else {
                connection_controller.last_connection->next = ncs;
                ncs->prev = connection_controller.last_connection;
                connection_controller.last_connection = ncs;
            }
        }
    } else {
        netdata_update_conn_stats(ncs, e);
    }

    if(ncs && e->removeme) {
        ret = (netdata_conn_stats_t *)avl_remove_lock(&p->destination_port, (avl *)ncs);
        if (ret != ncs) {
            error("[NETWORK VIEWER] Cannot remove a connection from index.");
        }

        p->etot -= 1;

        if(ncs == connection_controller.tree) {
            connection_controller.tree = ncs->next;
            ncs->prev = NULL;
        } else {
            ret = ncs->prev;
            if(ret) {
                ret->next  = ncs->next;
            }
        }

        free(ncs);
    }
}

static int compare_destination_ip(void *a, void *b) {
    netdata_conn_stats_t *conn1 = (netdata_conn_stats_t *)a;
    netdata_conn_stats_t *conn2 = (netdata_conn_stats_t *)b;

    int ret = 0;

    uint32_t ip1 = conn1->daddr.addr32[0];
    uint32_t ip2 = conn2->daddr.addr32[0];

    if (ip1 < ip2) ret = -1;
    else if (ip1 > ip2) ret = 1;

    if (!ret) {
        uint16_t port1 = conn1->dport;
        uint16_t port2 = conn2->dport;

        if (port1 < port2) ret = -1;
        else if (port1 > port2) ret = 1;

        if (!ret) {
            ip1 = conn1->saddr.addr32[0];
            ip2 = conn2->saddr.addr32[0];

            if (ip1 < ip2) ret = -1;
            else if (ip1 > ip2) ret = 1;
        }
    }

    return ret;
}

netdata_port_stats_t *store_new_port_stat(uint16_t dport, uint8_t protocol) {
    netdata_port_stats_t *p = callocz(1, sizeof(netdata_port_stats_t));

    if(p) {
        netdata_set_port_stats(p, dport, protocol);
        avl_init_lock(&p->destination_port, compare_destination_ip);
    }

    return p;
}

static int is_monitored_port(uint16_t port) {
    netdata_port_list_t lnpl;
    lnpl.port = port;

    netdata_port_list_t *ans = (netdata_port_list_t *)avl_search_lock(&connection_controller.port_list, (avl *)&lnpl);
    return (ans)?1:0;
}

int netdata_store_bpf(void *data, int size) {
    (void)size;

    if(netdata_exit)
        return 0; //LIBBPF_PERF_EVENT_DONE

    netdata_kern_stats_t *e = data;
    if(!e->dport) {
        return -2;//LIBBPF_PERF_EVENT_CONT;
    }

    if(!is_monitored_port(e->dport) ) {
        return -2;//LIBBPF_PERF_EVENT_CONT;
    }

    if(e->family == AF_INET) {
        int inside = is_ip_inside_table(e->saddr.addr32[0], e->daddr.addr32[0]);
        if(inside < 0) {
            return -2;//LIBBPF_PERF_EVENT_CONT;
        }
    }

    netdata_port_stats_t search_proto = { .port = e->dport, .protocol = e->protocol };
    netdata_port_stats_t *pp = (netdata_port_stats_t *)avl_search_lock((e->family == AF_INET)?&connection_controller.port_stat_ipv4:&connection_controller.port_stat_ipv6, (avl *)&search_proto);
    if (!pp) {
        return -2;
    } else {
       netdata_update_port_stats(pp, e);
    }

    return -2; //LIBBPF_PERF_EVENT_CONT;
}

static int compare_port(void *a, void *b) {
    netdata_port_stats_t *p1 = (netdata_port_stats_t *)a;
    netdata_port_stats_t *p2 = (netdata_port_stats_t *)b;

    int ret = 0;

    uint8_t proto1 = p1->protocol;
    uint8_t proto2 = p2->protocol;

    if ( proto1 < proto2 ) ret = -1;
    if ( proto1 > proto2 ) ret = 1;

    if (!ret) {
        uint16_t port1 = p1->port;
        uint16_t port2 = p2->port;

        if ( port1 < port2 ) ret = -1;
        if ( port1 > port2 ) ret = 1;
    }

    return ret;
}

void *network_viewer_collector(void *ptr) {
    (void)ptr;

    int nprocs = sysconf(_SC_NPROCESSORS_ONLN);

    netdata_perf_loop_multi(pmu_fd, headers, nprocs, (int *)&netdata_exit, netdata_store_bpf);

    return NULL;
}
// ----------------------------------------------------------------------
static void clean_networks() {
    netdata_network_t *move;
    netdata_network_t *next;

    if(outgoing_table) {
        move = outgoing_table->next;
        while (move) {
            next = move->next;

            free(move);

            move = next;
        }

        free(outgoing_table);
    }

    if(ingoing_table) {
        move = ingoing_table->next;
        while (move) {
            next = move->next;

            free(move);

            move = next;
        }

        free(ingoing_table);
    }
}

void clean_port_index(netdata_port_stats_t *r) {
    avl_tree_lock *ptr = (r->family == AF_INET)?&connection_controller.port_stat_ipv4:&connection_controller.port_stat_ipv6;
    netdata_port_stats_t *ncs = (netdata_port_stats_t *)avl_search_lock(ptr, (avl *)r);
    if (ncs) {
        ncs = (netdata_port_stats_t *)avl_remove_lock(ptr, (avl *)r);
        if (ncs != r) {
            error("[NETWORK VIEWER] Cannot remove a port");
        }
    }
}

void clean_ports(netdata_port_stats_t *clean) {
    netdata_port_stats_t *move = clean->next;
    while (move) {
        netdata_port_stats_t *next = move->next;
        clean_port_index(move);

        free(move->dimension);

        free(move);
        move = next;
    }
    free(clean);
}

void clean_connections() {
    netdata_conn_stats_t *move = connection_controller.tree->next;
    while (move) {
        netdata_conn_stats_t *next = move->next;
        free(move);
        move = next;
    }
    free(connection_controller.tree);

}

void clean_list_ports() {
    netdata_port_list_t *move = port_list->next;
    while (move) {
        netdata_port_list_t *next = move->next;

        free(move);

        move = next;
    }
    free(port_list);
}

static void int_exit(int sig) {
    if(libnetdatanv) {
        dlclose(libnetdatanv);
    }

    if(connection_controller.tree) {
        clean_connections();
    }

    if(connection_controller.ports_ipv4) {
        clean_ports(connection_controller.ports_ipv4);
    }

    if(connection_controller.ports_ipv6) {
        clean_ports(connection_controller.ports_ipv6);
    }

    if(outgoing_table || ingoing_table) {
        clean_networks();
    }

    if(port_list) {
        clean_list_ports();
    }

    if (connection_controller.pti) {
        parse_text_input_t *r = connection_controller.pti;
        while (r) {
            free(r->value);
            parse_text_input_t *save = r->next;

            free(r);
            r = save;
        }
    }

    if (econn_udp) {
        freez(econn_udp);
    }

    if (ibytes_udp) {
        freez(ibytes_udp);
    }

    if (ebytes_udp) {
        freez(econn_udp);
    }

    if (econn_tcp) {
        freez(econn_tcp);
    }

    if (ibytes_tcp) {
        freez(ibytes_tcp);
    }

    if (ebytes_tcp) {
        freez(ebytes_tcp);
    }

    exit(sig);
}

static void build_complete_path(char *out, size_t length, char *filename) {
    if(plugin_dir){
        snprintf(out, length, "%s/%s", plugin_dir, filename);
    } else {
        snprintf(out, length, "%s", filename);
    }
}

int network_viewer_load_libraries() {
    char *err = NULL;
    char lpath[4096];

    build_complete_path(lpath, 4096, "libnetdata_ebpf.so");
    libnetdatanv = dlopen(lpath ,RTLD_LAZY);
    if (!libnetdatanv) {
        error("[NETWORK VIEWER] Cannot load %s.", lpath);
        return -1;
    } else {
        load_bpf_file = dlsym(libnetdatanv, "load_bpf_file");
        if ((err = dlerror()) != NULL) {
            error("[NETWORK VIEWER] Cannot find load_bpf_file: %s", err);
            return -1;
        }

        test_bpf_perf_event = dlsym(libnetdatanv, "test_bpf_perf_event");
        if ((err = dlerror()) != NULL) {
            error("[NETWORK VIEWER] Cannot find test_bpf_perf_event: %s", err);
            return -1;
        }

        netdata_perf_loop_multi = dlsym(libnetdatanv, "my_perf_loop_multi");
        if ((err = dlerror()) != NULL) {
            error("[NETWORK VIEWER] Cannot find my_perf_loop_multi: %s", err);
            return -1;
        }

        perf_event_mmap =  dlsym(libnetdatanv, "perf_event_mmap");
        if ((err = dlerror()) != NULL) {
            error("[NETWORK VIEWER] Cannot find perf_event_mmap: %s", err);
            return -1;
        }

        perf_event_mmap_header =  dlsym(libnetdatanv, "perf_event_mmap_header");
        if ((err = dlerror()) != NULL) {
            error("[NETWORK VIEWER] Cannot find perf_event_mmap_header: %s", err);
            return -1;
        }
    }

    return 0;
}

int network_viewer_load_ebpf() {
    char lpath[4096];

    build_complete_path(lpath, 4096, "netdata_ebpf_network_viewer.o");
    if (load_bpf_file(lpath) ) {
        return -1;
    }

    return 0;
}

static int map_memory() {
    int nprocs = sysconf(_SC_NPROCESSORS_ONLN);

    if ( nprocs > NETDATA_MAX_PROCESSOR ) {
        nprocs = NETDATA_MAX_PROCESSOR;
    }

    int i;
    for ( i = 0 ; i < nprocs ; i++ ) {
        pmu_fd[i] = test_bpf_perf_event(i);

        if (perf_event_mmap(pmu_fd[i]) < 0) {
            error("[NETWORK VIEWER] Cannot map memory used to transfer data.");
            return -1;
        }
    }

    for ( i = 0 ; i < nprocs ; i++ ) {
        if (perf_event_mmap_header(pmu_fd[i], &headers[i]) < 0) {
            error("[NETWORK VIEWER] Cannot map header used to transfer data.");
            return -1;
        }
    }

    return 0;
}

static int compare_port_value(void *a, void *b) {
    netdata_port_list_t *p1 = (netdata_port_list_t *)a;
    netdata_port_list_t *p2 = (netdata_port_list_t *)b;

    uint16_t port1 = p1->port;
    uint16_t port2 = p2->port;

    if ( port1 < port2 ) return -1;
    if ( port1 > port2 ) return  1;

    return 0;
}

static int port_stat_link_list(netdata_port_list_t *pl,uint16_t port, int version) {
    netdata_port_stats_t *pp;
    netdata_port_stats_t *rp;

    netdata_port_stats_t *pi;
    netdata_port_stats_t *lpi;

    if (version == AF_INET) {
        pi = connection_controller.ports_ipv4;
        lpi = connection_controller.last_port_ipv4;
    } else {
        pi = connection_controller.ports_ipv6;
        lpi = connection_controller.last_port_ipv6;
    }

    int ret;
    pp = store_new_port_stat(port, 6); //tcp
    if(pp) {
        rp = (netdata_port_stats_t *)avl_insert_lock((version == AF_INET)?&connection_controller.port_stat_ipv4:&connection_controller.port_stat_ipv6, (avl *)pp);
        if (rp != pp) {
            error("[NETWORK VIEWER] Cannot insert a new port stat inside index.");
            ret = -1;
            goto endpsll;
        }

        pl->dimension = pp->dimension;
        if (!pi) {
            pi = pp;
            lpi = pp;
        } else {
            lpi->next = pp;
            lpi = pp;
        }

        pp = store_new_port_stat(port, 17); //udp
        if(pp) {
            rp = (netdata_port_stats_t *)avl_insert_lock((version == AF_INET)?&connection_controller.port_stat_ipv4:&connection_controller.port_stat_ipv6, (avl *)pp);
            if (rp != pp) {
                error("[NETWORK VIEWER] Cannot insert a new port stat inside index.");
                ret = -1;
                goto endpsll;
            }

            lpi->next = pp;
            lpi = pp;
        } else {
            ret = -1;
            goto endpsll;
        }
    } else {
        ret = -1;
        goto endpsll;
    }

    ret = 0;

endpsll:
    if (version == AF_INET) {
        connection_controller.ports_ipv4 = pi;
        connection_controller.last_port_ipv4 = lpi;
    } else {
        connection_controller.ports_ipv6 = pi;
        connection_controller.last_port_ipv6 = lpi;
    }

    return ret;
}

netdata_port_list_t *netdata_list_ports(char *ports) {
    netdata_port_list_t *ret = NULL, *next;
    netdata_port_list_t *set;

    uint16_t port;

    uint16_t i;
    uint16_t end;
    if (!ports) {
        end = 50;
        for (i = 0; i < end; i++) {
            uint16_t def[] = {  20, 21, 22, 25, 37, 43, 53, 80, 88, 110,
                                118, 123, 135, 137, 138, 139, 143, 156, 194, 389,
                                443, 445, 464, 465, 513, 520, 530, 546, 547, 563,
                                631, 636, 691, 749, 901, 989, 990, 993, 995, 1381,
                                1433, 1434, 1512, 1525, 3389, 3306, 5432, 6000, 8080, 19999 } ;

            set = (netdata_port_list_t *) callocz(1,sizeof(netdata_port_list_t));
            if(set) {
                port = htons(def[i]);
                set->port = port;

                netdata_port_list_t *r = (netdata_port_list_t *)avl_insert_lock(&connection_controller.port_list, (avl *)set);
                if (r != set ) {
                    error("[NETWORK VIEWER] Cannot insert port inside list.");
                }

                if ( port_stat_link_list(r, port, AF_INET) ) {
                    return NULL;
                }

                if ( port_stat_link_list(set, port, AF_INET6) ) {
                    return NULL;
                }

                if(!ret) {
                    ret =  set;
                } else {
                    next->next = set;
                }

                next = set;
            }
        }
        connection_controller.maxports = end;
    } else {
        uint16_t counter =0;
        while (*ports) {
            char *vport = ports;
            while (*ports && *ports != ',' && *ports != ' ') ports++;

            if(*ports) {
                *ports = 0x00;
                ports++;
            }

            vport = trim_all(vport);

            if(vport) {
                char *dash = strchr(vport,'-');
                if(!dash) {
                    i = 0;
                    end = 0;
                } else {
                    *dash = 0x00;
                    i = (uint16_t) strtol(vport,NULL, 10);
                    dash++;
                    end = (uint16_t) strtol(dash,NULL, 10);
                }

                for ( ; i <= end ; i++) {
                    counter++;
                    set = (netdata_port_list_t *) callocz(1,sizeof(netdata_port_list_t));
                    if(set) {
                        if (!dash) {
                            port = htons((uint16_t)strtol(vport,NULL,10));
                        } else {
                            port = htons(i);
                        }
                        set->port = port;

                        netdata_port_list_t *r = (netdata_port_list_t *)avl_insert_lock(&connection_controller.port_list, (avl *)set);
                        if (r != set ) {
                            error("[NETWORK VIEWER] Cannot insert port inside list.");
                            return NULL;
                        }

                        if ( port_stat_link_list(set, port, AF_INET) ) {
                            return NULL;
                        }

                        if ( port_stat_link_list(set, port, AF_INET6) ) {
                            return NULL;
                        }

                        if(!ret) {
                            ret =  set;
                        } else {
                            next->next = set;
                        }

                        next = set;
                    }
                }
            }
        }
        connection_controller.maxports = counter;
    }

    return ret;
}

netdata_network_t *netdata_list_ips(char *ips, int outgoing) {
    netdata_network_t *ret = NULL, *next;
    netdata_network_t *set;
    in_addr_t network;

    if (!ips) {
        static const char *pips[] = { "10.0.0.0", "172.16.0.0", "192.168.0.0", "0.0.0.0" };
        static const char *masks[] = { "255.0.0.0", "255.240.0.0", "255.255.0.0", "0.0.0.0" };

        int i;
        int end = (outgoing)?3:4;
        for (i = (outgoing)?0:3; i < end ; i++) {
            set = (netdata_network_t *) callocz(1,sizeof(netdata_network_t));
            if(set) {
                network = inet_addr(pips[i]);
                set->ipv4addr = network;

                set->first = htonl(ntohl(network)+1);

                network = inet_addr(masks[i]);
                set->netmask = network;

                if(!ret) {
                    ret =  set;
                } else {
                    next->next = set;
                }

                next = set;
            }
        }
    } else {
        while (*ips) {
            char *vip = ips;
            while (*ips && *ips != '/' && *ips != '-') ips++;

            if(*ips) {
                *ips = 0x00;
                ips++;
            }

            char *vmask = ips;
            while (*ips && *ips != ' ' && *ips != ',') ips++;

            if(*ips) {
                *ips = 0x00;
                ips++;
            }

            if(vip && vmask) {
                vip = trim_all(vip);
                vmask = trim_all(vmask);

                set = (netdata_network_t *) callocz(1,sizeof(netdata_network_t));
                if(set) {
                    network = inet_addr(vip);
                    set->ipv4addr = network;

                    set->first = htonl(ntohl(network)+1);

                    if(strlen(vmask) > 3) {
                        network = inet_addr(vmask);
                    } else {
                        network = htonl(~(0xffffffff >> strtol(vmask,NULL,10)));
                    }
                    set->netmask = network;

                    if(!ret) {
                        ret =  set;
                    } else {
                        next->next = set;
                    }

                    next = set;
                }
            }
        }
    }
    return ret;
}

static int read_config_file(const char *path) {
    char filename[512];

    snprintf(filename, 512, "%s/network_viewer.conf", path);
    FILE *fp = fopen(filename, "r");
    if(!fp) {
        return -1;
    }

    char *s;
    char buffer[1024];
    while((s = fgets(buffer,  1024, fp))) {
        s = trim(buffer);
        if (!s || *s == '#') continue;

        char *key = s;
        while (*s && *s != '=') s++;

        if (!*s) {
            continue;
        }

        *s = '\0';
        s++;

        char *value = s;
        key = trim_all(key);
        value = trim_all(value);

        if (!strcasecmp(key, "outgoing")) {
            outgoing_table = netdata_list_ips(value, 1);
        } else if (!strcasecmp(key, "destination_ports")) {
            port_list = netdata_list_ports(value);
        } else if (!strcasecmp(key, "ingoing")) {
            ingoing_table = netdata_list_ips(value, 0);
        } else if (isdigit(key[0])) {
            parse_text_input_t *pti = callocz(1, sizeof(pti));
            if (pti) {
                pti->port = htons((uint16_t) strtol(key, NULL, 10));
                pti->value = strdup(value);
            }

            if (!connection_controller.pti) {
                connection_controller.pti = pti;
            } else {
                parse_text_input_t *r;
                parse_text_input_t *save;
                for (r = connection_controller.pti; r; save = r, r = r->next);
                if (save) {
                    save->next = pti;
                }
            }
        }
    }

    fclose(fp);
    return 0;
}

static void update_dimensions() {
    if (connection_controller.pti) {
        parse_text_input_t *r = connection_controller.pti;
        while (r) {
            //UPDATE TCP
            netdata_port_stats_t search_proto = { .port = r->port, .protocol = 6 };
            netdata_port_stats_t *rp = (netdata_port_stats_t *)avl_insert_lock(&connection_controller.port_stat_ipv4, (avl *)&search_proto);
            if(rp) {
                free(rp->dimension);
                rp->dimension = strdup(r->value);

                netdata_port_list_t lnpl;
                lnpl.port = r->port;
                netdata_port_list_t *r = (netdata_port_list_t *)avl_search_lock(&connection_controller.port_list, (avl *)&lnpl);
                if (r) {
                    r->dimension = rp->dimension;
                }
            }

            //UPDATE UDP
            search_proto.protocol = 17;
            rp = (netdata_port_stats_t *)avl_insert_lock(&connection_controller.port_stat_ipv4, (avl *)&search_proto);
            if(rp) {
                free(rp->dimension);
                rp->dimension = strdup(r->value);
            }

            r = r->next;
        }
    }
}

int allocate_publish_vectors() {
    if(ebytes_tcp) {
        return 0;
    }

    size_t length = (size_t)connection_controller.maxports;
    econn_udp = (uint32_t *)callocz(length, sizeof(uint32_t));
    if(!econn_udp) {
        return -1;
    }

    ibytes_udp = (uint64_t *)callocz(length, sizeof(uint64_t));
    if(!ibytes_udp) {
        return -1;
    }

    ebytes_udp = (uint64_t *)callocz(length, sizeof(uint64_t));
    if(!ebytes_udp) {
        return -1;
    }

    econn_tcp = (uint32_t *)callocz(length, sizeof(uint32_t));
    if(!econn_tcp) {
        return -1;
    }

    ibytes_tcp = (uint64_t *)callocz(length, sizeof(uint64_t));
    if(!ibytes_udp) {
        return -1;
    }

    ebytes_tcp = (uint64_t *)callocz(length, sizeof(uint64_t));
    if(!ebytes_udp) {
        return -1;
    }

    return 0;
}

void parse_config() {
    user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    plugin_dir = getenv("NETDATA_PLUGINS_DIR");

    memset(&connection_controller,0,sizeof(connection_controller));
    avl_init_lock(&connection_controller.port_list, compare_port_value);
    avl_init_lock(&(connection_controller.port_stat_ipv4), compare_port);
    avl_init_lock(&(connection_controller.port_stat_ipv6), compare_port);

    if (!user_config_dir && !stock_config_dir) {
        return;
    }

    int read_file = 0;
    if (user_config_dir) {
        read_file =  read_config_file(user_config_dir);
    }

    if (read_file && stock_config_dir) {
        info("[NETWORK VIEWER] Cannot read configuration file network_viewer.conf");
        read_file =  read_config_file(stock_config_dir);
    }

    if (read_file) {
        info("[NETWORK VIEWER] Cannot read stock file network_viewer.conf, collector will start with default.");
    } else {
        if(port_list) {
            update_dimensions();
        }
    }

    if(!connection_controller.maxports) {
        int_exit(0);
    }

    if(allocate_publish_vectors()) {
        int_exit(8);
    }
}

int main(int argc, char **argv) {
    (void)argc;
    (void)argv;

    //set name
    program_name = "networkviewer.plugin";

    //disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    /*
     * Sometimes this call raises an error when software is called from Netdata, it is necessary to investigate
    struct rlimit r = {RLIM_INFINITY, RLIM_INFINITY};
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        error("[NETWORK VIEWER] setrlimit(RLIMIT_MEMLOCK)");
        return 1;
    }
     */

    if (argc) {
        update_every = (int)strtol(argv[1], NULL, 10);
    }

    parse_config();

    if (network_viewer_load_libraries()) {
        int_exit(2);
    }

    signal(SIGINT, int_exit);
    signal(SIGTERM, int_exit);

    if(network_viewer_load_ebpf()) {
        error("[NETWORK VIEWER] Cannot load eBPF program.");
        int_exit(3);
    }

    if (map_memory()) {
        int_exit(4);
    }

    if(!outgoing_table) {
        outgoing_table = netdata_list_ips(NULL, 1);
        if(!outgoing_table) {
            error("[NETWORK VIEWER] Cannot load outgoing network range to monitor.");
            int_exit(5);
        }
    }

    if(!ingoing_table) {
        ingoing_table = netdata_list_ips(NULL, 0);
        if(!ingoing_table) {
            error("[NETWORK VIEWER] Cannot load ingoing network range to monitor.");
            int_exit(6);
        }
    }

    if(!port_list) {
        port_list = netdata_list_ports(NULL);
        if(!port_list) {
            error("[NETWORK VIEWER] Cannot load network ports to monitor.");
            int_exit(7);
        }
    }

    pthread_attr_t attr;
    pthread_attr_init(&attr);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_JOINABLE);
    pthread_t thread[2];

    int i;
    int end = 2;
    for ( i = 0; i < end ; i++ ) {
        if ( ( pthread_create(&thread[i], &attr, (!i)?network_viewer_publisher:network_viewer_collector, NULL) ) ) {
            error("[NETWORK VIEWER] Cannot create the necessaries threads.");
            int_exit(8);
        }
    }

    for ( i = 0; i < end ; i++ ) {
        if ( (pthread_join(thread[i], NULL) ) ) {
            error("[NETWORK VIEWER] Cannot join the necessaries threads.");
            int_exit(9);
        }
    }

    return 0;
}
