// SPDX-License-Identifier: GPL-3.0-or-later

// pfvar.h pulls sys/refcount.h, whose function names collide with libnetdata's refcount macros,
// so the system headers must be parsed before libnetdata.
#include <fcntl.h>
#include <sys/types.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <sys/nv.h>
#include <net/if.h>
#include <netinet/in.h>
#include <net/pfvar.h>

#include "plugin_freebsd.h"

// pf(4) packet filter metrics (pfSense, OPNsense and stock FreeBSD hosts running pf).
//
// Global status is read with DIOCGETSTATUSNV and per-interface counters with DIOCIGETIFACES.
// Both ioctls are allowed on a read-only descriptor, so the agent only needs read access to /dev/pf.
// Counter names come from the kernel (the same names `pfctl -si -v` prints), so dimension IDs follow
// whatever the running kernel exports.

#define CONFIG_SECTION_PF "plugin:freebsd:pf"
#define PF_DEVICE "/dev/pf"
#define PF_NV_INITIAL_SIZE 4096
#define PF_NV_MAX_SIZE (1024 * 1024)
#define PF_IFACES_INITIAL 64
#define PF_IFACES_MAX 4096 // the kernel rejects requests above net.pf.request_maxcount (65535 by default)
#define DEFAULT_EXCLUDED_PF_INTERFACES "lo* pflog* pfsync* usbus*"

// --------------------------------------------------------------------------------------------------------------------
// named counter charts, built from the kernel's {counters, names} nvlists

struct pf_counter_chart {
    const char *nvl_name;       // nvlist key in the DIOCGETSTATUSNV reply
    const char *config_key;     // option name under [plugin:freebsd:pf]
    const char *id;
    const char *title;
    const char *units;
    const char *family;
    long priority;

    int enabled;
    RRDSET *st;
    size_t dims;
    RRDDIM **rd;
};

static struct pf_counter_chart pf_counter_charts[] = {
    {
        .nvl_name = "fcounters",
        .config_key = "state table operations",
        .id = "state_operations",
        .title = "pf State Table Operations",
        .units = "operations/s",
        .family = "states",
        .priority = NETDATA_CHART_PRIO_PF_STATE_OPERATIONS,
    },
    {
        .nvl_name = "counters",
        .config_key = "packet counters",
        .id = "packets",
        .title = "pf Packets by Reason",
        .units = "packets/s",
        .family = "packets",
        .priority = NETDATA_CHART_PRIO_PF_PACKETS,
    },
    {
        .nvl_name = "lcounters",
        .config_key = "limit counters",
        .id = "limits",
        .title = "pf Limit Events",
        .units = "events/s",
        .family = "limits",
        .priority = NETDATA_CHART_PRIO_PF_LIMITS,
    },
    {
        .nvl_name = "scounters",
        .config_key = "source node operations",
        .id = "source_node_operations",
        .title = "pf Source Node Operations",
        .units = "operations/s",
        .family = "source nodes",
        .priority = NETDATA_CHART_PRIO_PF_SRC_NODE_OPERATIONS,
    },
    {.nvl_name = NULL}
};

static void pf_counter_chart_update(struct pf_counter_chart *c, const nvlist_t *status, int update_every) {
    if (!nvlist_exists_nvlist(status, c->nvl_name))
        return;

    const nvlist_t *nvc = nvlist_get_nvlist(status, c->nvl_name);
    if (!nvlist_exists_number_array(nvc, "counters") || !nvlist_exists_string_array(nvc, "names"))
        return;

    size_t ncounters, nnames;
    const uint64_t *counters = nvlist_get_number_array(nvc, "counters", &ncounters);
    const char *const *names = nvlist_get_string_array(nvc, "names", &nnames);
    size_t n = MIN(ncounters, nnames);
    if (!n)
        return;

    if (unlikely(!c->st)) {
        c->st = rrdset_create_localhost(
            "pf",
            c->id,
            NULL,
            c->family,
            NULL,
            c->title,
            c->units,
            "freebsd.plugin",
            "pf",
            c->priority,
            update_every,
            RRDSET_TYPE_LINE);
    }

    // the kernel's counter list is fixed for its lifetime; grow only if a newer kernel exports more
    if (unlikely(c->dims < n)) {
        c->rd = reallocz(c->rd, n * sizeof(RRDDIM *));
        for (size_t i = c->dims; i < n; i++) {
            // kernel names contain spaces and dashes ("max states per rule", "bad-offset"):
            // use them as display names, and an underscore form as the dimension ID
            char id[RRD_ID_LENGTH_MAX + 1];
            strncpyz(id, names[i], sizeof(id) - 1);
            for (char *s = id; *s; s++)
                if (*s == ' ' || *s == '-')
                    *s = '_';

            c->rd[i] = rrddim_add(c->st, id, names[i], 1, 1, RRD_ALGORITHM_INCREMENTAL);
        }
        c->dims = n;
    }

    for (size_t i = 0; i < n; i++)
        rrddim_set_by_pointer(c->st, c->rd[i], (collected_number)counters[i]);

    rrdset_done(c->st);
}

