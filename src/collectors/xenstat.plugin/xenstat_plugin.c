// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

#include <xenstat.h>
#include <libxl.h>

#define PLUGIN_XENSTAT_NAME "xenstat.plugin"

#define NETDATA_CHART_PRIO_XENSTAT_NODE_CPUS              30001
#define NETDATA_CHART_PRIO_XENSTAT_NODE_CPU_FREQ          30002
#define NETDATA_CHART_PRIO_XENSTAT_NODE_MEM               30003
#define NETDATA_CHART_PRIO_XENSTAT_NODE_TMEM              30004
#define NETDATA_CHART_PRIO_XENSTAT_NODE_DOMAINS           30005

#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_STATES          30101
#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_CPU             30102
#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_VCPU            30103
#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_MEM             30104

#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_TMEM_PAGES      30104
#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_TMEM_OPERATIONS 30105

#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_VBD_OO_REQ      30200
#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_VBD_REQUESTS    30300
#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_VBD_SECTORS     30400

#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_NET_BYTES       30500
#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_NET_PACKETS     30600
#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_NET_ERRORS      30700
#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_NET_DROPS       30800

#define TYPE_LENGTH_MAX 200

#define CHART_IS_OBSOLETE     1
#define CHART_IS_NOT_OBSOLETE 0

// Variables
static int debug = 0;
static int netdata_update_every = 1;

struct vcpu_metrics {
    unsigned int id;

    unsigned int online;
    unsigned long long ns;

    int chart_generated;
    int updated;

    struct vcpu_metrics *next;
};

struct vbd_metrics {
    unsigned int id;

    unsigned int error;
    unsigned long long oo_reqs;
    unsigned long long rd_reqs;
    unsigned long long wr_reqs;
    unsigned long long rd_sects;
    unsigned long long wr_sects;

    int oo_req_chart_generated;
    int requests_chart_generated;
    int sectors_chart_generated;
    int updated;

    struct vbd_metrics *next;
};

struct network_metrics {
    unsigned int id;

    unsigned long long rbytes;
    unsigned long long rpackets;
    unsigned long long rerrs;
    unsigned long long rdrops;

    unsigned long long tbytes;
    unsigned long long tpackets;
    unsigned long long terrs;
    unsigned long long tdrops;

    int bytes_chart_generated;
    int packets_chart_generated;
    int errors_chart_generated;
    int drops_chart_generated;
    int updated;

    struct network_metrics *next;
};

struct domain_metrics {
    char *uuid;
    uint32_t hash;

    unsigned int id;
    char *name;

    // states
    unsigned int running;
    unsigned int blocked;
    unsigned int paused;
    unsigned int shutdown;
    unsigned int crashed;
    unsigned int dying;
    unsigned int cur_vcpus;

    unsigned long long cpu_ns;
    unsigned long long cur_mem;
    unsigned long long max_mem;

    struct vcpu_metrics *vcpu_root;
    struct vbd_metrics *vbd_root;
    struct network_metrics *network_root;

    int states_chart_generated;
    int cpu_chart_generated;
    int vcpu_chart_generated;
    int num_vcpus_changed;
    int mem_chart_generated;
    int updated;

    struct domain_metrics *next;
};

struct node_metrics{
    unsigned long long tot_mem;
    unsigned long long free_mem;
    int num_domains;
    unsigned int num_cpus;
    unsigned long long node_cpu_hz;

    struct domain_metrics *domain_root;
};

static struct node_metrics node_metrics = {
        .domain_root = NULL
};

static inline struct domain_metrics *domain_metrics_get(const char *uuid, uint32_t hash) {
    struct domain_metrics *d = NULL, *last = NULL;
    for(d = node_metrics.domain_root; d ; last = d, d = d->next) {
        if(unlikely(d->hash == hash && !strcmp(d->uuid, uuid)))
            return d;
    }

    if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: allocating memory for domain with uuid %s\n", uuid);

    d = callocz(1, sizeof(struct domain_metrics));
    d->uuid = strdupz(uuid);
    d->hash = hash;

    if(unlikely(!last)) {
        d->next = node_metrics.domain_root;
        node_metrics.domain_root = d;
    }
    else {
        d->next = last->next;
        last->next = d;
    }

    return d;
}

static struct domain_metrics *domain_metrics_free(struct domain_metrics *d) {
    struct domain_metrics *cur = NULL, *last = NULL;
    struct vcpu_metrics *vcpu, *vcpu_f;
    struct vbd_metrics *vbd, *vbd_f;
    struct network_metrics *network, *network_f;

    if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: freeing memory for domain '%s' id %u, uuid %s\n", d->name, d->id, d->uuid);

