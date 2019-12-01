// SPDX-License-Identifier: GPL-3.0-or-later

//#include <daemon/main.h>
//#include "../../libnetdata/libnetdata.h"

#include <sys/time.h>
#include <sys/resource.h>

#include "network_viewer_plugin.h"

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

//used ,if and only if, there is not any connection
static netdata_port_stats_t *fake1 = NULL;
static netdata_port_stats_t *fake2 = NULL;

//protocols used with this collector
static char *protocols[] = { "tcp", "udp" };

// required by get_system_cpus()
char *netdata_configured_host_prefix = "";

// callback required by fatal()
void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

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

void netdata_set_port_stats(netdata_port_stats_t *p, netdata_kern_stats_t *e) {
    char port[8];
    struct servent *sname;

    p->port = e->dport;
    p->protocol = e->protocol;

    p->iprev = 0;
    p->inow = e->recv;
    p->etot = 1;
    p->eprev = 0;
    p->enow = e->sent;

    char *proto = ( p->protocol == 6 )?protocols[0]:protocols[1];
    sname = getservbyport(e->dport,proto);

    if (sname) {
        p->dimension = strdup(sname->s_name);
    } else {
        snprintf(port, 8, "%d", ntohs(p->port) );
        p->dimension = strdup(port);
    }
    p->next = NULL ;
}


// ----------------------------------------------------------------------
static void write_report(netdata_port_stats_t *ptr) {
    char *chart1;
    char *chart2;
    char *chart4;
    char *dim;
    uint64_t ibytes;
    uint64_t ebytes;
    uint32_t econn;

    if ( ptr->protocol == 6 ) { //TCP
        chart1 = NETWORK_VIEWER_CHART1;
        chart2 = NETWORK_VIEWER_CHART2;
        chart4 = NETWORK_VIEWER_CHART6;
    } else {  //UDP(17)
        chart1 = NETWORK_VIEWER_CHART3;
        chart2 = NETWORK_VIEWER_CHART4;
        chart4 = NETWORK_VIEWER_CHART8;
    }

    dim = ptr->dimension;
    if(dim) {
        ibytes = ptr->inow - ptr->iprev;
        ptr->iprev = ptr->inow;

        ebytes = ptr->enow - ptr->eprev;
        ptr->eprev = ptr->enow;

        econn = ptr->etot;

        //--------------------------------------
        printf( "BEGIN %s.%s\n"
                , NETWORK_VIEWER_FAMILY
                , chart1);

        printf( "SET %s = %lu\n"
                , dim
                , ibytes);

        printf("END\n");

        //--------------------------------------
        printf( "BEGIN %s.%s\n"
                , NETWORK_VIEWER_FAMILY
                , chart2);

        printf( "SET %s = %lu\n"
                , dim
                , ebytes);

        printf("END\n");

        //--------------------------------------
        printf( "BEGIN %s.%s\n"
                , NETWORK_VIEWER_FAMILY
                , chart4);

        printf( "SET %s = %u\n"
                , dim
                , econn);

        printf("END\n");
    }
}

static void fill_fake_vector(netdata_kern_stats_t *ptr, uint8_t proto) {
    ptr->first = 0;
    ptr->ct = 0;
    ptr->saddr = 0;
    ptr->daddr = 0;
    ptr->dport = port_list[0].port;
    ptr->retransmit = 0;
    ptr->sent = 0;
    ptr->recv = 0;
    ptr->protocol = proto;
    ptr->removeme = 0;
}

static void netdata_create_chart(char *family, char *name, char *msg, char *axis, int proto) {
    char port[8];
    char *dimname;
    printf("CHART %s.%s '' '%s' '%s' 'network' '' line 1000 1 ''\n"
            , family
            , name
            , msg
            , axis);

    netdata_port_list_t *move = port_list;
    struct servent *sname;
    while (move) {
        char *proto_name = (!proto)?protocols[0]:protocols[1];
        sname = getservbyport(move->port, proto_name);
        if (!sname) {
            snprintf(port, 8, "%d", ntohs(move->port) );
            dimname = port;
        } else {
            dimname = sname->s_name;
        }
        printf("DIMENSION %s '' absolute 1 1\n", dimname);

        move = move->next;
    }
}

static void netdata_create_charts() {
    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_CHART1, "Network Viewer TCP bytes received from request to specific port.", "kilobits/s", 0);

    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_CHART2, "Network viewer TCP request length to specific port.", "kilobits/s", 0);

    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_CHART3, "Network viewer UDP bytes received from request to specific port.", "kilobits/s", 1);

    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_CHART4, "Network viewer UDP request length to specific port.", "kilobits/s", 1);

    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_CHART6, "Network viewer TCP active connections per port.", "active connections", 0);

    netdata_create_chart(NETWORK_VIEWER_FAMILY, NETWORK_VIEWER_CHART8, "Network viewer UDP active connections per port.", "active connections", 1);
}


