// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_NFSD_NAME "/proc/net/rpc/nfsd"

struct nfsd_procs {
    char name[30];
    unsigned long long value;
    int present;
    RRDDIM *rd;
};

struct nfsd_procs nfsd_proc2_values[] = {
        {  "null"    , 0ULL, 0, NULL}
        , {"getattr" , 0ULL, 0, NULL}
        , {"setattr" , 0ULL, 0, NULL}
        , {"root"    , 0ULL, 0, NULL}
        , {"lookup"  , 0ULL, 0, NULL}
        , {"readlink", 0ULL, 0, NULL}
        , {"read"    , 0ULL, 0, NULL}
        , {"wrcache" , 0ULL, 0, NULL}
        , {"write"   , 0ULL, 0, NULL}
        , {"create"  , 0ULL, 0, NULL}
        , {"remove"  , 0ULL, 0, NULL}
        , {"rename"  , 0ULL, 0, NULL}
        , {"link"    , 0ULL, 0, NULL}
        , {"symlink" , 0ULL, 0, NULL}
        , {"mkdir"   , 0ULL, 0, NULL}
        , {"rmdir"   , 0ULL, 0, NULL}
        , {"readdir" , 0ULL, 0, NULL}
        , {"fsstat"  , 0ULL, 0, NULL}
        ,

        /* termination */
        {  ""        , 0ULL, 0, NULL}
};

struct nfsd_procs nfsd_proc3_values[] = {
        {  "null"       , 0ULL, 0, NULL}
        , {"getattr"    , 0ULL, 0, NULL}
        , {"setattr"    , 0ULL, 0, NULL}
        , {"lookup"     , 0ULL, 0, NULL}
        , {"access"     , 0ULL, 0, NULL}
        , {"readlink"   , 0ULL, 0, NULL}
        , {"read"       , 0ULL, 0, NULL}
        , {"write"      , 0ULL, 0, NULL}
        , {"create"     , 0ULL, 0, NULL}
        , {"mkdir"      , 0ULL, 0, NULL}
        , {"symlink"    , 0ULL, 0, NULL}
        , {"mknod"      , 0ULL, 0, NULL}
        , {"remove"     , 0ULL, 0, NULL}
        , {"rmdir"      , 0ULL, 0, NULL}
        , {"rename"     , 0ULL, 0, NULL}
        , {"link"       , 0ULL, 0, NULL}
        , {"readdir"    , 0ULL, 0, NULL}
        , {"readdirplus", 0ULL, 0, NULL}
        , {"fsstat"     , 0ULL, 0, NULL}
        , {"fsinfo"     , 0ULL, 0, NULL}
        , {"pathconf"   , 0ULL, 0, NULL}
        , {"commit"     , 0ULL, 0, NULL}
        ,

        /* termination */
        {  ""           , 0ULL, 0, NULL}
};

struct nfsd_procs nfsd_proc4_values[] = {
        {  "null"            , 0ULL, 0, NULL}
        , {"read"            , 0ULL, 0, NULL}
        , {"write"           , 0ULL, 0, NULL}
        , {"commit"          , 0ULL, 0, NULL}
        , {"open"            , 0ULL, 0, NULL}
        , {"open_conf"       , 0ULL, 0, NULL}
        , {"open_noat"       , 0ULL, 0, NULL}
        , {"open_dgrd"       , 0ULL, 0, NULL}
        , {"close"           , 0ULL, 0, NULL}
        , {"setattr"         , 0ULL, 0, NULL}
        , {"fsinfo"          , 0ULL, 0, NULL}
        , {"renew"           , 0ULL, 0, NULL}
        , {"setclntid"       , 0ULL, 0, NULL}
        , {"confirm"         , 0ULL, 0, NULL}
        , {"lock"            , 0ULL, 0, NULL}
        , {"lockt"           , 0ULL, 0, NULL}
        , {"locku"           , 0ULL, 0, NULL}
        , {"access"          , 0ULL, 0, NULL}
        , {"getattr"         , 0ULL, 0, NULL}
        , {"lookup"          , 0ULL, 0, NULL}
        , {"lookup_root"     , 0ULL, 0, NULL}
        , {"remove"          , 0ULL, 0, NULL}
        , {"rename"          , 0ULL, 0, NULL}
        , {"link"            , 0ULL, 0, NULL}
        , {"symlink"         , 0ULL, 0, NULL}
        , {"create"          , 0ULL, 0, NULL}
        , {"pathconf"        , 0ULL, 0, NULL}
        , {"statfs"          , 0ULL, 0, NULL}
        , {"readlink"        , 0ULL, 0, NULL}
        , {"readdir"         , 0ULL, 0, NULL}
        , {"server_caps"     , 0ULL, 0, NULL}
        , {"delegreturn"     , 0ULL, 0, NULL}
        , {"getacl"          , 0ULL, 0, NULL}
        , {"setacl"          , 0ULL, 0, NULL}
        , {"fs_locations"    , 0ULL, 0, NULL}
        , {"rel_lkowner"     , 0ULL, 0, NULL}
        , {"secinfo"         , 0ULL, 0, NULL}
        , {"fsid_present"    , 0ULL, 0, NULL}
        ,

