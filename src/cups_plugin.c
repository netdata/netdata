
#include "common.h"

void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

#ifdef HAVE_CUPS
#include <cups/cups.h>

static int debug = 0;

static int update_every = 1;

// TODO: Validate if we can use ARL to scan printer options.
// TODO: Define alarms for this plugin.

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

void parse_command_line(int argc, char **argv) {
    int i;
    int update_every_found = 0;
    for(i = 1; i < argc ; i++) {
        if(isdigit(*argv[i]) && !update_every_found) {
            int n = atoi(argv[i]);
            if(n > 0) {
                update_every = n;
                continue;
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
}

int main(int argc, char **argv) {
    parse_command_line(argc, argv);

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

        if (unlikely(netdata_exit)) {
            break;
        }

        num_accepting_jobs = 0;
        num_shared = 0;
        num_idle = 0;
        num_printing = 0;
        num_stopped = 0;
    

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

            // TODO implement collection

            // TODO Generate chart for printer.

            unsigned long long size_total = 0;
            int                num_pending = 0;
            unsigned long long size_pending = 0;
            int                num_held = 0;
            unsigned long long size_held = 0;
            int                num_processing = 0;
            unsigned long long size_processing = 0;
            int                num_stopped = 0;
            unsigned long long size_stopped = 0;
            int                num_canceled = 0;
            unsigned long long size_canceled = 0;
            int                num_aborted = 0;
            unsigned long long size_aborted = 0;
            int                num_completed = 0;
            unsigned long long size_completed = 0;

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

            static int cups_chart_created = 0;
            
                            if (unlikely(!cups_chart_created)) {
                                cups_chart_created = 1;
                                printf("CHART cups.%s '' 'Jobs of %s' printer printer cups line 2999 1\n", curr_dest->name, curr_dest->name);        
            
                                printf("DIMENSION pending '' absolute 1 1\n");
                                printf("DIMENSION held '' absolute 1 1\n");
                                printf("DIMENSION processing '' absolute 1 1\n");
                                printf("DIMENSION stopped '' absolute 1 1\n");
                                printf("DIMENSION canceled '' absolute 1 1\n");
                                printf("DIMENSION aborted '' absolute 1 1\n");
                                printf("DIMENSION completed '' absolute 1 1\n");
                    
                            }
                        
                            printf(
                                "BEGIN cups.%s\n"
                                "SET pending = %d\n"
                                "SET held = %d\n"
                                "SET processing = %d\n"
                                "SET stopped = %d\n"
                                "SET canceled = %d\n"
                                "SET aborted = %d\n"
                                "SET completed = %d\n"
                                "END\n"
                                , curr_dest->name
                                , num_pending
                                , num_held
                                , num_processing
                                , num_stopped
                                , num_canceled
                                , num_aborted
                                , num_completed
                            );
        }

        /// Todo add more charts

        static int cups_jobs_chart_created = 0;
        static int cups_printer_by_option_created = 0;

        if (unlikely(!cups_printer_by_option_created)) {
            cups_printer_by_option_created = 1;
            printf("CHART cups.printer_by_option '' 'CUPS Printers by option' printer printer cups line 3000 1\n");
    
            printf("DIMENSION accepting_jobs '' absolute 1 1\n");
            printf("DIMENSION shared '' absolute 1 1\n");
            printf("DIMENSION idle '' absolute 1 1\n");
            printf("DIMENSION printing '' absolute 1 1\n");
            printf("DIMENSION stopped '' absolute 1 1\n");

        }

        printf(
            "BEGIN cups.printer_by_option\n"
            "SET accepting_jobs = %d\n"
            "SET shared = %d\n"
            "SET idle = %d\n"
            "SET printing = %d\n"
            "SET stopped = %d\n"
            "END\n"
            , num_accepting_jobs
            , num_shared
            , num_idle
            , num_printing
            , num_stopped
        );
       

        if (unlikely(!cups_jobs_chart_created)) {
            cups_jobs_chart_created = 1;
            printf("CHART cups.jobs '' 'Total CUPS job number' jobs jobs cups line 3001 1\n");
    
            printf("DIMENSION jobs '' absolute 1 1\n");
        }

        printf(
            "BEGIN cups.jobs\n"
            "SET jobs = %d\n"
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