// --------------------------------------------------------------------------------------------------------------------
// per-interface counters

struct pf_interface {
    char name[IFNAMSIZ];
    uint32_t hash;
    bool updated;

    RRDSET *st_packets;
    RRDDIM *rd_packets_in_pass, *rd_packets_in_block, *rd_packets_out_pass, *rd_packets_out_block;

    RRDSET *st_traffic;
    RRDDIM *rd_traffic_in_pass, *rd_traffic_in_block, *rd_traffic_out_pass, *rd_traffic_out_block;

    struct pf_interface *prev, *next;
};

static struct pf_interface *pf_interfaces_root = NULL;

static struct pf_interface *pf_interface_get(const char *name) {
    uint32_t hash = simple_hash(name);

    for (struct pf_interface *pi = pf_interfaces_root; pi; pi = pi->next)
        if (pi->hash == hash && !strcmp(pi->name, name))
            return pi;

    struct pf_interface *pi = callocz(1, sizeof(*pi));
    strncpyz(pi->name, name, sizeof(pi->name) - 1);
    pi->hash = hash;
    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(pf_interfaces_root, pi, prev, next);
    return pi;
}

static void pf_interface_free(struct pf_interface *pi) {
    if (pi->st_packets)
        rrdset_is_obsolete___safe_from_collector_thread(pi->st_packets);
    if (pi->st_traffic)
        rrdset_is_obsolete___safe_from_collector_thread(pi->st_traffic);

    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(pf_interfaces_root, pi, prev, next);
    freez(pi);
}

// interfaces missing from a successful DIOCIGETIFACES snapshot were removed from pf
static void pf_interfaces_cleanup(void) {
    struct pf_interface *pi = pf_interfaces_root;
    while (pi) {
        struct pf_interface *next = pi->next;
        if (!pi->updated)
            pf_interface_free(pi);
        else
            pi->updated = false;
        pi = next;
    }
}

static RRDSET *pf_interface_chart_create(struct pf_interface *pi, const char *suffix, const char *context,
                                         const char *title, const char *units, long priority, int update_every) {
    char id[RRD_ID_LENGTH_MAX + 1];
    snprintfz(id, sizeof(id), "%s_%s", pi->name, suffix);

    RRDSET *st = rrdset_create_localhost(
        "pf",
        id,
        NULL,
        "interfaces",
        context,
        title,
        units,
        "freebsd.plugin",
        "pf",
        priority,
        update_every,
        RRDSET_TYPE_LINE);

    rrdlabels_add(st->rrdlabels, "device", pi->name, RRDLABEL_SRC_AUTO);
    return st;
}

// pfik_packets / pfik_bytes are indexed [address family: inet, inet6][direction: in, out][action: pass, block]
static inline uint64_t pf_kif_sum_af(const uint64_t v[2][2][2], int dir, int action) {
    return v[0][dir][action] + v[1][dir][action];
}