        /* nfsv4.1 client ops */
        {  "exchange_id"     , 0ULL, 0, NULL}
        , {"create_session"  , 0ULL, 0, NULL}
        , {"destroy_session" , 0ULL, 0, NULL}
        , {"sequence"        , 0ULL, 0, NULL}
        , {"get_lease_time"  , 0ULL, 0, NULL}
        , {"reclaim_comp"    , 0ULL, 0, NULL}
        , {"layoutget"       , 0ULL, 0, NULL}
        , {"getdevinfo"      , 0ULL, 0, NULL}
        , {"layoutcommit"    , 0ULL, 0, NULL}
        , {"layoutreturn"    , 0ULL, 0, NULL}
        , {"secinfo_no"      , 0ULL, 0, NULL}
        , {"test_stateid"    , 0ULL, 0, NULL}
        , {"free_stateid"    , 0ULL, 0, NULL}
        , {"getdevicelist"   , 0ULL, 0, NULL}
        , {"bind_conn_to_ses", 0ULL, 0, NULL}
        , {"destroy_clientid", 0ULL, 0, NULL}
        ,

        /* nfsv4.2 client ops */
        {  "seek"            , 0ULL, 0, NULL}
        , {"allocate"        , 0ULL, 0, NULL}
        , {"deallocate"      , 0ULL, 0, NULL}
        , {"layoutstats"     , 0ULL, 0, NULL}
        , {"clone"           , 0ULL, 0, NULL}
        ,

        /* termination */
        {  ""                , 0ULL, 0, NULL}
};

struct nfsd_procs nfsd4_ops_values[] = {
        {  "unused_op0"          , 0ULL, 0, NULL}
        , {"unused_op1"          , 0ULL, 0, NULL}
        , {"future_op2"          , 0ULL, 0, NULL}
        , {"access"              , 0ULL, 0, NULL}
        , {"close"               , 0ULL, 0, NULL}
        , {"commit"              , 0ULL, 0, NULL}
        , {"create"              , 0ULL, 0, NULL}
        , {"delegpurge"          , 0ULL, 0, NULL}
        , {"delegreturn"         , 0ULL, 0, NULL}
        , {"getattr"             , 0ULL, 0, NULL}
        , {"getfh"               , 0ULL, 0, NULL}
        , {"link"                , 0ULL, 0, NULL}
        , {"lock"                , 0ULL, 0, NULL}
        , {"lockt"               , 0ULL, 0, NULL}
        , {"locku"               , 0ULL, 0, NULL}
        , {"lookup"              , 0ULL, 0, NULL}
        , {"lookup_root"         , 0ULL, 0, NULL}
        , {"nverify"             , 0ULL, 0, NULL}
        , {"open"                , 0ULL, 0, NULL}
        , {"openattr"            , 0ULL, 0, NULL}
        , {"open_confirm"        , 0ULL, 0, NULL}
        , {"open_downgrade"      , 0ULL, 0, NULL}
        , {"putfh"               , 0ULL, 0, NULL}
        , {"putpubfh"            , 0ULL, 0, NULL}
        , {"putrootfh"           , 0ULL, 0, NULL}
        , {"read"                , 0ULL, 0, NULL}
        , {"readdir"             , 0ULL, 0, NULL}
        , {"readlink"            , 0ULL, 0, NULL}
        , {"remove"              , 0ULL, 0, NULL}
        , {"rename"              , 0ULL, 0, NULL}
        , {"renew"               , 0ULL, 0, NULL}
        , {"restorefh"           , 0ULL, 0, NULL}
        , {"savefh"              , 0ULL, 0, NULL}
        , {"secinfo"             , 0ULL, 0, NULL}
        , {"setattr"             , 0ULL, 0, NULL}
        , {"setclientid"         , 0ULL, 0, NULL}
        , {"setclientid_confirm" , 0ULL, 0, NULL}
        , {"verify"              , 0ULL, 0, NULL}
        , {"write"               , 0ULL, 0, NULL}
        , {"release_lockowner"   , 0ULL, 0, NULL}
        ,

