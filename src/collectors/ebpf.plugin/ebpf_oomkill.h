// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_OOMKILL_H
#define NETDATA_EBPF_OOMKILL_H 1

// Module description
#define NETDATA_EBPF_OOMKILL_MODULE_DESC "Show OOM kills for all applications recognized via the apps.plugin."

/*****************************************************************
 *  copied from kernel-collectors repo, with modifications needed
 *  for inclusion here.
 *****************************************************************/

#define NETDATA_OOMKILL_MAX_ENTRIES 64

typedef uint8_t oomkill_ebpf_val_t;

/*****************************************************************
 * below this is eBPF plugin-specific code.
 *****************************************************************/

#define NETDATA_EBPF_MODULE_NAME_OOMKILL "oomkill"
#define NETDATA_OOMKILL_CONFIG_FILE "oomkill.conf"

#define NETDATA_OOMKILL_CHART "_ebpf_oomkill"

#define EBPF_OOMKILL_UNIT_KILLS "kills/s"

// Contexts
#define NETDATA_CGROUP_OOMKILLS_CONTEXT "cgroup.oomkills"
#define NETDATA_SYSTEMD_OOMKILLS_CONTEXT "systemd.oomkills"

extern struct config oomkill_config;
void ebpf_oomkill_thread(void *ptr);
void ebpf_oomkill_create_apps_charts(struct ebpf_module *em, void *ptr);

#endif /* NETDATA_EBPF_OOMKILL_H */
