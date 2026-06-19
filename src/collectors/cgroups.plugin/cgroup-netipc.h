// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_NETIPC_H
#define NETDATA_CGROUP_NETIPC_H 1

struct cgroup;

#ifdef OS_LINUX
void cgroup_netipc_init(void);
void cgroup_netipc_cleanup(void);
void cgroup_snapshot_rebuild_and_publish(void);
#else
static inline void cgroup_netipc_init(void) {}
static inline void cgroup_netipc_cleanup(void) {}
static inline void cgroup_snapshot_rebuild_and_publish(void) {}
#endif

#if defined(OS_LINUX) && defined(ENABLE_CGROUPS_LOOKUP_SERVER)
void cgroup_netipc_lookup_init(void);
void cgroup_netipc_lookup_cleanup(void);
void cgroup_netipc_lookup_update_charts(int update_every);
#ifdef NETDATA_INTERNAL_CHECKS
void cgroup_netipc_lookup_init_for_testing(const char *run_dir, const char *service_name);
#endif
#else
static inline void cgroup_netipc_lookup_init(void) {
    // No-op when the CGROUPS_LOOKUP server is disabled or unavailable.
}
static inline void cgroup_netipc_lookup_cleanup(void) {
    // No-op when the CGROUPS_LOOKUP server is disabled or unavailable.
}
static inline void cgroup_netipc_lookup_update_charts(int update_every) { (void)update_every; }
#endif

#endif // NETDATA_CGROUP_NETIPC_H
