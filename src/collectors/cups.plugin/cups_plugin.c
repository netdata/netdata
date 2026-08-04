// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#include <cups/cups.h>
#include <limits.h>

// Variables

static int debug = 0;

static int netdata_update_every = 1;
static uint32_t netdata_priority = 100004;

http_t *http; // connection to the cups daemon

/*
 * Used to aggregate job metrics for a destination (and all destinations).
 */
struct job_metrics {
    uint32_t id;

    bool is_collected; // flag if this was collected in the current cycle

    int num_pending;
    int num_processing;
    int num_held;

    int size_pending;    // in kilobyte
    int size_processing; // in kilobyte
    int size_held;       // in kilobyte
};
DICTIONARY *dict_dest_job_metrics = NULL;
struct job_metrics global_job_metrics;

int num_dest_total;
int num_dest_accepting_jobs;
int num_dest_shared;

int num_dest_idle;
int num_dest_printing;
int num_dest_stopped;

void print_help() {
    fprintf(stderr,
            "\n"
            "netdata cups.plugin %s\n"
            "\n"
            "Copyright 2018-2025 Netdata Inc.\n"
            "Original Author: Simon Nagl <simon.nagl@gmx.de>\n"
            "Released under GNU General Public License v3+.\n"
            "\n"
            "This program is a data collector plugin for netdata.\n"
            "\n"
            "SYNOPSIS: cups.plugin [-d][-h][-v] COLLECTION_FREQUENCY\n"
            "\n"
            "Options:"
            "\n"
            "  COLLECTION_FREQUENCY    data collection frequency in seconds\n"
            "\n"
            "  -d                      enable verbose output\n"
            "                          default: disabled\n"
            "\n"
            "  -v                      print version and exit\n"
            "\n"
            "  -h                      print this message and exit\n"
            "\n",
            NETDATA_VERSION);
}

void parse_command_line(int argc, char **argv) {
    int i;
    int freq = 0;
    int update_every_found = 0;
    for (i = 1; i < argc; i++) {
        if (isdigit(*argv[i]) && !update_every_found) {
            int n = str2i(argv[i]);
            if (n > 0 && n < 86400) {
                freq = n;
                continue;
            }
        } else if (strcmp("-v", argv[i]) == 0) {
            printf("cups.plugin %s\n", NETDATA_VERSION);
            exit(0);
        } else if (strcmp("-d", argv[i]) == 0) {
            debug = 1;
            continue;
        } else if (strcmp("-h", argv[i]) == 0) {
            print_help();
            exit(0);
        }

        print_help();
        exit(1);
    }

    if (freq >= netdata_update_every) {
        netdata_update_every = freq;
    } else if (freq) {
        netdata_log_error("update frequency %d seconds is too small for CUPS. Using %d.", freq, netdata_update_every);
    }
}

static int reset_job_metrics(const DICTIONARY_ITEM *item __maybe_unused, void *entry, void *data __maybe_unused) {
    struct job_metrics *jm = (struct job_metrics *)entry;

    jm->is_collected = false;
    jm->num_held = 0;
    jm->num_pending = 0;
    jm->num_processing = 0;
    jm->size_held = 0;
    jm->size_pending = 0;
    jm->size_processing = 0;

    return 0;
}

void send_job_charts_definitions_to_netdata(const char *name, uint32_t job_id, bool obsolete) {
    printf("CHART cups.job_num_%s '' 'Active jobs of %s' jobs '%s' cups.destination_job_num stacked %u %i %s\n",
           name, name, name, netdata_priority + job_id, netdata_update_every, obsolete?"obsolete":"");
    printf("DIMENSION pending '' absolute 1 1\n");
    printf("DIMENSION held '' absolute 1 1\n");
    printf("DIMENSION processing '' absolute 1 1\n");

    printf("CHART cups.job_size_%s '' 'Active jobs size of %s' KB '%s' cups.destination_job_size stacked %u %i %s\n",
           name, name, name, netdata_priority + 1 + job_id, netdata_update_every, obsolete?"obsolete":"");
    printf("DIMENSION pending '' absolute 1 1\n");
    printf("DIMENSION held '' absolute 1 1\n");
    printf("DIMENSION processing '' absolute 1 1\n");
}