    for(cur = node_metrics.domain_root; cur ; last = cur, cur = cur->next) {
        if(unlikely(cur->hash == d->hash && !strcmp(cur->uuid, d->uuid))) break;
    }

    if(unlikely(!cur)) {
        netdata_log_error("XENSTAT: failed to free domain metrics.");
        return NULL;
    }

    if(likely(last))
        last->next = cur->next;
    else
        node_metrics.domain_root = NULL;

    freez(cur->uuid);
    freez(cur->name);

    vcpu = cur->vcpu_root;
    while(vcpu) {
        vcpu_f = vcpu;
        vcpu = vcpu->next;
        freez(vcpu_f);
    }

    vbd = cur->vbd_root;
    while(vbd) {
        vbd_f = vbd;
        vbd = vbd->next;
        freez(vbd_f);
    }

    network = cur->network_root;
    while(network) {
        network_f = network;
        network = network->next;
        freez(network_f);
    }

    freez(cur);

    return last ? last : NULL;
}

static int vcpu_metrics_collect(struct domain_metrics *d, xenstat_domain *domain) {
    unsigned int num_vcpus = 0;
    xenstat_vcpu *vcpu = NULL;
    struct vcpu_metrics *vcpu_m = NULL, *last_vcpu_m = NULL;

    num_vcpus = xenstat_domain_num_vcpus(domain);

    for(vcpu_m = d->vcpu_root; vcpu_m ; vcpu_m = vcpu_m->next)
        vcpu_m->updated = 0;

    vcpu_m = d->vcpu_root;

    unsigned int  i, num_online_vcpus=0;
    for(i = 0; i < num_vcpus; i++) {
        if(unlikely(!vcpu_m)) {
            vcpu_m = callocz(1, sizeof(struct vcpu_metrics));

            if(unlikely(i == 0)) d->vcpu_root = vcpu_m;
            else last_vcpu_m->next = vcpu_m;
        }

        vcpu_m->id = i;

        vcpu = xenstat_domain_vcpu(domain, i);

        if(unlikely(!vcpu)) {
            netdata_log_error("XENSTAT: cannot get VCPU statistics.");
            return 1;
        }

        vcpu_m->online = xenstat_vcpu_online(vcpu);
        if(likely(vcpu_m->online)) { num_online_vcpus++; }
        vcpu_m->ns = xenstat_vcpu_ns(vcpu);

        vcpu_m->updated = 1;

        last_vcpu_m = vcpu_m;
        vcpu_m = vcpu_m->next;
    }

    if(unlikely(num_online_vcpus != d->cur_vcpus)) {
        d->num_vcpus_changed = 1;
        d->cur_vcpus = num_online_vcpus;
    }

    return 0;
}

static int vbd_metrics_collect(struct domain_metrics *d, xenstat_domain *domain) {
    unsigned int num_vbds = xenstat_domain_num_vbds(domain);
    xenstat_vbd *vbd = NULL;
    struct vbd_metrics *vbd_m = NULL, *last_vbd_m = NULL;

    for(vbd_m = d->vbd_root; vbd_m ; vbd_m = vbd_m->next)
        vbd_m->updated = 0;

    vbd_m = d->vbd_root;

    unsigned int  i;
    for(i = 0; i < num_vbds; i++) {
        if(unlikely(!vbd_m)) {
            vbd_m = callocz(1, sizeof(struct vbd_metrics));

            if(unlikely(i == 0)) d->vbd_root = vbd_m;
            else last_vbd_m->next = vbd_m;
        }

        vbd_m->id = i;

        vbd = xenstat_domain_vbd(domain, i);

        if(unlikely(!vbd)) {
            netdata_log_error("XENSTAT: cannot get VBD statistics.");
            return 1;
        }

#ifdef HAVE_XENSTAT_VBD_ERROR
        vbd_m->error    = xenstat_vbd_error(vbd);
#else
        vbd_m->error    = 0;
#endif
        vbd_m->oo_reqs  = xenstat_vbd_oo_reqs(vbd);
        vbd_m->rd_reqs  = xenstat_vbd_rd_reqs(vbd);
        vbd_m->wr_reqs  = xenstat_vbd_wr_reqs(vbd);
        vbd_m->rd_sects = xenstat_vbd_rd_sects(vbd);
        vbd_m->wr_sects = xenstat_vbd_wr_sects(vbd);

        vbd_m->updated = 1;

        last_vbd_m = vbd_m;
        vbd_m = vbd_m->next;
    }

    return 0;
}

