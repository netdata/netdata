// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CGROUP_LOOKUP_RESOLVER_H
#define NETDATA_CGROUP_LOOKUP_RESOLVER_H 1

#include "cgroup-snapshot-store.h"

void cgroup_lookup_resolver_init(void);
void cgroup_lookup_resolver_cleanup(void);

const CGROUP_SNAPSHOT_ENTRY *cgroup_lookup_resolver_resolve(
    const CGROUP_SNAPSHOT_STORE *store,
    uint64_t generation,
    const char *path,
    size_t path_len);

#ifdef NETDATA_INTERNAL_CHECKS
size_t cgroup_lookup_resolver_suffix_scans_for_testing(void);
#endif

#endif /* NETDATA_CGROUP_LOOKUP_RESOLVER_H */
