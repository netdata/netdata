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

netdata_network_t *nn = NULL;

netdata_control_connection_t ncc;

/*
 */
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

int netdata_is_inside(in_addr_t val, uint32_t *router) {
    netdata_network_t *lnn = nn;
    while (lnn) {
        if (!lnn->isloopback) {
            in_addr_t ip = lnn->ipv4addr;
            in_addr_t mask = lnn->netmask;

            if ((ip & mask) == (val & mask)) {
                if(router) {
                    *router = lnn->router;
                }
                return 1;
            }
        }

        lnn = lnn->next;
    }

    return 0;
}


// ----------------------------------------------------------------------
static void netdata_publish_data() {
    static int not_initialized = 0;
    uint32_t egress = 0;
    uint32_t ingress = 0;

#define NETWORK_VIEWER_FAMILY  "network_viewer"
#define NETWORK_VIEWER_CHART  "connections"
#define NETWORK_VIEWER_INGRESS "ingress"
#define NETWORK_VIEWER_EGRESS  "egress"

    if(ncc.tree) {
        netdata_conn_stats_t *move = ncc.tree;
        while (move) {
            in_addr_t arg = move->saddr;
            //COMPARE TIMESTAMP TO DEFINE THE CORRECT COUNTER
            //USE clock_gettime WITH CLOCK_MONOTONIC
            if (netdata_is_inside(arg, NULL) ) {
                egress++;
            } else {
                ingress++;
            }
            move = move->next;
        }
    }

    // ------------------------------------------------------------------------
    if(!not_initialized) {
        printf("CHART %s.%s '' '%s' 'kilobits/s' 'network' '' line 1000 1 ''\n"
                ,NETWORK_VIEWER_FAMILY
                ,NETWORK_VIEWER_CHART
                ,"Network viewer ingress traffic."
        );
        printf("DIMENSION ingress '' absolute 1 1\n");
        printf("DIMENSION egress '' absolute 1 1\n");

        not_initialized++;
    }

    printf(
            "BEGIN %s.%s\n"
            , NETWORK_VIEWER_FAMILY
            , NETWORK_VIEWER_INGRESS
    );

    printf(
            "SET %s = %u\n"
            , "ingress"
            , ingress
    );

    printf("END\n");

    printf(
            "BEGIN %s.%s\n"
            , NETWORK_VIEWER_FAMILY
            , NETWORK_VIEWER_EGRESS
    );

    printf(
            "SET %s = %u\n"
            , "egress"
            , egress
    );

    printf("END\n");
}

void *network_viewer_publisher(void *ptr)
{
    (void)ptr;
    while(1) {
        sleep(1);

        netdata_publish_data();

        fflush(stdout);
    }

    return NULL;
}

// ----------------------------------------------------------------------
void netdata_set_conn_stats(netdata_conn_stats_t *ncs, netdata_kern_stats_t *e) {
    uint32_t ip;
    uint8_t proto;

    ncs->first = e->first;
    ncs->ct = e->ct;
    ncs->saddr = e->saddr;

    ip = e->daddr;
    in_addr_t daddr = ip;

    ncs->daddr = ntohl(ip);
    ncs->internal = (netdata_is_inside(daddr, NULL))?1:0;

    ncs->dport = ntohs(e->dport);
    ncs->retransmit = e->retransmit;
    ncs->sent = e->sent;
    ncs->recv = e->recv;

    proto = e->protocol;
    ncs->protocol = proto;
    ncs->removeme = (proto == 253)?1:0;

    ncs->next = NULL;
}

void netdata_update_conn_stats(netdata_conn_stats_t *ncs, netdata_kern_stats_t *e) {
    ncs->ct = e->ct;

    ncs->retransmit = e->retransmit;
    ncs->sent = e->sent;
    ncs->recv = e->recv;

    ncs->removeme = (e->protocol == 253)?1:0;
}