static int network_metrics_collect(struct domain_metrics *d, xenstat_domain *domain) {
    unsigned int num_networks = xenstat_domain_num_networks(domain);
    xenstat_network *network = NULL;
    struct network_metrics *network_m = NULL, *last_network_m = NULL;

    for(network_m = d->network_root; network_m ; network_m = network_m->next)
        network_m->updated = 0;

    network_m = d->network_root;

    unsigned int  i;
    for(i = 0; i < num_networks; i++) {
        if(unlikely(!network_m)) {
            network_m = callocz(1, sizeof(struct network_metrics));

            if(unlikely(i == 0)) d->network_root = network_m;
            else last_network_m->next = network_m;
        }

        network_m->id = i;

        network = xenstat_domain_network(domain, i);

        if(unlikely(!network)) {
            netdata_log_error("XENSTAT: cannot get network statistics.");
            return 1;
        }

        network_m->rbytes   = xenstat_network_rbytes(network);
        network_m->rpackets = xenstat_network_rpackets(network);
        network_m->rerrs    = xenstat_network_rerrs(network);
        network_m->rdrops   = xenstat_network_rdrop(network);

        network_m->tbytes   = xenstat_network_tbytes(network);
        network_m->tpackets = xenstat_network_tpackets(network);
        network_m->terrs    = xenstat_network_terrs(network);
        network_m->tdrops   = xenstat_network_tdrop(network);

        network_m->updated = 1;

        last_network_m = network_m;
        network_m = network_m->next;
    }

    return 0;
}

static int xenstat_collect(xenstat_handle *xhandle, libxl_ctx *ctx, libxl_dominfo *info) {

    // mark all old metrics as not-updated
    struct domain_metrics *d;
    for(d = node_metrics.domain_root; d ; d = d->next)
        d->updated = 0;

    xenstat_node *node = xenstat_get_node(xhandle, XENSTAT_ALL);
    if (unlikely(!node)) {
        netdata_log_error("XENSTAT: failed to retrieve statistics from libxenstat.");
        return 1;
    }

    node_metrics.tot_mem = xenstat_node_tot_mem(node);
    node_metrics.free_mem = xenstat_node_free_mem(node);
    node_metrics.num_domains = xenstat_node_num_domains(node);
    node_metrics.num_cpus = xenstat_node_num_cpus(node);
    node_metrics.node_cpu_hz = xenstat_node_cpu_hz(node);

    int i;
    for(i = 0; i < node_metrics.num_domains; i++) {
        xenstat_domain *domain = NULL;
        char uuid[LIBXL_UUID_FMTLEN + 1];

        domain = xenstat_node_domain_by_index(node, i);

        // get domain UUID
        unsigned int id = xenstat_domain_id(domain);
        if(unlikely(libxl_domain_info(ctx, info, id))) {
            netdata_log_error("XENSTAT: cannot get domain info.");
        }
        else {
            snprintfz(uuid, LIBXL_UUID_FMTLEN, LIBXL_UUID_FMT "\n", LIBXL_UUID_BYTES(info->uuid));
        }

        uint32_t hash = simple_hash(uuid);
        d = domain_metrics_get(uuid, hash);

        d->id = id;
        if(unlikely(!d->name)) {
            d->name = strdupz(xenstat_domain_name(domain));
            netdata_fix_chart_id(d->name);
            if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: domain id %u, uuid %s has name '%s'\n", d->id, d->uuid, d->name);
        }

        d->running  = xenstat_domain_running(domain);
        d->blocked  = xenstat_domain_blocked(domain);
        d->paused   = xenstat_domain_paused(domain);
        d->shutdown = xenstat_domain_shutdown(domain);
        d->crashed  = xenstat_domain_crashed(domain);
        d->dying    = xenstat_domain_dying(domain);

        d->cpu_ns = xenstat_domain_cpu_ns(domain);
        d->cur_mem = xenstat_domain_cur_mem(domain);
        d->max_mem = xenstat_domain_max_mem(domain);

        if(unlikely(vcpu_metrics_collect(d, domain) || vbd_metrics_collect(d, domain) || network_metrics_collect(d, domain))) {
            xenstat_free_node(node);
            return 1;
        }

        d->updated = 1;
    }

    xenstat_free_node(node);

    return 0;
}

