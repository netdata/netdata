
#include "common.h"

void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

#ifdef HAVE_CUPS
#include <cups/cups.h>

static int debug = 0;

// TODO: Validate if we can use ARL to scan printer options.
// TODO: Define alarms for this plugin.
// TODO: Add configuration options to disable per charts / per printer charts etc.

void print_help() {
    fprintf(stderr,
        "\n"
        "netdata cups.plugin %s\n"
        "\n"
        "Copyright (C) 2017 Simon Nagl <simonnagl@aim.com>\n"
        "Released under GNU General Public License v3 or later.\n"
        "All rights reserved.\n"
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
        "\n"
        , VERSION
    );
}

int main(int argc, char **argv) {

    int i, freq = 0;
    for(i = 1; i < argc ; i++) {
        if(isdigit(*argv[i]) && !freq) {
            if(freq) {
                fprintf(stderr, "Invalid command line option '%s'\n", argv[i]);
            } else {
                int n = atoi(argv[i]);
                if(n > 0 && freq < 86400) {
                    freq = n;
                    continue;
                }
            }
        }
        else if(strcmp("-v", argv[i]) == 0) {
            printf("cups.plugin %s\n", VERSION);
            exit(0);
        }
        else if(strcmp("-d", argv[i]) == 0) {
            debug = 1;
            continue;
        }
        else if(strcmp("-h", argv[i]) == 0) {
            print_help();
            exit(0);
        }

        print_help();
        exit(1);
    }

    int update_every = 1;
    if(freq != 0) {
        update_every = freq;
    }

    // TODO: Check if cups is enabled

    int num_dests; // Number of CUPS destiantions.
    int num_accepting_jobs;
    int num_shared;
    int num_idle;
    int num_printing;
    int num_stopped;
    int num_cups_printer_3D;
    int num_CUPS_PRINTER_AUTHENTICATED;
    int num_CUPS_PRINTER_BIND;
    int num_CUPS_PRINTER_BW;
    int num_CUPS_PRINTER_CLASS;
    int num_CUPS_PRINTER_COLLATE;
    int num_CUPS_PRINTER_COLOR;
    int num_CUPS_PRINTER_COMMANDS;
    int num_CUPS_PRINTER_COPIES;
    int num_CUPS_PRINTER_COVER;
    int num_CUPS_PRINTER_DEFAULT;
    int num_CUPS_PRINTER_DELETE;
    int num_CUPS_PRINTER_DUPLEX;
    int num_CUPS_PRINTER_FAX;
    int num_CUPS_PRINTER_LARGE;
    int num_CUPS_PRINTER_LOCAL;
    int num_CUPS_PRINTER_MEDIUM;
    int num_CUPS_PRINTER_MFP;
    int num_CUPS_PRINTER_NOT_SHARED;
    int num_CUPS_PRINTER_PUNCH;
    int num_CUPS_PRINTER_REJECTING;
    int num_CUPS_PRINTER_REMOTE;
    int num_CUPS_PRINTER_SCANNER;
    int num_CUPS_PRINTER_SMALL;
    int num_CUPS_PRINTER_SORT;
    int num_CUPS_PRINTER_STAPLE;
    int num_CUPS_PRINTER_VARIABLE;
    int num_jobs_total;
    unsigned long long job_size; // in bytes

    usec_t duration = 0;
    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    for (;;)
    {
        duration = heartbeat_dt_usec(&hb);
        /* usec_t hb_dt = */ heartbeat_next(&hb, step);

        if (unlikely(netdata_exit))
            break;

        cups_dest_t *dests;
        num_dests = cupsGetDests(&dests);

        static int i;
        static cups_dest_t *curr_dest;
        for (i = num_dests, curr_dest = dests; i > 0; i--, curr_dest++)
        {
            const char *value = cupsGetOption("printer-is-accepting-jobs", curr_dest->num_options, curr_dest->options);
            if (strcmp("true", value))
            {
                num_accepting_jobs++;
            }

            const char *value2 = cupsGetOption("printer-is-shared", curr_dest->num_options, curr_dest->options);
            if (strcmp("true", value2))
            {
                num_shared++;
            }

            const char *value3 = cupsGetOption("printer-state", curr_dest->num_options, curr_dest->options);
            int val = str2i(value3);
            switch (val)
            {
            case 3:
                num_idle++;
                break;
            case 4:
                num_printing++;
                break;
            case 5:
                num_stopped++;
                break;
            default:
                error("Unknown printer state (%d) found.", val);
                break;
            }

            const char *value4 = cupsGetOption("printer-is-shared", curr_dest->num_options, curr_dest->options);
            // TODO implement collection

            // TODO Generate chart for printer.

            unsigned long long size_total;
            int num_pending;
            unsigned long long size_pending;
            int num_held;
            unsigned long long size_held;
            int num_processing;
            unsigned long long size_processing;
            int num_stopped;
            unsigned long long size_stopped;
            int num_canceled;
            unsigned long long size_canceled;
            int num_aborted;
            unsigned long long size_aborted;
            int num_completed;
            unsigned long long size_completed;

            cups_job_t *jobs, *curr_job;
            int num_jobs = cupsGetJobs(&jobs, curr_dest->name, 0, CUPS_WHICHJOBS_ALL);
            for (i = num_jobs, curr_job = jobs; i > 0; i--, curr_job++)
            {
                size_total = +curr_job->size;

                switch (curr_job->state)
                {
                case IPP_JOB_PENDING:
                    num_pending++;
                    size_pending = +curr_job->size;
                    break;
                case IPP_JOB_HELD:
                    num_held++;
                    size_held = +curr_job->size;
                    break;
                case IPP_JOB_PROCESSING:
                    num_processing++;
                    size_processing = +curr_job->size;
                    break;
                case IPP_JOB_STOPPED:
                    num_stopped++;
                    size_stopped = +curr_job->size;
                    break;
                case IPP_JOB_CANCELED:
                    num_canceled++;
                    size_canceled = +curr_job->size;
                    break;
                case IPP_JOB_ABORTED:
                    num_aborted++;
                    size_aborted = +curr_job->size;
                    break;
                case IPP_JOB_COMPLETED:
                    num_completed++;
                    size_completed = +curr_job->size;
                    break;
                }

                num_jobs_total = +num_jobs;
                job_size = +size_total;

                // TODO validate if we should display priority
                // TODO validate how to collect time_t completed_time, time_t creation_time, processing_time
                // TODO upgrade dimension format.

                // TODO Add per printer charts
            }
        }

        /// Todo add more charts

        static int cups_jobs_chart_created = 0;

        if (unlikely(!cups_jobs_chart_created)) {
            cups_jobs_chart_created = 1;
            printf("CHART cups.jobs '' 'Total CUPS job number' jobs jobs cups line 3000 1\n");
    
            printf("DIMENSION jobs '' absolute 1 1\n");
        }

        printf(
            "BEGIN cups.jobs\n"
            "SET jobs = %zu\n"
            "END\n"
            , num_jobs_total
        );

        fflush(stdout);        

        if (unlikely(netdata_exit))
            break;
    }

    info("CUPS process exiting");
}

#else // !HAVE_CUPS

int main(int argc, char **argv) {
    fatal("cups.plugin is not compiled");
    return 0;
}

#endif // !HAVE_CUPS