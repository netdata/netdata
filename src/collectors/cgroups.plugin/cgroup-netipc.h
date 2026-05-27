// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_NETIPC_H
#define NETDATA_CGROUP_NETIPC_H 1

struct cgroup;

#ifdef OS_LINUX
void cgroup_netipc_init(void);
void cgroup_netipc_cleanup(void);
#else
static inline void cgroup_netipc_init(void) {}
static inline void cgroup_netipc_cleanup(void) {}
#endif

#if defined(OS_LINUX) && defined(ENABLE_CGROUPS_LOOKUP_SERVER)
void cgroup_netipc_lookup_init(void);
void cgroup_netipc_lookup_cleanup(void);
void cgroup_netipc_lookup_update_charts(int update_every);
void cgroup_netipc_lookup_reaped_path_add(const char *path);
#ifdef NETDATA_INTERNAL_CHECKS
void cgroup_netipc_lookup_reaped_set_insert(const char *path);
void cgroup_netipc_lookup_init_for_testing(const char *run_dir, const char *service_name);
void cgroup_lookup_set_cgroup_root_for_testing(struct cgroup *root);
#endif
#else
static inline void cgroup_netipc_lookup_init(void) {}
static inline void cgroup_netipc_lookup_cleanup(void) {}
static inline void cgroup_netipc_lookup_update_charts(int update_every) { (void)update_every; }
static inline void cgroup_netipc_lookup_reaped_path_add(const char *path) { (void)path; }
#endif

#endif // NETDATA_CGROUP_NETIPC_H