static void xenstat_send_node_metrics() {
    static int mem_chart_generated = 0, domains_chart_generated = 0, cpus_chart_generated = 0, cpu_freq_chart_generated = 0;

    // ----------------------------------------------------------------

    if(unlikely(!mem_chart_generated)) {
        printf("CHART xenstat.mem '' 'Memory Usage' 'MiB' 'memory' '' stacked %d %d '' %s\n"
               , NETDATA_CHART_PRIO_XENSTAT_NODE_MEM
               , netdata_update_every
               , PLUGIN_XENSTAT_NAME
        );
        printf("DIMENSION %s '' absolute 1 %d\n", "free", netdata_update_every * 1024 * 1024);
        printf("DIMENSION %s '' absolute 1 %d\n", "used", netdata_update_every * 1024 * 1024);
        mem_chart_generated = 1;
    }

    printf(
            "BEGIN xenstat.mem\n"
            "SET free = %lld\n"
            "SET used = %lld\n"
            "END\n"
            , (collected_number) node_metrics.free_mem
            , (collected_number) (node_metrics.tot_mem - node_metrics.free_mem)
    );

    // ----------------------------------------------------------------

    if(unlikely(!domains_chart_generated)) {
        printf("CHART xenstat.domains '' 'Number of Domains' 'domains' 'domains' '' line %d %d '' %s\n"
               , NETDATA_CHART_PRIO_XENSTAT_NODE_DOMAINS
               , netdata_update_every
               , PLUGIN_XENSTAT_NAME
        );
        printf("DIMENSION %s '' absolute 1 %d\n", "domains", netdata_update_every);
        domains_chart_generated = 1;
    }

    printf(
            "BEGIN xenstat.domains\n"
            "SET domains = %lld\n"
            "END\n"
            , (collected_number) node_metrics.num_domains
    );

    // ----------------------------------------------------------------

    if(unlikely(!cpus_chart_generated)) {
        printf("CHART xenstat.cpus '' 'Number of CPUs' 'cpus' 'cpu' '' line %d %d '' %s\n"
               , NETDATA_CHART_PRIO_XENSTAT_NODE_CPUS
               , netdata_update_every
               , PLUGIN_XENSTAT_NAME
        );
        printf("DIMENSION %s '' absolute 1 %d\n", "cpus", netdata_update_every);
        cpus_chart_generated = 1;
    }

    printf(
            "BEGIN xenstat.cpus\n"
            "SET cpus = %lld\n"
            "END\n"
            , (collected_number) node_metrics.num_cpus
    );

    // ----------------------------------------------------------------

    if(unlikely(!cpu_freq_chart_generated)) {
        printf("CHART xenstat.cpu_freq '' 'CPU Frequency' 'MHz' 'cpu' '' line %d %d '' %s\n"
               , NETDATA_CHART_PRIO_XENSTAT_NODE_CPU_FREQ
               , netdata_update_every
               , PLUGIN_XENSTAT_NAME
        );
        printf("DIMENSION %s '' absolute 1 %d\n", "frequency", netdata_update_every * 1024 * 1024);
        cpu_freq_chart_generated = 1;
    }

    printf(
            "BEGIN xenstat.cpu_freq\n"
            "SET frequency = %lld\n"
            "END\n"
            , (collected_number) node_metrics.node_cpu_hz
    );
}

static void print_domain_states_chart_definition(char *type, int obsolete_flag) {
    printf("CHART %s.states '' 'Domain States' 'boolean' 'states' 'xendomain.states' line %d %d %s %s\n"
                       , type
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_STATES
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );
    printf("DIMENSION running '' absolute 1 %d\n", netdata_update_every);
    printf("DIMENSION blocked '' absolute 1 %d\n", netdata_update_every);
    printf("DIMENSION paused '' absolute 1 %d\n", netdata_update_every);
    printf("DIMENSION shutdown '' absolute 1 %d\n", netdata_update_every);
    printf("DIMENSION crashed '' absolute 1 %d\n", netdata_update_every);
    printf("DIMENSION dying '' absolute 1 %d\n", netdata_update_every);
}

static void print_domain_cpu_chart_definition(char *type, int obsolete_flag) {
    printf("CHART %s.cpu '' 'CPU Usage (100%% = 1 core)' 'percentage' 'cpu' 'xendomain.cpu' line %d %d %s %s\n"
                       , type
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_CPU
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );
    printf("DIMENSION used '' incremental 100 %d\n", netdata_update_every * 1000000000);
}

static void print_domain_mem_chart_definition(char *type, int obsolete_flag) {
    printf("CHART %s.mem '' 'Memory Reservation' 'MiB' 'memory' 'xendomain.mem' line %d %d %s %s\n"
                       , type
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_MEM
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );
    printf("DIMENSION maximum '' absolute 1 %d\n", netdata_update_every * 1024 * 1024);
    printf("DIMENSION current '' absolute 1 %d\n", netdata_update_every * 1024 * 1024);
}

static void print_domain_vcpu_chart_definition(char *type, struct domain_metrics *d, int obsolete_flag) {
    struct vcpu_metrics *vcpu_m;

    printf("CHART %s.vcpu '' 'CPU Usage per VCPU' 'percentage' 'cpu' 'xendomain.vcpu' line %d %d %s %s\n"
                       , type
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_VCPU
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );

    for(vcpu_m = d->vcpu_root; vcpu_m; vcpu_m = vcpu_m->next) {
        if(likely(vcpu_m->updated && vcpu_m->online)) {
            printf("DIMENSION vcpu%u '' incremental 100 %d\n", vcpu_m->id, netdata_update_every * 1000000000);
        }
    }
}