int netdata_store_bpf(void *data, int size)
{
    netdata_kern_stats_t *e = data;
    /*
    char saddr[32], daddr[32];

    convert_addr(saddr, 32, e->saddr);
    convert_addr(daddr, 32, e->daddr);

    uint8_t proto = e->protocol;
     */

    //   printf("%-17s %-17s %-5u %-17u %-10u %-10u %-10u %-14u\n", saddr, daddr, ntohs(e->dport), e->retransmit, e->sent, e->recv, proto, (e->ct - e->first));

    netdata_conn_stats_t *ncs = (netdata_conn_stats_t *)avl_search_lock(&ncc.destination_port, (avl *)e);
    if (!ncs) {
        ncs = malloc(sizeof(netdata_conn_stats_t));
        if(ncs) {

            netdata_set_conn_stats(ncs, e);
            if(!ncc.tree) {
                ncc.tree = ncs;
            } else {
                netdata_conn_stats_t *move, *save;
                for (move = ncc.tree; move ; save = move, move = move->next);
                if(save) {
                    save->next = ncs;
                }
            }

            netdata_conn_stats_t *ret = (netdata_conn_stats_t *)avl_insert_lock(&ncc.destination_port, (avl *)ncs);
            if(ret == ncs) {
            }
        }
    } else {
        netdata_update_conn_stats(ncs, e);
    }

    return -2;
}

int compare_destination_ip(void *a, void *b) {
    netdata_conn_stats_t *conn1 = (netdata_conn_stats_t *)a;
    netdata_conn_stats_t *conn2 = (netdata_conn_stats_t *)b;

    if(conn1->daddr < conn2->daddr){
        if(conn1->dport < conn2->dport) {
            return -1;
        }

        if(conn1->dport > conn2->dport) {
            return 1;
        }
    }

    if(conn1->daddr > conn2->daddr){
        if(conn1->dport < conn2->dport) {
            return -1;
        }

        if(conn1->dport > conn2->dport) {
            return 1;
        }
    }

    return 0;
}

void *network_collector(void *ptr) {
    (void)ptr;
    ncc.tree = NULL;

    avl_init_lock(&(ncc.destination_port), compare_destination_ip);

    int nprocs = sysconf(_SC_NPROCESSORS_ONLN);
    netdata_perf_loop_multi(pmu_fd, headers, nprocs, (int *)&netdata_exit, netdata_store_bpf);

    return NULL;
}
// ----------------------------------------------------------------------
static void clean_networks() {
    netdata_network_t *move = nn->next;
    while (move) {
        netdata_network_t *next = move->next;
        free(move);
        move = next;
    }
    free(nn);
}

void clean_connections() {
    netdata_conn_stats_t * move = ncc.tree->next;
    while (move) {
        netdata_conn_stats_t *next = move->next;
        netdata_conn_stats_t *ncs = (netdata_conn_stats_t *)avl_remove_lock(&ncc.destination_port, (avl *)move);
        if (ncs != move) { //REVIEW THIS
            //error("[NETWORK VIEWER] Cannot remove a connection");
        }

        free(move);
        move = next;
    }
    free(ncc.tree);
}

static void int_exit(int sig) {
    (void)sig;
    if(libnetdatanv) {
        dlclose(libnetdatanv);
    }

    if(ncc.tree) {
        clean_connections();
    }

    if(nn) {
        clean_networks();
    }

    exit(0);
}


int network_viewer_load_libraries()
{
    char *err = NULL;
    libnetdatanv = dlopen("./libnetdata_network_viewer.so",RTLD_LAZY);
    if (!libnetdatanv) {
        error(err);
        return -1;
    } else {
        load_bpf_file = dlsym(libnetdatanv, "load_bpf_file");
        if ((err = dlerror()) != NULL) {
            error(err);
            return -1;
        }

        test_bpf_perf_event = dlsym(libnetdatanv, "test_bpf_perf_event");
        if ((err = dlerror()) != NULL) {
            error(err);
            return -1;
        }

        netdata_perf_loop_multi = dlsym(libnetdatanv, "my_perf_loop_multi");
        if ((err = dlerror()) != NULL) {
            error(err);
            return -1;
        }

        perf_event_mmap =  dlsym(libnetdatanv, "perf_event_mmap");
        if ((err = dlerror()) != NULL) {
            error(err);
            return -1;
        }

        perf_event_mmap_header =  dlsym(libnetdatanv, "perf_event_mmap_header");
        if ((err = dlerror()) != NULL) {
            error(err);
            return -1;
        }
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
            return -1;
        }
    }

    for ( i = 0 ; i < nprocs ; i++ ) {
        if (perf_event_mmap_header(pmu_fd[i], &headers[i]) < 0) {
            return -1;
        }
    }


    return 0;
}