struct job_metrics *get_job_metrics(const char *dest) {
    struct job_metrics *jm = dictionary_get(dict_dest_job_metrics, dest);

    if (unlikely(!jm)) {
        static uint32_t job_id = 0;
        struct job_metrics new_job_metrics = { .id = ++job_id };
        jm = dictionary_set(dict_dest_job_metrics, dest, &new_job_metrics, sizeof(struct job_metrics));
        send_job_charts_definitions_to_netdata(dest, jm->id, false);
    }

    return jm;
}

int send_job_metrics_to_netdata(const DICTIONARY_ITEM *item, void *entry, void *data __maybe_unused) {
    const char *name = dictionary_acquired_item_name(item);

    struct job_metrics *jm = (struct job_metrics *)entry;

    if (jm->is_collected) {
        printf(
            "BEGIN cups.job_num_%s\n"
            "SET pending = %d\n"
            "SET held = %d\n"
            "SET processing = %d\n"
            "END\n",
            name, jm->num_pending, jm->num_held, jm->num_processing);
        printf(
            "BEGIN cups.job_size_%s\n"
            "SET pending = %d\n"
            "SET held = %d\n"
            "SET processing = %d\n"
            "END\n",
            name, jm->size_pending, jm->size_held, jm->size_processing);
    }
    else {
        // mark it obsolete
        send_job_charts_definitions_to_netdata(name, jm->id, true);

        // delete it
        dictionary_del(dict_dest_job_metrics, name);
    }

    return 0;
}

struct destination_metrics {
    const char *name;
    bool has_uri;
    bool is_accepting_jobs;
    bool is_shared;
    int state;
};

static void reset_destination_metrics(struct destination_metrics *dest) {
    *dest = (struct destination_metrics) {
        .state = INT_MIN,
    };
}

static void collect_destination_metrics(struct destination_metrics *dest) {
    // Keep the former cupsGetDests2() configured-queue guard without invoking Bonjour discovery.
    if (!dest->name || !dest->has_uri)
        return;

    num_dest_total++;

    if (dest->is_accepting_jobs)
        num_dest_accepting_jobs++;

    if (dest->is_shared)
        num_dest_shared++;

    switch (dest->state) {
        case 3:
            num_dest_idle++;
            break;
        case 4:
            num_dest_printing++;
            break;
        case 5:
            num_dest_stopped++;
            break;
        case INT_MIN:
            if (debug)
                fprintf(stderr, "printer state is missing for destination %s\n", dest->name);
            break;
        default:
            netdata_log_error("Unknown printer state (%d) found.", dest->state);
            break;
    }

    /*
     * Flag job metrics to print values. This reports destinations with zero
     * active jobs without invoking libcups destination discovery.
     */
    struct job_metrics *jm = get_job_metrics(dest->name);
    jm->is_collected = true;
}

