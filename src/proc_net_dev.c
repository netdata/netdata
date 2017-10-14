#include "common.h"

// ----------------------------------------------------------------------------
// netdev list

static struct netdev {
    char *name;
    uint32_t hash;
    size_t len;

    // flags
    int configured;
    int enabled;
    int updated;

    int do_bandwidth;
    int do_packets;
    int do_errors;
    int do_drops;
    int do_fifo;
    int do_compressed;
    int do_events;

    const char *chart_type_net_bytes;
    const char *chart_type_net_packets;
    const char *chart_type_net_errors;
    const char *chart_type_net_fifo;
    const char *chart_type_net_events;
    const char *chart_type_net_drops;
    const char *chart_type_net_compressed;

    const char *chart_id_net_bytes;
    const char *chart_id_net_packets;
    const char *chart_id_net_errors;
    const char *chart_id_net_fifo;
    const char *chart_id_net_events;
    const char *chart_id_net_drops;
    const char *chart_id_net_compressed;

    const char *chart_family;

    int flipped;
    unsigned long priority;

    // data collected
    kernel_uint_t rbytes;
    kernel_uint_t rpackets;
    kernel_uint_t rerrors;
    kernel_uint_t rdrops;
    kernel_uint_t rfifo;
    kernel_uint_t rframe;
    kernel_uint_t rcompressed;
    kernel_uint_t rmulticast;

    kernel_uint_t tbytes;
    kernel_uint_t tpackets;
    kernel_uint_t terrors;
    kernel_uint_t tdrops;
    kernel_uint_t tfifo;
    kernel_uint_t tcollisions;
    kernel_uint_t tcarrier;
    kernel_uint_t tcompressed;

    // charts
    RRDSET *st_bandwidth;
    RRDSET *st_packets;
    RRDSET *st_errors;
    RRDSET *st_drops;
    RRDSET *st_fifo;
    RRDSET *st_compressed;
    RRDSET *st_events;

    // dimensions
    RRDDIM *rd_rbytes;
    RRDDIM *rd_rpackets;
    RRDDIM *rd_rerrors;
    RRDDIM *rd_rdrops;
    RRDDIM *rd_rfifo;
    RRDDIM *rd_rframe;
    RRDDIM *rd_rcompressed;
    RRDDIM *rd_rmulticast;

    RRDDIM *rd_tbytes;
    RRDDIM *rd_tpackets;
    RRDDIM *rd_terrors;
    RRDDIM *rd_tdrops;
    RRDDIM *rd_tfifo;
    RRDDIM *rd_tcollisions;
    RRDDIM *rd_tcarrier;
    RRDDIM *rd_tcompressed;

    struct netdev *next;
} *netdev_root = NULL, *netdev_last_used = NULL;

static size_t netdev_added = 0, netdev_found = 0;

// ----------------------------------------------------------------------------

static void netdev_charts_release(struct netdev *d) {
    if(d->st_bandwidth)  rrdset_is_obsolete(d->st_bandwidth);
    if(d->st_packets)    rrdset_is_obsolete(d->st_packets);
    if(d->st_errors)     rrdset_is_obsolete(d->st_errors);
    if(d->st_drops)      rrdset_is_obsolete(d->st_drops);
    if(d->st_fifo)       rrdset_is_obsolete(d->st_fifo);
    if(d->st_compressed) rrdset_is_obsolete(d->st_compressed);
    if(d->st_events)     rrdset_is_obsolete(d->st_events);

    d->st_bandwidth   = NULL;
    d->st_compressed  = NULL;
    d->st_drops       = NULL;
    d->st_errors      = NULL;
    d->st_events      = NULL;
    d->st_fifo        = NULL;
    d->st_packets     = NULL;

    d->rd_rbytes      = NULL;
    d->rd_rpackets    = NULL;
    d->rd_rerrors     = NULL;
    d->rd_rdrops      = NULL;
    d->rd_rfifo       = NULL;
    d->rd_rframe      = NULL;
    d->rd_rcompressed = NULL;
    d->rd_rmulticast  = NULL;

    d->rd_tbytes      = NULL;
    d->rd_tpackets    = NULL;
    d->rd_terrors     = NULL;
    d->rd_tdrops      = NULL;
    d->rd_tfifo       = NULL;
    d->rd_tcollisions = NULL;
    d->rd_tcarrier    = NULL;
    d->rd_tcompressed = NULL;
}