static void netdata_publish_data() {
    if(connection_controller.ports) {
        netdata_port_stats_t *move = connection_controller.ports;
        while (move) {
            write_report(move);

            move = move->next;
        }
    } else {
        netdata_kern_stats_t e;
        if(!fake1) {
            fake1 = calloc(1, sizeof(netdata_port_stats_t));
            if(fake1) {
                fill_fake_vector(&e, 6);
                netdata_set_port_stats(fake1, &e);
            }
        }
        write_report(fake1);

        if(!fake2) {
            fake2 = calloc(1, sizeof(netdata_port_stats_t));
            if(fake2) {
                fill_fake_vector(&e, 17);
                netdata_set_port_stats(fake2, &e);
            }
        }
        write_report(fake2);
    }
}

void *network_viewer_publisher(void *ptr) {
    (void)ptr;
    netdata_create_charts();

    usec_t step = 1* USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!netdata_exit) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;
        netdata_publish_data();

        fflush(stdout);
 //       sleep(1);
    }

    return NULL;
}

// ----------------------------------------------------------------------
void netdata_set_conn_stats(netdata_conn_stats_t *ncs, netdata_kern_stats_t *e) {
    ncs->first = e->first;
    ncs->ct = e->ct;
    ncs->saddr = e->saddr;

    ncs->daddr = e->daddr;

    ncs->dport = e->dport;
    ncs->retransmit = e->retransmit;
    ncs->sent = e->sent;
    ncs->recv = e->recv;

    ncs->protocol = e->protocol;

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
    netdata_conn_stats_t *ncs = calloc(1, sizeof(netdata_conn_stats_t));

    if(ncs) {
        netdata_set_conn_stats(ncs, e);
    }

    return ncs;
}

void netdata_update_port_stats(netdata_port_stats_t *p, netdata_kern_stats_t *e) {
    p->inow += e->recv;
    p->enow += e->sent;

    netdata_conn_stats_t *ret;
    netdata_conn_stats_t search = { .daddr = e->daddr, .saddr = e->saddr, .dport = e->dport };
    netdata_conn_stats_t *ncs = (netdata_conn_stats_t *)avl_search_lock(&p->destination_port, (avl *)&search);
    if (!ncs) {
        ncs = store_new_connection_stat(e);
        if(ncs) {
            netdata_conn_stats_t *ret = (netdata_conn_stats_t *)avl_insert_lock(&p->destination_port, (avl *)ncs);
            if(ret != ncs) {
                error("[NETWORK VIEWER] Cannot insert a new connection to index.");
                free(ncs);
            } else {
                connection_controller.last_connection->next = ncs;
                ncs->prev = connection_controller.last_connection;
                connection_controller.last_connection = ncs;

                p->etot += 1;
            }
        }
    } else {
        netdata_update_conn_stats(ncs, e);
    }

    if(e->removeme) {
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
            ret->next  = ncs->next;
        }

        free(ncs);
    }
}

static int compare_destination_ip(void *a, void *b) {
    netdata_conn_stats_t *conn1 = (netdata_conn_stats_t *)a;
    netdata_conn_stats_t *conn2 = (netdata_conn_stats_t *)b;

    int ret = 0;

    uint32_t ip1 = conn1->daddr;
    uint32_t ip2 = conn2->daddr;

    if (ip1 < ip2) ret = -1;
    else if (ip1 > ip2) ret = 1;

    if (!ret) {
        uint16_t port1 = conn1->dport;
        uint16_t port2 = conn2->dport;

        if (port1 < port2) ret = -1;
        else if (port1 > port2) ret = 1;

        if (!ret) {
            ip1 = conn1->saddr;
            ip2 = conn2->saddr;

            if (ip1 < ip2) ret = -1;
            else if (ip1 > ip2) ret = 1;
        }
    }

    return ret;
}

