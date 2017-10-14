#include "common.h"

struct nfs_procs {
    char name[30];
    unsigned long long value;
    int present;
};

struct nfs_procs nfs_proc2_values[] = {
    { "null", 0ULL, 0 },
    { "getattr", 0ULL, 0 },
    { "setattr", 0ULL, 0 },
    { "root", 0ULL, 0 },
    { "lookup", 0ULL, 0 },
    { "readlink", 0ULL, 0 },
    { "read", 0ULL, 0 },
    { "wrcache", 0ULL, 0 },
    { "write", 0ULL, 0 },
    { "create", 0ULL, 0 },
    { "remove", 0ULL, 0 },
    { "rename", 0ULL, 0 },
    { "link", 0ULL, 0 },
    { "symlink", 0ULL, 0 },
    { "mkdir", 0ULL, 0 },
    { "rmdir", 0ULL, 0 },
    { "readdir", 0ULL, 0 },
    { "fsstat", 0ULL, 0 },
    
    /* termination */
    { "", 0ULL, 0 }
};

struct nfs_procs nfs_proc3_values[] = {
    { "null", 0ULL, 0 },
    { "getattr", 0ULL, 0 },
    { "setattr", 0ULL, 0 },
    { "lookup", 0ULL, 0 },
    { "access", 0ULL, 0 },
    { "readlink", 0ULL, 0 },
    { "read", 0ULL, 0 },
    { "write", 0ULL, 0 },
    { "create", 0ULL, 0 },
    { "mkdir", 0ULL, 0 },
    { "symlink", 0ULL, 0 },
    { "mknod", 0ULL, 0 },
    { "remove", 0ULL, 0 },
    { "rmdir", 0ULL, 0 },
    { "rename", 0ULL, 0 },
    { "link", 0ULL, 0 },
    { "readdir", 0ULL, 0 },
    { "readdirplus", 0ULL, 0 },
    { "fsstat", 0ULL, 0 },
    { "fsinfo", 0ULL, 0 },
    { "pathconf", 0ULL, 0 },
    { "commit", 0ULL, 0 },

    /* termination */
    { "", 0ULL, 0 }
};

struct nfs_procs nfs_proc4_values[] = {
    { "null", 0ULL, 0 },
    { "read", 0ULL, 0 },
    { "write", 0ULL, 0 },
    { "commit", 0ULL, 0 },
    { "open", 0ULL, 0 },
    { "open_conf", 0ULL, 0 },
    { "open_noat", 0ULL, 0 },
    { "open_dgrd", 0ULL, 0 },
    { "close", 0ULL, 0 },
    { "setattr", 0ULL, 0 },
    { "fsinfo", 0ULL, 0 },
    { "renew", 0ULL, 0 },
    { "setclntid", 0ULL, 0 },
    { "confirm", 0ULL, 0 },
    { "lock", 0ULL, 0 },
    { "lockt", 0ULL, 0 },
    { "locku", 0ULL, 0 },
    { "access", 0ULL, 0 },
    { "getattr", 0ULL, 0 },
    { "lookup", 0ULL, 0 },
    { "lookup_root", 0ULL, 0 },
    { "remove", 0ULL, 0 },
    { "rename", 0ULL, 0 },
    { "link", 0ULL, 0 },
    { "symlink", 0ULL, 0 },
    { "create", 0ULL, 0 },
    { "pathconf", 0ULL, 0 },
    { "statfs", 0ULL, 0 },
    { "readlink", 0ULL, 0 },
    { "readdir", 0ULL, 0 },
    { "server_caps", 0ULL, 0 },
    { "delegreturn", 0ULL, 0 },
    { "getacl", 0ULL, 0 },
    { "setacl", 0ULL, 0 },
    { "fs_locations", 0ULL, 0 },
    { "rel_lkowner", 0ULL, 0 },
    { "secinfo", 0ULL, 0 },
    { "fsid_present", 0ULL, 0 },

    /* nfsv4.1 client ops */
    { "exchange_id", 0ULL, 0 },
    { "create_session", 0ULL, 0 },
    { "destroy_session", 0ULL, 0 },
    { "sequence", 0ULL, 0 },
    { "get_lease_time", 0ULL, 0 },
    { "reclaim_comp", 0ULL, 0 },
    { "layoutget", 0ULL, 0 },
    { "getdevinfo", 0ULL, 0 },
    { "layoutcommit", 0ULL, 0 },
    { "layoutreturn", 0ULL, 0 },
    { "secinfo_no", 0ULL, 0 },
    { "test_stateid", 0ULL, 0 },
    { "free_stateid", 0ULL, 0 },
    { "getdevicelist", 0ULL, 0 },
    { "bind_conn_to_ses", 0ULL, 0 },
    { "destroy_clientid", 0ULL, 0 },