static void netdev_free_strings(struct netdev *d) {
    freez((void *)d->chart_type_net_bytes);
    freez((void *)d->chart_type_net_compressed);
    freez((void *)d->chart_type_net_drops);
    freez((void *)d->chart_type_net_errors);
    freez((void *)d->chart_type_net_events);
    freez((void *)d->chart_type_net_fifo);
    freez((void *)d->chart_type_net_packets);

    freez((void *)d->chart_id_net_bytes);
    freez((void *)d->chart_id_net_compressed);
    freez((void *)d->chart_id_net_drops);
    freez((void *)d->chart_id_net_errors);
    freez((void *)d->chart_id_net_events);
    freez((void *)d->chart_id_net_fifo);
    freez((void *)d->chart_id_net_packets);

    freez((void *)d->chart_family);
}

static void netdev_free(struct netdev *d) {
    netdev_charts_release(d);
    netdev_free_strings(d);

    freez((void *)d->name);
    freez((void *)d);
    netdev_added--;
}


// ----------------------------------------------------------------------------
// netdev renames

static struct netdev_rename {
    const char *host_device;
    uint32_t hash;

    const char *container_device;
    const char *container_name;

    int processed;

    struct netdev_rename *next;
} *netdev_rename_root = NULL;

static int netdev_pending_renames = 0;
static netdata_mutex_t netdev_rename_mutex = NETDATA_MUTEX_INITIALIZER;

static struct netdev_rename *netdev_rename_find(const char *host_device, uint32_t hash) {
    struct netdev_rename *r;

    for(r = netdev_rename_root; r ; r = r->next)
        if(r->hash == hash && !strcmp(host_device, r->host_device))
            return r;

    return NULL;
}

// other threads can call this function to register a rename to a netdev
void netdev_rename_device_add(const char *host_device, const char *container_device, const char *container_name) {
    netdata_mutex_lock(&netdev_rename_mutex);

    uint32_t hash = simple_hash(host_device);
    struct netdev_rename *r = netdev_rename_find(host_device, hash);
    if(!r) {
        r = callocz(1, sizeof(struct netdev_rename));
        r->host_device      = strdupz(host_device);
        r->container_device = strdupz(container_device);
        r->container_name   = strdupz(container_name);
        r->hash             = hash;
        r->next             = netdev_rename_root;
        r->processed        = 0;
        netdev_rename_root  = r;
        netdev_pending_renames++;
        info("CGROUP: registered network interface rename for '%s' as '%s' under '%s'", r->host_device, r->container_device, r->container_name);
    }
    else {
        if(strcmp(r->container_device, container_device) != 0 || strcmp(r->container_name, container_name) != 0) {
            freez((void *) r->container_device);
            freez((void *) r->container_name);

            r->container_device = strdupz(container_device);
            r->container_name   = strdupz(container_name);
            r->processed        = 0;
            netdev_pending_renames++;
            info("CGROUP: altered network interface rename for '%s' as '%s' under '%s'", r->host_device, r->container_device, r->container_name);
        }
    }

    netdata_mutex_unlock(&netdev_rename_mutex);
}

// other threads can call this function to delete a rename to a netdev
void netdev_rename_device_del(const char *host_device) {
    netdata_mutex_lock(&netdev_rename_mutex);

    struct netdev_rename *r, *last = NULL;

    uint32_t hash = simple_hash(host_device);
    for(r = netdev_rename_root; r ; last = r, r = r->next) {
        if (r->hash == hash && !strcmp(host_device, r->host_device)) {
            if (netdev_rename_root == r)
                netdev_rename_root = r->next;
            else if (last)
                last->next = r->next;

            if(!r->processed)
                netdev_pending_renames--;

            info("CGROUP: unregistered network interface rename for '%s' as '%s' under '%s'", r->host_device, r->container_device, r->container_name);

            freez((void *) r->host_device);
            freez((void *) r->container_name);
            freez((void *) r->container_device);
            freez((void *) r);
            break;
        }
    }

    netdata_mutex_unlock(&netdev_rename_mutex);
}