static void pf_interface_update(struct pf_interface *pi, const struct pfi_kif *kif, int do_packets, int do_traffic,
                                int update_every) {
    if (do_packets) {
        if (unlikely(!pi->st_packets)) {
            pi->st_packets = pf_interface_chart_create(pi, "packets", "pf.interface_packets",
                                                       "pf Interface Packets", "packets/s",
                                                       NETDATA_CHART_PRIO_PF_INTERFACE_PACKETS, update_every);
            pi->rd_packets_in_pass   = rrddim_add(pi->st_packets, "in_pass",   NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            pi->rd_packets_in_block  = rrddim_add(pi->st_packets, "in_block",  NULL,  1, 1, RRD_ALGORITHM_INCREMENTAL);
            pi->rd_packets_out_pass  = rrddim_add(pi->st_packets, "out_pass",  NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
            pi->rd_packets_out_block = rrddim_add(pi->st_packets, "out_block", NULL, -1, 1, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(pi->st_packets, pi->rd_packets_in_pass,   (collected_number)pf_kif_sum_af(kif->pfik_packets, 0, 0));
        rrddim_set_by_pointer(pi->st_packets, pi->rd_packets_in_block,  (collected_number)pf_kif_sum_af(kif->pfik_packets, 0, 1));
        rrddim_set_by_pointer(pi->st_packets, pi->rd_packets_out_pass,  (collected_number)pf_kif_sum_af(kif->pfik_packets, 1, 0));
        rrddim_set_by_pointer(pi->st_packets, pi->rd_packets_out_block, (collected_number)pf_kif_sum_af(kif->pfik_packets, 1, 1));
        rrdset_done(pi->st_packets);
    }

    if (do_traffic) {
        if (unlikely(!pi->st_traffic)) {
            pi->st_traffic = pf_interface_chart_create(pi, "traffic", "pf.interface_traffic",
                                                       "pf Interface Traffic", "kilobits/s",
                                                       NETDATA_CHART_PRIO_PF_INTERFACE_TRAFFIC, update_every);
            pi->rd_traffic_in_pass   = rrddim_add(pi->st_traffic, "in_pass",   NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            pi->rd_traffic_in_block  = rrddim_add(pi->st_traffic, "in_block",  NULL,  8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            pi->rd_traffic_out_pass  = rrddim_add(pi->st_traffic, "out_pass",  NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
            pi->rd_traffic_out_block = rrddim_add(pi->st_traffic, "out_block", NULL, -8, BITS_IN_A_KILOBIT, RRD_ALGORITHM_INCREMENTAL);
        }

        rrddim_set_by_pointer(pi->st_traffic, pi->rd_traffic_in_pass,   (collected_number)pf_kif_sum_af(kif->pfik_bytes, 0, 0));
        rrddim_set_by_pointer(pi->st_traffic, pi->rd_traffic_in_block,  (collected_number)pf_kif_sum_af(kif->pfik_bytes, 0, 1));
        rrddim_set_by_pointer(pi->st_traffic, pi->rd_traffic_out_pass,  (collected_number)pf_kif_sum_af(kif->pfik_bytes, 1, 0));
        rrddim_set_by_pointer(pi->st_traffic, pi->rd_traffic_out_block, (collected_number)pf_kif_sum_af(kif->pfik_bytes, 1, 1));
        rrdset_done(pi->st_traffic);
    }
}

// --------------------------------------------------------------------------------------------------------------------
// ioctl helpers

// DIOCGETSTATUSNV returns ENOSPC when the buffer is too small; grow it like libpfctl does, but bounded.
static nvlist_t *pf_get_status(int dev) {
    static void *buf = NULL;
    static size_t buf_size = 0;

    if (unlikely(!buf)) {
        buf_size = PF_NV_INITIAL_SIZE;
        buf = mallocz(buf_size);
    }

    for (;;) {
        struct pfioc_nv nv = {.data = buf, .len = 0, .size = buf_size};

        if (ioctl(dev, DIOCGETSTATUSNV, &nv) == 0)
            return nvlist_unpack(buf, nv.len, 0);

        if (errno != ENOSPC || buf_size >= PF_NV_MAX_SIZE)
            return NULL;

        buf_size *= 2;
        buf = reallocz(buf, buf_size);
    }
}

// Returns the number of entries copied into *kifs, or -1 on error.
static int pf_get_interfaces(int dev, struct pfi_kif **kifs, int *kifs_size) {
    if (unlikely(!*kifs)) {
        *kifs_size = PF_IFACES_INITIAL;
        *kifs = mallocz(*kifs_size * sizeof(struct pfi_kif));
    }

    for (;;) {
        struct pfioc_iface io = {
            .pfiio_name = "",
            .pfiio_buffer = *kifs,
            .pfiio_esize = sizeof(struct pfi_kif),
            .pfiio_size = *kifs_size,
        };

        if (ioctl(dev, DIOCIGETIFACES, &io) != 0)
            return -1;

        // the kernel reports how many entries exist; retry if they did not all fit
        if (io.pfiio_size <= *kifs_size)
            return io.pfiio_size;

        if (*kifs_size >= PF_IFACES_MAX)
            return *kifs_size;

        *kifs_size = MIN(io.pfiio_size * 2, PF_IFACES_MAX);
        *kifs = reallocz(*kifs, *kifs_size * sizeof(struct pfi_kif));
    }
}

// --------------------------------------------------------------------------------------------------------------------
// pf

int do_pf(int update_every, usec_t dt) {
    (void)dt;

    static int pf_dev = -1;
    static int do_states = -1, do_src_nodes = -1, do_if_packets = -1, do_if_traffic = -1;
    static SIMPLE_PATTERN *excluded_interfaces = NULL;
    static bool status_error_logged = false, ifaces_error_logged = false;

    if (unlikely(do_states == -1)) {
        do_states     = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PF, "state table", 1);
        do_src_nodes  = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PF, "source nodes", 1);
        do_if_packets = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PF, "interface packets", 1);
        do_if_traffic = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PF, "interface traffic", 1);

        for (struct pf_counter_chart *c = pf_counter_charts; c->nvl_name; c++)
            c->enabled = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_PF, c->config_key, 1);

        excluded_interfaces = simple_pattern_create(
            inicfg_get(&netdata_config, CONFIG_SECTION_PF, "disable interfaces matching", DEFAULT_EXCLUDED_PF_INTERFACES),
            NULL,
            SIMPLE_PATTERN_EXACT,
            true);
    }

    if (unlikely(pf_dev == -1)) {
        pf_dev = open(PF_DEVICE, O_RDONLY | O_CLOEXEC);
        if (pf_dev == -1) {
            if (errno == ENOENT || errno == ENXIO)
                collector_info("DISABLED: pf module (%s not available, pf(4) is not loaded)", PF_DEVICE);
            else
                collector_error("DISABLED: pf module (cannot open %s: %s; the netdata user needs read access to it)",
                                PF_DEVICE, strerror(errno));
            return 1;
        }
    }

    // ----------------------------------------------------------------------------------------------------------------
    // global status

    nvlist_t *status = pf_get_status(pf_dev);
    if (unlikely(!status)) {
        if (!status_error_logged) {
            collector_error("pf: DIOCGETSTATUSNV on %s failed: %s", PF_DEVICE, strerror(errno));
            status_error_logged = true;
        }
    } else {
        status_error_logged = false;

        // when pf is disabled the counters are frozen; leave a gap instead of charting stale values
        bool running = nvlist_exists_bool(status, "running") && nvlist_get_bool(status, "running");

        if (running) {
            if (do_states && nvlist_exists_number(status, "states")) {
                static RRDSET *st = NULL;
                static RRDDIM *rd_states = NULL, *rd_halfopen = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                        "pf",
                        "states",
                        NULL,
                        "states",
                        NULL,
                        "pf State Table Entries",
                        "states",
                        "freebsd.plugin",
                        "pf",
                        NETDATA_CHART_PRIO_PF_STATES,
                        update_every,
                        RRDSET_TYPE_LINE);

                    rd_states = rrddim_add(st, "current", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }

                // halfopen_states is exported by newer kernels only
                if (unlikely(!rd_halfopen && nvlist_exists_number(status, "halfopen_states")))
                    rd_halfopen = rrddim_add(st, "halfopen", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                rrddim_set_by_pointer(st, rd_states, (collected_number)nvlist_get_number(status, "states"));
                if (rd_halfopen && nvlist_exists_number(status, "halfopen_states"))
                    rrddim_set_by_pointer(st, rd_halfopen, (collected_number)nvlist_get_number(status, "halfopen_states"));
                rrdset_done(st);
            }

            if (do_src_nodes && nvlist_exists_number(status, "src_nodes")) {
                static RRDSET *st = NULL;
                static RRDDIM *rd = NULL;

                if (unlikely(!st)) {
                    st = rrdset_create_localhost(
                        "pf",
                        "source_nodes",
                        NULL,
                        "source nodes",
                        NULL,
                        "pf Source Tracking Nodes",
                        "nodes",
                        "freebsd.plugin",
                        "pf",
                        NETDATA_CHART_PRIO_PF_SRC_NODES,
                        update_every,
                        RRDSET_TYPE_LINE);

                    rd = rrddim_add(st, "current", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                }

                rrddim_set_by_pointer(st, rd, (collected_number)nvlist_get_number(status, "src_nodes"));
                rrdset_done(st);
            }

            for (struct pf_counter_chart *c = pf_counter_charts; c->nvl_name; c++)
                if (c->enabled)
                    pf_counter_chart_update(c, status, update_every);
        }

        nvlist_destroy(status);
    }

    // ----------------------------------------------------------------------------------------------------------------
    // per-interface counters

    if (do_if_packets || do_if_traffic) {
        static struct pfi_kif *kifs = NULL;
        static int kifs_size = 0;

        int n = pf_get_interfaces(pf_dev, &kifs, &kifs_size);
        if (unlikely(n < 0)) {
            // keep the charts: a failed read is a gap, not a removal
            if (!ifaces_error_logged) {
                collector_error("pf: DIOCIGETIFACES on %s failed: %s", PF_DEVICE, strerror(errno));
                ifaces_error_logged = true;
            }
        } else {
            ifaces_error_logged = false;

            for (int i = 0; i < n; i++) {
                const struct pfi_kif *kif = &kifs[i];

                // interface groups (all, enc, openvpn, ...) aggregate their members; skipped interfaces are not filtered
                if (kif->pfik_group || (kif->pfik_flags & PFI_IFLAG_SKIP))
                    continue;

                char name[IFNAMSIZ];
                strncpyz(name, kif->pfik_name, sizeof(name) - 1);
                if (!*name || simple_pattern_matches(excluded_interfaces, name))
                    continue;

                struct pf_interface *pi = pf_interface_get(name);
                pi->updated = true;
                pf_interface_update(pi, kif, do_if_packets, do_if_traffic, update_every);
            }

            pf_interfaces_cleanup();
        }
    }

    return 0;
}
