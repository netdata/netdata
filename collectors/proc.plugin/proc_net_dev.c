// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_NETDEV_NAME "/proc/net/dev"
#define CONFIG_SECTION_PLUGIN_PROC_NETDEV "plugin:" PLUGIN_PROC_CONFIG_NAME ":" PLUGIN_PROC_MODULE_NETDEV_NAME

#define STATE_LENGTH_MAX 32

// As defined in https://www.kernel.org/doc/Documentation/ABI/testing/sysfs-class-net
const char *operstate_names[] = { "unknown", "notpresent", "down", "lowerlayerdown", "testing", "dormant", "up" };

static inline int get_operstate(char *operstate)
{
    int i;

    for (i = 0; i < (int) (sizeof(operstate_names) / sizeof(char *)); i++) {
        if (!strcmp(operstate, operstate_names[i])) {
            return i;
        }
    }
    return 0;
}

// ----------------------------------------------------------------------------
// netdev list

static struct netdev {
    char *name;
    uint32_t hash;
    size_t len;

    // flags
    int virtual;
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
    int do_speed;
    int do_duplex;
    int do_operstate;
    int do_carrier;
    int do_mtu;

    const char *chart_type_net_bytes;
    const char *chart_type_net_packets;
    const char *chart_type_net_errors;
    const char *chart_type_net_fifo;
    const char *chart_type_net_events;
    const char *chart_type_net_drops;
    const char *chart_type_net_compressed;
    const char *chart_type_net_speed;
    const char *chart_type_net_duplex;
    const char *chart_type_net_operstate;
    const char *chart_type_net_carrier;
    const char *chart_type_net_mtu;

    const char *chart_id_net_bytes;
    const char *chart_id_net_packets;
    const char *chart_id_net_errors;
    const char *chart_id_net_fifo;
    const char *chart_id_net_events;
    const char *chart_id_net_drops;
    const char *chart_id_net_compressed;
    const char *chart_id_net_speed;
    const char *chart_id_net_duplex;
    const char *chart_id_net_operstate;
    const char *chart_id_net_carrier;
    const char *chart_id_net_mtu;

    const char *chart_ctx_net_bytes;
    const char *chart_ctx_net_packets;
    const char *chart_ctx_net_errors;
    const char *chart_ctx_net_fifo;
    const char *chart_ctx_net_events;
    const char *chart_ctx_net_drops;
    const char *chart_ctx_net_compressed;
    const char *chart_ctx_net_speed;
    const char *chart_ctx_net_duplex;
    const char *chart_ctx_net_operstate;
    const char *chart_ctx_net_carrier;
    const char *chart_ctx_net_mtu;

    const char *chart_family;

    struct label *chart_labels;

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
    kernel_uint_t speed;
    kernel_uint_t duplex;
    kernel_uint_t operstate;
    unsigned long long carrier;
    unsigned long long mtu;

    // charts
    RRDSET *st_bandwidth;
    RRDSET *st_packets;
    RRDSET *st_errors;
    RRDSET *st_drops;
    RRDSET *st_fifo;
    RRDSET *st_compressed;
    RRDSET *st_events;
    RRDSET *st_speed;
    RRDSET *st_duplex;
    RRDSET *st_operstate;
    RRDSET *st_carrier;
    RRDSET *st_mtu;

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

    RRDDIM *rd_speed;
    RRDDIM *rd_duplex;
    RRDDIM *rd_operstate;
    RRDDIM *rd_carrier;
    RRDDIM *rd_mtu;

    char *filename_speed;
    RRDSETVAR *chart_var_speed;

    char *filename_duplex;
    char *filename_operstate;
    char *filename_carrier;
    char *filename_mtu;

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
    if(d->st_speed)      rrdset_is_obsolete(d->st_speed);
    if(d->st_duplex)     rrdset_is_obsolete(d->st_duplex);
    if(d->st_operstate)  rrdset_is_obsolete(d->st_operstate);
    if(d->st_carrier)    rrdset_is_obsolete(d->st_carrier);
    if(d->st_mtu)        rrdset_is_obsolete(d->st_mtu);

    d->st_bandwidth   = NULL;
    d->st_compressed  = NULL;
    d->st_drops       = NULL;
    d->st_errors      = NULL;
    d->st_events      = NULL;
    d->st_fifo        = NULL;
    d->st_packets     = NULL;
    d->st_speed       = NULL;
    d->st_duplex      = NULL;
    d->st_operstate   = NULL;
    d->st_carrier     = NULL;
    d->st_mtu         = NULL;

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

