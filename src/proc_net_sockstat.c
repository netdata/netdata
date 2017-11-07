#include "common.h"

#define RRD_TYPE_NET_SNMP           "ipv4"

static struct proc_net_sockstat {
    kernel_uint_t sockets_used;

    kernel_uint_t tcp_inuse;
    kernel_uint_t tcp_orphan;
    kernel_uint_t tcp_tw;
    kernel_uint_t tcp_alloc;
    kernel_uint_t tcp_mem;

    kernel_uint_t udp_inuse;
    kernel_uint_t udp_mem;

    kernel_uint_t udplite_inuse;

    kernel_uint_t raw_inuse;

    kernel_uint_t frag_inuse;
    kernel_uint_t frag_memory;
} sockstat_root = { 0 };


static inline void arl_callback_str2kernel_uint_t(const char *name, uint32_t hash, const char *value, void *dst) {
    (void)name;
    (void)hash;

    register kernel_uint_t *d = dst;
    *d = str2kernel_uint_t(value);
    // fprintf(stderr, "name '%s' with hash %u and value '%s' is %llu\n", name, hash, value, (unsigned long long)*d);
}

static kernel_uint_t read_tcp_max_orphans(void) {
    static char *filename = NULL;
    static RRDVAR *tcp_max_orphans_var = NULL;

    if(unlikely(!filename)) {
        char buffer[FILENAME_MAX + 1];
        snprintfz(buffer, FILENAME_MAX, "%s/proc/sys/net/ipv4/tcp_max_orphans", netdata_configured_host_prefix);
        filename = strdupz(buffer);
    }

    unsigned long long tcp_max_orphans = 0;
    if(read_single_number_file(filename, &tcp_max_orphans) == 0) {

        if(unlikely(!tcp_max_orphans_var))
            tcp_max_orphans_var = rrdvar_custom_host_variable_create(localhost, "tcp_max_orphans");

        rrdvar_custom_host_variable_set(localhost, tcp_max_orphans_var, tcp_max_orphans);
        return  tcp_max_orphans;
    }

    return 0;
}