static void print_domain_vbd_oo_chart_definition(char *type, unsigned int vbd, int obsolete_flag) {
    printf("CHART %s.oo_req_vbd%u '' 'VBD%u \"Out Of\" Requests' 'requests/s' 'vbd' 'xendomain.oo_req_vbd' line %u %d %s %s\n"
                       , type
                       , vbd
                       , vbd
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_VBD_OO_REQ + vbd
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );
    printf("DIMENSION requests '' incremental 1 %d\n", netdata_update_every);
}

static void print_domain_vbd_requests_chart_definition(char *type, unsigned int vbd, int obsolete_flag) {
    printf("CHART %s.requests_vbd%u '' 'VBD%u Requests' 'requests/s' 'vbd' 'xendomain.requests_vbd' line %u %d %s %s\n"
                       , type
                       , vbd
                       , vbd
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_VBD_REQUESTS + vbd
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );
    printf("DIMENSION read '' incremental 1 %d\n", netdata_update_every);
    printf("DIMENSION write '' incremental -1 %d\n", netdata_update_every);
}

static void print_domain_vbd_sectors_chart_definition(char *type, unsigned int vbd, int obsolete_flag) {
    printf("CHART %s.sectors_vbd%u '' 'VBD%u Read/Written Sectors' 'sectors/s' 'vbd' 'xendomain.sectors_vbd' line %u %d %s %s\n"
                       , type
                       , vbd
                       , vbd
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_VBD_SECTORS + vbd
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );
    printf("DIMENSION read '' incremental 1 %d\n", netdata_update_every);
    printf("DIMENSION write '' incremental -1 %d\n", netdata_update_every);
}

static void print_domain_network_bytes_chart_definition(char *type, unsigned int network, int obsolete_flag) {
    printf("CHART %s.bytes_network%u '' 'Network%u Received/Sent Bytes' 'kilobits/s' 'network' 'xendomain.bytes_network' line %u %d %s %s\n"
                       , type
                       , network
                       , network
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_NET_BYTES + network
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );
    printf("DIMENSION received '' incremental 8 %d\n", netdata_update_every * 1000);
    printf("DIMENSION sent '' incremental -8 %d\n", netdata_update_every * 1000);
}

static void print_domain_network_packets_chart_definition(char *type, unsigned int network, int obsolete_flag) {
    printf("CHART %s.packets_network%u '' 'Network%u Received/Sent Packets' 'packets/s' 'network' 'xendomain.packets_network' line %u %d %s %s\n"
                       , type
                       , network
                       , network
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_NET_PACKETS + network
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );
    printf("DIMENSION received '' incremental 1 %d\n", netdata_update_every);
    printf("DIMENSION sent '' incremental -1 %d\n", netdata_update_every);
}

static void print_domain_network_errors_chart_definition(char *type, unsigned int network, int obsolete_flag) {
    printf("CHART %s.errors_network%u '' 'Network%u Receive/Transmit Errors' 'errors/s' 'network' 'xendomain.errors_network' line %u %d %s %s\n"
                       , type
                       , network
                       , network
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_NET_PACKETS + network
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );
    printf("DIMENSION received '' incremental 1 %d\n", netdata_update_every);
    printf("DIMENSION sent '' incremental -1 %d\n", netdata_update_every);
}

static void print_domain_network_drops_chart_definition(char *type, unsigned int network, int obsolete_flag) {
    printf("CHART %s.drops_network%u '' 'Network%u Receive/Transmit Drops' 'drops/s' 'network' 'xendomain.drops_network' line %u %d %s %s\n"
                       , type
                       , network
                       , network
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_NET_PACKETS + network
                       , netdata_update_every
                       , obsolete_flag ? "obsolete": "''"
                       , PLUGIN_XENSTAT_NAME
    );
    printf("DIMENSION received '' incremental 1 %d\n", netdata_update_every);
    printf("DIMENSION sent '' incremental -1 %d\n", netdata_update_every);
}