        /* nfs41 */
        {  "backchannel_ctl"     , 0ULL, 0, NULL}
        , {"bind_conn_to_session", 0ULL, 0, NULL}
        , {"exchange_id"         , 0ULL, 0, NULL}
        , {"create_session"      , 0ULL, 0, NULL}
        , {"destroy_session"     , 0ULL, 0, NULL}
        , {"free_stateid"        , 0ULL, 0, NULL}
        , {"get_dir_delegation"  , 0ULL, 0, NULL}
        , {"getdeviceinfo"       , 0ULL, 0, NULL}
        , {"getdevicelist"       , 0ULL, 0, NULL}
        , {"layoutcommit"        , 0ULL, 0, NULL}
        , {"layoutget"           , 0ULL, 0, NULL}
        , {"layoutreturn"        , 0ULL, 0, NULL}
        , {"secinfo_no_name"     , 0ULL, 0, NULL}
        , {"sequence"            , 0ULL, 0, NULL}
        , {"set_ssv"             , 0ULL, 0, NULL}
        , {"test_stateid"        , 0ULL, 0, NULL}
        , {"want_delegation"     , 0ULL, 0, NULL}
        , {"destroy_clientid"    , 0ULL, 0, NULL}
        , {"reclaim_complete"    , 0ULL, 0, NULL}
        ,

        /* nfs42 */
        {  "allocate"            , 0ULL, 0, NULL}
        , {"copy"                , 0ULL, 0, NULL}
        , {"copy_notify"         , 0ULL, 0, NULL}
        , {"deallocate"          , 0ULL, 0, NULL}
        , {"ioadvise"            , 0ULL, 0, NULL}
        , {"layouterror"         , 0ULL, 0, NULL}
        , {"layoutstats"         , 0ULL, 0, NULL}
        , {"offload_cancel"      , 0ULL, 0, NULL}
        , {"offload_status"      , 0ULL, 0, NULL}
        , {"read_plus"           , 0ULL, 0, NULL}
        , {"seek"                , 0ULL, 0, NULL}
        , {"write_same"          , 0ULL, 0, NULL}
        ,

        /* termination */
        {  ""                    , 0ULL, 0, NULL}
};