static inline void netdev_rename_cgroup(struct netdev *d, struct netdev_rename *r) {
    info("CGROUP: renaming network interface '%s' as '%s' under '%s'", r->host_device, r->container_device, r->container_name);

    netdev_charts_release(d);
    netdev_free_strings(d);

    char buffer[RRD_ID_LENGTH_MAX + 1];

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "cgroup_%s", r->container_name);
    d->chart_type_net_bytes      = strdupz(buffer);
    d->chart_type_net_compressed = strdupz(buffer);
    d->chart_type_net_drops      = strdupz(buffer);
    d->chart_type_net_errors     = strdupz(buffer);
    d->chart_type_net_events     = strdupz(buffer);
    d->chart_type_net_fifo       = strdupz(buffer);
    d->chart_type_net_packets    = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_%s", r->container_device);
    d->chart_id_net_bytes      = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_compressed_%s", r->container_device);
    d->chart_id_net_compressed = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_drops_%s", r->container_device);
    d->chart_id_net_drops      = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_errors_%s", r->container_device);
    d->chart_id_net_errors     = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_events_%s", r->container_device);
    d->chart_id_net_events     = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_fifo_%s", r->container_device);
    d->chart_id_net_fifo       = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_packets_%s", r->container_device);
    d->chart_id_net_packets    = strdupz(buffer);

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net %s", r->container_device);
    d->chart_family = strdupz(buffer);

    d->priority = 43000;
    d->flipped = 1;
}

static inline void netdev_rename(struct netdev *d) {
    struct netdev_rename *r = netdev_rename_find(d->name, d->hash);
    if(unlikely(r && !r->processed)) {
        netdev_rename_cgroup(d, r);
        r->processed = 1;
        netdev_pending_renames--;
    }
}

static inline void netdev_rename_lock(struct netdev *d) {
    netdata_mutex_lock(&netdev_rename_mutex);
    netdev_rename(d);
    netdata_mutex_unlock(&netdev_rename_mutex);
}

static inline void netdev_rename_all_lock(void) {
    netdata_mutex_lock(&netdev_rename_mutex);

    struct netdev *d;
    for(d = netdev_root; d ; d = d->next)
        netdev_rename(d);

    netdev_pending_renames = 0;
    netdata_mutex_unlock(&netdev_rename_mutex);
}

// ----------------------------------------------------------------------------
// netdev data collection

static void netdev_cleanup() {
    if(likely(netdev_found == netdev_added)) return;

    netdev_added = 0;
    struct netdev *d = netdev_root, *last = NULL;
    while(d) {
        if(unlikely(!d->updated)) {
            // info("Removing network device '%s', linked after '%s'", d->name, last?last->name:"ROOT");

            if(netdev_last_used == d)
                netdev_last_used = last;

            struct netdev *t = d;

            if(d == netdev_root || !last)
                netdev_root = d = d->next;

            else
                last->next = d = d->next;

            t->next = NULL;
            netdev_free(t);
        }
        else {
            netdev_added++;
            last = d;
            d->updated = 0;
            d = d->next;
        }
    }
}

static struct netdev *get_netdev(const char *name) {
    struct netdev *d;

    uint32_t hash = simple_hash(name);

    // search it, from the last position to the end
    for(d = netdev_last_used ; d ; d = d->next) {
        if(unlikely(hash == d->hash && !strcmp(name, d->name))) {
            netdev_last_used = d->next;
            return d;
        }
    }

    // search it from the beginning to the last position we used
    for(d = netdev_root ; d != netdev_last_used ; d = d->next) {
        if(unlikely(hash == d->hash && !strcmp(name, d->name))) {
            netdev_last_used = d->next;
            return d;
        }
    }

