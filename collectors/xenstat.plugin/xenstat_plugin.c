// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../libnetdata/libnetdata.h"

#define PLUGIN_XENSTAT_NAME "xenstat.plugin"

#define NETDATA_CHART_PRIO_XENSTAT_NODE_MEM         8701
#define NETDATA_CHART_PRIO_XENSTAT_NODE_TMEM        8702
#define NETDATA_CHART_PRIO_XENSTAT_NODE_DOMAINS     8703
#define NETDATA_CHART_PRIO_XENSTAT_NODE_CPUS        8704
#define NETDATA_CHART_PRIO_XENSTAT_NODE_CPU_FREQ    8705

#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_CPU       8901
#define NETDATA_CHART_PRIO_XENSTAT_DOMAIN_MEM       8902

// callback required by fatal()
void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

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

// callback required by eval()
int health_variable_lookup(const char *variable, uint32_t hash, struct rrdcalc *rc, calculated_number *result) {
    (void)variable;
    (void)hash;
    (void)rc;
    (void)result;
    return 0;
};

// required by get_system_cpus()
char *netdata_configured_host_prefix = "";

// Variables

static int debug = 0;

static int netdata_update_every = 1;

#ifdef HAVE_LIBXENSTAT
#include <xenstat.h>
#include <libxl.h>

static xenstat_handle *xhandle = NULL;
static libxl_ctx *ctx = NULL;

struct domain_metrics {
    char *uuid;
    uint32_t hash;

    unsigned int id;
    char *name;

    unsigned long long cpu_ns;
    unsigned long long cur_mem;
    unsigned long long max_mem;

    int cpu_chart_generated;
    int mem_chart_generated;

    int updated;

    struct domain_metrics *next;
};

struct node_metrics{
    unsigned long long tot_mem;
    unsigned long long free_mem;
    long freeable_mb;
    int num_domains;
    unsigned int num_cpus;
    unsigned long long node_cpu_hz;

    struct domain_metrics *domain_root;
};

static struct node_metrics node_metrics = {
        .domain_root = NULL
};

static inline struct domain_metrics *domain_metrics_get(const char *name, uint32_t hash) {
    struct domain_metrics *d = NULL, *last = NULL;
    for(d = node_metrics.domain_root; d ; last = d, d = d->next) {
        if(unlikely(d->hash == hash && !strcmp(d->name, name)))
            return d;
    }

    d = callocz(1, sizeof(struct domain_metrics));
    d->name = strdupz(name);
    d->hash = hash;

    if(!last) {
        d->next = node_metrics.domain_root;
        node_metrics.domain_root = d;
    }
    else {
        d->next = last->next;
        last->next = d;
    }

    return d;
}

static int xenstat_collect() {
    static xenstat_node *node = NULL;

    // mark all old metrics as not-updated
    struct domain_metrics *d;
    for(d = node_metrics.domain_root; d ; d = d->next)
        d->updated = 0;

    if (likely(node))
        xenstat_free_node(node);
    node = xenstat_get_node(xhandle, XENSTAT_ALL);
    if (unlikely(!node)) {
        printf("XENSTAT: failed to retrieve statistics from libxenstat\n");
        return 1;
    }

    node_metrics.tot_mem = xenstat_node_tot_mem(node);
    node_metrics.free_mem = xenstat_node_free_mem(node);
    node_metrics.freeable_mb = xenstat_node_freeable_mb(node);
    node_metrics.num_domains = xenstat_node_num_domains(node);
    node_metrics.num_cpus = xenstat_node_num_cpus(node);
    node_metrics.node_cpu_hz = xenstat_node_cpu_hz(node);

    int i;
    for(i = 0; i < node_metrics.num_domains; i++) {
        xenstat_domain *domain = NULL;
        domain = xenstat_node_domain_by_index(node, i);

        const char *name = xenstat_domain_name(domain);
        uint32_t hash = simple_hash(name);
        d = domain_metrics_get(name, hash);

        d->cpu_ns = xenstat_domain_cpu_ns(domain);
        d->cur_mem = xenstat_domain_cur_mem(domain);
        d->max_mem = xenstat_domain_max_mem(domain);

        d->updated = 1;
    }
    return 0;
}

