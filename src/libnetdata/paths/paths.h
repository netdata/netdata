// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PATHS_H
#define NETDATA_PATHS_H

#include "../libnetdata.h"

size_t filename_from_path_entry(char out[FILENAME_MAX], const char *path, const char *entry, const char *extension);
char *filename_from_path_entry_strdupz(const char *path, const char *entry);

bool filename_is_file(const char *filename);
bool filename_is_dir(const char *filename, bool create_it);

// Open a file for writing secrets (private keys, claim tokens, bearer tokens).
// Truncates like fopen(filename, "w"), but the result is never accessible to
// group or other - it is set to 0600 both when the file is created and when it
// already exists with wider permissions, and the only case where a different
// mode survives is a filesystem that refuses chmod on a file that is already
// owner-only (which may leave it tighter than 0600, never wider). fopen() would
// create it 0666 & ~umask, which with the daemon's umask(0007) leaves secrets
// readable and writable by the whole netdata group.
//
// The secret directories are group-writable, so symlinks and non-regular files
// at the target path are refused instead of being followed or written into. Where
// O_NOFOLLOW exists that is the open() itself. Elsewhere the path is lstat()-ed
// first, creation is forced with O_EXCL, and the opened inode is compared against
// what was inspected; that closes the symlink redirect but not a hardlink planted
// before the lstat(), so callers needing that guarantee too must write to a
// unique temporary file and rename() it into place instead.
//
// An existing file that cannot be chmod()-ed - chmod needs ownership, not write
// access, so a secret left behind by an agent that ran as root is writable
// through the netdata group yet cannot be tightened - is handled as follows: it
// is accepted as-is while nothing is exposed (already inaccessible to group and
// other), and otherwise removed and recreated at 0600. A secret is never written
// to a file this call could not make private.
//
// Returns NULL and leaves errno set on failure.
FILE *fopen_secret_write(const char *filename, const char *mode);

bool path_entry_is_file(const char *path, const char *entry);
bool path_entry_is_dir(const char *path, const char *entry, bool create_it);

void recursive_config_double_dir_load(
    const char *user_path
    , const char *stock_path
    , const char *entry
    , int (*callback)(const char *filename, void *data, bool stock_config)
    , void *data
    , size_t depth
);

// true when a host prefix would be unsafe to prepend to a printf format string.
// verify_netdata_host_prefix() applies this, and every other site that assigns
// netdata_configured_host_prefix calls that right after. exported for the one
// site that cannot - nd_log_initialize_for_external_plugins() - so the rule
// stays defined once, in paths.c, next to the call sites that motivate it.
bool netdata_host_prefix_has_format_specifier(const char *prefix);

// checks for fopen_secret_write(); paths_unittest() runs it, so it executes in CI
// through `netdata -W unittest`. exported for the standalone test target too.
int fopen_secret_write_unittest(void);

int paths_unittest(void);

#endif //NETDATA_PATHS_H