    // create a new one
    d = callocz(1, sizeof(struct netdev));
    d->name = strdupz(name);
    d->hash = simple_hash(d->name);
    d->len = strlen(d->name);

    d->chart_type_net_bytes      = strdupz("net");
    d->chart_type_net_compressed = strdupz("net_compressed");
    d->chart_type_net_drops      = strdupz("net_drops");
    d->chart_type_net_errors     = strdupz("net_errors");
    d->chart_type_net_events     = strdupz("net_events");
    d->chart_type_net_fifo       = strdupz("net_fifo");
    d->chart_type_net_packets    = strdupz("net_packets");

    d->chart_id_net_bytes      = strdupz(d->name);
    d->chart_id_net_compressed = strdupz(d->name);
    d->chart_id_net_drops      = strdupz(d->name);
    d->chart_id_net_errors     = strdupz(d->name);
    d->chart_id_net_events     = strdupz(d->name);
    d->chart_id_net_fifo       = strdupz(d->name);
    d->chart_id_net_packets    = strdupz(d->name);

    d->chart_family = strdupz(d->name);
    d->priority = 7000;

    netdev_rename_lock(d);

    netdev_added++;

    // link it to the end
    if(netdev_root) {
        struct netdev *e;
        for(e = netdev_root; e->next ; e = e->next) ;
        e->next = d;
    }
    else
        netdev_root = d;

    return d;
}

