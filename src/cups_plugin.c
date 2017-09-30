

#include "common.h"

void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

#ifdef HAVE_CUPS
#include <cups/cups.h>

// TODO: Validate if we can use ARL to scan printer options.
// TODO: Define alarms for this plugin.
// TODO: Add configuration options to disable per charts / per printer charts etc.

int main(int argc, char **argv) {
    fatal("cups.plugin is compiled");
    return 0;
}

void *cups_main(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("CUPS thread created with task id %d", gettid());

    if (pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if (pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    // TODO: Check if cups is enabled

    int rrd_update_every =  1;
    int update_every = (int)config_get_number("plugin:cups", "update every", rrd_update_every);
    if (update_every < rrd_update_every)
    {
        update_every = rrd_update_every;
    }

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
                error("Loop dest");

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

        RRDSET *st = rrdset_find_byname_localhost("cups.jobs");
        if (unlikely(!st))
        {
            st = rrdset_create_localhost("cups", "jobs", NULL, "jobs", NULL, "Total CUPS job number", "jobs",
                               3001, update_every, RRDSET_TYPE_LINE);

            rrddim_add(st, "jobs", "jobs", 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            rrdset_next(st);

        rrddim_set(st, "jobs", (collected_number) num_jobs_total);
        rrdset_done(st);

        if (unlikely(netdata_exit))
            break;
    }

    info("CUPS thread exiting");

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}

#else // !HAVE_CUPS

int main(int argc, char **argv) {
    fatal("cups.plugin is not compiled");
    return 0;
}

#endif // !HAVE_CUPS