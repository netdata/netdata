// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RUN_DIR_H
#define NETDATA_RUN_DIR_H

#include "libnetdata/libnetdata.h"

/**
 * Initialize and get the runtime directory for Netdata
 * This function gets or creates the runtime directory based on environment or system defaults
 *
 * The answer is cached for the process lifetime, per level of validation: a
 * read-only answer only reports a directory that already exists and is readable,
 * so the first rw=true caller re-runs the detection rather than inherit it. A
 * pointer already returned stays valid either way.
 *
 * @param rw When true, the directory must be writable by us, and is created if it
 *           does not exist. Returns NULL when there is no such directory, even if
 *           a read-only caller already found a readable one.
 * @return const char* The runtime directory path, or NULL
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
 *
 * rw is the level of validation, not just an access mode. With rw the directory is
 * about to be created, chowned and bound to while still root, so in a
 * world-writable sticky parent (the /tmp fallback) it must also belong to us or to
 * the declared "run as user". A read-only caller is exempt from that one
 * requirement: it only reads or connects, and it cannot know which uid the agent
 * runs as.
 */
bool os_run_dir_is_safe(const char *candidate, bool rw);

#endif //NETDATA_RUN_DIR_H
