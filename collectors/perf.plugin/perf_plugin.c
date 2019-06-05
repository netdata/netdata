// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../libnetdata/libnetdata.h"

#define PLUGIN_PERF_NAME "perf.plugin"

#define NETDATA_CHART_PRIO_PERF_CPU_CYCLES            8701

#define NETDATA_CHART_PRIO_PERF_CACHE_LL              8906

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

#define RRD_TYPE_PERF "perf"

static struct {
    int update_every;
    char *buf;
    size_t buf_size;
    unsigned int seq;
    uint32_t portid;
    kernel_uint_t metrics[10];
} perf_root = {
        .update_every = 1,
        .buf = NULL,
        .buf_size = 0,
        .seq = 0,
        .metrics = {},
};


static int perf_init(int update_every) {
    perf_root.update_every = update_every;

    perf_root.buf_size = 10;
    perf_root.buf = mallocz(perf_root.buf_size);

    perf_root.seq = (unsigned int)now_realtime_sec() - 1;

    return 0;
}

static int perf_collect() {
    perf_root.seq++;

    return 0;
}

static void perf_send_metrics() {
    static int new_chart_generated = 0;

    if(!new_chart_generated) {
        new_chart_generated = 1;

        printf("CHART %s.%s '' 'CPU cycles' 'cycles/s' %s '' line %d %d %s\n"
               , RRD_TYPE_PERF
               , "cpu_cycles"
               , RRD_TYPE_PERF
               , NETDATA_CHART_PRIO_PERF_CPU_CYCLES
               , perf_root.update_every
               , PLUGIN_PERF_NAME
        );
        printf("DIMENSION %s '' incremental 1 1\n", "cycles");
    }

    printf(
           "BEGIN %s.%s\n"
           , RRD_TYPE_PERF
           , "cpu_cycles"
    );
    printf(
           "SET %s = %lld\n"
           , "cycles"
           , (collected_number) perf_root.metrics[0]
    );
    printf("END\n");
}

int main(int argc, char **argv) {

    // ------------------------------------------------------------------------
    // initialization of netdata plugin

    program_name = "perf.plugin";

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
            printf("perf.plugin %s\n", VERSION);
            exit(0);
        }
        else if(strcmp("debug", argv[i]) == 0) {
            debug = 1;
            continue;
        }
        else if(strcmp("-h", argv[i]) == 0 || strcmp("--help", argv[i]) == 0) {
            fprintf(stderr,
                    "\n"
                    " netdata perf.plugin %s\n"
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
                    " https://github.com/netdata/netdata/tree/master/collectors/perf.plugin\n"
                    "\n"
                    , VERSION
                    , netdata_update_every
            );
            exit(1);
        }

        error("perf.plugin: ignoring parameter '%s'", argv[i]);
    }

    errno = 0;

    if(freq >= netdata_update_every)
        netdata_update_every = freq;
    else if(freq)
        error("update frequency %d seconds is too small for PERF. Using %d.", freq, netdata_update_every);

    if(debug) fprintf(stderr, "perf.plugin: calling perf_init()\n");
    int perf = !perf_init(netdata_update_every);

    // ------------------------------------------------------------------------
    // the main loop

    if(debug) fprintf(stderr, "perf.plugin: starting data collection\n");

    time_t started_t = now_monotonic_sec();

    size_t iteration;
    usec_t step = netdata_update_every * USEC_PER_SEC;

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1; iteration++) {
        usec_t dt = heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

        if(debug && iteration)
            fprintf(stderr, "perf.plugin: iteration %zu, dt %llu usec\n"
                    , iteration
                    , dt
            );

        if(likely(perf)) {
            if(debug) fprintf(stderr, "perf.plugin: calling perf_collect()\n");
            perf = !perf_collect();

            if(likely(perf)) {
                if(debug) fprintf(stderr, "perf.plugin: calling perf_send_metrics()\n");
                perf_send_metrics();
            }
        }

        fflush(stdout);

        // restart check (14400 seconds)
        if(now_monotonic_sec() - started_t > 14400) break;
    }

    info("PERF process exiting");
}
