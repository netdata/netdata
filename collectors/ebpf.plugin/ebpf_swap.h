// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_SWAP_H
#define NETDATA_EBPF_SWAP_H 1

extern void *ebpf_swap_thread(void *ptr);
extern void ebpf_swap_create_apps_charts(struct ebpf_module *em, void *ptr);

#endif