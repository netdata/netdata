#include "common.h"

#define RRD_TYPE_NET_STAT_NETFILTER     "netfilter"
#define RRD_TYPE_NET_STAT_CONNTRACK     "conntrack"

int do_proc_net_stat_conntrack(int update_every, usec_t dt) {
    static procfile *ff = NULL;
    static int do_sockets = -1, do_new = -1, do_changes = -1, do_expect = -1, do_search = -1, do_errors = -1;
    static usec_t get_max_every = 10 * USEC_PER_SEC, usec_since_last_max = 0;
    static int read_full = 1;
    static char *nf_conntrack_filename, *nf_conntrack_count_filename, *nf_conntrack_max_filename;
    static RRDVAR *rrdvar_max = NULL;

    unsigned long long aentries = 0, asearched = 0, afound = 0, anew = 0, ainvalid = 0, aignore = 0, adelete = 0, adelete_list = 0,
            ainsert = 0, ainsert_failed = 0, adrop = 0, aearly_drop = 0, aicmp_error = 0, aexpect_new = 0, aexpect_create = 0, aexpect_delete = 0, asearch_restart = 0;

    if(unlikely(do_sockets == -1)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/stat/nf_conntrack");
        nf_conntrack_filename = config_get("plugin:proc:/proc/net/stat/nf_conntrack", "filename to monitor", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/sys/net/netfilter/nf_conntrack_max");
        nf_conntrack_max_filename = config_get("plugin:proc:/proc/sys/net/netfilter/nf_conntrack_max", "filename to monitor", filename);
        usec_since_last_max = get_max_every = config_get_number("plugin:proc:/proc/sys/net/netfilter/nf_conntrack_max", "read every seconds", 10) * USEC_PER_SEC;

        read_full = 1;
        ff = procfile_open(nf_conntrack_filename, " \t:", PROCFILE_FLAG_DEFAULT);
        if(!ff) read_full = 0;

        do_new = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter new connections", read_full);
        do_changes = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connection changes", read_full);
        do_expect = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connection expectations", read_full);
        do_search = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connection searches", read_full);
        do_errors = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter errors", read_full);

        do_sockets = 1;
        if(!read_full) {
            snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/sys/net/netfilter/nf_conntrack_count");
            nf_conntrack_count_filename = config_get("plugin:proc:/proc/sys/net/netfilter/nf_conntrack_count", "filename to monitor", filename);

            if(read_single_number_file(nf_conntrack_count_filename, &aentries))
                do_sockets = 0;
        }

        do_sockets = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connections", do_sockets);

        if(!do_sockets && !read_full)
            return 1;

        rrdvar_max = rrdvar_custom_host_variable_create(localhost, "netfilter.conntrack.max");
    }

    if(likely(read_full)) {
        if(unlikely(!ff)) {
            ff = procfile_open(nf_conntrack_filename, " \t:", PROCFILE_FLAG_DEFAULT);
            if(unlikely(!ff))
                return 0; // we return 0, so that we will retry to open it next time
        }

        ff = procfile_readall(ff);
        if(unlikely(!ff))
            return 0; // we return 0, so that we will retry to open it next time

        size_t lines = procfile_lines(ff), l;

        for(l = 1; l < lines ;l++) {
            size_t words = procfile_linewords(ff, l);
            if(unlikely(words < 17)) {
                if(unlikely(words)) error("Cannot read /proc/net/stat/nf_conntrack line. Expected 17 params, read %zu.", words);
                continue;
            }

            unsigned long long tentries = 0, tsearched = 0, tfound = 0, tnew = 0, tinvalid = 0, tignore = 0, tdelete = 0, tdelete_list = 0, tinsert = 0, tinsert_failed = 0, tdrop = 0, tearly_drop = 0, ticmp_error = 0, texpect_new = 0, texpect_create = 0, texpect_delete = 0, tsearch_restart = 0;

            tentries        = strtoull(procfile_lineword(ff, l, 0), NULL, 16);
            tsearched       = strtoull(procfile_lineword(ff, l, 1), NULL, 16);
            tfound          = strtoull(procfile_lineword(ff, l, 2), NULL, 16);
            tnew            = strtoull(procfile_lineword(ff, l, 3), NULL, 16);
            tinvalid        = strtoull(procfile_lineword(ff, l, 4), NULL, 16);
            tignore         = strtoull(procfile_lineword(ff, l, 5), NULL, 16);
            tdelete         = strtoull(procfile_lineword(ff, l, 6), NULL, 16);
            tdelete_list    = strtoull(procfile_lineword(ff, l, 7), NULL, 16);
            tinsert         = strtoull(procfile_lineword(ff, l, 8), NULL, 16);
            tinsert_failed  = strtoull(procfile_lineword(ff, l, 9), NULL, 16);
            tdrop           = strtoull(procfile_lineword(ff, l, 10), NULL, 16);
            tearly_drop     = strtoull(procfile_lineword(ff, l, 11), NULL, 16);
            ticmp_error     = strtoull(procfile_lineword(ff, l, 12), NULL, 16);
            texpect_new     = strtoull(procfile_lineword(ff, l, 13), NULL, 16);
            texpect_create  = strtoull(procfile_lineword(ff, l, 14), NULL, 16);
            texpect_delete  = strtoull(procfile_lineword(ff, l, 15), NULL, 16);
            tsearch_restart = strtoull(procfile_lineword(ff, l, 16), NULL, 16);

            if(unlikely(!aentries)) aentries =  tentries;

            // sum all the cpus together
            asearched           += tsearched;       // conntrack.search
            afound              += tfound;          // conntrack.search
            anew                += tnew;            // conntrack.new
            ainvalid            += tinvalid;        // conntrack.new
            aignore             += tignore;         // conntrack.new
            adelete             += tdelete;         // conntrack.changes
            adelete_list        += tdelete_list;    // conntrack.changes
            ainsert             += tinsert;         // conntrack.changes
            ainsert_failed      += tinsert_failed;  // conntrack.errors
            adrop               += tdrop;           // conntrack.errors
            aearly_drop         += tearly_drop;     // conntrack.errors
            aicmp_error         += ticmp_error;     // conntrack.errors
            aexpect_new         += texpect_new;     // conntrack.expect
            aexpect_create      += texpect_create;  // conntrack.expect
            aexpect_delete      += texpect_delete;  // conntrack.expect
            asearch_restart     += tsearch_restart; // conntrack.search
        }
    }
    else {
        if(unlikely(read_single_number_file(nf_conntrack_count_filename, &aentries)))
            return 0; // we return 0, so that we will retry to open it next time
    }

    usec_since_last_max += dt;
    if(unlikely(rrdvar_max && usec_since_last_max >= get_max_every)) {
        usec_since_last_max = 0;

        unsigned long long max;
        if(likely(!read_single_number_file(nf_conntrack_max_filename, &max)))
            rrdvar_custom_host_variable_set(localhost, rrdvar_max, max);
    }

    // --------------------------------------------------------------------

    if(do_sockets) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_connections = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_STAT_NETFILTER
                    , RRD_TYPE_NET_STAT_CONNTRACK "_sockets"
                    , NULL
                    , RRD_TYPE_NET_STAT_CONNTRACK
                    , NULL
                    , "Connection Tracker Connections"
                    , "active connections"
                    , "proc"
                    , "net/stat/nf_conntrack"
                    , 3000
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_connections = rrddim_add(st, "connections", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_connections, aentries);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_new) {
        static RRDSET *st = NULL;
        static RRDDIM
                *rd_new     = NULL,
                *rd_ignore  = NULL,
                *rd_invalid = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_STAT_NETFILTER
                    , RRD_TYPE_NET_STAT_CONNTRACK "_new"
                    , NULL
                    , RRD_TYPE_NET_STAT_CONNTRACK
                    , NULL
                    , "Connection Tracker New Connections"
                    , "connections/s"
                    , "proc"
                    , "net/stat/nf_conntrack"
                    , 3001
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_new     = rrddim_add(st, "new",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_ignore  = rrddim_add(st, "ignore",  NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_invalid = rrddim_add(st, "invalid", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_new,     anew);
        rrddim_set_by_pointer(st, rd_ignore,  aignore);
        rrddim_set_by_pointer(st, rd_invalid, ainvalid);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_changes) {
        static RRDSET *st = NULL;
        static RRDDIM
                *rd_inserted    = NULL,
                *rd_deleted     = NULL,
                *rd_delete_list = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_STAT_NETFILTER
                    , RRD_TYPE_NET_STAT_CONNTRACK "_changes"
                    , NULL
                    , RRD_TYPE_NET_STAT_CONNTRACK
                    , NULL
                    , "Connection Tracker Changes"
                    , "changes/s"
                    , "proc"
                    , "net/stat/nf_conntrack"
                    , 3002
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_inserted = rrddim_add(st, "inserted", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_deleted = rrddim_add(st, "deleted", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_delete_list = rrddim_add(st, "delete_list", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inserted, ainsert);
        rrddim_set_by_pointer(st, rd_deleted, adelete);
        rrddim_set_by_pointer(st, rd_delete_list, adelete_list);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_expect) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_created = NULL,
                      *rd_deleted = NULL,
                      *rd_new     = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_STAT_NETFILTER
                    , RRD_TYPE_NET_STAT_CONNTRACK "_expect"
                    , NULL
                    , RRD_TYPE_NET_STAT_CONNTRACK
                    , NULL
                    , "Connection Tracker Expectations"
                    , "expectations/s"
                    , "proc"
                    , "net/stat/nf_conntrack"
                    , 3003
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_created = rrddim_add(st, "created", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_deleted = rrddim_add(st, "deleted", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_new     = rrddim_add(st, "new",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_created, aexpect_create);
        rrddim_set_by_pointer(st, rd_deleted, aexpect_delete);
        rrddim_set_by_pointer(st, rd_new,     aexpect_new);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_search) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_searched  = NULL,
                      *rd_restarted = NULL,
                      *rd_found     = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_STAT_NETFILTER
                    , RRD_TYPE_NET_STAT_CONNTRACK "_search"
                    , NULL
                    , RRD_TYPE_NET_STAT_CONNTRACK
                    , NULL
                    , "Connection Tracker Searches"
                    , "searches/s"
                    , "proc"
                    , "net/stat/nf_conntrack"
                    , 3010
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_searched  = rrddim_add(st, "searched",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_restarted = rrddim_add(st, "restarted", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_found     = rrddim_add(st, "found",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_searched,  asearched);
        rrddim_set_by_pointer(st, rd_restarted, asearch_restart);
        rrddim_set_by_pointer(st, rd_found,     afound);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_errors) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_icmp_error    = NULL,
                      *rd_insert_failed = NULL,
                      *rd_drop          = NULL,
                      *rd_early_drop    = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_STAT_NETFILTER
                    , RRD_TYPE_NET_STAT_CONNTRACK "_errors"
                    , NULL
                    , RRD_TYPE_NET_STAT_CONNTRACK
                    , NULL
                    , "Connection Tracker Errors"
                    , "events/s"
                    , "proc"
                    , "net/stat/nf_conntrack"
                    , 3005
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rd_icmp_error    = rrddim_add(st, "icmp_error",    NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_insert_failed = rrddim_add(st, "insert_failed", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_drop          = rrddim_add(st, "drop",          NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_early_drop    = rrddim_add(st, "early_drop",    NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_icmp_error,    aicmp_error);
        rrddim_set_by_pointer(st, rd_insert_failed, ainsert_failed);
        rrddim_set_by_pointer(st, rd_drop,          adrop);
        rrddim_set_by_pointer(st, rd_early_drop,    aearly_drop);
        rrdset_done(st);
    }

    return 0;
}
