// SPDX-License-Identifier: GPL-3.0-or-later

#include <daemon/main.h>
#include "../../libnetdata/libnetdata.h"

#include "network_viewer_plugin.h"

//Pointer to the libraries and functions
void *libnetdatanv = NULL;
int (*load_bpf_file)(char *) = NULL;
int (*test_bpf_perf_event)(int) = NULL;
int (*perf_event_mmap)(int) = NULL;
int (*perf_event_mmap_header)(int, struct perf_event_mmap_page **) = NULL;
void (*netdata_perf_loop_multi)(int *, struct perf_event_mmap_page **, int, int *, int (*netdata_ebpf_callback)(void *, int));

//Interval used to deliver the data
static int update_every = 1;
static int freq = 0;

//perf variables
int pmu_fd[NETDATA_MAX_PROCESSOR];
static struct perf_event_mmap_page *headers[NETDATA_MAX_PROCESSOR];

netdata_network_t *nn;

netdata_control_connection_t ncc;

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

void clean_networks() {
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
        if (ncs != move) {
            error("[NETWORK VIEWER] Cannot remove a connection");
        }

        free(move);
        move = next;
    }
    free(ncc.tree);
}

static void network_viewer_exit(int sig) {
    (void)sig;
    if (libnetdatanv) {
        dlclose(libnetdatanv);
    }

    if(nn) {
        clean_networks();
    }

    if(ncc.tree) {
        clean_connections();
    }

    exit(0);
}

int map_memory() {
    size_t nprocs = sysconf(_SC_NPROCESSORS_ONLN);
    size_t i;

    if (nprocs < NETDATA_MAX_PROCESSOR) {
        nprocs = NETDATA_MAX_PROCESSOR;
    }

    for (i = 0; i < nprocs; i++) {
        pmu_fd[i] = test_bpf_perf_event(i);

        if (perf_event_mmap(pmu_fd[i]) < 0) {
            return -1;
        }
    }

    for (i = 0; i < nprocs; i++) {
        if (perf_event_mmap_header(pmu_fd[i], &headers[i]) < 0) {
            return -1;
        }
    }

    return 0;
}

static int network_viewer_load_libraries() {
    char *error = NULL;

    libnetdatanv = dlopen("./libnetdata_network_viewer.so",RTLD_LAZY);
    if (!libnetdatanv) {
        error("[NETWORK VIEWER] : Cannot load the library libnetdata_network_viewer.so");
        return -1;
    } else {
        load_bpf_file = dlsym(libnetdatanv, "load_bpf_file");
        error = dlerror();
        if (error != NULL) {
            goto comm_load_err;
        }

        test_bpf_perf_event = dlsym(libnetdatanv, "test_bpf_perf_event");
        error = dlerror();
        if (error != NULL) {
            goto comm_load_err;
        }

        netdata_perf_loop_multi = dlsym(libnetdatanv, "my_perf_loop_multi");
        error = dlerror();
        if (error != NULL) {
            goto comm_load_err;
        }

        perf_event_mmap =  dlsym(libnetdatanv, "perf_event_mmap");
        error = dlerror();
        if (error != NULL) {
            goto comm_load_err;
        }

        perf_event_mmap_header =  dlsym(libnetdatanv, "perf_event_mmap_header");
        error = dlerror();
        if (error != NULL) {
            goto comm_load_err;
        }
    }

    return 0;

comm_load_err:
    error("[NETWORK VIEWER] : %s", error);
    return -1;
}

// Start of collector group

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

/*
uint32_t netdata_select_router(in_addr_t val) {
    uint32_t router = 0;
    netdata_is_inside(val, &router);

    return (router)?router:val.s_addr;
}

void netdata_convert_addr(char *output, size_t len, uint32_t src) {
    struct in_addr laddr = { .s_addr = src} ;
    char *val = inet_ntoa(laddr);
    snprintf(output, len, "%s", val);
}
 */