static bool collect_scheduler_destinations(void) {
    static const char * const requested_attributes[] = {
        "printer-name",
        "printer-uri-supported",
        "printer-is-accepting-jobs",
        "printer-is-shared",
        "printer-state",
    };

    ipp_t *request = ippNewRequest(IPP_OP_CUPS_GET_PRINTERS);
    if (unlikely(!request))
        return false;

    ippAddStrings(
        request,
        IPP_TAG_OPERATION,
        IPP_TAG_KEYWORD,
        "requested-attributes",
        sizeof(requested_attributes) / sizeof(requested_attributes[0]),
        NULL,
        requested_attributes);
    ippAddString(request, IPP_TAG_OPERATION, IPP_TAG_NAME, "requesting-user-name", NULL, cupsUser());

    ipp_t *response = cupsDoRequest(http, request, "/");
    if (unlikely(!response))
        return false;

    ipp_status_t status = ippGetStatusCode(response);
    if (unlikely(status >= IPP_STATUS_REDIRECTION_OTHER_SITE)) {
        ippDelete(response);
        return false;
    }

    ipp_attribute_t *attr = ippFirstAttribute(response);
    while (attr) {
        while (attr && ippGetGroupTag(attr) != IPP_TAG_PRINTER)
            attr = ippNextAttribute(response);

        if (!attr)
            break;

        struct destination_metrics dest;
        reset_destination_metrics(&dest);

        for (; attr && ippGetGroupTag(attr) == IPP_TAG_PRINTER; attr = ippNextAttribute(response)) {
            const char *name = ippGetName(attr);
            if (!name)
                continue;

            ipp_tag_t value_tag = ippGetValueTag(attr);

            if (!strcmp(name, "printer-name") && value_tag == IPP_TAG_NAME)
                dest.name = ippGetString(attr, 0, NULL);
            else if (!strcmp(name, "printer-uri-supported") && value_tag == IPP_TAG_URI) {
                const char *uri = ippGetString(attr, 0, NULL);
                dest.has_uri = uri && *uri;
            }
            else if (!strcmp(name, "printer-is-accepting-jobs") && value_tag == IPP_TAG_BOOLEAN)
                dest.is_accepting_jobs = ippGetBoolean(attr, 0);
            else if (!strcmp(name, "printer-is-shared") && value_tag == IPP_TAG_BOOLEAN)
                dest.is_shared = ippGetBoolean(attr, 0);
            else if (!strcmp(name, "printer-state") && value_tag == IPP_TAG_ENUM)
                dest.state = ippGetInteger(attr, 0);
        }

        collect_destination_metrics(&dest);
    }

    ippDelete(response);
    return true;
}

void reset_metrics() {
    num_dest_total = 0;
    num_dest_accepting_jobs = 0;
    num_dest_shared = 0;

    num_dest_idle = 0;
    num_dest_printing = 0;
    num_dest_stopped = 0;

    reset_job_metrics(NULL, &global_job_metrics, NULL);
    dictionary_walkthrough_write(dict_dest_job_metrics, reset_job_metrics, NULL);
}