    d->rd_speed       = NULL;
    d->rd_duplex      = NULL;
    d->rd_operstate   = NULL;
    d->rd_carrier     = NULL;
    d->rd_mtu         = NULL;

    d->chart_var_speed     = NULL;
}

static void netdev_free_chart_strings(struct netdev *d) {
    freez((void *)d->chart_type_net_bytes);
    freez((void *)d->chart_type_net_compressed);
    freez((void *)d->chart_type_net_drops);
    freez((void *)d->chart_type_net_errors);
    freez((void *)d->chart_type_net_events);
    freez((void *)d->chart_type_net_fifo);
    freez((void *)d->chart_type_net_packets);
    freez((void *)d->chart_type_net_speed);
    freez((void *)d->chart_type_net_duplex);
    freez((void *)d->chart_type_net_operstate);
    freez((void *)d->chart_type_net_carrier);
    freez((void *)d->chart_type_net_mtu);

    freez((void *)d->chart_id_net_bytes);
    freez((void *)d->chart_id_net_compressed);
    freez((void *)d->chart_id_net_drops);
    freez((void *)d->chart_id_net_errors);
    freez((void *)d->chart_id_net_events);
    freez((void *)d->chart_id_net_fifo);
    freez((void *)d->chart_id_net_packets);
    freez((void *)d->chart_id_net_speed);
    freez((void *)d->chart_id_net_duplex);
    freez((void *)d->chart_id_net_operstate);
    freez((void *)d->chart_id_net_carrier);
    freez((void *)d->chart_id_net_mtu);

    freez((void *)d->chart_ctx_net_bytes);
    freez((void *)d->chart_ctx_net_compressed);
    freez((void *)d->chart_ctx_net_drops);
    freez((void *)d->chart_ctx_net_errors);
    freez((void *)d->chart_ctx_net_events);
    freez((void *)d->chart_ctx_net_fifo);
    freez((void *)d->chart_ctx_net_packets);
    freez((void *)d->chart_ctx_net_speed);
    freez((void *)d->chart_ctx_net_duplex);
    freez((void *)d->chart_ctx_net_operstate);
    freez((void *)d->chart_ctx_net_carrier);
    freez((void *)d->chart_ctx_net_mtu);

    freez((void *)d->chart_family);
}

static void netdev_free(struct netdev *d) {
    netdev_charts_release(d);
    netdev_free_chart_strings(d);
    free_label_list(d->chart_labels);

    freez((void *)d->name);
    freez((void *)d->filename_speed);
    freez((void *)d->filename_duplex);
    freez((void *)d->filename_operstate);
    freez((void *)d->filename_carrier);
    freez((void *)d->filename_mtu);
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

    struct label *chart_labels;

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
void netdev_rename_device_add(
    const char *host_device, const char *container_device, const char *container_name, struct label *labels)
{
    netdata_mutex_lock(&netdev_rename_mutex);

    uint32_t hash = simple_hash(host_device);
    struct netdev_rename *r = netdev_rename_find(host_device, hash);
    if(!r) {
        r = callocz(1, sizeof(struct netdev_rename));
        r->host_device      = strdupz(host_device);
        r->container_device = strdupz(container_device);
        r->container_name   = strdupz(container_name);
        update_label_list(&r->chart_labels, labels);
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

            update_label_list(&r->chart_labels, labels);
            
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
            free_label_list(r->chart_labels);
            freez((void *) r);
            break;
        }
    }

    netdata_mutex_unlock(&netdev_rename_mutex);
}