int do_proc_net_sockstat(int update_every, usec_t dt) {
    (void)dt;

    static procfile *ff = NULL;

    static uint32_t hash_sockets = 0,
                    hash_raw = 0,
                    hash_frag = 0,
                    hash_tcp = 0,
                    hash_udp = 0,
                    hash_udplite = 0;

    static long long update_tcp_max_orphans_every = 60, update_tcp_max_orphans_count = 0;

    static ARL_BASE *arl_sockets = NULL;
    static ARL_BASE *arl_tcp = NULL;
    static ARL_BASE *arl_udp = NULL;
    static ARL_BASE *arl_udplite = NULL;
    static ARL_BASE *arl_raw = NULL;
    static ARL_BASE *arl_frag = NULL;

    static int do_sockets = -1, do_tcp_sockets = -1, do_tcp_mem = -1, do_udp_sockets = -1, do_udp_mem = -1, do_udplite_sockets = -1, do_raw_sockets = -1, do_frag_sockets = -1, do_frag_mem = -1;

    static char     *keys[6]  = { NULL };
    static uint32_t hashes[6] = { 0 };
    static ARL_BASE *bases[6] = { NULL };

    if(unlikely(!arl_sockets)) {
        do_sockets         = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat", "ipv4 sockets", CONFIG_BOOLEAN_AUTO);
        do_tcp_sockets     = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat", "ipv4 TCP sockets", CONFIG_BOOLEAN_AUTO);
        do_tcp_mem         = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat", "ipv4 TCP memory", CONFIG_BOOLEAN_AUTO);
        do_udp_sockets     = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat", "ipv4 UDP sockets", CONFIG_BOOLEAN_AUTO);
        do_udp_mem         = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat", "ipv4 UDP memory", CONFIG_BOOLEAN_AUTO);
        do_udplite_sockets = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat", "ipv4 UDPLITE sockets", CONFIG_BOOLEAN_AUTO);
        do_raw_sockets     = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat", "ipv4 RAW sockets", CONFIG_BOOLEAN_AUTO);
        do_frag_sockets    = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat", "ipv4 FRAG sockets", CONFIG_BOOLEAN_AUTO);
        do_frag_mem        = config_get_boolean_ondemand("plugin:proc:/proc/net/sockstat", "ipv4 FRAG memory", CONFIG_BOOLEAN_AUTO);
        update_tcp_max_orphans_every = config_get_number("plugin:proc:/proc/net/sockstat", "update tcp_max_orphans every", update_tcp_max_orphans_every);
        update_tcp_max_orphans_count = update_tcp_max_orphans_every;

        arl_sockets = arl_create("sockstat/sockets", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_sockets, "used", &sockstat_root.sockets_used);

        arl_tcp = arl_create("sockstat/TCP", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_tcp, "inuse",  &sockstat_root.tcp_inuse);
        arl_expect(arl_tcp, "orphan", &sockstat_root.tcp_orphan);
        arl_expect(arl_tcp, "tw",     &sockstat_root.tcp_tw);
        arl_expect(arl_tcp, "alloc",  &sockstat_root.tcp_alloc);
        arl_expect(arl_tcp, "mem",    &sockstat_root.tcp_mem);

        arl_udp = arl_create("sockstat/UDP", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_udp, "inuse", &sockstat_root.udp_inuse);
        arl_expect(arl_udp, "mem", &sockstat_root.udp_mem);

        arl_udplite = arl_create("sockstat/UDPLITE", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_udplite, "inuse", &sockstat_root.udplite_inuse);

        arl_raw = arl_create("sockstat/RAW", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_raw, "inuse", &sockstat_root.raw_inuse);

        arl_frag = arl_create("sockstat/FRAG", arl_callback_str2kernel_uint_t, 60);
        arl_expect(arl_frag, "inuse", &sockstat_root.frag_inuse);
        arl_expect(arl_frag, "memory", &sockstat_root.frag_memory);

        hash_sockets = simple_hash("sockets");
        hash_tcp = simple_hash("TCP");
        hash_udp = simple_hash("UDP");
        hash_udplite = simple_hash("UDPLITE");
        hash_raw = simple_hash("RAW");
        hash_frag = simple_hash("FRAG");

        keys[0] = "sockets"; hashes[0] = hash_sockets; bases[0] = arl_sockets;
        keys[1] = "TCP";     hashes[1] = hash_tcp;     bases[1] = arl_tcp;
        keys[2] = "UDP";     hashes[2] = hash_udp;     bases[2] = arl_udp;
        keys[3] = "UDPLITE"; hashes[3] = hash_udplite; bases[3] = arl_udplite;
        keys[4] = "RAW";     hashes[4] = hash_raw;     bases[4] = arl_raw;
        keys[5] = "FRAG";    hashes[5] = hash_frag;    bases[5] = arl_frag;
    }

    update_tcp_max_orphans_count += update_every;
    if(unlikely(update_tcp_max_orphans_count > update_tcp_max_orphans_every))
        read_tcp_max_orphans();

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/sockstat");
        ff = procfile_open(config_get("plugin:proc:/proc/net/sockstat", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    size_t lines = procfile_lines(ff), l;

    for(l = 0; l < lines ;l++) {
        size_t  words = procfile_linewords(ff, l);
        char     *key = procfile_lineword(ff, l, 0);
        uint32_t hash = simple_hash(key);

        int k;
        for(k = 0; keys[k] ; k++) {
            if(unlikely(hash == hashes[k] && strcmp(key, keys[k]) == 0)) {
                // fprintf(stderr, "KEY: '%s', l=%zu, w=1, words=%zu\n", key, l, words);
                ARL_BASE *arl = bases[k];
                arl_begin(arl);
                size_t w = 1;

                while(w + 1 < words) {
                    char *name  = procfile_lineword(ff, l, w); w++;
                    char *value = procfile_lineword(ff, l, w); w++;
                    // fprintf(stderr, " > NAME '%s', VALUE '%s', l=%zu, w=%zu, words=%zu\n", name, value, l, w, words);
                    if(unlikely(arl_check(arl, name, value) != 0))
                        break;
                }

                break;
            }
        }
    }

    // ------------------------------------------------------------------------

    if(do_sockets == CONFIG_BOOLEAN_YES || (do_sockets == CONFIG_BOOLEAN_AUTO && sockstat_root.sockets_used)) {
        do_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_used = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv4"
                    , "sockstat_sockets"
                    , NULL
                    , "sockets"
                    , NULL
                    , "IPv4 Sockets In Use"
                    , "sockets"
                    , "proc"
                    , "net/sockstat"
                    , 2400
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_used = rrddim_add(st, "used", NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_used, (collected_number)sockstat_root.sockets_used);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_tcp_sockets == CONFIG_BOOLEAN_YES || (do_tcp_sockets == CONFIG_BOOLEAN_AUTO && (sockstat_root.tcp_inuse || sockstat_root.tcp_orphan || sockstat_root.tcp_tw || sockstat_root.tcp_alloc))) {
        do_tcp_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_inuse = NULL,
                      *rd_orphan = NULL,
                      *rd_timewait = NULL,
                      *rd_alloc = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv4"
                    , "sockstat_tcp_sockets"
                    , NULL
                    , "sockets"
                    , NULL
                    , "IPv4 TCP Sockets"
                    , "sockets"
                    , "proc"
                    , "net/sockstat"
                    , 2405
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_alloc    = rrddim_add(st, "alloc",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_orphan   = rrddim_add(st, "orphan",    NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_inuse    = rrddim_add(st, "inuse",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
            rd_timewait = rrddim_add(st, "timewait",  NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inuse,    (collected_number)sockstat_root.tcp_inuse);
        rrddim_set_by_pointer(st, rd_orphan,   (collected_number)sockstat_root.tcp_orphan);
        rrddim_set_by_pointer(st, rd_timewait, (collected_number)sockstat_root.tcp_tw);
        rrddim_set_by_pointer(st, rd_alloc,    (collected_number)sockstat_root.tcp_alloc);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_tcp_mem == CONFIG_BOOLEAN_YES || (do_tcp_mem == CONFIG_BOOLEAN_AUTO && sockstat_root.tcp_mem)) {
        do_tcp_mem = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_mem = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv4"
                    , "sockstat_tcp_mem"
                    , NULL
                    , "sockets"
                    , NULL
                    , "IPv4 TCP Sockets Memory"
                    , "KB"
                    , "proc"
                    , "net/sockstat"
                    , 2406
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_mem = rrddim_add(st, "mem", NULL, sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_mem, (collected_number)sockstat_root.tcp_mem);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_udp_sockets == CONFIG_BOOLEAN_YES || (do_udp_sockets == CONFIG_BOOLEAN_AUTO && sockstat_root.udp_inuse)) {
        do_udp_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_inuse = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv4"
                    , "sockstat_udp_sockets"
                    , NULL
                    , "sockets"
                    , NULL
                    , "IPv4 UDP Sockets"
                    , "sockets"
                    , "proc"
                    , "net/sockstat"
                    , 2410
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_inuse    = rrddim_add(st, "inuse",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inuse,    (collected_number)sockstat_root.udp_inuse);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_udp_mem == CONFIG_BOOLEAN_YES || (do_udp_mem == CONFIG_BOOLEAN_AUTO && sockstat_root.udp_mem)) {
        do_udp_mem = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_mem = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv4"
                    , "sockstat_udp_mem"
                    , NULL
                    , "sockets"
                    , NULL
                    , "IPv4 UDP Sockets Memory"
                    , "KB"
                    , "proc"
                    , "net/sockstat"
                    , 2411
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_mem = rrddim_add(st, "mem", NULL, sysconf(_SC_PAGESIZE), 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_mem, (collected_number)sockstat_root.udp_mem);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_udplite_sockets == CONFIG_BOOLEAN_YES || (do_udplite_sockets == CONFIG_BOOLEAN_AUTO && sockstat_root.udplite_inuse)) {
        do_udplite_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_inuse = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv4"
                    , "sockstat_udplite_sockets"
                    , NULL
                    , "sockets"
                    , NULL
                    , "IPv4 UDPLITE Sockets"
                    , "sockets"
                    , "proc"
                    , "net/sockstat"
                    , 2420
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_inuse    = rrddim_add(st, "inuse",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inuse,    (collected_number)sockstat_root.udplite_inuse);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_raw_sockets == CONFIG_BOOLEAN_YES || (do_raw_sockets == CONFIG_BOOLEAN_AUTO && sockstat_root.raw_inuse)) {
        do_raw_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_inuse = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv4"
                    , "sockstat_raw_sockets"
                    , NULL
                    , "sockets"
                    , NULL
                    , "IPv4 RAW Sockets"
                    , "sockets"
                    , "proc"
                    , "net/sockstat"
                    , 2430
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_inuse    = rrddim_add(st, "inuse",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inuse,    (collected_number)sockstat_root.raw_inuse);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_frag_sockets == CONFIG_BOOLEAN_YES || (do_frag_sockets == CONFIG_BOOLEAN_AUTO && sockstat_root.frag_inuse)) {
        do_frag_sockets = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_inuse = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv4"
                    , "sockstat_frag_sockets"
                    , NULL
                    , "sockets"
                    , NULL
                    , "IPv4 FRAG Sockets"
                    , "sockets"
                    , "proc"
                    , "net/sockstat"
                    , 2440
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_inuse    = rrddim_add(st, "inuse",     NULL,   1, 1, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_inuse,    (collected_number)sockstat_root.frag_inuse);
        rrdset_done(st);
    }

    // ------------------------------------------------------------------------

    if(do_frag_mem == CONFIG_BOOLEAN_YES || (do_frag_mem == CONFIG_BOOLEAN_AUTO && sockstat_root.frag_memory)) {
        do_frag_mem = CONFIG_BOOLEAN_YES;

        static RRDSET *st = NULL;
        static RRDDIM *rd_mem = NULL;

        if(unlikely(!st)) {
            st = rrdset_create_localhost(
                    "ipv4"
                    , "sockstat_frag_mem"
                    , NULL
                    , "sockets"
                    , NULL
                    , "IPv4 FRAG Sockets Memory"
                    , "KB"
                    , "proc"
                    , "net/sockstat"
                    , 2441
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_mem = rrddim_add(st, "mem", NULL, 1, 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else rrdset_next(st);

        rrddim_set_by_pointer(st, rd_mem, (collected_number)sockstat_root.frag_memory);
        rrdset_done(st);
    }

    return 0;
}