void netdata_set_conn_stats(netdata_conn_stats_t *ncs, netdata_kern_stats_t *e) {
    uint32_t ip;
    uint8_t proto;

    ncs->first = e->first;
    ncs->ct = e->ct;
    ncs->saddr = e->saddr;
 //   in_addr_t saddr = e->saddr ;

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

static int netdata_store_bpf(void *data, int size)
{
    (void)size;
    netdata_kern_stats_t *e = data;

    netdata_conn_stats_t *ncs = (netdata_conn_stats_t *)avl_search_lock(&ncc.destination_port, (avl *)e);
    if (!ncs) {
        ncs = mallocz(sizeof(netdata_conn_stats_t));

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
    } else {
        netdata_update_conn_stats(ncs, e);
    }

    //return LIBBPF_PERF_EVENT_CONT;
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

void *network_viewer_collector(void *ptr) {
    (void)ptr;

    ncc.tree = NULL;

    //avl_init_lock(&(ncc.bytessent), alarm_compare_name);
    avl_init_lock(&(ncc.destination_port), compare_destination_ip);

    size_t nprocs = sysconf(_SC_NPROCESSORS_ONLN);
    netdata_perf_loop_multi(pmu_fd, headers, nprocs, (int *)&netdata_exit, netdata_store_bpf);

    return NULL;
}

// End of collector group

// Start of publisher group
static void netdata_send_data() {
    static int not_initialized = 0;
    static collected_number bilocal = 0, binet = 0;
    static collected_number belocal = 0, benet = 0;

    collected_number nelocal=0, nenet = 0;
    collected_number nilocal=0, ninet = 0, value;

#define NETWORK_VIEWER_FAMILY "network_viewer"
#define NETWORK_VIEWER_INGRESS "ingress"
#define NETWORK_VIEWER_EGRESS "egress"

    if(ncc.tree) {
        netdata_conn_stats_t *first = ncc.tree;
        netdata_conn_stats_t *move = ncc.tree->next;
        netdata_conn_stats_t *prev = NULL;
        while (move) {
            netdata_conn_stats_t *next = move->next;
            if(next->protocol == 253) {
                prev = move;
            }

            if (move->internal) {
                nelocal += (collected_number )move->sent;
                nilocal += (collected_number )move->recv;
            } else {
                nenet += (collected_number )move->sent;
                ninet += (collected_number )move->recv;
            }

            if (move->protocol == 253) {
                if (first == move) {
                    first = move->next;
                } else {
                    prev->next = move->next;
                    netdata_conn_stats_t *ret = (netdata_conn_stats_t *)avl_remove_lock(&ncc.destination_port, (avl *)move);
                    if(ret) {
                    }
                    freez(move);
                }
            }
            move = next;
        }
    }

    // --------------------------------------------------------------------------------
    if(!not_initialized) {
        printf("CHART %s.%s '' '%s' 'kilobits/s' 'network' '' line 1000 1 ''\n"
              ,NETWORK_VIEWER_FAMILY
              ,NETWORK_VIEWER_INGRESS
              ,"Network viewer ingress traffic."
        );
        printf("DIMENSION local '' absolute 1 1\n");
        printf("DIMENSION web '' absolute 1 1\n");
    }

    printf(
            "BEGIN %s.%s\n"
            , NETWORK_VIEWER_FAMILY
            , NETWORK_VIEWER_INGRESS
    );

    value = nilocal - bilocal;
    printf(
            "SET %s = "COLLECTED_NUMBER_FORMAT"\n"
            , "local"
            , (collected_number) value
    );
    bilocal = nilocal;

    value = ninet - binet;
    printf(
            "SET %s = "COLLECTED_NUMBER_FORMAT"\n"
            , "web"
            , (collected_number) value
    );
    binet = ninet;

    printf("END\n");

    // --------------------------------------------------------------------------------
    if(!not_initialized) {
        printf("CHART %s.%s '' '%s' 'kilobits/s' 'network' '' line 1000 1 ''\n"
                ,NETWORK_VIEWER_FAMILY
                ,NETWORK_VIEWER_EGRESS
                ,"Network viewer egress traffic."
        );

        printf("DIMENSION local '' absolute 1 1\n");
        printf("DIMENSION web '' absolute 1 1\n");
    }

    printf(
            "BEGIN %s.%s\n"
            , NETWORK_VIEWER_FAMILY
            , NETWORK_VIEWER_EGRESS
    );

    value = nelocal - belocal;
    printf(
            "SET %s = "COLLECTED_NUMBER_FORMAT"\n"
            , "local"
            , (collected_number) value
    );
    belocal = nelocal;

    value = nenet - benet;
    printf(
            "SET %s = "COLLECTED_NUMBER_FORMAT"\n"
            , "web"
            , (collected_number) value
    );
    benet = nenet;

    printf("END\n");
}

void *network_viewer_publisher(void *ptr) {
    (void)ptr;

    heartbeat_t hb;
    heartbeat_init(&hb);
    usec_t step = update_every * USEC_PER_SEC;
    for (;;) {
        usec_t dt = heartbeat_next(&hb, step);
        (void)dt;

        if(unlikely(netdata_exit))
            break;

        netdata_send_data();

        fflush(stdout);
    }

    return NULL;
}

// End of publisher group

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
            set = (netdata_network_t *) callocz(1,sizeof(*ret));
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

void netdata_set_router(netdata_network_t *lnn, in_addr_t val)
{
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
    int sock;
    char buffer[4096];
    char *msg;

    sock = socket(AF_NETLINK, SOCK_RAW, NETLINK_ROUTE);
    if (sock) {
        return -1;
    }

    struct timeval tv ={ .tv_sec = 1 };
    setsockopt(sock, SOL_SOCKET, SO_RCVTIMEO, (struct timeval *)&tv, sizeof(struct timeval));

    struct  nlmsghdr *nlh, nhm;

    uint32_t mypid = getpid();
    nhm.nlmsg_len = NLMSG_LENGTH(sizeof(struct rtmsg));
    nhm.nlmsg_type = RTM_GETROUTE;
    nhm.nlmsg_flags = NLM_F_DUMP | NLM_F_REQUEST;
    nhm.nlmsg_seq = 1;
    nhm.nlmsg_pid = mypid;

    ssize_t bytes = send(sock, &nhm, nhm.nlmsg_len, 0);
    if (bytes < 0 ) {
        msg = "send callback";
        goto end_err;
    }

    char *move = buffer;
    ssize_t length = 0;

    do {
        bytes =  recv(sock, move, sizeof(buffer) - length, 0);
        if (bytes < 0) {
            msg = "recv callback";
            goto end_err;
        }

        nlh = (struct nlmsghdr *)move;
        if((NLMSG_OK(&nhm, bytes) == 0) || (nhm.nlmsg_type == NLMSG_ERROR)) {
            msg = "Error to receive packet";
            goto end_err;
        }

        if (nlh->nlmsg_type == NLMSG_DONE) {
            break;
        } else {
            move += bytes;
            length += bytes;
        }

        if ((nhm.nlmsg_flags & NLM_F_MULTI) == 0) {
            break;
        }
    } while ((nhm.nlmsg_seq != 1) || (nhm.nlmsg_pid != mypid));

    for ( ; NLMSG_OK(nlh, bytes); nlh = NLMSG_NEXT(nlh, bytes)) {
        struct  rtmsg *r = (struct rtmsg *) NLMSG_DATA(nlh);
        if (r->rtm_table != RT_TABLE_MAIN) {
            break;
        }

        struct rtattr *ra = (struct rtattr *) RTM_RTA(r);
        size_t payload_length = RTM_PAYLOAD(nlh);

        for ( ; RTA_OK(ra, payload_length); ra = RTA_NEXT(ra, payload_length)) {
            if (ra->rta_type == RTA_GATEWAY) {
                char discard[128];
                in_addr_t router = inet_addr(inet_ntop(AF_INET, RTA_DATA(ra), discard, sizeof(discard)));
                netdata_set_router(lnn, router);
            }
        }
    }

    close(sock);
    return 0;
end_err:
    perror(msg);
    close(sock);
    return -1;
}

int main(int argc, char **argv) {
    (void)argc;
    (void)argv;
    program_name = "networkviewer.plugin";

    //Threads used in this plugin
    struct netdata_static_thread static_threads_network_viewer[] = {
            {"COLLECTOR",             NULL,                    NULL,         1, NULL, NULL, network_viewer_collector},
            {"PUBLISHER",             NULL,                    NULL,         1, NULL, NULL, network_viewer_publisher},
            {NULL,                   NULL,                    NULL,         0, NULL, NULL, NULL}
    };

    //parse input here

    if (freq >= update_every)
        update_every = freq;

    //We are adjusting the limit, because we are not creating limits
    //to the number of connections we are monitoring.
    struct rlimit r = {RLIM_INFINITY, RLIM_INFINITY};
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        error("[NETWORK VIEWER]: %s", strerror(errno));
        return 2;
    }

    if (network_viewer_load_libraries()) {
        return 1;
    }

    signal(SIGTERM, network_viewer_exit);
    signal(SIGINT, network_viewer_exit);

    if (load_bpf_file("netdata_ebpf_network_viewer.o")) {
        error("[NETWORK VIEWER]: Cannot load the eBPF program.");
        return 3;
    }

    if (map_memory()) {
        error("[NETWORK VIEWER]: Cannot allocate the necessary vectors");
        network_viewer_exit(SIGTERM);
    }

    nn = netdata_list_ips();

    if (netdata_get_router(nn)) {
        return 4;
    }

    error_log_syslog = 0;

    int i;
    struct netdata_static_thread *st;
    for (i = 0; static_threads_network_viewer[i].name != NULL ; i++) {
        st = &static_threads_network_viewer[i];
        netdata_thread_create(st->thread, st->name, NETDATA_THREAD_OPTION_DEFAULT, st->start_routine, st);
    }

    for (i = 0; static_threads_network_viewer[i].name != NULL ; i++) {
        st = &static_threads_network_viewer[i];
        netdata_thread_join(*st->thread, NULL);
    }

    network_viewer_exit(SIGTERM);
}