netdata_port_stats_t *store_new_port_stat(netdata_kern_stats_t *e) {
    netdata_port_stats_t *p = calloc(1, sizeof(netdata_port_stats_t));

    if(p) {
        netdata_set_port_stats(p, e);
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

    netdata_kern_stats_t *e = data;

    if(!e->dport) {
        return -2;//LIBBPF_PERF_EVENT_CONT;
    }

    if(!is_monitored_port(e->dport) ) {
        return -2;//LIBBPF_PERF_EVENT_CONT;
    }

    int inside = is_ip_inside_table(e->saddr, e->daddr);
    if(inside < 0) {
        return -2;//LIBBPF_PERF_EVENT_CONT;
    }

    netdata_port_stats_t *pp;
    netdata_port_stats_t *rp;
    netdata_conn_stats_t *ncs;
    netdata_conn_stats_t *ret;
    if (!connection_controller.ports) {
        pp = store_new_port_stat(e);
        connection_controller.ports = pp;
        connection_controller.last_port = pp;

        rp = (netdata_port_stats_t *)avl_insert_lock(&connection_controller.port_stat, (avl *)pp);
        if(rp != pp) {
            error("[NETWORK VIEWER] Cannot insert a new port stat inside index.");
        }

        ncs = store_new_connection_stat(e);
        if(ncs) {
            connection_controller.last_connection = ncs;
            ncs->prev = NULL;
            connection_controller.tree = ncs;
            ret = (netdata_conn_stats_t *)avl_insert_lock(&pp->destination_port, (avl *)ncs);
            if(ret != ncs) {
                error("[NETWORK VIEWER] Cannot insert a new connection inside index.");
            } else {
                pp->etot += 1;
            }
        }
        return -2;//LIBBPF_PERF_EVENT_CONT;
    }


    netdata_port_stats_t search_proto = { .port = e->dport, .protocol = e->protocol };

    pp = (netdata_port_stats_t *)avl_search_lock(&connection_controller.port_stat, (avl *)&search_proto);
    if (!pp) {
        pp = store_new_port_stat(e);
        if(pp) {
            connection_controller.last_port->next = pp;
            connection_controller.last_port = pp;

            rp = (netdata_port_stats_t *)avl_insert_lock(&connection_controller.port_stat, (avl *)pp);
            if(rp != pp) {
                error("[NETWORK VIEWER] Cannot insert a new port stat inside index.");
            }

            ncs = store_new_connection_stat(e);
            if(ncs) {
                connection_controller.last_connection->next = ncs;
                ncs->prev = connection_controller.last_connection;
                connection_controller.last_connection = ncs;
                ret = (netdata_conn_stats_t *)avl_insert_lock(&pp->destination_port, (avl *)ncs);
                if(ret != ncs) {
                    error("[NETWORK VIEWER] Cannot insert a new connection inside index.");
                } else {
                    pp->etot += 1;
                }
            }
        }
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

void *network_collector(void *ptr) {
    (void)ptr;

    avl_init_lock(&(connection_controller.port_stat), compare_port);

    int nprocs = sysconf(_SC_NPROCESSORS_ONLN);
    int netdata_exit = 0;

    netdata_perf_loop_multi(pmu_fd, headers, nprocs, &netdata_exit, netdata_store_bpf);

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
    netdata_port_stats_t *ncs = (netdata_port_stats_t *)avl_search_lock(&connection_controller.port_stat, (avl *)r);
    if (ncs) {
        ncs = (netdata_port_stats_t *)avl_remove_lock(&connection_controller.port_stat, (avl *)r);
        if (ncs != r) {
            error("[NETWORK VIEWER] Cannot remove a port");
        }
    }
}

void clean_ports() {
    netdata_port_stats_t *move = connection_controller.ports->next;
    while (move) {
        netdata_port_stats_t *next = move->next;
        clean_port_index(move);

        free(move->dimension);

        free(move);
        move = next;
    }
    free(connection_controller.ports);
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
    (void)sig;
    if(libnetdatanv) {
        dlclose(libnetdatanv);
    }

    if(connection_controller.tree) {
        clean_connections();
    }

    if(connection_controller.ports) {
        clean_ports();
    }

    if(outgoing_table || ingoing_table) {
        clean_networks();
    }

    if(port_list) {
        clean_list_ports();
    }

    if(fake1) {
        free(fake1->dimension);
        free(fake1);
    }

    if(fake2) {
        free(fake2->dimension);
        free(fake2);
    }

    exit(0);
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

    build_complete_path(lpath, 4096, "libnetdata_network_viewer.so");
    libnetdatanv = dlopen(lpath ,RTLD_LAZY);
    if (!libnetdatanv) {
        error("[NETWORK VIEWER] Cannot load ./libnetdata_network_viewer.so.");
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

netdata_port_list_t *netdata_list_ports(char *ports) {
    netdata_port_list_t *ret = NULL, *next;
    netdata_port_list_t *set;
    uint16_t port;

    if (!ports) {
        int i = 0;
        for (i = 0; i < 50; i++) {
            uint16_t def[] = {  20, 21, 25, 37, 43, 53, 80, 88, 110, 118,
                                123, 135, 137, 138, 139, 143, 156, 194, 389, 443,
                                445, 464, 465, 513, 520, 530, 546, 547, 563, 631,
                                636, 691, 749, 901, 989, 990, 993, 995, 1381, 1433,
                                1434, 1512, 1525, 3128, 3389, 3306, 5432, 6000, 8080, 19999 } ;
            set = (netdata_port_list_t *) callocz(1,sizeof(netdata_port_list_t));
            if(set) {
                port = htons(def[i]);
                set->port = port;

                netdata_port_list_t *r = (netdata_port_list_t *)avl_insert_lock(&connection_controller.port_list, (avl *)set);
                if (r != set ) {
                    error("[NETWORK VIEWER] Cannot insert port inside list.");
                }

                if(!ret) {
                    ret =  set;
                } else {
                    next->next = set;
                }

                next = set;
            }
        }
    } else {
        while (*ports) {
            char *vport = ports;
            while (*ports && *ports != ',' && *ports != ' ') ports++;

            if(*ports) {
                *ports = 0x00;
                ports++;
            }

            vport = trim_all(vport);

            if(vport) {
                set = (netdata_port_list_t *) callocz(1,sizeof(netdata_port_list_t));
                if(set) {
                    port = htons((uint16_t)strtol(vport,NULL,10));
                    set->port = port;

                    netdata_port_list_t *r = (netdata_port_list_t *)avl_insert_lock(&connection_controller.port_list, (avl *)set);
                    if (r != set ) {
                        error("[NETWORK VIEWER] Cannot insert port inside list.");
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
        if(!s || *s == '#') continue;

        char *key = s;
        while(*s && *s != '=') s++;

        if (!*s) {
            //print an error
            continue;
        }

        *s = '\0';
        s++;

        char *value = s;
        key = trim_all(key);
        value = trim_all(value);

        if (!strcasecmp(key,"outgoing")) {
            outgoing_table = netdata_list_ips(value,1);
        } else if (!strcasecmp(key,"destination_ports")) {
            port_list = netdata_list_ports(value);
        } else  if (!strcasecmp(key,"ingoing")) {
            ingoing_table = netdata_list_ips(value, 0);
        }
    }

    fclose(fp);
    return 0;
}

void parse_config() {
    user_config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    stock_config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
    plugin_dir = getenv("NETDATA_PLUGINS_DIR");

    memset(&connection_controller,0,sizeof(connection_controller));
    avl_init_lock(&connection_controller.port_list, compare_port_value);

    if (!user_config_dir && !stock_config_dir) {
        return;
    }

    int read_stock = 0;
    if (user_config_dir) {
        read_stock =  read_config_file(user_config_dir);
    }

    if (read_stock && stock_config_dir) {
        //PUT INFO HERE
        read_stock =  read_config_file(stock_config_dir);
    }

    if (read_stock) {
        //REPORT ERROR FOR ALL HERE
    }
}

int main(int argc, char **argv) {
    (void)argc;
    (void)argv;
    //program_name = "networkviewer.plugin";

    struct rlimit r = {RLIM_INFINITY, RLIM_INFINITY};
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        error("[NETWORK VIEWER] setrlimit(RLIMIT_MEMLOCK)");
        return 1;
    }

    parse_config();

    if (network_viewer_load_libraries()) {
        return 2;
    }

    signal(SIGINT, int_exit);
    signal(SIGTERM, int_exit);

    if(network_viewer_load_ebpf()) {
        error("[NETWORK VIEWER] Cannot load eBPF program.");
        return 3;
    }

    if (map_memory()) {
        return 4;
    }

    if(!outgoing_table) {
        outgoing_table = netdata_list_ips(NULL, 1);
        if(!outgoing_table) {
            error("[NETWORK VIEWER] Cannot load outgoing network range to monitor.");
            return 5;
        }
    }

    if(!ingoing_table) {
        ingoing_table = netdata_list_ips(NULL, 0);
        if(!ingoing_table) {
            error("[NETWORK VIEWER] Cannot load ingoing network range to monitor.");
            return 6;
        }
    }

    if(!port_list) {
        port_list = netdata_list_ports(NULL);
        if(!port_list) {
            error("[NETWORK VIEWER] Cannot load network ports to monitor.");
            return 7;
        }
    }

    pthread_attr_t attr;
    pthread_attr_init(&attr);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_JOINABLE);
    pthread_t thread[2];

    int i;
    int end = 2;
    for ( i = 0; i < end ; i++ ) {
        if ( ( pthread_create(&thread[i], &attr, (!i)?network_viewer_publisher:network_collector, NULL) ) ) {
            error("[NETWORK VIEWER] Cannot create the necessaries threads.");
            return 8;
        }
    }

    for ( i = 0; i < end ; i++ ) {
        if ( (pthread_join(thread[i], NULL) ) ) {
            error("[NETWORK VIEWER] Cannot join the necessaries threads.");
            return 9;
        }
    }

    return 0;
}