    /* nfsv4.2 client ops */
    { "seek", 0ULL, 0 },
    { "allocate", 0ULL, 0 },
    { "deallocate", 0ULL, 0 },
    { "layoutstats", 0ULL, 0 },
    { "clone", 0ULL, 0 },

    /* termination */
    { "", 0ULL, 0 }
};

int do_proc_net_rpc_nfs(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;
    static int do_net = -1, do_rpc = -1, do_proc2 = -1, do_proc3 = -1, do_proc4 = -1;
    static int proc2_warning = 0, proc3_warning = 0, proc4_warning = 0;

    if(!ff) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/rpc/nfs");
        ff = procfile_open(config_get("plugin:proc:/proc/net/rpc/nfs", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
    }
    if(!ff) return 1;

    ff = procfile_readall(ff);
    if(!ff) return 0; // we return 0, so that we will retry to open it next time

    if(do_net == -1) do_net = config_get_boolean("plugin:proc:/proc/net/rpc/nfs", "network", 1);
    if(do_rpc == -1) do_rpc = config_get_boolean("plugin:proc:/proc/net/rpc/nfs", "rpc", 1);
    if(do_proc2 == -1) do_proc2 = config_get_boolean("plugin:proc:/proc/net/rpc/nfs", "NFS v2 procedures", 1);
    if(do_proc3 == -1) do_proc3 = config_get_boolean("plugin:proc:/proc/net/rpc/nfs", "NFS v3 procedures", 1);
    if(do_proc4 == -1) do_proc4 = config_get_boolean("plugin:proc:/proc/net/rpc/nfs", "NFS v4 procedures", 1);

    // if they are enabled, reset them to 1
    // later we do them =2 to avoid doing strcmp() for all lines
    if(do_net) do_net = 1;
    if(do_rpc) do_rpc = 1;
    if(do_proc2) do_proc2 = 1;
    if(do_proc3) do_proc3 = 1;
    if(do_proc4) do_proc4 = 1;

    size_t lines = procfile_lines(ff), l;

    char *type;
    unsigned long long net_count = 0, net_udp_count = 0, net_tcp_count = 0, net_tcp_connections = 0;
    unsigned long long rpc_calls = 0, rpc_retransmits = 0, rpc_auth_refresh = 0;

    for(l = 0; l < lines ;l++) {
        size_t words = procfile_linewords(ff, l);
        if(!words) continue;

        type        = procfile_lineword(ff, l, 0);

        if(do_net == 1 && strcmp(type, "net") == 0) {
            if(words < 5) {
                error("%s line of /proc/net/rpc/nfs has %zu words, expected %d", type, words, 5);
                continue;
            }

            net_count = str2ull(procfile_lineword(ff, l, 1));
            net_udp_count = str2ull(procfile_lineword(ff, l, 2));
            net_tcp_count = str2ull(procfile_lineword(ff, l, 3));
            net_tcp_connections = str2ull(procfile_lineword(ff, l, 4));

            unsigned long long sum = net_count + net_udp_count + net_tcp_count + net_tcp_connections;
            if(sum == 0ULL) do_net = -1;
            else do_net = 2;
        }
        else if(do_rpc == 1 && strcmp(type, "rpc") == 0) {
            if(words < 4) {
                error("%s line of /proc/net/rpc/nfs has %zu words, expected %d", type, words, 6);
                continue;
            }

            rpc_calls = str2ull(procfile_lineword(ff, l, 1));
            rpc_retransmits = str2ull(procfile_lineword(ff, l, 2));
            rpc_auth_refresh = str2ull(procfile_lineword(ff, l, 3));

            unsigned long long sum = rpc_calls + rpc_retransmits + rpc_auth_refresh;
            if(sum == 0ULL) do_rpc = -1;
            else do_rpc = 2;
        }
        else if(do_proc2 == 1 && strcmp(type, "proc2") == 0) {
            // the first number is the count of numbers present
            // so we start for word 2

            unsigned long long sum = 0;
            unsigned int i, j;
            for(i = 0, j = 2; j < words && nfs_proc2_values[i].name[0] ; i++, j++) {
                nfs_proc2_values[i].value = str2ull(procfile_lineword(ff, l, j));
                nfs_proc2_values[i].present = 1;
                sum += nfs_proc2_values[i].value;
            }

            if(sum == 0ULL) {
                if(!proc2_warning) {
                    error("Disabling /proc/net/rpc/nfs v2 procedure calls chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
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
            for(i = 0, j = 2; j < words && nfs_proc3_values[i].name[0] ; i++, j++) {
                nfs_proc3_values[i].value = str2ull(procfile_lineword(ff, l, j));
                nfs_proc3_values[i].present = 1;
                sum += nfs_proc3_values[i].value;
            }

            if(sum == 0ULL) {
                if(!proc3_warning) {
                    info("Disabling /proc/net/rpc/nfs v3 procedure calls chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
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
            for(i = 0, j = 2; j < words && nfs_proc4_values[i].name[0] ; i++, j++) {
                nfs_proc4_values[i].value = str2ull(procfile_lineword(ff, l, j));
                nfs_proc4_values[i].present = 1;
                sum += nfs_proc4_values[i].value;
            }

            if(sum == 0ULL) {
                if(!proc4_warning) {
                    info("Disabling /proc/net/rpc/nfs v4 procedure calls chart. It seems unused on this machine. It will be enabled automatically when found with data in it.");
                    proc4_warning = 1;
                }
                do_proc4 = 0;
            }
            else do_proc4 = 2;
        }
    }

    RRDSET *st;

    // --------------------------------------------------------------------

    if(do_net == 2) {
        st = rrdset_find_bytype_localhost("nfs", "net");
        if(!st) {
            st = rrdset_create_localhost(
                    "nfs"
                    , "net"
                    , NULL
                    , "network"
                    , NULL
                    , "NFS Client Network"
                    , "operations/s"
                    , "proc"
                    , "net/rpc/nfs"
                    , 5007
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "udp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "tcp", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        // ignore net_count, net_tcp_connections
        if(net_count) {};
        if(net_tcp_connections) {};

        rrddim_set(st, "udp", net_udp_count);
        rrddim_set(st, "tcp", net_tcp_count);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_rpc == 2) {
        st = rrdset_find_bytype_localhost("nfs", "rpc");
        if(!st) {
            st = rrdset_create_localhost(
                    "nfs"
                    , "rpc"
                    , NULL
                    , "rpc"
                    , NULL
                    , "NFS Client Remote Procedure Calls Statistics"
                    , "calls/s"
                    , "proc"
                    , "net/rpc/nfs"
                    , 5008
                    , update_every
                    , RRDSET_TYPE_LINE
            );
            rrdset_flag_set(st, RRDSET_FLAG_DETAIL);

            rrddim_add(st, "calls", NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "retransmits", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            rrddim_add(st, "auth_refresh", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        rrddim_set(st, "calls", rpc_calls);
        rrddim_set(st, "retransmits", rpc_retransmits);
        rrddim_set(st, "auth_refresh", rpc_auth_refresh);
        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_proc2 == 2) {
        unsigned int i;
        st = rrdset_find_bytype_localhost("nfs", "proc2");
        if(!st) {
            st = rrdset_create_localhost(
                    "nfs"
                    , "proc2"
                    , NULL
                    , "nfsv2rpc"
                    , NULL
                    , "NFS v2 Client Remote Procedure Calls"
                    , "calls/s"
                    , "proc"
                    , "net/rpc/nfs"
                    , 5009
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            for(i = 0; nfs_proc2_values[i].present ; i++)
                rrddim_add(st, nfs_proc2_values[i].name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        for(i = 0; nfs_proc2_values[i].present ; i++)
            rrddim_set(st, nfs_proc2_values[i].name, nfs_proc2_values[i].value);

        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_proc3 == 2) {
        unsigned int i;
        st = rrdset_find_bytype_localhost("nfs", "proc3");
        if(!st) {
            st = rrdset_create_localhost(
                    "nfs"
                    , "proc3"
                    , NULL
                    , "nfsv3rpc"
                    , NULL
                    , "NFS v3 Client Remote Procedure Calls"
                    , "calls/s"
                    , "proc"
                    , "net/rpc/nfs"
                    , 5010
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            for(i = 0; nfs_proc3_values[i].present ; i++)
                rrddim_add(st, nfs_proc3_values[i].name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        for(i = 0; nfs_proc3_values[i].present ; i++)
            rrddim_set(st, nfs_proc3_values[i].name, nfs_proc3_values[i].value);

        rrdset_done(st);
    }

    // --------------------------------------------------------------------

    if(do_proc4 == 2) {
        unsigned int i;
        st = rrdset_find_bytype_localhost("nfs", "proc4");
        if(!st) {
            st = rrdset_create_localhost(
                    "nfs"
                    , "proc4"
                    , NULL
                    , "nfsv4rpc"
                    , NULL
                    , "NFS v4 Client Remote Procedure Calls"
                    , "calls/s"
                    , "proc"
                    , "net/rpc/nfs"
                    , 5011
                    , update_every
                    , RRDSET_TYPE_STACKED
            );

            for(i = 0; nfs_proc4_values[i].present ; i++)
                rrddim_add(st, nfs_proc4_values[i].name, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        else rrdset_next(st);

        for(i = 0; nfs_proc4_values[i].present ; i++)
            rrddim_set(st, nfs_proc4_values[i].name, nfs_proc4_values[i].value);

        rrdset_done(st);
    }

    return 0;
}