static void xenstat_send_node_metrics() {
    static int mem_chart_generated = 0, tmem_chart_generated = 0, domains_chart_generated = 0, cpus_chart_generated = 0, cpu_freq_chart_generated = 0;

    if(!mem_chart_generated) {
        mem_chart_generated = 1;
        printf("CHART xenstat.mem '' 'Node Memory Usage' 'MiB' 'xenstat' '' stacked %d %d %s\n"
               , NETDATA_CHART_PRIO_XENSTAT_NODE_MEM
               , netdata_update_every
               , PLUGIN_XENSTAT_NAME
        );
        printf("DIMENSION %s '' absolute 1 %d\n", "free", netdata_update_every * 1024 * 1024);
        printf("DIMENSION %s '' absolute 1 %d\n", "used", netdata_update_every * 1024 * 1024);
    }

    printf(
            "BEGIN xenstat.mem\n"
            "SET free = %lld\n"
            "SET used = %lld\n"
            "END\n"
            , (collected_number) node_metrics.free_mem
            , (collected_number) (node_metrics.tot_mem - node_metrics.free_mem)
    );

    if(!tmem_chart_generated) {
        tmem_chart_generated = 1;
        printf("CHART xenstat.tmem '' 'Freeable Node Transcedent Memory' 'MiB' 'xenstat' '' line %d %d %s\n"
               , NETDATA_CHART_PRIO_XENSTAT_NODE_TMEM
               , netdata_update_every
               , PLUGIN_XENSTAT_NAME
        );
        printf("DIMENSION %s '' absolute 1 %d\n", "freeable", netdata_update_every * 1024 * 1024);
    }

    printf(
            "BEGIN xenstat.tmem\n"
            "SET freeable = %lld\n"
            "END\n"
            , (collected_number) node_metrics.freeable_mb
    );

    if(!domains_chart_generated) {
        domains_chart_generated = 1;
        printf("CHART xenstat.domains '' 'Number of Domains on XenServer Node' 'domains' 'xenstat' '' line %d %d %s\n"
               , NETDATA_CHART_PRIO_XENSTAT_NODE_DOMAINS
               , netdata_update_every
               , PLUGIN_XENSTAT_NAME
        );
        printf("DIMENSION %s '' absolute 1 %d\n", "domains", netdata_update_every);
    }

    printf(
            "BEGIN xenstat.domains\n"
            "SET domains = %lld\n"
            "END\n"
            , (collected_number) node_metrics.num_domains
    );

    if(!cpus_chart_generated) {
        cpus_chart_generated = 1;
        printf("CHART xenstat.cpus '' 'Number of CPUs on XenServer Node' 'cpus' 'xenstat' '' line %d %d %s\n"
               , NETDATA_CHART_PRIO_XENSTAT_NODE_CPUS
               , netdata_update_every
               , PLUGIN_XENSTAT_NAME
        );
        printf("DIMENSION %s '' absolute 1 %d\n", "cpus", netdata_update_every);
    }

    printf(
            "BEGIN xenstat.cpus\n"
            "SET cpus = %lld\n"
            "END\n"
            , (collected_number) node_metrics.num_cpus
    );

    if(!cpu_freq_chart_generated) {
        cpu_freq_chart_generated = 1;
        printf("CHART xenstat.cpu_freq '' 'CPU frequency on XenServer Node' 'MHz' 'xenstat' '' line %d %d %s\n"
               , NETDATA_CHART_PRIO_XENSTAT_NODE_CPU_FREQ
               , netdata_update_every
               , PLUGIN_XENSTAT_NAME
        );
        printf("DIMENSION %s '' absolute 1 %d\n", "frequency", netdata_update_every * 1024 * 1024);
    }

    printf(
            "BEGIN xenstat.cpu_freq\n"
            "SET frequency = %lld\n"
            "END\n"
            , (collected_number) node_metrics.node_cpu_hz
    );
}