int main(int argc, char **argv) {
    nd_log_initialize_for_external_plugins("cups.plugin");
    if(nd_environment_freeze_process() != 0)
        fatal("Cannot freeze the process environment: %s", strerror(errno));
    netdata_threads_init_for_external_plugins(0);

    parse_command_line(argc, argv);

    errno_clear();

    dict_dest_job_metrics = dictionary_create_advanced(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct job_metrics));

    // ------------------------------------------------------------------------
    // the main loop

    if (debug)
        fprintf(stderr, "starting data collection\n");

    time_t started_t = now_monotonic_sec();
    size_t iteration = 0;

    heartbeat_t hb;
    heartbeat_init(&hb, netdata_update_every * USEC_PER_SEC);
    for (iteration = 0; 1; iteration++) {
        heartbeat_next(&hb);

        if (unlikely(exit_initiated_get()))
            break;

        reset_metrics();

        if (unlikely(!collect_scheduler_destinations())) {
            // reconnect to cups to check if the server is down.
            if (http)
                httpClose(http);
            http = httpConnect2(cupsServer(), ippPort(), NULL, AF_UNSPEC, cupsEncryption(), 0, netdata_update_every * 1000, NULL);
            if(http == NULL) {
                netdata_log_error("cups daemon is not running. Exiting!");
                exit(1);
            }
        }

        if (unlikely(exit_initiated_get()))
            break;

        cups_job_t *jobs, *curr_job;
        int num_jobs = cupsGetJobs2(http, &jobs, NULL, 0, CUPS_WHICHJOBS_ACTIVE);
        int i;
        for (i = num_jobs, curr_job = jobs; i > 0; i--, curr_job++) {
            struct job_metrics *jm = get_job_metrics(curr_job->dest);
            jm->is_collected = true;

            switch (curr_job->state) {
                case IPP_JOB_PENDING:
                    jm->num_pending++;
                    jm->size_pending += curr_job->size;
                    global_job_metrics.num_pending++;
                    global_job_metrics.size_pending += curr_job->size;
                    break;
                case IPP_JOB_HELD:
                    jm->num_held++;
                    jm->size_held += curr_job->size;
                    global_job_metrics.num_held++;
                    global_job_metrics.size_held += curr_job->size;
                    break;
                case IPP_JOB_PROCESSING:
                    jm->num_processing++;
                    jm->size_processing += curr_job->size;
                    global_job_metrics.num_processing++;
                    global_job_metrics.size_processing += curr_job->size;
                    break;
                default:
                    netdata_log_error("Unsupported job state (%u) found.", curr_job->state);
                    break;
            }
        }
        cupsFreeJobs(num_jobs, jobs);

        dictionary_walkthrough_write(dict_dest_job_metrics, send_job_metrics_to_netdata, NULL);
        dictionary_garbage_collect(dict_dest_job_metrics);

        static int cups_printer_by_option_created = 0;
        if (unlikely(!cups_printer_by_option_created))
        {
            cups_printer_by_option_created = 1;
            printf("CHART cups.dest_state '' 'Destinations by state' dests overview cups.dests_state stacked 100000 %i\n", netdata_update_every);
            printf("DIMENSION idle '' absolute 1 1\n");
            printf("DIMENSION printing '' absolute 1 1\n");
            printf("DIMENSION stopped '' absolute 1 1\n");

            printf("CHART cups.dest_option '' 'Destinations by option' dests overview cups.dests_option line 100001 %i\n", netdata_update_every);
            printf("DIMENSION total '' absolute 1 1\n");
            printf("DIMENSION acceptingjobs '' absolute 1 1\n");
            printf("DIMENSION shared '' absolute 1 1\n");

            printf("CHART cups.job_num '' 'Active jobs' jobs overview cups.job_num stacked 100002 %i\n", netdata_update_every);
            printf("DIMENSION pending '' absolute 1 1\n");
            printf("DIMENSION held '' absolute 1 1\n");
            printf("DIMENSION processing '' absolute 1 1\n");

            printf("CHART cups.job_size '' 'Active jobs size' KB overview cups.job_size stacked 100003 %i\n", netdata_update_every);
            printf("DIMENSION pending '' absolute 1 1\n");
            printf("DIMENSION held '' absolute 1 1\n");
            printf("DIMENSION processing '' absolute 1 1\n");
        }

        printf(
            "BEGIN cups.dest_state\n"
            "SET idle = %d\n"
            "SET printing = %d\n"
            "SET stopped = %d\n"
            "END\n",
            num_dest_idle, num_dest_printing, num_dest_stopped);
        printf(
            "BEGIN cups.dest_option\n"
            "SET total = %d\n"
            "SET acceptingjobs = %d\n"
            "SET shared = %d\n"
            "END\n",
            num_dest_total, num_dest_accepting_jobs, num_dest_shared);
        printf(
            "BEGIN cups.job_num\n"
            "SET pending = %d\n"
            "SET held = %d\n"
            "SET processing = %d\n"
            "END\n",
            global_job_metrics.num_pending, global_job_metrics.num_held, global_job_metrics.num_processing);
        printf(
            "BEGIN cups.job_size\n"
            "SET pending = %d\n"
            "SET held = %d\n"
            "SET processing = %d\n"
            "END\n",
            global_job_metrics.size_pending, global_job_metrics.size_held, global_job_metrics.size_processing);

        fflush(stdout);

        if (unlikely(exit_initiated_get()))
            break;

        // restart check (14400 seconds)
        if (!now_monotonic_sec() - started_t > 14400)
            break;

        fprintf(stdout, "\n");
        fflush(stdout);
        if (ferror(stdout) && errno == EPIPE) {
            netdata_log_error("error writing to stdout: EPIPE. Exiting...");
            return 1;
        }
    }

    httpClose(http);
    netdata_log_info("CUPS process exiting");
}
