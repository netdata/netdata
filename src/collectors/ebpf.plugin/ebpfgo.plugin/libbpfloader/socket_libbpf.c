//go:build netdata_ebpf_libbpf
// +build netdata_ebpf_libbpf

#include <errno.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include <bpf/bpf.h>
#include <bpf/libbpf.h>

/*
 * libbpf 0.0.9 (CentOS 7) compatibility shims — identical to those in
 * cachestat_libbpf.c so both files compile cleanly on old and new libbpf.
 */
#ifndef LIBBPF_MAJOR_VERSION
static inline int bpf_program__set_autoload(struct bpf_program *prog, bool autoload)
{
    (void)prog;
    (void)autoload;
    return 0;
}

static inline enum bpf_map_type bpf_map__type(const struct bpf_map *map)
{
    return bpf_map__def(map)->type;
}

static inline int bpf_map__set_type(struct bpf_map *map, enum bpf_map_type type)
{
    ((struct bpf_map_def *)bpf_map__def(map))->type = type;
    return 0;
}
#endif /* !LIBBPF_MAJOR_VERSION */

enum netdata_ebpf_socket_runtime_kind {
    NETDATA_SOCKET_RUNTIME_LEGACY = 0,
    NETDATA_SOCKET_RUNTIME_CORE   = 1,
};

struct netdata_ebpf_socket_runtime {
    int kind;
    struct bpf_object *obj;
    struct bpf_link **links;
    int nlinks;
};

/* -------------------------------------------------------------------------
 * Autoload helpers
 * ---------------------------------------------------------------------- */

static void socket_disable_programs_with_suffix(struct bpf_object *obj, const char *suffix)
{
    struct bpf_program *prog;
    bpf_object__for_each_program(prog, obj)
    {
        if (strstr(bpf_program__name(prog), suffix))
            bpf_program__set_autoload(prog, false);
    }
}

/* In legacy mode disable fentry/fexit; in CO-RE mode disable kprobe/kretprobe. */
static void socket_prepare_autoload(struct bpf_object *obj, int use_core)
{
    if (use_core) {
        socket_disable_programs_with_suffix(obj, "_kprobe");
        socket_disable_programs_with_suffix(obj, "_kretprobe");
    } else {
        socket_disable_programs_with_suffix(obj, "_fentry");
        socket_disable_programs_with_suffix(obj, "_fexit");
    }
}

/* -------------------------------------------------------------------------
 * Map type configuration
 * ---------------------------------------------------------------------- */

static void socket_update_map_types(struct bpf_object *obj, int maps_per_core)
{
    /* Only socket_global benefits from per-CPU map type toggling.
     * The open-socket and UDP session hashes are keyed by 5-tuple and are
     * not per-CPU (each socket entry is unique across the system). */
    struct bpf_map *map = bpf_object__find_map_by_name(obj, "socket_global");
    if (!map)
        return;

    enum bpf_map_type type = bpf_map__type(map);
    if (maps_per_core) {
        if (type == BPF_MAP_TYPE_ARRAY)
            bpf_map__set_type(map, BPF_MAP_TYPE_PERCPU_ARRAY);
    } else {
        if (type == BPF_MAP_TYPE_PERCPU_ARRAY)
            bpf_map__set_type(map, BPF_MAP_TYPE_ARRAY);
    }
}

/* -------------------------------------------------------------------------
 * Link management
 * ---------------------------------------------------------------------- */

static void socket_destroy_links(struct netdata_ebpf_socket_runtime *rt)
{
    if (!rt || !rt->links)
        return;

    for (int i = 0; i < rt->nlinks; i++) {
        if (rt->links[i])
            bpf_link__destroy(rt->links[i]);
    }

    free(rt->links);
    rt->links  = NULL;
    rt->nlinks = 0;
}

/* -------------------------------------------------------------------------
 * Attach — kprobe / kretprobe (legacy mode)
 * ---------------------------------------------------------------------- */

struct socket_kprobe_target {
    const char *prog_name;
    const char *kernel_func;
    bool retprobe;
    bool optional; /* tcp_v6_connect may be absent on some kernels */
};

static const struct socket_kprobe_target socket_kprobe_targets[] = {
    {"netdata_inet_csk_accept_kretprobe", "inet_csk_accept",    true,  false},
    {"netdata_tcp_retransmit_skb_kprobe", "tcp_retransmit_skb", false, false},
    {"netdata_tcp_cleanup_rbuf_kprobe",   "tcp_cleanup_rbuf",   false, false},
    {"netdata_tcp_close_kprobe",          "tcp_close",          false, false},
    {"netdata_udp_recvmsg_kprobe",        "udp_recvmsg",        false, false},
    {"netdata_udp_recvmsg_kretprobe",     "udp_recvmsg",        true,  false},
    {"netdata_tcp_sendmsg_kprobe",        "tcp_sendmsg",        false, false},
    {"netdata_tcp_sendmsg_kretprobe",     "tcp_sendmsg",        true,  false},
    {"netdata_udp_sendmsg_kprobe",        "udp_sendmsg",        false, false},
    {"netdata_udp_sendmsg_kretprobe",     "udp_sendmsg",        true,  false},
    {"netdata_tcp_v4_connect_kprobe",     "tcp_v4_connect",     false, false},
    {"netdata_tcp_v4_connect_kretprobe",  "tcp_v4_connect",     true,  false},
    {"netdata_tcp_v6_connect_kprobe",     "tcp_v6_connect",     false, true},
    {"netdata_tcp_v6_connect_kretprobe",  "tcp_v6_connect",     true,  true},
};