static void xenstat_send_domain_metrics() {

    if(unlikely(!node_metrics.domain_root)) return;
    struct domain_metrics *d;

    for(d = node_metrics.domain_root; d; d = d->next) {
        char type[TYPE_LENGTH_MAX + 1];
        snprintfz(type, TYPE_LENGTH_MAX, "xendomain_%s_%s", d->name, d->uuid);

        if(likely(d->updated)) {

            // ----------------------------------------------------------------

            if(unlikely(!d->states_chart_generated)) {
                print_domain_states_chart_definition(type, CHART_IS_NOT_OBSOLETE);
                d->states_chart_generated = 1;
            }
            printf(
                    "BEGIN %s.states\n"
                    "SET running = %lld\n"
                    "SET blocked = %lld\n"
                    "SET paused = %lld\n"
                    "SET shutdown = %lld\n"
                    "SET crashed = %lld\n"
                    "SET dying = %lld\n"
                    "END\n"
                    , type
                    , (collected_number)d->running
                    , (collected_number)d->blocked
                    , (collected_number)d->paused
                    , (collected_number)d->shutdown
                    , (collected_number)d->crashed
                    , (collected_number)d->dying
            );

            // ----------------------------------------------------------------

            if(unlikely(!d->cpu_chart_generated)) {
                print_domain_cpu_chart_definition(type, CHART_IS_NOT_OBSOLETE);
                d->cpu_chart_generated = 1;
            }
            printf(
                    "BEGIN %s.cpu\n"
                    "SET used = %lld\n"
                    "END\n"
                    , type
                    , (collected_number)d->cpu_ns
            );

            // ----------------------------------------------------------------

            struct vcpu_metrics *vcpu_m;

            if(unlikely(!d->vcpu_chart_generated || d->num_vcpus_changed)) {
                print_domain_vcpu_chart_definition(type, d, CHART_IS_NOT_OBSOLETE);
                d->num_vcpus_changed = 0;
                d->vcpu_chart_generated = 1;
            }

            printf("BEGIN %s.vcpu\n", type);
            for(vcpu_m = d->vcpu_root; vcpu_m; vcpu_m = vcpu_m->next) {
                if(likely(vcpu_m->updated && vcpu_m->online)) {
                    printf(
                            "SET vcpu%u = %lld\n"
                            , vcpu_m->id
                            , (collected_number)vcpu_m->ns
                    );
                }
            }
            printf("END\n");

            // ----------------------------------------------------------------

            if(unlikely(!d->mem_chart_generated)) {
                print_domain_mem_chart_definition(type, CHART_IS_NOT_OBSOLETE);
                d->mem_chart_generated = 1;
            }
            printf(
                    "BEGIN %s.mem\n"
                    "SET maximum = %lld\n"
                    "SET current = %lld\n"
                    "END\n"
                    , type
                    , (collected_number)d->max_mem
                    , (collected_number)d->cur_mem
            );

            // ----------------------------------------------------------------

            struct vbd_metrics *vbd_m;
            for(vbd_m = d->vbd_root; vbd_m; vbd_m = vbd_m->next) {
                if(likely(vbd_m->updated && !vbd_m->error)) {
                    if(unlikely(!vbd_m->oo_req_chart_generated)) {
                        print_domain_vbd_oo_chart_definition(type, vbd_m->id, CHART_IS_NOT_OBSOLETE);
                        vbd_m->oo_req_chart_generated = 1;
                    }
                    printf(
                            "BEGIN %s.oo_req_vbd%u\n"
                            "SET requests = %lld\n"
                            "END\n"
                            , type
                            , vbd_m->id
                            , (collected_number)vbd_m->oo_reqs
                    );

                    // ----------------------------------------------------------------

                    if(unlikely(!vbd_m->requests_chart_generated)) {
                        print_domain_vbd_requests_chart_definition(type, vbd_m->id, CHART_IS_NOT_OBSOLETE);
                        vbd_m->requests_chart_generated = 1;
                    }
                    printf(
                            "BEGIN %s.requests_vbd%u\n"
                            "SET read = %lld\n"
                            "SET write = %lld\n"
                            "END\n"
                            , type
                            , vbd_m->id
                            , (collected_number)vbd_m->rd_reqs
                            , (collected_number)vbd_m->wr_reqs
                    );

                    // ----------------------------------------------------------------

                    if(unlikely(!vbd_m->sectors_chart_generated)) {
                        print_domain_vbd_sectors_chart_definition(type, vbd_m->id, CHART_IS_NOT_OBSOLETE);
                        vbd_m->sectors_chart_generated = 1;
                    }
                    printf(
                            "BEGIN %s.sectors_vbd%u\n"
                            "SET read = %lld\n"
                            "SET write = %lld\n"
                            "END\n"
                            , type
                            , vbd_m->id
                            , (collected_number)vbd_m->rd_sects
                            , (collected_number)vbd_m->wr_sects
                    );
                }
                else {
                    if(unlikely(vbd_m->oo_req_chart_generated
                                || vbd_m->requests_chart_generated
                                || vbd_m->sectors_chart_generated)) {
                        if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: mark charts as obsolete for vbd %u, domain '%s', id %u, uuid %s\n", vbd_m->id, d->name, d->id, d->uuid);
                        print_domain_vbd_oo_chart_definition(type, vbd_m->id, CHART_IS_OBSOLETE);
                        print_domain_vbd_requests_chart_definition(type, vbd_m->id, CHART_IS_OBSOLETE);
                        print_domain_vbd_sectors_chart_definition(type, vbd_m->id, CHART_IS_OBSOLETE);
                        vbd_m->oo_req_chart_generated = 0;
                        vbd_m->requests_chart_generated = 0;
                        vbd_m->sectors_chart_generated = 0;
                    }
                }
            }

            // ----------------------------------------------------------------

            struct network_metrics *network_m;
            for(network_m = d->network_root; network_m; network_m = network_m->next) {
                if(likely(network_m->updated)) {
                    if(unlikely(!network_m->bytes_chart_generated)) {
                        print_domain_network_bytes_chart_definition(type, network_m->id, CHART_IS_NOT_OBSOLETE);
                        network_m->bytes_chart_generated = 1;
                    }
                    printf(
                            "BEGIN %s.bytes_network%u\n"
                            "SET received = %lld\n"
                            "SET sent = %lld\n"
                            "END\n"
                            , type
                            , network_m->id
                            , (collected_number)network_m->rbytes
                            , (collected_number)network_m->tbytes
                    );

                    // ----------------------------------------------------------------

                    if(unlikely(!network_m->packets_chart_generated)) {
                        print_domain_network_packets_chart_definition(type, network_m->id, CHART_IS_NOT_OBSOLETE);
                        network_m->packets_chart_generated = 1;
                    }
                    printf(
                            "BEGIN %s.packets_network%u\n"
                            "SET received = %lld\n"
                            "SET sent = %lld\n"
                            "END\n"
                            , type
                            , network_m->id
                            , (collected_number)network_m->rpackets
                            , (collected_number)network_m->tpackets
                    );

                    // ----------------------------------------------------------------

                    if(unlikely(!network_m->errors_chart_generated)) {
                        print_domain_network_errors_chart_definition(type, network_m->id, CHART_IS_NOT_OBSOLETE);
                        network_m->errors_chart_generated = 1;
                    }
                    printf(
                            "BEGIN %s.errors_network%u\n"
                            "SET received = %lld\n"
                            "SET sent = %lld\n"
                            "END\n"
                            , type
                            , network_m->id
                            , (collected_number)network_m->rerrs
                            , (collected_number)network_m->terrs
                    );

                    // ----------------------------------------------------------------

                    if(unlikely(!network_m->drops_chart_generated)) {
                        print_domain_network_drops_chart_definition(type, network_m->id, CHART_IS_NOT_OBSOLETE);
                        network_m->drops_chart_generated = 1;
                    }
                    printf(
                            "BEGIN %s.drops_network%u\n"
                            "SET received = %lld\n"
                            "SET sent = %lld\n"
                            "END\n"
                            , type
                            , network_m->id
                            , (collected_number)network_m->rdrops
                            , (collected_number)network_m->tdrops
                    );
                }
                else {
                    if(unlikely(network_m->bytes_chart_generated
                                || network_m->packets_chart_generated
                                || network_m->errors_chart_generated
                                || network_m->drops_chart_generated))
                    if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: mark charts as obsolete for network %u, domain '%s', id %u, uuid %s\n", network_m->id, d->name, d->id, d->uuid);
                    print_domain_network_bytes_chart_definition(type, network_m->id, CHART_IS_OBSOLETE);
                    print_domain_network_packets_chart_definition(type, network_m->id, CHART_IS_OBSOLETE);
                    print_domain_network_errors_chart_definition(type, network_m->id, CHART_IS_OBSOLETE);
                    print_domain_network_drops_chart_definition(type, network_m->id, CHART_IS_OBSOLETE);
                    network_m->bytes_chart_generated   = 0;
                    network_m->packets_chart_generated = 0;
                    network_m->errors_chart_generated  = 0;
                    network_m->drops_chart_generated   = 0;
                }
            }
        }
        else{
            if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: mark charts as obsolete for domain '%s', id %u, uuid %s\n", d->name, d->id, d->uuid);
            print_domain_states_chart_definition(type, CHART_IS_OBSOLETE);
            print_domain_cpu_chart_definition(type, CHART_IS_OBSOLETE);
            print_domain_vcpu_chart_definition(type, d, CHART_IS_OBSOLETE);
            print_domain_mem_chart_definition(type, CHART_IS_OBSOLETE);

            d = domain_metrics_free(d);
        }
    }
}