int do_proc_net_dev(int update_every, usec_t dt) {
    (void)dt;
    static SIMPLE_PATTERN *disabled_list = NULL;
    static procfile *ff = NULL;
    static int enable_new_interfaces = -1;
    static int do_bandwidth = -1, do_packets = -1, do_errors = -1, do_drops = -1, do_fifo = -1, do_compressed = -1, do_events = -1;

    if(unlikely(enable_new_interfaces == -1)) {
        enable_new_interfaces = config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "enable new interfaces detected at runtime", CONFIG_BOOLEAN_AUTO);

        do_bandwidth    = config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "bandwidth for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_packets      = config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "packets for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_errors       = config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "errors for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_drops        = config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "drops for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_fifo         = config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "fifo for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_compressed   = config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "compressed packets for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_events       = config_get_boolean_ondemand("plugin:proc:/proc/net/dev", "frames, collisions, carrier counters for all interfaces", CONFIG_BOOLEAN_AUTO);

        disabled_list = simple_pattern_create(config_get("plugin:proc:/proc/net/dev", "disable by default interfaces matching", "lo fireqos* *-ifb"), SIMPLE_PATTERN_EXACT);
    }

    if(unlikely(!ff)) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/proc/net/dev");
        ff = procfile_open(config_get("plugin:proc:/proc/net/dev", "filename to monitor", filename), " \t,:|", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    // rename all the devices, if we have pending renames
    if(unlikely(netdev_pending_renames))
        netdev_rename_all_lock();

    netdev_found = 0;

    size_t lines = procfile_lines(ff), l;
    for(l = 2; l < lines ;l++) {
        // require 17 words on each line
        if(unlikely(procfile_linewords(ff, l) < 17)) continue;

        struct netdev *d = get_netdev(procfile_lineword(ff, l, 0));
        d->updated = 1;
        netdev_found++;

        if(unlikely(!d->configured)) {
            // this is the first time we see this interface

            // remember we configured it
            d->configured = 1;

            d->enabled = enable_new_interfaces;

            if(d->enabled)
                d->enabled = !simple_pattern_matches(disabled_list, d->name);

            char var_name[512 + 1];
            snprintfz(var_name, 512, "plugin:proc:/proc/net/dev:%s", d->name);
            d->enabled = config_get_boolean_ondemand(var_name, "enabled", d->enabled);

            if(d->enabled == CONFIG_BOOLEAN_NO)
                continue;

            d->do_bandwidth  = config_get_boolean_ondemand(var_name, "bandwidth",  do_bandwidth);
            d->do_packets    = config_get_boolean_ondemand(var_name, "packets",    do_packets);
            d->do_errors     = config_get_boolean_ondemand(var_name, "errors",     do_errors);
            d->do_drops      = config_get_boolean_ondemand(var_name, "drops",      do_drops);
            d->do_fifo       = config_get_boolean_ondemand(var_name, "fifo",       do_fifo);
            d->do_compressed = config_get_boolean_ondemand(var_name, "compressed", do_compressed);
            d->do_events     = config_get_boolean_ondemand(var_name, "events",     do_events);
        }

        if(unlikely(!d->enabled))
            continue;

        if(likely(d->do_bandwidth != CONFIG_BOOLEAN_NO)) {
            d->rbytes      = str2kernel_uint_t(procfile_lineword(ff, l, 1));
            d->tbytes      = str2kernel_uint_t(procfile_lineword(ff, l, 9));
        }

        if(likely(d->do_packets != CONFIG_BOOLEAN_NO)) {
            d->rpackets    = str2kernel_uint_t(procfile_lineword(ff, l, 2));
            d->rmulticast  = str2kernel_uint_t(procfile_lineword(ff, l, 8));
            d->tpackets    = str2kernel_uint_t(procfile_lineword(ff, l, 10));
        }

        if(likely(d->do_errors != CONFIG_BOOLEAN_NO)) {
            d->rerrors     = str2kernel_uint_t(procfile_lineword(ff, l, 3));
            d->terrors     = str2kernel_uint_t(procfile_lineword(ff, l, 11));
        }

        if(likely(d->do_drops != CONFIG_BOOLEAN_NO)) {
            d->rdrops      = str2kernel_uint_t(procfile_lineword(ff, l, 4));
            d->tdrops      = str2kernel_uint_t(procfile_lineword(ff, l, 12));
        }

        if(likely(d->do_fifo != CONFIG_BOOLEAN_NO)) {
            d->rfifo       = str2kernel_uint_t(procfile_lineword(ff, l, 5));
            d->tfifo       = str2kernel_uint_t(procfile_lineword(ff, l, 13));
        }

        if(likely(d->do_compressed != CONFIG_BOOLEAN_NO)) {
            d->rcompressed = str2kernel_uint_t(procfile_lineword(ff, l, 7));
            d->tcompressed = str2kernel_uint_t(procfile_lineword(ff, l, 16));
        }

        if(likely(d->do_events != CONFIG_BOOLEAN_NO)) {
            d->rframe      = str2kernel_uint_t(procfile_lineword(ff, l, 6));
            d->tcollisions = str2kernel_uint_t(procfile_lineword(ff, l, 14));
            d->tcarrier    = str2kernel_uint_t(procfile_lineword(ff, l, 15));
        }

        // --------------------------------------------------------------------

        if(unlikely((d->do_bandwidth == CONFIG_BOOLEAN_AUTO && (d->rbytes || d->tbytes))))
            d->do_bandwidth = CONFIG_BOOLEAN_YES;

        if(d->do_bandwidth == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_bandwidth)) {

                d->st_bandwidth = rrdset_create_localhost(
                        d->chart_type_net_bytes
                        , d->chart_id_net_bytes
                        , NULL
                        , d->chart_family
                        , "net.net"
                        , "Bandwidth"
                        , "kilobits/s"
                        , "proc"
                        , "net/dev"
                        , d->priority
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                d->rd_rbytes = rrddim_add(d->st_bandwidth, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tbytes = rrddim_add(d->st_bandwidth, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/trasmit

                    RRDDIM *td = d->rd_rbytes;
                    d->rd_rbytes = d->rd_tbytes;
                    d->rd_tbytes = td;
                }
            }
            else rrdset_next(d->st_bandwidth);

            rrddim_set_by_pointer(d->st_bandwidth, d->rd_rbytes, (collected_number)d->rbytes);
            rrddim_set_by_pointer(d->st_bandwidth, d->rd_tbytes, (collected_number)d->tbytes);
            rrdset_done(d->st_bandwidth);
        }

        // --------------------------------------------------------------------

        if(unlikely((d->do_packets == CONFIG_BOOLEAN_AUTO && (d->rpackets || d->tpackets || d->rmulticast))))
            d->do_packets = CONFIG_BOOLEAN_YES;

        if(d->do_packets == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_packets)) {

                d->st_packets = rrdset_create_localhost(
                        d->chart_type_net_packets
                        , d->chart_id_net_packets
                        , NULL
                        , d->chart_family
                        , "net.packets"
                        , "Packets"
                        , "packets/s"
                        , "proc"
                        , "net/dev"
                        , d->priority + 1
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_packets, RRDSET_FLAG_DETAIL);

                d->rd_rpackets   = rrddim_add(d->st_packets, "received",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tpackets   = rrddim_add(d->st_packets, "sent",      NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_rmulticast = rrddim_add(d->st_packets, "multicast", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/trasmit

                    RRDDIM *td = d->rd_rpackets;
                    d->rd_rpackets = d->rd_tpackets;
                    d->rd_tpackets = td;
                }
            }
            else rrdset_next(d->st_packets);

            rrddim_set_by_pointer(d->st_packets, d->rd_rpackets, (collected_number)d->rpackets);
            rrddim_set_by_pointer(d->st_packets, d->rd_tpackets, (collected_number)d->tpackets);
            rrddim_set_by_pointer(d->st_packets, d->rd_rmulticast, (collected_number)d->rmulticast);
            rrdset_done(d->st_packets);
        }

        // --------------------------------------------------------------------

        if(unlikely((d->do_errors == CONFIG_BOOLEAN_AUTO && (d->rerrors || d->terrors))))
            d->do_errors = CONFIG_BOOLEAN_YES;

        if(d->do_errors == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_errors)) {

                d->st_errors = rrdset_create_localhost(
                        d->chart_type_net_errors
                        , d->chart_id_net_errors
                        , NULL
                        , d->chart_family
                        , "net.errors"
                        , "Interface Errors"
                        , "errors/s"
                        , "proc"
                        , "net/dev"
                        , d->priority + 2
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_errors, RRDSET_FLAG_DETAIL);

                d->rd_rerrors = rrddim_add(d->st_errors, "inbound",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_terrors = rrddim_add(d->st_errors, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/trasmit

                    RRDDIM *td = d->rd_rerrors;
                    d->rd_rerrors = d->rd_terrors;
                    d->rd_terrors = td;
                }
            }
            else rrdset_next(d->st_errors);

            rrddim_set_by_pointer(d->st_errors, d->rd_rerrors, (collected_number)d->rerrors);
            rrddim_set_by_pointer(d->st_errors, d->rd_terrors, (collected_number)d->terrors);
            rrdset_done(d->st_errors);
        }

        // --------------------------------------------------------------------

        if(unlikely((d->do_drops == CONFIG_BOOLEAN_AUTO && (d->rdrops || d->tdrops))))
            d->do_drops = CONFIG_BOOLEAN_YES;

        if(d->do_drops == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_drops)) {

                d->st_drops = rrdset_create_localhost(
                        d->chart_type_net_drops
                        , d->chart_id_net_drops
                        , NULL
                        , d->chart_family
                        , "net.drops"
                        , "Interface Drops"
                        , "drops/s"
                        , "proc"
                        , "net/dev"
                        , d->priority + 3
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_drops, RRDSET_FLAG_DETAIL);

                d->rd_rdrops = rrddim_add(d->st_drops, "inbound",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tdrops = rrddim_add(d->st_drops, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/trasmit

                    RRDDIM *td = d->rd_rdrops;
                    d->rd_rdrops = d->rd_tdrops;
                    d->rd_tdrops = td;
                }
            }
            else rrdset_next(d->st_drops);

            rrddim_set_by_pointer(d->st_drops, d->rd_rdrops, (collected_number)d->rdrops);
            rrddim_set_by_pointer(d->st_drops, d->rd_tdrops, (collected_number)d->tdrops);
            rrdset_done(d->st_drops);
        }

        // --------------------------------------------------------------------

        if(unlikely((d->do_fifo == CONFIG_BOOLEAN_AUTO && (d->rfifo || d->tfifo))))
            d->do_fifo = CONFIG_BOOLEAN_YES;

        if(d->do_fifo == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_fifo)) {

                d->st_fifo = rrdset_create_localhost(
                        d->chart_type_net_fifo
                        , d->chart_id_net_fifo
                        , NULL
                        , d->chart_family
                        , "net.fifo"
                        , "Interface FIFO Buffer Errors"
                        , "errors"
                        , "proc"
                        , "net/dev"
                        , d->priority + 4
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_fifo, RRDSET_FLAG_DETAIL);

                d->rd_rfifo = rrddim_add(d->st_fifo, "receive",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tfifo = rrddim_add(d->st_fifo, "transmit", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/trasmit

                    RRDDIM *td = d->rd_rfifo;
                    d->rd_rfifo = d->rd_tfifo;
                    d->rd_tfifo = td;
                }
            }
            else rrdset_next(d->st_fifo);

            rrddim_set_by_pointer(d->st_fifo, d->rd_rfifo, (collected_number)d->rfifo);
            rrddim_set_by_pointer(d->st_fifo, d->rd_tfifo, (collected_number)d->tfifo);
            rrdset_done(d->st_fifo);
        }

        // --------------------------------------------------------------------

        if(unlikely((d->do_compressed == CONFIG_BOOLEAN_AUTO && (d->rcompressed || d->tcompressed))))
            d->do_compressed = CONFIG_BOOLEAN_YES;

        if(d->do_compressed == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_compressed)) {

                d->st_compressed = rrdset_create_localhost(
                        d->chart_type_net_compressed
                        , d->chart_id_net_compressed
                        , NULL
                        , d->chart_family
                        , "net.compressed"
                        , "Compressed Packets"
                        , "packets/s"
                        , "proc"
                        , "net/dev"
                        , d->priority + 5
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_compressed, RRDSET_FLAG_DETAIL);

                d->rd_rcompressed = rrddim_add(d->st_compressed, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tcompressed = rrddim_add(d->st_compressed, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/trasmit

                    RRDDIM *td = d->rd_rcompressed;
                    d->rd_rcompressed = d->rd_tcompressed;
                    d->rd_tcompressed = td;
                }
            }
            else rrdset_next(d->st_compressed);

            rrddim_set_by_pointer(d->st_compressed, d->rd_rcompressed, (collected_number)d->rcompressed);
            rrddim_set_by_pointer(d->st_compressed, d->rd_tcompressed, (collected_number)d->tcompressed);
            rrdset_done(d->st_compressed);
        }

        // --------------------------------------------------------------------

        if(unlikely((d->do_events == CONFIG_BOOLEAN_AUTO && (d->rframe || d->tcollisions || d->tcarrier))))
            d->do_events = CONFIG_BOOLEAN_YES;

        if(d->do_events == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_events)) {

                d->st_events = rrdset_create_localhost(
                        d->chart_type_net_events
                        , d->chart_id_net_events
                        , NULL
                        , d->chart_family
                        , "net.events"
                        , "Network Interface Events"
                        , "events/s"
                        , "proc"
                        , "net/dev"
                        , d->priority + 6
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_events, RRDSET_FLAG_DETAIL);

                d->rd_rframe      = rrddim_add(d->st_events, "frames",     NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tcollisions = rrddim_add(d->st_events, "collisions", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tcarrier    = rrddim_add(d->st_events, "carrier",    NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            }
            else rrdset_next(d->st_events);

            rrddim_set_by_pointer(d->st_events, d->rd_rframe,      (collected_number)d->rframe);
            rrddim_set_by_pointer(d->st_events, d->rd_tcollisions, (collected_number)d->tcollisions);
            rrddim_set_by_pointer(d->st_events, d->rd_tcarrier,    (collected_number)d->tcarrier);
            rrdset_done(d->st_events);
        }
    }

    netdev_cleanup();

    return 0;
}
