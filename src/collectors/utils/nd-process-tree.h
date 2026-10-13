// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef NETDATA_ND_PROCESS_TREE_H
#define NETDATA_ND_PROCESS_TREE_H

// Called after nd-run applies its existing identity and environment policy.
// The final NDTREE1 frame proves child exhaustion; helper exit alone does not.
// NDTREE1 unavailable means setup failed before launch, with ECHILD verified.
int nd_process_tree_run(int control_fd, int status_fd, char **command);

#endif
