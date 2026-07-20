// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RUN_DIR_H
#define NETDATA_RUN_DIR_H

#include "libnetdata/libnetdata.h"

/**
 * Initialize and get the runtime directory for Netdata
 * This function gets or creates the runtime directory based on environment or system defaults
 *
 * @param rw When true, create the directory if it doesn't exist
 * @return const char* The runtime directory path
 */
const char *os_run_dir(bool rw);

/**
 * Declare the uid netdata will switch to after the privilege drop.
 *
 * Only the daemon knows this ("run as user"), and it must call this before the
 * first os_run_dir() call. It matters for the world-writable /tmp fallback: the
 * first run creates /tmp/netdata as root and then chowns it to that uid, so
 * without this the directory would look like a third account's on restart.
 *
 * Other processes do not call it and do not need to: a plugin runs as that uid
 * (covered by geteuid()), and a setuid-root plugin still has it as its real uid
 * (covered by getuid()).
 */
void os_run_dir_set_target_uid(uid_t uid);

/**
 * Decide whether dir may be used as the run directory.
 *
 * Exported so the security-critical rule is directly testable: it must refuse a
 * symlinked final component, a non-directory, and any directory another account
 * could have planted or could replace. Trailing slashes are trimmed internally,
 * so a caller cannot accidentally disable the symlink check by passing one.
 */
bool os_run_dir_is_safe(const char *candidate, bool rw);

#endif //NETDATA_RUN_DIR_H