netdata_network_t *netdata_list_ips() {
    struct ifaddrs *i, *ip;
    netdata_network_t *ret, *set, *move;
    struct sockaddr_in *sa;

    if (getifaddrs(&ip)) {
        return NULL;
    }

    ret = NULL;
    move = NULL;
    for (i = ip; i; i = i->ifa_next) {
        if (i->ifa_addr && i->ifa_addr->sa_family==AF_INET) {
            set = (netdata_network_t *) calloc(1,sizeof(*ret));
            if(set) {
                sa = (struct sockaddr_in *) i->ifa_addr;
                set->ipv4addr = sa->sin_addr.s_addr;

                sa = (struct sockaddr_in *) i->ifa_netmask;
                set->netmask = sa->sin_addr.s_addr;

                set->isloopback = (!strcmp(i->ifa_name,"lo"))?1:0;

                if(!ret) {
                    ret =  set;
                    move = set;
                } else {
                    move->next = set;
                    move = set;
                }
            }
        }
    }

    freeifaddrs(ip);

    return ret;
}

void netdata_set_router(netdata_network_t *lnn, in_addr_t val) {
    while (lnn) {
        in_addr_t ip = lnn->ipv4addr;
        in_addr_t mask = lnn->netmask;

        if ((ip & mask) == (val & mask )) {
            lnn->router = val;
        }
        lnn = lnn->next;
    }
}

int netdata_get_router(netdata_network_t *lnn) {
    FILE *fp = fopen("/proc/net/route", "r");

    if(!fp) {
        return -1;
    }

    char iface[IF_NAMESIZE];
    char buf[512];
    uint32_t destination, gateway;
    while (fgets(buf, sizeof(buf), fp)) {
        if (sscanf(buf, "%s %x %x", iface, &destination, &gateway) == 3) {
            if(!destination) {
                in_addr_t router = (in_addr_t)gateway;
                netdata_set_router(lnn, router);
            }
        }
    }

    fclose(fp);
    return 0;
}

int main(int argc, char **argv) {
    (void)argc;
    (void)argv;
    //program_name = "networkviewer.plugin";

    struct rlimit r = {RLIM_INFINITY, RLIM_INFINITY};
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        error("setrlimit(RLIMIT_MEMLOCK)");
        return 1;
    }

    if(network_viewer_load_libraries()) {
        return 2;
    }

    signal(SIGINT, int_exit);
    signal(SIGTERM, int_exit);

    if (load_bpf_file("netdata_ebpf_network_viewer.o") )
    {
        return 3;
    }

    if (map_memory()) {
        return 4;
    }

    nn = netdata_list_ips();
    if(!nn) {
        return 5;
    }

    if (netdata_get_router(nn)) {
        return 6;
    }

    pthread_attr_t attr;
    pthread_attr_init(&attr);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_JOINABLE);
    pthread_t thread[2];

    int i;
    int end = 2;
    for ( i = 0; i < end ; i++ ) {
        if ( ( pthread_create(&thread[i], &attr, (!i)?network_viewer_publisher:network_collector, NULL) ) ) {
            perror("");
            return 7;
        }

    }

    for ( i = 0; i < end ; i++ ) {
        if ( (pthread_join(thread[i], NULL) ) )
        {
            perror("");
            return 7;
        }
    }

    return 0;
}
