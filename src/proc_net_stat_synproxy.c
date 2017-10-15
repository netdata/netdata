#include "common.h"

#define RRD_TYPE_NET_STAT_NETFILTER         "netfilter"
#define RRD_TYPE_NET_STAT_SYNPROXY          "synproxy"

int do_proc_net_stat_synproxy(int update_every, usec_t dt) {
    (void)dt;

    static int do_entries = -1, do_cookies = -1, do_syns = -1, do_reopened = -1;
    static procfile *ff = NULL;

    if(unlikely(do_entries == -1)) {
        do_entries  = config_get_boolean_ondemand("plugin:proc:/proc/net/stat/synproxy", "SYNPROXY entries", CONFIG_BOOLEAN_AUTO);
        do_cookies  = config_get_boolean_ondemand("plugin:proc:/proc/net/stat/synproxy", "SYNPROXY cookies", CONFIG_BOOLEAN_AUTO);
        do_syns     = config_get_boolean_ondemand("plugin:proc:/proc/net/stat/synproxy", "SYNPROXY SYN received", CONFIG_BOOLEAN_AUTO);
        do_reopened = config_get_boolean_ondemand("plugin:proc:/proc/net/stat/synproxy", "SYNPROXY connections reopened", CONFIG_BOOLEAN_AUTO);
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/stat/synproxy");
        ff = procfile_open(config_get("plugin:proc:/proc/net/stat/synproxy", "filename to monitor", filename), " \t,:|", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff))
            return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return 0; // we return 0, so that we will retry to open it next time

    // make sure we have 3 lines
    size_t lines = procfile_lines(ff), l;
    if(unlikely(lines < 2)) {
        error("/proc/net/stat/synproxy has %zu lines, expected no less than 2. Disabling it.", lines);
        return 1;
    }

    unsigned long long entries = 0, syn_received = 0, cookie_invalid = 0, cookie_valid = 0, cookie_retrans = 0, conn_reopened = 0;

    // synproxy gives its values per CPU
    for(l = 1; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(words < 6))
            continue;

        entries         += strtoull(procfile_lineword(ff, l, 0), NULL, 16);
        syn_received    += strtoull(procfile_lineword(ff, l, 1), NULL, 16);
        cookie_invalid  += strtoull(procfile_lineword(ff, l, 2), NULL, 16);
        cookie_valid    += strtoull(procfile_lineword(ff, l, 3), NULL, 16);
        cookie_retrans  += strtoull(procfile_lineword(ff, l, 4), NULL, 16);
        conn_reopened   += strtoull(procfile_lineword(ff, l, 5), NULL, 16);
    }

    unsigned long long events = entries + syn_received + cookie_invalid + cookie_valid + cookie_retrans + conn_reopened;

    // --------------------------------------------------------------------

    if((do_entries == CONFIG_BOOLEAN_AUTO && events) || do_entries == CONFIG_BOOLEAN_YES) {
        do_entries = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_STAT_NETFILTER
                    , RRD_TYPE_NET_STAT_SYNPROXY "_entries"
                    , NULL
                    , RRD_TYPE_NET_STAT_SYNPROXY
                    , NULL
                    , "SYNPROXY Entries Used"
                    , "entries"
                    , "proc"
                    , "net/stat/synproxy"
                    , 3304
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "entries", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set(st, "entries", entries);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if((do_syns == CONFIG_BOOLEAN_AUTO && events) || do_syns == CONFIG_BOOLEAN_YES) {
        do_syns = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_STAT_NETFILTER
                    , RRD_TYPE_NET_STAT_SYNPROXY "_syn_received"
                    , NULL
                    , RRD_TYPE_NET_STAT_SYNPROXY
                    , NULL
                    , "SYNPROXY SYN Packets received"
                    , "SYN/s"
                    , "proc"
                    , "net/stat/synproxy"
                    , 3301
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "received", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "received", syn_received);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if((do_reopened == CONFIG_BOOLEAN_AUTO && events) || do_reopened == CONFIG_BOOLEAN_YES) {
        do_reopened = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_STAT_NETFILTER
                    , RRD_TYPE_NET_STAT_SYNPROXY "_conn_reopened"
                    , NULL
                    , RRD_TYPE_NET_STAT_SYNPROXY
                    , NULL
                    , "SYNPROXY Connections Reopened"
                    , "connections/s"
                    , "proc"
                    , "net/stat/synproxy"
                    , 3303
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "reopened", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "reopened", conn_reopened);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if((do_cookies == CONFIG_BOOLEAN_AUTO && events) || do_cookies == CONFIG_BOOLEAN_YES) {
        do_cookies = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    RRD_TYPE_NET_STAT_NETFILTER
                    , RRD_TYPE_NET_STAT_SYNPROXY "_cookies"
                    , NULL
                    , RRD_TYPE_NET_STAT_SYNPROXY
                    , NULL
                    , "SYNPROXY TCP Cookies"
                    , "cookies/s"
                    , "proc"
                    , "net/stat/synproxy"
                    , 3302
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rrddim_add(st, "valid", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "invalid", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "retransmits", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "valid", cookie_valid);
        rrddim_set(st, "invalid", cookie_invalid);
        rrddim_set(st, "retransmits", cookie_retrans);
        rrdset_done(st);
    }

    return 0;
}