int do_proc_net_rpc_nfsd(int update_every, usec_t dt) {
    (void)dt;
    static procfile *ff = NULL;
    static int do_rc = -1, do_fh = -1, do_io = -1, do_th = -1, do_net = -1, do_rpc = -1, do_proc2 = -1, do_proc3 = -1, do_proc4 = -1, do_proc4ops = -1;
    static int proc2_warning = 0, proc3_warning = 0, proc4_warning = 0, proc4ops_warning = 0;

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/rpc/nfsd");
        ff = procfile_open(inicfg_get(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    if(unlikely(do_rc == -1)) {
        do_rc = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "read cache", 1);
        do_fh = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "file handles", 1);
        do_io = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "I/O", 1);
        do_th = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "threads", 1);
        do_net = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "network", 1);
        do_rpc = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "rpc", 1);
        do_proc2 = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "NFS v2 procedures", 1);
        do_proc3 = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "NFS v3 procedures", 1);
        do_proc4 = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "NFS v4 procedures", 1);
        do_proc4ops = inicfg_get_boolean(&netdata_config, "plugin:proc:/proc/net/rpc/nfsd", "NFS v4 operations", 1);
    }

    // if they are enabled, reset them to 1
    // later we do them = 2 to avoid doing strcmp() for all lines
    if(do_rc) do_rc = 1;
    if(do_fh) do_fh = 1;
    if(do_io) do_io = 1;
    if(do_th) do_th = 1;
    if(do_net) do_net = 1;
    if(do_rpc) do_rpc = 1;
    if(do_proc2) do_proc2 = 1;
    if(do_proc3) do_proc3 = 1;
    if(do_proc4) do_proc4 = 1;
    if(do_proc4ops) do_proc4ops = 1;

    size_t lines = procfile_lines(ff), l;

    char *type;
    unsigned long long rc_hits = 0, rc_misses = 0, rc_nocache = 0;
    unsigned long long fh_stale = 0;
    unsigned long long io_read = 0, io_write = 0;
    unsigned long long th_threads = 0;
    unsigned long long net_count = 0, net_udp_count = 0, net_tcp_count = 0, net_tcp_connections = 0;
    unsigned long long rpc_calls = 0, rpc_bad_format = 0, rpc_bad_auth = 0, rpc_bad_client = 0;

    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(unlikely(!words)) continue;

        type = procfile_lineword(ff, l, 0);

        if(do_rc == 1 && strcmp(type, "rc") == 0) {
            if(unlikely(words < 4)) {
                collector_error("%s line of /proc/net/rpc/nfsd has %zu words, expected %d", type, words, 4);
                continue;
            }

            rc_hits = str2ull(procfile_lineword(ff, l, 1), NULL);
            rc_misses = str2ull(procfile_lineword(ff, l, 2), NULL);
            rc_nocache = str2ull(procfile_lineword(ff, l, 3), NULL);

            unsigned long long sum = rc_hits + rc_misses + rc_nocache;
            if(sum == 0ULL) do_rc = -1;
            else do_rc = 2;
        }
        else if(do_fh == 1 && strcmp(type, "fh") == 0) {
            if(unlikely(words < 6)) {
                collector_error("%s line of /proc/net/rpc/nfsd has %zu words, expected %d", type, words, 6);
                continue;
            }

            fh_stale = str2ull(procfile_lineword(ff, l, 1), NULL);
            
            // other file handler metrics were never used and are always zero

            if(fh_stale == 0ULL) do_fh = -1;
            else do_fh = 2;
        }
        else if(do_io == 1 && strcmp(type, "io") == 0) {
            if(unlikely(words < 3)) {
                collector_error("%s line of /proc/net/rpc/nfsd has %zu words, expected %d", type, words, 3);
                continue;
            }

            io_read = str2ull(procfile_lineword(ff, l, 1), NULL);
            io_write = str2ull(procfile_lineword(ff, l, 2), NULL);

            unsigned long long sum = io_read + io_write;
            if(sum == 0ULL) do_io = -1;
            else do_io = 2;
        }
        else if(do_th == 1 && strcmp(type, "th") == 0) {
            if(unlikely(words < 13)) {
                collector_error("%s line of /proc/net/rpc/nfsd has %zu words, expected %d", type, words, 13);
                continue;
            }

            th_threads = str2ull(procfile_lineword(ff, l, 1), NULL);

            // thread histogram has been disabled since 2009 (kernel 2.6.30)
            // https://git.kernel.org/pub/scm/linux/kernel/git/stable/linux.git/commit/?id=8bbfa9f3889b643fc7de82c0c761ef17097f8faf
            
            do_th = 2;
        }
        else if(do_net == 1 && strcmp(type, "net") == 0) {
            if(unlikely(words < 5)) {
                collector_error("%s line of /proc/net/rpc/nfsd has %zu words, expected %d", type, words, 5);
                continue;
            }

            net_count = str2ull(procfile_lineword(ff, l, 1), NULL);
            net_udp_count = str2ull(procfile_lineword(ff, l, 2), NULL);
            net_tcp_count = str2ull(procfile_lineword(ff, l, 3), NULL);
            net_tcp_connections = str2ull(procfile_lineword(ff, l, 4), NULL);

            unsigned long long sum = net_count + net_udp_count + net_tcp_count + net_tcp_connections;
            if(sum == 0ULL) do_net = -1;
            else do_net = 2;
        }
        else if(do_rpc == 1 && strcmp(type, "rpc") == 0) {
            if(unlikely(words < 6)) {
                collector_error("%s line of /proc/net/rpc/nfsd has %zu words, expected %d", type, words, 6);
                continue;
            }

            rpc_calls = str2ull(procfile_lineword(ff, l, 1), NULL);
            rpc_bad_format = str2ull(procfile_lineword(ff, l, 3), NULL);
            rpc_bad_auth = str2ull(procfile_lineword(ff, l, 4), NULL);
            rpc_bad_client = str2ull(procfile_lineword(ff, l, 5), NULL);

            unsigned long long sum = rpc_calls + rpc_bad_format + rpc_bad_auth + rpc_bad_client;
            if(sum == 0ULL) do_rpc = -1;
            else do_rpc = 2;
        }
        else if(do_proc2 == 1 && strcmp(type, "proc2") == 0) {
            // the first number is the count of numbers present
            // so we start for word 2

            unsigned long long sum = 0;
            unsigned int i, j;
            for(i = 0, j = 2; j < words && nfsd_proc2_values[i].name[0] ; i++, j++) {
                nfsd_proc2_values[i].value = str2ull(procfile_lineword(ff, l, j), NULL);
                nfsd_proc2_values[i].present = 1;
                sum += nfsd_proc2_values[i].value;
            }

            if(sum == 0ULL) {
                if(!proc2_warning) {
                    collector_error("Disabling /proc/net/rpc/nfsd v2 procedure calls chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
                    proc2_warning = 1;
                }
                do_proc2 = 0;
            }
            else do_proc2 = 2;
        }
        else if(do_proc3 == 1 && strcmp(type, "proc3") == 0) {
            // the first number is the count of numbers present
            // so we start for word 2

            unsigned long long sum = 0;
            unsigned int i, j;
            for(i = 0, j = 2; j < words && nfsd_proc3_values[i].name[0] ; i++, j++) {
                nfsd_proc3_values[i].value = str2ull(procfile_lineword(ff, l, j), NULL);
                nfsd_proc3_values[i].present = 1;
                sum += nfsd_proc3_values[i].value;
            }

            if(sum == 0ULL) {
                if(!proc3_warning) {
                    collector_info("Disabling /proc/net/rpc/nfsd v3 procedure calls chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
                    proc3_warning = 1;
                }
                do_proc3 = 0;
            }
            else do_proc3 = 2;
        }
        else if(do_proc4 == 1 && strcmp(type, "proc4") == 0) {
            // the first number is the count of numbers present
            // so we start for word 2

            unsigned long long sum = 0;
            unsigned int i, j;
            for(i = 0, j = 2; j < words && nfsd_proc4_values[i].name[0] ; i++, j++) {
                nfsd_proc4_values[i].value = str2ull(procfile_lineword(ff, l, j), NULL);
                nfsd_proc4_values[i].present = 1;
                sum += nfsd_proc4_values[i].value;
            }

            if(sum == 0ULL) {
                if(!proc4_warning) {
                    collector_info("Disabling /proc/net/rpc/nfsd v4 procedure calls chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
                    proc4_warning = 1;
                }
                do_proc4 = 0;
            }
            else do_proc4 = 2;
        }
        else if(do_proc4ops == 1 && strcmp(type, "proc4ops") == 0) {
            // the first number is the count of numbers present
            // so we start for word 2

            unsigned long long sum = 0;
            unsigned int i, j;
            for(i = 0, j = 2; j < words && nfsd4_ops_values[i].name[0] ; i++, j++) {
                nfsd4_ops_values[i].value = str2ull(procfile_lineword(ff, l, j), NULL);
                nfsd4_ops_values[i].present = 1;
                sum += nfsd4_ops_values[i].value;
            }

            if(sum == 0ULL) {
                if(!proc4ops_warning) {
                    collector_info("Disabling /proc/net/rpc/nfsd v4 operations chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
                    proc4ops_warning = 1;
                }
                do_proc4ops = 0;
            }
            else do_proc4ops = 2;
        }
    }

    if(do_rc == 2) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_hits    = NULL,
                      *rd_misses  = NULL,
                      *rd_nocache = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "nfsd"
                    , "readcache"
                    , NULL
                    , "cache"
                    , NULL
                    , "NFS Server Read Cache"
                    , "reads/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NFSD_NAME
                    , NETDATA_CHART_PRIO_NFSD_READCACHE
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_hits    = rrddim_add(st, "hits",    NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_misses  = rrddim_add(st, "misses",  NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_nocache = rrddim_add(st, "nocache", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_hits,    rc_hits);
        rrddim_set_by_pointer(st, rd_misses,  rc_misses);
        rrddim_set_by_pointer(st, rd_nocache, rc_nocache);
        rrdset_done(st);
    }

    if(do_fh == 2) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_stale                 = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "nfsd"
                    , "filehandles"
                    , NULL
                    , "filehandles"
                    , NULL
                    , "NFS Server File Handles"
                    , "handles/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NFSD_NAME
                    , NETDATA_CHART_PRIO_NFSD_FILEHANDLES
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_stale                 = rrddim_add(st, "stale",                 NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st, rd_stale,                 fh_stale);
        rrdset_done(st);
    }

    if(do_io == 2) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_read  = NULL,
                      *rd_write = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "nfsd"
                    , "io"
                    , NULL
                    , "io"
                    , NULL
                    , "NFS Server I/O"
                    , "kilobytes/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NFSD_NAME
                    , NETDATA_CHART_PRIO_NFSD_IO
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_read  = rrddim_add(st, "read",  NULL,  1, 1000, RRD_ALGORITHM_INCREMENTAL);
            rd_write = rrddim_add(st, "write", NULL, -1, 1000, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(st, rd_read,  io_read);
        rrddim_set_by_pointer(st, rd_write, io_write);
        rrdset_done(st);
    }

    if(do_th == 2) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_threads = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "nfsd"
                    , "threads"
                    , NULL
                    , "threads"
                    , NULL
                    , "NFS Server Threads"
                    , "threads"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NFSD_NAME
                    , NETDATA_CHART_PRIO_NFSD_THREADS
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_threads = rrddim_add(st, "threads", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        }

        rrddim_set_by_pointer(st, rd_threads, th_threads);
        rrdset_done(st);
    }

    if(do_net == 2) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_udp = NULL,
                      *rd_tcp = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "nfsd"
                    , "net"
                    , NULL
                    , "network"
                    , NULL
                    , "NFS Server Network Statistics"
                    , "packets/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NFSD_NAME
                    , NETDATA_CHART_PRIO_NFSD_NET
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rd_udp = rrddim_add(st, "udp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_tcp = rrddim_add(st, "tcp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        // ignore net_count, net_tcp_connections
        (void)net_count;
        (void)net_tcp_connections;

        rrddim_set_by_pointer(st, rd_udp, net_udp_count);
        rrddim_set_by_pointer(st, rd_tcp, net_tcp_count);
        rrdset_done(st);
    }

    if(do_rpc == 2) {
        static RRDSET *st = NULL;
        static RRDDIM *rd_calls      = NULL,
                      *rd_bad_format = NULL,
                      *rd_bad_auth   = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "nfsd"
                    , "rpc"
                    , NULL
                    , "rpc"
                    , NULL
                    , "NFS Server Remote Procedure Calls Statistics"
                    , "calls/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NFSD_NAME
                    , NETDATA_CHART_PRIO_NFSD_RPC
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_calls      = rrddim_add(st, "calls",      NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_bad_format = rrddim_add(st, "bad_format", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rd_bad_auth   = rrddim_add(st, "bad_auth",   NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        // ignore rpc_bad_client
        (void)rpc_bad_client;

        rrddim_set_by_pointer(st, rd_calls, rpc_calls);
        rrddim_set_by_pointer(st, rd_bad_format, rpc_bad_format);
        rrddim_set_by_pointer(st, rd_bad_auth, rpc_bad_auth);
        rrdset_done(st);
    }

    if(do_proc2 == 2) {
        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "nfsd"
                    , "proc2"
                    , NULL
                    , "nfsv2rpc"
                    , NULL
                    , "NFS v2 Server Remote Procedure Calls"
                    , "calls/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NFSD_NAME
                    , NETDATA_CHART_PRIO_NFSD_PROC2
                    , update_every
                    , RRDSET_TYPE_STACKED
            );
        }

        size_t i;
        for(i = 0; nfsd_proc2_values[i].present ; i++) {
            if(unlikely(!nfsd_proc2_values[i].rd))
                nfsd_proc2_values[i].rd = rrddim_add(st, nfsd_proc2_values[i].name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st, nfsd_proc2_values[i].rd, nfsd_proc2_values[i].value);
        }

        rrdset_done(st);
    }

    if(do_proc3 == 2) {
        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "nfsd"
                    , "proc3"
                    , NULL
                    , "nfsv3rpc"
                    , NULL
                    , "NFS v3 Server Remote Procedure Calls"
                    , "calls/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NFSD_NAME
                    , NETDATA_CHART_PRIO_NFSD_PROC3
                    , update_every
                    , RRDSET_TYPE_STACKED
            );
        }

        size_t i;
        for(i = 0; nfsd_proc3_values[i].present ; i++) {
            if(unlikely(!nfsd_proc3_values[i].rd))
                nfsd_proc3_values[i].rd = rrddim_add(st, nfsd_proc3_values[i].name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st, nfsd_proc3_values[i].rd, nfsd_proc3_values[i].value);
        }

        rrdset_done(st);
    }

    if(do_proc4 == 2) {
        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "nfsd"
                    , "proc4"
                    , NULL
                    , "nfsv4rpc"
                    , NULL
                    , "NFS v4 Server Remote Procedure Calls"
                    , "calls/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NFSD_NAME
                    , NETDATA_CHART_PRIO_NFSD_PROC4
                    , update_every
                    , RRDSET_TYPE_STACKED
            );
        }

        size_t i;
        for(i = 0; nfsd_proc4_values[i].present ; i++) {
            if(unlikely(!nfsd_proc4_values[i].rd))
                nfsd_proc4_values[i].rd = rrddim_add(st, nfsd_proc4_values[i].name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st, nfsd_proc4_values[i].rd, nfsd_proc4_values[i].value);
        }

        rrdset_done(st);
    }

    if(do_proc4ops == 2) {
        static RRDSET *st = NULL;
        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "nfsd"
                    , "proc4ops"
                    , NULL
                    , "nfsv4ops"
                    , NULL
                    , "NFS v4 Server Operations"
                    , "operations/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NFSD_NAME
                    , NETDATA_CHART_PRIO_NFSD_PROC4OPS
                    , update_every
                    , RRDSET_TYPE_STACKED
            );
        }

        size_t i;
        for(i = 0; nfsd4_ops_values[i].present ; i++) {
            if(unlikely(!nfsd4_ops_values[i].rd))
                nfsd4_ops_values[i].rd = rrddim_add(st, nfsd4_ops_values[i].name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);

            rrddim_set_by_pointer(st, nfsd4_ops_values[i].rd, nfsd4_ops_values[i].value);
        }

        rrdset_done(st);
    }

    return 0;
}