#define SOCKET_KPROBE_TARGET_COUNT \
    (sizeof(socket_kprobe_targets) / sizeof(socket_kprobe_targets[0]))

static int socket_attach_kprobes(struct netdata_ebpf_socket_runtime *rt)
{
    rt->links = calloc(SOCKET_KPROBE_TARGET_COUNT, sizeof(*rt->links));
    if (!rt->links)
        return -1;

    for (size_t i = 0; i < SOCKET_KPROBE_TARGET_COUNT; i++) {
        const struct socket_kprobe_target *t = &socket_kprobe_targets[i];

        struct bpf_program *prog = bpf_object__find_program_by_name(rt->obj, t->prog_name);
        if (!prog) {
            if (!t->optional)
                fprintf(stderr, "ebpf-go: socket kprobe program %s not found\n", t->prog_name);
            continue;
        }

        struct bpf_link *link = bpf_program__attach_kprobe(prog, t->retprobe, t->kernel_func);
        if (!link || libbpf_get_error(link)) {
            if (!t->optional) {
                fprintf(stderr, "ebpf-go: attach %s -> %s failed (errno %d)\n",
                        t->prog_name, t->kernel_func, errno);
                socket_destroy_links(rt);
                return -1;
            }
            continue;
        }

        rt->links[rt->nlinks++] = link;
    }

    return 0;
}

/* -------------------------------------------------------------------------
 * Attach — fentry / fexit (CO-RE mode)
 * ---------------------------------------------------------------------- */

static int socket_attach_core(struct netdata_ebpf_socket_runtime *rt)
{
    /* Count programs so we know how much to allocate. */
    int count = 0;
    struct bpf_program *prog;
    bpf_object__for_each_program(prog, rt->obj)
        count++;

    if (count == 0)
        return 0;

    rt->links = calloc((size_t)count, sizeof(*rt->links));
    if (!rt->links)
        return -1;

    bpf_object__for_each_program(prog, rt->obj)
    {
        struct bpf_link *link = bpf_program__attach(prog);
        if (!link || libbpf_get_error(link)) {
            fprintf(stderr, "ebpf-go: socket CO-RE attach failed for %s (errno %d)\n",
                    bpf_program__name(prog), errno);
            socket_destroy_links(rt);
            return -1;
        }
        rt->links[rt->nlinks++] = link;
    }

    return 0;
}

/* -------------------------------------------------------------------------
 * Public API
 * ---------------------------------------------------------------------- */

struct netdata_ebpf_socket_runtime *netdata_socket_runtime_open_mode(const char *path, int use_core)
{
    struct netdata_ebpf_socket_runtime *rt = calloc(1, sizeof(*rt));
    if (!rt)
        return NULL;

    /* Socket has no skeleton support; always open via raw object API. */
    (void)use_core;

    struct bpf_object *obj = bpf_object__open_file(path, NULL);
    if (!obj || libbpf_get_error(obj)) {
        if (obj && libbpf_get_error(obj))
            bpf_object__close(obj);
        free(rt);
        return NULL;
    }

    rt->obj  = obj;
    rt->kind = use_core ? NETDATA_SOCKET_RUNTIME_CORE : NETDATA_SOCKET_RUNTIME_LEGACY;
    return rt;
}

int netdata_socket_runtime_prepare(struct netdata_ebpf_socket_runtime *rt, int maps_per_core)
{
    if (!rt || !rt->obj)
        return -1;

    socket_prepare_autoload(rt->obj, rt->kind == NETDATA_SOCKET_RUNTIME_CORE);
    socket_update_map_types(rt->obj, maps_per_core);
    return 0;
}

int netdata_socket_runtime_load(struct netdata_ebpf_socket_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;

    return bpf_object__load(rt->obj);
}

int netdata_socket_runtime_attach(struct netdata_ebpf_socket_runtime *rt)
{
    if (!rt || !rt->obj)
        return -1;

    if (rt->links)
        socket_destroy_links(rt);

    if (rt->kind == NETDATA_SOCKET_RUNTIME_CORE)
        return socket_attach_core(rt);

    return socket_attach_kprobes(rt);
}

void netdata_socket_runtime_close(struct netdata_ebpf_socket_runtime *rt)
{
    if (!rt)
        return;

    socket_destroy_links(rt);

    if (rt->obj)
        bpf_object__close(rt->obj);

    free(rt);
}
