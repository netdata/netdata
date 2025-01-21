// SPDX-License-Identifier: GPL-3.0-or-later

#include "network-viewer.bpf.h"

#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char _license[] SEC("license") = "GPL";

struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, 1000);
    __type(key, struct bpf_connection_key);
    __type(value, struct bpf_connection_data);
} connections SEC(".maps");

static __always_inline bool is_localhost_ipv4(u32 ip) {
    // compare in network byte order of 127.0.0.0/8
    const unsigned char *ip_bytes = (const unsigned char *)&ip;
    return ip_bytes[0] == 127;
}

static __always_inline bool is_localhost_ipv6(const struct in6_addr *addr) {
    if( addr->in6_u.u6_addr8[0]  != 0 ||
        addr->in6_u.u6_addr8[1]  != 0 ||
        addr->in6_u.u6_addr8[2]  != 0 ||
        addr->in6_u.u6_addr8[3]  != 0 ||
        addr->in6_u.u6_addr8[4]  != 0 ||
        addr->in6_u.u6_addr8[5]  != 0 ||
        addr->in6_u.u6_addr8[6]  != 0 ||
        addr->in6_u.u6_addr8[7]  != 0 ||
        addr->in6_u.u6_addr8[8]  != 0 ||
        addr->in6_u.u6_addr8[9]  != 0
    ) return false;

    // Check if the first 15 bytes are 0
    if( addr->in6_u.u6_addr8[10] == 0 &&
        addr->in6_u.u6_addr8[11] == 0 &&
        addr->in6_u.u6_addr8[12] == 0 &&
        addr->in6_u.u6_addr8[13] == 0 &&
        addr->in6_u.u6_addr8[14] == 0 &&
        addr->in6_u.u6_addr8[15] == 1)
        return true;

    return
        addr->in6_u.u6_addr8[10] == 0xff &&
        addr->in6_u.u6_addr8[11] == 0xff &&
        addr->in6_u.u6_addr8[12] == 127;
}

static __always_inline void update_outbound_connection(struct sock *sk, bool new_socket, u16 dst_port, int state, u64 bytes) {
    bpf_trace_printk("XXXXX\n", 6);

    if(!sk) return;

    struct bpf_connection_key key = {};
    struct bpf_connection_data data = {};

    struct sock_common sc;
    if(!bpf_probe_read_kernel(&sc, sizeof(sc), &sk->__sk_common))
        return;

    u16 family = sc.skc_family;

    if (family == AF_INET) {
        // IPv4 handling

        key.src_ip.ipv4 = sc.skc_rcv_saddr;
        key.dst_ip.ipv4 = sc.skc_daddr;

        if(is_localhost_ipv4(key.dst_ip.ipv4))
            return;
    }
    else if (family == AF_INET6) {
        // IPv6 handling

        key.src_ip.ipv6 = sc.skc_v6_rcv_saddr;
        key.dst_ip.ipv6 = sc.skc_v6_daddr;

        if(is_localhost_ipv6(&key.dst_ip.ipv6))
            return;
    } else {
        // Unsupported family
        return;
    }

    // Port extraction
    key.src_port = sc.skc_num;
    key.dst_port = dst_port ? dst_port : sc.skc_dport;

    key.pid = bpf_get_current_pid_tgid() >> 32;
    key.family = family;
    if(!bpf_probe_read_kernel(&key.protocol, sizeof(key.protocol), &sk->sk_protocol))
        return;

    if(key.protocol != IPPROTO_TCP && key.protocol != IPPROTO_UDP)
        return;

    // find the entry in the hashtable
    struct bpf_connection_data *d = bpf_map_lookup_elem(&connections, &key);
    if(d) {
        d->timestamp_last_seen = bpf_ktime_get_ns();
        d->state = state;
        d->total_bytes_sent += bytes;
    }
    else if (new_socket) {
        data.timestamp_first_seen = data.timestamp_last_seen = bpf_ktime_get_ns();
        data.state = state;
        data.total_bytes_sent = bytes;
        data.type = CONN_TYPE_DETECTED;
        bpf_get_current_comm(&data.comm, sizeof(data.comm));
        bpf_map_update_elem(&connections, &key, &data, BPF_ANY);
    }
}

SEC("kprobe/tcp_set_state")
int BPF_KPROBE(tcp_set_state, struct sock *sk, int state) {
    (void)ctx;

    bpf_trace_printk("XXXXX\n", 6);

    if(!sk) return 0;

    update_outbound_connection(sk, false, 0, state, 0);
    return 0;
}

SEC("kprobe/tcp_v4_connect")
int BPF_KPROBE(tcp_v4_connect, struct sock *sk) {
    (void)ctx;

    bpf_trace_printk("XXXXX\n", 6);

    if(!sk) return 0;
    update_outbound_connection(sk, true, 0, 0, 0);
    return 0;
}

SEC("kprobe/tcp_v6_connect")
int BPF_KPROBE(tcp_v6_connect, struct sock *sk) {
    (void)ctx;
    bpf_trace_printk("XXXXX\n", 6);
    if(!sk) return 0;
    update_outbound_connection(sk, true, 0, 0, 0);
    return 0;
}

SEC("kprobe/udp_sendmsg")
int BPF_KPROBE(udp_sendmsg, struct sock *sk, struct msghdr *msg, size_t size) {
    (void)ctx;

    bpf_trace_printk("XXXXX\n", 6);

    if (!sk || !msg) return 0;

    void *ptr = NULL;
    if(!bpf_probe_read_kernel(&ptr, sizeof(ptr), &msg->msg_name) || !ptr)
        return 0;

    u16 family;
    if(!bpf_probe_read_kernel(&family, sizeof(family), &sk->__sk_common.skc_family))
        return 0;

    u16 dst_port = 0;

    if (family == AF_INET) {
        // For IPv4

        struct sockaddr_in addr_in;
        if(!bpf_probe_read_kernel(&addr_in, sizeof(addr_in), ptr))
            return 0;

        dst_port = addr_in.sin_port;
    } else if (family == AF_INET6) {
        // For IPv6

        struct sockaddr_in6 addr_in6;
        if(!bpf_probe_read_kernel(&addr_in6, sizeof(addr_in6), ptr))
            return 0;

        dst_port = addr_in6.sin6_port;
    }

    update_outbound_connection(sk, true, dst_port, 0, size);
    return 0;
}
