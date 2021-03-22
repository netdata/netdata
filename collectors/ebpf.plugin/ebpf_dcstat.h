// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_DCSTAT_H
#define NETDATA_EBPF_DCSTAT_H 1

extern void *ebpf_dcstat_thread(void *ptr);
extern void ebpf_dcstat_create_apps_charts(struct ebpf_module *em, void *ptr);

#endif // NETDATA_EBPF_DCSTAT_H