static inline void netdev_rename_cgroup(struct netdev *d, struct netdev_rename *r) {
    info("CGROUP: renaming network interface '%s' as '%s' under '%s'", r->host_device, r->container_device, r->container_name);

    netdev_charts_release(d);
    netdev_free_chart_strings(d);

    char buffer[RRD_ID_LENGTH_MAX + 1];

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "cgroup_%s", r->container_name);
    d->chart_type_net_bytes      = strdupz(buffer);
    d->chart_type_net_compressed = strdupz(buffer);
    d->chart_type_net_drops      = strdupz(buffer);
    d->chart_type_net_errors     = strdupz(buffer);
    d->chart_type_net_events     = strdupz(buffer);
    d->chart_type_net_fifo       = strdupz(buffer);
    d->chart_type_net_packets    = strdupz(buffer);
    d->chart_type_net_speed      = strdupz(buffer);
    d->chart_type_net_duplex     = strdupz(buffer);
    d->chart_type_net_operstate  = strdupz(buffer);
    d->chart_type_net_carrier    = strdupz(buffer);
    d->chart_type_net_mtu        = strdupz(buffer);

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
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_speed_%s", r->container_device);
    d->chart_id_net_speed      = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_duplex_%s", r->container_device);
    d->chart_id_net_duplex     = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_operstate_%s", r->container_device);
    d->chart_id_net_operstate  = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_carrier_%s", r->container_device);
    d->chart_id_net_carrier    = strdupz(buffer);
    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net_mtu_%s", r->container_device);
    d->chart_id_net_mtu        = strdupz(buffer);

    d->chart_ctx_net_bytes      = strdupz("cgroup.net_net");
    d->chart_ctx_net_compressed = strdupz("cgroup.net_compressed");
    d->chart_ctx_net_drops      = strdupz("cgroup.net_drops");
    d->chart_ctx_net_errors     = strdupz("cgroup.net_errors");
    d->chart_ctx_net_events     = strdupz("cgroup.net_events");
    d->chart_ctx_net_fifo       = strdupz("cgroup.net_fifo");
    d->chart_ctx_net_packets    = strdupz("cgroup.net_packets");
    d->chart_ctx_net_speed      = strdupz("cgroup.net_speed");
    d->chart_ctx_net_duplex     = strdupz("cgroup.net_duplex");
    d->chart_ctx_net_operstate  = strdupz("cgroup.net_operstate");
    d->chart_ctx_net_carrier    = strdupz("cgroup.net_carrier");
    d->chart_ctx_net_mtu        = strdupz("cgroup.net_mtu");

    snprintfz(buffer, RRD_ID_LENGTH_MAX, "net %s", r->container_device);
    d->chart_family = strdupz(buffer);

    update_label_list(&d->chart_labels, r->chart_labels);

    d->priority = NETDATA_CHART_PRIO_CGROUP_NET_IFACE;
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
    d->chart_type_net_speed      = strdupz("net_speed");
    d->chart_type_net_duplex     = strdupz("net_duplex");
    d->chart_type_net_operstate  = strdupz("net_operstate");
    d->chart_type_net_carrier    = strdupz("net_carrier");
    d->chart_type_net_mtu        = strdupz("net_mtu");

    d->chart_id_net_bytes      = strdupz(d->name);
    d->chart_id_net_compressed = strdupz(d->name);
    d->chart_id_net_drops      = strdupz(d->name);
    d->chart_id_net_errors     = strdupz(d->name);
    d->chart_id_net_events     = strdupz(d->name);
    d->chart_id_net_fifo       = strdupz(d->name);
    d->chart_id_net_packets    = strdupz(d->name);
    d->chart_id_net_speed      = strdupz(d->name);
    d->chart_id_net_duplex     = strdupz(d->name);
    d->chart_id_net_operstate  = strdupz(d->name);
    d->chart_id_net_carrier    = strdupz(d->name);
    d->chart_id_net_mtu        = strdupz(d->name);

    d->chart_ctx_net_bytes      = strdupz("net.net");
    d->chart_ctx_net_compressed = strdupz("net.compressed");
    d->chart_ctx_net_drops      = strdupz("net.drops");
    d->chart_ctx_net_errors     = strdupz("net.errors");
    d->chart_ctx_net_events     = strdupz("net.events");
    d->chart_ctx_net_fifo       = strdupz("net.fifo");
    d->chart_ctx_net_packets    = strdupz("net.packets");
    d->chart_ctx_net_speed      = strdupz("net.speed");
    d->chart_ctx_net_duplex     = strdupz("net.duplex");
    d->chart_ctx_net_operstate  = strdupz("net.operstate");
    d->chart_ctx_net_carrier    = strdupz("net.carrier");
    d->chart_ctx_net_mtu        = strdupz("net.mtu");

    d->chart_family = strdupz(d->name);
    d->priority = NETDATA_CHART_PRIO_FIRST_NET_IFACE;

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
    static int do_bandwidth = -1, do_packets = -1, do_errors = -1, do_drops = -1, do_fifo = -1, do_compressed = -1,
               do_events = -1, do_speed = -1, do_duplex = -1, do_operstate = -1, do_carrier = -1, do_mtu = -1;
    static char *path_to_sys_devices_virtual_net = NULL, *path_to_sys_class_net_speed = NULL,
                *proc_net_dev_filename = NULL;
    static char *path_to_sys_class_net_duplex = NULL;
    static char *path_to_sys_class_net_operstate = NULL;
    static char *path_to_sys_class_net_carrier = NULL;
    static char *path_to_sys_class_net_mtu = NULL;

    if(unlikely(enable_new_interfaces == -1)) {
        char filename[FILENAME_MAX + 1];

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, (*netdata_configured_host_prefix)?"/proc/1/net/dev":"/proc/net/dev");
        proc_net_dev_filename = config_get(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "filename to monitor", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/devices/virtual/net/%s");
        path_to_sys_devices_virtual_net = config_get(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "path to get virtual interfaces", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/net/%s/speed");
        path_to_sys_class_net_speed = config_get(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "path to get net device speed", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/net/%s/duplex");
        path_to_sys_class_net_duplex = config_get(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "path to get net device duplex", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/net/%s/operstate");
        path_to_sys_class_net_operstate = config_get(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "path to get net device operstate", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/net/%s/carrier");
        path_to_sys_class_net_carrier = config_get(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "path to get net device carrier", filename);

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/class/net/%s/mtu");
        path_to_sys_class_net_mtu = config_get(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "path to get net device mtu", filename);


        enable_new_interfaces = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "enable new interfaces detected at runtime", CONFIG_BOOLEAN_AUTO);

        do_bandwidth    = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "bandwidth for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_packets      = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "packets for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_errors       = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "errors for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_drops        = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "drops for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_fifo         = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "fifo for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_compressed   = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "compressed packets for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_events       = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "frames, collisions, carrier counters for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_speed        = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "speed for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_duplex       = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "duplex for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_operstate    = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "operstate for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_carrier      = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "carrier for all interfaces", CONFIG_BOOLEAN_AUTO);
        do_mtu          = config_get_boolean_ondemand(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "mtu for all interfaces", CONFIG_BOOLEAN_AUTO);

        disabled_list = simple_pattern_create(config_get(CONFIG_SECTION_PLUGIN_PROC_NETDEV, "disable by default interfaces matching", "lo fireqos* *-ifb fwpr* fwbr* fwln*"), NULL, SIMPLE_PATTERN_EXACT);
    }

    if(unlikely(!ff)) {
        ff = procfile_open(proc_net_dev_filename, " \t,|", PROCFILE_FLAG_DEFAULT);
        if(unlikely(!ff)) return 1;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) return 0; // we return 0, so that we will retry to open it next time

    // rename all the devices, if we have pending renames
    if(unlikely(netdev_pending_renames))
        netdev_rename_all_lock();

    netdev_found = 0;

    kernel_uint_t system_rbytes = 0;
    kernel_uint_t system_tbytes = 0;

    size_t lines = procfile_lines(ff), l;
    for(l = 2; l < lines ;l++) {
        // require 17 words on each line
        if(unlikely(procfile_linewords(ff, l) < 17)) continue;

        char *name = procfile_lineword(ff, l, 0);
        size_t len = strlen(name);
        if(name[len - 1] == ':') name[len - 1] = '\0';

        struct netdev *d = get_netdev(name);
        d->updated = 1;
        netdev_found++;

        if(unlikely(!d->configured)) {
            // this is the first time we see this interface

            // remember we configured it
            d->configured = 1;

            d->enabled = enable_new_interfaces;

            if(d->enabled)
                d->enabled = !simple_pattern_matches(disabled_list, d->name);

            char buffer[FILENAME_MAX + 1];

            snprintfz(buffer, FILENAME_MAX, path_to_sys_devices_virtual_net, d->name);
            if(likely(access(buffer, R_OK) == 0)) {
                d->virtual = 1;
            }
            else
                d->virtual = 0;

            if(likely(!d->virtual)) {
                // set the filename to get the interface speed
                snprintfz(buffer, FILENAME_MAX, path_to_sys_class_net_speed, d->name);
                d->filename_speed = strdupz(buffer);

                snprintfz(buffer, FILENAME_MAX, path_to_sys_class_net_duplex, d->name);
                d->filename_duplex = strdupz(buffer);
            }

            snprintfz(buffer, FILENAME_MAX, path_to_sys_class_net_operstate, d->name);
            d->filename_operstate = strdupz(buffer);

            snprintfz(buffer, FILENAME_MAX, path_to_sys_class_net_carrier, d->name);
            d->filename_carrier = strdupz(buffer);

            snprintfz(buffer, FILENAME_MAX, path_to_sys_class_net_mtu, d->name);
            d->filename_mtu = strdupz(buffer);

            snprintfz(buffer, FILENAME_MAX, "plugin:proc:/proc/net/dev:%s", d->name);
            d->enabled = config_get_boolean_ondemand(buffer, "enabled", d->enabled);
            d->virtual = config_get_boolean(buffer, "virtual", d->virtual);

            if(d->enabled == CONFIG_BOOLEAN_NO)
                continue;

            d->do_bandwidth  = config_get_boolean_ondemand(buffer, "bandwidth",  do_bandwidth);
            d->do_packets    = config_get_boolean_ondemand(buffer, "packets",    do_packets);
            d->do_errors     = config_get_boolean_ondemand(buffer, "errors",     do_errors);
            d->do_drops      = config_get_boolean_ondemand(buffer, "drops",      do_drops);
            d->do_fifo       = config_get_boolean_ondemand(buffer, "fifo",       do_fifo);
            d->do_compressed = config_get_boolean_ondemand(buffer, "compressed", do_compressed);
            d->do_events     = config_get_boolean_ondemand(buffer, "events",     do_events);
            d->do_speed      = config_get_boolean_ondemand(buffer, "speed",      do_speed);
            d->do_duplex     = config_get_boolean_ondemand(buffer, "duplex",     do_duplex);
            d->do_operstate  = config_get_boolean_ondemand(buffer, "operstate",  do_operstate);
            d->do_carrier    = config_get_boolean_ondemand(buffer, "carrier",    do_carrier);
            d->do_mtu        = config_get_boolean_ondemand(buffer, "mtu",        do_mtu);
        }

        if(unlikely(!d->enabled))
            continue;

        if(likely(d->do_bandwidth != CONFIG_BOOLEAN_NO || !d->virtual)) {
            d->rbytes      = str2kernel_uint_t(procfile_lineword(ff, l, 1));
            d->tbytes      = str2kernel_uint_t(procfile_lineword(ff, l, 9));

            if(likely(!d->virtual)) {
                system_rbytes += d->rbytes;
                system_tbytes += d->tbytes;
            }
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

        if (d->do_duplex != CONFIG_BOOLEAN_NO && d->filename_duplex) {
            char buffer[STATE_LENGTH_MAX + 1];

            if (read_file(d->filename_duplex, buffer, STATE_LENGTH_MAX)) {
                error("Cannot refresh interface %s duplex state by reading '%s'. I will stop updating it.", d->name, d->filename_duplex);
                freez(d->filename_duplex);
                d->filename_duplex = NULL;
            } else {
                // values can be unknown, half or full -- just check the first letter for speed
                if (buffer[0] == 'f')
                    d->duplex = 2;
                else if (buffer[0] == 'h')
                    d->duplex = 1;
                else
                    d->duplex = 0;
            }
        }

        if(d->do_operstate != CONFIG_BOOLEAN_NO && d->filename_operstate) {
            char buffer[STATE_LENGTH_MAX + 1], *trimmed_buffer;

            if (read_file(d->filename_operstate, buffer, STATE_LENGTH_MAX)) {
                error(
                    "Cannot refresh %s operstate by reading '%s'. Will not update its status anymore.",
                    d->name, d->filename_operstate);
                freez(d->filename_operstate);
                d->filename_operstate = NULL;
            } else {
                trimmed_buffer = trim(buffer);
                d->operstate = get_operstate(trimmed_buffer);
            }
        }

        if (d->do_carrier != CONFIG_BOOLEAN_NO && d->filename_carrier) {
            if (read_single_number_file(d->filename_carrier, &d->carrier)) {
                error("Cannot refresh interface %s carrier state by reading '%s'. Stop updating it.", d->name, d->filename_carrier);
                freez(d->filename_carrier);
                d->filename_carrier = NULL;
            }
        }

        if (d->do_mtu != CONFIG_BOOLEAN_NO && d->filename_mtu) {
            if (read_single_number_file(d->filename_mtu, &d->mtu)) {
                error("Cannot refresh mtu for interface %s by reading '%s'. Stop updating it.", d->name, d->filename_carrier);
                freez(d->filename_carrier);
                d->filename_carrier = NULL;
            }
        }

        //info("PROC_NET_DEV: %s speed %zu, bytes %zu/%zu, packets %zu/%zu/%zu, errors %zu/%zu, drops %zu/%zu, fifo %zu/%zu, compressed %zu/%zu, rframe %zu, tcollisions %zu, tcarrier %zu"
        //        , d->name, d->speed
        //        , d->rbytes, d->tbytes
        //        , d->rpackets, d->tpackets, d->rmulticast
        //        , d->rerrors, d->terrors
        //        , d->rdrops, d->tdrops
        //        , d->rfifo, d->tfifo
        //        , d->rcompressed, d->tcompressed
        //        , d->rframe, d->tcollisions, d->tcarrier
        //        );

        // --------------------------------------------------------------------

        if(unlikely(d->do_bandwidth == CONFIG_BOOLEAN_AUTO &&
                    (d->rbytes || d->tbytes || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
            d->do_bandwidth = CONFIG_BOOLEAN_YES;

        if(d->do_bandwidth == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_bandwidth)) {

                d->st_bandwidth = rrdset_create_localhost(
                        d->chart_type_net_bytes
                        , d->chart_id_net_bytes
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_bytes
                        , "Bandwidth"
                        , "kilobits/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority
                        , update_every
                        , RRDSET_TYPE_AREA
                );

                rrdset_update_labels(d->st_bandwidth, d->chart_labels);

                d->rd_rbytes = rrddim_add(d->st_bandwidth, "received", NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tbytes = rrddim_add(d->st_bandwidth, "sent",     NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

                    RRDDIM *td = d->rd_rbytes;
                    d->rd_rbytes = d->rd_tbytes;
                    d->rd_tbytes = td;
                }
            }
            else rrdset_next(d->st_bandwidth);

            rrddim_set_by_pointer(d->st_bandwidth, d->rd_rbytes, (collected_number)d->rbytes);
            rrddim_set_by_pointer(d->st_bandwidth, d->rd_tbytes, (collected_number)d->tbytes);
            rrdset_done(d->st_bandwidth);

            // update the interface speed
            if(d->filename_speed) {
                if(unlikely(!d->chart_var_speed)) {
                    d->chart_var_speed = rrdsetvar_custom_chart_variable_create(d->st_bandwidth, "nic_speed_max");
                    if(!d->chart_var_speed) {
                        error("Cannot create interface %s chart variable 'nic_speed_max'. Will not update its speed anymore.", d->name);
                        freez(d->filename_speed);
                        d->filename_speed = NULL;
                    }
                }

                if(d->filename_speed && d->chart_var_speed) {
                    if(read_single_number_file(d->filename_speed, (unsigned long long *) &d->speed)) {
                        error("Cannot refresh interface %s speed by reading '%s'. Will not update its speed anymore.", d->name, d->filename_speed);
                        freez(d->filename_speed);
                        d->filename_speed = NULL;
                    }
                    else {
                        rrdsetvar_custom_chart_variable_set(d->chart_var_speed, (calculated_number) d->speed * KILOBITS_IN_A_MEGABIT);

                        if(d->do_speed != CONFIG_BOOLEAN_NO) {
                            if(unlikely(!d->st_speed)) {
                                d->st_speed = rrdset_create_localhost(
                                        d->chart_type_net_speed
                                        , d->chart_id_net_speed
                                        , NULL
                                        , d->chart_family
                                        , d->chart_ctx_net_speed
                                        , "Interface Speed"
                                        , "kilobits/s"
                                        , PLUGIN_PROC_NAME
                                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                                        , d->priority + 7
                                        , update_every
                                        , RRDSET_TYPE_LINE
                                );

                                rrdset_flag_set(d->st_speed, RRDSET_FLAG_DETAIL);

                                rrdset_update_labels(d->st_speed, d->chart_labels);

                                d->rd_speed = rrddim_add(d->st_speed, "speed",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
                            }
                            else rrdset_next(d->st_speed);

                            rrddim_set_by_pointer(d->st_speed, d->rd_speed, (collected_number)d->speed * KILOBITS_IN_A_MEGABIT);
                            rrdset_done(d->st_speed);
                        }
                    }
                }
            }
        }

        // --------------------------------------------------------------------

        if(d->do_duplex != CONFIG_BOOLEAN_NO && d->filename_duplex) {
            if(unlikely(!d->st_duplex)) {
                d->st_duplex = rrdset_create_localhost(
                        d->chart_type_net_duplex
                        , d->chart_id_net_duplex
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_duplex
                        , "Interface Duplex State"
                        , "state"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 8
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_duplex, RRDSET_FLAG_DETAIL);

                rrdset_update_labels(d->st_duplex, d->chart_labels);

                d->rd_duplex = rrddim_add(d->st_duplex, "duplex",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(d->st_duplex);

            rrddim_set_by_pointer(d->st_duplex, d->rd_duplex, (collected_number)d->duplex);
            rrdset_done(d->st_duplex);
        }

        // --------------------------------------------------------------------

        if(d->do_operstate != CONFIG_BOOLEAN_NO && d->filename_operstate) {
            if(unlikely(!d->st_operstate)) {
                d->st_operstate = rrdset_create_localhost(
                        d->chart_type_net_operstate
                        , d->chart_id_net_operstate
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_operstate
                        , "Interface Operational State"
                        , "state"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 9
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_operstate, RRDSET_FLAG_DETAIL);

                rrdset_update_labels(d->st_operstate, d->chart_labels);

                d->rd_operstate = rrddim_add(d->st_operstate, "state",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(d->st_operstate);

            rrddim_set_by_pointer(d->st_operstate, d->rd_operstate, (collected_number)d->operstate);
            rrdset_done(d->st_operstate);
        }

        // --------------------------------------------------------------------

        if(d->do_carrier != CONFIG_BOOLEAN_NO && d->filename_carrier) {
            if(unlikely(!d->st_carrier)) {
                d->st_carrier = rrdset_create_localhost(
                        d->chart_type_net_carrier
                        , d->chart_id_net_carrier
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_carrier
                        , "Interface Physical Link State"
                        , "state"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 10
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_carrier, RRDSET_FLAG_DETAIL);

                rrdset_update_labels(d->st_carrier, d->chart_labels);

                d->rd_carrier = rrddim_add(d->st_carrier, "carrier",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(d->st_carrier);

            rrddim_set_by_pointer(d->st_carrier, d->rd_carrier, (collected_number)d->carrier);
            rrdset_done(d->st_carrier);
        }

        // --------------------------------------------------------------------

        if(d->do_mtu != CONFIG_BOOLEAN_NO && d->filename_mtu) {
            if(unlikely(!d->st_mtu)) {
                d->st_mtu = rrdset_create_localhost(
                        d->chart_type_net_mtu
                        , d->chart_id_net_mtu
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_mtu
                        , "Interface MTU"
                        , "octets"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 11
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_mtu, RRDSET_FLAG_DETAIL);

                rrdset_update_labels(d->st_mtu, d->chart_labels);

                d->rd_mtu = rrddim_add(d->st_mtu, "mtu",  NULL,  1, 1, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(d->st_mtu);

            rrddim_set_by_pointer(d->st_mtu, d->rd_mtu, (collected_number)d->mtu);
            rrdset_done(d->st_mtu);
        }

        // --------------------------------------------------------------------

        if(unlikely(d->do_packets == CONFIG_BOOLEAN_AUTO &&
           (d->rpackets || d->tpackets || d->rmulticast || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
            d->do_packets = CONFIG_BOOLEAN_YES;

        if(d->do_packets == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_packets)) {

                d->st_packets = rrdset_create_localhost(
                        d->chart_type_net_packets
                        , d->chart_id_net_packets
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_packets
                        , "Packets"
                        , "packets/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 1
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_packets, RRDSET_FLAG_DETAIL);

                rrdset_update_labels(d->st_packets, d->chart_labels);

                d->rd_rpackets   = rrddim_add(d->st_packets, "received",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tpackets   = rrddim_add(d->st_packets, "sent",      NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_rmulticast = rrddim_add(d->st_packets, "multicast", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

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

        if(unlikely(d->do_errors == CONFIG_BOOLEAN_AUTO &&
                    (d->rerrors || d->terrors || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
            d->do_errors = CONFIG_BOOLEAN_YES;

        if(d->do_errors == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_errors)) {

                d->st_errors = rrdset_create_localhost(
                        d->chart_type_net_errors
                        , d->chart_id_net_errors
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_errors
                        , "Interface Errors"
                        , "errors/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 2
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_errors, RRDSET_FLAG_DETAIL);

                rrdset_update_labels(d->st_errors, d->chart_labels);

                d->rd_rerrors = rrddim_add(d->st_errors, "inbound",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_terrors = rrddim_add(d->st_errors, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

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

        if(unlikely(d->do_drops == CONFIG_BOOLEAN_AUTO &&
                    (d->rdrops || d->tdrops || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
            d->do_drops = CONFIG_BOOLEAN_YES;

        if(d->do_drops == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_drops)) {

                d->st_drops = rrdset_create_localhost(
                        d->chart_type_net_drops
                        , d->chart_id_net_drops
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_drops
                        , "Interface Drops"
                        , "drops/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 3
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_drops, RRDSET_FLAG_DETAIL);

                rrdset_update_labels(d->st_drops, d->chart_labels);

                d->rd_rdrops = rrddim_add(d->st_drops, "inbound",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tdrops = rrddim_add(d->st_drops, "outbound", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

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

        if(unlikely(d->do_fifo == CONFIG_BOOLEAN_AUTO &&
                    (d->rfifo || d->tfifo || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
            d->do_fifo = CONFIG_BOOLEAN_YES;

        if(d->do_fifo == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_fifo)) {

                d->st_fifo = rrdset_create_localhost(
                        d->chart_type_net_fifo
                        , d->chart_id_net_fifo
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_fifo
                        , "Interface FIFO Buffer Errors"
                        , "errors"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 4
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_fifo, RRDSET_FLAG_DETAIL);

                rrdset_update_labels(d->st_fifo, d->chart_labels);

                d->rd_rfifo = rrddim_add(d->st_fifo, "receive",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tfifo = rrddim_add(d->st_fifo, "transmit", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

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

        if(unlikely(d->do_compressed == CONFIG_BOOLEAN_AUTO &&
                    (d->rcompressed || d->tcompressed || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
            d->do_compressed = CONFIG_BOOLEAN_YES;

        if(d->do_compressed == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_compressed)) {

                d->st_compressed = rrdset_create_localhost(
                        d->chart_type_net_compressed
                        , d->chart_id_net_compressed
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_compressed
                        , "Compressed Packets"
                        , "packets/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 5
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_compressed, RRDSET_FLAG_DETAIL);

                rrdset_update_labels(d->st_compressed, d->chart_labels);

                d->rd_rcompressed = rrddim_add(d->st_compressed, "received", NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
                d->rd_tcompressed = rrddim_add(d->st_compressed, "sent",     NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);

                if(d->flipped) {
                    // flip receive/transmit

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

        if(unlikely(d->do_events == CONFIG_BOOLEAN_AUTO &&
                    (d->rframe || d->tcollisions || d->tcarrier || netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES)))
            d->do_events = CONFIG_BOOLEAN_YES;

        if(d->do_events == CONFIG_BOOLEAN_YES) {
            if(unlikely(!d->st_events)) {

                d->st_events = rrdset_create_localhost(
                        d->chart_type_net_events
                        , d->chart_id_net_events
                        , NULL
                        , d->chart_family
                        , d->chart_ctx_net_events
                        , "Network Interface Events"
                        , "events/s"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_NETDEV_NAME
                        , d->priority + 6
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                rrdset_flag_set(d->st_events, RRDSET_FLAG_DETAIL);

                rrdset_update_labels(d->st_events, d->chart_labels);

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

    if(do_bandwidth == CONFIG_BOOLEAN_YES || (do_bandwidth == CONFIG_BOOLEAN_AUTO &&
                                              (system_rbytes || system_tbytes ||
                                               netdata_zero_metrics_enabled == CONFIG_BOOLEAN_YES))) {
        do_bandwidth = CONFIG_BOOLEAN_YES;
        static RRDSET *st_system_net = NULL;
        static RRDDIM *rd_in = NULL, *rd_out = NULL;

        if(unlikely(!st_system_net)) {
            st_system_net = rrdset_create_localhost(
                    "system"
                    , "net"
                    , NULL
                    , "network"
                    , NULL
                    , "Physical Network Interfaces Aggregated Bandwidth"
                    , "kilobits/s"
                    , PLUGIN_PROC_NAME
                    , PLUGIN_PROC_MODULE_NETDEV_NAME
                    , NETDATA_CHART_PRIO_SYSTEM_NET
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_in  = rrddim_add(st_system_net, "InOctets",  "received", 8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            rd_out = rrddim_add(st_system_net, "OutOctets", "sent",    -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }
        else
            rrdset_next(st_system_net);

        rrddim_set_by_pointer(st_system_net, rd_in,  (collected_number)system_rbytes);
        rrddim_set_by_pointer(st_system_net, rd_out, (collected_number)system_tbytes);

        rrdset_done(st_system_net);
    }

    netdev_cleanup();

    return 0;
}

static void netdev_main_cleanup(void *ptr)
{
    UNUSED(ptr);

    info("cleaning up...");

    worker_unregister();
}

void *netdev_main(void *ptr)
{
    worker_register("NETDEV");
    worker_register_job_name(0, "netdev");

    netdata_thread_cleanup_push(netdev_main_cleanup, ptr);

    usec_t step = localhost->rrd_update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);

    while (!netdata_exit) {
        worker_is_idle();
        usec_t hb_dt = heartbeat_next(&hb, step);

        if (unlikely(netdata_exit))
            break;

        worker_is_busy(0);
        if(do_proc_net_dev(localhost->rrd_update_every, hb_dt))
            break;
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
