// SPDX-License-Identifier: GPL-3.0-or-later

#include "network-viewer.bpf.h"
#include "network-viewer-userspace-bpf.h"

// ----------------------------------------------------------------------------------------------------------------
// initialize ebpf hashtable with existing outbound connections

uint32_t bpf_connection_key_size(void) {
    return sizeof(struct bpf_connection_key);
}

uint32_t bpf_connection_data_size(void) {
    return sizeof(struct bpf_connection_data);
}

void populate_connection_key_and_data(void *key, void *data, BPF_CONNECTION *c) {
    struct bpf_connection_key *k = key;
    struct bpf_connection_data *d = data;

    k->protocol = c->protocol;
    k->family = c->family;
    k->pid = c->pid;
    k->src_port = c->local.port;
    k->dst_port = c->remote.port;

    if(c->family == AF_INET) {
        k->src_ip.ipv4 = c->local.ip.ipv4;
        k->dst_ip.ipv4 = c->remote.ip.ipv4;
    }
    else if(c->family == AF_INET6) {
        k->src_ip.ipv6 = c->local.ip.ipv6;
        k->dst_ip.ipv6 = c->remote.ip.ipv6;
    }

    d->state = c->state;
    d->type = CONN_TYPE_LOADED;
    d->total_bytes_sent = 0;
    d->timestamp_last_seen = c->last_seen_s;
    d->timestamp_first_seen = c->first_seen_s;

    unsigned i;
    for(i = 0; i < sizeof(d->comm) - 1 && c->comm[i] ; i++) {
        d->comm[i] = c->comm[i];
    }
    d->comm[i] = '\0';
}

void populate_connection_from_key_and_data(BPF_CONNECTION *c, void *key, void *data) {
    struct bpf_connection_key *k = key;
    struct bpf_connection_data *d = data;

    c->protocol = k->protocol;
    c->family = k->family;
    c->pid = (pid_t)k->pid;
    c->local.port = k->src_port;
    c->remote.port = k->dst_port;

    if(k->family == AF_INET) {
        c->local.ip.ipv4 = k->src_ip.ipv4;
        c->remote.ip.ipv4 = k->dst_ip.ipv4;
    }
    else if(k->family == AF_INET6) {
        c->local.ip.ipv6 = k->src_ip.ipv6;
        c->remote.ip.ipv6 = k->dst_ip.ipv6;
    }

    c->state = d->state;
    c->type = d->type;
    c->first_seen_s = d->timestamp_first_seen;
    c->last_seen_s = d->timestamp_last_seen;

    unsigned i;
    for(i = 0; i < sizeof(c->comm) - 1 && d->comm[i] ; i++) {
        c->comm[i] = d->comm[i];
    }
    c->comm[i] = '\0';

}