static void xenstat_send_domain_metrics() {

    if(!node_metrics.domain_root) return;
    struct domain_metrics *d;

    for(d = node_metrics.domain_root; d; d = d->next) {
        if(likely(d->updated)) {
            if(!d->cpu_chart_generated) {
                d->cpu_chart_generated = 1;
                printf("CHART %s.xenstat_domain_cpu '' 'CPU usage for XenServer Domain' 'percentage' '' '' line %d %d %s\n"
                       , d->name
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_CPU
                       , netdata_update_every
                       , PLUGIN_XENSTAT_NAME
                );
                printf("DIMENSION usage '' incremental 100 %d\n", netdata_update_every * 1000000000);
            }
            printf(
                    "BEGIN %s.xenstat_domain_cpu\n"
                    "SET usage = %lld\n"
                    "END\n"
                    , d->name
                    , (collected_number)d->cpu_ns
            );

            if(!d->mem_chart_generated) {
                d->mem_chart_generated = 1;
                printf("CHART %s.xenstat_domain_mem '' 'Memory reservation for XenServer Domain' 'MiB' '' '' line %d %d %s\n"
                       , d->name
                       , NETDATA_CHART_PRIO_XENSTAT_DOMAIN_MEM
                       , netdata_update_every
                       , PLUGIN_XENSTAT_NAME
                );
                printf("DIMENSION maximum '' absolute 1 %d\n", netdata_update_every * 1024 * 1024);
                printf("DIMENSION current '' absolute 1 %d\n", netdata_update_every * 1024 * 1024);
            }
            printf(
                    "BEGIN %s.xenstat_domain_mem\n"
                    "SET maximum = %lld\n"
                    "SET current = %lld\n"
                    "END\n"
                    , d->name
                    , (collected_number)d->max_mem
                    , (collected_number)d->cur_mem
            );
        }
    }
}

int main(int argc, char **argv) {

    // ------------------------------------------------------------------------
    // initialization of netdata plugin

    program_name = "xenstat.plugin";

    // disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

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
            printf("xenstat.plugin %s\n", VERSION);
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
                    " Copyright (C) 2019 Netdata Inc.\n"
                    " Released under GNU General Public License v3 or later.\n"
                    " All rights reserved.\n"
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
                    " https://github.com/netdata/netdata/tree/master/collectors/xenstat.plugin\n"
                    "\n"
                    , VERSION
                    , netdata_update_every
            );
            exit(1);
        }

        error("xenstat.plugin: ignoring parameter '%s'", argv[i]);
    }

    errno = 0;

    if(freq >= netdata_update_every)
        netdata_update_every = freq;
    else if(freq)
        error("update frequency %d seconds is too small for XENSTAT. Using %d.", freq, netdata_update_every);

    // ------------------------------------------------------------------------
    // initialize xen API handles

    if(debug) fprintf(stderr, "xenstat.plugin: calling xenstat_init()\n");
    xhandle = xenstat_init();
    if (xhandle == NULL)
        error("XENSTAT: failed to initialize xenstat library.");

    if(debug) fprintf(stderr, "xenstat.plugin: calling libxl_ctx_alloc()\n");
    if (libxl_ctx_alloc(&ctx, LIBXL_VERSION, 0, NULL)) {
        error("XENSTAT: failed to initialize xl context.");
    }

    // ------------------------------------------------------------------------
    // the main loop

    if(debug) fprintf(stderr, "xenstat.plugin: starting data collection\n");

    time_t started_t = now_monotonic_sec();

    size_t iteration;
    usec_t step = netdata_update_every * USEC_PER_SEC;

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1; iteration++) {
        usec_t dt = heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

        if(debug && iteration)
            fprintf(stderr, "xenstat.plugin: iteration %zu, dt %llu usec\n"
                    , iteration
                    , dt
            );

        if(likely(xhandle)) {
            if(debug) fprintf(stderr, "xenstat.plugin: calling xenstat_collect()\n");
            int ret = xenstat_collect();

            if(likely(!ret)) {
                if(debug) fprintf(stderr, "xenstat.plugin: calling xenstat_send_node_metrics()\n");
                xenstat_send_node_metrics();
                if(debug) fprintf(stderr, "xenstat.plugin: calling xenstat_send_domain_metrics()\n");
                xenstat_send_domain_metrics();
            }
        }

        fflush(stdout);

        // restart check (14400 seconds)
        if(now_monotonic_sec() - started_t > 14400) break;
    }

    info("XENSTAT process exiting");
}

#else // !HAVE_LIBXENSTAT

int main(int argc, char **argv) {
    (void)argc;
    (void)argv;

    fatal("xenstat.plugin is not compiled.");
}

#endif // !HAVE_LIBXENSTAT