int main(int argc, char **argv) {
    // ------------------------------------------------------------------------
    // initialization of netdata plugin

    program_name = PLUGIN_XENSTAT_NAME;

    nd_log_initialize_for_external_plugins(PLUGIN_XENSTAT_NAME);
    netdata_threads_init_for_external_plugins(0);

    // ------------------------------------------------------------------------
    // parse command line parameters

    int i, freq = 0;
    for(i = 1; i < argc ; i++) {
        if(isdigit(*argv[i]) && !freq) {
            int n = str2i(argv[i]);
            if(n > 0 && n < 86400) {
                freq = n;
                continue;
            }
        }
        else if(strcmp("version", argv[i]) == 0 || strcmp("-version", argv[i]) == 0 || strcmp("--version", argv[i]) == 0 || strcmp("-v", argv[i]) == 0 || strcmp("-V", argv[i]) == 0) {
            printf("xenstat.plugin %s\n", NETDATA_VERSION);
            exit(0);
        }
        else if(strcmp("debug", argv[i]) == 0) {
            debug = 1;
            continue;
        }
        else if(strcmp("-h", argv[i]) == 0 || strcmp("--help", argv[i]) == 0) {
            fprintf(stderr,
                    "\n"
                    " netdata xenstat.plugin %s\n"
                    " Copyright 2018-2025 Netdata Inc.\n"
                    " Released under GNU General Public License v3 or later.\n"
                    "\n"
                    " This program is a data collector plugin for netdata.\n"
                    "\n"
                    " Available command line options:\n"
                    "\n"
                    "  COLLECTION_FREQUENCY    data collection frequency in seconds\n"
                    "                          minimum: %d\n"
                    "\n"
                    "  debug                   enable verbose output\n"
                    "                          default: disabled\n"
                    "\n"
                    "  -v\n"
                    "  -V\n"
                    "  --version               print version and exit\n"
                    "\n"
                    "  -h\n"
                    "  --help                  print this message and exit\n"
                    "\n"
                    " For more information:\n"
                    " https://github.com/netdata/netdata/tree/master/src/collectors/xenstat.plugin\n"
                    "\n"
                    , NETDATA_VERSION
                    , netdata_update_every
            );
            exit(1);
        }

        netdata_log_error("xenstat.plugin: ignoring parameter '%s'", argv[i]);
    }

    errno_clear();

    if(freq >= netdata_update_every)
        netdata_update_every = freq;
    else if(freq)
        netdata_log_error("update frequency %d seconds is too small for XENSTAT. Using %d.", freq, netdata_update_every);

    // ------------------------------------------------------------------------
    // initialize xen API handles
    xenstat_handle *xhandle = NULL;
    libxl_ctx *ctx = NULL;
    libxl_dominfo info;

    if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: calling xenstat_init()\n");
    xhandle = xenstat_init();
    if (xhandle == NULL) {
        netdata_log_error("XENSTAT: failed to initialize xenstat library.");
        return 1;
    }

    if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: calling libxl_ctx_alloc()\n");
    if (libxl_ctx_alloc(&ctx, LIBXL_VERSION, 0, NULL)) {
        netdata_log_error("XENSTAT: failed to initialize xl context.");
        xenstat_uninit(xhandle);
        return 1;
    }
    libxl_dominfo_init(&info);

    // ------------------------------------------------------------------------
    // the main loop

    if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: starting data collection\n");

    time_t started_t = now_monotonic_sec();

    size_t iteration;

    heartbeat_t hb;
    heartbeat_init(&hb, netdata_update_every * USEC_PER_SEC);
    for(iteration = 0; 1; iteration++) {
        usec_t dt = heartbeat_next(&hb);

        if(unlikely(exit_initiated)) break;

        if(unlikely(debug && iteration))
            fprintf(stderr, "xenstat.plugin: iteration %zu, dt %lu usec\n", iteration, dt);

        if(likely(xhandle)) {
            if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: calling xenstat_collect()\n");
            int ret = xenstat_collect(xhandle, ctx, &info);

            if(likely(!ret)) {
                if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: calling xenstat_send_node_metrics()\n");
                xenstat_send_node_metrics();
                if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: calling xenstat_send_domain_metrics()\n");
                xenstat_send_domain_metrics();
            }
            else {
                if(unlikely(debug)) fprintf(stderr, "xenstat.plugin: can't collect data\n");
            }
        }

        fflush(stdout);

        // restart check (14400 seconds)
        if(unlikely(now_monotonic_sec() - started_t > 14400)) break;
    }

    libxl_ctx_free(ctx);
    xenstat_uninit(xhandle);
    netdata_log_info("XENSTAT process exiting");
    
    return 0;
}
